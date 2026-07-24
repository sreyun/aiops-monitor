package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RetentionConfig controls daily cleanup windows (days). Zero = use defaults.
type RetentionConfig struct {
	AuditDays        int `json:"audit_days,omitempty"`         // audit_log + events
	AlertHistoryDays int `json:"alert_history_days,omitempty"` // alert_history
	ContentAuditDays int `json:"content_audit_days,omitempty"` // content_audit
	MemoryDays       int `json:"memory_days,omitempty"`        // soft age for memory cleanup
	NetFlowMonths    int `json:"netflow_months,omitempty"`     // drop partitions older than N months
	AICallDays       int `json:"ai_call_days,omitempty"`       // AI 调用与人工反馈观测
}

func (r RetentionConfig) withDefaults() RetentionConfig {
	if r.AuditDays <= 0 {
		r.AuditDays = 180
	}
	if r.AlertHistoryDays <= 0 {
		r.AlertHistoryDays = 90
	}
	if r.ContentAuditDays <= 0 {
		r.ContentAuditDays = 30
	}
	if r.MemoryDays <= 0 {
		r.MemoryDays = 365
	}
	if r.NetFlowMonths <= 0 {
		r.NetFlowMonths = 12
	}
	if r.AICallDays <= 0 {
		r.AICallDays = 365
	}
	return r
}

// BackupConfig schedules PostgreSQL dumps via pg_dump.
type BackupConfig struct {
	Enabled     bool   `json:"enabled"`
	DailyAt     string `json:"daily_at,omitempty"` // HH:MM local, default 02:30
	RetainCount int    `json:"retain_count,omitempty"`
	Dir         string `json:"dir,omitempty"` // override AIOPS_BACKUP_DIR
}

func (b BackupConfig) withDefaults() BackupConfig {
	if b.DailyAt == "" {
		b.DailyAt = "02:30"
	}
	if b.RetainCount <= 0 {
		b.RetainCount = 14
	}
	return b
}

type BackupMeta struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	Operator  string `json:"operator"`
	Path      string `json:"path"`
	Note      string `json:"note,omitempty"`
}

func (cs *ConfigStore) Retention() RetentionConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.Retention.withDefaults()
}

func (cs *ConfigStore) BackupCfg() BackupConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.Backup.withDefaults()
}

func (cs *ConfigStore) CmdPolicy() CmdPolicyConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	p := cs.cfg.CmdPolicy
	if p.Mode == "" {
		p.Mode = "strict"
	}
	return p
}

func (cs *ConfigStore) SetRetention(r RetentionConfig) error {
	cs.mu.Lock()
	cs.cfg.Retention = r.withDefaults()
	cs.mu.Unlock()
	return cs.save()
}

func (cs *ConfigStore) SetBackupCfg(b BackupConfig) error {
	cs.mu.Lock()
	cs.cfg.Backup = b.withDefaults()
	cs.mu.Unlock()
	return cs.save()
}

func (cs *ConfigStore) SetCmdPolicy(p CmdPolicyConfig) error {
	if p.Mode == "" {
		p.Mode = "strict"
	}
	cs.mu.Lock()
	cs.cfg.CmdPolicy = p
	cs.mu.Unlock()
	return cs.save()
}

func backupDir(cfg BackupConfig) string {
	if d := strings.TrimSpace(cfg.Dir); d != "" {
		return d
	}
	if d := strings.TrimSpace(os.Getenv("AIOPS_BACKUP_DIR")); d != "" {
		return d
	}
	return filepath.Join(".", "backups")
}

func (s *Server) listBackups() ([]BackupMeta, error) {
	if s.pg == nil {
		return nil, fmt.Errorf("PostgreSQL 未启用")
	}
	rows, err := s.pg.db.Query(`SELECT id, created_at, size_bytes, sha256, operator, path, note FROM backup_meta ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		// table may not exist yet on very old failed migrate — fall back to filesystem
		return s.listBackupsFS()
	}
	defer rows.Close()
	var out []BackupMeta
	for rows.Next() {
		var m BackupMeta
		if err := rows.Scan(&m.ID, &m.CreatedAt, &m.SizeBytes, &m.SHA256, &m.Operator, &m.Path, &m.Note); err != nil {
			return out, err
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return s.listBackupsFS()
	}
	return out, rows.Err()
}

func (s *Server) listBackupsFS() ([]BackupMeta, error) {
	dir := backupDir(s.cfg.BackupCfg())
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []BackupMeta
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".dump") {
			continue
		}
		info, _ := e.Info()
		var sz int64
		var mod time.Time
		if info != nil {
			sz = info.Size()
			mod = info.ModTime()
		}
		out = append(out, BackupMeta{
			ID: e.Name(), CreatedAt: mod.Unix(), SizeBytes: sz,
			Path: filepath.Join(dir, e.Name()), Operator: "fs",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

func (s *Server) createPGBackup(operator, note string) (BackupMeta, error) {
	dsn := strings.TrimSpace(os.Getenv("AIOPS_POSTGRES_DSN"))
	if dsn == "" {
		s.cfg.mu.RLock()
		dsn = s.cfg.cfg.PostgresDSN
		s.cfg.mu.RUnlock()
	}
	if dsn == "" {
		return BackupMeta{}, fmt.Errorf("未配置 PostgreSQL DSN")
	}
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return BackupMeta{}, fmt.Errorf("未找到 pg_dump，请安装 PostgreSQL 客户端工具")
	}
	cfg := s.cfg.BackupCfg()
	dir := backupDir(cfg)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return BackupMeta{}, err
	}
	id := fmt.Sprintf("aiops-pg-%s.dump", time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, id)
	cmd := exec.Command("pg_dump", "--format=custom", "--file", path, dsn)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		return BackupMeta{}, fmt.Errorf("pg_dump 失败: %v (%s)", err, truncateRunes(string(out), 400))
	}
	sum, size, err := fileSHA256(path)
	if err != nil {
		return BackupMeta{}, err
	}
	meta := BackupMeta{
		ID: id, CreatedAt: time.Now().Unix(), SizeBytes: size,
		SHA256: sum, Operator: operator, Path: path, Note: note,
	}
	if s.pg != nil {
		_, _ = s.pg.db.Exec(`INSERT INTO backup_meta(id, created_at, size_bytes, sha256, operator, path, note)
			VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO UPDATE SET size_bytes=EXCLUDED.size_bytes, sha256=EXCLUDED.sha256`,
			meta.ID, meta.CreatedAt, meta.SizeBytes, meta.SHA256, meta.Operator, meta.Path, meta.Note)
	}
	s.pruneBackups(cfg.RetainCount)
	return meta, nil
}

func (s *Server) pruneBackups(retain int) {
	if retain <= 0 {
		return
	}
	list, err := s.listBackups()
	if err != nil || len(list) <= retain {
		return
	}
	for _, m := range list[retain:] {
		_ = os.Remove(m.Path)
		if s.pg != nil {
			_, _ = s.pg.db.Exec(`DELETE FROM backup_meta WHERE id=$1`, m.ID)
		}
	}
}

func (s *Server) restorePGBackup(id, operator string) error {
	list, err := s.listBackups()
	if err != nil {
		return err
	}
	var meta *BackupMeta
	for i := range list {
		if list[i].ID == id {
			meta = &list[i]
			break
		}
	}
	if meta == nil {
		return fmt.Errorf("备份不存在")
	}
	if _, err := os.Stat(meta.Path); err != nil {
		return fmt.Errorf("备份文件缺失: %w", err)
	}
	if _, err := exec.LookPath("pg_restore"); err != nil {
		return fmt.Errorf("未找到 pg_restore，请安装 PostgreSQL 客户端工具")
	}
	dsn := strings.TrimSpace(os.Getenv("AIOPS_POSTGRES_DSN"))
	if dsn == "" {
		s.cfg.mu.RLock()
		dsn = s.cfg.cfg.PostgresDSN
		s.cfg.mu.RUnlock()
	}
	cmd := exec.Command("pg_restore", "--clean", "--if-exists", "--no-owner", "--dbname", dsn, meta.Path)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		// pg_restore often returns non-zero with warnings; treat as soft unless empty
		msg := string(out)
		if !strings.Contains(msg, "ERROR") && len(msg) < 8 {
			slog.Warn("pg_restore finished with warnings", "err", err, "out", truncateRunes(msg, 400))
		} else if strings.Contains(strings.ToUpper(msg), "ERROR") {
			return fmt.Errorf("pg_restore 失败: %v (%s)", err, truncateRunes(msg, 600))
		}
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: operator, Message: "从备份还原 PostgreSQL：" + id})
	return nil
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func maskEmail(s string) string {
	s = strings.TrimSpace(s)
	at := strings.IndexByte(s, '@')
	if at <= 1 {
		return "***"
	}
	return s[:1] + "***" + s[at:]
}

func maskPhone(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 7 {
		return "***"
	}
	return s[:3] + "****" + s[len(s)-4:]
}

var backupSchedOnce sync.Once

func (s *Server) startBackupScheduler() {
	backupSchedOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			var lastDay string
			for range ticker.C {
				cfg := s.cfg.BackupCfg()
				if !cfg.Enabled {
					continue
				}
				now := time.Now()
				want := cfg.DailyAt
				hhmm := now.Format("15:04")
				day := now.Format("2006-01-02")
				if hhmm == want && lastDay != day {
					lastDay = day
					if _, err := s.createPGBackup("scheduler", "scheduled"); err != nil {
						slog.Error("scheduled PG backup failed", "err", err)
					} else {
						slog.Info("scheduled PG backup ok")
					}
				}
			}
		}()
	})
}

// --- HTTP handlers (admin only via routeAllowed on /api/v1/admin/*) ---

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	list, err := s.listBackups()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	meta, err := s.createPGBackup(s.actorName(r), "manual")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.actorName(r), IP: s.clientIP(r), Message: "创建 PostgreSQL 备份：" + meta.ID})
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	list, err := s.listBackups()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	for _, m := range list {
		if m.ID != id {
			continue
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+m.ID+"\"")
		http.ServeFile(w, r, m.Path)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "备份不存在"})
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Confirm string `json:"confirm"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Confirm != "RESTORE" && in.Confirm != id {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请在 confirm 字段填写 RESTORE 或备份 ID 以二次确认"})
		return
	}
	if err := s.restorePGBackup(id, s.actorName(r)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "hint": "还原已执行，建议重启服务端进程以重新加载内存状态"})
}

func (s *Server) handleGetRetention(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Retention())
}

func (s *Server) handleSetRetention(w http.ResponseWriter, r *http.Request) {
	var in RetentionConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if err := s.cfg.SetRetention(in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "retention": s.cfg.Retention()})
}

func (s *Server) handleGetBackupCfg(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.BackupCfg())
}

func (s *Server) handleSetBackupCfg(w http.ResponseWriter, r *http.Request) {
	var in BackupConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if err := s.cfg.SetBackupCfg(in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "backup": s.cfg.BackupCfg()})
}

func (s *Server) handleGetCmdPolicy(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.CmdPolicy())
}

func (s *Server) handleSetCmdPolicy(w http.ResponseWriter, r *http.Request) {
	var in CmdPolicyConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if err := s.cfg.SetCmdPolicy(in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cmd_policy": s.cfg.CmdPolicy()})
}
