package weft

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	webassets "github.com/abcdlsj/weft/web"
)

const sessionCookie = "weft_session"

type Server struct {
	store    *Store
	mux      *http.ServeMux
	demo     bool
	mu       sync.Mutex
	sessions map[string]int64 // token -> expiry unix
}

func NewServer(st *Store, demo bool) *Server {
	s := &Server{store: st, mux: http.NewServeMux(), demo: demo, sessions: map[string]int64{}}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	// public
	s.mux.HandleFunc("GET /api/me", s.handleMe)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/setup", s.handleSetup)
	s.mux.HandleFunc("POST /api/agent/report", s.handleAgentReport)
	s.mux.HandleFunc("GET /sub/{token}", s.handleSubscription)

	// authenticated
	s.mux.HandleFunc("POST /api/logout", s.authed(s.handleLogout))
	s.mux.HandleFunc("GET /api/state", s.authed(s.handleState))
	s.mux.HandleFunc("POST /api/hosts", s.authed(s.handleCreateHost))
	s.mux.HandleFunc("DELETE /api/hosts/{id}", s.authed(s.handleDeleteHost))
	s.mux.HandleFunc("PATCH /api/hosts/{id}", s.authed(s.handlePatchHost))
	s.mux.HandleFunc("POST /api/hosts/{id}/nodes", s.authed(s.handleCreateNode))
	s.mux.HandleFunc("PUT /api/hosts/{id}/nodes/{key}", s.authed(s.handleUpdateNode))
	s.mux.HandleFunc("DELETE /api/hosts/{id}/nodes/{key}", s.authed(s.handleDeleteNode))
	s.mux.HandleFunc("POST /api/nodes/parse", s.authed(s.handleParseNode))
	s.mux.HandleFunc("POST /api/bundles", s.authed(s.handleCreateBundle))
	s.mux.HandleFunc("PUT /api/bundles/{id}", s.authed(s.handleUpdateBundle))
	s.mux.HandleFunc("DELETE /api/bundles/{id}", s.authed(s.handleDeleteBundle))
	s.mux.HandleFunc("GET /api/bundles/{id}/preview", s.authed(s.handlePreview))
	s.mux.HandleFunc("POST /api/subscriptions", s.authed(s.handleCreateSubscription))
	s.mux.HandleFunc("PATCH /api/subscriptions/{id}", s.authed(s.handlePatchSubscription))
	s.mux.HandleFunc("DELETE /api/subscriptions/{id}", s.authed(s.handleDeleteSubscription))
	s.mux.HandleFunc("GET /api/subscriptions/{id}/qr.png", s.authed(s.handleQR))
	s.mux.HandleFunc("POST /api/password", s.authed(s.handlePassword))
	s.mux.HandleFunc("PUT /api/settings", s.authed(s.handleSettings))

	// panel
	s.mux.HandleFunc("GET /", s.handleStatic)
}

// ---------- auth ----------

func (s *Server) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticated(r) {
			writeErr(w, http.StatusUnauthorized, "未登录")
			return
		}
		next(w, r)
	}
}

func (s *Server) authenticated(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[c.Value]
	if !ok || exp < now() {
		delete(s.sessions, c.Value)
		return false
	}
	return true
}

// PasswordSet reports whether the panel password has been created.
func (s *Server) PasswordSet() bool { return s.passwordSet() }

func (s *Server) passwordSet() bool {
	_, err := s.store.GetSetting("password_hash")
	return err == nil
}

func hashPassword(password, salt string) (string, error) {
	key, err := pbkdf2.Key(sha256.New, password, []byte(salt), 120_000, 32)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

// SetPassword replaces the panel password. It is exported for first-run setup
// from the CLI (`weft serve --password`).
func SetPassword(st *Store, password string) error { return (&Server{store: st}).setPassword(password) }

func (s *Server) setPassword(password string) error {
	saltB := make([]byte, 16)
	rand.Read(saltB)
	salt := hex.EncodeToString(saltB)
	h, err := hashPassword(password, salt)
	if err != nil {
		return err
	}
	if err := s.store.SetSetting("password_salt", salt); err != nil {
		return err
	}
	return s.store.SetSetting("password_hash", h)
}

func (s *Server) checkPassword(password string) bool {
	salt, err := s.store.GetSetting("password_salt")
	if err != nil {
		return false
	}
	want, err := s.store.GetSetting("password_hash")
	if err != nil {
		return false
	}
	got, err := hashPassword(password, salt)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func (s *Server) newSession(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 32)
	rand.Read(b)
	tok := hex.EncodeToString(b)
	s.mu.Lock()
	s.sessions[tok] = now() + 30*86400
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
		MaxAge:   30 * 86400,
	})
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"authenticated": s.authenticated(r),
		"needsSetup":    !s.passwordSet(),
		"demo":          s.demo,
	})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s.passwordSet() {
		writeErr(w, http.StatusConflict, "已完成初始化")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Password) < 6 {
		writeErr(w, http.StatusBadRequest, "口令至少 6 位")
		return
	}
	if err := s.setPassword(body.Password); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.newSession(w, r)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	if !s.checkPassword(body.Password) {
		time.Sleep(300 * time.Millisecond)
		writeErr(w, http.StatusUnauthorized, "口令错误")
		return
	}
	s.newSession(w, r)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	if !s.checkPassword(body.Current) {
		writeErr(w, http.StatusForbidden, "当前口令错误")
		return
	}
	if len(body.Next) < 6 {
		writeErr(w, 400, "新口令至少 6 位")
		return
	}
	if err := s.setPassword(body.Next); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- agent ----------

func (s *Server) handleAgentReport(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	host, err := s.store.HostByToken(strings.TrimSpace(token))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "agent token 无效")
		return
	}
	var body struct {
		AgentVersion  string `json:"agent_version"`
		Controller    string `json:"controller"`
		MihomoVersion string `json:"mihomo_version"`
		Nodes         []Node `json:"nodes"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(&body); err != nil {
		writeErr(w, 400, "上报负载无效")
		return
	}
	if err := s.store.ReplaceNodes(host.ID, body.Nodes); err != nil {
		log.Printf("agent %s: storing report: %v", host.Name, err)
		writeErr(w, 500, err.Error())
		return
	}
	_, err = s.store.db.Exec(`UPDATE hosts SET last_seen=?,controller=?,agent_version=?,mihomo_version=? WHERE id=?`,
		now(), body.Controller, body.AgentVersion, body.MihomoVersion, host.ID)
	if err != nil {
		log.Printf("agent %s: updating host: %v", host.Name, err)
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "accepted": len(body.Nodes)})
}

// ---------- state ----------

type hostView struct {
	Host
	Status string `json:"status"`
}

func hostStatus(h Host) string {
	if h.Kind == HostStatic {
		return "manual"
	}
	switch {
	case h.LastSeen == 0:
		return "never"
	case now()-h.LastSeen < 45:
		return "online"
	case now()-h.LastSeen < 600:
		return "stale"
	default:
		return "offline"
	}
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.store.ListHosts()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	views := make([]hostView, 0, len(hosts))
	for _, h := range hosts {
		views = append(views, hostView{Host: h, Status: hostStatus(h)})
	}
	nodes, err := s.store.ListNodes("")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	bundles, err := s.store.ListBundles()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	subs, err := s.store.ListSubscriptions()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for i := range subs {
		devices, err := s.store.AccessStats(subs[i].ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		subs[i].Devices = devices
	}
	series, err := s.store.RecentReports(now() - 6*3600)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	tpl, _ := s.store.GetSetting("default_template")
	writeJSON(w, 200, map[string]any{
		"hosts":           views,
		"nodes":           nodes,
		"bundles":         bundles,
		"subscriptions":   subs,
		"series":          bucketSeries(series),
		"publicBase":      publicBase(r),
		"demo":            s.demo,
		"defaultTemplate": tpl,
		"builtinTemplate": DefaultTemplate,
		"stats":           computeStats(views, nodes, subs),
	})
}

func computeStats(hosts []hostView, nodes []Node, subs []Subscription) map[string]any {
	online, alive := 0, 0
	for _, h := range hosts {
		if h.Status == "online" {
			online++
		}
	}
	for _, n := range nodes {
		if n.Alive {
			alive++
		}
	}
	var hits int64
	var lastUse int64
	for _, sub := range subs {
		hits += sub.Hits
		if sub.LastUsedAt > lastUse {
			lastUse = sub.LastUsedAt
		}
	}
	return map[string]any{
		"hostsTotal":  len(hosts),
		"hostsOnline": online,
		"nodesTotal":  len(nodes),
		"nodesAlive":  alive,
		"subHits":     hits,
		"lastUse":     lastUse,
	}
}

// bucketSeries folds raw reports into 10-minute points for the overview chart.
func bucketSeries(reports []Report) []map[string]any {
	if len(reports) == 0 {
		return []map[string]any{}
	}
	const bucket = 600
	type point struct {
		at, nodes, alive, delayWeighted, delayCount int64
	}
	byHost := map[string]map[int64]*point{}
	hosts := map[string]bool{}
	for _, r := range reports {
		b := r.At / bucket * bucket
		if byHost[r.HostID] == nil {
			byHost[r.HostID] = map[int64]*point{}
		}
		p := byHost[r.HostID][b]
		if p == nil {
			p = &point{at: b}
			byHost[r.HostID][b] = p
		}
		p.nodes = int64(r.NodeCount)
		p.alive = int64(r.AliveCount)
		p.delayWeighted = int64(r.AvgDelay) * int64(r.AliveCount)
		p.delayCount = int64(r.AliveCount)
		hosts[r.HostID] = true
	}
	buckets := map[int64]*point{}
	for _, perHost := range byHost {
		for b, p := range perHost {
			agg := buckets[b]
			if agg == nil {
				agg = &point{at: b}
				buckets[b] = agg
			}
			agg.nodes += p.nodes
			agg.alive += p.alive
			agg.delayWeighted += p.delayWeighted
			agg.delayCount += p.delayCount
		}
	}
	keys := make([]int64, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		b := buckets[k]
		delay := int64(0)
		if b.delayCount > 0 {
			delay = b.delayWeighted / b.delayCount
		}
		out = append(out, map[string]any{"at": k, "nodes": b.nodes, "alive": b.alive, "delay": delay})
	}
	return out
}

// ---------- hosts api ----------

func (s *Server) handleCreateHost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || strings.ContainsAny(name, " /\\") {
		writeErr(w, 400, "机器名需非空且不含空格或斜杠")
		return
	}
	h, err := s.store.CreateHost(name, body.Kind)
	if err != nil {
		writeErr(w, 409, "机器名已存在："+name)
		return
	}
	writeJSON(w, 200, h)
}

func (s *Server) handlePatchHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Name   *string `json:"name"`
		Rotate bool    `json:"rotate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" || strings.ContainsAny(name, " /\\") {
			writeErr(w, 400, "机器名需非空且不含空格或斜杠")
			return
		}
		if err := s.store.RenameHost(id, name); err != nil {
			writeErr(w, 409, "机器名已存在")
			return
		}
	}
	out := map[string]any{"ok": true}
	if body.Rotate {
		t, err := s.store.RotateHostToken(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out["token"] = t
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteHost(r.PathValue("id")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- nodes api ----------

// Nodes can arrive two ways: a proxy map from the panel form, or a batch of
// share links pasted straight out of a client. Both end up in the same table.
func (s *Server) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("id")
	if _, err := s.store.HostByID(hostID); err != nil {
		writeErr(w, 404, "机器不存在")
		return
	}
	var body struct {
		Links []string       `json:"links"`
		Proxy map[string]any `json:"proxy"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}

	var proxies []map[string]any
	var failed []string
	for _, link := range body.Links {
		link = strings.TrimSpace(link)
		if link == "" {
			continue
		}
		p, err := ParseProxyLink(link)
		if err != nil {
			failed = append(failed, err.Error())
			continue
		}
		proxies = append(proxies, p)
	}
	if body.Proxy != nil {
		proxies = append(proxies, body.Proxy)
	}
	if len(proxies) == 0 {
		if len(failed) > 0 {
			writeErr(w, 400, strings.Join(failed, "；"))
			return
		}
		writeErr(w, 400, "没有可保存的节点")
		return
	}

	saved := make([]Node, 0, len(proxies))
	for _, p := range proxies {
		n, err := NodeFromProxy(p)
		if err != nil {
			failed = append(failed, err.Error())
			continue
		}
		if err := s.store.UpsertNode(hostID, n); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		saved = append(saved, n)
	}
	writeJSON(w, 200, map[string]any{"saved": saved, "failed": failed})
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	hostID, key := r.PathValue("id"), r.PathValue("key")
	if _, err := s.store.NodeByKey(hostID, key); err != nil {
		writeErr(w, 404, "节点不存在")
		return
	}
	var proxy map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&proxy); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	n, err := NodeFromProxy(proxy)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// the key follows the credential, so an edit that changes the endpoint has
	// to retire the old row before writing the new one
	if err := s.store.DeleteNode(hostID, key); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.store.UpsertNode(hostID, n); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, n)
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteNode(r.PathValue("id"), r.PathValue("key")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleParseNode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	p, err := ParseProxyLink(body.Link)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	n, err := NodeFromProxy(p)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"proxy": n.Raw, "name": n.Name, "type": n.Type})
}

// ---------- bundles api ----------

type bundlePayload struct {
	Name      string   `json:"name"`
	Slug      string   `json:"slug"`
	GroupBy   string   `json:"groupBy"`
	GroupType string   `json:"groupType"`
	Template  string   `json:"template"`
	Members   []Member `json:"members"`
}

func (s *Server) handleCreateBundle(w http.ResponseWriter, r *http.Request) {
	var body bundlePayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeErr(w, 400, "名称不能为空")
		return
	}
	b := Bundle{Name: body.Name, Slug: body.Slug, GroupBy: body.GroupBy, GroupType: body.GroupType, Template: body.Template}
	normalizeBundle(&b)
	if b.Slug == "" {
		b.Slug = slugify(b.Name)
	}
	created, err := s.store.CreateBundle(b)
	if err != nil {
		writeErr(w, 409, "slug 已被占用")
		return
	}
	if err := s.store.SetMembers(created.ID, body.Members); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, created)
}

func (s *Server) handleUpdateBundle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body bundlePayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	b, err := s.store.Bundle(id)
	if err != nil {
		writeErr(w, 404, "未找到")
		return
	}
	if strings.TrimSpace(body.Name) != "" {
		b.Name = body.Name
	}
	b.Slug = body.Slug
	b.GroupBy = body.GroupBy
	b.GroupType = body.GroupType
	b.Template = body.Template
	normalizeBundle(b)
	if b.Slug == "" {
		b.Slug = slugify(b.Name)
	}
	if err := s.store.UpdateBundle(*b); err != nil {
		writeErr(w, 409, "slug 已被占用")
		return
	}
	if err := s.store.SetMembers(id, body.Members); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	updated, _ := s.store.Bundle(id)
	writeJSON(w, 200, updated)
}

func normalizeBundle(b *Bundle) {
	switch b.GroupBy {
	case "host", "type", "flat":
	default:
		b.GroupBy = "host"
	}
	switch b.GroupType {
	case "url-test", "select", "fallback", "load-balance":
	default:
		b.GroupType = "url-test"
	}
}

func (s *Server) handleDeleteBundle(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteBundle(r.PathValue("id")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) bundleNodes(b *Bundle) ([]Host, []Node, error) {
	hosts, err := s.store.ListHosts()
	if err != nil {
		return nil, nil, err
	}
	all, err := s.store.ListNodes("")
	if err != nil {
		return nil, nil, err
	}
	want := map[string]bool{}
	for _, m := range b.Members {
		want[m.HostID+"|"+m.Key] = true
	}
	var picked []Node
	for _, n := range all {
		if want[n.HostID+"|"+n.Key] {
			picked = append(picked, n)
		}
	}
	return hosts, picked, nil
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.Bundle(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "未找到")
		return
	}
	hosts, nodes, err := s.bundleNodes(b)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	target := r.URL.Query().Get("target")
	if target == "text" || target == "base64" {
		links := BuildLinks(hosts, nodes)
		payload := strings.Join(links, "\n")
		if target == "base64" {
			payload = base64Std(payload)
		}
		writeJSON(w, 200, map[string]any{"body": payload, "nodes": len(links)})
		return
	}
	body, err := BuildMihomoYAML(*b, hosts, nodes)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"body": string(body), "nodes": len(nodes)})
}

// ---------- subscriptions api ----------

func (s *Server) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BundleID string `json:"bundleId"`
		Label    string `json:"label"`
		Format   string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	if _, err := s.store.Bundle(body.BundleID); err != nil {
		writeErr(w, 404, "编排不存在")
		return
	}
	if body.Format == "" {
		body.Format = "mihomo"
	}
	sub, err := s.store.CreateSubscription(body.BundleID, body.Label, body.Format)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"subscription": sub, "url": publicBase(r) + "/sub/" + sub.Token})
}

func (s *Server) handlePatchSubscription(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Label   *string `json:"label"`
		Enabled *bool   `json:"enabled"`
		Format  *string `json:"format"`
		Rotate  bool    `json:"rotate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	if body.Format != nil {
		switch *body.Format {
		case "mihomo", "base64", "text":
		default:
			writeErr(w, 400, "未知格式")
			return
		}
	}
	if err := s.store.UpdateSubscription(id, body.Label, body.Enabled, body.Format); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := map[string]any{"ok": true}
	if body.Rotate {
		t, err := s.store.RotateSubscriptionToken(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out["url"] = publicBase(r) + "/sub/" + t
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteSubscription(r.PathValue("id")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	subs, err := s.store.ListSubscriptions()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	token := ""
	for _, sub := range subs {
		if sub.ID == r.PathValue("id") {
			if full, err := s.store.SubscriptionByID(sub.ID); err == nil {
				token = full.Token
			}
		}
	}
	if token == "" {
		writeErr(w, 404, "未找到")
		return
	}
	size := 320
	if v, err := strconv.Atoi(r.URL.Query().Get("size")); err == nil && v >= 120 && v <= 1024 {
		size = v
	}
	png, err := qrcode.Encode(publicBase(r)+"/sub/"+token, qrcode.Medium, size)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
}

// ---------- settings ----------

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DefaultTemplate *string `json:"defaultTemplate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	if body.DefaultTemplate != nil {
		if err := s.store.SetSetting("default_template", *body.DefaultTemplate); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- public subscription ----------

func (s *Server) handleSubscription(w http.ResponseWriter, r *http.Request) {
	sub, err := s.store.SubscriptionByToken(r.PathValue("token"))
	if err != nil || !sub.Enabled {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}
	b, err := s.store.Bundle(sub.BundleID)
	if err != nil {
		http.Error(w, "bundle missing", http.StatusNotFound)
		return
	}
	hosts, nodes, err := s.bundleNodes(b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	target := r.URL.Query().Get("target")
	if target == "" {
		target = sub.Format
	}
	ua := r.UserAgent()
	device := DetectDevice(ua)
	// ?tag= lets two devices of the same model tell themselves apart.
	if tag := strings.TrimSpace(r.URL.Query().Get("tag")); tag != "" {
		device = sanitizeTag(tag)
	}
	s.store.RecordSubscriptionHit(sub.ID, device, ua,
		ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For")))
	w.Header().Set("Profile-Update-Interval", "12")
	w.Header().Set("Subscription-Userinfo", fmt.Sprintf("upload=0; download=0; total=0; expire=0"))
	switch target {
	case "base64":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, base64Std(strings.Join(BuildLinks(hosts, nodes), "\n")))
	case "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, strings.Join(BuildLinks(hosts, nodes), "\n"))
	default:
		body, err := BuildMihomoYAML(*b, hosts, nodes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Header().Set("Content-Disposition", `inline; filename="weft.yaml"`)
		w.Write(body)
	}
}

// ---------- static ----------

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	dist, err := fs.Sub(webassets.Dist, "dist")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path != "" {
		if f, err := dist.Open(path); err == nil {
			f.Close()
			http.FileServer(http.FS(dist)).ServeHTTP(w, r)
			return
		}
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.Error(w, "panel not built", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(index)
}

// ---------- helpers ----------

func publicBase(r *http.Request) string {
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	return scheme + "://" + host
}

// sanitizeTag keeps a device tag short and printable.
func sanitizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if len([]rune(tag)) > 32 {
		tag = string([]rune(tag)[:32])
	}
	tag = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, tag)
	if tag == "" {
		return "未标注设备"
	}
	return tag
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}
