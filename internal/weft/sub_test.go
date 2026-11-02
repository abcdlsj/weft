package weft

import (
	"encoding/json"
	"strings"
	"testing"
)

func testNodes() ([]Host, []Node) {
	hosts := []Host{{ID: "h1", Name: "hk-01"}, {ID: "h2", Name: "sg-02"}}
	raw := func(name, typ, server, key string) json.RawMessage {
		return json.RawMessage(`{"name":"` + name + `","type":"` + typ + `","server":"` + server +
			`","port":443,"password":"p","udp":true}`)
	}
	nodes := []Node{
		{HostID: "h1", Key: "k1", Name: "A", Type: "ss", Alive: true, DelayMS: 30,
			Raw: raw("A", "ss", "203.0.113.1", "k1")},
		{HostID: "h2", Key: "k2", Name: "B", Type: "trojan", Alive: true, DelayMS: 90,
			Raw: raw("B", "trojan", "203.0.113.2", "k2")},
	}
	return hosts, nodes
}

func TestBuildMihomoYAMLByHost(t *testing.T) {
	hosts, nodes := testNodes()
	out, err := BuildMihomoYAML(Bundle{GroupBy: "host", GroupType: "url-test"}, hosts, nodes)
	if err != nil {
		t.Fatal(err)
	}
	yaml := string(out)
	for _, want := range []string{
		"hk-01/A", "sg-02/B",
		"name: PROXY", "name: AUTO",
		"name: hk-01", "name: sg-02",
		"MATCH,PROXY",
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("config is missing %q\n%s", want, yaml)
		}
	}
	// the template's own key order survives (mixed-port is first in the default)
	if !strings.HasPrefix(yaml, "mixed-port: 7890") {
		t.Errorf("template order was not preserved, got:\n%s", yaml[:40])
	}
}

func TestBuildMihomoYAMLByTypeAddsGroups(t *testing.T) {
	hosts, nodes := testNodes()
	out, err := BuildMihomoYAML(Bundle{GroupBy: "type", GroupType: "select"}, hosts, nodes)
	if err != nil {
		t.Fatal(err)
	}
	yaml := string(out)
	for _, want := range []string{"name: SS", "name: TROJAN"} {
		if !strings.Contains(yaml, want) {
			t.Errorf("missing generated group %q\n%s", want, yaml)
		}
	}
}

func TestBuildMihomoYAMLRejectsBrokenTemplate(t *testing.T) {
	hosts, nodes := testNodes()
	if _, err := BuildMihomoYAML(Bundle{Template: "proxies: [\n"}, hosts, nodes); err == nil {
		t.Fatal("expected an error for invalid yaml")
	}
}

func TestBuildLinksNamespacesNodes(t *testing.T) {
	hosts, nodes := testNodes()
	links := BuildLinks(hosts, nodes)
	if len(links) != 2 {
		t.Fatalf("want 2 links, got %d", len(links))
	}
	joined := strings.Join(links, "\n")
	if !strings.Contains(joined, "ss://") || !strings.Contains(joined, "trojan://") {
		t.Fatalf("unexpected links:\n%s", joined)
	}
	if !strings.Contains(joined, "hk-01%2FA") {
		t.Fatalf("link is not namespaced to its host:\n%s", joined)
	}
}

func TestNodeKeyIsStableAndDistinct(t *testing.T) {
	a := map[string]any{"type": "ss", "server": "203.0.113.1", "port": float64(443), "password": "p"}
	b := map[string]any{"type": "ss", "server": "203.0.113.1", "port": float64(443), "password": "q"}
	if nodeKey(a) != nodeKey(a) {
		t.Fatal("node key is not stable")
	}
	if nodeKey(a) == nodeKey(b) {
		t.Fatal("different credentials share a key")
	}
}

func TestDetectDevice(t *testing.T) {
	cases := map[string]string{
		"ClashMetaForAndroid/2.11.1 (mihomo/1.19.2)":             "Clash Meta for Android",
		"clash-verge/v2.2.3":                                     "Clash Verge",
		"sing-box/1.11.0 (darwin arm64)":                         "sing-box",
		"Shadowrocket/2.2.65 (iPhone17,2; iOS 18.4)":             "Shadowrocket",
		"Stash/2.5.2 (iPhone16,1; iOS 18.3)":                     "Stash",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) curl/8": "Mozilla",
		"": "未知客户端",
	}
	for ua, want := range cases {
		if got := DetectDevice(ua); got != want {
			t.Errorf("DetectDevice(%q) = %q, want %q", ua, got, want)
		}
	}
}

func TestSanitizeTag(t *testing.T) {
	if got := sanitizeTag("  客厅 AppleTV  "); got != "客厅 AppleTV" {
		t.Errorf("got %q", got)
	}
	if got := sanitizeTag(strings.Repeat("x", 80)); len([]rune(got)) != 32 {
		t.Errorf("tag not capped: %d runes", len([]rune(got)))
	}
	if got := sanitizeTag("   "); got != "未标注设备" {
		t.Errorf("got %q", got)
	}
}
