package weft

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MihomoClient talks to a mihomo external-controller.
type MihomoClient struct {
	BaseURL string
	Secret  string
	http    *http.Client
}

func NewMihomoClient(base, secret string) *MihomoClient {
	return &MihomoClient{
		BaseURL: strings.TrimRight(base, "/"),
		Secret:  secret,
		http: &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}},
	}
}

func (c *MihomoClient) get(path string, out any) error {
	u := c.BaseURL + path
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	if c.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.Secret)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("mihomo %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *MihomoClient) Version() (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	// /version is unauthenticated in some builds; keep the secret anyway.
	err := c.get("/version", &v)
	return v.Version, err
}

type proxyEntry struct {
	Type    string `json:"type"`
	Alive   bool   `json:"alive"`
	UDP     bool   `json:"udp"`
	History []struct {
		Time  string `json:"time"`
		Delay int    `json:"delay"`
	} `json:"history"`
}

func (c *MihomoClient) Proxies() (map[string]proxyEntry, error) {
	var out struct {
		Proxies map[string]proxyEntry `json:"proxies"`
	}
	err := c.get("/proxies", &out)
	return out.Proxies, err
}

// Providers returns the full proxy configuration of every provider, which is
// what the mihomo HTTP API keeps for both `proxies:` and `proxy-providers:`.
func (c *MihomoClient) Providers() ([]map[string]any, error) {
	var out struct {
		Providers map[string]map[string]struct {
			VehicleType string           `json:"vehicleType"`
			Proxies     []map[string]any `json:"proxies"`
		} `json:"providers"`
	}
	if err := c.get("/providers/proxies", &out); err != nil {
		return nil, err
	}
	var all []map[string]any
	for _, group := range out.Providers {
		for _, p := range group {
			all = append(all, p.Proxies...)
		}
	}
	return all, nil
}

// Collect merges provider configuration with the live delay history and returns
// report-ready nodes. Nodes without a server/port are skipped (groups, DIRECT,
// built-in REJECT and similar).
func (c *MihomoClient) Collect() ([]Node, string, error) {
	version, _ := c.Version()
	proxies, err := c.Proxies()
	if err != nil {
		return nil, version, err
	}
	raw, err := c.Providers()
	if err != nil {
		return nil, version, err
	}
	seen := map[string]bool{}
	nodes := make([]Node, 0, len(raw))
	for _, p := range raw {
		name, _ := p["name"].(string)
		typ, _ := p["type"].(string)
		server, _ := p["server"].(string)
		portF, _ := p["port"].(float64)
		if name == "" || server == "" || portF == 0 || isBuiltin(name) {
			continue
		}
		key := nodeKey(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		n := Node{
			Key:    key,
			Name:   name,
			Type:   strings.ToLower(typ),
			Server: server,
			Port:   int(portF),
			Raw:    toJSON(p),
		}
		if pe, ok := proxies[name]; ok {
			n.Alive = pe.Alive
			n.DelayMS = lastDelay(pe)
			if !n.Alive && n.DelayMS > 0 {
				n.Alive = false
			}
			if pe.History != nil && n.DelayMS == 0 {
				n.Alive = false
			}
		}
		nodes = append(nodes, n)
	}
	return nodes, version, nil
}

func lastDelay(p proxyEntry) int {
	if len(p.History) == 0 {
		return 0
	}
	return p.History[len(p.History)-1].Delay
}

func isBuiltin(name string) bool {
	switch strings.ToUpper(name) {
	case "DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE", "GLOBAL":
		return true
	}
	return false
}

// nodeKey is stable across restarts and unique per endpoint+credential.
func nodeKey(p map[string]any) string {
	var b strings.Builder
	for _, f := range []string{"type", "server", "port", "uuid", "password", "cipher", "sni", "servername", "auth"} {
		if v, ok := p[f]; ok {
			fmt.Fprintf(&b, "%s=%v;", f, v)
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8])
}

func toJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// NodeURI renders a mihomo proxy config as a share link for the URI / Shadowrocket targets.
func NodeURI(p map[string]any, displayName string) string {
	typ, _ := p["type"].(string)
	server, _ := p["server"].(string)
	port := int(toFloat(p["port"]))
	hostPort := net.JoinHostPort(server, strconv.Itoa(port))
	name := url.PathEscape(displayName)
	switch strings.ToLower(typ) {
	case "ss", "shadowsocks":
		method, _ := p["cipher"].(string)
		pass, _ := p["password"].(string)
		cred := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + pass))
		q := url.Values{}
		if plugin := strOr(p["plugin"], ""); plugin != "" {
			q.Set("plugin", ssPluginParam(plugin, p["plugin-opts"]))
		}
		if len(q) > 0 {
			return fmt.Sprintf("ss://%s@%s?%s#%s", cred, hostPort, q.Encode(), name)
		}
		return fmt.Sprintf("ss://%s@%s#%s", cred, hostPort, name)
	case "trojan":
		pass, _ := p["password"].(string)
		q := url.Values{}
		if sni := strOr(p["sni"], strOr(p["servername"], "")); sni != "" {
			q.Set("sni", sni)
		}
		if boolVal(p["skip-cert-verify"]) {
			q.Set("allowInsecure", "1")
		}
		if alpn := joinList(p["alpn"]); alpn != "" {
			q.Set("alpn", alpn)
		}
		if fp := strOr(p["client-fingerprint"], strOr(p["fingerprint"], "")); fp != "" {
			q.Set("fp", fp)
		}
		netType := strOr(p["network"], "")
		if netType != "" && netType != "tcp" {
			q.Set("type", netType)
			applyTransportQuery(q, p, netType)
		}
		return buildQueryURI("trojan", pass, hostPort, q, name)
	case "vmess":
		netType := strOr(p["network"], "tcp")
		path, host, svc := transportOpts(p, netType)
		headerType := "none"
		switch netType {
		case "grpc":
			path = svc
			headerType = "gun"
		case "http":
			headerType = "http"
		}
		vm := map[string]any{
			"v": "2", "ps": displayName, "add": server, "port": port,
			"id": p["uuid"], "aid": int(toFloat(p["alterId"])), "scy": strOr(p["cipher"], "auto"),
			"net": netType, "type": headerType, "host": host,
			"path": path, "tls": tlsValue(p), "sni": strOr(p["servername"], strOr(p["sni"], "")),
		}
		if alpn := joinList(p["alpn"]); alpn != "" {
			vm["alpn"] = alpn
		}
		if fp := strOr(p["client-fingerprint"], strOr(p["fingerprint"], "")); fp != "" {
			vm["fp"] = fp
		}
		if boolVal(p["skip-cert-verify"]) {
			vm["allowInsecure"] = true
		}
		return "vmess://" + base64Std(string(toJSON(vm)))
	case "vless":
		netType := strOr(p["network"], "tcp")
		q := url.Values{}
		q.Set("encryption", strOr(p["encryption"], "none"))
		q.Set("type", netType)
		reality, _ := p["reality-opts"].(map[string]any)
		pbk := strOr(reality["public-key"], "")
		if pbk != "" {
			q.Set("security", "reality")
			q.Set("pbk", pbk)
			if sid := strOr(reality["short-id"], ""); sid != "" {
				q.Set("sid", sid)
			}
			if spx := strOr(reality["spider-x"], ""); spx != "" {
				q.Set("spx", spx)
			}
			fp := strOr(p["client-fingerprint"], strOr(p["fingerprint"], "chrome"))
			q.Set("fp", fp)
		} else {
			sec := tlsValue(p)
			if sec == "" {
				sec = "none"
			}
			q.Set("security", sec)
			if fp := strOr(p["client-fingerprint"], strOr(p["fingerprint"], "")); fp != "" {
				q.Set("fp", fp)
			}
			if boolVal(p["skip-cert-verify"]) {
				q.Set("allowInsecure", "1")
			}
		}
		if sni := strOr(p["servername"], strOr(p["sni"], "")); sni != "" {
			q.Set("sni", sni)
		}
		if alpn := joinList(p["alpn"]); alpn != "" {
			q.Set("alpn", alpn)
		}
		applyTransportQuery(q, p, netType)
		if flow := strOr(p["flow"], ""); flow != "" {
			q.Set("flow", flow)
		}
		if pe := strOr(p["packet-encoding"], ""); pe != "" {
			q.Set("packetEncoding", pe)
		}
		uuid, _ := p["uuid"].(string)
		return buildQueryURI("vless", uuid, hostPort, q, name)
	case "hysteria2", "hy2":
		pass, _ := p["password"].(string)
		q := url.Values{}
		if sni := strOr(p["sni"], strOr(p["servername"], "")); sni != "" {
			q.Set("sni", sni)
		}
		if obfs := strOr(p["obfs"], ""); obfs != "" {
			q.Set("obfs", obfs)
		}
		if obfsPass := strOr(p["obfs-password"], ""); obfsPass != "" {
			q.Set("obfs-password", obfsPass)
		}
		if boolVal(p["skip-cert-verify"]) {
			q.Set("insecure", "1")
		}
		if alpn := joinList(p["alpn"]); alpn != "" {
			q.Set("alpn", alpn)
		}
		if pin := strOr(p["fingerprint"], ""); pin != "" {
			q.Set("pinSHA256", pin)
		}
		if ports := strOr(p["ports"], ""); ports != "" {
			q.Set("mport", ports)
		}
		if up := strOr(p["up"], ""); up != "" {
			q.Set("up", up)
		}
		if down := strOr(p["down"], ""); down != "" {
			q.Set("down", down)
		}
		return buildQueryURI("hysteria2", pass, hostPort, q, name)
	}
	return ""
}

func buildQueryURI(scheme, user, hostPort string, q url.Values, name string) string {
	userinfo := url.User(user).String()
	if len(q) > 0 {
		return fmt.Sprintf("%s://%s@%s?%s#%s", scheme, userinfo, hostPort, q.Encode(), name)
	}
	return fmt.Sprintf("%s://%s@%s#%s", scheme, userinfo, hostPort, name)
}

func applyTransportQuery(q url.Values, p map[string]any, netType string) {
	path, host, svc := transportOpts(p, netType)
	switch netType {
	case "ws", "httpupgrade", "h2", "http":
		if path != "" {
			q.Set("path", path)
		}
		if host != "" {
			q.Set("host", host)
		}
	case "grpc":
		if svc != "" {
			q.Set("serviceName", svc)
		}
	}
}

// transportOpts reads ws-opts / grpc-opts / h2-opts / http-opts for share links.
func transportOpts(p map[string]any, netType string) (path, host, serviceName string) {
	switch netType {
	case "ws", "httpupgrade":
		opts, _ := p["ws-opts"].(map[string]any)
		if opts == nil {
			opts, _ = p["httpupgrade-opts"].(map[string]any)
		}
		if opts == nil {
			return "", "", ""
		}
		path = strOr(opts["path"], "")
		if headers, ok := opts["headers"].(map[string]any); ok {
			host = strOr(headers["Host"], strOr(headers["host"], ""))
		}
		if host == "" {
			host = strOr(opts["host"], "")
		}
		return path, host, ""
	case "grpc":
		if opts, ok := p["grpc-opts"].(map[string]any); ok {
			serviceName = strOr(opts["grpc-service-name"], strOr(opts["service-name"], ""))
		}
		return "", "", serviceName
	case "h2":
		if opts, ok := p["h2-opts"].(map[string]any); ok {
			path = strOr(opts["path"], "")
			host = joinList(opts["host"])
		}
		return path, host, ""
	case "http":
		if opts, ok := p["http-opts"].(map[string]any); ok {
			path = firstListItem(opts["path"])
			if headers, ok := opts["headers"].(map[string]any); ok {
				host = firstListItem(headers["Host"])
				if host == "" {
					host = firstListItem(headers["host"])
				}
			}
		}
		return path, host, ""
	}
	return "", "", ""
}

func ssPluginParam(plugin string, rawOpts any) string {
	opts, _ := rawOpts.(map[string]any)
	name := plugin
	parts := make([]string, 0, len(opts)+1)
	handled := map[string]bool{}
	switch strings.ToLower(plugin) {
	case "obfs", "simple-obfs", "obfs-local":
		name = "obfs-local"
		if mode := strOr(opts["mode"], ""); mode != "" {
			parts = append(parts, "obfs="+escapePluginOpt(mode))
			handled["mode"] = true
		}
		if host := strOr(opts["host"], ""); host != "" {
			parts = append(parts, "obfs-host="+escapePluginOpt(host))
			handled["host"] = true
		}
	case "v2ray-plugin":
		name = "v2ray-plugin"
		if mode := strOr(opts["mode"], ""); mode != "" {
			parts = append(parts, "mode="+escapePluginOpt(mode))
			handled["mode"] = true
		}
		if boolVal(opts["tls"]) {
			parts = append(parts, "tls")
		}
		handled["tls"] = true
		if host := strOr(opts["host"], ""); host != "" {
			parts = append(parts, "host="+escapePluginOpt(host))
			handled["host"] = true
		}
		if path := strOr(opts["path"], ""); path != "" {
			parts = append(parts, "path="+escapePluginOpt(path))
			handled["path"] = true
		}
		if boolVal(opts["mux"]) {
			parts = append(parts, "mux")
		}
		handled["mux"] = true
		if boolVal(opts["skip-cert-verify"]) {
			parts = append(parts, "skip-cert-verify=1")
		}
		handled["skip-cert-verify"] = true
	}
	if len(opts) > 0 {
		keys := make([]string, 0, len(opts))
		for k := range opts {
			if !handled[k] {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch v := opts[k].(type) {
			case bool:
				if v {
					parts = append(parts, escapePluginOpt(k))
				}
			case string:
				if v != "" {
					parts = append(parts, escapePluginOpt(k)+"="+escapePluginOpt(v))
				}
			case float64, int, int64:
				parts = append(parts, fmt.Sprintf("%s=%v", escapePluginOpt(k), v))
			}
		}
	}
	if len(parts) == 0 {
		return name
	}
	return name + ";" + strings.Join(parts, ";")
}

func escapePluginOpt(s string) string {
	return strings.NewReplacer(`\`, `\\`, `;`, `\;`, `=`, `\=`).Replace(s)
}

func boolVal(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return truthy(b)
	case float64:
		return b != 0
	case int:
		return b != 0
	}
	return false
}

func joinList(v any) string {
	switch list := v.(type) {
	case string:
		return strings.TrimSpace(list)
	case []string:
		out := make([]string, 0, len(list))
		for _, s := range list {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return strings.Join(out, ",")
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		}
		return strings.Join(out, ",")
	}
	return ""
}

func firstListItem(v any) string {
	switch list := v.(type) {
	case string:
		return strings.TrimSpace(list)
	case []string:
		for _, s := range list {
			if s = strings.TrimSpace(s); s != "" {
				return s
			}
		}
	case []any:
		for _, item := range list {
			if s, ok := item.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// tlsValue normalises mihomo's tls field (bool or string) for share links.
func tlsValue(p map[string]any) string {
	switch v := p["tls"].(type) {
	case bool:
		if v {
			return "tls"
		}
	case string:
		return v
	}
	return ""
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case uint64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	}
	return 0
}

func strOr(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}
