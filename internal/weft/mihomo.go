package weft

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// NodeURI renders a mihomo proxy config as a share link for the base64 targets.
func NodeURI(p map[string]any, displayName string) string {
	typ, _ := p["type"].(string)
	server, _ := p["server"].(string)
	port := int(toFloat(p["port"]))
	name := url.QueryEscape(displayName)
	switch strings.ToLower(typ) {
	case "ss", "shadowsocks":
		method, _ := p["cipher"].(string)
		pass, _ := p["password"].(string)
		cred := base64Std(method + ":" + pass)
		return fmt.Sprintf("ss://%s@%s:%d#%s", cred, server, port, name)
	case "trojan":
		pass, _ := p["password"].(string)
		q := url.Values{}
		if sni, ok := p["sni"].(string); ok && sni != "" {
			q.Set("sni", sni)
		}
		if skip, ok := p["skip-cert-verify"].(bool); ok && skip {
			q.Set("allowInsecure", "1")
		}
		return fmt.Sprintf("trojan://%s@%s:%d?%s#%s", pass, server, port, q.Encode(), name)
	case "vmess":
		path, host := transportHost(p)
		vm := map[string]any{
			"v": "2", "ps": displayName, "add": server, "port": port,
			"id": p["uuid"], "aid": numOr(p["alterId"], 0), "scy": strOr(p["cipher"], "auto"),
			"net": strOr(p["network"], "tcp"), "type": "none", "host": host,
			"path": path, "tls": tlsValue(p), "sni": strOr(p["servername"], strOr(p["sni"], "")),
		}
		return "vmess://" + base64Std(string(toJSON(vm)))
	case "vless":
		path, host := transportHost(p)
		q := url.Values{}
		q.Set("encryption", "none")
		q.Set("security", tlsValue(p))
		q.Set("type", strOr(p["network"], "tcp"))
		if sni := strOr(p["servername"], strOr(p["sni"], "")); sni != "" {
			q.Set("sni", sni)
		}
		if q.Get("type") == "ws" {
			if path != "" {
				q.Set("path", path)
			}
			if host != "" {
				q.Set("host", host)
			}
		}
		if flow := strOr(p["flow"], ""); flow != "" {
			q.Set("flow", flow)
		}
		return fmt.Sprintf("vless://%v@%s:%d?%s#%s", p["uuid"], server, port, q.Encode(), name)
	case "hysteria2", "hy2":
		q := url.Values{}
		if sni, ok := p["sni"].(string); ok && sni != "" {
			q.Set("sni", sni)
		}
		return fmt.Sprintf("hysteria2://%v@%s:%d?%s#%s", p["password"], server, port, q.Encode(), name)
	}
	return ""
}

// transportHost reads the ws-opts used by vmess/vless for its path and Host header.
func transportHost(p map[string]any) (path, host string) {
	opts, ok := p["ws-opts"].(map[string]any)
	if !ok {
		return "", ""
	}
	path = strOr(opts["path"], "")
	if headers, ok := opts["headers"].(map[string]any); ok {
		host = strOr(headers["Host"], strOr(headers["host"], ""))
	}
	return path, host
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

func numOr(v any, def int) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return def
}
