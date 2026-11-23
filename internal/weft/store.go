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
  probe_state TEXT NOT NULL DEFAULT '',
  probed_at INTEGER NOT NULL DEFAULT 0,
  resolved_v4 INTEGER NOT NULL DEFAULT 0,
  resolved_v6 INTEGER NOT NULL DEFAULT 0,
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
  rule_set_id TEXT NOT NULL DEFAULT '',
  exclude_ipv6_only INTEGER NOT NULL DEFAULT 0,
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
  access_key TEXT NOT NULL DEFAULT '',
  bearer TEXT NOT NULL DEFAULT '',
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
CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,
  csrf TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions (expires_at);
CREATE TABLE IF NOT EXISTS rule_sets (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL
);
`)
	if err != nil {
		return err
	}
	// older databases predate these columns; check pragma table_info before adding
	// so every migration step stays strictly idempotent without dropping data.
	migrations := []struct {
		table, column, def string
	}{
		{"reports", "avg_delay", "INTEGER NOT NULL DEFAULT 0"},
		{"hosts", "kind", "TEXT NOT NULL DEFAULT 'agent'"},
		{"hosts", "sub_url", "TEXT NOT NULL DEFAULT ''"},
		{"hosts", "sub_error", "TEXT NOT NULL DEFAULT ''"},
		{"hosts", "sub_checked_at", "INTEGER NOT NULL DEFAULT 0"},
		{"nodes", "source", "TEXT NOT NULL DEFAULT 'agent'"},
		{"nodes", "region", "TEXT NOT NULL DEFAULT ''"},
		{"nodes", "probe_state", "TEXT NOT NULL DEFAULT ''"},
		{"nodes", "probed_at", "INTEGER NOT NULL DEFAULT 0"},
		{"nodes", "resolved_v4", "INTEGER NOT NULL DEFAULT 0"},
		{"nodes", "resolved_v6", "INTEGER NOT NULL DEFAULT 0"},
		{"subscriptions", "access_key", "TEXT NOT NULL DEFAULT ''"},
		{"subscriptions", "bearer", "TEXT NOT NULL DEFAULT ''"},
		{"bundles", "rule_set_id", "TEXT NOT NULL DEFAULT ''"},
		{"bundles", "exclude_ipv6_only", "INTEGER NOT NULL DEFAULT 0"},
	}
	for _, m := range migrations {
		if err := s.ensureColumn(m.table, m.column, m.def); err != nil {
			return err
		}
	}
	// every link gets a key, including ones created before keys existed
	if _, err := s.db.Exec(`UPDATE subscriptions SET access_key = lower(hex(randomblob(8))) WHERE access_key = ''`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE subscriptions SET bearer = lower(hex(randomblob(24))) WHERE bearer = ''`); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureColumn(table, column, def string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + def)
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

// Host is one machine in the fleet. kind separates hosts that report through
// an agent (`agent`) from hosts whose nodes are entered by hand (`static`),
// which is what a bare hysteria2 / xray box needs, from subscription sources
// (`sub`) that weft pulls and flattens on its own schedule.
type Host struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Token                 string `json:"token,omitempty"`
	Kind                  string `json:"kind"`
	Controller            string `json:"controller"`
	SubURL                string `json:"subUrl"`
	SubError              string `json:"subError"`
	SubCheckedAt          int64  `json:"subCheckedAt"`
	AgentVersion          string `json:"agentVersion"`
	MihomoVersion         string `json:"mihomoVersion"`
	LastSeen              int64  `json:"lastSeen"`
	CreatedAt             int64  `json:"createdAt"`
	NodeCount             int    `json:"nodeCount"`
	AliveCount            int    `json:"aliveCount"`
	ProbeOkCount          int    `json:"probeOkCount"`
	ProbeUnreachableCount int    `json:"probeUnreachableCount"`
	ProbeUnverifiedCount  int    `json:"probeUnverifiedCount"`
	LastProbedAt          int64  `json:"lastProbedAt"`
}

const (
	HostAgent  = "agent"
	HostStatic = "static"
	HostSource = "sub"
)

func NormalizeHostKind(kind string) string {
	switch kind {
	case HostStatic:
		return HostStatic
	case HostSource:
		return HostSource
	}
	return HostAgent
}

func (s *Store) CreateHost(name, kind string) (*Host, error) {
	kind = NormalizeHostKind(kind)
	h := &Host{ID: newID(), Name: name, Kind: kind, Token: newToken(), CreatedAt: now()}
	_, err := s.db.Exec(`INSERT INTO hosts (id,name,token,kind,created_at) VALUES (?,?,?,?,?)`,
		h.ID, h.Name, h.Token, h.Kind, h.CreatedAt)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// CreateSourceHost adds a subscription source: a host whose nodes weft pulls
// itself instead of being told about them by an agent.
func (s *Store) CreateSourceHost(name, rawURL string) (*Host, error) {
	h, err := s.CreateHost(name, HostSource)
	if err != nil {
		return nil, err
	}
	if err := s.SetSourceURL(h.ID, rawURL); err != nil {
		return nil, err
	}
	h.SubURL = rawURL
	return h, nil
}

func (s *Store) SetSourceURL(id, rawURL string) error {
	_, err := s.db.Exec(`UPDATE hosts SET sub_url=?, sub_error='', sub_checked_at=0 WHERE id=?`, rawURL, id)
	return err
}

// SetSourceState records the outcome of the last pull so the panel can show a
// broken subscription instead of an empty host.
func (s *Store) SetSourceState(id, errMsg string, checkedAt int64) error {
	_, err := s.db.Exec(`UPDATE hosts SET sub_error=?, sub_checked_at=? WHERE id=?`, errMsg, checkedAt, id)
	return err
}

func (s *Store) ListHosts() ([]Host, error) {
	rows, err := s.db.Query(`
SELECT h.id, h.name, h.kind, h.controller, h.sub_url, h.sub_error, h.sub_checked_at,
       h.agent_version, h.mihomo_version, h.last_seen, h.created_at,
       COALESCE(n.total,0), COALESCE(n.alive,0),
       COALESCE(n.probe_ok,0), COALESCE(n.probe_unreachable,0), COALESCE(n.probe_unverified,0),
       COALESCE(n.last_probed_at,0)
FROM hosts h
LEFT JOIN (
  SELECT host_id,
         COUNT(*) total,
         SUM(CASE WHEN source IN ('sub','manual') THEN CASE WHEN probe_state='ok' THEN 1 ELSE 0 END ELSE alive END) alive,
         SUM(CASE WHEN source IN ('sub','manual') AND probe_state='ok' THEN 1 ELSE 0 END) probe_ok,
         SUM(CASE WHEN source IN ('sub','manual') AND probe_state='unreachable' THEN 1 ELSE 0 END) probe_unreachable,
         SUM(CASE WHEN source IN ('sub','manual') AND probe_state='unverified' THEN 1 ELSE 0 END) probe_unverified,
         MAX(CASE WHEN source IN ('sub','manual') THEN probed_at ELSE 0 END) last_probed_at
  FROM nodes GROUP BY host_id
) n ON n.host_id = h.id
ORDER BY h.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Host
	for rows.Next() {
		var h Host
		if err := rows.Scan(&h.ID, &h.Name, &h.Kind, &h.Controller, &h.SubURL, &h.SubError, &h.SubCheckedAt,
			&h.AgentVersion, &h.MihomoVersion, &h.LastSeen, &h.CreatedAt, &h.NodeCount, &h.AliveCount,
			&h.ProbeOkCount, &h.ProbeUnreachableCount, &h.ProbeUnverifiedCount, &h.LastProbedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) HostByID(id string) (*Host, error) {
	var h Host
	err := s.db.QueryRow(`SELECT id,name,token,kind,controller,sub_url,sub_error,sub_checked_at,
agent_version,mihomo_version,last_seen,created_at
FROM hosts WHERE id=?`, id).Scan(&h.ID, &h.Name, &h.Token, &h.Kind, &h.Controller,
		&h.SubURL, &h.SubError, &h.SubCheckedAt, &h.AgentVersion, &h.MihomoVersion, &h.LastSeen, &h.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &h, err
}

func (s *Store) HostByToken(token string) (*Host, error) {
	var h Host
	err := s.db.QueryRow(`SELECT id,name,token,kind,controller,sub_url,sub_error,sub_checked_at,
agent_version,mihomo_version,last_seen,created_at
FROM hosts WHERE token=?`, token).Scan(&h.ID, &h.Name, &h.Token, &h.Kind, &h.Controller,
		&h.SubURL, &h.SubError, &h.SubCheckedAt, &h.AgentVersion, &h.MihomoVersion, &h.LastSeen, &h.CreatedAt)
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
	HostID     string          `json:"hostId"`
	HostName   string          `json:"hostName"`
	Key        string          `json:"key"`
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Server     string          `json:"server"`
	Port       int             `json:"port"`
	Alive      bool            `json:"alive"`
	DelayMS    int             `json:"delayMs"`
	ProbeState string          `json:"probeState,omitempty"` // "" | ok | unreachable | unverified
	ProbedAt   int64           `json:"probedAt,omitempty"`
	ResolvedV4 bool            `json:"resolvedV4,omitempty"`
	ResolvedV6 bool            `json:"resolvedV6,omitempty"`
	Region     string          `json:"region"` // 香港 / 新加坡 / … from the source
	Source     string          `json:"source"` // agent | config | manual | sub
	Raw        json.RawMessage `json:"raw,omitempty"`
	UpdatedAt  int64           `json:"updatedAt"`
}

const (
	NodeAgent  = "agent"
	NodeConfig = "config" // read from a hysteria / xray config on the host
	NodeManual = "manual"
	NodeSub    = "sub" // pulled from a subscription source
)

func (s *Store) ReplaceNodes(hostID string, nodes []Node) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Only the agent snapshot is replaced wholesale; hand-entered nodes survive
	// every report so a host can carry both.
	if _, err := tx.Exec(`DELETE FROM nodes WHERE host_id=? AND source<>?`, hostID, NodeManual); err != nil {
		return err
	}
	ts := now()
	alive, delaySum := 0, 0
	for _, n := range nodes {
		raw := n.Raw
		if len(raw) == 0 {
			raw = json.RawMessage(`{}`)
		}
		source := n.Source
		if source != NodeConfig {
			source = NodeAgent
		}
		if _, err := tx.Exec(`INSERT INTO nodes (host_id,key,name,type,server,port,alive,delay_ms,source,raw,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`, hostID, n.Key, n.Name, n.Type, n.Server, n.Port,
			boolInt(n.Alive), n.DelayMS, source, string(raw), ts); err != nil {
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

// ReplaceSourceNodes swaps the whole inventory of a subscription source. Hand
// entered nodes survive, and so does the report history, so the overview still
// shows the source's node count over time. Existing probe results are carried
// forward for nodes whose endpoint has not changed.
func (s *Store) ReplaceSourceNodes(hostID string, nodes []Node) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	type prevProbe struct {
		typ, server, state string
		port, delay        int
		probedAt           int64
		v4, v6             int
	}
	prev := map[string]prevProbe{}
	if rows, err := tx.Query(`SELECT key, type, server, port, delay_ms, probe_state, probed_at, resolved_v4, resolved_v6
FROM nodes WHERE host_id=? AND source<>?`, hostID, NodeManual); err == nil {
		for rows.Next() {
			var k string
			var p prevProbe
			if rows.Scan(&k, &p.typ, &p.server, &p.port, &p.delay, &p.state, &p.probedAt, &p.v4, &p.v6) == nil {
				prev[k] = p
			}
		}
		rows.Close()
	}

	if _, err := tx.Exec(`DELETE FROM nodes WHERE host_id=? AND source<>?`, hostID, NodeManual); err != nil {
		return err
	}
	ts := now()
	okCount, delaySum, delayCount := 0, 0, 0
	for _, n := range nodes {
		raw := n.Raw
		if len(raw) == 0 {
			raw = json.RawMessage(`{}`)
		}
		probeState, probedAt, v4, v6, delayMS := n.ProbeState, n.ProbedAt, boolInt(n.ResolvedV4), boolInt(n.ResolvedV6), n.DelayMS
		if old, ok := prev[n.Key]; ok && old.typ == n.Type && old.server == n.Server && old.port == n.Port && probedAt == 0 {
			probeState, probedAt, v4, v6, delayMS = old.state, old.probedAt, old.v4, old.v6, old.delay
		}
		if _, err := tx.Exec(`INSERT INTO nodes (host_id,key,name,type,server,port,alive,delay_ms,probe_state,probed_at,resolved_v4,resolved_v6,region,source,raw,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(host_id,key) DO UPDATE SET
  name=excluded.name, type=excluded.type, server=excluded.server, port=excluded.port,
  region=excluded.region, source=excluded.source, raw=excluded.raw, updated_at=excluded.updated_at`,
			hostID, n.Key, n.Name, n.Type, n.Server, n.Port, boolInt(n.Alive), delayMS,
			probeState, probedAt, v4, v6, n.Region, NodeSub, string(raw), ts); err != nil {
			return err
		}
		if probeState == ProbeOK {
			okCount++
			if delayMS > 0 {
				delaySum += delayMS
				delayCount++
			}
		}
	}
	avgDelay := 0
	if delayCount > 0 {
		avgDelay = delaySum / delayCount
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO reports (host_id,at,node_count,alive_count,avg_delay) VALUES (?,?,?,?,?)`,
		hostID, ts, len(nodes), okCount, avgDelay); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM reports WHERE host_id=? AND at < ?`, hostID, ts-86400); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveProbeResults writes server-side probe outcomes back to nodes (only for
// sub/manual nodes, leaving agent nodes and `alive` untouched) and appends a
// report row per affected host.
func (s *Store) SaveProbeResults(updates []ProbeUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	hosts := map[string]bool{}
	for _, u := range updates {
		hosts[u.HostID] = true
		if _, err := tx.Exec(`UPDATE nodes
SET probe_state=?, probed_at=?, resolved_v4=?, resolved_v6=?, delay_ms=?
WHERE host_id=? AND key=? AND source IN (?,?)`,
			u.Result.State, u.Result.ProbedAt, boolInt(u.Result.ResolvedV4), boolInt(u.Result.ResolvedV6),
			u.Result.DelayMS, u.HostID, u.Key, NodeSub, NodeManual); err != nil {
			return err
		}
	}

	ts := now()
	for hostID := range hosts {
		var nodeCount, aliveCount int
		var avgDelay float64
		if err := tx.QueryRow(`SELECT COUNT(*),
COALESCE(SUM(CASE WHEN source IN ('sub','manual') THEN CASE WHEN probe_state='ok' THEN 1 ELSE 0 END ELSE alive END), 0),
COALESCE(AVG(CASE WHEN (CASE WHEN source IN ('sub','manual') THEN probe_state='ok' ELSE alive=1 END) AND delay_ms > 0 THEN delay_ms END), 0)
FROM nodes WHERE host_id=?`, hostID).Scan(&nodeCount, &aliveCount, &avgDelay); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO reports (host_id,at,node_count,alive_count,avg_delay) VALUES (?,?,?,?,?)`,
			hostID, ts, nodeCount, aliveCount, int(avgDelay)); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM reports WHERE host_id=? AND at < ?`, hostID, ts-86400); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListNodes(hostID string) ([]Node, error) {
	q := `SELECT n.host_id, h.name, n.key, n.name, n.type, n.server, n.port, n.alive, n.delay_ms,
       n.probe_state, n.probed_at, n.resolved_v4, n.resolved_v6, n.region, n.source, n.raw, n.updated_at
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
		var alive, v4, v6 int
		var raw string
		if err := rows.Scan(&n.HostID, &n.HostName, &n.Key, &n.Name, &n.Type, &n.Server,
			&n.Port, &alive, &n.DelayMS, &n.ProbeState, &n.ProbedAt, &v4, &v6,
			&n.Region, &n.Source, &raw, &n.UpdatedAt); err != nil {
			return nil, err
		}
		n.Alive = alive == 1
		n.ResolvedV4 = v4 == 1
		n.ResolvedV6 = v6 == 1
		n.Raw = json.RawMessage(raw)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) NodeByKey(hostID, key string) (*Node, error) {
	var n Node
	var alive, v4, v6 int
	var raw string
	err := s.db.QueryRow(`SELECT host_id, key, name, type, server, port, alive, delay_ms,
       probe_state, probed_at, resolved_v4, resolved_v6, region, source, raw, updated_at
FROM nodes WHERE host_id=? AND key=?`, hostID, key).Scan(&n.HostID, &n.Key, &n.Name, &n.Type,
		&n.Server, &n.Port, &alive, &n.DelayMS, &n.ProbeState, &n.ProbedAt, &v4, &v6,
		&n.Region, &n.Source, &raw, &n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	n.Alive = alive == 1
	n.ResolvedV4 = v4 == 1
	n.ResolvedV6 = v6 == 1
	n.Raw = json.RawMessage(raw)
	return &n, nil
}

// UpsertNode writes a hand-entered node. The key is derived from the proxy
// itself, so re-importing the same link updates instead of duplicating.
func (s *Store) UpsertNode(hostID string, n Node) error {
	n.Source = NodeManual
	raw := n.Raw
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	_, err := s.db.Exec(`INSERT INTO nodes (host_id,key,name,type,server,port,alive,delay_ms,probe_state,probed_at,resolved_v4,resolved_v6,region,source,raw,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(host_id,key) DO UPDATE SET
  name=excluded.name, type=excluded.type, server=excluded.server, port=excluded.port,
  source=excluded.source, raw=excluded.raw, updated_at=excluded.updated_at,
  probe_state=CASE WHEN nodes.server=excluded.server AND nodes.port=excluded.port AND nodes.type=excluded.type THEN nodes.probe_state ELSE '' END,
  probed_at=CASE WHEN nodes.server=excluded.server AND nodes.port=excluded.port AND nodes.type=excluded.type THEN nodes.probed_at ELSE 0 END,
  resolved_v4=CASE WHEN nodes.server=excluded.server AND nodes.port=excluded.port AND nodes.type=excluded.type THEN nodes.resolved_v4 ELSE 0 END,
  resolved_v6=CASE WHEN nodes.server=excluded.server AND nodes.port=excluded.port AND nodes.type=excluded.type THEN nodes.resolved_v6 ELSE 0 END`,
		hostID, n.Key, n.Name, n.Type, n.Server, n.Port, boolInt(n.Alive), n.DelayMS,
		n.ProbeState, n.ProbedAt, boolInt(n.ResolvedV4), boolInt(n.ResolvedV6),
		n.Region, NodeManual, string(raw), now())
	return err
}

func (s *Store) DeleteNode(hostID, key string) error {
	if _, err := s.db.Exec(`DELETE FROM nodes WHERE host_id=? AND key=? AND source=?`,
		hostID, key, NodeManual); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM bundle_members WHERE host_id=? AND node_key=?`, hostID, key)
	return err
}

// ---------- bundles ----------

type Bundle struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Slug            string   `json:"slug"`
	GroupBy         string   `json:"groupBy"`   // host | type | flat
	GroupType       string   `json:"groupType"` // url-test | select | fallback
	Template        string   `json:"template"`
	RuleSetID       string   `json:"ruleSetId"`
	ExcludeIPv6Only bool     `json:"excludeIpv6Only"`
	CreatedAt       int64    `json:"createdAt"`
	Members         []Member `json:"members,omitempty"`
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
	_, err := s.db.Exec(`INSERT INTO bundles (id,name,slug,group_by,group_type,template,rule_set_id,exclude_ipv6_only,created_at)
VALUES (?,?,?,?,?,?,?,?,?)`, b.ID, b.Name, b.Slug, b.GroupBy, b.GroupType, b.Template, b.RuleSetID, boolInt(b.ExcludeIPv6Only), b.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) ListBundles() ([]Bundle, error) {
	rows, err := s.db.Query(`SELECT id,name,slug,group_by,group_type,template,rule_set_id,exclude_ipv6_only,created_at FROM bundles ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bundle
	for rows.Next() {
		var b Bundle
		var exclV6 int
		if err := rows.Scan(&b.ID, &b.Name, &b.Slug, &b.GroupBy, &b.GroupType, &b.Template, &b.RuleSetID, &exclV6, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.ExcludeIPv6Only = exclV6 == 1
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
	var exclV6 int
	err := s.db.QueryRow(`SELECT id,name,slug,group_by,group_type,template,rule_set_id,exclude_ipv6_only,created_at FROM bundles WHERE id=?`, id).
		Scan(&b.ID, &b.Name, &b.Slug, &b.GroupBy, &b.GroupType, &b.Template, &b.RuleSetID, &exclV6, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	b.ExcludeIPv6Only = exclV6 == 1
	b.Members, err = s.Members(id)
	return &b, err
}

func (s *Store) UpdateBundle(b Bundle) error {
	_, err := s.db.Exec(`UPDATE bundles SET name=?,slug=?,group_by=?,group_type=?,template=?,rule_set_id=?,exclude_ipv6_only=? WHERE id=?`,
		b.Name, slugify(b.Slug), b.GroupBy, b.GroupType, b.Template, b.RuleSetID, boolInt(b.ExcludeIPv6Only), b.ID)
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
	AccessKey  string       `json:"accessKey"`
	Bearer     string       `json:"bearer"`
	Format     string       `json:"format"`
	Enabled    bool         `json:"enabled"`
	Hits       int64        `json:"hits"`
	LastUsedAt int64        `json:"lastUsedAt"`
	LastUA     string       `json:"lastUa"`
	CreatedAt  int64        `json:"createdAt"`
	Devices    []AccessStat `json:"devices"`
}

// newAccessKey is the short second factor on a subscription URL. Shorter than
// the token on purpose: it travels in a query string and people do retype it.
func newAccessKey() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// newBearer is the token some clients send as an Authorization header. It is
// longer than the query key because it never appears in a URL.
func newBearer() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Store) CreateSubscription(bundleID, label, format string) (*Subscription, error) {
	sub := &Subscription{ID: newID(), BundleID: bundleID, Label: label, Token: newToken(),
		AccessKey: newAccessKey(), Bearer: newBearer(), Format: format, Enabled: true, CreatedAt: now()}
	_, err := s.db.Exec(`INSERT INTO subscriptions (id,bundle_id,label,token,access_key,bearer,format,enabled,created_at)
VALUES (?,?,?,?,?,?,?,?,?)`, sub.ID, sub.BundleID, sub.Label, sub.Token, sub.AccessKey, sub.Bearer, sub.Format, 1, sub.CreatedAt)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *Store) ListSubscriptions() ([]Subscription, error) {
	rows, err := s.db.Query(`SELECT s.id,s.bundle_id,COALESCE(b.name,''),s.label,s.token,s.access_key,s.bearer,s.format,s.enabled,
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
		if err := rows.Scan(&sub.ID, &sub.BundleID, &sub.BundleName, &sub.Label, &sub.Token, &sub.AccessKey,
			&sub.Bearer, &sub.Format, &enabled, &sub.Hits, &sub.LastUsedAt, &sub.LastUA, &sub.CreatedAt); err != nil {
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
	err := s.db.QueryRow(`SELECT id,bundle_id,label,token,access_key,bearer,format,enabled,hits,last_used_at,last_ua,created_at
FROM subscriptions WHERE id=?`, id).Scan(&sub.ID, &sub.BundleID, &sub.Label, &sub.Token, &sub.AccessKey,
		&sub.Bearer, &sub.Format, &enabled, &sub.Hits, &sub.LastUsedAt, &sub.LastUA, &sub.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	sub.Enabled = enabled == 1
	return &sub, err
}

func (s *Store) SubscriptionByToken(token string) (*Subscription, error) {
	var sub Subscription
	var enabled int
	err := s.db.QueryRow(`SELECT id,bundle_id,label,token,access_key,bearer,format,enabled,hits,last_used_at,last_ua,created_at
FROM subscriptions WHERE token=?`, token).Scan(&sub.ID, &sub.BundleID, &sub.Label, &sub.Token, &sub.AccessKey,
		&sub.Bearer, &sub.Format, &enabled, &sub.Hits, &sub.LastUsedAt, &sub.LastUA, &sub.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	sub.Enabled = enabled == 1
	return &sub, err
}

func (s *Store) UpdateSubscription(id string, label *string, enabled *bool, format, bearer *string) error {
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
	if bearer != nil {
		if _, err := s.db.Exec(`UPDATE subscriptions SET bearer=? WHERE id=?`, *bearer, id); err != nil {
			return err
		}
	}
	return nil
}

// RotateSubscription retires both halves of the URL: the token in the path and
// the key in the query.
func (s *Store) RotateSubscription(id string) (string, string, error) {
	t, key := newToken(), newAccessKey()
	_, err := s.db.Exec(`UPDATE subscriptions SET token=?,access_key=?,bearer=? WHERE id=?`, t, key, newBearer(), id)
	return t, key, err
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

// ---------- sessions ----------

// Session is a panel login. Persisting it in SQLite keeps a restart from
// signing every device out.
type Session struct {
	Token     string
	CSRF      string
	CreatedAt int64
	ExpiresAt int64
}

func (s *Store) CreateSession(token, csrf string, expiresAt int64) error {
	_, err := s.db.Exec(`INSERT INTO sessions (token,csrf,created_at,expires_at) VALUES (?,?,?,?)`,
		token, csrf, now(), expiresAt)
	return err
}

func (s *Store) Session(token string) (*Session, error) {
	var sess Session
	err := s.db.QueryRow(`SELECT token,csrf,created_at,expires_at FROM sessions WHERE token=?`, token).
		Scan(&sess.Token, &sess.CSRF, &sess.CreatedAt, &sess.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &sess, err
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token=?`, token)
	return err
}

func (s *Store) DeleteOtherSessions(keep string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token<>?`, keep)
	return err
}

func (s *Store) CleanupSessions(at int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, at)
	return err
}

// ---------- rule sets ----------

// RuleSet is a user-editable slice of mihomo routing: the `rules` list, the
// `rule-providers` it references, and any extra proxy groups those rules use.
// It is stored as a small YAML document so the panel can stay a text editor.
type RuleSet struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Content       string   `json:"content"`
	Source        string   `json:"source"`
	UpdatedAt     int64    `json:"updatedAt"`
	CreatedAt     int64    `json:"createdAt"`
	Warnings      []string `json:"warnings,omitempty"`
	RuleCount     int      `json:"ruleCount,omitempty"`
	ProviderCount int      `json:"providerCount,omitempty"`
	UsedBy        int      `json:"usedBy,omitempty"`
}

func (s *Store) CreateRuleSet(rs RuleSet) (*RuleSet, error) {
	rs.ID = newID()
	rs.CreatedAt = now()
	rs.UpdatedAt = rs.CreatedAt
	_, err := s.db.Exec(`INSERT INTO rule_sets (id,name,description,content,source,updated_at,created_at)
VALUES (?,?,?,?,?,?,?)`, rs.ID, rs.Name, rs.Description, rs.Content, rs.Source, rs.UpdatedAt, rs.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &rs, nil
}

func (s *Store) ListRuleSets() ([]RuleSet, error) {
	rows, err := s.db.Query(`SELECT id,name,description,content,source,updated_at,created_at FROM rule_sets ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RuleSet
	for rows.Next() {
		var rs RuleSet
		if err := rows.Scan(&rs.ID, &rs.Name, &rs.Description, &rs.Content, &rs.Source, &rs.UpdatedAt, &rs.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rs)
	}
	return out, rows.Err()
}

func (s *Store) RuleSet(id string) (*RuleSet, error) {
	var rs RuleSet
	err := s.db.QueryRow(`SELECT id,name,description,content,source,updated_at,created_at FROM rule_sets WHERE id=?`, id).
		Scan(&rs.ID, &rs.Name, &rs.Description, &rs.Content, &rs.Source, &rs.UpdatedAt, &rs.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &rs, err
}

func (s *Store) UpdateRuleSet(rs RuleSet) error {
	_, err := s.db.Exec(`UPDATE rule_sets SET name=?,description=?,content=?,source=?,updated_at=? WHERE id=?`,
		rs.Name, rs.Description, rs.Content, rs.Source, now(), rs.ID)
	return err
}

func (s *Store) DeleteRuleSet(id string) error {
	if _, err := s.db.Exec(`UPDATE bundles SET rule_set_id='' WHERE rule_set_id=?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM rule_sets WHERE id=?`, id)
	return err
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
