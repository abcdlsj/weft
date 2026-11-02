package weft

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// SeedDemo fills an empty store with a plausible fleet so the panel can be
// reviewed without any agent. Only used by `weft serve --demo`.
func SeedDemo(st *Store) (*Bundle, error) {
	hosts := []struct {
		name     string
		codes    []string
		tz       string
		nodeBase int
	}{
		{"hk-01", []string{"HK", "Kowloon", "Shatin"}, "Asia/Hong_Kong", 100},
		{"sg-02", []string{"SG", "Jurong", "Tampines"}, "Asia/Singapore", 200},
		{"jp-03", []string{"TYO", "OSA", "NRT"}, "Asia/Tokyo", 300},
		{"us-04", []string{"LAX", "SJC", "SEA"}, "America/Los_Angeles", 400},
	}
	types := []string{"ss", "trojan", "vmess", "vless", "hysteria2"}
	var members []Member
	var bundles []*Bundle
	hostDelay := map[string]int{}
	for hi, h := range hosts {
		host, err := st.CreateHost(h.name, HostAgent)
		if err != nil {
			return nil, err
		}
		nodes := make([]Node, 0, 12)
		for i := 0; i < 12; i++ {
			typ := types[(hi+i)%len(types)]
			code := h.codes[i%len(h.codes)]
			name := fmt.Sprintf("%s-%02d", code, i+1)
			raw := demoProxy(typ, name, h.nodeBase+i, i)
			n := Node{
				Name:    name,
				Type:    typ,
				Server:  raw["server"].(string),
				Port:    raw["port"].(int),
				Key:     fmt.Sprintf("%s-%d", host.Name, i),
				Alive:   !(hi == 3 && i%5 == 0),
				DelayMS: 18 + rand.Intn(240),
				Raw:     toJSON(raw),
			}
			if !n.Alive {
				n.DelayMS = 0
			}
			nodes = append(nodes, n)
			members = append(members, Member{HostID: host.ID, Key: n.Key})
		}
		if err := st.ReplaceNodes(host.ID, nodes); err != nil {
			return nil, err
		}
		sum, alive := 0, 0
		for _, n := range nodes {
			if n.Alive {
				sum += n.DelayMS
				alive++
			}
		}
		if alive > 0 {
			hostDelay[host.ID] = sum / alive
		}
		st.db.Exec(`UPDATE hosts SET last_seen=?,controller=?,agent_version=?,mihomo_version=? WHERE id=?`,
			now(), "http://127.0.0.1:9090", AgentVersion, "v1.19.2", host.ID)
	}

	// A bare hysteria2 / xray box: no mihomo to read, so its nodes are entered
	// by hand. This is the case weft exists for.
	static, err := st.CreateHost("h2nexus", HostStatic)
	if err != nil {
		return nil, err
	}
	for _, link := range []string{
		"hy2://demo-berlin@203.0.113.24:36906?sni=www.bing.com&insecure=1#Berlin-Hy2",
		"vless://11111111-2222-3333-4444-555555555555@203.0.113.24:8443?encryption=none" +
			"&flow=xtls-rprx-vision&security=reality&sni=www.bing.com&fp=chrome" +
			"&pbk=RgDO6nczQXgj_D-VGus4e3pZHL4bIwFA7fHXocnW9QI&sid=f8a64b66&type=tcp#Berlin-Reality",
	} {
		p, err := ParseProxyLink(link)
		if err != nil {
			return nil, err
		}
		n, err := NodeFromProxy(p)
		if err != nil {
			return nil, err
		}
		if err := st.UpsertNode(static.ID, n); err != nil {
			return nil, err
		}
		members = append(members, Member{HostID: static.ID, Key: n.Key})
	}

	main, err := st.CreateBundle(Bundle{
		Name: "全部线路", Slug: "all", GroupBy: "host", GroupType: "url-test",
		Template: DefaultTemplate,
	})
	if err != nil {
		return nil, err
	}
	if err := st.SetMembers(main.ID, members); err != nil {
		return nil, err
	}
	bundles = append(bundles, main)

	asia, err := st.CreateBundle(Bundle{
		Name: "亚洲低延迟", Slug: "asia", GroupBy: "type", GroupType: "url-test",
		Template: DefaultTemplate,
	})
	if err != nil {
		return nil, err
	}
	var asiaMembers []Member
	for _, m := range members {
		if len(asiaMembers) >= 18 {
			break
		}
		asiaMembers = append(asiaMembers, m)
	}
	if err := st.SetMembers(asia.ID, asiaMembers); err != nil {
		return nil, err
	}
	bundles = append(bundles, asia)

	subA, err := st.CreateSubscription(main.ID, "主力订阅", "mihomo")
	if err != nil {
		return nil, err
	}
	st.db.Exec(`UPDATE subscriptions SET hits=?,last_used_at=?,last_ua=? WHERE id=?`,
		126, now()-420, "ClashMetaForAndroid/2.11.1", subA.ID)
	subB, err := st.CreateSubscription(asia.ID, "Mac · Safari 备用", "base64")
	if err != nil {
		return nil, err
	}
	st.db.Exec(`UPDATE subscriptions SET hits=?,last_used_at=?,last_ua=? WHERE id=?`,
		18, now()-7200, "sing-box/1.11.0", subB.ID)
	if _, err := st.CreateSubscription(asia.ID, "机场面板读取", "text"); err != nil {
		return nil, err
	}

	// Access log so the panel can show which devices actually pull each link.
	access := []struct {
		sub    string
		device string
		ua     string
		ip     string
		at     int64
	}{
		{subA.ID, "Clash Meta for Android", "ClashMetaForAndroid/2.11.1 (mihomo/1.19.2)", "10.0.0.24", now() - 420},
		{subA.ID, "Clash Meta for Android", "ClashMetaForAndroid/2.11.1 (mihomo/1.19.2)", "10.0.0.24", now() - 5400},
		{subA.ID, "Clash Verge", "clash-verge/v2.2.3", "10.0.0.31", now() - 9000},
		{subA.ID, "Shadowrocket", "Shadowrocket/2.2.65 (iPhone17,2; iOS 18.4)", "10.0.0.57", now() - 46800},
		{subB.ID, "sing-box", "sing-box/1.11.0 (darwin arm64)", "10.0.0.31", now() - 7200},
		{subB.ID, "Stash", "Stash/2.5.2 (iPhone16,1; iOS 18.3)", "10.0.0.57", now() - 172800},
	}
	for _, a := range access {
		st.db.Exec(`INSERT INTO sub_access (subscription_id,at,device,ua,ip) VALUES (?,?,?,?,?)`,
			a.sub, a.at, a.device, a.ua, a.ip)
	}

	// Backfill six hours of reports so the overview chart has an honest shape.
	hostRows, _ := st.ListHosts()
	base := now() - 6*3600
	for t := base; t < now(); t += 600 {
		for i, h := range hostRows {
			jitter := rand.Intn(3) - 1
			alive := h.AliveCount + jitter
			if alive < 0 {
				alive = 0
			}
			if alive > h.NodeCount {
				alive = h.NodeCount
			}
			// history follows the fleet's real average, drifting a little
			delay := hostDelay[h.ID] + rand.Intn(9) - 4 + int(7*math.Sin(float64(t)/5400)+float64(i))
			st.db.Exec(`INSERT OR REPLACE INTO reports (host_id,at,node_count,alive_count,avg_delay) VALUES (?,?,?,?,?)`,
				h.ID, t, h.NodeCount, alive, delay)
		}
	}
	return main, nil
}

func demoProxy(typ, name string, seed, variant int) map[string]any {
	server := fmt.Sprintf("198.51.100.%d", 10+seed%200)
	port := 443
	switch typ {
	case "ss":
		return map[string]any{
			"name": name, "type": "ss", "server": server, "port": 8388 + variant,
			"cipher":   []string{"aes-128-gcm", "chacha20-ietf-poly1305"}[variant%2],
			"password": fmt.Sprintf("weft-demo-%d", seed), "udp": true,
		}
	case "trojan":
		return map[string]any{
			"name": name, "type": "trojan", "server": server, "port": port,
			"password": fmt.Sprintf("weft-%d", seed), "sni": "cdn.example.net",
			"skip-cert-verify": false, "udp": true,
		}
	case "vmess":
		return map[string]any{
			"name": name, "type": "vmess", "server": server, "port": port,
			"uuid":    fmt.Sprintf("6f1c2d34-0a9b-4c5d-8e7f-%012d", seed),
			"alterId": 0, "cipher": "auto", "network": "ws", "tls": true,
			"udp": true,
		}
	case "vless":
		return map[string]any{
			"name": name, "type": "vless", "server": server, "port": port,
			"uuid":    fmt.Sprintf("9b8a7c6d-5e4f-4321-9abc-%012d", seed),
			"network": "tcp", "tls": true, "servername": "edge.example.net",
			"flow": "xtls-rprx-vision", "udp": true,
		}
	default: // hysteria2
		return map[string]any{
			"name": name, "type": "hysteria2", "server": server, "port": 8443,
			"password": fmt.Sprintf("hy2-%d", seed), "sni": "hy.example.net",
			"skip-cert-verify": true,
		}
	}
}

// TickDemo keeps the seeded fleet feeling alive: reports keep arriving and
// latency drifts. It never invents new hosts or nodes.
func TickDemo(st *Store, stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastReport := time.Now()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			st.db.Exec(`UPDATE hosts SET last_seen=?`, now())
			st.db.Exec(`UPDATE nodes SET delay_ms = MAX(9, delay_ms + (ABS(RANDOM()) % 11) - 5)
WHERE alive=1 AND ABS(RANDOM()) % 3 = 0`)
			if time.Since(lastReport) > 60*time.Second {
				lastReport = time.Now()
				hosts, _ := st.ListHosts()
				for _, h := range hosts {
					var avg int
					st.db.QueryRow(`SELECT COALESCE(AVG(delay_ms),0) FROM nodes WHERE host_id=? AND alive=1`, h.ID).Scan(&avg)
					st.db.Exec(`INSERT OR REPLACE INTO reports (host_id,at,node_count,alive_count,avg_delay) VALUES (?,?,?,?,?)`,
						h.ID, now(), h.NodeCount, h.AliveCount, avg)
				}
			}
		}
	}
}

func toJSONAny(v any) json.RawMessage { return toJSON(v) }
