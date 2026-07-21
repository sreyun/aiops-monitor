package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"aiops-monitor/shared"
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
	db       *sql.DB
	flowJobs chan flowJob // NetFlow 明细异步入库队列（解耦 agent POST 与 PG 写入，防连接池饿死）
}

// flowJob 是一批待入库的 Flow 明细。
type flowJob struct {
	hostID string
	source string
	flows  []shared.FlowRecord
}

// applyPGSafetyTimeouts 向 DSN 注入连接级安全超时（作为运行时 GUC，随连接建立生效）：
//   - lock_timeout：单条语句等待锁不超过 15s，避免锁等待堆积拖垮连接池；
//   - idle_in_transaction_session_timeout：事务内空闲超 60s 即断开，回收泄漏/挂起的连接。
//
// 刻意不设全局 statement_timeout —— 迁移、分区重建、大聚合等合法长查询不应被硬杀；上面两项
// 已能防止"长时间阻塞耗尽连接池"这一核心风险。若用户已在 DSN 里显式配置同名参数则尊重之。
func applyPGSafetyTimeouts(dsn string) string {
	params := map[string]string{
		"lock_timeout":                        "15000",
		"idle_in_transaction_session_timeout": "60000",
	}
	lower := strings.ToLower(dsn)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return dsn
		}
		q := u.Query()
		for k, v := range params {
			if q.Get(k) == "" {
				q.Set(k, v)
			}
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	// keyword/value 形式：只追加未出现过的参数
	out := dsn
	for k, v := range params {
		if !strings.Contains(lower, k) {
			out += fmt.Sprintf(" %s=%s", k, v)
		}
	}
	return out
}

// openPGStore connects, pings and migrates. A non-nil error means fall back to
// the embedded snapshot.
func openPGStore(dsn string) (*pgStore, error) {
	db, err := sql.Open("postgres", applyPGSafetyTimeouts(dsn))
	if err != nil {
		return nil, err
	}
	// 面向 500+ 并发用户/多机上报的连接池：放大到 200 上限，空闲保留 50 以摊薄高峰突发的建连开销。
	// 注意：PostgreSQL 的 max_connections 需 ≥ 200（外加 superuser 预留），否则会出现 "too many clients"；
	// 若 PG 端上限较低，请相应下调此值或在 PG 前置 PgBouncer 做连接复用。
	db.SetMaxOpenConns(200)
	db.SetMaxIdleConns(50)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute) // 回收长期空闲连接，避免占满 PG 侧会话
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
	ps := &pgStore{db: db, flowJobs: make(chan flowJob, 512)}
	if err := ps.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	// 2 个后台工作协程串行化 Flow 明细写入：HTTP 摄入只入队即返回，写库不再占住请求连接。
	for i := 0; i < 2; i++ {
		go ps.flowIngestWorker()
	}
	return ps, nil
}

// flowIngestWorker 从队列取批次并批量写库（见 insertFlowRecords）。
func (p *pgStore) flowIngestWorker() {
	for j := range p.flowJobs {
		p.insertFlowRecords(j.hostID, j.source, j.flows)
	}
}

// insertFlowRecordsAsync 非阻塞入队；队列满时丢弃本批并告警（背压优于把服务拖垮）。
func (p *pgStore) insertFlowRecordsAsync(hostID, source string, flows []shared.FlowRecord) {
	if p == nil || len(flows) == 0 {
		return
	}
	select {
	case p.flowJobs <- flowJob{hostID: hostID, source: source, flows: flows}:
	default:
		slog.Warn("Flow 入库队列已满，丢弃本批明细（写入跟不上摄入速率）", "host", hostID, "rows", len(flows))
	}
}

func (p *pgStore) migrate() error {
	// 必须先于建表：老库里 flow_records 已存在时，下面的
	// CREATE TABLE IF NOT EXISTS 会直接跳过，分区永远不会生效。
	if err := p.migrateFlowRecordsToPartitioned(); err != nil {
		// 改造失败不该让整个服务起不来——退回非分区老表照样能跑，只是没法按月归档。
		slog.Error("flow_records 分区改造失败，继续以现有表结构运行", "err", err)
	}

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
		-- AI 技能库（自进化核心）：从 experience/resolution 记忆中提炼出的「可复用 SOP」。
		-- 与 ai_memory_embeddings（原始经验片段）不同，skill 是更高阶、命名化、带触发条件与
		-- 操作步骤的结构化产物，检索后作为「已掌握技能」注入提示词，让 AI 直接复用被验证的做法。
		CREATE TABLE IF NOT EXISTS ai_skills (
			id            BIGSERIAL PRIMARY KEY,
			name          TEXT NOT NULL,
			trigger_desc  TEXT NOT NULL,          -- 何时适用（自然语言，供语义匹配；trigger 是 SQL 关键字故用 _desc）
			steps         TEXT NOT NULL,          -- 怎么做（步骤 / SOP）
			tags          TEXT DEFAULT '',
			embedding     vector(1536),           -- name+trigger_desc 的向量，用于检索
			use_count     INT  DEFAULT 0,
			success_count INT  DEFAULT 0,
			priority      REAL DEFAULT 1.0,
			source        TEXT DEFAULT 'distilled', -- distilled | manual
			created_at    BIGINT NOT NULL,
			updated_at    BIGINT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS ai_skills_priority ON ai_skills(priority DESC);
		-- 经验规则库（高频问题 best practice）
		CREATE TABLE IF NOT EXISTS experience_rules (
			id          BIGSERIAL PRIMARY KEY,
			pattern     TEXT NOT NULL,
			conclusion  TEXT NOT NULL,
			severity    TEXT,
			incident_id BIGINT,
			created_at  TIMESTAMPTZ DEFAULT NOW()
		);
		-- Sreyun Agent 规则库（诊断规则 + 行动策略）
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
		-- Sreyun Agent 提示模板库（系统提示 + 场景模板）
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
		-- Sreyun Agent 会话记忆
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
		-- Redfish 硬件最新快照（UPSERT by host_id + target_name）
		CREATE TABLE IF NOT EXISTS hardware_snapshot (
			host_id     TEXT NOT NULL,
			target_name TEXT NOT NULL,
			target_url  TEXT,
			snapshot    JSONB NOT NULL,
			health      TEXT,
			updated_at  TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (host_id, target_name)
		);
		-- Redfish 硬件事件（状态变更/故障/固件升级）
		CREATE TABLE IF NOT EXISTS hardware_events (
			id          BIGSERIAL PRIMARY KEY,
			host_id     TEXT NOT NULL,
			target_name TEXT,
			event_type  TEXT NOT NULL,
			severity    TEXT,
			message     TEXT,
			created_at  TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_hw_events_host_time ON hardware_events(host_id, created_at DESC);
		-- 硬件资产变更历史：**只在部件真的增/删/换时**写一条，永久保留。
		-- 快照表只存最新一份（主键 host_id+target_name），换过哪块盘、哪条内存
		-- 事后完全查不到——这张表就是补这个洞。每轮都存整份快照则 99% 是重复数据。
		CREATE TABLE IF NOT EXISTS hardware_changes (
			id          BIGSERIAL PRIMARY KEY,
			host_id     TEXT NOT NULL,
			target_name TEXT NOT NULL,
			kind        TEXT NOT NULL,   -- disk / dimm / psu / cpu / gpu / raid / firmware / enclosure
			component   TEXT NOT NULL,   -- 槽位或部件名，如 "Bay 3" / "DIMM A1"
			action      TEXT NOT NULL,   -- added / removed / replaced / changed
			old_value   TEXT,
			new_value   TEXT,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_hw_changes_host_time ON hardware_changes(host_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_hw_changes_component ON hardware_changes(host_id, kind, component);
		-- Hyper-V 虚拟机清单：每台物理宿主机一份（整份 guests 存 JSONB），覆盖式 upsert。
		-- 与 hardware_snapshot 同构，只是一台宿主对应一份清单，故主键仅 host_id。
		CREATE TABLE IF NOT EXISTS hyperv_inventory (
			host_id     TEXT PRIMARY KEY,
			host_name   TEXT,
			guest_count INT DEFAULT 0,
			snapshot    JSONB NOT NULL,
			updated_at  TIMESTAMPTZ DEFAULT NOW()
		);
		-- Hyper-V 虚拟机事件：VM 增/删/状态跳变，只在变化时写一条，永久保留。
		CREATE TABLE IF NOT EXISTS hyperv_events (
			id         BIGSERIAL PRIMARY KEY,
			host_id    TEXT NOT NULL,
			vm_name    TEXT,
			vm_id      TEXT,
			kind       TEXT NOT NULL,   -- vm_added / vm_removed / state_change
			severity   TEXT,
			message    TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_hyperv_events_host_time ON hyperv_events(host_id, created_at DESC);
		-- Flow 明细：按月分区、**永久保留**（归档靠 DROP/DETACH 分区，不再定时删除）。
		-- 分区键必须进主键，故 PK 是 (id, created_at)。
		CREATE TABLE IF NOT EXISTS flow_records (
			id          BIGSERIAL,
			host_id     TEXT NOT NULL,
			source      TEXT NOT NULL,
			src_ip      INET,
			dst_ip      INET,
			src_port    INT,
			dst_port    INT,
			protocol    INT,
			bytes       BIGINT,
			packets     BIGINT,
			first_seen  TIMESTAMPTZ,
			last_seen   TIMESTAMPTZ,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (id, created_at)
		) PARTITION BY RANGE (created_at);
		-- 兜底分区：任何月份分区没来得及建时，数据落这里而不是插入失败。
		CREATE TABLE IF NOT EXISTS flow_records_default PARTITION OF flow_records DEFAULT;
		CREATE INDEX IF NOT EXISTS idx_flow_host_time ON flow_records(host_id, created_at DESC);

		-- SNMP 设备快照：一台设备一份，按 (host_id, device_name) upsert。
		-- 与 hardware_snapshot 同构：采集失败（Error 非空）时上层不覆盖上一份好数据。
		CREATE TABLE IF NOT EXISTS snmp_snapshot (
			host_id     TEXT NOT NULL,
			device_name TEXT NOT NULL,
			device_ip   TEXT,
			snapshot    JSONB NOT NULL,
			reachable   BOOLEAN DEFAULT TRUE,
			updated_at  TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (host_id, device_name)
		);
		-- SNMP Trap 事件：追加写，供告警联动/查询/取证。
		CREATE TABLE IF NOT EXISTS snmp_traps (
			id          BIGSERIAL PRIMARY KEY,
			host_id     TEXT NOT NULL,
			source_ip   TEXT,
			version     TEXT,
			trap_oid    TEXT,
			severity    TEXT,
			uptime_sec  DOUBLE PRECISION,
			varbinds    JSONB,
			received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_snmp_traps_host_time ON snmp_traps(host_id, received_at DESC);

		-- 明文 HTTP 内容审计（Phase 2）。高敏感：body 可能含用户发给大模型的 prompt。
		-- 落 PG 是因为审计记录不可易失；保留期由 cleanup 定期清理（见 cleanupContentAudit）。
		CREATE TABLE IF NOT EXISTS content_audit (
			id          BIGSERIAL PRIMARY KEY,
			host_id     TEXT NOT NULL,
			src_ip      TEXT,
			dst_ip      TEXT,
			dst_port    INT,
			method      TEXT,
			host        TEXT,
			path        TEXT,
			ctype       TEXT,
			body        TEXT,
			status         INT,
			resp_ctype     TEXT,
			resp_body      TEXT,
			req_truncated  BOOLEAN,
			resp_truncated BOOLEAN,
			sensitive   TEXT,
			observed_at TIMESTAMPTZ,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_content_audit_host_time ON content_audit(host_id, created_at DESC);
		-- 兼容：早期表可能缺响应/敏感列，幂等补齐。
		ALTER TABLE content_audit ADD COLUMN IF NOT EXISTS status INT;
		ALTER TABLE content_audit ADD COLUMN IF NOT EXISTS resp_ctype TEXT;
		ALTER TABLE content_audit ADD COLUMN IF NOT EXISTS resp_body TEXT;
		ALTER TABLE content_audit ADD COLUMN IF NOT EXISTS req_truncated BOOLEAN;
		ALTER TABLE content_audit ADD COLUMN IF NOT EXISTS resp_truncated BOOLEAN;
		ALTER TABLE content_audit ADD COLUMN IF NOT EXISTS sensitive TEXT;
	`)
	return err
}

// migrateFlowRecordsToPartitioned converts a pre-existing non-partitioned
// flow_records into a monthly-partitioned one, preserving rows.
//
// 必须在 initSchema **之前**跑：老表存在时 CREATE TABLE IF NOT EXISTS 不会报错也不会改造它，
// 于是分区永远不会生效。整个改造在一个事务里完成（PG 的 DDL 是事务性的），
// 中途失败会整体回滚，不会留下半吊子状态。
func (p *pgStore) migrateFlowRecordsToPartitioned() error {
	var exists, partitioned bool
	if err := p.db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='flow_records')`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil // 全新部署：initSchema 会直接建成分区表
	}
	if err := p.db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM pg_partitioned_table pt JOIN pg_class c ON c.oid=pt.partrelid
		WHERE c.relname='flow_records')`).Scan(&partitioned); err != nil {
		return err
	}
	if partitioned {
		return nil // 已经是分区表
	}

	// 数据量太大时不在启动路径上做在线拷贝——那会把服务卡住好几分钟。
	// 老表此前一直有 7 天清理，正常不会很大；真超了就明确报出来让人工处理。
	var n int64
	if err := p.db.QueryRow(`SELECT count(*) FROM flow_records`).Scan(&n); err != nil {
		return err
	}
	const maxInlineRows = 5_000_000
	if n > maxInlineRows {
		slog.Error("flow_records 行数过多，跳过自动分区改造（避免启动时长时间锁表）",
			"rows", n, "limit", maxInlineRows,
			"action", "请在维护窗口手工改造：重命名旧表→建分区表→分批回灌→删旧表")
		return nil
	}

	slog.Info("开始把 flow_records 改造成按月分区表", "rows", n)
	start := time.Now()
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`ALTER TABLE flow_records RENAME TO flow_records_legacy`,
		`DROP INDEX IF EXISTS idx_flow_host_time`,
		`CREATE TABLE flow_records (
			id BIGSERIAL, host_id TEXT NOT NULL, source TEXT NOT NULL,
			src_ip INET, dst_ip INET, src_port INT, dst_port INT, protocol INT,
			bytes BIGINT, packets BIGINT, first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (id, created_at)
		) PARTITION BY RANGE (created_at)`,
		`CREATE TABLE flow_records_default PARTITION OF flow_records DEFAULT`,
		`INSERT INTO flow_records (host_id, source, src_ip, dst_ip, src_port, dst_port,
			protocol, bytes, packets, first_seen, last_seen, created_at)
		 SELECT host_id, source, src_ip, dst_ip, src_port, dst_port,
			protocol, bytes, packets, first_seen, last_seen, COALESCE(created_at, NOW())
		 FROM flow_records_legacy`,
		`DROP TABLE flow_records_legacy`,
		`CREATE INDEX idx_flow_host_time ON flow_records(host_id, created_at DESC)`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("分区改造失败于 [%.60s]: %w", q, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	slog.Info("flow_records 已改造为按月分区表", "rows", n, "耗时", time.Since(start))
	return nil
}

// isSafeFlowPartitionName 校验分区表标识符只能是 flow_records_ 前缀 + 6 位数字（YYYYMM），
// 作为拼接进 DDL 前的最后一道防线。
func isSafeFlowPartitionName(name string) bool {
	const prefix = "flow_records_"
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	suffix := name[len(prefix):]
	if len(suffix) != 6 {
		return false
	}
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ensureFlowPartitions creates monthly partitions for the current and next
// months. Idempotent; safe to call on every tick.
//
// 有 DEFAULT 兜底分区在，缺分区也不会插入失败；但数据落在 DEFAULT 里就没法按月
// DROP 归档了。注意：DEFAULT 里一旦已有该月数据，PG 会拒绝再建这个月的分区，
// 因此这里失败只记日志、不当错误——数据仍在 DEFAULT 中可查。
func (p *pgStore) ensureFlowPartitions() {
	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, i, 0)
		end := start.AddDate(0, 1, 0)
		name := fmt.Sprintf("flow_records_%04d%02d", start.Year(), start.Month())
		// 防御性白名单：分区表名由 time.Now() 生成，理应恒为 flow_records_YYYYMM。这里再校验一次，
		// 万一上游逻辑被篡改产生异常标识符也不会拼进 DDL 执行（SQL 注入面归零）。
		if !isSafeFlowPartitionName(name) {
			slog.Warn("跳过异常分区名（疑似被篡改）", "partition", name)
			continue
		}
		q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s PARTITION OF flow_records
			FOR VALUES FROM ('%s') TO ('%s')`,
			name, start.Format("2006-01-02"), end.Format("2006-01-02"))
		if _, err := p.db.Exec(q); err != nil {
			slog.Debug("创建 Flow 月分区未成功（多为 DEFAULT 分区已有该月数据，可忽略）",
				"partition", name, "err", err)
		}
	}
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

// ---- 反馈驱动的检索重排：让 👍/👎 真正改变 RAG 结果（learn 闭环）----
//
// 用户对诊断结论的 👍/👎（helpful/unhelpful）此前只作为提示标注展示，并不影响检索排序，
// 反馈形同虚设。这里把用户评价折算成「有效距离」的增减：👍 上浮、👎 下沉（通常被挤出 Top-N），
// 使每一次反馈都改变后续对话能检索到的历史案例——这才是可自我进化的学习闭环。
//
// 权重刻意保守且仅用于排序：对外返回的 similarCase.Distance 保持原始余弦距离，
// 展示的相似度% 依旧真实，不会被反馈"注水"。
const (
	feedbackHelpfulBonus     = 0.05 // 👍 案例：有效距离 -0.05，轻微提前
	feedbackUnhelpfulPenalty = 0.20 // 👎 案例：有效距离 +0.20，显著靠后（通常被挤出 Top-N）
)

// feedbackAdjustedDistance 返回用于排序的「有效距离」：在原始余弦距离上叠加反馈增减。
// 空 / 未知反馈按中性处理（不调整）。
func feedbackAdjustedDistance(rawDistance float64, feedback string) float64 {
	switch feedback {
	case "helpful":
		return rawDistance - feedbackHelpfulBonus
	case "unhelpful":
		return rawDistance + feedbackUnhelpfulPenalty
	default:
		return rawDistance
	}
}

// rerankByFeedback 按「有效距离」升序稳定重排候选案例，再截断到 limit：
// 👍 案例上浮、👎 案例下沉（通常被挤出 Top-N），实现反馈学习闭环。
// limit<=0 表示不截断；原始 Distance 不被修改。
func rerankByFeedback(cases []similarCase, limit int) []similarCase {
	sort.SliceStable(cases, func(i, j int) bool {
		return feedbackAdjustedDistance(cases[i].Distance, cases[i].Feedback) <
			feedbackAdjustedDistance(cases[j].Distance, cases[j].Feedback)
	})
	if limit > 0 && len(cases) > limit {
		cases = cases[:limit]
	}
	return cases
}

// searchSimilarCases returns the top-N similar diagnosis cases, re-ranked by user feedback.
// 先用向量索引按余弦距离取较大候选集（保留 ivfflat 索引加速），再交给 rerankByFeedback 让
// 👍/👎 影响最终排序，使用户反馈真正改变 RAG 检索结果（learn 闭环），而非仅作展示标注。
func (p *pgStore) searchSimilarCases(emb []float64, limit int) ([]similarCase, error) {
	if limit <= 0 {
		limit = 3
	}
	// 放大候选集：Top 案例被 👎 惩罚挤下去后，仍需有优质案例补位；至少取 12 条。
	fetch := limit * 4
	if fetch < 12 {
		fetch = 12
	}
	rows, err := p.db.Query(
		`SELECT id, incident_id, summary, severity, tags, feedback,
		        embedding <=> $1::vector AS distance
		 FROM diagnosis_embeddings
		 ORDER BY embedding <=> $1::vector
		 LIMIT $2`,
		vecStr(emb), fetch,
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rerankByFeedback(out, limit), nil
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

// ---- 正向强化：与 decayOldMemories 负向衰减对称，构成「采纳/成功/解决即强化」学习闭环 ----
//
// 检索排序公式为 distance / (priority * time_factor * recency)，priority 越大越靠前。此前
// priority 只会因衰减【下降】、从不上升，"好记忆"无法脱颖而出。这里补上正向半环：真实结果
// （被采纳 / 执行成功 / 事件解决 / 👍）把相关记忆的 priority 上调，让被验证有效的知识随使用上浮。
// 上限 5.0 与衰减下限 0.1 对称，避免单次反馈过度主导。
const memoryPriorityCap = 5.0

// boostMemoryPriority 按 factor 调整单条记忆优先级（factor>1 强化、<1 惩罚），并刷新 last_hit_at。
func (p *pgStore) boostMemoryPriority(id int64, factor float64) {
	if factor <= 0 {
		factor = 1.3
	}
	if _, err := p.db.Exec(
		`UPDATE ai_memory_embeddings
		 SET priority = LEAST(GREATEST(priority, 0.1) * $2, $3), last_hit_at = $4
		 WHERE id = $1`,
		id, factor, memoryPriorityCap, time.Now().Unix()); err != nil {
		slog.Warn("记忆强化失败", "id", id, "err", err)
	}
}

// boostMemoryBySource 对某 kind+source 的记忆整体调整优先级。适用于 source 唯一的场景
// （incident:ID / playbook:ID / session:ID）。返回受影响条数。
func (p *pgStore) boostMemoryBySource(kind, source string, factor float64) int64 {
	if factor <= 0 {
		factor = 1.3
	}
	res, err := p.db.Exec(
		`UPDATE ai_memory_embeddings
		 SET priority = LEAST(GREATEST(priority, 0.1) * $3, $4), last_hit_at = $5
		 WHERE kind = $1 AND source = $2`,
		kind, source, factor, memoryPriorityCap, time.Now().Unix())
	if err != nil {
		slog.Warn("按来源强化记忆失败", "kind", kind, "source", source, "err", err)
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}

// boostNearestMemory 找与 emb 语义最相近的一条 kind 记忆并调整其优先级，返回其 id。
// 适用于 source 不唯一、需按内容定位具体交互的场景（如 AI 辅助采纳反馈）。
func (p *pgStore) boostNearestMemory(emb []float64, kind string, factor float64) (int64, bool) {
	var id int64
	if err := p.db.QueryRow(
		`SELECT id FROM ai_memory_embeddings WHERE kind = $2 ORDER BY embedding <=> $1::vector LIMIT 1`,
		vecStr(emb), kind).Scan(&id); err != nil {
		return 0, false
	}
	p.boostMemoryPriority(id, factor)
	return id, true
}

// ---- AI 技能库（自进化）：提炼产物的存取 / 检索 / 强化 / 管理 ----

// Skill 是从经验记忆中提炼出的一条可复用 SOP。
type Skill struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Trigger      string  `json:"trigger"` // 何时适用
	Steps        string  `json:"steps"`   // 怎么做
	Tags         string  `json:"tags"`
	UseCount     int     `json:"use_count"`
	SuccessCount int     `json:"success_count"`
	Priority     float64 `json:"priority"`
	Source       string  `json:"source"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
	Distance     float64 `json:"distance,omitempty"`
}

func (p *pgStore) insertSkill(name, trigger, steps, tags, source string, emb []float64) (int64, error) {
	now := time.Now().Unix()
	var id int64
	err := p.db.QueryRow(
		`INSERT INTO ai_skills(name, trigger_desc, steps, tags, embedding, source, created_at, updated_at)
		 VALUES($1,$2,$3,$4,$5::vector,$6,$7,$7) RETURNING id`,
		name, trigger, steps, tags, vecStr(emb), source, now).Scan(&id)
	return id, err
}

// findSimilarSkill 返回与 emb 语义最近的技能 id（若距离 ≤ maxDist），用于提炼时去重/合并。
func (p *pgStore) findSimilarSkill(emb []float64, maxDist float64) (int64, bool) {
	var id int64
	var dist float64
	if err := p.db.QueryRow(
		`SELECT id, embedding <=> $1::vector AS d FROM ai_skills ORDER BY embedding <=> $1::vector LIMIT 1`,
		vecStr(emb)).Scan(&id, &dist); err != nil || dist > maxDist {
		return 0, false
	}
	return id, true
}

// updateSkill 覆盖一条技能（用于「用中自改进」——把更好的步骤写回）。
func (p *pgStore) updateSkill(id int64, name, trigger, steps string, emb []float64) error {
	_, err := p.db.Exec(
		`UPDATE ai_skills SET name=$2, trigger_desc=$3, steps=$4, embedding=$5::vector, updated_at=$6 WHERE id=$1`,
		id, name, trigger, steps, vecStr(emb), time.Now().Unix())
	return err
}

// searchSkills 按 距离/优先级 检索最相关技能，供注入提示词。
// maxDist 是【原始余弦距离】上限：先用它在 WHERE 里筛掉真正不相关的技能，再对相关候选做
// priority 加权排序取 Top-K。此顺序很关键——否则高 priority 的无关技能会凭加权分挤进 LIMIT、
// 再被上层按原始距离过滤掉，把真正相关但 priority 低的技能挤出候选集（系统越学越严重）。
func (p *pgStore) searchSkills(emb []float64, limit int, maxDist float64) ([]Skill, error) {
	if limit <= 0 {
		limit = 5
	}
	if maxDist <= 0 {
		maxDist = skillRelevantDist
	}
	rows, err := p.db.Query(
		`SELECT id, name, trigger_desc, steps, tags, use_count, success_count, priority, source,
		        embedding <=> $1::vector AS distance
		 FROM ai_skills
		 WHERE embedding <=> $1::vector <= $3
		 ORDER BY (embedding <=> $1::vector) / GREATEST(priority, 0.1) LIMIT $2`,
		vecStr(emb), limit, maxDist)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Skill
	for rows.Next() {
		var s Skill
		if err := rows.Scan(&s.ID, &s.Name, &s.Trigger, &s.Steps, &s.Tags, &s.UseCount, &s.SuccessCount, &s.Priority, &s.Source, &s.Distance); err == nil {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

func (p *pgStore) listSkills() ([]Skill, error) {
	rows, err := p.db.Query(
		`SELECT id, name, trigger_desc, steps, tags, use_count, success_count, priority, source, created_at, updated_at
		 FROM ai_skills ORDER BY priority DESC, updated_at DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Skill
	for rows.Next() {
		var s Skill
		if err := rows.Scan(&s.ID, &s.Name, &s.Trigger, &s.Steps, &s.Tags, &s.UseCount, &s.SuccessCount, &s.Priority, &s.Source, &s.CreatedAt, &s.UpdatedAt); err == nil {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

func (p *pgStore) deleteSkill(id int64) error {
	_, err := p.db.Exec(`DELETE FROM ai_skills WHERE id=$1`, id)
	return err
}

// recordSkillUse 记录一次技能被检索命中（use_count++），成功时额外强化 priority + success_count。
func (p *pgStore) recordSkillUse(id int64, success bool) {
	sc, factor := 0, 1.0
	if success {
		sc, factor = 1, 1.15
	}
	_, _ = p.db.Exec(
		`UPDATE ai_skills SET use_count=use_count+1, success_count=success_count+$2,
		 priority=LEAST(GREATEST(priority,0.1)*$3, 5.0), updated_at=$4 WHERE id=$1`,
		id, sc, factor, time.Now().Unix())
}

// boostSkillNearest 语义定位最近技能并强化（事件解决 / 采纳时调用），实现技能层面的学习闭环。
// 同步 use_count++（视强化为「一次被验证的使用」），保证 success_count ≤ use_count，前端成功率不越界。
func (p *pgStore) boostSkillNearest(emb []float64, factor float64) {
	var id int64
	if err := p.db.QueryRow(`SELECT id FROM ai_skills ORDER BY embedding <=> $1::vector LIMIT 1`, vecStr(emb)).Scan(&id); err == nil {
		_, _ = p.db.Exec(
			`UPDATE ai_skills SET priority=LEAST(GREATEST(priority,0.1)*$2,5.0), use_count=use_count+1, success_count=success_count+1, updated_at=$3 WHERE id=$1`,
			id, factor, time.Now().Unix())
	}
}

// skillProven 判断一条技能是否已被现实验证（有成功记录或被多次使用）——提炼去重时用它保护
// 已验证的优质 SOP 不被一次较差的新生成覆盖。
func (p *pgStore) skillProven(id int64) bool {
	var uc, sc int
	if err := p.db.QueryRow(`SELECT use_count, success_count FROM ai_skills WHERE id=$1`, id).Scan(&uc, &sc); err != nil {
		return false
	}
	return sc > 0 || uc >= 3
}

func (p *pgStore) skillCount() int {
	var n int
	_ = p.db.QueryRow(`SELECT COUNT(*) FROM ai_skills`).Scan(&n)
	return n
}

// memoriesForDistill 取用于技能提炼的候选记忆：experience/resolution/diagnosis 类、较新、
// 按优先级(被强化程度)优先。这些是"被验证有价值"的经验，最适合提炼成可复用技能。
func (p *pgStore) memoriesForDistill(sinceTs int64, limit int) []memoryHit {
	rows, err := p.db.Query(
		`SELECT id, kind, source, content FROM ai_memory_embeddings
		 WHERE kind IN ('experience','resolution','diagnosis') AND created_at >= $1
		 ORDER BY priority DESC, created_at DESC LIMIT $2`,
		sinceTs, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []memoryHit
	for rows.Next() {
		var m memoryHit
		if err := rows.Scan(&m.ID, &m.Kind, &m.Source, &m.Content); err == nil {
			out = append(out, m)
		}
	}
	return out
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

// cleanupExpiredMemories 删除超过 365 天且优先级已降至 < 0.3 的记忆。
// 这些记忆已经历多次衰减且从未被检索命中，可安全清理以释放存储空间。
// P3-2: 记忆生命周期管理的硬清理环节。
func (p *pgStore) cleanupExpiredMemories() {
	cutoff := time.Now().Add(-365 * 24 * time.Hour).Unix()
	res, err := p.db.Exec(
		`DELETE FROM ai_memory_embeddings
		 WHERE created_at < $1 AND priority < 0.3
		   AND (last_hit_at = 0 OR last_hit_at < $1)`,
		cutoff)
	if err != nil {
		slog.Warn("记忆清理执行失败", "err", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Info("记忆清理完成", "删除过期记忆", n)
	}
}

// capMemoriesByKind 对每种 kind 的记忆数量设置上限（maxPerKind），
// 超出时删除最旧且优先级最低的记忆，防止单一类型无限增长。
func (p *pgStore) capMemoriesByKind(maxPerKind int) {
	if maxPerKind <= 0 {
		maxPerKind = 2000
	}
	rows, err := p.db.Query(`SELECT kind, COUNT(*) FROM ai_memory_embeddings GROUP BY kind HAVING COUNT(*) > $1`, maxPerKind)
	if err != nil {
		slog.Warn("记忆容量检查失败", "err", err)
		return
	}
	defer rows.Close()
	totalDeleted := int64(0)
	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			continue
		}
		excess := count - maxPerKind
		if excess <= 0 {
			continue
		}
		// 删除最旧且优先级最低的 excess 条
		res, err := p.db.Exec(
			`DELETE FROM ai_memory_embeddings WHERE id IN (
				SELECT id FROM ai_memory_embeddings WHERE kind = $1
				ORDER BY priority ASC, created_at ASC LIMIT $2
			)`, kind, excess)
		if err != nil {
			slog.Warn("记忆容量裁剪失败", "kind", kind, "err", err)
			continue
		}
		if n, _ := res.RowsAffected(); n > 0 {
			totalDeleted += n
		}
	}
	if totalDeleted > 0 {
		slog.Info("记忆容量裁剪完成", "删除总数", totalDeleted, "上限", maxPerKind)
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

// --- Sreyun rules CRUD ---

type sreyunRule struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Priority    int             `json:"priority"`
	Enabled     bool            `json:"enabled"`
	Config      json.RawMessage `json:"config"`
	CreatedAt   string          `json:"created_at,omitempty"`
	UpdatedAt   string          `json:"updated_at,omitempty"`
}

func (p *pgStore) listSreyunRules() ([]sreyunRule, error) {
	rows, err := p.db.Query(`SELECT id,name,description,priority,enabled,config,created_at,updated_at FROM hermes_rules ORDER BY priority DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sreyunRule
	for rows.Next() {
		var r sreyunRule
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

func (p *pgStore) upsertSreyunRule(r sreyunRule) (int64, error) {
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

func (p *pgStore) deleteSreyunRule(id int64) error {
	_, err := p.db.Exec(`DELETE FROM hermes_rules WHERE id=$1`, id)
	return err
}

// --- Sreyun templates CRUD ---

type sreyunTemplate struct {
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

func (p *pgStore) listSreyunTemplates(activeOnly bool) ([]sreyunTemplate, error) {
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
	var out []sreyunTemplate
	for rows.Next() {
		var t sreyunTemplate
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

func (p *pgStore) upsertSreyunTemplate(t sreyunTemplate) (int64, error) {
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

func (p *pgStore) deleteSreyunTemplate(id int64) error {
	_, err := p.db.Exec(`DELETE FROM hermes_templates WHERE id=$1`, id)
	return err
}

// --- Sreyun sessions ---

func (p *pgStore) loadSreyunSession(id int64) ([]byte, error) {
	var raw []byte
	err := p.db.QueryRow(`SELECT messages FROM hermes_sessions WHERE id=$1`, id).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return raw, err
}

func (p *pgStore) saveSreyunSession(id int64, messages []byte, incidentID int64) (int64, error) {
	if id > 0 {
		_, err := p.db.Exec(`UPDATE hermes_sessions SET messages=$1,updated_at=NOW() WHERE id=$2`, messages, id)
		return id, err
	}
	var newID int64
	err := p.db.QueryRow(`INSERT INTO hermes_sessions(incident_id,messages) VALUES($1,$2) RETURNING id`, incidentID, messages).Scan(&newID)
	return newID, err
}

func (p *pgStore) listSreyunSessions(limit int) ([]map[string]any, error) {
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
		title, summary, count := sreyunSessionDigest(raw)
		m["title"] = title
		m["summary"] = summary
		m["msg_count"] = count
		out = append(out, m)
	}
	return out, rows.Err()
}

// sreyunSessionDigest 从会话 messages(JSON) 提取标题（首条 user 内容）、摘要（末条内容）与消息条数。
func sreyunSessionDigest(raw []byte) (title, summary string, count int) {
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
			title = sreyunTrunc(m["content"], 24)
			break
		}
	}
	if title == "" {
		title = "新会话"
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.TrimSpace(msgs[i]["content"]) != "" {
			summary = sreyunTrunc(msgs[i]["content"], 40)
			break
		}
	}
	return title, summary, count
}

// sreyunTrunc 按 Unicode 字符（rune）截断字符串，避免中文被切成半个字符。
func sreyunTrunc(s string, n int) string {
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

// ============================================================================
// Hardware / NetFlow PG methods
// ============================================================================

func (p *pgStore) upsertHardwareSnapshot(hostID string, snap shared.HardwareSnapshot) {
	raw, _ := json.Marshal(snap)
	_, err := p.db.Exec(`
		INSERT INTO hardware_snapshot(host_id, target_name, target_url, snapshot, health, updated_at)
		VALUES($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (host_id, target_name) DO UPDATE
		SET snapshot=$4, health=$5, target_url=$3, updated_at=NOW()`,
		hostID, snap.TargetName, snap.TargetURL, raw, snap.Health)
	if err != nil {
		slog.Warn("Upsert 硬件快照失败", "host", hostID, "target", snap.TargetName, "err", err)
	}
}

// getHardwareSnapshotDecoded returns the stored snapshot for one target,
// decoded back into the wire struct so it can be diffed against a fresh one.
func (p *pgStore) getHardwareSnapshotDecoded(hostID, targetName string) (shared.HardwareSnapshot, bool) {
	var raw []byte
	err := p.db.QueryRow(`SELECT snapshot FROM hardware_snapshot WHERE host_id=$1 AND target_name=$2`,
		hostID, targetName).Scan(&raw)
	if err != nil {
		return shared.HardwareSnapshot{}, false
	}
	var snap shared.HardwareSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return shared.HardwareSnapshot{}, false
	}
	return snap, true
}

func (p *pgStore) insertHardwareChange(hostID, targetName string, c hwChange) {
	_, err := p.db.Exec(`
		INSERT INTO hardware_changes(host_id, target_name, kind, component, action, old_value, new_value)
		VALUES($1,$2,$3,$4,$5,$6,$7)`,
		hostID, targetName, c.Kind, c.Component, c.Action, c.Old, c.New)
	if err != nil {
		slog.Warn("写入硬件变更记录失败", "host", hostID, "component", c.Component, "err", err)
	}
}

// getHardwareChanges returns asset change history, newest first.
func (p *pgStore) getHardwareChanges(hostID, target string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT target_name, kind, component, action, COALESCE(old_value,''), COALESCE(new_value,''), created_at
	      FROM hardware_changes WHERE host_id=$1`
	args := []any{hostID}
	if target != "" {
		q += ` AND target_name=$2`
		args = append(args, target)
	}
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT %d`, limit)

	rows, err := p.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var tn, kind, comp, action, oldV, newV string
		var ts time.Time
		if err := rows.Scan(&tn, &kind, &comp, &action, &oldV, &newV, &ts); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"target_name": tn, "kind": kind, "component": comp, "action": action,
			"old_value": oldV, "new_value": newV, "created_at": ts,
		})
	}
	return out, rows.Err()
}

func (p *pgStore) insertHardwareEvent(hostID, targetName, eventType, severity, message string) {
	_, err := p.db.Exec(`
		INSERT INTO hardware_events(host_id, target_name, event_type, severity, message)
		VALUES($1, $2, $3, $4, $5)`,
		hostID, targetName, eventType, severity, message)
	if err != nil {
		slog.Warn("插入硬件事件失败", "err", err)
	}
}

// getHardwareEvents returns recorded hardware state transitions for a host,
// newest first. Optionally narrowed to one Redfish target.
func (p *pgStore) getHardwareEvents(hostID, target string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT target_name, event_type, severity, message, created_at
	      FROM hardware_events WHERE host_id=$1`
	args := []any{hostID}
	if target != "" {
		q += ` AND target_name=$2`
		args = append(args, target)
	}
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT %d`, limit)

	rows, err := p.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var targetName, eventType, severity, message sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&targetName, &eventType, &severity, &message, &createdAt); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"target_name": targetName.String,
			"event_type":  eventType.String,
			"severity":    severity.String,
			"message":     message.String,
			"created_at":  createdAt,
		})
	}
	return out, rows.Err()
}

func (p *pgStore) getHardwareSnapshots(hostID string) ([]map[string]any, error) {
	rows, err := p.db.Query(`
		SELECT target_name, target_url, snapshot, health, updated_at
		FROM hardware_snapshot WHERE host_id=$1 ORDER BY updated_at DESC`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var targetName, targetURL, health string
		var snapshot json.RawMessage
		var updatedAt time.Time
		if err := rows.Scan(&targetName, &targetURL, &snapshot, &health, &updatedAt); err != nil {
			continue
		}
		var snapData any
		json.Unmarshal(snapshot, &snapData)
		results = append(results, map[string]any{
			"target_name": targetName,
			"target_url":  targetURL,
			"health":      health,
			"snapshot":    snapData,
			"updated_at":  updatedAt,
		})
	}
	if results == nil {
		results = []map[string]any{}
	}
	return results, rows.Err()
}

func (p *pgStore) deleteHardwareSnapshot(hostID, targetName string) {
	_, err := p.db.Exec(`DELETE FROM hardware_snapshot WHERE host_id=$1 AND target_name=$2`, hostID, targetName)
	if err != nil {
		slog.Warn("删除硬件快照失败", "host", hostID, "target", targetName, "err", err)
	}
	// 级联清理关联的事件与变更记录
	_, _ = p.db.Exec(`DELETE FROM hardware_events WHERE host_id=$1 AND target_name=$2`, hostID, targetName)
	_, _ = p.db.Exec(`DELETE FROM hardware_changes WHERE host_id=$1 AND target_name=$2`, hostID, targetName)
}

// findHardwareTargetByURL returns the target_name of an existing snapshot that
// matches the given target_url, or "" if none found. Used to detect renames:
// when a user changes the config.json "name" field for the same physical device
// (same URL), we need to migrate the old record instead of creating a new one.
func (p *pgStore) findHardwareTargetByURL(hostID, targetURL string) string {
	if targetURL == "" {
		return ""
	}
	var name string
	err := p.db.QueryRow(`SELECT target_name FROM hardware_snapshot WHERE host_id=$1 AND target_url=$2`,
		hostID, targetURL).Scan(&name)
	if err != nil {
		return ""
	}
	return name
}

// renameHardwareTarget migrates all data from oldName to newName for a given
// host, covering snapshots, events, and changes. Called when the agent's
// config.json "name" field is changed for the same physical device (matched
// by target_url). Without this migration the old record lingers forever.
func (p *pgStore) renameHardwareTarget(hostID, oldName, newName string) {
	if oldName == newName || oldName == "" || newName == "" {
		return
	}
	slog.Info("硬件目标改名迁移", "host", hostID, "old", oldName, "new", newName)
	// 1. Delete the new name if it already exists (will be re-inserted by upsert)
	_, _ = p.db.Exec(`DELETE FROM hardware_snapshot WHERE host_id=$1 AND target_name=$2`, hostID, newName)
	// 2. Rename old → new in snapshots (preserves history)
	_, _ = p.db.Exec(`UPDATE hardware_snapshot SET target_name=$3 WHERE host_id=$1 AND target_name=$2`,
		hostID, oldName, newName)
	// 3. Rename in events (state transitions timeline)
	_, _ = p.db.Exec(`UPDATE hardware_events SET target_name=$3 WHERE host_id=$1 AND target_name=$2`,
		hostID, oldName, newName)
	// 4. Rename in changes (asset change history)
	_, _ = p.db.Exec(`UPDATE hardware_changes SET target_name=$3 WHERE host_id=$1 AND target_name=$2`,
		hostID, oldName, newName)
}

// purgeOtherHardwareByURL deletes sibling snapshots that share the same
// target_url but a different target_name — cleans any historical rename orphans.
func (p *pgStore) purgeOtherHardwareByURL(hostID, keepName, targetURL string) {
	if targetURL == "" || keepName == "" {
		return
	}
	res, err := p.db.Exec(`
		DELETE FROM hardware_snapshot
		WHERE host_id=$1 AND target_url=$2 AND target_name<>$3`, hostID, targetURL, keepName)
	if err != nil {
		slog.Warn("清理硬件同 URL 重复行失败", "host", hostID, "url", targetURL, "err", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Info("已清理硬件同 URL 旧名残留", "host", hostID, "url", targetURL, "keep", keepName, "removed", n)
	}
}

// ============================================================================
// Hyper-V 虚拟机清单 PG methods（结构与 hardware_* 同构）
// ============================================================================

// upsertHyperVInventory overwrites a host's guest inventory (whole list as JSONB).
func (p *pgStore) upsertHyperVInventory(hostID, hostName string, guests []shared.HyperVGuest) {
	if guests == nil {
		guests = []shared.HyperVGuest{}
	}
	raw, _ := json.Marshal(guests)
	_, err := p.db.Exec(`
		INSERT INTO hyperv_inventory(host_id, host_name, guest_count, snapshot, updated_at)
		VALUES($1, $2, $3, $4, NOW())
		ON CONFLICT (host_id) DO UPDATE
		SET host_name=$2, guest_count=$3, snapshot=$4, updated_at=NOW()`,
		hostID, hostName, len(guests), raw)
	if err != nil {
		slog.Warn("Upsert Hyper-V 清单失败", "host", hostID, "err", err)
	}
}

// getHyperVInventoryDecoded returns a host's stored guests decoded back into wire
// structs, so a fresh report can be diffed against it for change detection.
func (p *pgStore) getHyperVInventoryDecoded(hostID string) ([]shared.HyperVGuest, bool) {
	var raw []byte
	err := p.db.QueryRow(`SELECT snapshot FROM hyperv_inventory WHERE host_id=$1`, hostID).Scan(&raw)
	if err != nil {
		return nil, false
	}
	var guests []shared.HyperVGuest
	if err := json.Unmarshal(raw, &guests); err != nil {
		return nil, false
	}
	return guests, true
}

// hypervInventoryRow is one host's inventory as returned to the frontend/AI.
func (p *pgStore) scanHyperVRow(hostID, hostName string, snapshot json.RawMessage, guestCount int, updatedAt time.Time) map[string]any {
	var guests any
	json.Unmarshal(snapshot, &guests)
	if guests == nil {
		guests = []any{}
	}
	return map[string]any{
		"host_id":     hostID,
		"host_name":   hostName,
		"guest_count": guestCount,
		"guests":      guests,
		"updated_at":  updatedAt,
	}
}

// getHyperVInventory returns one host's inventory (nil,false when none).
func (p *pgStore) getHyperVInventory(hostID string) (map[string]any, bool) {
	var hostName string
	var snapshot json.RawMessage
	var guestCount int
	var updatedAt time.Time
	err := p.db.QueryRow(`SELECT host_name, guest_count, snapshot, updated_at
		FROM hyperv_inventory WHERE host_id=$1`, hostID).Scan(&hostName, &guestCount, &snapshot, &updatedAt)
	if err != nil {
		return nil, false
	}
	return p.scanHyperVRow(hostID, hostName, snapshot, guestCount, updatedAt), true
}

// getAllHyperVInventories returns every host's inventory, most-recently-updated first.
func (p *pgStore) getAllHyperVInventories() ([]map[string]any, error) {
	rows, err := p.db.Query(`SELECT host_id, host_name, guest_count, snapshot, updated_at
		FROM hyperv_inventory ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var hostID, hostName string
		var guestCount int
		var snapshot json.RawMessage
		var updatedAt time.Time
		if err := rows.Scan(&hostID, &hostName, &guestCount, &snapshot, &updatedAt); err != nil {
			continue
		}
		out = append(out, p.scanHyperVRow(hostID, hostName, snapshot, guestCount, updatedAt))
	}
	return out, rows.Err()
}

const hypervEventsPerHostCap = 500

func (p *pgStore) insertHyperVEvent(hostID, vmName, vmID, kind, severity, message string) {
	_, err := p.db.Exec(`
		INSERT INTO hyperv_events(host_id, vm_name, vm_id, kind, severity, message)
		VALUES($1, $2, $3, $4, $5, $6)`,
		hostID, vmName, vmID, kind, severity, message)
	if err != nil {
		slog.Warn("插入 Hyper-V 事件失败", "host", hostID, "vm", vmName, "err", err)
		return
	}
	// 保留每宿主最近 N 条，防止事件表无界增长。事件只在 VM 增删/状态跳变时写入，
	// 频率很低，故随插入裁剪的开销可忽略。
	_, _ = p.db.Exec(`DELETE FROM hyperv_events WHERE host_id=$1 AND id NOT IN (
		SELECT id FROM hyperv_events WHERE host_id=$1 ORDER BY created_at DESC, id DESC LIMIT $2)`,
		hostID, hypervEventsPerHostCap)
}

// getHyperVEvents returns a host's VM change/state events, newest first.
func (p *pgStore) getHyperVEvents(hostID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := p.db.Query(fmt.Sprintf(`SELECT vm_name, vm_id, kind, severity, message, created_at
		FROM hyperv_events WHERE host_id=$1 ORDER BY created_at DESC LIMIT %d`, limit), hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var vmName, vmID, kind, severity, message sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&vmName, &vmID, &kind, &severity, &message, &createdAt); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"vm_name":    vmName.String,
			"vm_id":      vmID.String,
			"kind":       kind.String,
			"severity":   severity.String,
			"message":    message.String,
			"created_at": createdAt,
		})
	}
	return out, rows.Err()
}

func (p *pgStore) deleteHyperVInventory(hostID string) {
	if _, err := p.db.Exec(`DELETE FROM hyperv_inventory WHERE host_id=$1`, hostID); err != nil {
		slog.Warn("删除 Hyper-V 清单失败", "host", hostID, "err", err)
	}
	_, _ = p.db.Exec(`DELETE FROM hyperv_events WHERE host_id=$1`, hostID)
}

// insertFlowRecords 批量写入 Flow 明细：多行 VALUES 分批（每批 500 行），把上万条的逐行往返
// 压缩到几十次，大幅缩短占用连接的时长（原来逐行 Exec 会拿住一条连接做上万次往返，饿死连接池）。
func (p *pgStore) insertFlowRecords(hostID, source string, flows []shared.FlowRecord) {
	if len(flows) == 0 {
		return
	}
	const cols = 11
	const batch = 500
	base := `INSERT INTO flow_records(host_id, source, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, first_seen, last_seen) VALUES `
	for start := 0; start < len(flows); start += batch {
		end := start + batch
		if end > len(flows) {
			end = len(flows)
		}
		chunk := flows[start:end]
		var sb strings.Builder
		sb.WriteString(base)
		args := make([]any, 0, len(chunk)*cols)
		for i, f := range chunk {
			if i > 0 {
				sb.WriteByte(',')
			}
			b := i * cols
			fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
				b+1, b+2, b+3, b+4, b+5, b+6, b+7, b+8, b+9, b+10, b+11)
			args = append(args, hostID, source, f.SrcIP, f.DstIP, f.SrcPort, f.DstPort, f.Protocol,
				f.Bytes, f.Packets, time.Unix(f.FirstSeen, 0), time.Unix(f.LastSeen, 0))
		}
		if _, err := p.db.Exec(sb.String(), args...); err != nil {
			slog.Warn("批量写入 flow_records 失败", "host", hostID, "rows", len(chunk), "err", err)
			return
		}
	}
}

// flowSummaryDims whitelists the columns callers may GROUP BY.
// 直接把 dimension 拼进 SQL 是注入面，必须白名单。
var flowSummaryDims = map[string]string{
	"src_ip":   "src_ip::text",
	"dst_ip":   "dst_ip::text",
	"src_port": "src_port::text",
	"dst_port": "dst_port::text",
	"protocol": "protocol::text",
	"source":   "source",
}

// getFlowSummary returns Top-N traffic grouped by one dimension, from PG.
//
// 为什么不查 VM：VM 里现在只存**基数可控的聚合**（总量/对端 Top-N/服务端口 Top-N），
// 不再有 src_port 这类高基数 label —— 那是压垮时序库的东西。按任意维度做
// Top-N 聚合本来就是关系库的活，明细在 PG 里永久保留，查它才对。
func (p *pgStore) getFlowSummary(hostID, dimension string, from, to int64, limit int) ([]map[string]any, error) {
	col, ok := flowSummaryDims[dimension]
	if !ok {
		col = flowSummaryDims["dst_ip"]
		dimension = "dst_ip"
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := fmt.Sprintf(`
		SELECT %s AS k, SUM(bytes)::bigint AS b, SUM(packets)::bigint AS pk, COUNT(*)::bigint AS n
		FROM flow_records
		WHERE host_id=$1 AND created_at >= to_timestamp($2) AND created_at <= to_timestamp($3)
		GROUP BY 1 ORDER BY b DESC LIMIT %d`, col, limit)

	rows, err := p.db.Query(q, hostID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var k sql.NullString
		var b, pk, n int64
		if err := rows.Scan(&k, &b, &pk, &n); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"key": k.String, "bytes": b, "packets": pk, "flows": n,
		})
	}
	return out, rows.Err()
}

// getFlowIPHistory returns bucketed traffic curves and drill-down dimensions for
// one ranked source/destination IP. Aggregation stays in PostgreSQL because the
// high-cardinality flow identity is intentionally not written to VictoriaMetrics.
func (p *pgStore) getFlowIPHistory(hostID, dimension, ip string, from, to int64) (map[string]any, error) {
	if dimension != "src_ip" && dimension != "dst_ip" {
		dimension = "dst_ip"
	}
	if net.ParseIP(stripMask(ip)) == nil {
		return nil, fmt.Errorf("invalid IP")
	}
	ip = stripMask(ip)
	span := to - from
	bucket := int64(60)
	switch {
	case span > 14*86400:
		bucket = 3600
	case span > 7*86400:
		bucket = 1800
	case span > 48*3600:
		bucket = 600
	case span > 12*3600:
		bucket = 300
	}
	peerCol := "src_ip"
	if dimension == "src_ip" {
		peerCol = "dst_ip"
	}
	q := fmt.Sprintf(`
		SELECT (floor(extract(epoch FROM created_at)/$5)*$5)::bigint AS ts,
		       COALESCE(SUM(bytes),0)::bigint,
		       COALESCE(SUM(packets),0)::bigint,
		       COUNT(*)::bigint,
		       COUNT(DISTINCT %s)::bigint
		FROM flow_records
		WHERE host_id=$1 AND created_at >= to_timestamp($2) AND created_at <= to_timestamp($3)
		  AND %s = $4::inet
		GROUP BY 1 ORDER BY 1`, peerCol, dimension)
	rows, err := p.db.Query(q, hostID, from, to, ip, bucket)
	if err != nil {
		return nil, err
	}
	points := []map[string]any{}
	for rows.Next() {
		var ts, bytes, packets, flows, peers int64
		if err := rows.Scan(&ts, &bytes, &packets, &flows, &peers); err != nil {
			continue
		}
		avg := float64(0)
		if packets > 0 {
			avg = float64(bytes) / float64(packets)
		}
		points = append(points, map[string]any{
			"timestamp": ts, "bytes": bytes, "packets": packets,
			"flows": flows, "peers": peers, "avg_packet_bytes": avg,
		})
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	top := func(expr string, limit int) ([]map[string]any, error) {
		query := fmt.Sprintf(`
			SELECT COALESCE(%s::text,''), COALESCE(SUM(bytes),0)::bigint,
			       COALESCE(SUM(packets),0)::bigint, COUNT(*)::bigint
			FROM flow_records
			WHERE host_id=$1 AND created_at >= to_timestamp($2) AND created_at <= to_timestamp($3)
			  AND %s = $4::inet
			GROUP BY 1 ORDER BY 2 DESC LIMIT %d`, expr, dimension, limit)
		rs, err := p.db.Query(query, hostID, from, to, ip)
		if err != nil {
			return nil, err
		}
		defer rs.Close()
		out := []map[string]any{}
		for rs.Next() {
			var key string
			var bytes, packets, flows int64
			if err := rs.Scan(&key, &bytes, &packets, &flows); err == nil && key != "" {
				out = append(out, map[string]any{
					"key": stripMask(key), "bytes": bytes, "packets": packets, "flows": flows,
				})
			}
		}
		return out, rs.Err()
	}

	peers, err := top(peerCol, 12)
	if err != nil {
		return nil, err
	}
	protocols, err := top("protocol", 8)
	if err != nil {
		return nil, err
	}
	srcPorts, err := top("src_port", 10)
	if err != nil {
		return nil, err
	}
	dstPorts, err := top("dst_port", 10)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"points": points, "peers": peers, "protocols": protocols,
		"src_ports": srcPorts, "dst_ports": dstPorts, "bucket_sec": bucket,
	}, nil
}

// ipIsh 判断字符串是否是可用于 inet 比较的 IP 或 CIDR（否则不加该条件，避免 SQL 报错）。
func ipIsh(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

// protoToNum 把协议名/数字转为 IP 协议号（flow_records.protocol 存的是数字）。
func protoToNum(s string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "tcp":
		return 6, true
	case "udp":
		return 17, true
	case "icmp":
		return 1, true
	}
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n, true
	}
	return 0, false
}

func (p *pgStore) getFlowRecords(hostID, filter string, limit int) ([]map[string]any, error) {
	return p.getFlowRecordsRange(hostID, filter, limit, 0, 0)
}

// getFlowRecordsRange is getFlowRecords with an optional created_at window.
// from/to <= 0 preserves the historical all-time behavior used by AI tools.
func (p *pgStore) getFlowRecordsRange(hostID, filter string, limit int, from, to int64) ([]map[string]any, error) {
	query := `SELECT source, src_ip::text, dst_ip::text, src_port, dst_port, protocol, bytes, packets, first_seen, last_seen
		FROM flow_records WHERE host_id=$1`
	args := []any{hostID}
	argIdx := 2
	if from > 0 && to > from {
		query += fmt.Sprintf(` AND created_at >= to_timestamp($%d) AND created_at <= to_timestamp($%d)`, argIdx, argIdx+1)
		args = append(args, from, to)
		argIdx += 2
	}

	// 筛选：支持 src_ip:/dst_ip:（精确 IP 或 CIDR，用 inet 包含 <<=）、src_port:/dst_port:/port:、
	// proto:（tcp/udp/icmp 或数字）、以及无前缀裸值（IP/CIDR→源或目的；纯数字→端口源或目的）。
	// 关键修复：原来 IP 用 `::text =` 精确字符串比较，CIDR 与松散写法永远不匹配、且未识别的写法静默不加
	// WHERE（返回全量，"筛选无效"）。现改为 inet 包含 + 裸值/别名兜底 + 去空格。
	if f := strings.TrimSpace(filter); f != "" {
		col, val := "", f
		if i := strings.Index(f, ":"); i > 0 {
			col = strings.ToLower(strings.TrimSpace(f[:i]))
			val = strings.TrimSpace(f[i+1:])
		}
		// 列名白名单：column 始终来自下方 switch 的硬编码字面量，绝不来自用户输入（val 才是用户值，
		// 已全部走 $N 参数化）。这里再做一次白名单断言作为纵深防御，防止未来误传入非受控列名。
		allowedFlowCols := map[string]bool{"src_ip": true, "dst_ip": true, "src_port": true, "dst_port": true}
		ipCond := func(column string) {
			if allowedFlowCols[column] && ipIsh(val) {
				query += fmt.Sprintf(` AND %s <<= $%d::inet`, column, argIdx)
				args = append(args, val)
				argIdx++
			}
		}
		portCond := func(column string) {
			if !allowedFlowCols[column] {
				return
			}
			if n, err := strconv.Atoi(val); err == nil {
				query += fmt.Sprintf(` AND %s = $%d`, column, argIdx)
				args = append(args, n)
				argIdx++
			}
		}
		switch col {
		case "src_ip", "src":
			ipCond("src_ip")
		case "dst_ip", "dst":
			ipCond("dst_ip")
		case "src_port":
			portCond("src_port")
		case "dst_port":
			portCond("dst_port")
		case "port":
			if n, err := strconv.Atoi(val); err == nil {
				query += fmt.Sprintf(` AND (src_port = $%d OR dst_port = $%d)`, argIdx, argIdx)
				args = append(args, n)
				argIdx++
			}
		case "proto", "protocol":
			if n, ok := protoToNum(val); ok {
				query += fmt.Sprintf(` AND protocol = $%d`, argIdx)
				args = append(args, n)
				argIdx++
			}
		case "ip", "": // 无前缀裸值兜底
			if ipIsh(val) {
				query += fmt.Sprintf(` AND (src_ip <<= $%d::inet OR dst_ip <<= $%d::inet)`, argIdx, argIdx)
				args = append(args, val)
				argIdx++
			} else if n, err := strconv.Atoi(val); err == nil {
				query += fmt.Sprintf(` AND (src_port = $%d OR dst_port = $%d)`, argIdx, argIdx)
				args = append(args, n)
				argIdx++
			}
		}
	}

	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, argIdx)
	args = append(args, limit)

	rows, err := p.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var source, srcIP, dstIP string
		var srcPort, dstPort, protocol int
		var bytes, packets int64
		var firstSeen, lastSeen time.Time
		if err := rows.Scan(&source, &srcIP, &dstIP, &srcPort, &dstPort, &protocol,
			&bytes, &packets, &firstSeen, &lastSeen); err != nil {
			continue
		}
		results = append(results, map[string]any{
			"source":     source,
			"src_ip":     srcIP,
			"dst_ip":     dstIP,
			"src_port":   srcPort,
			"dst_port":   dstPort,
			"protocol":   protocol,
			"bytes":      bytes,
			"packets":    packets,
			"first_seen": firstSeen,
			"last_seen":  lastSeen,
		})
	}
	if results == nil {
		results = []map[string]any{}
	}
	return results, rows.Err()
}

// getFlowHosts returns the host_ids that actually have flow records in the window,
// ranked by total bytes desc (packets as tiebreak so acct-off hosts with 0 bytes but
// real connections still rank). Powers the "流量页只列有流量的主机" filter —— GROUP BY
// host_id inherently excludes hosts with no traffic at all.
func (p *pgStore) getFlowHosts(from, to int64, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := p.db.Query(`
		SELECT host_id, SUM(bytes)::bigint AS b, SUM(packets)::bigint AS pk, COUNT(*)::bigint AS n
		FROM flow_records
		WHERE created_at >= to_timestamp($1) AND created_at <= to_timestamp($2)
		GROUP BY host_id
		ORDER BY b DESC, pk DESC LIMIT $3`, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var hid string
		var b, pk, n int64
		if err := rows.Scan(&hid, &b, &pk, &n); err != nil {
			continue
		}
		out = append(out, map[string]any{"host_id": hid, "bytes": b, "packets": pk, "flows": n})
	}
	return out, rows.Err()
}

// ============================================================================
// SNMP PG methods
// ============================================================================

// upsertSNMPSnapshot 按 (host_id, device_name) upsert 一台设备的最新快照。
func (p *pgStore) upsertSNMPSnapshot(hostID string, snap shared.SNMPSnapshot) {
	raw, _ := json.Marshal(snap)
	_, err := p.db.Exec(`
		INSERT INTO snmp_snapshot(host_id, device_name, device_ip, snapshot, reachable, updated_at)
		VALUES($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (host_id, device_name) DO UPDATE
		SET snapshot=$4, device_ip=$3, reachable=$5, updated_at=NOW()`,
		hostID, snap.TargetName, snap.TargetIP, raw, snap.Reachable)
	if err != nil {
		slog.Warn("Upsert SNMP 快照失败", "host", hostID, "device", snap.TargetName, "err", err)
	}
}

// findSNMPDeviceByIP returns the device_name of an existing snapshot that matches
// the given device_ip (most recently updated wins), or "" if none. Used to detect
// renames: config.json "name" changed but IP (the connection identity) is unchanged.
func (p *pgStore) findSNMPDeviceByIP(hostID, deviceIP string) string {
	if deviceIP == "" {
		return ""
	}
	var name string
	err := p.db.QueryRow(`
		SELECT device_name FROM snmp_snapshot
		WHERE host_id=$1 AND device_ip=$2
		ORDER BY updated_at DESC LIMIT 1`, hostID, deviceIP).Scan(&name)
	if err != nil {
		return ""
	}
	return name
}

// renameSNMPDevice migrates a device row from oldName to newName for one agent
// host. Mirrors renameHardwareTarget: without this, changing config "name" for the
// same IP leaves the old row forever and the UI shows duplicates.
func (p *pgStore) renameSNMPDevice(hostID, oldName, newName string) {
	if oldName == newName || oldName == "" || newName == "" {
		return
	}
	slog.Info("SNMP 设备改名迁移", "host", hostID, "old", oldName, "new", newName)
	_, _ = p.db.Exec(`DELETE FROM snmp_snapshot WHERE host_id=$1 AND device_name=$2`, hostID, newName)
	_, _ = p.db.Exec(`UPDATE snmp_snapshot SET device_name=$3 WHERE host_id=$1 AND device_name=$2`,
		hostID, oldName, newName)
}

// purgeOtherSNMPByIP deletes sibling rows that share the same device_ip but a
// different device_name. Cleans historical rename orphans in one shot after the
// canonical name has been upserted.
func (p *pgStore) purgeOtherSNMPByIP(hostID, keepName, deviceIP string) {
	if deviceIP == "" || keepName == "" {
		return
	}
	res, err := p.db.Exec(`
		DELETE FROM snmp_snapshot
		WHERE host_id=$1 AND device_ip=$2 AND device_name<>$3`, hostID, deviceIP, keepName)
	if err != nil {
		slog.Warn("清理 SNMP 同 IP 重复行失败", "host", hostID, "ip", deviceIP, "err", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Info("已清理 SNMP 同 IP 旧名残留", "host", hostID, "ip", deviceIP, "keep", keepName, "removed", n)
	}
}

// deleteSNMPSnapshot removes one device's snapshot for an agent host.
func (p *pgStore) deleteSNMPSnapshot(hostID, deviceName string) {
	if hostID == "" || deviceName == "" {
		return
	}
	_, err := p.db.Exec(`DELETE FROM snmp_snapshot WHERE host_id=$1 AND device_name=$2`, hostID, deviceName)
	if err != nil {
		slog.Warn("删除 SNMP 快照失败", "host", hostID, "device", deviceName, "err", err)
	}
}

// getSNMPSnapshots 返回一台主机（agent）下所有被轮询设备的最新快照。
func (p *pgStore) getSNMPSnapshots(hostID string) ([]map[string]any, error) {
	rows, err := p.db.Query(`
		SELECT device_name, device_ip, snapshot, reachable, updated_at
		FROM snmp_snapshot WHERE host_id=$1 ORDER BY updated_at DESC`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []map[string]any{}
	for rows.Next() {
		var deviceName, deviceIP string
		var snapshot json.RawMessage
		var reachable bool
		var updatedAt time.Time
		if err := rows.Scan(&deviceName, &deviceIP, &snapshot, &reachable, &updatedAt); err != nil {
			continue
		}
		var snapData any
		json.Unmarshal(snapshot, &snapData)
		results = append(results, map[string]any{
			"device_name": deviceName,
			"device_ip":   deviceIP,
			"reachable":   reachable,
			"snapshot":    snapData,
			"updated_at":  updatedAt,
		})
	}
	return results, rows.Err()
}

// insertSNMPTrap 追加写一条 trap 事件。
func (p *pgStore) insertSNMPTrap(hostID string, ev shared.SNMPTrapEvent) {
	vb, _ := json.Marshal(ev.Varbinds)
	_, err := p.db.Exec(`
		INSERT INTO snmp_traps(host_id, source_ip, version, trap_oid, severity, uptime_sec, varbinds, received_at)
		VALUES($1,$2,$3,$4,$5,$6,$7, to_timestamp($8))`,
		hostID, ev.SourceIP, ev.Version, ev.TrapOID, ev.Severity, ev.UptimeSec, vb, ev.Timestamp)
	if err != nil {
		slog.Warn("写入 SNMP Trap 失败", "host", hostID, "trap_oid", ev.TrapOID, "err", err)
	}
}

// getSNMPTraps 返回一台主机最近的 trap 事件（倒序）。
func (p *pgStore) getSNMPTraps(hostID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := p.db.Query(`
		SELECT source_ip, version, trap_oid, severity, COALESCE(uptime_sec,0), varbinds, received_at
		FROM snmp_traps WHERE host_id=$1 ORDER BY received_at DESC LIMIT $2`, hostID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []map[string]any{}
	for rows.Next() {
		var sourceIP, version, trapOID, severity string
		var uptime float64
		var varbinds json.RawMessage
		var receivedAt time.Time
		if err := rows.Scan(&sourceIP, &version, &trapOID, &severity, &uptime, &varbinds, &receivedAt); err != nil {
			continue
		}
		var vbData any
		json.Unmarshal(varbinds, &vbData)
		results = append(results, map[string]any{
			"source_ip":   sourceIP,
			"version":     version,
			"trap_oid":    trapOID,
			"severity":    severity,
			"uptime_sec":  uptime,
			"varbinds":    vbData,
			"received_at": receivedAt,
		})
	}
	return results, rows.Err()
}

// getSNMPHosts returns the hosts (agents) that have SNMP network-device data —
// polled device snapshots and/or received traps — ranked by device count desc.
// Powers the "网络设备页只列有网络设备的主机" filter. UNION 让只有 trap 的主机也能被选到，
// 其 traps 才在该页可见；DISTINCT 收敛 trap 侧，避免全表计数。
func (p *pgStore) getSNMPHosts() ([]map[string]any, error) {
	rows, err := p.db.Query(`
		SELECT host_id, SUM(dev)::bigint AS devices, SUM(reach)::bigint AS reachable, SUM(trp)::bigint AS traps
		FROM (
			SELECT host_id, 1 AS dev, (CASE WHEN reachable THEN 1 ELSE 0 END) AS reach, 0 AS trp FROM snmp_snapshot
			UNION ALL
			SELECT DISTINCT host_id, 0 AS dev, 0 AS reach, 1 AS trp FROM snmp_traps
		) u
		GROUP BY host_id
		ORDER BY devices DESC, traps DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var hid string
		var devices, reachable, traps int64
		if err := rows.Scan(&hid, &devices, &reachable, &traps); err != nil {
			continue
		}
		out = append(out, map[string]any{"host_id": hid, "devices": devices, "reachable": reachable, "traps": traps})
	}
	return out, rows.Err()
}

// insertContentAudit 批量写入明文 HTTP 内容审计事件。
// insertContentAudit 批量写入。labels 与 evs 一一对应（敏感命中标签，逗号分隔；空=未命中）。
func (p *pgStore) insertContentAudit(hostID string, evs []shared.ContentAuditEvent, labels []string) {
	if len(evs) == 0 {
		return
	}
	tx, err := p.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO content_audit
		(host_id, src_ip, dst_ip, dst_port, method, host, path, ctype, body,
		 status, resp_ctype, resp_body, req_truncated, resp_truncated, sensitive, observed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`)
	if err != nil {
		return
	}
	defer stmt.Close()
	for i, e := range evs {
		var sens string
		if i < len(labels) {
			sens = labels[i]
		}
		_, _ = stmt.Exec(hostID, e.SrcIP, e.DstIP, int(e.DstPort), e.Method, e.Host, e.Path, e.CType, e.Body,
			e.Status, e.RespCType, e.RespBody, e.ReqTruncated, e.RespTruncated, sens, time.Unix(e.Ts, 0))
	}
	_ = tx.Commit()
}

// getContentAudit 查询内容审计记录，最新在前。filter 支持 "host:", "src_ip:", "path:", "kw:"(body/path 模糊)。
func (p *pgStore) getContentAudit(hostID, filter string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT src_ip, dst_ip, dst_port, method, host, path, ctype, body,
	             COALESCE(status,0), COALESCE(resp_ctype,''), COALESCE(resp_body,''),
	             COALESCE(req_truncated,false), COALESCE(resp_truncated,false),
	             COALESCE(sensitive,''), observed_at
	      FROM content_audit WHERE host_id=$1`
	args := []any{hostID}
	idx := 2
	if filter != "" {
		if col, val, ok := strings.Cut(filter, ":"); ok && val != "" {
			switch col {
			case "src_ip", "dst_ip", "host", "method":
				q += fmt.Sprintf(" AND %s = $%d", col, idx)
				args = append(args, val)
				idx++
			case "kw": // body/resp_body/path/host 模糊匹配
				q += fmt.Sprintf(" AND (body ILIKE $%d OR resp_body ILIKE $%d OR path ILIKE $%d OR host ILIKE $%d)", idx, idx, idx, idx)
				args = append(args, "%"+val+"%")
				idx++
			case "sens": // 只看命中敏感的
				q += " AND sensitive <> '' AND sensitive IS NOT NULL"
			}
		}
	}
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", idx)
	args = append(args, limit)
	rows, err := p.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var srcIP, dstIP, method, host, path, ctype, body, respCType, respBody, sensitive string
		var dstPort, status int
		var reqTrunc, respTrunc bool
		var observedAt time.Time
		if err := rows.Scan(&srcIP, &dstIP, &dstPort, &method, &host, &path, &ctype, &body,
			&status, &respCType, &respBody, &reqTrunc, &respTrunc, &sensitive, &observedAt); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"src_ip": srcIP, "dst_ip": dstIP, "dst_port": dstPort, "method": method,
			"host": host, "path": path, "ctype": ctype, "body": body,
			"status": status, "resp_ctype": respCType, "resp_body": respBody,
			"req_truncated": reqTrunc, "resp_truncated": respTrunc, "sensitive": sensitive, "observed_at": observedAt,
		})
	}
	return out, rows.Err()
}

// getContentAuditHosts 返回有内容审计记录的主机，按最近记录降序 + 条数。供"只列有数据的主机"过滤。
func (p *pgStore) getContentAuditHosts() ([]map[string]any, error) {
	rows, err := p.db.Query(`
		SELECT host_id, COUNT(*)::bigint AS events, EXTRACT(EPOCH FROM MAX(created_at))::bigint AS last
		FROM content_audit
		GROUP BY host_id
		ORDER BY last DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var hid string
		var events, last int64
		if err := rows.Scan(&hid, &events, &last); err != nil {
			continue
		}
		out = append(out, map[string]any{"host_id": hid, "events": events, "last": last})
	}
	return out, rows.Err()
}

// cleanupContentAudit 按保留天数清理内容审计（默认 30 天）。审计数据高敏感，不宜永久留存。
func (p *pgStore) cleanupContentAudit(retainDays int) {
	if retainDays <= 0 {
		retainDays = 30
	}
	// 参数化天数（make_interval），杜绝任何字符串拼接进 SQL —— 即便 retainDays 未来来源变化也安全。
	_, _ = p.db.Exec("DELETE FROM content_audit WHERE created_at < NOW() - make_interval(days => $1)", retainDays)
}

// cleanupFlowRecords deletes flow records older than 7 days (called periodically).
func (p *pgStore) cleanupFlowRecords() {
	// Flow 明细现在**永久保留**（分区表，归档靠 DROP/DETACH 某个月的分区）。
	// 这里只维护分区，不再删数据 —— 原先的 7 天 DELETE 与"永久存储"直接冲突。
	p.ensureFlowPartitions()
}
