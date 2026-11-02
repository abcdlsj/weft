package weft

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

// Store is the single-writer persistence layer. Nodes arrive as full snapshots
// per host, so writes are replace-shaped rather than incremental.
type Store struct {
	db *sql.DB
}

var ErrNotFound = errors.New("not found")

func OpenStore(path string) (*Store, error) {
	dsn := path
	if path == ":memory:" {
		dsn = "file:weft?mode=memory&cache=shared"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS hosts (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  token TEXT NOT NULL UNIQUE,
  controller TEXT NOT NULL DEFAULT '',
  agent_version TEXT NOT NULL DEFAULT '',
  mihomo_version TEXT NOT NULL DEFAULT '',
  last_seen INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS nodes (
  host_id TEXT NOT NULL,
  key TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  server TEXT NOT NULL,
  port INTEGER NOT NULL,
  alive INTEGER NOT NULL DEFAULT 0,
  delay_ms INTEGER NOT NULL DEFAULT 0,
  raw TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (host_id, key)
);
CREATE TABLE IF NOT EXISTS bundles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  group_by TEXT NOT NULL DEFAULT 'host',
  group_type TEXT NOT NULL DEFAULT 'url-test',
  template TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS bundle_members (
  bundle_id TEXT NOT NULL,
  host_id TEXT NOT NULL,
  node_key TEXT NOT NULL,
  PRIMARY KEY (bundle_id, host_id, node_key)
);
CREATE TABLE IF NOT EXISTS subscriptions (
  id TEXT PRIMARY KEY,
  bundle_id TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  token TEXT NOT NULL UNIQUE,
  format TEXT NOT NULL DEFAULT 'mihomo',
  enabled INTEGER NOT NULL DEFAULT 1,
  hits INTEGER NOT NULL DEFAULT 0,
  last_used_at INTEGER NOT NULL DEFAULT 0,
  last_ua TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS reports (
  host_id TEXT NOT NULL,
  at INTEGER NOT NULL,
  node_count INTEGER NOT NULL,
  alive_count INTEGER NOT NULL,
  avg_delay INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (host_id, at)
);
CREATE TABLE IF NOT EXISTS sub_access (
  subscription_id TEXT NOT NULL,
  at INTEGER NOT NULL,
  device TEXT NOT NULL,
  ua TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sub_access ON sub_access (subscription_id, at DESC);
CREATE TABLE IF NOT EXISTS settings (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL
);
`)
	// older databases predate the latency column; adding it is idempotent enough
	s.db.Exec(`ALTER TABLE reports ADD COLUMN avg_delay INTEGER NOT NULL DEFAULT 0`)
	return err
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func newToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func now() int64 { return time.Now().Unix() }

// ---------- hosts ----------

type Host struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Token         string `json:"token,omitempty"`
	Controller    string `json:"controller"`
	AgentVersion  string `json:"agentVersion"`
	MihomoVersion string `json:"mihomoVersion"`
	LastSeen      int64  `json:"lastSeen"`
	CreatedAt     int64  `json:"createdAt"`
	NodeCount     int    `json:"nodeCount"`
	AliveCount    int    `json:"aliveCount"`
}

func (s *Store) CreateHost(name string) (*Host, error) {
	h := &Host{ID: newID(), Name: name, Token: newToken(), CreatedAt: now()}
	_, err := s.db.Exec(`INSERT INTO hosts (id,name,token,created_at) VALUES (?,?,?,?)`,
		h.ID, h.Name, h.Token, h.CreatedAt)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func (s *Store) ListHosts() ([]Host, error) {
	rows, err := s.db.Query(`
SELECT h.id, h.name, h.controller, h.agent_version, h.mihomo_version, h.last_seen, h.created_at,
       COALESCE(n.total,0), COALESCE(n.alive,0)
FROM hosts h
LEFT JOIN (SELECT host_id, COUNT(*) total, SUM(alive) alive FROM nodes GROUP BY host_id) n ON n.host_id = h.id
ORDER BY h.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Host
	for rows.Next() {
		var h Host
		if err := rows.Scan(&h.ID, &h.Name, &h.Controller, &h.AgentVersion, &h.MihomoVersion,
			&h.LastSeen, &h.CreatedAt, &h.NodeCount, &h.AliveCount); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) HostByID(id string) (*Host, error) {
	var h Host
	err := s.db.QueryRow(`SELECT id,name,token,controller,agent_version,mihomo_version,last_seen,created_at
FROM hosts WHERE id=?`, id).Scan(&h.ID, &h.Name, &h.Token, &h.Controller, &h.AgentVersion,
		&h.MihomoVersion, &h.LastSeen, &h.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &h, err
}

func (s *Store) HostByToken(token string) (*Host, error) {
	var h Host
	err := s.db.QueryRow(`SELECT id,name,token,controller,agent_version,mihomo_version,last_seen,created_at
FROM hosts WHERE token=?`, token).Scan(&h.ID, &h.Name, &h.Token, &h.Controller, &h.AgentVersion,
		&h.MihomoVersion, &h.LastSeen, &h.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &h, err
}

func (s *Store) RotateHostToken(id string) (string, error) {
	t := newToken()
	_, err := s.db.Exec(`UPDATE hosts SET token=? WHERE id=?`, t, id)
	return t, err
}

func (s *Store) RenameHost(id, name string) error {
	_, err := s.db.Exec(`UPDATE hosts SET name=? WHERE id=?`, name, id)
	return err
}

func (s *Store) DeleteHost(id string) error {
	if _, err := s.db.Exec(`DELETE FROM nodes WHERE host_id=?`, id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM bundle_members WHERE host_id=?`, id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM reports WHERE host_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM hosts WHERE id=?`, id)
	return err
}

// ---------- nodes ----------

type Node struct {
	HostID    string          `json:"hostId"`
	HostName  string          `json:"hostName"`
	Key       string          `json:"key"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Server    string          `json:"server"`
	Port      int             `json:"port"`
	Alive     bool            `json:"alive"`
	DelayMS   int             `json:"delayMs"`
	Raw       json.RawMessage `json:"raw,omitempty"`
	UpdatedAt int64           `json:"updatedAt"`
}

func (s *Store) ReplaceNodes(hostID string, nodes []Node) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM nodes WHERE host_id=?`, hostID); err != nil {
		return err
	}
	ts := now()
	alive, delaySum := 0, 0
	for _, n := range nodes {
		raw := n.Raw
		if len(raw) == 0 {
			raw = json.RawMessage(`{}`)
		}
		if _, err := tx.Exec(`INSERT INTO nodes (host_id,key,name,type,server,port,alive,delay_ms,raw,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)`, hostID, n.Key, n.Name, n.Type, n.Server, n.Port,
			boolInt(n.Alive), n.DelayMS, string(raw), ts); err != nil {
			return err
		}
		if n.Alive {
			alive++
			delaySum += n.DelayMS
		}
	}
	avgDelay := 0
	if alive > 0 {
		avgDelay = delaySum / alive
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO reports (host_id,at,node_count,alive_count,avg_delay) VALUES (?,?,?,?,?)`,
		hostID, ts, len(nodes), alive, avgDelay); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM reports WHERE host_id=? AND at < ?`, hostID, ts-86400); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListNodes(hostID string) ([]Node, error) {
	q := `SELECT n.host_id, h.name, n.key, n.name, n.type, n.server, n.port, n.alive, n.delay_ms, n.raw, n.updated_at
FROM nodes n JOIN hosts h ON h.id = n.host_id`
	args := []any{}
	if hostID != "" {
		q += ` WHERE n.host_id=?`
		args = append(args, hostID)
	}
	q += ` ORDER BY h.name, n.delay_ms, n.name`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		var n Node
		var alive int
		var raw string
		if err := rows.Scan(&n.HostID, &n.HostName, &n.Key, &n.Name, &n.Type, &n.Server,
			&n.Port, &alive, &n.DelayMS, &raw, &n.UpdatedAt); err != nil {
			return nil, err
		}
		n.Alive = alive == 1
		n.Raw = json.RawMessage(raw)
		out = append(out, n)
	}
	return out, rows.Err()
}

// ---------- bundles ----------

type Bundle struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Slug      string   `json:"slug"`
	GroupBy   string   `json:"groupBy"`   // host | type | flat
	GroupType string   `json:"groupType"` // url-test | select | fallback
	Template  string   `json:"template"`
	CreatedAt int64    `json:"createdAt"`
	Members   []Member `json:"members,omitempty"`
}

type Member struct {
	HostID string `json:"hostId"`
	Key    string `json:"key"`
}

func (s *Store) CreateBundle(b Bundle) (*Bundle, error) {
	b.ID = newID()
	if b.Slug == "" {
		b.Slug = b.Name
	}
	b.Slug = slugify(b.Slug)
	if b.Slug == "" {
		b.Slug = b.ID
	}
	b.CreatedAt = now()
	_, err := s.db.Exec(`INSERT INTO bundles (id,name,slug,group_by,group_type,template,created_at)
VALUES (?,?,?,?,?,?,?)`, b.ID, b.Name, b.Slug, b.GroupBy, b.GroupType, b.Template, b.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) ListBundles() ([]Bundle, error) {
	rows, err := s.db.Query(`SELECT id,name,slug,group_by,group_type,template,created_at FROM bundles ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bundle
	for rows.Next() {
		var b Bundle
		if err := rows.Scan(&b.ID, &b.Name, &b.Slug, &b.GroupBy, &b.GroupType, &b.Template, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	for i := range out {
		m, err := s.Members(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Members = m
	}
	return out, rows.Err()
}

func (s *Store) Bundle(id string) (*Bundle, error) {
	var b Bundle
	err := s.db.QueryRow(`SELECT id,name,slug,group_by,group_type,template,created_at FROM bundles WHERE id=?`, id).
		Scan(&b.ID, &b.Name, &b.Slug, &b.GroupBy, &b.GroupType, &b.Template, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	b.Members, err = s.Members(id)
	return &b, err
}

func (s *Store) UpdateBundle(b Bundle) error {
	_, err := s.db.Exec(`UPDATE bundles SET name=?,slug=?,group_by=?,group_type=?,template=? WHERE id=?`,
		b.Name, slugify(b.Slug), b.GroupBy, b.GroupType, b.Template, b.ID)
	return err
}

func (s *Store) SetMembers(bundleID string, members []Member) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM bundle_members WHERE bundle_id=?`, bundleID); err != nil {
		return err
	}
	for _, m := range members {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO bundle_members (bundle_id,host_id,node_key) VALUES (?,?,?)`,
			bundleID, m.HostID, m.Key); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Members(bundleID string) ([]Member, error) {
	rows, err := s.db.Query(`SELECT host_id,node_key FROM bundle_members WHERE bundle_id=?`, bundleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.HostID, &m.Key); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) DeleteBundle(id string) error {
	if _, err := s.db.Exec(`DELETE FROM bundle_members WHERE bundle_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM bundles WHERE id=?`, id)
	return err
}

// ---------- subscriptions ----------

type AccessStat struct {
	Device string `json:"device"`
	UA     string `json:"ua"`
	IP     string `json:"ip"`
	Hits   int    `json:"hits"`
	LastAt int64  `json:"lastAt"`
}

type Subscription struct {
	ID         string       `json:"id"`
	BundleID   string       `json:"bundleId"`
	BundleName string       `json:"bundleName,omitempty"`
	Label      string       `json:"label"`
	Token      string       `json:"token"`
	Format     string       `json:"format"`
	Enabled    bool         `json:"enabled"`
	Hits       int64        `json:"hits"`
	LastUsedAt int64        `json:"lastUsedAt"`
	LastUA     string       `json:"lastUa"`
	CreatedAt  int64        `json:"createdAt"`
	Devices    []AccessStat `json:"devices"`
}

func (s *Store) CreateSubscription(bundleID, label, format string) (*Subscription, error) {
	sub := &Subscription{ID: newID(), BundleID: bundleID, Label: label, Token: newToken(),
		Format: format, Enabled: true, CreatedAt: now()}
	_, err := s.db.Exec(`INSERT INTO subscriptions (id,bundle_id,label,token,format,enabled,created_at)
VALUES (?,?,?,?,?,?,?)`, sub.ID, sub.BundleID, sub.Label, sub.Token, sub.Format, 1, sub.CreatedAt)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *Store) ListSubscriptions() ([]Subscription, error) {
	rows, err := s.db.Query(`SELECT s.id,s.bundle_id,COALESCE(b.name,''),s.label,s.token,s.format,s.enabled,
s.hits,s.last_used_at,s.last_ua,s.created_at
FROM subscriptions s LEFT JOIN bundles b ON b.id = s.bundle_id ORDER BY s.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		var sub Subscription
		var enabled int
		if err := rows.Scan(&sub.ID, &sub.BundleID, &sub.BundleName, &sub.Label, &sub.Token, &sub.Format,
			&enabled, &sub.Hits, &sub.LastUsedAt, &sub.LastUA, &sub.CreatedAt); err != nil {
			return nil, err
		}
		sub.Enabled = enabled == 1
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Store) SubscriptionByID(id string) (*Subscription, error) {
	var sub Subscription
	var enabled int
	err := s.db.QueryRow(`SELECT id,bundle_id,label,token,format,enabled,hits,last_used_at,last_ua,created_at
FROM subscriptions WHERE id=?`, id).Scan(&sub.ID, &sub.BundleID, &sub.Label, &sub.Token,
		&sub.Format, &enabled, &sub.Hits, &sub.LastUsedAt, &sub.LastUA, &sub.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	sub.Enabled = enabled == 1
	return &sub, err
}

func (s *Store) SubscriptionByToken(token string) (*Subscription, error) {
	var sub Subscription
	var enabled int
	err := s.db.QueryRow(`SELECT id,bundle_id,label,token,format,enabled,hits,last_used_at,last_ua,created_at
FROM subscriptions WHERE token=?`, token).Scan(&sub.ID, &sub.BundleID, &sub.Label, &sub.Token,
		&sub.Format, &enabled, &sub.Hits, &sub.LastUsedAt, &sub.LastUA, &sub.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	sub.Enabled = enabled == 1
	return &sub, err
}

func (s *Store) UpdateSubscription(id string, label *string, enabled *bool, format *string) error {
	if label != nil {
		if _, err := s.db.Exec(`UPDATE subscriptions SET label=? WHERE id=?`, *label, id); err != nil {
			return err
		}
	}
	if enabled != nil {
		if _, err := s.db.Exec(`UPDATE subscriptions SET enabled=? WHERE id=?`, boolInt(*enabled), id); err != nil {
			return err
		}
	}
	if format != nil {
		if _, err := s.db.Exec(`UPDATE subscriptions SET format=? WHERE id=?`, *format, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RotateSubscriptionToken(id string) (string, error) {
	t := newToken()
	_, err := s.db.Exec(`UPDATE subscriptions SET token=? WHERE id=?`, t, id)
	return t, err
}

func (s *Store) DeleteSubscription(id string) error {
	if _, err := s.db.Exec(`DELETE FROM sub_access WHERE subscription_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM subscriptions WHERE id=?`, id)
	return err
}

// RecordSubscriptionHit updates the counters and keeps a short access log so
// the panel can name the devices a link is actually used on.
func (s *Store) RecordSubscriptionHit(id, device, ua, ip string) {
	ts := now()
	s.db.Exec(`UPDATE subscriptions SET hits=hits+1,last_used_at=?,last_ua=? WHERE id=?`, ts, ua, id)
	s.db.Exec(`INSERT INTO sub_access (subscription_id,at,device,ua,ip) VALUES (?,?,?,?,?)`,
		id, ts, device, ua, ip)
	s.db.Exec(`DELETE FROM sub_access WHERE at < ?`, ts-7*86400)
}

// AccessStats folds the access log into one row per device, newest first.
func (s *Store) AccessStats(subID string) ([]AccessStat, error) {
	rows, err := s.db.Query(`
SELECT device, MAX(ua), MAX(ip), COUNT(*), MAX(at)
FROM sub_access WHERE subscription_id=? GROUP BY device ORDER BY MAX(at) DESC`, subID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccessStat
	for rows.Next() {
		var a AccessStat
		if err := rows.Scan(&a.Device, &a.UA, &a.IP, &a.Hits, &a.LastAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---------- settings ----------

func (s *Store) GetSetting(k string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT v FROM settings WHERE k=?`, k).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

func (s *Store) SetSetting(k, v string) error {
	_, err := s.db.Exec(`INSERT INTO settings (k,v) VALUES (?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
	return err
}

// ---------- reports ----------

type Report struct {
	HostID     string `json:"hostId"`
	At         int64  `json:"at"`
	NodeCount  int    `json:"nodeCount"`
	AliveCount int    `json:"aliveCount"`
	AvgDelay   int    `json:"avgDelay"`
}

func (s *Store) RecentReports(since int64) ([]Report, error) {
	rows, err := s.db.Query(`SELECT host_id,at,node_count,alive_count,avg_delay FROM reports WHERE at>=? ORDER BY at`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Report
	for rows.Next() {
		var r Report
		if err := rows.Scan(&r.HostID, &r.At, &r.NodeCount, &r.AliveCount, &r.AvgDelay); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func slugify(s string) string {
	out := make([]rune, 0, len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
			prevDash = false
		case r >= 'A' && r <= 'Z':
			out = append(out, r+32)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if !prevDash && len(out) > 0 {
				out = append(out, '-')
				prevDash = true
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}
