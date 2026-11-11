package weft

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// A subscription source is a host weft pulls itself: an airport or any URL
// that hands out nodes. The panel never shows the raw payload — only the
// flattened, filtered node list — so a chatty airport cannot pollute the picker.

const (
	maxSubBytes       = 8 << 20
	subFetchTimeout   = 30 * time.Second
	subFetchRedirect  = 3
	sourceFetchUA     = "clash-verge/v2.2.3"
	sourceInterval    = 6 * time.Hour
	otherRegion       = "其他"
	subscriptionKind  = NodeSub
	maxNameLength     = 40
	base64ProbePrefix = "://"
)

// noiseWords are the ad and quota banners every airport smuggles in as fake
// nodes. Matching any of them drops the node: the credentials are shared with
// the real ones, so keeping them only produces dead entries in the picker.
var noiseWords = []string{
	"剩余流量", "距离下次", "套餐到期", "到期时间", "重置", "官网", "建议", "通知", "公告",
	"客服", "电报", "流量", "已用", "倍率", "请切换", "订阅", "网址", "购买", "试用",
	"expire", "traffic", "website", "http://", "https://",
}

// FetchSubscription downloads a subscription payload with the same SSRF guard
// rule import uses: scheme allow-list, redirect cap, size cap, and no dialling
// into private ranges.
func FetchSubscription(rawURL string) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, fmt.Errorf("只支持 http 或 https 地址")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	client := &http.Client{
		Timeout:   subFetchTimeout,
		Transport: &http.Transport{DialContext: safeDialContext(dialer), TLSHandshakeTimeout: 10 * time.Second},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= subFetchRedirect {
				return fmt.Errorf("重定向过多")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("只支持 http 或 https 地址")
			}
			return nil
		},
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", sourceFetchUA)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("拉取失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("远程返回 %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSubBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSubBytes {
		return nil, fmt.Errorf("订阅内容超过 %d MB", maxSubBytes>>20)
	}
	return data, nil
}

// ParseSubscription accepts the three shapes airports actually serve: a Clash
// config, a base64 blob of share links, or a plain list of share links.
func ParseSubscription(data []byte) ([]map[string]any, error) {
	text := strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
	if text == "" {
		return nil, fmt.Errorf("订阅内容为空")
	}
	if proxies, ok := parseClashProxies(text); ok {
		return proxies, nil
	}
	if raw, err := decodeSubBase64(text); err == nil {
		decoded := strings.TrimSpace(raw)
		if strings.Contains(decoded, base64ProbePrefix) {
			return parseLinkList(decoded)
		}
	}
	if strings.Contains(text, base64ProbePrefix) {
		return parseLinkList(text)
	}
	return nil, fmt.Errorf("无法识别的订阅格式")
}

func parseClashProxies(text string) ([]map[string]any, bool) {
	if !strings.Contains(text, "proxies:") {
		return nil, false
	}
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, false
	}
	return doc.Proxies, true
}

func decodeSubBase64(text string) (string, error) {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, text)
	clean = strings.NewReplacer("-", "+", "_", "/").Replace(clean)
	clean = strings.TrimRight(clean, "=")
	var lastErr error
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		b, err := enc.DecodeString(clean + strings.Repeat("=", (4-len(clean)%4)%4))
		if err == nil {
			return string(b), nil
		}
		lastErr = err
	}
	return "", lastErr
}

func parseLinkList(text string) ([]map[string]any, error) {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	var proxies []map[string]any
	var firstErr error
	for _, line := range fields {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, base64ProbePrefix) {
			continue
		}
		p, err := ParseProxyLink(line)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		proxies = append(proxies, p)
	}
	if len(proxies) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, fmt.Errorf("没有可用的节点链接")
	}
	return proxies, nil
}

// CleanProxyName strips what an airport adds for its own benefit — flags,
// region banners, multiplier tags — and returns the region separately so
// groups can be built by country instead of by marketing section.
func CleanProxyName(raw string) (name, region string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	lower := strings.ToLower(raw)
	for _, w := range noiseWords {
		if strings.Contains(lower, strings.ToLower(w)) {
			return "", "", false
		}
	}
	region = RegionOf(raw)
	name = stripDecorations(raw)
	if name == "" || strings.Contains(name, base64ProbePrefix) {
		return "", "", false
	}
	if r := []rune(name); len(r) > maxNameLength {
		name = string(r[:maxNameLength])
	}
	return name, region, true
}

var regionTags = []string{
	"亚洲", "欧洲", "北美洲", "南美洲", "非洲", "大洋洲", "亚太", "中东", "全球", "其他", "国际",
}

func stripDecorations(raw string) string {
	var b strings.Builder
	runes := []rune(raw)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if isModifier(r) {
			// skip the pair of regional indicators that make a flag
			if isRegionalIndicator(r) && i+1 < len(runes) && isRegionalIndicator(runes[i+1]) {
				i++
			}
			continue
		}
		if r == '【' || r == '[' {
			if end := closingTag(runes[i:]); end > 0 {
				body := string(runes[i+1 : i+end])
				if !isRegionTag(body) && !isMultiplierTag(body) {
					b.WriteString(body)
					b.WriteString(" ")
				}
				i += end
				continue
			}
		}
		if r == '丨' || r == '|' {
			b.WriteString(" ")
			continue
		}
		b.WriteRune(r)
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	out = strings.Trim(out, " -·,，、:：~•")
	return strings.TrimSpace(out)
}

func closingTag(runes []rune) int {
	if len(runes) == 0 {
		return 0
	}
	close := '】'
	if runes[0] == '[' {
		close = ']'
	}
	for i := 1; i < len(runes); i++ {
		if runes[i] == close {
			return i
		}
	}
	return 0
}

func isRegionTag(body string) bool {
	body = strings.TrimSpace(body)
	for _, t := range regionTags {
		if body == t || strings.HasSuffix(body, t) {
			return true
		}
	}
	return false
}

func isMultiplierTag(body string) bool {
	body = strings.ToLower(strings.TrimSpace(body))
	body = strings.TrimSuffix(body, "倍")
	body = strings.TrimPrefix(body, "x")
	body = strings.TrimPrefix(body, "×")
	body = strings.TrimPrefix(body, "*")
	body = strings.TrimSuffix(body, "x")
	if body == "" {
		return false
	}
	for _, r := range body {
		if !unicode.IsDigit(r) && r != '.' {
			return false
		}
	}
	return true
}

func isRegionalIndicator(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }

func isModifier(r rune) bool {
	switch {
	case isRegionalIndicator(r):
		return true
	case r >= 0x1F300 && r <= 0x1FAFF: // pictographs, symbols
		return true
	case r >= 0x2600 && r <= 0x27BF: // misc symbols, dingbats
		return true
	case r == 0xFE0F || r == 0x200D || r == 0x20E3:
		return true
	}
	return false
}

// RegionOf prefers the flag an airport already encodes, then a country name in
// the node title. Anything else lands in 其他 at group time.
func RegionOf(raw string) string {
	runes := []rune(raw)
	for i := 0; i+1 < len(runes); i++ {
		if isRegionalIndicator(runes[i]) && isRegionalIndicator(runes[i+1]) {
			code := string([]rune{runes[i] - 0x1F1E6 + 'A', runes[i+1] - 0x1F1E6 + 'A'})
			if name, ok := regionNames[code]; ok {
				return name
			}
		}
	}
	for _, name := range regionOrder {
		if strings.Contains(raw, name) {
			return name
		}
	}
	return ""
}

// regionOrder doubles as the fallback keyword scan order and the group order.
var regionOrder = []string{
	"香港", "台湾", "澳门", "日本", "韩国", "新加坡", "马来西亚", "泰国", "越南", "菲律宾",
	"印度尼西亚", "印度", "美国", "加拿大", "巴西", "阿根廷", "英国", "德国", "法国", "荷兰",
	"瑞士", "瑞典", "芬兰", "挪威", "意大利", "西班牙", "波兰", "俄罗斯", "乌克兰", "土耳其",
	"阿联酋", "以色列", "澳大利亚", "新西兰", "南非", "埃及", "中国",
}

var regionNames = map[string]string{
	"HK": "香港", "TW": "台湾", "MO": "澳门", "JP": "日本", "KR": "韩国", "SG": "新加坡",
	"MY": "马来西亚", "TH": "泰国", "VN": "越南", "PH": "菲律宾", "ID": "印度尼西亚",
	"IN": "印度", "US": "美国", "CA": "加拿大", "BR": "巴西", "AR": "阿根廷", "GB": "英国",
	"UK": "英国", "DE": "德国", "FR": "法国", "NL": "荷兰", "CH": "瑞士", "SE": "瑞典",
	"FI": "芬兰", "NO": "挪威", "IT": "意大利", "ES": "西班牙", "PL": "波兰", "RU": "俄罗斯",
	"UA": "乌克兰", "TR": "土耳其", "AE": "阿联酋", "IL": "以色列", "AU": "澳大利亚",
	"NZ": "新西兰", "ZA": "南非", "EG": "埃及", "CN": "中国",
}

// BuildSourceNodes turns raw proxies into the rows we store: cleaned names,
// regions, stable keys, and duplicates dropped by endpoint+credential.
func BuildSourceNodes(proxies []map[string]any) []Node {
	// Airports reuse one endpoint for all their banner nodes, so a real node
	// pointing at the same host:port as a banner is a banner with a nicer name.
	noise := map[string]bool{}
	for _, p := range proxies {
		if _, _, ok := CleanProxyName(strOr(p["name"], "")); ok {
			continue
		}
		if f := endpointFingerprint(p); f != "" {
			noise[f] = true
		}
	}
	seen := map[string]bool{}
	out := make([]Node, 0, len(proxies))
	for _, p := range proxies {
		rawName, _ := p["name"].(string)
		name, region, ok := CleanProxyName(rawName)
		if !ok {
			continue
		}
		server, _ := p["server"].(string)
		if strings.TrimSpace(server) == "" {
			continue
		}
		if noise[endpointFingerprint(p)] {
			continue
		}
		typ, _ := p["type"].(string)
		key := nodeKey(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Node{
			Key:    key,
			Name:   name,
			Type:   typ,
			Server: server,
			Port:   int(toFloat(p["port"])),
			Region: region,
			Source: subscriptionKind,
			Raw:    toJSON(p),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Region != out[j].Region {
			return regionRank(out[i].Region) < regionRank(out[j].Region)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// endpointFingerprint identifies where a node dials, ignoring credentials: the
// same airport ships every node with one password, so credentials do not
// separate a banner from a real node.
func endpointFingerprint(p map[string]any) string {
	server, _ := p["server"].(string)
	if strings.TrimSpace(server) == "" {
		return ""
	}
	typ, _ := p["type"].(string)
	return fmt.Sprintf("%s|%s|%d", strings.ToLower(typ), server, int(toFloat(p["port"])))
}

func regionRank(region string) int {
	if region == "" {
		return len(regionOrder) + 1
	}
	for i, name := range regionOrder {
		if name == region {
			return i
		}
	}
	return len(regionOrder)
}

// SyncSource pulls one subscription and replaces its nodes. A failed pull keeps
// the previous inventory: a flaky airport should not empty a working bundle.
func SyncSource(st *Store, h *Host) (int, error) {
	if strings.TrimSpace(h.SubURL) == "" {
		return 0, fmt.Errorf("订阅地址为空")
	}
	data, err := FetchSubscription(h.SubURL)
	if err != nil {
		st.SetSourceState(h.ID, err.Error(), now())
		return 0, err
	}
	return ApplySourcePayload(st, h.ID, data)
}

// ApplySourcePayload parses a downloaded subscription and swaps the host's
// inventory. Split from the fetch so the parse-and-store half stays testable
// without dialling anything.
func ApplySourcePayload(st *Store, hostID string, data []byte) (int, error) {
	proxies, err := ParseSubscription(data)
	if err != nil {
		st.SetSourceState(hostID, err.Error(), now())
		return 0, err
	}
	nodes := BuildSourceNodes(proxies)
	if len(nodes) == 0 {
		st.SetSourceState(hostID, "订阅里没有可用节点", now())
		return 0, fmt.Errorf("订阅里没有可用节点")
	}
	if err := st.ReplaceSourceNodes(hostID, nodes); err != nil {
		st.SetSourceState(hostID, err.Error(), now())
		return 0, err
	}
	if err := st.SetSourceState(hostID, "", now()); err != nil {
		return len(nodes), err
	}
	return len(nodes), nil
}

// SyncSources refreshes the sources that are due, so one broken airport cannot
// block the others.
func SyncSources(st *Store, due time.Duration) int {
	hosts, err := st.ListHosts()
	if err != nil {
		return 0
	}
	refreshed := 0
	for i := range hosts {
		h := hosts[i]
		if h.Kind != HostSource || strings.TrimSpace(h.SubURL) == "" {
			continue
		}
		if h.SubCheckedAt != 0 && now()-h.SubCheckedAt < int64(due/time.Second) {
			continue
		}
		if _, err := SyncSource(st, &h); err == nil {
			refreshed++
		}
	}
	return refreshed
}

// SyncDueSources refreshes every subscription source whose last pull is older
// than the built-in interval. Called on start and on a slow ticker.
func SyncDueSources(st *Store) int { return SyncSources(st, sourceInterval) }
