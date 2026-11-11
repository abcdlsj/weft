package weft

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestCleanProxyNameDropsBanners(t *testing.T) {
	for _, raw := range []string{
		"剩余流量：58.39 GB",
		"距离下次重置剩余：6 天",
		"套餐到期：2026-11-17",
		"建议：感到卡顿请切换到专线节点",
		"放丢失官网:https://love3.p6m6.com",
	} {
		if name, _, ok := CleanProxyName(raw); ok {
			t.Errorf("%q should be dropped, got %q", raw, name)
		}
	}
}

func TestCleanProxyName(t *testing.T) {
	cases := []struct {
		raw, name, region string
	}{
		{"🇭🇰【亚洲】香港01丨直连", "香港01 直连", "香港"},
		{"🇺🇸【北美洲】美国01原生丨直连【2x】", "美国01原生 直连", "美国"},
		{"🇯🇵【亚洲】日本03丨测试", "日本03 测试", "日本"},
		{"🇸🇬 【亚洲】新加坡01丨Vless", "新加坡01 Vless", "新加坡"},
		{"🇭🇰【亚洲】香港01丨V6【1x】", "香港01 V6", "香港"},
		{"HK-01 0.5x", "HK-01 0.5x", ""},
		{"台湾 01", "台湾 01", "台湾"},
	}
	for _, c := range cases {
		name, region, ok := CleanProxyName(c.raw)
		if !ok {
			t.Fatalf("%q should be kept", c.raw)
		}
		if name != c.name || region != c.region {
			t.Errorf("%q → (%q,%q), want (%q,%q)", c.raw, name, region, c.name, c.region)
		}
	}
}

func TestParseClashSubscription(t *testing.T) {
	doc := `proxies:
  - { name: '剩余流量：1 GB', type: trojan, server: 1.1.1.1, port: 443, password: x }
  - { name: 🇭🇰【亚洲】香港01丨直连, type: hysteria2, server: hk.example.com, port: 20000, password: p, sni: bing.com }
  - { name: 🇯🇵【亚洲】日本01丨Vless, type: vless, server: jp.example.com, port: 443, uuid: u }
proxy-groups:
  - { name: 宝可梦, type: select, proxies: [] }
`
	proxies, err := ParseSubscription([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(proxies) != 3 {
		t.Fatalf("want 3 proxies, got %d", len(proxies))
	}
	nodes := BuildSourceNodes(proxies)
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes after filtering, got %d", len(nodes))
	}
	if nodes[0].Name != "香港01 直连" || nodes[0].Region != "香港" || nodes[0].Source != NodeSub {
		t.Errorf("unexpected first node: %+v", nodes[0])
	}
	if nodes[1].Region != "日本" {
		t.Errorf("unexpected second node: %+v", nodes[1])
	}
	if nodes[0].Server != "hk.example.com" || nodes[0].Port != 20000 {
		t.Errorf("endpoint not carried over: %+v", nodes[0])
	}
}

func TestParseBase64Subscription(t *testing.T) {
	links := strings.Join([]string{
		"trojan://pass@1.1.1.1:443?sni=bing.com#%E5%89%A9%E4%BD%99%E6%B5%81%E9%87%8F%EF%BC%9A1%20GB",
		"hysteria2://pw@hk.example.com:20000?sni=bing.com#%F0%9F%87%AD%F0%9F%87%B0%E3%80%90%E4%BA%9A%E6%B4%B2%E3%80%91%E9%A6%99%E6%B8%AF01%E4%B8%A8%E7%9B%B4%E8%BF%9E",
	}, "\r\n")
	payload := base64.StdEncoding.EncodeToString([]byte(links))
	for _, form := range []string{payload, strings.TrimRight(payload, "=")} {
		proxies, err := ParseSubscription([]byte(form))
		if err != nil {
			t.Fatalf("base64 form failed: %v", err)
		}
		nodes := BuildSourceNodes(proxies)
		if len(nodes) != 1 {
			t.Fatalf("want 1 node, got %d", len(nodes))
		}
		if nodes[0].Name != "香港01 直连" || nodes[0].Type != "hysteria2" {
			t.Errorf("unexpected node: %+v", nodes[0])
		}
	}
}

func TestParsePlainLinkList(t *testing.T) {
	plain := "vless://uuid@jp.example.com:443?security=reality&sni=www.bing.com#%F0%9F%87%AF%F0%9F%87%B5%E6%97%A5%E6%9C%AC01\n"
	proxies, err := ParseSubscription([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	nodes := BuildSourceNodes(proxies)
	if len(nodes) != 1 || nodes[0].Region != "日本" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
}

func TestParseSubscriptionRejectsGarbage(t *testing.T) {
	if _, err := ParseSubscription([]byte("hello world")); err == nil {
		t.Fatal("want an error for an unknown payload")
	}
}

func TestBuildSourceNodesDedupesByEndpoint(t *testing.T) {
	dup := "trojan://pass@1.1.1.1:443?sni=bing.com#%F0%9F%87%AD%F0%9F%87%B0%E9%A6%99%E6%B8%AF01"
	proxies, err := ParseSubscription([]byte(dup + "\n" + dup))
	if err != nil {
		t.Fatal(err)
	}
	if nodes := BuildSourceNodes(proxies); len(nodes) != 1 {
		t.Fatalf("want 1 node after dedupe, got %d", len(nodes))
	}
}

func TestBuildSourceNodesDropsBannerEndpoints(t *testing.T) {
	doc := `proxies:
  - { name: '剩余流量：1 GB', type: trojan, server: 9.9.9.9, port: 443, password: shared }
  - { name: 🇯🇵【亚洲】日本03丨测试, type: trojan, server: 9.9.9.9, port: 443, password: shared }
  - { name: 🇭🇰【亚洲】香港01丨直连, type: hysteria2, server: hk.example.com, port: 20000, password: shared }
`
	proxies, err := ParseSubscription([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	nodes := BuildSourceNodes(proxies)
	if len(nodes) != 1 || nodes[0].Name != "香港01 直连" {
		t.Fatalf("banner endpoint should be dropped, got %+v", nodes)
	}
}

func TestSyncSourceStoresNodesAndRegion(t *testing.T) {
	doc := `proxies:
  - { name: '剩余流量：1 GB', type: trojan, server: 9.9.9.9, port: 443, password: shared }
  - { name: 🇭🇰【亚洲】香港01丨直连, type: hysteria2, server: hk.example.com, port: 20000, password: shared, sni: bing.com }
  - { name: 🇺🇸【北美洲】美国01原生丨直连【2x】, type: hysteria2, server: us.example.com, port: 443, password: shared, sni: bing.com }
`
	st, err := OpenStore(t.TempDir() + "/weft.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h, err := st.CreateSourceHost("宝可梦", "https://example.com/sub")
	if err != nil {
		t.Fatal(err)
	}
	count, err := ApplySourcePayload(st, h.ID, []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("want 2 nodes, got %d", count)
	}
	nodes, err := st.ListNodes(h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("want 2 rows, got %d", len(nodes))
	}
	regions := map[string]int{}
	for _, n := range nodes {
		regions[n.Region]++
		if n.Source != NodeSub {
			t.Errorf("unexpected source %q", n.Source)
		}
	}
	if regions["香港"] != 1 || regions["美国"] != 1 {
		t.Fatalf("regions not stored: %+v", regions)
	}

	fresh, err := st.HostByID(h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.SubError != "" || fresh.SubCheckedAt == 0 || fresh.Kind != HostSource {
		t.Fatalf("source state not recorded: %+v", fresh)
	}

	// A refresh replaces the inventory instead of duplicating it.
	if _, err := ApplySourcePayload(st, h.ID, []byte(doc)); err != nil {
		t.Fatal(err)
	}
	again, _ := st.ListNodes(h.ID)
	if len(again) != 2 {
		t.Fatalf("refresh duplicated rows: %d", len(again))
	}
}

func TestSyncSourceKeepsNodesWhenFetchFails(t *testing.T) {
	st, err := OpenStore(t.TempDir() + "/weft.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h, err := st.CreateSourceHost("airport", "http://127.0.0.1:1/sub")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceSourceNodes(h.ID, []Node{{Key: "k", Name: "node", Type: "trojan", Server: "1.1.1.1", Port: 443, Source: NodeSub}}); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncSource(st, h); err == nil {
		t.Fatal("want an error for an unreachable source")
	}
	nodes, _ := st.ListNodes(h.ID)
	if len(nodes) != 1 {
		t.Fatalf("a failed pull must keep the old inventory, got %d nodes", len(nodes))
	}
	fresh, _ := st.HostByID(h.ID)
	if fresh.SubError == "" {
		t.Fatal("want the error recorded on the host")
	}
}

func TestBundleGroupsByRegion(t *testing.T) {
	hosts := []Host{{ID: "h1", Name: "宝可梦"}}
	nodes := []Node{
		{HostID: "h1", Key: "a", Name: "香港01 直连", Region: "香港", Raw: json.RawMessage(`{"name":"a","type":"hysteria2","server":"hk","port":1}`)},
		{HostID: "h1", Key: "b", Name: "日本01 Vless", Region: "日本", Raw: json.RawMessage(`{"name":"b","type":"vless","server":"jp","port":2}`)},
		{HostID: "h1", Key: "c", Name: "未知", Raw: json.RawMessage(`{"name":"c","type":"trojan","server":"x","port":3}`)},
	}
	out, err := BuildMihomoYAML(Bundle{Name: "all", GroupBy: "region", GroupType: "url-test", Template: DefaultTemplate}, hosts, nodes)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{"name: 香港", "name: 日本", "name: 其他", "宝可梦/香港01 直连"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}
