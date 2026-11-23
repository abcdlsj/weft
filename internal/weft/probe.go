package weft

import (
	"context"
	"encoding/json"
	"log"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ProbeOK          = "ok"
	ProbeUnreachable = "unreachable"
	ProbeUnverified  = "unverified"

	DefaultProbeInterval    = 6 * time.Hour
	DefaultProbeTimeout     = 5 * time.Second
	DefaultProbeConcurrency = 4

	probeStartupMinJitter = 2 * time.Minute
	probeStartupMaxJitter = 5 * time.Minute
)

// ProbeConfig controls the server-side periodic node prober.
type ProbeConfig struct {
	Interval    time.Duration
	Timeout     time.Duration
	Concurrency int
	// InitialDelay overrides the 2~5 minute startup jitter when > 0.
	InitialDelay time.Duration
}

// ProbeResult is the outcome of probing one node from the server's viewpoint.
type ProbeResult struct {
	State      string // ok | unreachable | unverified
	DelayMS    int
	ResolvedV4 bool
	ResolvedV6 bool
	ProbedAt   int64
}

// ProbeUpdate binds a ProbeResult to a stored node row.
type ProbeUpdate struct {
	HostID string
	Key    string
	Result ProbeResult
}

// ProbeSummary aggregates one probe pass for logging and diagnostics.
type ProbeSummary struct {
	Total       int
	OK          int
	Unreachable int
	Unverified  int
	Duration    time.Duration
}

// Prober checks subscription and manual nodes from the server host without
// pulling in any proxy protocol engine.
type Prober struct {
	Timeout     time.Duration
	Concurrency int
	LookupIP    func(ctx context.Context, network, host string) ([]net.IP, error)
	DialContext func(ctx context.Context, network, address string) (net.Conn, error)
}

func NewProber(timeout time.Duration, concurrency int) *Prober {
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}
	if concurrency <= 0 {
		concurrency = DefaultProbeConcurrency
	}
	resolver := &net.Resolver{}
	dialer := &net.Dialer{Timeout: timeout}
	return &Prober{
		Timeout:     timeout,
		Concurrency: concurrency,
		LookupIP:    resolver.LookupIP,
		DialContext: dialer.DialContext,
	}
}

func isProbedSource(source string) bool {
	return source == NodeSub || source == NodeManual
}

// nodeAvailable reports whether a node counts as available in fleet totals.
// Agent-reported nodes use the agent's own liveness; subscription and manual
// nodes use server-side probe results.
func nodeAvailable(n Node) bool {
	if isProbedSource(n.Source) {
		return n.ProbeState == ProbeOK
	}
	return n.Alive
}

// isUDPNode classifies nodes whose transport is UDP-only, where a TCP connect
// cannot verify the service without speaking the proxy protocol.
func isUDPNode(n Node) bool {
	switch strings.ToLower(strings.TrimSpace(n.Type)) {
	case "hysteria", "hysteria2", "hy2", "tuic", "wireguard", "wg", "ss-udp", "snell-udp":
		return true
	}
	if len(n.Raw) > 0 {
		var m map[string]any
		if json.Unmarshal(n.Raw, &m) == nil {
			if netw, ok := m["network"].(string); ok {
				switch strings.ToLower(strings.TrimSpace(netw)) {
				case "udp", "quic", "kcp":
					return true
				}
			}
		}
	}
	return false
}

func (p *Prober) timeout() time.Duration {
	if p != nil && p.Timeout > 0 {
		return p.Timeout
	}
	return DefaultProbeTimeout
}

func (p *Prober) concurrency() int {
	if p != nil && p.Concurrency > 0 {
		return p.Concurrency
	}
	return DefaultProbeConcurrency
}

func (p *Prober) lookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	if p != nil && p.LookupIP != nil {
		return p.LookupIP(ctx, network, host)
	}
	return (&net.Resolver{}).LookupIP(ctx, network, host)
}

func (p *Prober) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if p != nil && p.DialContext != nil {
		return p.DialContext(ctx, network, address)
	}
	return (&net.Dialer{Timeout: p.timeout()}).DialContext(ctx, network, address)
}

func (p *Prober) resolveServer(ctx context.Context, host string) (v4, v6 []net.IP) {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return nil, nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return []net.IP{ip4}, nil
		}
		return nil, []net.IP{ip}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ips, err := p.lookupIP(ctx, "ip4", host)
		if err != nil {
			return
		}
		for _, ip := range ips {
			if ip4 := ip.To4(); ip4 != nil {
				v4 = append(v4, ip4)
			}
		}
	}()
	go func() {
		defer wg.Done()
		ips, err := p.lookupIP(ctx, "ip6", host)
		if err != nil {
			return
		}
		for _, ip := range ips {
			if ip.To4() == nil && len(ip) == net.IPv6len {
				v6 = append(v6, ip)
			}
		}
	}()
	wg.Wait()
	return v4, v6
}

// ProbeNode probes a single node.
//   - Both TCP and UDP nodes resolve A/AAAA records (or classify IP literals).
//   - UDP nodes stop at DNS resolution: resolvable => unverified, else unreachable.
//   - TCP nodes dial server:port; handshake duration is recorded as delay_ms.
func (p *Prober) ProbeNode(ctx context.Context, n Node) ProbeResult {
	res := ProbeResult{ProbedAt: now()}
	host := strings.Trim(strings.TrimSpace(n.Server), "[]")
	if host == "" || n.Port <= 0 || n.Port > 65535 {
		res.State = ProbeUnreachable
		return res
	}
	pctx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()

	v4, v6 := p.resolveServer(pctx, host)
	res.ResolvedV4 = len(v4) > 0
	res.ResolvedV6 = len(v6) > 0
	if !res.ResolvedV4 && !res.ResolvedV6 {
		res.State = ProbeUnreachable
		return res
	}
	if isUDPNode(n) {
		res.State = ProbeUnverified
		return res
	}

	// Prefer IPv4 first, then IPv6, bounded so multi-record domains don't burn
	// the entire timeout budget on dead records.
	candidates := make([]net.IP, 0, 4)
	for i := 0; i < len(v4) && i < 2; i++ {
		candidates = append(candidates, v4[i])
	}
	for i := 0; i < len(v6) && i < 2; i++ {
		candidates = append(candidates, v6[i])
	}
	portStr := strconv.Itoa(n.Port)
	for _, ip := range candidates {
		if pctx.Err() != nil {
			break
		}
		start := time.Now()
		conn, err := p.dialContext(pctx, "tcp", net.JoinHostPort(ip.String(), portStr))
		if err == nil {
			rtt := int(time.Since(start).Milliseconds())
			if rtt < 1 {
				rtt = 1
			}
			_ = conn.Close()
			res.State = ProbeOK
			res.DelayMS = rtt
			return res
		}
	}
	res.State = ProbeUnreachable
	return res
}

// ProbeAll probes all subscription and manual nodes across every host,
// regardless of bundle membership, and persists the results and host reports.
func (p *Prober) ProbeAll(ctx context.Context, st *Store) (ProbeSummary, error) {
	start := time.Now()
	all, err := st.ListNodes("")
	if err != nil {
		return ProbeSummary{}, err
	}
	var targets []Node
	for _, n := range all {
		if isProbedSource(n.Source) {
			targets = append(targets, n)
		}
	}
	if len(targets) == 0 {
		return ProbeSummary{Duration: time.Since(start)}, nil
	}

	updates := make([]ProbeUpdate, len(targets))
	jobs := make(chan int, len(targets))
	for i := range targets {
		jobs <- i
	}
	close(jobs)

	workers := p.concurrency()
	if workers > len(targets) {
		workers = len(targets)
	}
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if ctx.Err() != nil {
					return
				}
				n := targets[idx]
				res := p.ProbeNode(ctx, n)
				updates[idx] = ProbeUpdate{HostID: n.HostID, Key: n.Key, Result: res}
			}
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ProbeSummary{}, ctx.Err()
	}

	if err := st.SaveProbeResults(updates); err != nil {
		return ProbeSummary{}, err
	}

	summary := ProbeSummary{
		Total:    len(updates),
		Duration: time.Since(start),
	}
	for _, u := range updates {
		switch u.Result.State {
		case ProbeOK:
			summary.OK++
		case ProbeUnreachable:
			summary.Unreachable++
		case ProbeUnverified:
			summary.Unverified++
		}
	}
	return summary, nil
}

// RunProbeLoop waits a startup jitter window (2~5 minutes by default) and then
// runs a probe pass every cfg.Interval until ctx is cancelled.
func RunProbeLoop(ctx context.Context, st *Store, cfg ProbeConfig) {
	interval := cfg.Interval
	if interval <= 0 {
		interval = DefaultProbeInterval
	}
	prober := NewProber(cfg.Timeout, cfg.Concurrency)

	initial := cfg.InitialDelay
	if initial <= 0 {
		span := probeStartupMaxJitter - probeStartupMinJitter
		initial = probeStartupMinJitter + time.Duration(rand.Int64N(int64(span)+1))
	}

	timer := time.NewTimer(initial)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	for {
		summary, err := prober.ProbeAll(ctx, st)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("probe: %v", err)
		} else {
			log.Printf("probe: %d nodes (ok=%d unreachable=%d unverified=%d) in %s",
				summary.Total, summary.OK, summary.Unreachable, summary.Unverified,
				summary.Duration.Round(time.Millisecond))
		}

		ticker := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
		}
	}
}
