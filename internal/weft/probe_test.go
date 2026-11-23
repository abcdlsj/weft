package weft

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProbeNodeClassification(t *testing.T) {
	// Live TCP endpoint
	liveLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer liveLn.Close()
	go func() {
		for {
			c, err := liveLn.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	livePort := liveLn.Addr().(*net.TCPAddr).Port

	// Dead TCP endpoint (bound then immediately closed)
	deadLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadPort := deadLn.Addr().(*net.TCPAddr).Port
	_ = deadLn.Close()

	// Mock DNS table so classification tests never touch external DNS
	dnsTable := map[string]struct {
		v4 []net.IP
		v6 []net.IP
	}{
		"tcp-live.test": {v4: []net.IP{net.ParseIP("127.0.0.1")}},
		"tcp-dead.test": {v4: []net.IP{net.ParseIP("127.0.0.1")}},
		"v6-only.test":  {v6: []net.IP{net.ParseIP("2001:db8::1")}},
		"dual.test":     {v4: []net.IP{net.ParseIP("127.0.0.1")}, v6: []net.IP{net.ParseIP("2001:db8::2")}},
		"hy2-v4.test":   {v4: []net.IP{net.ParseIP("203.0.113.10")}},
		"hy2-v6.test":   {v6: []net.IP{net.ParseIP("2001:db8::99")}},
		"nxdomain.test": {},
	}

	prober := &Prober{
		Timeout:     2 * time.Second,
		Concurrency: 2,
		LookupIP: func(ctx context.Context, network, host string) ([]net.IP, error) {
			rec, ok := dnsTable[host]
			if !ok {
				return nil, errors.New("no such host")
			}
			switch network {
			case "ip4":
				if len(rec.v4) == 0 {
					return nil, errors.New("no A record")
				}
				return rec.v4, nil
			case "ip6":
				if len(rec.v6) == 0 {
					return nil, errors.New("no AAAA record")
				}
				return rec.v6, nil
			}
			return nil, errors.New("unsupported network")
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(address)
			if strings.HasPrefix(host, "2001:db8:") {
				return nil, errors.New("network is unreachable")
			}
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, address)
		},
	}

	ctx := context.Background()

	t.Run("tcp live", func(t *testing.T) {
		res := prober.ProbeNode(ctx, Node{Type: "vless", Server: "tcp-live.test", Port: livePort, Source: NodeSub})
		if res.State != ProbeOK || res.DelayMS < 1 || !res.ResolvedV4 || res.ResolvedV6 || res.ProbedAt == 0 {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("tcp dead port", func(t *testing.T) {
		res := prober.ProbeNode(ctx, Node{Type: "trojan", Server: "tcp-dead.test", Port: deadPort, Source: NodeSub})
		if res.State != ProbeUnreachable || res.DelayMS != 0 || !res.ResolvedV4 || res.ResolvedV6 {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("tcp ipv6-only unreachable without v6 route", func(t *testing.T) {
		res := prober.ProbeNode(ctx, Node{Type: "trojan", Server: "v6-only.test", Port: 443, Source: NodeSub})
		if res.State != ProbeUnreachable || res.ResolvedV4 || !res.ResolvedV6 {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("tcp dual-stack falls back to live v4", func(t *testing.T) {
		res := prober.ProbeNode(ctx, Node{Type: "ss", Server: "dual.test", Port: livePort, Source: NodeSub})
		if res.State != ProbeOK || !res.ResolvedV4 || !res.ResolvedV6 {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("udp resolvable stays unverified", func(t *testing.T) {
		for _, tc := range []struct {
			typ    string
			server string
			raw    string
			wantV4 bool
			wantV6 bool
		}{
			{"hysteria2", "hy2-v4.test", `{"type":"hysteria2"}`, true, false},
			{"tuic", "hy2-v6.test", `{"type":"tuic"}`, false, true},
			{"ss-udp", "hy2-v4.test", `{"type":"ss-udp"}`, true, false},
			{"vless", "hy2-v4.test", `{"type":"vless","network":"quic"}`, true, false},
		} {
			res := prober.ProbeNode(ctx, Node{
				Type: tc.typ, Server: tc.server, Port: 443, Source: NodeSub, Raw: json.RawMessage(tc.raw),
			})
			if res.State != ProbeUnverified || res.DelayMS != 0 || res.ResolvedV4 != tc.wantV4 || res.ResolvedV6 != tc.wantV6 {
				t.Errorf("%s/%s: got %+v, want unverified (v4=%v,v6=%v)", tc.typ, tc.server, res, tc.wantV4, tc.wantV6)
			}
		}
	})

	t.Run("udp unresolvable is unreachable", func(t *testing.T) {
		res := prober.ProbeNode(ctx, Node{Type: "hysteria2", Server: "nxdomain.test", Port: 443, Source: NodeSub})
		if res.State != ProbeUnreachable || res.ResolvedV4 || res.ResolvedV6 {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("ip literals classify without dns", func(t *testing.T) {
		res4 := prober.ProbeNode(ctx, Node{Type: "vmess", Server: "127.0.0.1", Port: livePort, Source: NodeManual})
		if res4.State != ProbeOK || !res4.ResolvedV4 || res4.ResolvedV6 {
			t.Fatalf("v4 literal: %+v", res4)
		}
		res6 := prober.ProbeNode(ctx, Node{Type: "hysteria2", Server: "2001:db8::42", Port: 443, Source: NodeManual})
		if res6.State != ProbeUnverified || res6.ResolvedV4 || !res6.ResolvedV6 {
			t.Fatalf("v6 literal: %+v", res6)
		}
	})
}

func TestProbeAllSkipsAgentWritesReportsAndUpdatesStats(t *testing.T) {
	liveLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer liveLn.Close()
	go func() {
		for {
			c, err := liveLn.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	livePort := liveLn.Addr().(*net.TCPAddr).Port

	deadLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadPort := deadLn.Addr().(*net.TCPAddr).Port
	_ = deadLn.Close()

	st, err := OpenStore(t.TempDir() + "/weft.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// 1) Airport host with 3 sub nodes (not in any bundle — probe must still cover them)
	airport, err := st.CreateSourceHost("pokemon", "https://example.com/sub")
	if err != nil {
		t.Fatal(err)
	}
	subNodes := []Node{
		{Key: "s-ok", Name: "SG-VLESS", Type: "vless", Server: "127.0.0.1", Port: livePort, Source: NodeSub, Raw: json.RawMessage(`{"type":"vless","server":"127.0.0.1","port":` + strconv.Itoa(livePort) + `}`)},
		{Key: "s-dead", Name: "HK-Trojan", Type: "trojan", Server: "127.0.0.1", Port: deadPort, Source: NodeSub, Raw: json.RawMessage(`{"type":"trojan","server":"127.0.0.1","port":` + strconv.Itoa(deadPort) + `}`)},
		{Key: "s-udp", Name: "JP-Hy2", Type: "hysteria2", Server: "127.0.0.1", Port: 8443, Source: NodeSub, Raw: json.RawMessage(`{"type":"hysteria2","server":"127.0.0.1","port":8443}`)},
	}
	if err := st.ReplaceSourceNodes(airport.ID, subNodes); err != nil {
		t.Fatal(err)
	}

	// 2) Agent host with 1 agent node (must NOT be touched by prober)
	agentHost, err := st.CreateHost("hk-01", HostAgent)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceNodes(agentHost.ID, []Node{
		{Key: "a-1", Name: "HK-Agent", Type: "vless", Server: "127.0.0.1", Port: deadPort, Alive: true, DelayMS: 77, Source: NodeAgent},
	}); err != nil {
		t.Fatal(err)
	}

	prober := NewProber(2*time.Second, 2)
	summary, err := prober.ProbeAll(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 3 || summary.OK != 1 || summary.Unreachable != 1 || summary.Unverified != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	// Verify agent node was completely untouched
	agentNode, err := st.NodeByKey(agentHost.ID, "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if !agentNode.Alive || agentNode.DelayMS != 77 || agentNode.ProbeState != "" || agentNode.ProbedAt != 0 {
		t.Fatalf("agent node was mutated by prober: %+v", agentNode)
	}

	// Verify sub nodes kept alive=false while getting probe_state/probed_at/delay_ms
	okNode, _ := st.NodeByKey(airport.ID, "s-ok")
	if okNode.Alive || okNode.ProbeState != ProbeOK || okNode.DelayMS < 1 || !okNode.ResolvedV4 || okNode.ProbedAt == 0 {
		t.Fatalf("sub ok node wrong: %+v", okNode)
	}
	udpNode, _ := st.NodeByKey(airport.ID, "s-udp")
	if udpNode.Alive || udpNode.ProbeState != ProbeUnverified || udpNode.DelayMS != 0 || !udpNode.ResolvedV4 {
		t.Fatalf("sub udp node wrong: %+v", udpNode)
	}

	// Verify ListHosts computes aliveCount from probe results for airport and alive for agent
	hosts, err := st.ListHosts()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Host{}
	for _, h := range hosts {
		byID[h.ID] = h
	}
	ah := byID[airport.ID]
	if ah.AliveCount != 1 || ah.ProbeOkCount != 1 || ah.ProbeUnreachableCount != 1 || ah.ProbeUnverifiedCount != 1 || ah.LastProbedAt == 0 {
		t.Fatalf("airport host summary wrong: %+v", ah)
	}
	gh := byID[agentHost.ID]
	if gh.AliveCount != 1 || gh.LastProbedAt != 0 {
		t.Fatalf("agent host summary wrong: %+v", gh)
	}

	// Verify report row recorded airport alive_count = 1
	reports, err := st.RecentReports(now() - 60)
	if err != nil {
		t.Fatal(err)
	}
	foundAirportReport := false
	for _, r := range reports {
		if r.HostID == airport.ID && r.AliveCount == 1 && r.NodeCount == 3 {
			foundAirportReport = true
		}
	}
	if !foundAirportReport {
		t.Fatalf("expected airport report with alive_count=1, got %+v", reports)
	}

	// Verify computeStats counts 1 agent alive + 1 probe ok = 2
	allNodes, _ := st.ListNodes("")
	stats := computeStats(nil, allNodes, nil)
	if stats["nodesAlive"] != 2 || stats["nodesUnverified"] != 1 || stats["nodesUnreachable"] != 1 {
		t.Fatalf("computeStats wrong: %+v", stats)
	}

	// Verify ReplaceSourceNodes carries probe results forward when endpoints don't change
	if err := st.ReplaceSourceNodes(airport.ID, subNodes); err != nil {
		t.Fatal(err)
	}
	okAgain, _ := st.NodeByKey(airport.ID, "s-ok")
	if okAgain.ProbeState != ProbeOK || !okAgain.ResolvedV4 || okAgain.ProbedAt == 0 {
		t.Fatalf("ReplaceSourceNodes wiped probe result: %+v", okAgain)
	}
}

func TestMigrationIsIdempotentAndPreservesData(t *testing.T) {
	dbPath := t.TempDir() + "/legacy.db"
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Seed a legacy schema missing probe_* columns and exclude_ipv6_only
	_, err = rawDB.Exec(`
CREATE TABLE hosts (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  token TEXT NOT NULL UNIQUE,
  controller TEXT NOT NULL DEFAULT '',
  agent_version TEXT NOT NULL DEFAULT '',
  mihomo_version TEXT NOT NULL DEFAULT '',
  last_seen INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE TABLE nodes (
  host_id TEXT NOT NULL,
  key TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  server TEXT NOT NULL,
  port INTEGER NOT NULL,
  alive INTEGER NOT NULL DEFAULT 0,
  delay_ms INTEGER NOT NULL DEFAULT 0,
  raw TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (host_id, key)
);
CREATE TABLE bundles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  group_by TEXT NOT NULL DEFAULT 'host',
  group_type TEXT NOT NULL DEFAULT 'url-test',
  template TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
INSERT INTO hosts (id,name,token,created_at) VALUES ('h1','legacy-host','tok1',1000);
INSERT INTO nodes (host_id,key,name,type,server,port,alive,delay_ms,raw,updated_at)
  VALUES ('h1','k1','Node1','vless','203.0.113.1',443,1,55,'{"type":"vless"}',1000);
INSERT INTO bundles (id,name,slug,created_at) VALUES ('b1','Legacy','legacy',1000);
`)
	if err != nil {
		t.Fatal(err)
	}
	_ = rawDB.Close()

	// First open migrates the legacy schema
	st, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("first migration failed: %v", err)
	}
	// Running migrate() a second time on the live store must be a no-op
	if err := st.migrate(); err != nil {
		t.Fatalf("second migrate failed: %v", err)
	}
	_ = st.Close()

	// Re-opening must also succeed without losing any rows
	st2, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("re-open failed: %v", err)
	}
	defer st2.Close()

	n, err := st2.NodeByKey("h1", "k1")
	if err != nil {
		t.Fatalf("legacy node lost: %v", err)
	}
	if !n.Alive || n.DelayMS != 55 || n.ProbeState != "" || n.ProbedAt != 0 || n.ResolvedV4 || n.ResolvedV6 {
		t.Fatalf("legacy node corrupted: %+v", n)
	}
	b, err := st2.Bundle("b1")
	if err != nil {
		t.Fatalf("legacy bundle lost: %v", err)
	}
	if b.ExcludeIPv6Only {
		t.Fatalf("legacy bundle exclude_ipv6_only should default to false")
	}
}

func TestSubscriptionExcludeIPv6OnlyFilter(t *testing.T) {
	st, err := OpenStore(t.TempDir() + "/weft.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	airport, err := st.CreateSourceHost("pokemon", "https://example.com/sub")
	if err != nil {
		t.Fatal(err)
	}
	makeRaw := func(name, typ, server string) json.RawMessage {
		return json.RawMessage(`{"name":"` + name + `","type":"` + typ + `","server":"` + server + `","port":443,"password":"p"}`)
	}
	if err := st.ReplaceSourceNodes(airport.ID, []Node{
		{Key: "v4", Name: "HK-V4", Type: "trojan", Server: "hk-v4.test", Port: 443, Source: NodeSub, Raw: makeRaw("HK-V4", "trojan", "hk-v4.test")},
		{Key: "dual", Name: "SG-Dual", Type: "hysteria2", Server: "sg-dual.test", Port: 443, Source: NodeSub, Raw: makeRaw("SG-Dual", "hysteria2", "sg-dual.test")},
		{Key: "v6only", Name: "JP-V6Only", Type: "trojan", Server: "pokemonv6-1.test", Port: 443, Source: NodeSub, Raw: makeRaw("JP-V6Only", "trojan", "pokemonv6-1.test")},
		{Key: "unprobed", Name: "US-Unprobed", Type: "trojan", Server: "us-new.test", Port: 443, Source: NodeSub, Raw: makeRaw("US-Unprobed", "trojan", "us-new.test")},
	}); err != nil {
		t.Fatal(err)
	}
	ts := now()
	if err := st.SaveProbeResults([]ProbeUpdate{
		{HostID: airport.ID, Key: "v4", Result: ProbeResult{State: ProbeOK, DelayMS: 80, ResolvedV4: true, ResolvedV6: false, ProbedAt: ts}},
		{HostID: airport.ID, Key: "dual", Result: ProbeResult{State: ProbeUnverified, ResolvedV4: true, ResolvedV6: true, ProbedAt: ts}},
		{HostID: airport.ID, Key: "v6only", Result: ProbeResult{State: ProbeUnreachable, ResolvedV4: false, ResolvedV6: true, ProbedAt: ts}},
		// "unprobed" intentionally left with probed_at = 0
	}); err != nil {
		t.Fatal(err)
	}

	// Agent host node should survive ExcludeIPv6Only even though server doesn't probe agent nodes
	agentHost, err := st.CreateHost("hk-agent", HostAgent)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceNodes(agentHost.ID, []Node{
		{Key: "ag1", Name: "Agent-Node", Type: "trojan", Server: "agent.example.com", Port: 443, Alive: true, DelayMS: 20, Source: NodeAgent, Raw: makeRaw("Agent-Node", "trojan", "agent.example.com")},
	}); err != nil {
		t.Fatal(err)
	}

	members := []Member{
		{HostID: airport.ID, Key: "v4"},
		{HostID: airport.ID, Key: "dual"},
		{HostID: airport.ID, Key: "v6only"},
		{HostID: airport.ID, Key: "unprobed"},
		{HostID: agentHost.ID, Key: "ag1"},
	}

	// Default bundle (ExcludeIPv6Only=false): keeps everything
	bDefault, err := st.CreateBundle(Bundle{Name: "Default", Slug: "def", GroupBy: "host", GroupType: "url-test", Template: DefaultTemplate})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetMembers(bDefault.ID, members); err != nil {
		t.Fatal(err)
	}
	subDefault, err := st.CreateSubscription(bDefault.ID, "default", "mihomo")
	if err != nil {
		t.Fatal(err)
	}

	// Filtered bundle (ExcludeIPv6Only=true): excludes v6only and unprobed
	bNoV6, err := st.CreateBundle(Bundle{Name: "NoV6", Slug: "nov6", GroupBy: "host", GroupType: "url-test", Template: DefaultTemplate, ExcludeIPv6Only: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetMembers(bNoV6.ID, members); err != nil {
		t.Fatal(err)
	}
	subNoV6, err := st.CreateSubscription(bNoV6.ID, "nov6", "mihomo")
	if err != nil {
		t.Fatal(err)
	}

	srv := NewServer(st, false)
	get := func(path string) string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s -> %d: %s", path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	// Default subscription keeps all 5 nodes
	defBody := get("/sub/" + subDefault.Token + "?key=" + subDefault.AccessKey)
	for _, name := range []string{"pokemon/HK-V4", "pokemon/SG-Dual", "pokemon/JP-V6Only", "pokemon/US-Unprobed", "hk-agent/Agent-Node"} {
		if !strings.Contains(defBody, name) {
			t.Errorf("default subscription missing %q", name)
		}
	}

	// Filtered subscription (both YAML and text links) skips JP-V6Only and US-Unprobed
	for _, suffix := range []string{"", "&target=text"} {
		body := get("/sub/" + subNoV6.Token + "?key=" + subNoV6.AccessKey + suffix)
		for _, want := range []string{"HK-V4", "SG-Dual", "Agent-Node"} {
			if !strings.Contains(body, want) {
				t.Errorf("filtered subscription (%s) missing %q:\n%s", suffix, want, body)
			}
		}
		for _, drop := range []string{"JP-V6Only", "US-Unprobed"} {
			if strings.Contains(body, drop) {
				t.Errorf("filtered subscription (%s) should have excluded %q:\n%s", suffix, drop, body)
			}
		}
	}
}
