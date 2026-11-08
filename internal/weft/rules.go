package weft

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// GFWListURL is the canonical gfwlist export. Importing it is the quickest way
// to give a new weft instance a working split-tunnel rule set.
const GFWListURL = "https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt"

// RulePreset is a starting point shown in the panel. The strings are full
// rule-set documents: `rules`, `rule-providers`, and optional extra groups.
type RulePreset struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

const presetGFW = `rule-providers:
  gfw:
    type: http
    behavior: domain
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/gfw.txt
    path: ./ruleset/gfw.yaml
    interval: 86400
rules:
  - RULE-SET,gfw,PROXY
  - GEOIP,CN,DIRECT
  - MATCH,PROXY
`

const presetLoyalsoldier = `rule-providers:
  reject:
    type: http
    behavior: domain
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/reject.txt
    path: ./ruleset/reject.yaml
    interval: 86400
  private:
    type: http
    behavior: domain
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/private.txt
    path: ./ruleset/private.yaml
    interval: 86400
  gfw:
    type: http
    behavior: domain
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/gfw.txt
    path: ./ruleset/gfw.yaml
    interval: 86400
  proxy:
    type: http
    behavior: domain
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/proxy.txt
    path: ./ruleset/proxy.yaml
    interval: 86400
  direct:
    type: http
    behavior: domain
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/direct.txt
    path: ./ruleset/direct.yaml
    interval: 86400
  cncidr:
    type: http
    behavior: ipcidr
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/cncidr.txt
    path: ./ruleset/cncidr.yaml
    interval: 86400
  lancidr:
    type: http
    behavior: ipcidr
    url: https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/lancidr.txt
    path: ./ruleset/lancidr.yaml
    interval: 86400
rules:
  - RULE-SET,private,DIRECT
  - RULE-SET,lancidr,DIRECT
  - RULE-SET,reject,REJECT
  - RULE-SET,gfw,PROXY
  - RULE-SET,proxy,PROXY
  - RULE-SET,direct,DIRECT
  - RULE-SET,cncidr,DIRECT
  - GEOIP,CN,DIRECT
  - MATCH,PROXY
`

const presetGeoSite = `rules:
  - GEOSITE,private,DIRECT
  - GEOSITE,category-ads-all,REJECT
  - GEOSITE,geolocation-!cn,PROXY
  - GEOIP,CN,DIRECT
  - MATCH,PROXY
`

const presetAIMedia = `proxy-groups:
  - name: AI
    type: select
    proxies: [PROXY, AUTO, DIRECT]
  - name: Media
    type: select
    proxies: [PROXY, AUTO, DIRECT]
rules:
  - DOMAIN-SUFFIX,openai.com,AI
  - DOMAIN-SUFFIX,chatgpt.com,AI
  - DOMAIN-SUFFIX,anthropic.com,AI
  - DOMAIN-SUFFIX,claude.ai,AI
  - DOMAIN-SUFFIX,gemini.google.com,AI
  - GEOSITE,netflix,Media
  - GEOSITE,disney,Media
  - GEOSITE,youtube,Media
  - GEOIP,CN,DIRECT
  - MATCH,PROXY
`

// RulePresets is a library of sensible starting points. They are copied into a
// user rule set on first use, so editing one never changes the preset.
func RulePresets() []RulePreset {
	return []RulePreset{
		{"gfw", "GFW 列表", "引用 Loyalsoldier 维护的 GFW 域名集，国内直连、其余走代理。", presetGFW},
		{"loyalsoldier", "Loyalsoldier 全套", "广告拦截、局域网直连、GFW、国内 CIDR 一次配齐。", presetLoyalsoldier},
		{"geosite", "GeoSite 分流", "用 mihomo 自带的 geosite / geoip 数据分流，不依赖远程规则集。", presetGeoSite},
		{"ai-media", "AI 与流媒体分组", "额外建 AI、Media 两个组，便于把它们固定到某条线路。", presetAIMedia},
	}
}

// ValidateRuleSetContent checks the three keys weft knows how to merge. Unknown
// keys are rejected so a pasted full config cannot silently override DNS or
// ports behind the panel's back.
func ValidateRuleSetContent(content string) error {
	doc, err := decodeRuleSet(content)
	if err != nil {
		return err
	}
	if len(doc) == 0 {
		return fmt.Errorf("规则内容为空")
	}
	for key := range doc {
		switch key {
		case "rules", "rule-providers", "proxy-groups":
		default:
			return fmt.Errorf("不支持的键 %q：只允许 rules、rule-providers、proxy-groups", key)
		}
	}
	if raw, ok := doc["rules"]; ok {
		list, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("rules 必须是列表")
		}
		for _, item := range list {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("rules 的每一项都必须是字符串")
			}
		}
	}
	if raw, ok := doc["rule-providers"]; ok {
		if _, ok := raw.(map[string]any); !ok {
			return fmt.Errorf("rule-providers 必须是映射")
		}
	}
	if raw, ok := doc["proxy-groups"]; ok {
		list, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("proxy-groups 必须是列表")
		}
		for _, item := range list {
			g, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("proxy-groups 的每一项都必须是映射")
			}
			if strOr(g["name"], "") == "" || strOr(g["type"], "") == "" {
				return fmt.Errorf("proxy-groups 的每一项都需要 name 与 type")
			}
		}
	}
	return nil
}

func decodeRuleSet(content string) (map[string]any, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("规则内容为空")
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, fmt.Errorf("规则不是合法 YAML：%w", err)
	}
	return doc, nil
}

// ApplyRuleSet merges a user rule set into a generated mihomo config. Rules are
// replaced; providers are merged; extra groups are appended only when the
// template and generator do not already define that name.
func ApplyRuleSet(root *yaml.Node, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	doc, err := decodeRuleSet(content)
	if err != nil {
		return err
	}
	if raw, ok := doc["rules"]; ok {
		rules, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("rules 必须是列表")
		}
		out := make([]string, 0, len(rules)+1)
		hasFinal := false
		for _, item := range rules {
			rule, ok := item.(string)
			if !ok {
				return fmt.Errorf("rules 的每一项都必须是字符串")
			}
			out = append(out, rule)
			head := strings.ToUpper(strings.TrimSpace(strings.SplitN(rule, ",", 2)[0]))
			if head == "MATCH" || head == "FINAL" {
				hasFinal = true
			}
		}
		if !hasFinal {
			out = append(out, "MATCH,PROXY")
		}
		setKey(root, "rules", nodeOf(out))
	}
	if raw, ok := doc["rule-providers"]; ok {
		providers, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("rule-providers 必须是映射")
		}
		mergeProviders(root, providers)
	}
	if raw, ok := doc["proxy-groups"]; ok {
		groups, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("proxy-groups 必须是列表")
		}
		appendMissingGroups(root, groups)
	}
	return nil
}

func mergeProviders(root *yaml.Node, providers map[string]any) {
	if len(providers) == 0 {
		return
	}
	node, ok := valueOf(root, "rule-providers")
	if !ok || node.Kind != yaml.MappingNode {
		setKey(root, "rule-providers", nodeOf(providers))
		return
	}
	keys := make([]string, 0, len(providers))
	for k := range providers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		setKey(node, k, nodeOf(providers[k]))
	}
}

func appendMissingGroups(root *yaml.Node, groups []any) {
	node, ok := valueOf(root, "proxy-groups")
	if !ok || node.Kind != yaml.SequenceNode {
		setKey(root, "proxy-groups", nodeOf(groups))
		return
	}
	present := map[string]bool{}
	for _, g := range node.Content {
		if g.Kind != yaml.MappingNode {
			continue
		}
		if name, ok := valueOf(g, "name"); ok {
			present[name.Value] = true
		}
	}
	for _, raw := range groups {
		g, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strOr(g["name"], "")
		if name == "" || present[name] {
			continue
		}
		present[name] = true
		node.Content = append(node.Content, nodeOf(g))
	}
}

// ImportRuleSetContent turns a remote document into a rule-set document. It
// accepts: gfwlist base64, a plain domain list, a clash rule-provider payload,
// a list of mihomo rule lines, or a YAML document with a `rules` key.
func ImportRuleSetContent(data []byte) (string, error) {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("远程内容为空")
	}
	compact := strings.Join(strings.Fields(text), "")
	if decoded, err := decodeBase64(compact); err == nil && looksLikeGFW(decoded) {
		text = decoded
	}
	// A full YAML document wins: it can carry providers and groups as well.
	if strings.Contains(text, "rules:") || strings.Contains(text, "rule-providers:") {
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(text), &doc); err == nil && doc != nil {
			trimmed := map[string]any{}
			for _, key := range []string{"rules", "rule-providers", "proxy-groups"} {
				if v, ok := doc[key]; ok {
					trimmed[key] = v
				}
			}
			if len(trimmed) > 0 {
				out, err := yaml.Marshal(trimmed)
				if err != nil {
					return "", err
				}
				if err := ValidateRuleSetContent(string(out)); err != nil {
					return "", err
				}
				return string(out), nil
			}
		}
	}
	// gfwlist uses base64 plus Adblock-style ||domain and @@ exceptions.
	if looksLikeGFW(text) {
		return gfwToRules(text)
	}
	// Other domain lists (Loyalsoldier .txt, Clash rule-provider payload).
	if rules, ok := payloadToRules(text); ok {
		out, err := yaml.Marshal(map[string]any{"rules": append(rules, "MATCH,PROXY")})
		return string(out), err
	}
	// Last resort: one mihomo rule per line.
	var rules []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		if strings.Count(line, ",") >= 2 {
			rules = append(rules, line)
		}
	}
	if len(rules) == 0 {
		return "", fmt.Errorf("无法识别这个规则源")
	}
	out, err := yaml.Marshal(map[string]any{"rules": append(rules, "MATCH,PROXY")})
	return string(out), err
}

func looksLikeGFW(text string) bool {
	return strings.Contains(text, "||") || strings.Contains(text, "[AutoProxy")
}

func gfwToRules(text string) (string, error) {
	direct := map[string]bool{}
	proxy := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "[") {
			continue
		}
		if strings.HasPrefix(line, "@@") {
			if d := gfwDomain(strings.TrimPrefix(line, "@@")); d != "" {
				direct[d] = true
			}
			continue
		}
		if d := gfwDomain(line); d != "" {
			proxy[d] = true
		}
	}
	if len(proxy) == 0 && len(direct) == 0 {
		return "", fmt.Errorf("列表里没有可用的域名")
	}
	rules := make([]string, 0, len(direct)+len(proxy)+1)
	for _, d := range sortedKeys(direct) {
		rules = append(rules, "DOMAIN-SUFFIX,"+d+",DIRECT")
	}
	for _, d := range sortedKeys(proxy) {
		if direct[d] {
			continue
		}
		rules = append(rules, "DOMAIN-SUFFIX,"+d+",PROXY")
	}
	rules = append(rules, "MATCH,PROXY")
	out, err := yaml.Marshal(map[string]any{"rules": rules})
	return string(out), err
}

// gfwDomain reduces an Adblock-ish pattern to its bare domain.
func gfwDomain(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "||")
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimPrefix(line, ".")
	if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
		if u, err := url.Parse(line); err == nil {
			line = u.Hostname()
		}
	}
	if i := strings.IndexAny(line, "/^$*?"); i >= 0 {
		line = line[:i]
	}
	line = strings.Trim(line, ".[]")
	if !strings.Contains(line, ".") || strings.ContainsAny(line, " @") {
		return ""
	}
	return strings.ToLower(line)
}

func payloadToRules(text string) ([]string, bool) {
	var rules []string
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") ||
			strings.HasPrefix(line, "payload:") {
			continue
		}
		line = strings.TrimPrefix(line, "- ")
		line = strings.Trim(line, `"'`)
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "+."):
			line = strings.TrimPrefix(line, "+.")
		case strings.HasPrefix(line, "."):
			line = strings.TrimPrefix(line, ".")
		case strings.HasPrefix(line, "*."):
			line = strings.TrimPrefix(line, "*.")
		}
		if !strings.Contains(line, ".") || strings.ContainsAny(line, " ,/@") {
			continue
		}
		if seen[line] {
			continue
		}
		seen[line] = true
		rules = append(rules, "DOMAIN-SUFFIX,"+line+",PROXY")
	}
	return rules, len(rules) > 0
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// FetchRuleSource downloads a rule source with a hard size and time limit.
func FetchRuleSource(rawURL string) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("只支持 http 或 https 地址")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("拉取失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("远程返回 %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// CountRules returns simple counts for the panel. It never fails: a broken rule
// set is still editable, so the list should still show something.
func CountRules(content string) (rules, providers int) {
	doc, err := decodeRuleSet(content)
	if err != nil {
		return 0, 0
	}
	if list, ok := doc["rules"].([]any); ok {
		rules = len(list)
	}
	if m, ok := doc["rule-providers"].(map[string]any); ok {
		providers = len(m)
	}
	return rules, providers
}
