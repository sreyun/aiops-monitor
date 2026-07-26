package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aiops-monitor/cmd/server/sqltoolkit"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// SlowSQLReport is one collection+advice run for a MySQL connection.
type SlowSQLReport struct {
	ID             string              `json:"id"`
	ConnectionID   string              `json:"connection_id"`
	ConnectionName string              `json:"connection_name,omitempty"`
	Trigger        string              `json:"trigger,omitempty"` // manual | schedule
	Source         string              `json:"source"`            // performance_schema
	Status         string              `json:"status"`            // running | completed | failed
	Error          string              `json:"error,omitempty"`
	StartedAt      int64               `json:"started_at"`
	FinishedAt     int64               `json:"finished_at,omitempty"`
	ItemCount      int                 `json:"item_count"`
	Items          []SlowSQLItem       `json:"items"`
	Trend          *SlowSQLDigestTrend `json:"trend,omitempty"`
}

// SlowSQLDigestTrend compares digests against the previous completed report.
type SlowSQLDigestTrend struct {
	PreviousReportID string   `json:"previous_report_id,omitempty"`
	NewDigests       int      `json:"new_digests"`
	GoneDigests      int      `json:"gone_digests"`
	Worsened         int      `json:"worsened"`
	Improved         int      `json:"improved"`
	SamplesNew       []string `json:"samples_new,omitempty"`
	SamplesWorse     []string `json:"samples_worse,omitempty"`
}

// SlowSQLItem is one digest with metrics and rule-engine advice.
type SlowSQLItem struct {
	Schema       string                 `json:"schema,omitempty"`
	Digest       string                 `json:"digest,omitempty"`
	SQL          string                 `json:"sql"`
	CountStar    int64                  `json:"count_star"`
	SumLatencyMs float64                `json:"sum_latency_ms"`
	AvgLatencyMs float64                `json:"avg_latency_ms"`
	MaxLatencyMs float64                `json:"max_latency_ms"`
	FirstSeen    string                 `json:"first_seen,omitempty"`
	LastSeen     string                 `json:"last_seen,omitempty"`
	Score        int                    `json:"score"`
	Findings     []sqltoolkit.Finding   `json:"findings,omitempty"`
	Suggestions  []sqltoolkit.Finding   `json:"suggestions,omitempty"`
	IndexHints   []sqltoolkit.IndexHint `json:"index_hints,omitempty"`
	RewrittenSQL string                 `json:"rewritten_sql,omitempty"`
	ExplainUsed  bool                   `json:"explain_used"`
	MetadataUsed bool                   `json:"metadata_used"`
	AnalyzeError string                 `json:"analyze_error,omitempty"`
	// Trend: new|worse|better|same (vs previous completed report).
	Trend string `json:"trend,omitempty"`
}

type slowDigestRow struct {
	Schema       string
	Digest       string
	SQL          string
	CountStar    int64
	SumLatencyMs float64
	AvgLatencyMs float64
	MaxLatencyMs float64
	FirstSeen    string
	LastSeen     string
}

type slowSQLManager struct {
	mu       sync.Mutex
	dir      string
	latest   map[string]*SlowSQLReport // connectionID -> latest
	history  []*SlowSQLReport          // ring, newest first
	lastRun  map[string]int64          // schedule bookkeeping
	inflight map[string]bool
}

const slowSQLHistoryCap = 40

func newSlowSQLManager(dir string) *slowSQLManager {
	m := &slowSQLManager{
		dir:      dir,
		latest:   map[string]*SlowSQLReport{},
		history:  make([]*SlowSQLReport, 0, 16),
		lastRun:  map[string]int64{},
		inflight: map[string]bool{},
	}
	m.load()
	return m
}

func (m *slowSQLManager) pathLatest(connID string) string {
	return filepath.Join(m.dir, "latest-"+sanitizeFilePart(connID)+".json")
}

func (m *slowSQLManager) pathHistory() string {
	return filepath.Join(m.dir, "history.json")
}

func sanitizeFilePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "unknown"
	}
	return out
}

func (m *slowSQLManager) load() {
	_ = os.MkdirAll(m.dir, 0o750)
	entries, _ := os.ReadDir(m.dir)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "latest-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(m.dir, name))
		if err != nil {
			continue
		}
		var rep SlowSQLReport
		if json.Unmarshal(b, &rep) != nil || rep.ConnectionID == "" {
			continue
		}
		cp := rep
		m.latest[rep.ConnectionID] = &cp
	}
	if b, err := os.ReadFile(m.pathHistory()); err == nil {
		var hist []*SlowSQLReport
		if json.Unmarshal(b, &hist) == nil {
			m.history = hist
		}
	}
}

func (m *slowSQLManager) saveLatestLocked(rep *SlowSQLReport) {
	if rep == nil || rep.ConnectionID == "" {
		return
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(m.dir, 0o750)
	_ = os.WriteFile(m.pathLatest(rep.ConnectionID), b, 0o640)
}

func (m *slowSQLManager) saveHistoryLocked() {
	if len(m.history) > slowSQLHistoryCap {
		m.history = m.history[:slowSQLHistoryCap]
	}
	b, err := json.MarshalIndent(m.history, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(m.dir, 0o750)
	_ = os.WriteFile(m.pathHistory(), b, 0o640)
}

func (m *slowSQLManager) begin(connID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inflight[connID] {
		return false
	}
	m.inflight[connID] = true
	return true
}

func (m *slowSQLManager) end(connID string) {
	m.mu.Lock()
	delete(m.inflight, connID)
	m.mu.Unlock()
}

func (m *slowSQLManager) store(rep *SlowSQLReport) {
	if rep == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *rep
	if cp.Items == nil {
		cp.Items = []SlowSQLItem{}
	}
	// Avoid stacking duplicate "running" placeholders in the history ring.
	if cp.Status == "running" {
		m.latest[cp.ConnectionID] = &cp
		m.saveLatestLocked(&cp)
		return
	}
	m.latest[cp.ConnectionID] = &cp
	hist := cp
	m.history = append([]*SlowSQLReport{&hist}, m.history...)
	m.saveLatestLocked(&cp)
	m.saveHistoryLocked()
}

func (m *slowSQLManager) previousCompleted(connID, excludeID string) *SlowSQLReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, h := range m.history {
		if h == nil || h.ConnectionID != connID || h.Status != "completed" || h.ID == excludeID {
			continue
		}
		cp := *h
		return &cp
	}
	if cur := m.latest[connID]; cur != nil && cur.Status == "completed" && cur.ID != excludeID {
		cp := *cur
		return &cp
	}
	return nil
}

func slowDigestKey(it SlowSQLItem) string {
	d := strings.TrimSpace(it.Digest)
	if d != "" {
		return strings.ToLower(d)
	}
	return strings.ToLower(strings.TrimSpace(it.Schema)) + "|" + strings.ToLower(strings.TrimSpace(it.SQL))
}

func attachSlowSQLTrend(prev, cur *SlowSQLReport) {
	if cur == nil || cur.Status != "completed" {
		return
	}
	if prev == nil || prev.Status != "completed" {
		return
	}
	trend := &SlowSQLDigestTrend{PreviousReportID: prev.ID}
	prevMap := map[string]SlowSQLItem{}
	for _, it := range prev.Items {
		prevMap[slowDigestKey(it)] = it
	}
	curMap := map[string]bool{}
	for i := range cur.Items {
		it := &cur.Items[i]
		k := slowDigestKey(*it)
		curMap[k] = true
		old, ok := prevMap[k]
		if !ok {
			it.Trend = "new"
			trend.NewDigests++
			if len(trend.SamplesNew) < 5 {
				trend.SamplesNew = append(trend.SamplesNew, truncateRun(it.SQL, 80))
			}
			continue
		}
		// Significant latency change: >20% and at least 20ms absolute.
		delta := it.AvgLatencyMs - old.AvgLatencyMs
		thresh := old.AvgLatencyMs * 0.2
		if thresh < 20 {
			thresh = 20
		}
		switch {
		case delta >= thresh:
			it.Trend = "worse"
			trend.Worsened++
			if len(trend.SamplesWorse) < 5 {
				trend.SamplesWorse = append(trend.SamplesWorse, truncateRun(it.SQL, 80))
			}
		case delta <= -thresh:
			it.Trend = "better"
			trend.Improved++
		default:
			it.Trend = "same"
		}
	}
	for k := range prevMap {
		if !curMap[k] {
			trend.GoneDigests++
		}
	}
	cur.Trend = trend
}

func (m *slowSQLManager) getLatest(connID string) *SlowSQLReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	rep := m.latest[connID]
	if rep == nil {
		return nil
	}
	cp := *rep
	return &cp
}

func (m *slowSQLManager) listLatest() []*SlowSQLReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*SlowSQLReport, 0, len(m.latest))
	for _, r := range m.latest {
		if r == nil {
			continue
		}
		cp := *r
		cp.Items = nil // overview omits heavy items
		out = append(out, &cp)
	}
	return out
}

func mysqlDSNSlow(c MySQLConnection) string {
	// Same as mysqlDSN but longer read timeout for digest scans.
	user := c.User
	if user == "" {
		user = "root"
	}
	port := c.Port
	if port <= 0 {
		port = 3306
	}
	cfg := mysqldriver.NewConfig()
	cfg.User = user
	cfg.Passwd = c.Password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", c.Host, port)
	cfg.DBName = c.Database
	cfg.ParseTime = true
	cfg.Timeout = 8 * time.Second
	cfg.ReadTimeout = 60 * time.Second
	cfg.WriteTimeout = 10 * time.Second
	cfg.InterpolateParams = true
	cfg.Params = map[string]string{}
	if c.TLS != "" {
		cfg.TLSConfig = c.TLS
	}
	if c.Params != "" {
		if extra, err := url.ParseQuery(c.Params); err == nil {
			for k, vs := range extra {
				if len(vs) > 0 {
					cfg.Params[k] = vs[0]
				}
			}
		}
	}
	return cfg.FormatDSN()
}

func mysqlOpenSlow(c MySQLConnection) (*sql.DB, error) {
	db, err := sql.Open("mysql", mysqlDSNSlow(c))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(3 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func picosecondsToMs(ps float64) float64 {
	return ps / 1e9
}

func formatSQLTime(v any) string {
	switch t := v.(type) {
	case time.Time:
		if t.IsZero() {
			return ""
		}
		return t.Format(time.RFC3339)
	case []byte:
		return string(t)
	case string:
		return t
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func humanizeSlowSQLErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "performance_schema") && (strings.Contains(low, "doesn't exist") || strings.Contains(low, "unknown table") || strings.Contains(low, "1146")):
		return "无法读取 performance_schema.events_statements_summary_by_digest。请确认已启用 performance_schema，且账号具备 SELECT 权限。示例：SET GLOBAL performance_schema=ON; GRANT SELECT ON performance_schema.* TO 'user'@'%';"
	case strings.Contains(low, "access denied") || strings.Contains(low, "1045") || strings.Contains(low, "1142"):
		return "账号权限不足，无法查询 performance_schema。请授予 SELECT ON performance_schema.*（建议只读账号）。"
	default:
		return "慢 SQL 采集失败：" + truncateRun(msg, 240)
	}
}

// buildSlowDigestQuery builds the P_S digest query. Exported logic kept testable via helpers.
func buildSlowDigestQuery(exclude []string, minAvgMs float64, topN int) (string, []any) {
	if topN <= 0 {
		topN = 30
	}
	if topN > 100 {
		topN = 100
	}
	if minAvgMs < 0 {
		minAvgMs = 0
	}
	ex := exclude
	if len(ex) == 0 {
		ex = defaultSlowSQLExcludeSchemas()
	}
	ph := make([]string, 0, len(ex))
	args := make([]any, 0, len(ex)+2)
	for _, s := range ex {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		ph = append(ph, "?")
		args = append(args, s)
	}
	// AVG_TIMER_WAIT is picoseconds; convert threshold ms → ps.
	args = append(args, minAvgMs*1e9, topN)
	whereEx := "1=1"
	if len(ph) > 0 {
		whereEx = "(SCHEMA_NAME IS NULL OR SCHEMA_NAME NOT IN (" + strings.Join(ph, ",") + "))"
	}
	q := `
SELECT SCHEMA_NAME, DIGEST, DIGEST_TEXT, COUNT_STAR,
       SUM_TIMER_WAIT, AVG_TIMER_WAIT, MAX_TIMER_WAIT,
       FIRST_SEEN, LAST_SEEN
FROM performance_schema.events_statements_summary_by_digest
WHERE DIGEST_TEXT IS NOT NULL
  AND ` + whereEx + `
  AND AVG_TIMER_WAIT >= ?
ORDER BY SUM_TIMER_WAIT DESC
LIMIT ?`
	return strings.TrimSpace(q), args
}

func mysqlCollectSlowDigests(c MySQLConnection, cfg *SlowSQLMonitorConfig) ([]slowDigestRow, error) {
	cfg = cfg.withDefaults()
	db, err := mysqlOpenSlow(c)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	q, args := buildSlowDigestQuery(cfg.ExcludeSchemas, cfg.MinAvgLatencyMs, cfg.TopN)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("%s", humanizeSlowSQLErr(err))
	}
	defer rows.Close()

	out := make([]slowDigestRow, 0, cfg.TopN)
	for rows.Next() {
		var schema, digest sql.NullString
		var digText string
		var count int64
		var sumPS, avgPS, maxPS float64
		var first, last any
		if err := rows.Scan(&schema, &digest, &digText, &count, &sumPS, &avgPS, &maxPS, &first, &last); err != nil {
			continue
		}
		digText = strings.TrimSpace(digText)
		if digText == "" {
			continue
		}
		out = append(out, slowDigestRow{
			Schema:       strings.TrimSpace(schema.String),
			Digest:       strings.TrimSpace(digest.String),
			SQL:          digText,
			CountStar:    count,
			SumLatencyMs: picosecondsToMs(sumPS),
			AvgLatencyMs: picosecondsToMs(avgPS),
			MaxLatencyMs: picosecondsToMs(maxPS),
			FirstSeen:    formatSQLTime(first),
			LastSeen:     formatSQLTime(last),
		})
	}
	return out, rows.Err()
}

func dialectForConn(c MySQLConnection) sqltoolkit.Dialect {
	switch c.VersionHint {
	case "mysql57":
		return sqltoolkit.DialectMySQL57
	case "mysql80":
		return sqltoolkit.DialectMySQL80
	default:
		return sqltoolkit.DialectMySQL80
	}
}

func analyzeSlowDigest(c MySQLConnection, row slowDigestRow) SlowSQLItem {
	item := SlowSQLItem{
		Schema:       row.Schema,
		Digest:       row.Digest,
		SQL:          row.SQL,
		CountStar:    row.CountStar,
		SumLatencyMs: row.SumLatencyMs,
		AvgLatencyMs: row.AvgLatencyMs,
		MaxLatencyMs: row.MaxLatencyMs,
		FirstSeen:    row.FirstSeen,
		LastSeen:     row.LastSeen,
		Score:        100,
	}
	d := dialectForConn(c)
	in := sqltoolkit.AnalyzeInput{SQL: row.SQL, Dialect: d}

	kw := sqltoolkit.FirstKeyword(row.SQL)
	canExplain := kw == "select" || kw == "with"
	if canExplain && !sqltoolkit.ForbiddenWrite(row.SQL) {
		shape := sqltoolkit.ExtractQueryShape(row.SQL)
		if shape != nil && shape.ParseOK {
			if meta, err := mysqlFetchMetadataInSchema(c, row.Schema, shape.TableNames()); err == nil {
				in.Meta = meta
			}
		}
		if expl, err := mysqlExplainInSchema(c, row.Schema, row.SQL); err == nil {
			if a, ok := expl["analysis"].(*sqltoolkit.ExplainAnalysis); ok {
				in.Explain = a
			}
		} else if item.AnalyzeError == "" {
			// Digest text often contains ? placeholders — EXPLAIN failure is expected.
			item.AnalyzeError = ""
		}
	}

	res := sqltoolkit.Analyze(in)
	item.Score = res.Score
	item.Findings = res.Findings
	item.Suggestions = res.Suggestions
	item.IndexHints = res.IndexHints
	item.RewrittenSQL = res.RewrittenSQL
	item.ExplainUsed = res.ExplainUsed
	item.MetadataUsed = res.MetadataUsed
	return item
}

func (s *Server) runSlowSQLCollect(connID, trigger string) (*SlowSQLReport, error) {
	c, ok := s.cfg.GetMySQLConnection(connID)
	if !ok || !c.Enabled {
		return nil, fmt.Errorf("connection not found or disabled")
	}
	if driverOf(c) == "postgres" {
		return nil, fmt.Errorf("慢 SQL 检查仅支持 MySQL")
	}
	if s.sqlSlow == nil {
		return nil, fmt.Errorf("slow sql manager not ready")
	}
	if !s.sqlSlow.begin(connID) {
		return nil, fmt.Errorf("该连接的慢 SQL 检查正在进行中")
	}
	defer s.sqlSlow.end(connID)

	cfg := c.SlowSQL.withDefaults()
	rep := &SlowSQLReport{
		ID:             "ss-" + randomHex(6),
		ConnectionID:   c.ID,
		ConnectionName: c.Name,
		Trigger:        trigger,
		Source:         "performance_schema",
		Status:         "running",
		StartedAt:      time.Now().Unix(),
		Items:          []SlowSQLItem{},
	}
	s.sqlSlow.store(rep)

	rows, err := mysqlCollectSlowDigests(c, cfg)
	if err != nil {
		rep.Status = "failed"
		rep.Error = err.Error()
		rep.FinishedAt = time.Now().Unix()
		s.sqlSlow.store(rep)
		return rep, err
	}

	items := make([]SlowSQLItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, analyzeSlowDigest(c, row))
	}
	rep.Items = items
	rep.ItemCount = len(items)
	rep.Status = "completed"
	rep.FinishedAt = time.Now().Unix()
	if prev := s.sqlSlow.previousCompleted(c.ID, rep.ID); prev != nil {
		attachSlowSQLTrend(prev, rep)
	}
	s.sqlSlow.store(rep)
	s.notifySlowSQLReport(c, rep)
	return rep, nil
}

func (s *Server) startSlowSQLScheduler() {
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		// small delay so boot storm settles
		time.Sleep(8 * time.Second)
		for range t.C {
			s.tickSlowSQLSchedule()
		}
	}()
}

func (s *Server) tickSlowSQLSchedule() {
	if s.sqlSlow == nil {
		return
	}
	now := time.Now()
	for _, c := range s.cfg.ListMySQLConnections() {
		if !c.Enabled || c.SlowSQL == nil || !c.SlowSQL.Enabled {
			continue
		}
		sc := c.SlowSQL.withDefaults().Schedule
		if !slowSQLScheduleDue(sc, s.sqlSlow, c.ID, now) {
			continue
		}
		connID := c.ID
		go func() {
			if _, err := s.runSlowSQLCollect(connID, "schedule"); err != nil {
				slog.Info("slow sql scheduled run", "connection_id", connID, "err", err.Error())
			}
		}()
	}
}

func slowSQLScheduleDue(sc *PlaybookSchedule, m *slowSQLManager, connID string, now time.Time) bool {
	if sc == nil || !sc.Enabled || m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inflight[connID] {
		return false
	}
	key := connID
	last := m.lastRun[key]
	if last == 0 {
		if rep := m.latest[connID]; rep != nil && rep.FinishedAt > 0 {
			last = rep.FinishedAt
			m.lastRun[key] = last
		}
	}
	switch sc.Kind {
	case "interval":
		min := sc.IntervalMin
		if min < 15 {
			min = 15
		}
		if last > 0 && now.Unix()-last < int64(min)*60 {
			return false
		}
		m.lastRun[key] = now.Unix()
		return true
	case "daily":
		mins, ok := parseHHMM(sc.At)
		if !ok || now.Hour()*60+now.Minute() != mins {
			return false
		}
		day := now.Format("2006-01-02")
		dayKey := key + ":" + day
		if m.lastRun[dayKey] > 0 {
			return false
		}
		m.lastRun[dayKey] = now.Unix()
		m.lastRun[key] = now.Unix()
		return true
	case "weekly":
		mins, ok := parseHHMM(sc.At)
		if !ok || int(now.Weekday()) != sc.Weekday || now.Hour()*60+now.Minute() != mins {
			return false
		}
		y, w := now.ISOWeek()
		weekKey := fmt.Sprintf("%s:%d-W%02d", key, y, w)
		if m.lastRun[weekKey] > 0 {
			return false
		}
		m.lastRun[weekKey] = now.Unix()
		m.lastRun[key] = now.Unix()
		return true
	default:
		return false
	}
}
