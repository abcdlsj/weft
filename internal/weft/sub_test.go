package weft

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
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

func TestNodeURIProtocols(t *testing.T) {
	t.Run("vless reality", func(t *testing.T) {
		raw := map[string]any{
			"type":               "vless",
			"server":             "203.0.113.24",
			"port":               float64(8443),
			"uuid":               "86c211bf-1693-4f6f-a061-71780d87e179",
			"flow":               "xtls-rprx-vision",
			"tls":                true,
			"servername":         "www.bing.com",
			"client-fingerprint": "chrome",
			"network":            "tcp",
			"reality-opts": map[string]any{
				"public-key": "RgDO6nczQXgj_D-VGus4e3pZHL4bIwFA7fHXocnW9QI",
				"short-id":   "f8a64b66",
			},
		}
		uri := NodeURI(raw, "berlin-05/Reality")
		u, err := url.Parse(uri)
		if err != nil {
			t.Fatalf("invalid uri %q: %v", uri, err)
		}
		if u.Scheme != "vless" || u.User.Username() != "86c211bf-1693-4f6f-a061-71780d87e179" {
			t.Fatalf("unexpected scheme/user: %s", uri)
		}
		q := u.Query()
		want := map[string]string{
			"encryption": "none",
			"security":   "reality",
			"pbk":        "RgDO6nczQXgj_D-VGus4e3pZHL4bIwFA7fHXocnW9QI",
			"sid":        "f8a64b66",
			"fp":         "chrome",
			"sni":        "www.bing.com",
			"flow":       "xtls-rprx-vision",
			"type":       "tcp",
		}
		for k, v := range want {
			if got := q.Get(k); got != v {
				t.Errorf("%s = %q, want %q (uri=%s)", k, got, v, uri)
			}
		}
		// Round-trip through ParseProxyLink keeps Reality parameters intact.
		parsed, err := ParseProxyLink(uri)
		if err != nil {
			t.Fatalf("round-trip ParseProxyLink failed: %v", err)
		}
		opts, _ := parsed["reality-opts"].(map[string]any)
		if opts["public-key"] != want["pbk"] || opts["short-id"] != want["sid"] || parsed["client-fingerprint"] != "chrome" {
			t.Fatalf("round-trip lost reality opts: %v", parsed)
		}
	})

	t.Run("vless reality defaults fp to chrome when missing", func(t *testing.T) {
		raw := map[string]any{
			"type":       "vless",
			"server":     "203.0.113.24",
			"port":       443,
			"uuid":       "11111111-2222-3333-4444-555555555555",
			"tls":        true,
			"servername": "www.apple.com",
			"reality-opts": map[string]any{
				"public-key": "testpubkey",
			},
		}
		uri := NodeURI(raw, "box/R")
		u, err := url.Parse(uri)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		if q.Get("security") != "reality" || q.Get("pbk") != "testpubkey" || q.Get("fp") != "chrome" {
			t.Fatalf("unexpected query %v in %s", q, uri)
		}
	})

	t.Run("vless tls ws insecure alpn", func(t *testing.T) {
		raw := map[string]any{
			"type":             "vless",
			"server":           "203.0.113.25",
			"port":             443,
			"uuid":             "11111111-2222-3333-4444-555555555555",
			"tls":              true,
			"servername":       "cdn.example.com",
			"skip-cert-verify": true,
			"alpn":             []any{"h2", "http/1.1"},
			"network":          "ws",
			"ws-opts": map[string]any{
				"path":    "/vless",
				"headers": map[string]any{"Host": "ws.example.com"},
			},
		}
		u, err := url.Parse(NodeURI(raw, "box/WS"))
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		if q.Get("security") != "tls" || q.Get("type") != "ws" || q.Get("path") != "/vless" ||
			q.Get("host") != "ws.example.com" || q.Get("allowInsecure") != "1" || q.Get("alpn") != "h2,http/1.1" {
			t.Fatalf("unexpected vless ws query: %v", q)
		}
	})

	t.Run("hysteria2 obfs insecure", func(t *testing.T) {
		raw := map[string]any{
			"type":             "hysteria2",
			"server":           "94.159.107.211",
			"port":             float64(36906),
			"password":         "secret-pass",
			"sni":              "www.bing.com",
			"obfs":             "salamander",
			"obfs-password":    "obfs-secret",
			"skip-cert-verify": true,
			"alpn":             []any{"h3"},
			"fingerprint":      "sha256:abcdef",
			"ports":            "20000-30000",
		}
		uri := NodeURI(raw, "berlin-05/Hy2")
		u, err := url.Parse(uri)
		if err != nil {
			t.Fatalf("invalid uri %q: %v", uri, err)
		}
		if u.Scheme != "hysteria2" || u.User.Username() != "secret-pass" {
			t.Fatalf("unexpected scheme/user: %s", uri)
		}
		q := u.Query()
		want := map[string]string{
			"sni":           "www.bing.com",
			"obfs":          "salamander",
			"obfs-password": "obfs-secret",
			"insecure":      "1",
			"alpn":          "h3",
			"pinSHA256":     "sha256:abcdef",
			"mport":         "20000-30000",
		}
		for k, v := range want {
			if got := q.Get(k); got != v {
				t.Errorf("%s = %q, want %q (uri=%s)", k, got, v, uri)
			}
		}
		parsed, err := ParseProxyLink(uri)
		if err != nil {
			t.Fatalf("round-trip ParseProxyLink failed: %v", err)
		}
		if parsed["obfs"] != "salamander" || parsed["obfs-password"] != "obfs-secret" || parsed["skip-cert-verify"] != true {
			t.Fatalf("round-trip lost hy2 fields: %v", parsed)
		}
	})

	t.Run("trojan ws and grpc", func(t *testing.T) {
		ws := map[string]any{
			"type":               "trojan",
			"server":             "203.0.113.30",
			"port":               443,
			"password":           "tr-pass",
			"sni":                "tr.example.com",
			"skip-cert-verify":   true,
			"alpn":               []string{"h2"},
			"client-fingerprint": "safari",
			"network":            "ws",
			"ws-opts": map[string]any{
				"path":    "/tr",
				"headers": map[string]any{"Host": "cdn.example.com"},
			},
		}
		u, err := url.Parse(NodeURI(ws, "hk/TrojanWS"))
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		if q.Get("sni") != "tr.example.com" || q.Get("allowInsecure") != "1" || q.Get("type") != "ws" ||
			q.Get("path") != "/tr" || q.Get("host") != "cdn.example.com" || q.Get("alpn") != "h2" || q.Get("fp") != "safari" {
			t.Fatalf("unexpected trojan ws query: %v", q)
		}

		grpc := map[string]any{
			"type":      "trojan",
			"server":    "203.0.113.31",
			"port":      443,
			"password":  "tr-pass",
			"sni":       "grpc.example.com",
			"network":   "grpc",
			"grpc-opts": map[string]any{"grpc-service-name": "tr-svc"},
		}
		u2, err := url.Parse(NodeURI(grpc, "hk/TrojanGRPC"))
		if err != nil {
			t.Fatal(err)
		}
		if u2.Query().Get("type") != "grpc" || u2.Query().Get("serviceName") != "tr-svc" {
			t.Fatalf("unexpected trojan grpc query: %v", u2.Query())
		}
	})

	t.Run("vmess ws and grpc", func(t *testing.T) {
		raw := map[string]any{
			"type":               "vmess",
			"server":             "198.51.100.9",
			"port":               443,
			"uuid":               "11111111-2222-3333-4444-555555555555",
			"alterId":            2,
			"cipher":             "aes-128-gcm",
			"network":            "ws",
			"tls":                true,
			"servername":         "sni.example.com",
			"alpn":               []any{"http/1.1"},
			"client-fingerprint": "chrome",
			"skip-cert-verify":   true,
			"ws-opts": map[string]any{
				"path":    "/ray",
				"headers": map[string]any{"Host": "cdn.example.com"},
			},
		}
		uri := NodeURI(raw, "us/VMessWS")
		if !strings.HasPrefix(uri, "vmess://") {
			t.Fatalf("unexpected scheme: %s", uri)
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "vmess://"))
		if err != nil {
			t.Fatalf("decode vmess: %v", err)
		}
		var vm map[string]any
		if err := json.Unmarshal(decoded, &vm); err != nil {
			t.Fatalf("json vmess: %v", err)
		}
		if vm["net"] != "ws" || vm["path"] != "/ray" || vm["host"] != "cdn.example.com" ||
			vm["tls"] != "tls" || vm["sni"] != "sni.example.com" || vm["aid"] != float64(2) ||
			vm["alpn"] != "http/1.1" || vm["fp"] != "chrome" || vm["allowInsecure"] != true {
			t.Fatalf("unexpected vmess payload: %v", vm)
		}

		grpc := map[string]any{
			"type":      "vmess",
			"server":    "198.51.100.10",
			"port":      443,
			"uuid":      "11111111-2222-3333-4444-555555555555",
			"network":   "grpc",
			"tls":       true,
			"grpc-opts": map[string]any{"grpc-service-name": "vm-svc"},
		}
		decoded2, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(NodeURI(grpc, "us/VMessGRPC"), "vmess://"))
		var vm2 map[string]any
		json.Unmarshal(decoded2, &vm2)
		if vm2["net"] != "grpc" || vm2["path"] != "vm-svc" || vm2["type"] != "gun" {
			t.Fatalf("unexpected vmess grpc payload: %v", vm2)
		}
	})

	t.Run("ss plugins and ipv6", func(t *testing.T) {
		obfs := map[string]any{
			"type":     "ss",
			"server":   "2001:db8::1",
			"port":     8388,
			"cipher":   "aes-256-gcm",
			"password": "pw",
			"plugin":   "obfs",
			"plugin-opts": map[string]any{
				"mode": "tls",
				"host": "www.bing.com",
			},
		}
		uri := NodeURI(obfs, "jp/SS-Obfs")
		u, err := url.Parse(uri)
		if err != nil {
			t.Fatalf("invalid ss uri %q: %v", uri, err)
		}
		if u.Host != "[2001:db8::1]:8388" {
			t.Fatalf("ipv6 host wrong: %q", u.Host)
		}
		if got := u.Query().Get("plugin"); got != "obfs-local;obfs=tls;obfs-host=www.bing.com" {
			t.Fatalf("plugin = %q", got)
		}

		v2 := map[string]any{
			"type":     "ss",
			"server":   "203.0.113.8",
			"port":     443,
			"cipher":   "chacha20-ietf-poly1305",
			"password": "pw2",
			"plugin":   "v2ray-plugin",
			"plugin-opts": map[string]any{
				"mode": "websocket",
				"tls":  true,
				"host": "ws.example.com",
				"path": "/ss",
			},
		}
		u2, err := url.Parse(NodeURI(v2, "jp/SS-V2"))
		if err != nil {
			t.Fatal(err)
		}
		if got := u2.Query().Get("plugin"); got != "v2ray-plugin;mode=websocket;tls;host=ws.example.com;path=/ss" {
			t.Fatalf("v2ray-plugin = %q", got)
		}
	})
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
