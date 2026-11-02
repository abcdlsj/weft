package weft

import "strings"

// deviceRule maps a User-Agent substring to the client a person would name.
// Order matters: the more specific names come first.
var deviceRules = []struct {
	match string
	name  string
}{
	{"clashmetaforandroid", "Clash Meta for Android"},
	{"clash verge", "Clash Verge"},
	{"clash-verge", "Clash Verge"},
	{"clashx pro", "ClashX Pro"},
	{"clashx", "ClashX"},
	{"clash for windows", "Clash for Windows"},
	{"mihomo party", "Mihomo Party"},
	{"mihomo", "mihomo"},
	{"flclash", "FlClash"},
	{"karing", "Karing"},
	{"hiddify", "Hiddify"},
	{"stash", "Stash"},
	{"shadowrocket", "Shadowrocket"},
	{"quantumult", "Quantumult X"},
	{"loon", "Loon"},
	{"surge", "Surge"},
	{"sing-box", "sing-box"},
	{"v2rayn", "v2rayN"},
	{"v2rayng", "v2rayNG"},
	{"v2ray", "v2ray"},
	{"nekoray", "NekoRay"},
	{"nekobox", "NekoBox"},
	{"openclash", "OpenClash"},
	{"passwall", "PassWall"},
	{"matsuri", "Matsuri"},
	{"sfi", "SagerNet"},
	{"sagernet", "SagerNet"},
	{"clash", "Clash"},
}

// DetectDevice names the client behind a subscription request. Unknown agents
// are grouped by their first product token so the panel still shows a shape.
func DetectDevice(ua string) string {
	if strings.TrimSpace(ua) == "" {
		return "未知客户端"
	}
	lower := strings.ToLower(ua)
	for _, rule := range deviceRules {
		if strings.Contains(lower, rule.match) {
			return rule.name
		}
	}
	// "/" separates a product from its version in most agent strings.
	head := ua
	if i := strings.IndexAny(head, "/ "); i > 0 {
		head = head[:i]
	}
	if len(head) > 24 {
		head = head[:24]
	}
	if head == "" {
		return "未知客户端"
	}
	return head
}

// ClientIP is the address that reached us, honouring one proxy hop.
func ClientIP(remoteAddr, forwardedFor string) string {
	if forwardedFor != "" {
		first := strings.TrimSpace(strings.Split(forwardedFor, ",")[0])
		if first != "" {
			return first
		}
	}
	if i := strings.LastIndex(remoteAddr, ":"); i > 0 {
		return strings.Trim(remoteAddr[:i], "[]")
	}
	return remoteAddr
}
