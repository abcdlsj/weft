package weft

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Share links are the only sane way to hand a hand-typed node to weft: the
// panel has a form for editing, but nobody wants to type a Reality key twice.
// ParseProxyLink accepts what clients already export.

func ParseProxyLink(link string) (map[string]any, error) {
	link = strings.TrimSpace(link)
	if link == "" {
		return nil, fmt.Errorf("链接为空")
	}
	switch {
	case strings.HasPrefix(link, "hy2://"), strings.HasPrefix(link, "hysteria2://"):
		return parseHysteria2(link)
	case strings.HasPrefix(link, "vless://"):
		return parseVLESS(link)
	case strings.HasPrefix(link, "trojan://"):
		return parseTrojan(link)
	case strings.HasPrefix(link, "ss://"):
		return parseSS(link)
	case strings.HasPrefix(link, "vmess://"):
		return parseVMess(link)
	}
	scheme := link
	if i := strings.Index(scheme, "://"); i > 0 {
		scheme = scheme[:i]
	}
	return nil, fmt.Errorf("暂不支持 %s 链接", scheme)
}

func parseHysteria2(link string) (map[string]any, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("hy2 链接无法解析：%w", err)
	}
	port, err := portOf(u)
	if err != nil {
		return nil, err
	}
	m := map[string]any{
		"type":     "hysteria2",
		"server":   u.Hostname(),
		"port":     port,
		"password": u.User.Username(),
	}
	q := u.Query()
	setString(m, "sni", firstOf(q, "sni", "peer"))
	setString(m, "obfs", q.Get("obfs"))
	setString(m, "obfs-password", firstOf(q, "obfs-password", "obfs_password"))
	setString(m, "fingerprint", firstOf(q, "pinSHA256", "pinsha256"))
	if alpn := q.Get("alpn"); alpn != "" {
		m["alpn"] = splitList(alpn)
	}
	if truthy(firstOf(q, "insecure", "allowInsecure")) {
		m["skip-cert-verify"] = true
	}
	if up := q.Get("up"); up != "" {
		m["up"] = up
	}
	if down := q.Get("down"); down != "" {
		m["down"] = down
	}
	m["name"] = nameOf(u, "hysteria2", u.Hostname(), port)
	return m, nil
}

func parseVLESS(link string) (map[string]any, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("vless 链接无法解析：%w", err)
	}
	port, err := portOf(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	m := map[string]any{
		"type":   "vless",
		"server": u.Hostname(),
		"port":   port,
		"uuid":   u.User.Username(),
	}
	if flow := q.Get("flow"); flow != "" {
		m["flow"] = flow
	}
	security := strings.ToLower(firstOf(q, "security", "tls"))
	if security == "tls" || security == "reality" || security == "xtls" {
		m["tls"] = true
	}
	setString(m, "servername", firstOf(q, "sni", "peer", "servername"))
	setString(m, "client-fingerprint", firstOf(q, "fp", "fingerprint"))
	if security == "reality" {
		opts := map[string]any{}
		setNested(opts, "public-key", firstOf(q, "pbk", "public-key"))
		setNested(opts, "short-id", firstOf(q, "sid", "short-id"))
		if len(opts) > 0 {
			m["reality-opts"] = opts
		}
	}
	network := firstOf(q, "type", "network")
	if network == "" {
		network = "tcp"
	}
	m["network"] = network
	switch network {
	case "ws", "httpupgrade":
		ws := map[string]any{}
		if path := q.Get("path"); path != "" {
			ws["path"] = path
		}
		if host := q.Get("host"); host != "" {
			ws["headers"] = map[string]any{"Host": host}
		}
		if len(ws) > 0 {
			m["ws-opts"] = ws
		}
	case "grpc":
		if svc := firstOf(q, "serviceName", "service"); svc != "" {
			m["grpc-opts"] = map[string]any{"grpc-service-name": svc}
		}
	}
	m["name"] = nameOf(u, "vless", u.Hostname(), port)
	return m, nil
}

func parseTrojan(link string) (map[string]any, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("trojan 链接无法解析：%w", err)
	}
	port, err := portOf(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	m := map[string]any{
		"type":     "trojan",
		"server":   u.Hostname(),
		"port":     port,
		"password": u.User.Username(),
	}
	setString(m, "sni", firstOf(q, "sni", "peer"))
	setString(m, "alpn", q.Get("alpn"))
	if truthy(firstOf(q, "allowInsecure", "insecure")) {
		m["skip-cert-verify"] = true
	}
	if network := q.Get("type"); network == "ws" {
		m["network"] = "ws"
		ws := map[string]any{}
		if path := q.Get("path"); path != "" {
			ws["path"] = path
		}
		if host := q.Get("host"); host != "" {
			ws["headers"] = map[string]any{"Host": host}
		}
		if len(ws) > 0 {
			m["ws-opts"] = ws
		}
	}
	m["name"] = nameOf(u, "trojan", u.Hostname(), port)
	return m, nil
}

func parseSS(link string) (map[string]any, error) {
	body := strings.TrimPrefix(link, "ss://")
	fragment := ""
	if i := strings.Index(body, "#"); i >= 0 {
		fragment = body[i+1:]
		body = body[:i]
	}
	if i := strings.Index(body, "?"); i > 0 {
		body = body[:i]
	}

	var methodPass, hostport string
	if at := strings.LastIndex(body, "@"); at >= 0 {
		userinfo := body[:at]
		hostport = body[at+1:]
		if decoded, err := decodeBase64(userinfo); err == nil {
			methodPass = decoded
		} else {
			methodPass = userinfo
		}
	} else {
		decoded, err := decodeBase64(body)
		if err != nil {
			return nil, fmt.Errorf("ss 链接无法解析")
		}
		at := strings.LastIndex(decoded, "@")
		if at < 0 {
			return nil, fmt.Errorf("ss 链接缺少服务器地址")
		}
		methodPass, hostport = decoded[:at], decoded[at+1:]
	}
	method, password, ok := strings.Cut(methodPass, ":")
	if !ok {
		return nil, fmt.Errorf("ss 链接缺少加密方式")
	}
	host, portStr, ok := strings.Cut(hostport, ":")
	if !ok {
		return nil, fmt.Errorf("ss 链接缺少端口")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("ss 端口无效：%s", portStr)
	}
	name := ""
	if fragment != "" {
		name, _ = url.QueryUnescape(fragment)
	}
	if name == "" {
		name = fmt.Sprintf("ss %s:%d", host, port)
	}
	return map[string]any{
		"name":     name,
		"type":     "ss",
		"server":   host,
		"port":     port,
		"cipher":   method,
		"password": password,
	}, nil
}

func parseVMess(link string) (map[string]any, error) {
	decoded, err := decodeBase64(strings.TrimPrefix(link, "vmess://"))
	if err != nil {
		return nil, fmt.Errorf("vmess 链接无法解析：%w", err)
	}
	var vm struct {
		PS   string `json:"ps"`
		Add  string `json:"add"`
		Port any    `json:"port"`
		ID   string `json:"id"`
		Aid  any    `json:"aid"`
		Scy  string `json:"scy"`
		Net  string `json:"net"`
		Host string `json:"host"`
		Path string `json:"path"`
		TLS  string `json:"tls"`
		SNI  string `json:"sni"`
	}
	if err := json.Unmarshal([]byte(decoded), &vm); err != nil {
		return nil, fmt.Errorf("vmess 内容不是合法 JSON：%w", err)
	}
	port := intOf(vm.Port)
	if vm.Add == "" || port == 0 || vm.ID == "" {
		return nil, fmt.Errorf("vmess 链接缺少必需字段")
	}
	m := map[string]any{
		"type":    "vmess",
		"server":  vm.Add,
		"port":    port,
		"uuid":    vm.ID,
		"alterId": int(toFloat(vm.Aid)),
		"cipher":  "auto",
	}
	if vm.Scy != "" {
		m["cipher"] = vm.Scy
	}
	network := vm.Net
	if network == "" {
		network = "tcp"
	}
	m["network"] = network
	if vm.TLS == "tls" {
		m["tls"] = true
	}
	setString(m, "servername", vm.SNI)
	if network == "ws" {
		ws := map[string]any{}
		if vm.Path != "" {
			ws["path"] = vm.Path
		}
		if vm.Host != "" {
			ws["headers"] = map[string]any{"Host": vm.Host}
		}
		if len(ws) > 0 {
			m["ws-opts"] = ws
		}
	}
	name := vm.PS
	if name == "" {
		name = fmt.Sprintf("vmess %s:%d", vm.Add, port)
	}
	m["name"] = name
	return m, nil
}

// NodeFromProxy validates a proxy map and derives the index columns. Anything
// missing that a dialer would need is an error here rather than a broken
// subscription later.
func NodeFromProxy(p map[string]any) (Node, error) {
	name := strings.TrimSpace(strOr(p["name"], ""))
	typ := strings.ToLower(strings.TrimSpace(strOr(p["type"], "")))
	server := strings.TrimSpace(strOr(p["server"], ""))
	port := intOf(p["port"])
	if name == "" {
		return Node{}, fmt.Errorf("节点需要名字")
	}
	if strings.ContainsAny(name, "/\\") {
		return Node{}, fmt.Errorf("节点名不能含斜杠")
	}
	if typ == "" {
		return Node{}, fmt.Errorf("节点需要协议类型")
	}
	if server == "" {
		return Node{}, fmt.Errorf("节点需要服务器地址")
	}
	if port <= 0 || port > 65535 {
		return Node{}, fmt.Errorf("节点端口无效")
	}
	p["name"] = name
	p["type"] = typ
	p["port"] = port
	return Node{
		Key:    nodeKey(p),
		Name:   name,
		Type:   typ,
		Server: server,
		Port:   port,
		Alive:  true,
		Raw:    toJSON(p),
	}, nil
}

func portOf(u *url.URL) (int, error) {
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("端口无效：%q", u.Port())
	}
	return port, nil
}

func nameOf(u *url.URL, typ, host string, port int) string {
	if u.Fragment != "" {
		if name, err := url.QueryUnescape(u.Fragment); err == nil && strings.TrimSpace(name) != "" {
			return name
		}
	}
	return fmt.Sprintf("%s %s:%d", typ, host, port)
}

func firstOf(q url.Values, keys ...string) string {
	for _, k := range keys {
		if v := q.Get(k); v != "" {
			return v
		}
	}
	return ""
}

func setString(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}

func setNested(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}

// intOf reads a number from a map that may have come from JSON (float64), from
// a Go literal (int) or from a share link (string).
func intOf(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}

func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func splitList(v string) []string {
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '|' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func decodeBase64(v string) (string, error) {
	v = strings.TrimSpace(v)
	if decoded, err := base64.RawURLEncoding.DecodeString(v); err == nil {
		return string(decoded), nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(v); err == nil {
		return string(decoded), nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
		return string(decoded), nil
	}
	return "", fmt.Errorf("不是合法的 base64")
}
