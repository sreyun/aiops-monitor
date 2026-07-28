package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var (
	auditChainMu   sync.Mutex
	auditChainPrev string
	auditChainSeq  int64
)

func auditChainSecret() []byte {
	if k := loadSecretKey(); len(k) > 0 {
		return k
	}
	// deterministic fallback so chain still works without AIOPS_SECRET_KEY
	sum := sha256.Sum256([]byte("aiops-audit-chain-default"))
	return sum[:]
}

func nextAuditChain(payload []byte) (contentHash, prevHash string, seq int64) {
	auditChainMu.Lock()
	defer auditChainMu.Unlock()
	prevHash = auditChainPrev
	auditChainSeq++
	seq = auditChainSeq
	mac := hmac.New(sha256.New, auditChainSecret())
	mac.Write([]byte(prevHash))
	mac.Write([]byte{0})
	mac.Write(payload)
	mac.Write([]byte{0})
	mac.Write([]byte(strconv.FormatInt(seq, 10)))
	contentHash = hex.EncodeToString(mac.Sum(nil))
	auditChainPrev = contentHash
	return contentHash, prevHash, seq
}

func (p *pgStore) hydrateAuditChainTip() {
	if p == nil || p.db == nil {
		return
	}
	var hash string
	var seq int64
	err := p.db.QueryRow(`SELECT COALESCE(content_hash,''), COALESCE(chain_seq,0) FROM audit_log
WHERE COALESCE(content_hash,'') <> '' ORDER BY id DESC LIMIT 1`).Scan(&hash, &seq)
	if err != nil {
		_ = p.db.QueryRow(`SELECT COALESCE(content_hash,''), COALESCE(chain_seq,0) FROM audit_log_p
WHERE COALESCE(content_hash,'') <> '' ORDER BY id DESC LIMIT 1`).Scan(&hash, &seq)
	}
	if hash != "" {
		auditChainMu.Lock()
		auditChainPrev = hash
		if seq > auditChainSeq {
			auditChainSeq = seq
		}
		auditChainMu.Unlock()
	}
}

func (p *pgStore) verifyAuditChain(limit int) (ok bool, checked int, brokenAt int64, detail string) {
	if p == nil || p.db == nil {
		return false, 0, 0, "pg unavailable"
	}
	if limit <= 0 {
		limit = 500
	}
	rows, err := p.db.Query(`
SELECT id, ts, data, COALESCE(content_hash,''), COALESCE(prev_hash,''), COALESCE(chain_seq,0)
FROM audit_log WHERE COALESCE(content_hash,'') <> ''
ORDER BY chain_seq ASC, id ASC LIMIT $1`, limit)
	if err != nil {
		return false, 0, 0, err.Error()
	}
	defer rows.Close()
	prev := ""
	for rows.Next() {
		var id, ts, seq int64
		var data []byte
		var ch, ph string
		if rows.Scan(&id, &ts, &data, &ch, &ph, &seq) != nil {
			continue
		}
		checked++
		if checked > 1 && ph != prev {
			return false, checked, id, fmt.Sprintf("prev_hash mismatch at id=%d", id)
		}
		mac := hmac.New(sha256.New, auditChainSecret())
		mac.Write([]byte(ph))
		mac.Write([]byte{0})
		mac.Write(data)
		mac.Write([]byte{0})
		mac.Write([]byte(strconv.FormatInt(seq, 10)))
		expect := hex.EncodeToString(mac.Sum(nil))
		if ch != expect {
			return false, checked, id, fmt.Sprintf("content_hash mismatch at id=%d", id)
		}
		prev = ch
	}
	return true, checked, 0, "ok"
}

func (s *Server) handleAuditVerifyChain(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if s.pg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "pg unavailable"})
		return
	}
	ok, checked, broken, detail := s.pg.verifyAuditChain(limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": ok, "checked": checked, "broken_at": broken, "detail": detail,
		"ts": time.Now().Unix(),
	})
}

func (s *Server) handleSecurityRewrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	if !secretEncryptionEnabled() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "未配置 AIOPS_SECRET_KEY"})
		return
	}
	ai := s.cfg.AIConfig()
	n := 0
	rew := func(field *string) {
		if field == nil || *field == "" {
			return
		}
		before := *field
		*field = rewrapSecretIfNeeded(*field)
		if *field != before {
			n++
		}
	}
	rew(&ai.APIKey)
	rew(&ai.EmbedAPIKey)
	rew(&ai.RerankAPIKey)
	rew(&ai.MCPToken)
	rew(&ai.WeKnoraAPIKey)
	rew(&ai.SpeechAPIKey)
	if err := s.cfg.SetAIConfig(ai); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	slog.Info("security rewrap completed", "fields", n)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "rewrapped_fields": n, "keys": secretKeyStatus()})
}

func (s *Server) handleSecurityKeyStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, secretKeyStatus())
}

// appendAuditChained writes legacy + partitioned audit with hash chain.
func (p *pgStore) appendAuditChained(e LogEntry) {
	raw, err := json.Marshal(e)
	if err != nil {
		return
	}
	ch, ph, seq := nextAuditChain(raw)
	ts := e.Timestamp
	if ts <= 0 {
		ts = time.Now().Unix()
	}
	if _, err := p.db.Exec(`INSERT INTO audit_log(ts,data,content_hash,prev_hash,chain_seq) VALUES($1,$2,$3,$4,$5)`,
		ts, raw, ch, ph, seq); err != nil {
		slog.Warn("PG 写审计日志失败", "err", err)
	}
	if _, err := p.db.Exec(`INSERT INTO audit_log_p(ts,data,content_hash,prev_hash,chain_seq) VALUES($1,$2,$3,$4,$5)`,
		ts, raw, ch, ph, seq); err != nil {
		slog.Debug("PG 写审计分区表失败", "err", err)
	}
}
