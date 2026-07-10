package main

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// pgFromEnv opens the PostgreSQL store from AIOPS_POSTGRES_DSN, or returns nil if
// it is unset or unreachable (callers then fall back to embedded/file mode).
func pgFromEnv() *pgStore {
	dsn := os.Getenv("AIOPS_POSTGRES_DSN")
	if dsn == "" {
		return nil
	}
	ps, err := openPGStore(dsn)
	if err != nil {
		slog.Error("PostgreSQL 连接失败，回落内嵌存储", "err", err)
		return nil
	}
	return ps
}

// ============================================================================
// PostgreSQL persistence (optional, enabled via AIOPS_POSTGRES_DSN).
//
// When a DSN is configured, the durable SRE records — incidents and work orders,
// which grow over time and benefit from a real database — are persisted to
// PostgreSQL instead of (well, in addition to) the embedded snapshot. Records
// are stored as JSONB rows keyed by id, so the Go structs stay the source of
// truth and no brittle column-per-field migration is needed. When no DSN is set,
// the server behaves exactly as before (embedded snapshot only).
// ============================================================================

type pgStore struct {
	db *sql.DB
}

// openPGStore connects, pings and migrates. A non-nil error means fall back to
// the embedded snapshot.
func openPGStore(dsn string) (*pgStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctxPing := make(chan error, 1)
	go func() { ctxPing <- db.Ping() }()
	select {
	case err := <-ctxPing:
		if err != nil {
			db.Close()
			return nil, err
		}
	case <-time.After(10 * time.Second):
		db.Close()
		return nil, sql.ErrConnDone
	}
	ps := &pgStore{db: db}
	if err := ps.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return ps, nil
}

func (p *pgStore) migrate() error {
	_, err := p.db.Exec(`
		CREATE TABLE IF NOT EXISTS incidents (
			id         BIGINT PRIMARY KEY,
			status     TEXT,
			created_at BIGINT,
			data       JSONB NOT NULL
		);
		CREATE INDEX IF NOT EXISTS incidents_status ON incidents(status);
		CREATE TABLE IF NOT EXISTS tickets (
			id         BIGINT PRIMARY KEY,
			status     TEXT,
			created_at BIGINT,
			data       JSONB NOT NULL
		);
		CREATE INDEX IF NOT EXISTS tickets_status ON tickets(status);
		CREATE TABLE IF NOT EXISTS app_config (
			id   INT PRIMARY KEY,
			data JSONB NOT NULL
		);
		CREATE TABLE IF NOT EXISTS audit_log (
			id   BIGSERIAL PRIMARY KEY,
			ts   BIGINT,
			data JSONB NOT NULL
		);
		CREATE INDEX IF NOT EXISTS audit_log_ts ON audit_log(ts);
		CREATE TABLE IF NOT EXISTS events (
			id   BIGSERIAL PRIMARY KEY,
			ts   BIGINT,
			data JSONB NOT NULL
		);
		CREATE INDEX IF NOT EXISTS events_ts ON events(ts);
		CREATE TABLE IF NOT EXISTS hosts (
			id   TEXT PRIMARY KEY,
			data JSONB NOT NULL
		);
		CREATE TABLE IF NOT EXISTS kv_state (
			k    TEXT PRIMARY KEY,
			data JSONB NOT NULL
		);
	`)
	return err
}

// --- hosts (metadata + latest + custom gauges; history lives in VM, not PG) ---

func (p *pgStore) loadHosts() ([]*Host, error) {
	rows, err := p.db.Query(`SELECT data FROM hosts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Host
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) != nil {
			continue
		}
		var h Host
		if json.Unmarshal(raw, &h) == nil && h.ID != "" {
			hh := h
			out = append(out, &hh)
		}
	}
	return out, rows.Err()
}

// saveHosts replaces the host set atomically (DELETE + INSERT in one tx) so
// operator-deleted hosts don't linger. Host counts are small, so this is cheap.
func (p *pgStore) saveHosts(hosts []*Host) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM hosts`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO hosts(id,data) VALUES($1,$2)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, h := range hosts {
		if h == nil || h.ID == "" {
			continue
		}
		raw, _ := json.Marshal(h)
		if _, err := stmt.Exec(h.ID, raw); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- small key-value state blobs (alert-ack states, login sessions) ---

func (p *pgStore) loadKV(key string) ([]byte, error) {
	var raw []byte
	err := p.db.QueryRow(`SELECT data FROM kv_state WHERE k=$1`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return raw, err
}

func (p *pgStore) saveKV(key string, raw []byte) error {
	_, err := p.db.Exec(`INSERT INTO kv_state(k,data) VALUES($1,$2)
		ON CONFLICT(k) DO UPDATE SET data=EXCLUDED.data`, key, raw)
	return err
}

// --- config blob (whole ServerConfig as one JSONB row; replaces the JSON file) ---

func (p *pgStore) loadConfigBlob() ([]byte, bool, error) {
	var raw []byte
	err := p.db.QueryRow(`SELECT data FROM app_config WHERE id=1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func (p *pgStore) saveConfigBlob(raw []byte) error {
	_, err := p.db.Exec(`INSERT INTO app_config(id,data) VALUES(1,$1)
		ON CONFLICT(id) DO UPDATE SET data=EXCLUDED.data`, raw)
	return err
}

// --- audit log (append-only, unbounded in PG; the store keeps a recent cache) ---

func (p *pgStore) appendAudit(e LogEntry) {
	raw, err := json.Marshal(e)
	if err != nil {
		return
	}
	if _, err := p.db.Exec(`INSERT INTO audit_log(ts,data) VALUES($1,$2)`, e.Timestamp, raw); err != nil {
		slog.Warn("PG 写审计日志失败", "err", err)
	}
}

func (p *pgStore) loadRecentAudit(limit int) ([]LogEntry, error) {
	rows, err := p.db.Query(`SELECT data FROM (SELECT id,data FROM audit_log ORDER BY id DESC LIMIT $1) t ORDER BY id ASC`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) != nil {
			continue
		}
		var e LogEntry
		if json.Unmarshal(raw, &e) == nil {
			out = append(out, e)
		}
	}
	return out, rows.Err()
}

// --- plugin events ---

func (p *pgStore) appendEvent(e storedEvent) {
	raw, err := json.Marshal(e)
	if err != nil {
		return
	}
	if _, err := p.db.Exec(`INSERT INTO events(ts,data) VALUES($1,$2)`, e.Timestamp, raw); err != nil {
		slog.Warn("PG 写事件失败", "err", err)
	}
}

func (p *pgStore) loadRecentEvents(limit int) ([]storedEvent, error) {
	rows, err := p.db.Query(`SELECT data FROM (SELECT id,data FROM events ORDER BY id DESC LIMIT $1) t ORDER BY id ASC`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []storedEvent
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) != nil {
			continue
		}
		var e storedEvent
		if json.Unmarshal(raw, &e) == nil {
			out = append(out, e)
		}
	}
	return out, rows.Err()
}

// --- incidents ---

func (p *pgStore) loadIncidents() ([]Incident, error) {
	rows, err := p.db.Query(`SELECT data FROM incidents ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Incident
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var inc Incident
		if json.Unmarshal(raw, &inc) == nil {
			out = append(out, inc)
		}
	}
	return out, rows.Err()
}

func (p *pgStore) saveIncidents(list []Incident) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO incidents(id,status,created_at,data) VALUES($1,$2,$3,$4)
		ON CONFLICT(id) DO UPDATE SET status=EXCLUDED.status, data=EXCLUDED.data`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, inc := range list {
		raw, _ := json.Marshal(inc)
		if _, err := stmt.Exec(inc.ID, inc.Status, inc.CreatedAt, raw); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- tickets ---

func (p *pgStore) loadTickets() ([]Ticket, error) {
	rows, err := p.db.Query(`SELECT data FROM tickets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ticket
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var tk Ticket
		if json.Unmarshal(raw, &tk) == nil {
			out = append(out, tk)
		}
	}
	return out, rows.Err()
}

func (p *pgStore) saveTickets(list []Ticket) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO tickets(id,status,created_at,data) VALUES($1,$2,$3,$4)
		ON CONFLICT(id) DO UPDATE SET status=EXCLUDED.status, data=EXCLUDED.data`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, tk := range list {
		raw, _ := json.Marshal(tk)
		if _, err := stmt.Exec(tk.ID, tk.Status, tk.CreatedAt, raw); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *pgStore) close() {
	if p != nil && p.db != nil {
		_ = p.db.Close()
	}
}

// bindPG wires an already-open PostgreSQL store as the persistence backend for
// all durable relational state: incidents, work orders, host metadata, alert-ack
// states and login sessions. It loads existing rows on start, then periodically
// writes the current state back.
func (s *Server) bindPG(ps *pgStore) {
	if ps == nil {
		return
	}
	if incs, err := ps.loadIncidents(); err == nil && len(incs) > 0 {
		s.incidents.Import(incs)
	}
	if tks, err := ps.loadTickets(); err == nil && len(tks) > 0 {
		s.tickets.Import(tks)
	}
	// Login sessions survive restart (no forced re-login in dual-DB mode).
	if raw, _ := ps.loadKV("sessions"); raw != nil {
		var sess map[string]dbSession
		if json.Unmarshal(raw, &sess) == nil {
			s.auth.importSessions(sess)
		}
	}
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for range t.C {
			s.pgFlush(ps)
		}
	}()
}

// pgFlush persists the current relational state to PostgreSQL (also called on
// shutdown for a final flush).
func (s *Server) pgFlush(ps *pgStore) {
	if err := ps.saveIncidents(s.incidents.Export()); err != nil {
		slog.Warn("PG 同步事件失败", "err", err)
	}
	if err := ps.saveTickets(s.tickets.Export()); err != nil {
		slog.Warn("PG 同步工单失败", "err", err)
	}
	if err := ps.saveHosts(s.store.exportHosts()); err != nil {
		slog.Warn("PG 同步主机失败", "err", err)
	}
	if raw, err := json.Marshal(s.store.exportAlertStates()); err == nil {
		_ = ps.saveKV("alert_states", raw)
	}
	if raw, err := json.Marshal(s.auth.exportSessions()); err == nil {
		_ = ps.saveKV("sessions", raw)
	}
}
