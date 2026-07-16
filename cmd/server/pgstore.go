package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
		-- 终端会话录制的「永久审计索引」：只存元数据(info)，录制内容(帧)留在本地文件
		-- (/app/data/recordings/<id>.json，随持久卷永久保存)，避免大 blob 撑爆 PG。
		CREATE TABLE IF NOT EXISTS terminal_recordings (
			id   TEXT PRIMARY KEY,
			ts   BIGINT,
			info JSONB NOT NULL
		);
		-- 兼容早期把整段录制塞进 PG 的版本：删掉重列，回归「内容存文件、PG 只存元数据」。
		ALTER TABLE terminal_recordings DROP COLUMN IF EXISTS recording;
		CREATE INDEX IF NOT EXISTS terminal_recordings_ts ON terminal_recordings(ts DESC);
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
		-- 通用 AI 记忆库：对话 / 文件 / URL / 多轮历史 全部向量化，持续沉淀为可 RAG 检索的知识
		CREATE TABLE IF NOT EXISTS ai_memory_embeddings (
			id         BIGSERIAL PRIMARY KEY,
			kind       TEXT NOT NULL,
			source     TEXT,
			content    TEXT NOT NULL,
			embedding  vector(1536),
			created_at BIGINT NOT NULL,
			last_hit_at BIGINT DEFAULT 0,
			priority   REAL DEFAULT 1.0
		);
		-- 兼容老表：补增 last_hit_at / priority 列（若不存在）
		ALTER TABLE ai_memory_embeddings ADD COLUMN IF NOT EXISTS last_hit_at BIGINT DEFAULT 0;
		ALTER TABLE ai_memory_embeddings ADD COLUMN IF NOT EXISTS priority REAL DEFAULT 1.0;
		CREATE INDEX IF NOT EXISTS ai_mem_kind ON ai_memory_embeddings(kind);
		CREATE INDEX IF NOT EXISTS ai_mem_created ON ai_memory_embeddings(created_at DESC);
		CREATE INDEX IF NOT EXISTS ai_mem_kind_created ON ai_memory_embeddings(kind, created_at DESC);
		-- 经验规则库（高频问题 best practice）
		CREATE TABLE IF NOT EXISTS experience_rules (
			id          BIGSERIAL PRIMARY KEY,
			pattern     TEXT NOT NULL,
			conclusion  TEXT NOT NULL,
			severity    TEXT,
			incident_id BIGINT,
			created_at  TIMESTAMPTZ DEFAULT NOW()
		);
		-- Hermes Agent 规则库（诊断规则 + 行动策略）
		CREATE TABLE IF NOT EXISTS hermes_rules (
			id          BIGSERIAL PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT DEFAULT '',
			priority    INT DEFAULT 0,
			enabled     BOOLEAN DEFAULT true,
			config      JSONB NOT NULL,
			created_at  TIMESTAMPTZ DEFAULT NOW(),
			updated_at  TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS hermes_rules_enabled ON hermes_rules(enabled);
		-- Hermes Agent 提示模板库（系统提示 + 场景模板）
		CREATE TABLE IF NOT EXISTS hermes_templates (
			id          BIGSERIAL PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT DEFAULT '',
			content     TEXT NOT NULL,
			category    TEXT DEFAULT 'system',
			version     INT DEFAULT 1,
			active      BOOLEAN DEFAULT true,
			created_at  TIMESTAMPTZ DEFAULT NOW(),
			updated_at  TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS hermes_templates_active ON hermes_templates(active);
		-- Hermes Agent 会话记忆
		CREATE TABLE IF NOT EXISTS hermes_sessions (
			id          BIGSERIAL PRIMARY KEY,
			incident_id BIGINT DEFAULT 0,
			status      TEXT DEFAULT 'active',
			messages    JSONB NOT NULL DEFAULT '[]',
			created_at  TIMESTAMPTZ DEFAULT NOW(),
			updated_at  TIMESTAMPTZ DEFAULT NOW()
		);
		-- 告警历史持久化记录（触发时写入，恢复时更新 resolved_at）
		CREATE TABLE IF NOT EXISTS alert_history (
			id          BIGSERIAL PRIMARY KEY,
			key         TEXT NOT NULL,
			fired_at    BIGINT NOT NULL,
			resolved_at BIGINT DEFAULT 0,
			data        JSONB NOT NULL
		);
		CREATE INDEX IF NOT EXISTS alert_history_key ON alert_history(key);
		CREATE INDEX IF NOT EXISTS alert_history_fired ON alert_history(fired_at DESC);
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

// --- terminal session recordings (permanent audit retention) ---

// saveTermRecording persists one ended session's METADATA to PG permanently
// (idempotent). The recording CONTENT (frames) stays in the local file
// /app/data/recordings/<id>.json — PG only holds the audit index so the session
// list shows full history without bloating the DB with large blobs.
func (p *pgStore) saveTermRecording(a termArchive) {
	if a.info.ID == "" {
		return
	}
	info, err := json.Marshal(a.info)
	if err != nil {
		return
	}
	if _, err := p.db.Exec(
		`INSERT INTO terminal_recordings(id,ts,info) VALUES($1,$2,$3) ON CONFLICT (id) DO NOTHING`,
		a.info.ID, a.info.CreatedAt, info); err != nil {
		slog.Warn("PG 写终端会话录制索引失败", "err", err)
	}
}

// listTermRecordings returns recent ended sessions' metadata (newest first) from
// the permanent PG store, so the session list shows the full history, not just
// the last termArchiveCap sessions held in memory.
func (p *pgStore) listTermRecordings(limit int) []termSessionInfo {
	rows, err := p.db.Query(`SELECT info FROM terminal_recordings ORDER BY ts DESC LIMIT $1`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []termSessionInfo
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) != nil {
			continue
		}
		var info termSessionInfo
		if json.Unmarshal(raw, &info) == nil {
			info.Active = false
			out = append(out, info)
		}
	}
	return out
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

// --- alert history (fire on insert, resolve on update; unbounded in PG) ---

func (p *pgStore) appendAlertRecord(r AlertRecord) {
	raw, err := json.Marshal(r)
	if err != nil {
		return
	}
	if _, err := p.db.Exec(`INSERT INTO alert_history(key,fired_at,data) VALUES($1,$2,$3)`,
		r.Key, r.FiredAt, raw); err != nil {
		slog.Warn("PG 写告警历史失败", "err", err)
	}
}

func (p *pgStore) resolveAlertRecord(id int64, resolvedAt int64) {
	if _, err := p.db.Exec(`UPDATE alert_history SET resolved_at=$1 WHERE id=$2`, resolvedAt, id); err != nil {
		slog.Warn("PG 更新告警恢复时间失败", "err", err)
	}
}

func (p *pgStore) loadRecentAlerts(limit int) ([]AlertRecord, error) {
	rows, err := p.db.Query(`SELECT data FROM (SELECT id,data FROM alert_history ORDER BY id DESC LIMIT $1) t ORDER BY id ASC`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AlertRecord
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) != nil {
			continue
		}
		var r AlertRecord
		if json.Unmarshal(raw, &r) == nil {
			out = append(out, r)
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

// ---- 通用 AI 记忆（对话 / 文件 / URL / 多轮历史 向量化，持续沉淀 RAG 知识，自我进化）----

// insertMemoryEmbedding 存一条 AI 记忆向量。kind: chat|file|url|history|diagnosis。
func (p *pgStore) insertMemoryEmbedding(kind, source, content string, emb []float64, ts int64) error {
	_, err := p.db.Exec(
		`INSERT INTO ai_memory_embeddings(kind, source, content, embedding, created_at)
		 VALUES($1, $2, $3, $4::vector, $5)`,
		kind, source, content, vecStr(emb), ts)
	return err
}

type memoryHit struct {
	ID       int64   `json:"id"`
	Kind     string  `json:"kind"`
	Source   string  `json:"source"`
	Content  string  `json:"content"`
	Distance float64 `json:"distance"`
}

// searchMemory 按余弦距离取最相近的 N 条 AI 记忆（RAG 检索，跨对话/文件/URL/历史）。
func (p *pgStore) searchMemory(emb []float64, limit int) ([]memoryHit, error) {
	if limit <= 0 {
		limit = 3
	}
	rows, err := p.db.Query(
		`SELECT id, kind, source, content, embedding <=> $1::vector AS distance
		 FROM ai_memory_embeddings
		 ORDER BY (embedding <=> $1::vector) / GREATEST(priority, 0.1) LIMIT $2`,
		vecStr(emb), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memoryHit
	for rows.Next() {
		var m memoryHit
		if err := rows.Scan(&m.ID, &m.Kind, &m.Source, &m.Content, &m.Distance); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// searchMemoryByKind 按 kind 优先检索记忆：先查指定 kind 的 Top-K，不足时补充其他 kind。
// 用于诊断对话优先召回历史诊断结论、普通对话优先召回通用知识等场景。
// 排序公式：distance / (priority * time_factor)，其中：
//   - time_factor = max(0.5, 1 - days/365) 时间衰减
//   - 最近 7 天额外 1.5x 权重加成
func (p *pgStore) searchMemoryByKind(emb []float64, preferKind string, limit int) ([]memoryHit, error) {
	if limit <= 0 {
		limit = 5
	}
	now := time.Now().Unix()
	sevenDaysAgo := now - 7*86400
	// 先查指定 kind 的前 limit 条
	preferred := limit * 2 / 3 // 2/3 给优先 kind
	if preferred < 1 {
		preferred = 1
	}
	rows, err := p.db.Query(
		`SELECT id, kind, source, content, embedding <=> $1::vector AS distance
		 FROM ai_memory_embeddings WHERE kind = $4
		 ORDER BY (embedding <=> $1::vector) / (GREATEST(priority, 0.1) *
		   GREATEST(0.5, 1.0 - (EXTRACT(EPOCH FROM NOW()) - created_at) / 31536000.0) *
		   CASE WHEN created_at > $3 THEN 1.5 ELSE 1.0 END)
		 LIMIT $2`,
		vecStr(emb), preferred, sevenDaysAgo, preferKind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memoryHit
	seen := make(map[string]bool)
	for rows.Next() {
		var m memoryHit
		if err := rows.Scan(&m.ID, &m.Kind, &m.Source, &m.Content, &m.Distance); err != nil {
			continue
		}
		key := m.Kind + ":" + m.Source
		if !seen[key] {
			out = append(out, m)
			seen[key] = true
		}
	}
	// 不足 limit 时，补充其他 kind
	if len(out) < limit {
		rows2, err2 := p.db.Query(
			`SELECT id, kind, source, content, embedding <=> $1::vector AS distance
			 FROM ai_memory_embeddings WHERE kind != $4
			 ORDER BY (embedding <=> $1::vector) / (GREATEST(priority, 0.1) *
			   GREATEST(0.5, 1.0 - (EXTRACT(EPOCH FROM NOW()) - created_at) / 31536000.0) *
			   CASE WHEN created_at > $3 THEN 1.5 ELSE 1.0 END)
			 LIMIT $2`,
			vecStr(emb), limit-len(out), sevenDaysAgo, preferKind)
		if err2 == nil {
			defer rows2.Close()
			for rows2.Next() {
				var m memoryHit
				if err := rows2.Scan(&m.ID, &m.Kind, &m.Source, &m.Content, &m.Distance); err != nil {
					continue
				}
				key := m.Kind + ":" + m.Source
				if !seen[key] {
					out = append(out, m)
					seen[key] = true
				}
			}
		}
	}
	return out, rows.Err()
}

// memoryContentHash 计算内容哈希用于去重判断（SHA256 前 16 位）。
func memoryContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:8])
}

// hasDuplicateMemory 检查是否已存在高度相似的记忆（余弦距离 < 0.12，即相似度 > 88%）。
// 阈值从 0.05 放宽到 0.12，覆盖更多语义等价内容（如 "CPU 90%" vs "CPU 使用率超过 90%"）。
// 返回 duplicate ID 以便调用方执行合并逻辑。
func (p *pgStore) hasDuplicateMemory(emb []float64, kind string) (bool, int64, error) {
	var id int64
	err := p.db.QueryRow(
		`SELECT id FROM ai_memory_embeddings
		 WHERE kind = $2 AND embedding <=> $1::vector < 0.12
		 ORDER BY embedding <=> $1::vector LIMIT 1`,
		vecStr(emb), kind).Scan(&id)
	if err != nil {
		return false, 0, nil // no duplicate found
	}
	return true, id, nil
}

// mergeDuplicateMemory appends new content to an existing memory and updates its embedding.
// Used when a near-duplicate is detected: instead of creating a new entry, the new
// knowledge is appended to preserve both the original and supplementary information.
func (p *pgStore) mergeDuplicateMemory(id int64, appendContent string, newEmb []float64) error {
	_, err := p.db.Exec(
		`UPDATE ai_memory_embeddings
		 SET content = content || E'\n' || $2,
		     embedding = $3::vector,
		     created_at = $4
		 WHERE id = $1`,
		id, appendContent, vecStr(newEmb), time.Now().Unix())
	return err
}

// touchMemoryHits 批量更新被检索命中的记忆的 last_hit_at 字段，
// 用于衰减策略判断“未被检索命中”的记忆。
func (p *pgStore) touchMemoryHits(ids []int64) {
	if len(ids) == 0 {
		return
	}
	now := time.Now().Unix()
	for _, id := range ids {
		_, _ = p.db.Exec(
			`UPDATE ai_memory_embeddings SET last_hit_at = $2 WHERE id = $1`,
			id, now)
	}
}

// decayOldMemories 对超过 90 天且未被检索命中的记忆降低优先级（priority *= 0.8），
// 而非删除——保留历史知识但让新鲜记忆在检索时排名更高。
// 建议每天调用一次（由 Server 启动时 goroutine 驱动）。
func (p *pgStore) decayOldMemories() {
	cutoff := time.Now().Add(-90 * 24 * time.Hour).Unix()
	res, err := p.db.Exec(
		`UPDATE ai_memory_embeddings
		 SET priority = GREATEST(priority * 0.8, 0.1)
		 WHERE created_at < $1 AND (last_hit_at = 0 OR last_hit_at < $1)`,
		cutoff)
	if err != nil {
		slog.Warn("记忆衰减执行失败", "err", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Info("记忆衰减完成", "降低优先级条数", n)
	}
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

// --- Hermes rules CRUD ---

type hermesRule struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Priority    int             `json:"priority"`
	Enabled     bool            `json:"enabled"`
	Config      json.RawMessage `json:"config"`
	CreatedAt   string          `json:"created_at,omitempty"`
	UpdatedAt   string          `json:"updated_at,omitempty"`
}

func (p *pgStore) listHermesRules() ([]hermesRule, error) {
	rows, err := p.db.Query(`SELECT id,name,description,priority,enabled,config,created_at,updated_at FROM hermes_rules ORDER BY priority DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hermesRule
	for rows.Next() {
		var r hermesRule
		var ca, ua sql.NullTime
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Priority, &r.Enabled, &r.Config, &ca, &ua); err != nil {
			continue
		}
		if ca.Valid {
			r.CreatedAt = ca.Time.Format(time.RFC3339)
		}
		if ua.Valid {
			r.UpdatedAt = ua.Time.Format(time.RFC3339)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *pgStore) upsertHermesRule(r hermesRule) (int64, error) {
	if r.ID > 0 {
		_, err := p.db.Exec(`UPDATE hermes_rules SET name=$1,description=$2,priority=$3,enabled=$4,config=$5,updated_at=NOW() WHERE id=$6`,
			r.Name, r.Description, r.Priority, r.Enabled, r.Config, r.ID)
		return r.ID, err
	}
	var id int64
	err := p.db.QueryRow(`INSERT INTO hermes_rules(name,description,priority,enabled,config) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		r.Name, r.Description, r.Priority, r.Enabled, r.Config).Scan(&id)
	return id, err
}

func (p *pgStore) deleteHermesRule(id int64) error {
	_, err := p.db.Exec(`DELETE FROM hermes_rules WHERE id=$1`, id)
	return err
}

// --- Hermes templates CRUD ---

type hermesTemplate struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content"`
	Category    string `json:"category"`
	Version     int    `json:"version"`
	Active      bool   `json:"active"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

func (p *pgStore) listHermesTemplates(activeOnly bool) ([]hermesTemplate, error) {
	q := `SELECT id,name,description,content,category,version,active,created_at,updated_at FROM hermes_templates`
	if activeOnly {
		q += ` WHERE active=true`
	}
	q += ` ORDER BY id ASC`
	rows, err := p.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hermesTemplate
	for rows.Next() {
		var t hermesTemplate
		var ca, ua sql.NullTime
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Content, &t.Category, &t.Version, &t.Active, &ca, &ua); err != nil {
			continue
		}
		if ca.Valid {
			t.CreatedAt = ca.Time.Format(time.RFC3339)
		}
		if ua.Valid {
			t.UpdatedAt = ua.Time.Format(time.RFC3339)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (p *pgStore) upsertHermesTemplate(t hermesTemplate) (int64, error) {
	if t.ID > 0 {
		_, err := p.db.Exec(`UPDATE hermes_templates SET name=$1,description=$2,content=$3,category=$4,version=version+1,active=$5,updated_at=NOW() WHERE id=$6`,
			t.Name, t.Description, t.Content, t.Category, t.Active, t.ID)
		return t.ID, err
	}
	var id int64
	err := p.db.QueryRow(`INSERT INTO hermes_templates(name,description,content,category,active) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		t.Name, t.Description, t.Content, t.Category, t.Active).Scan(&id)
	return id, err
}

func (p *pgStore) deleteHermesTemplate(id int64) error {
	_, err := p.db.Exec(`DELETE FROM hermes_templates WHERE id=$1`, id)
	return err
}

// --- Hermes sessions ---

func (p *pgStore) loadHermesSession(id int64) ([]byte, error) {
	var raw []byte
	err := p.db.QueryRow(`SELECT messages FROM hermes_sessions WHERE id=$1`, id).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return raw, err
}

func (p *pgStore) saveHermesSession(id int64, messages []byte, incidentID int64) (int64, error) {
	if id > 0 {
		_, err := p.db.Exec(`UPDATE hermes_sessions SET messages=$1,updated_at=NOW() WHERE id=$2`, messages, id)
		return id, err
	}
	var newID int64
	err := p.db.QueryRow(`INSERT INTO hermes_sessions(incident_id,messages) VALUES($1,$2) RETURNING id`, incidentID, messages).Scan(&newID)
	return newID, err
}

func (p *pgStore) listHermesSessions(limit int) ([]map[string]any, error) {
	rows, err := p.db.Query(`SELECT id,incident_id,status,created_at,updated_at,messages FROM hermes_sessions ORDER BY updated_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, iid int64
		var status string
		var ca, ua sql.NullTime
		var raw []byte
		if err := rows.Scan(&id, &iid, &status, &ca, &ua, &raw); err != nil {
			continue
		}
		m := map[string]any{"id": id, "incident_id": iid, "status": status}
		if ca.Valid {
			m["created_at"] = ca.Time.Format(time.RFC3339)
		}
		if ua.Valid {
			m["updated_at"] = ua.Time.Format(time.RFC3339)
		}
		// 从消息内容提取标题（首条用户消息）、摘要（末条消息）与条数，便于前端列表展示
		title, summary, count := hermesSessionDigest(raw)
		m["title"] = title
		m["summary"] = summary
		m["msg_count"] = count
		out = append(out, m)
	}
	return out, rows.Err()
}

// hermesSessionDigest 从会话 messages(JSON) 提取标题（首条 user 内容）、摘要（末条内容）与消息条数。
func hermesSessionDigest(raw []byte) (title, summary string, count int) {
	if len(raw) == 0 {
		return "新会话", "", 0
	}
	var msgs []map[string]string
	if json.Unmarshal(raw, &msgs) != nil {
		return "新会话", "", 0
	}
	count = len(msgs)
	for _, m := range msgs {
		if m["role"] == "user" && strings.TrimSpace(m["content"]) != "" {
			title = hermesTrunc(m["content"], 24)
			break
		}
	}
	if title == "" {
		title = "新会话"
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.TrimSpace(msgs[i]["content"]) != "" {
			summary = hermesTrunc(msgs[i]["content"], 40)
			break
		}
	}
	return title, summary, count
}

// hermesTrunc 按 Unicode 字符（rune）截断字符串，避免中文被切成半个字符。
func hermesTrunc(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
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
	// Playbook execution history survives restart (剧本执行审计).
	if raw, _ := ps.loadKV("playbook_executions"); raw != nil {
		var execs []PlaybookExecution
		if json.Unmarshal(raw, &execs) == nil {
			s.playbooks.importExecutions(execs)
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
	// Playbook execution history is small (≤ 100 records) — persist every flush.
	if raw, err := json.Marshal(s.playbooks.exportExecutions()); err == nil {
		_ = ps.saveKV("playbook_executions", raw)
	}
}
