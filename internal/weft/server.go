package weft

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/crypto/argon2"

	webassets "github.com/abcdlsj/weft/web"
)

const sessionCookie = "weft_session"

type Server struct {
	store    *Store
	mux      *http.ServeMux
	demo     bool
	loginLim *rateLimiter
	subLim   *rateLimiter
}

func NewServer(st *Store, demo bool) *Server {
	s := &Server{
		store:    st,
		mux:      http.NewServeMux(),
		demo:     demo,
		loginLim: newRateLimiter(10, 5*time.Minute),
		subLim:   newRateLimiter(120, time.Minute),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return securityHeaders(s.mux) }

func (s *Server) routes() {
	// public
	s.mux.HandleFunc("GET /api/me", s.handleMe)
	s.mux.HandleFunc("GET /api/version", s.handleVersion)
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
	s.mux.HandleFunc("POST /api/hosts/{id}/refresh", s.authed(s.handleRefreshHost))
	s.mux.HandleFunc("POST /api/hosts/{id}/nodes", s.authed(s.handleCreateNode))
	s.mux.HandleFunc("PUT /api/hosts/{id}/nodes/{key}", s.authed(s.handleUpdateNode))
	s.mux.HandleFunc("DELETE /api/hosts/{id}/nodes/{key}", s.authed(s.handleDeleteNode))
	s.mux.HandleFunc("POST /api/nodes/parse", s.authed(s.handleParseNode))
	s.mux.HandleFunc("POST /api/bundles", s.authed(s.handleCreateBundle))
	s.mux.HandleFunc("PUT /api/bundles/{id}", s.authed(s.handleUpdateBundle))
	s.mux.HandleFunc("DELETE /api/bundles/{id}", s.authed(s.handleDeleteBundle))
	s.mux.HandleFunc("GET /api/bundles/{id}/preview", s.authed(s.handlePreview))
	s.mux.HandleFunc("POST /api/bundles/preview", s.authed(s.handlePreviewDraft))
	s.mux.HandleFunc("POST /api/rulesets", s.authed(s.handleCreateRuleSet))
	s.mux.HandleFunc("POST /api/rulesets/import", s.authed(s.handleImportRuleSet))
	s.mux.HandleFunc("GET /api/rulesets/{id}", s.authed(s.handleRuleSet))
	s.mux.HandleFunc("POST /api/rulesets/{id}/refresh", s.authed(s.handleRefreshRuleSet))
	s.mux.HandleFunc("PUT /api/rulesets/{id}", s.authed(s.handleUpdateRuleSet))
	s.mux.HandleFunc("DELETE /api/rulesets/{id}", s.authed(s.handleDeleteRuleSet))
	s.mux.HandleFunc("POST /api/subscriptions", s.authed(s.handleCreateSubscription))
	s.mux.HandleFunc("PATCH /api/subscriptions/{id}", s.authed(s.handlePatchSubscription))
	s.mux.HandleFunc("DELETE /api/subscriptions/{id}", s.authed(s.handleDeleteSubscription))
	s.mux.HandleFunc("GET /api/subscriptions/{id}/qr.png", s.authed(s.handleQR))
	s.mux.HandleFunc("POST /api/password", s.authed(s.handlePassword))
	s.mux.HandleFunc("PUT /api/settings", s.authed(s.handleSettings))
	s.mux.HandleFunc("GET /api/backup", s.authed(s.handleBackup))
	s.mux.HandleFunc("POST /api/backup", s.authed(s.handleRestore))

	// panel
	s.mux.HandleFunc("GET /", s.handleStatic)
}

// ---------- auth ----------

func (s *Server) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := s.session(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "未登录")
			return
		}
		if !safeMethod(r.Method) {
			if r.ContentLength != 0 && !isJSONRequest(r) {
				writeErr(w, http.StatusUnsupportedMediaType, "请求必须是 application/json")
				return
			}
			got := r.Header.Get("X-CSRF-Token")
			if subtle.ConstantTimeCompare([]byte(got), []byte(sess.CSRF)) != 1 {
				writeErr(w, http.StatusForbidden, "CSRF token 无效")
				return
			}
		}
		next(w, r)
	}
}

func safeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

func isJSONRequest(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}

func (s *Server) session(r *http.Request) (*Session, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil, false
	}
	sess, err := s.store.Session(c.Value)
	if err != nil || sess.ExpiresAt < now() {
		if sess != nil {
			s.store.DeleteSession(sess.Token)
		}
		return nil, false
	}
	return sess, true
}

func (s *Server) authenticated(r *http.Request) bool {
	_, ok := s.session(r)
	return ok
}

// PasswordSet reports whether the panel password has been created.
func (s *Server) PasswordSet() bool { return s.passwordSet() }

func (s *Server) passwordSet() bool {
	_, err := s.store.GetSetting("password_hash")
	return err == nil
}

// Password hashes are Argon2id PHC strings. The old PBKDF2 format is still
// verified and transparently upgraded on the next successful login.
const (
	argonMemory  = 32 * 1024 // KiB
	argonTime    = 3
	argonThreads = 2
	argonKeyLen  = 32
)

func newPasswordHash(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyPasswordHash(encoded, password string) bool {
	if !strings.HasPrefix(encoded, "$argon2id$") {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(want, got) == 1
}

func legacyPasswordHash(password, salt string) (string, error) {
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
	h, err := newPasswordHash(password)
	if err != nil {
		return err
	}
	if err := s.store.SetSetting("password_salt", ""); err != nil {
		return err
	}
	return s.store.SetSetting("password_hash", h)
}

func (s *Server) checkPassword(password string) bool {
	want, err := s.store.GetSetting("password_hash")
	if err != nil {
		return false
	}
	if strings.HasPrefix(want, "$argon2id$") {
		return verifyPasswordHash(want, password)
	}
	// Legacy PBKDF2: verify, then replace it with Argon2id in place.
	salt, err := s.store.GetSetting("password_salt")
	if err != nil {
		return false
	}
	got, err := legacyPasswordHash(password, salt)
	if err != nil || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		return false
	}
	if err := s.setPassword(password); err != nil {
		log.Printf("rehashing password: %v", err)
	}
	return true
}

func (s *Server) newSession(w http.ResponseWriter, r *http.Request) string {
	tok := newToken()
	csrf := newToken()
	expires := now() + 30*86400
	if err := s.store.CreateSession(tok, csrf, expires); err != nil {
		log.Printf("creating session: %v", err)
	}
	s.store.CleanupSessions(now())
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
		MaxAge:   30 * 86400,
	})
	return csrf
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"version": AgentVersion})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(r)
	out := map[string]any{
		"authenticated": ok,
		"needsSetup":    !s.passwordSet(),
		"demo":          s.demo,
	}
	if ok {
		out["csrf"] = sess.CSRF
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s.passwordSet() {
		writeErr(w, http.StatusConflict, "已完成初始化")
		return
	}
	if !s.loginLim.allow(ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))) {
		rateLimited(w, time.Minute)
		return
	}
	if !isJSONRequest(r) {
		writeErr(w, http.StatusUnsupportedMediaType, "请求必须是 application/json")
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
	csrf := s.newSession(w, r)
	writeJSON(w, 200, map[string]any{"ok": true, "csrf": csrf})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
	if !s.loginLim.allow(ip) {
		rateLimited(w, 5*time.Minute)
		return
	}
	if !isJSONRequest(r) {
		writeErr(w, http.StatusUnsupportedMediaType, "请求必须是 application/json")
		return
	}
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
	csrf := s.newSession(w, r)
	writeJSON(w, 200, map[string]any{"ok": true, "csrf": csrf})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.store.DeleteSession(c.Value)
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
	// keep the device that changed the password signed in, retire the rest
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.store.DeleteOtherSessions(c.Value)
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

// An agent speaks only when its inventory changes, plus an hourly heartbeat,
// so a host stays online through one missed beat and is written off after six.
func hostStatus(h Host) string {
	switch h.Kind {
	case HostStatic:
		return "manual"
	case HostSource:
		if h.SubCheckedAt == 0 {
			return "never"
		}
		if h.SubError != "" {
			return "offline"
		}
		return "online"
	}
	switch {
	case h.LastSeen == 0:
		return "never"
	case now()-h.LastSeen < 5400:
		return "online"
	case now()-h.LastSeen < 21600:
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
	rules, err := s.ruleSetViews()
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
		"ruleSets":        rules,
		"rulePresets":     RulePresets(),
		"gfwListUrl":      GFWListURL,
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
		URL  string `json:"url"`
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
	if NormalizeHostKind(body.Kind) == HostSource {
		rawURL := strings.TrimSpace(body.URL)
		if rawURL == "" {
			writeErr(w, 400, "订阅地址不能为空")
			return
		}
		h, err := s.store.CreateSourceHost(name, rawURL)
		if err != nil {
			writeErr(w, 409, "机器名已存在："+name)
			return
		}
		// Pull once now so the panel never shows an empty source the admin has
		// to guess about; the error still lands on the host card.
		count, syncErr := SyncSource(s.store, h)
		if fresh, err := s.store.HostByID(h.ID); err == nil {
			h = fresh
		}
		writeJSON(w, 200, map[string]any{"host": h, "nodes": count, "error": errText(syncErr)})
		return
	}
	h, err := s.store.CreateHost(name, body.Kind)
	if err != nil {
		writeErr(w, 409, "机器名已存在："+name)
		return
	}
	writeJSON(w, 200, h)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// handleRefreshHost re-pulls a subscription source on demand.
func (s *Server) handleRefreshHost(w http.ResponseWriter, r *http.Request) {
	h, err := s.store.HostByID(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "机器不存在")
		return
	}
	if h.Kind != HostSource {
		writeErr(w, 400, "只有订阅源可以刷新")
		return
	}
	count, syncErr := SyncSource(s.store, h)
	if fresh, err := s.store.HostByID(h.ID); err == nil {
		h = fresh
	}
	status := 200
	if syncErr != nil {
		status = 502
	}
	writeJSON(w, status, map[string]any{"host": h, "nodes": count, "error": errText(syncErr)})
}

func (s *Server) handlePatchHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Name    *string `json:"name"`
		URL     *string `json:"url"`
		Refresh bool    `json:"refresh"`
		Rotate  bool    `json:"rotate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	if body.URL != nil {
		rawURL := strings.TrimSpace(*body.URL)
		if rawURL == "" {
			writeErr(w, 400, "订阅地址不能为空")
			return
		}
		if err := s.store.SetSourceURL(id, rawURL); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
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
	if body.Refresh {
		h, err := s.store.HostByID(id)
		if err != nil {
			writeErr(w, 404, "机器不存在")
			return
		}
		if h.Kind == HostSource {
			count, syncErr := SyncSource(s.store, h)
			out["nodes"] = count
			out["error"] = errText(syncErr)
			if fresh, err := s.store.HostByID(id); err == nil {
				out["host"] = fresh
			}
		}
	}
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
	RuleSetID *string  `json:"ruleSetId"`
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
	ruleSetID := ""
	if body.RuleSetID != nil {
		ruleSetID = *body.RuleSetID
	}
	if ruleSetID != "" {
		if _, err := s.store.RuleSet(ruleSetID); err != nil {
			writeErr(w, 400, "规则集不存在")
			return
		}
	}
	b := Bundle{Name: body.Name, Slug: body.Slug, GroupBy: body.GroupBy, GroupType: body.GroupType,
		Template: body.Template, RuleSetID: ruleSetID}
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
	if body.RuleSetID != nil {
		if *body.RuleSetID != "" {
			if _, err := s.store.RuleSet(*body.RuleSetID); err != nil {
				writeErr(w, 400, "规则集不存在")
				return
			}
		}
		b.RuleSetID = *body.RuleSetID
	}
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
	case "host", "type", "flat", "region":
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
	return s.nodesForMembers(b.Members)
}

func (s *Server) nodesForMembers(members []Member) ([]Host, []Node, error) {
	hosts, err := s.store.ListHosts()
	if err != nil {
		return nil, nil, err
	}
	all, err := s.store.ListNodes("")
	if err != nil {
		return nil, nil, err
	}
	want := map[string]bool{}
	for _, m := range members {
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
	s.writePreview(w, r, b, hosts, nodes)
}

// handlePreviewDraft renders a bundle that has not been saved. The editor uses
// it so node picks, groups, templates and rule sets can be inspected before a
// write.
func (s *Server) handlePreviewDraft(w http.ResponseWriter, r *http.Request) {
	var body bundlePayload
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	b := Bundle{Name: body.Name, Slug: body.Slug, GroupBy: body.GroupBy, GroupType: body.GroupType,
		Template: body.Template, Members: body.Members}
	if body.RuleSetID != nil {
		b.RuleSetID = *body.RuleSetID
	}
	normalizeBundle(&b)
	hosts, nodes, err := s.nodesForMembers(b.Members)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.writePreview(w, r, &b, hosts, nodes)
}

func (s *Server) writePreview(w http.ResponseWriter, r *http.Request, b *Bundle, hosts []Host, nodes []Node) {
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
	rs, err := s.ruleSetFor(b)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	out, err := BuildMihomoYAML(*b, hosts, nodes, rs)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"body": string(out), "nodes": len(nodes)})
}

// ---------- subscriptions api ----------

// subURL is the one place a subscription link is built, so the key can never
// be forgotten in one of the four places that show it.
func (s *Server) subURL(r *http.Request, sub *Subscription) string {
	u := publicBase(r) + "/sub/" + sub.Token
	if sub.AccessKey != "" {
		u += "?key=" + url.QueryEscape(sub.AccessKey)
	}
	return u
}

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
	writeJSON(w, 200, map[string]any{"subscription": sub, "url": s.subURL(r, sub)})
}

func (s *Server) handlePatchSubscription(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Label   *string `json:"label"`
		Enabled *bool   `json:"enabled"`
		Format  *string `json:"format"`
		Bearer  *string `json:"bearer"`
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
	if body.Bearer != nil {
		v := strings.TrimSpace(*body.Bearer)
		if v != "" && len(v) < 8 {
			writeErr(w, 400, "bearer token 至少 8 位")
			return
		}
		body.Bearer = &v
	}
	if err := s.store.UpdateSubscription(id, body.Label, body.Enabled, body.Format, body.Bearer); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := map[string]any{"ok": true}
	if body.Rotate {
		t, key, err := s.store.RotateSubscription(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out["url"] = s.subURL(r, &Subscription{Token: t, AccessKey: key})
		if full, err := s.store.SubscriptionByID(id); err == nil {
			out["bearer"] = full.Bearer
		}
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
	token, accessKey := "", ""
	for _, sub := range subs {
		if sub.ID == r.PathValue("id") {
			if full, err := s.store.SubscriptionByID(sub.ID); err == nil {
				token, accessKey = full.Token, full.AccessKey
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
	link := s.subURL(r, &Subscription{Token: token, AccessKey: accessKey})
	png, err := qrcode.Encode(link, qrcode.Medium, size)
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
	ip := ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
	if !s.subLim.allow(ip) {
		rateLimited(w, time.Minute)
		return
	}
	sub, err := s.store.SubscriptionByToken(r.PathValue("token"))
	if err != nil || !sub.Enabled {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}
	// Two proofs are accepted: the query key that travels with the URL, and a
	// bearer token a client can store in its own header. Either one is enough,
	// so a client that cannot add headers keeps working unchanged.
	keyOK := sub.AccessKey != "" &&
		subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("key")), []byte(sub.AccessKey)) == 1
	bearer := requestBearer(r)
	bearerOK := sub.Bearer != "" && bearer != "" &&
		subtle.ConstantTimeCompare([]byte(bearer), []byte(sub.Bearer)) == 1
	if (sub.AccessKey != "" || sub.Bearer != "") && !keyOK && !bearerOK {
		http.Error(w, "subscription key required", http.StatusForbidden)
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
	s.store.RecordSubscriptionHit(sub.ID, device, ua, ip)
	title := strings.TrimSpace(sub.Label)
	if title == "" {
		title = b.Name
	}
	if title == "" {
		title = "weft"
	}
	w.Header().Set("Profile-Update-Interval", "12")
	w.Header().Set("Profile-Web-Page-Url", publicBase(r)+"/subscriptions")
	// weft does not meter traffic. A large total keeps clients from showing a
	// meaningless 0 B / 0 B, and expire=0 means the profile never expires.
	w.Header().Set("Subscription-Userinfo", fmt.Sprintf("upload=0; download=0; total=%d; expire=0", subscriptionTrafficTotal))
	extension := ".yaml"
	switch target {
	case "base64", "text":
		extension = ".txt"
	default:
		extension = ".yaml"
	}
	w.Header().Set("Content-Disposition", contentDisposition(title+extension))
	switch target {
	case "base64":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, base64Std(strings.Join(BuildLinks(hosts, nodes), "\n")))
	case "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, strings.Join(BuildLinks(hosts, nodes), "\n"))
	default:
		rs, err := s.ruleSetFor(b)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		body, err := BuildMihomoYAML(*b, hosts, nodes, rs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
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

// ---------- rule sets ----------

const subscriptionTrafficTotal = int64(100 * 1024 * 1024 * 1024 * 1024) // 100 TiB

// ruleSetView is the list projection. Content can be a GFW list with thousands
// of lines, so it is fetched only when someone opens the editor.
type ruleSetView struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Source        string `json:"source"`
	UpdatedAt     int64  `json:"updatedAt"`
	CreatedAt     int64  `json:"createdAt"`
	RuleCount     int    `json:"ruleCount"`
	ProviderCount int    `json:"providerCount"`
	WarningCount  int    `json:"warningCount"`
	UsedBy        int    `json:"usedBy"`
}

func (s *Server) ruleSetViews() ([]ruleSetView, error) {
	sets, err := s.store.ListRuleSets()
	if err != nil {
		return nil, err
	}
	bundles, err := s.store.ListBundles()
	if err != nil {
		return nil, err
	}
	used := map[string]int{}
	for _, b := range bundles {
		if b.RuleSetID != "" {
			used[b.RuleSetID]++
		}
	}
	out := make([]ruleSetView, 0, len(sets))
	for _, rs := range sets {
		rules, providers := CountRules(rs.Content)
		out = append(out, ruleSetView{
			ID: rs.ID, Name: rs.Name, Description: rs.Description, Source: rs.Source,
			UpdatedAt: rs.UpdatedAt, CreatedAt: rs.CreatedAt,
			RuleCount: rules, ProviderCount: providers, WarningCount: len(RuleWarnings(rs.Content)),
			UsedBy: used[rs.ID],
		})
	}
	return out, nil
}

func (s *Server) decorateRuleSet(rs *RuleSet) {
	rs.RuleCount, rs.ProviderCount = CountRules(rs.Content)
	rs.Warnings = RuleWarnings(rs.Content)
	bundles, _ := s.store.ListBundles()
	for _, b := range bundles {
		if b.RuleSetID == rs.ID {
			rs.UsedBy++
		}
	}
}

func (s *Server) ruleSetFor(b *Bundle) (*RuleSet, error) {
	if b == nil || b.RuleSetID == "" {
		return nil, nil
	}
	rs, err := s.store.RuleSet(b.RuleSetID)
	if err != nil {
		return nil, fmt.Errorf("规则集缺失：%w", err)
	}
	return rs, nil
}

func (s *Server) handleCreateRuleSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Source      string `json:"source"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, 400, "名称不能为空")
		return
	}
	if err := ValidateRuleSetContent(body.Content); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	rs, err := s.store.CreateRuleSet(RuleSet{Name: name, Description: strings.TrimSpace(body.Description),
		Content: body.Content, Source: strings.TrimSpace(body.Source)})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.decorateRuleSet(rs)
	writeJSON(w, 200, rs)
}

func (s *Server) handleRuleSet(w http.ResponseWriter, r *http.Request) {
	rs, err := s.store.RuleSet(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "规则集不存在")
		return
	}
	s.decorateRuleSet(rs)
	writeJSON(w, 200, rs)
}

func (s *Server) handleRefreshRuleSet(w http.ResponseWriter, r *http.Request) {
	rs, err := s.store.RuleSet(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "规则集不存在")
		return
	}
	if !strings.HasPrefix(rs.Source, "http://") && !strings.HasPrefix(rs.Source, "https://") {
		writeErr(w, 400, "这套规则集没有可刷新的远程来源")
		return
	}
	data, err := FetchRuleSource(rs.Source)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	content, err := ImportRuleSetContent(data)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := ValidateRuleSetContent(content); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"content":  content,
		"warnings": RuleWarnings(content),
		"source":   rs.Source,
	})
}

func (s *Server) handleUpdateRuleSet(w http.ResponseWriter, r *http.Request) {
	rs, err := s.store.RuleSet(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "规则集不存在")
		return
	}
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Source      string `json:"source"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	if strings.TrimSpace(body.Name) != "" {
		rs.Name = strings.TrimSpace(body.Name)
	}
	rs.Description = strings.TrimSpace(body.Description)
	rs.Source = strings.TrimSpace(body.Source)
	rs.Content = body.Content
	if err := ValidateRuleSetContent(rs.Content); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.store.UpdateRuleSet(*rs); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	updated, _ := s.store.RuleSet(rs.ID)
	s.decorateRuleSet(updated)
	writeJSON(w, 200, updated)
}

func (s *Server) handleDeleteRuleSet(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteRuleSet(r.PathValue("id")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleImportRuleSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		URL         string `json:"url"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, 400, "请求无效")
		return
	}
	name := strings.TrimSpace(body.Name)
	source := strings.TrimSpace(body.URL)
	if source == "" {
		source = GFWListURL
	}
	if name == "" {
		name = "导入的规则"
	}
	data, err := FetchRuleSource(source)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	content, err := ImportRuleSetContent(data)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	rs, err := s.store.CreateRuleSet(RuleSet{Name: name, Description: strings.TrimSpace(body.Description),
		Content: content, Source: source})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.decorateRuleSet(rs)
	writeJSON(w, 200, rs)
}

// contentDisposition names the downloaded profile for Clash-family clients and
// Shadowrocket. The ASCII fallback keeps old clients happy; filename* carries
// the real, possibly Chinese, name.
func contentDisposition(filename string) string {
	ext := ".yaml"
	if i := strings.LastIndex(filename, "."); i > 0 {
		ext = filename[i:]
	}
	stem := strings.TrimSuffix(filename, ext)
	ascii := strings.Map(func(r rune) rune {
		switch {
		case r < 32 || r > 126:
			return '-'
		case r == '"' || r == '\\':
			return '-'
		}
		return r
	}, stem)
	ascii = strings.TrimSpace(strings.Trim(ascii, "-_ "))
	if strings.Trim(ascii, "-_. ") == "" {
		ascii = "weft"
	}
	return fmt.Sprintf(`attachment; filename="%s%s"; filename*=UTF-8''%s`, ascii, ext, url.PathEscape(filename))
}

// requestBearer reads an RFC 6750 Authorization header without caring about
// the case of the scheme name.
func requestBearer(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(h) <= 7 || !strings.EqualFold(h[:7], "bearer ") {
		return ""
	}
	return strings.TrimSpace(h[7:])
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
