package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
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
		CREATE EXTENSION IF NOT EXISTS vector;
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
		-- AI 诊断向量记忆（RAG 相似案例检索）
		CREATE TABLE IF NOT EXISTS diagnosis_embeddings (
			id          BIGSERIAL PRIMARY KEY,
			incident_id BIGINT,
			embedding   vector(1536),
			summary     TEXT NOT NULL,
			severity    TEXT,
			tags        TEXT,
			feedback    TEXT DEFAULT '',
			created_at  TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS diag_emb_incident ON diagnosis_embeddings(incident_id);
		-- 经验规则库（高频问题 best practice）
		CREATE TABLE IF NOT EXISTS experience_rules (
			id          BIGSERIAL PRIMARY KEY,
			pattern     TEXT NOT NULL,
			conclusion  TEXT NOT NULL,
			severity    TEXT,
			incident_id BIGINT,
			created_at  TIMESTAMPTZ DEFAULT NOW()
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

// ============================================================================
// pgvector: AI 诊断向量记忆（RAG 相似案例检索）
// ============================================================================

// vecStr formats a []float64 as a pgvector literal string, e.g. "[0.1,0.2,...]".
func vecStr(v []float64) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%g", f)
	}
	b.WriteByte(']')
	return b.String()
}

// insertDiagnosisEmbedding stores a diagnosis embedding for later RAG retrieval.
func (p *pgStore) insertDiagnosisEmbedding(incidentID int64, emb []float64, summary, severity, tags string) (int64, error) {
	var id int64
	err := p.db.QueryRow(
		`INSERT INTO diagnosis_embeddings(incident_id, embedding, summary, severity, tags)
		 VALUES($1, $2::vector, $3, $4, $5) RETURNING id`,
		incidentID, vecStr(emb), summary, severity, tags,
	).Scan(&id)
	return id, err
}

// searchSimilarCases returns the top-N similar diagnosis cases by cosine distance.
func (p *pgStore) searchSimilarCases(emb []float64, limit int) ([]similarCase, error) {
	if limit <= 0 {
		limit = 3
	}
	rows, err := p.db.Query(
		`SELECT id, incident_id, summary, severity, tags, feedback,
		        embedding <=> $1::vector AS distance
		 FROM diagnosis_embeddings
		 ORDER BY embedding <=> $1::vector
		 LIMIT $2`,
		vecStr(emb), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []similarCase
	for rows.Next() {
		var c similarCase
		if err := rows.Scan(&c.ID, &c.IncidentID, &c.Summary, &c.Severity, &c.Tags, &c.Feedback, &c.Distance); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// updateDiagnosisFeedback records user feedback on a diagnosis embedding.
func (p *pgStore) updateDiagnosisFeedback(incidentID int64, feedback string) error {
	_, err := p.db.Exec(
		`UPDATE diagnosis_embeddings SET feedback=$1 WHERE incident_id=$2`,
		feedback, incidentID,
	)
	return err
}

type similarCase struct {
	ID         int64   `json:"id"`
	IncidentID int64   `json:"incident_id"`
	Summary    string  `json:"summary"`
	Severity   string  `json:"severity"`
	Tags       string  `json:"tags"`
	Feedback   string  `json:"feedback"`
	Distance   float64 `json:"distance"` // cosine distance, lower = more similar
}

// ============================================================================
// 经验规则库 CRUD
// ============================================================================

// experienceRule is one manually-curated or AI-extracted best-practice rule.
type experienceRule struct {
	ID         int64  `json:"id"`
	Pattern    string `json:"pattern"`
	Conclusion string `json:"conclusion"`
	Severity   string `json:"severity,omitempty"`
	IncidentID int64  `json:"incident_id,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

func (p *pgStore) insertExperienceRule(r experienceRule) (int64, error) {
	var id int64
	err := p.db.QueryRow(
		`INSERT INTO experience_rules(pattern, conclusion, severity, incident_id)
		 VALUES($1, $2, $3, $4) RETURNING id`,
		r.Pattern, r.Conclusion, r.Severity, r.IncidentID,
	).Scan(&id)
	return id, err
}

func (p *pgStore) listExperienceRules() ([]experienceRule, error) {
	rows, err := p.db.Query(`SELECT id, pattern, conclusion, severity, incident_id, created_at FROM experience_rules ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []experienceRule
	for rows.Next() {
		var r experienceRule
		if err := rows.Scan(&r.ID, &r.Pattern, &r.Conclusion, &r.Severity, &r.IncidentID, &r.CreatedAt); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *pgStore) deleteExperienceRule(id int64) error {
	_, err := p.db.Exec(`DELETE FROM experience_rules WHERE id=$1`, id)
	return err
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
	s.pg = ps
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
	// Notification-center feed + read state survive restart.
	if raw, _ := ps.loadKV("messages"); raw != nil {
		var msgs []Message
		if json.Unmarshal(raw, &msgs) == nil {
			s.messages.importMsgs(msgs)
		}
	}
	// AI inspection history survives restart (SRE 中枢巡检报告).
	if raw, _ := ps.loadKV("ai_inspections"); raw != nil {
		var reps []InspectionReport
		if json.Unmarshal(raw, &reps) == nil {
			s.ai.importReports(reps)
		}
	}
	// Remediation run history survives restart (自动修复执行历史).
	if raw, _ := ps.loadKV("remediation_runs"); raw != nil {
		var runs []RemediationRun
		if json.Unmarshal(raw, &runs) == nil {
			s.remediation.Import(runs)
		}
	}
	// SLO burning state survives restart (SLO 燃烧状态).
	if raw, _ := ps.loadKV("slo_burning"); raw != nil {
		var burning map[string]bool
		if json.Unmarshal(raw, &burning) == nil {
			s.slos.importBurning(burning)
		}
	}
	// Aggregated agent logs survive restart (日志检索缓冲).
	if raw, _ := ps.loadKV("logs"); raw != nil {
		var logs []StoredLog
		if json.Unmarshal(raw, &logs) == nil {
			s.logs.importLogs(logs)
		}
	}
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		tick := 0
		for range t.C {
			tick++
			s.pgFlush(ps, tick%2 == 0) // heavy log blob every ~30s
		}
	}()
}

// pgFlush persists the current relational state to PostgreSQL (also called on
// shutdown for a final flush). withLogs gates the large aggregated-log blob so
// the periodic 15s flush does not rewrite it every time.
func (s *Server) pgFlush(ps *pgStore, withLogs bool) {
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
	if raw, err := json.Marshal(s.messages.export()); err == nil {
		_ = ps.saveKV("messages", raw)
	}
	// AI inspection history is small (≤ inspectionReportCap) — persist every flush.
	if raw, err := json.Marshal(s.ai.exportReports()); err == nil {
		_ = ps.saveKV("ai_inspections", raw)
	}
	// Remediation run history is small (≤ remediationRunCap) — persist every flush.
	if raw, err := json.Marshal(s.remediation.Export()); err == nil {
		_ = ps.saveKV("remediation_runs", raw)
	}
	// SLO burning state is tiny — persist every flush.
	if raw, err := json.Marshal(s.slos.exportBurning()); err == nil {
		_ = ps.saveKV("slo_burning", raw)
	}
	// Aggregated agent logs can be large — only on the slower cadence / shutdown.
	if withLogs {
		if raw, err := json.Marshal(s.logs.export()); err == nil {
			_ = ps.saveKV("logs", raw)
		}
	}
}
