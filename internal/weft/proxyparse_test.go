package weft

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

const (
	hy2Link = "hy2://cab3041b03bea16205c9241522f1baa3@94.159.107.211:36906" +
		"?sni=www.bing.com&insecure=1#H2Nexus-Hy2"
	realityLink = "vless://86c211bf-1693-4f6f-a061-71780d87e179@94.159.107.211:8443" +
		"?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.bing.com" +
		"&fp=chrome&pbk=RgDO6nczQXgj_D-VGus4e3pZHL4bIwFA7fHXocnW9QI&sid=f8a64b66" +
		"&type=tcp&headerType=none#H2Nexus-Reality"
)

func TestParseHysteria2Link(t *testing.T) {
	p, err := ParseProxyLink(hy2Link)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	n, err := NodeFromProxy(p)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	if n.Type != "hysteria2" || n.Server != "94.159.107.211" || n.Port != 36906 {
		t.Fatalf("index columns wrong: %+v", n)
	}
	if n.Name != "H2Nexus-Hy2" {
		t.Fatalf("name: %q", n.Name)
	}
	var raw map[string]any
	if err := json.Unmarshal(n.Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["password"] != "cab3041b03bea16205c9241522f1baa3" {
		t.Fatalf("password: %v", raw["password"])
	}
	if raw["sni"] != "www.bing.com" || raw["skip-cert-verify"] != true {
		t.Fatalf("tls fields: %v", raw)
	}
	if n.Key == "" {
		t.Fatal("key is empty")
	}
}

func TestParseRealityLink(t *testing.T) {
	p, err := ParseProxyLink(realityLink)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	n, err := NodeFromProxy(p)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(n.Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["uuid"] != "86c211bf-1693-4f6f-a061-71780d87e179" {
		t.Fatalf("uuid: %v", raw["uuid"])
	}
	if raw["tls"] != true || raw["servername"] != "www.bing.com" {
		t.Fatalf("tls fields: %v", raw)
	}
	if raw["flow"] != "xtls-rprx-vision" || raw["client-fingerprint"] != "chrome" {
		t.Fatalf("vless fields: %v", raw)
	}
	opts, ok := raw["reality-opts"].(map[string]any)
	if !ok {
		t.Fatalf("reality-opts missing: %v", raw)
	}
	if opts["public-key"] != "RgDO6nczQXgj_D-VGus4e3pZHL4bIwFA7fHXocnW9QI" || opts["short-id"] != "f8a64b66" {
		t.Fatalf("reality-opts: %v", opts)
	}
}

func TestParseSSLink(t *testing.T) {
	cred := base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:pw"))
	p, err := ParseProxyLink("ss://" + cred + "@203.0.113.7:8388#Tokyo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	n, err := NodeFromProxy(p)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	if n.Type != "ss" || n.Port != 8388 || n.Name != "Tokyo" {
		t.Fatalf("node: %+v", n)
	}
	var raw map[string]any
	json.Unmarshal(n.Raw, &raw)
	if raw["cipher"] != "aes-256-gcm" || raw["password"] != "pw" {
		t.Fatalf("raw: %v", raw)
	}
}

func TestParseVMessLink(t *testing.T) {
	body := `{"v":"2","ps":"LAX","add":"198.51.100.9","port":"443","id":"11111111-2222-3333-4444-555555555555","aid":"0","net":"ws","host":"cdn.example.com","path":"/ray","tls":"tls"}`
	p, err := ParseProxyLink("vmess://" + base64.StdEncoding.EncodeToString([]byte(body)))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	n, err := NodeFromProxy(p)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	if n.Port != 443 || n.Name != "LAX" {
		t.Fatalf("node: %+v", n)
	}
	var raw map[string]any
	json.Unmarshal(n.Raw, &raw)
	ws, ok := raw["ws-opts"].(map[string]any)
	if !ok || ws["path"] != "/ray" {
		t.Fatalf("ws-opts: %v", raw)
	}
}

func TestParseRejectsJunk(t *testing.T) {
	for _, bad := range []string{"", "ftp://x", "hy2://", "vless://uuid@host"} {
		if _, err := ParseProxyLink(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

// A hand-entered node must survive a later agent report for the same host.
func TestManualNodeSurvivesAgentReport(t *testing.T) {
	st, err := OpenStore(t.TempDir() + "/weft.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	host, err := st.CreateHost("h2nexus", HostStatic)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := ParseProxyLink(hy2Link)
	manual, err := NodeFromProxy(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertNode(host.ID, manual); err != nil {
		t.Fatal(err)
	}

	agentNode := Node{Key: "agentkey", Name: "A", Type: "ss", Server: "1.1.1.1", Port: 8388, Raw: json.RawMessage(`{}`)}
	if err := st.ReplaceNodes(host.ID, []Node{agentNode}); err != nil {
		t.Fatal(err)
	}

	nodes, err := st.ListNodes(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("want manual + agent, got %d: %+v", len(nodes), nodes)
	}
	var manualSeen bool
	for _, n := range nodes {
		if n.Key == manual.Key {
			manualSeen = n.Source == NodeManual
		}
	}
	if !manualSeen {
		t.Fatalf("manual node lost or relabelled: %+v", nodes)
	}

	// a second report must not duplicate the agent node either
	if err := st.ReplaceNodes(host.ID, []Node{agentNode}); err != nil {
		t.Fatal(err)
	}
	nodes, _ = st.ListNodes(host.ID)
	if len(nodes) != 2 {
		t.Fatalf("want 2 after second report, got %d", len(nodes))
	}
}

// The whole point: a hand-entered node reaches the subscription output.
func TestManualNodeReachesSubscription(t *testing.T) {
	st, err := OpenStore(t.TempDir() + "/weft.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	host, _ := st.CreateHost("h2nexus", HostStatic)
	p, _ := ParseProxyLink(hy2Link)
	n, _ := NodeFromProxy(p)
	if err := st.UpsertNode(host.ID, n); err != nil {
		t.Fatal(err)
	}
	hosts, _ := st.ListHosts()
	nodes, _ := st.ListNodes("")

	out, err := BuildMihomoYAML(Bundle{GroupBy: "host", GroupType: "url-test"}, hosts, nodes)
	if err != nil {
		t.Fatal(err)
	}
	body := string(out)
	if !strings.Contains(body, "h2nexus/H2Nexus-Hy2") {
		t.Fatalf("node missing from subscription:\n%s", body)
	}
	if !strings.Contains(body, "type: hysteria2") {
		t.Fatalf("protocol wrong:\n%s", body)
	}
}
