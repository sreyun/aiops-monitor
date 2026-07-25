package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"aiops-monitor/cmd/server/sqltoolkit"
)

const sqlChangeApprovalTTL = 30 * time.Minute

type SQLChangeRequest struct {
	ID           string         `json:"id"`
	ConnectionID string         `json:"connection_id"`
	Connection   string         `json:"connection_name,omitempty"`
	Environment  string         `json:"environment"`
	SQL          string         `json:"sql"`
	Reason       string         `json:"reason,omitempty"`
	Status       string         `json:"status"`
	Proposer     string         `json:"proposer"`
	Approver     string         `json:"approver,omitempty"`
	Executor     string         `json:"executor,omitempty"`
	CreatedAt    int64          `json:"created_at"`
	ApprovedAt   int64          `json:"approved_at,omitempty"`
	ExpiresAt    int64          `json:"expires_at,omitempty"`
	ExecutedAt   int64          `json:"executed_at,omitempty"`
	Error        string         `json:"error,omitempty"`
	Result       map[string]any `json:"result,omitempty"`
}

type sqlChangeRequestManager struct {
	mu    sync.Mutex
	items map[string]*SQLChangeRequest
	ttl   time.Duration
}

func newSQLChangeRequestManager() *sqlChangeRequestManager {
	return &sqlChangeRequestManager{items: make(map[string]*SQLChangeRequest), ttl: sqlChangeApprovalTTL}
}

func sqlConnectionEnv(c MySQLConnection) string {
	env := strings.ToLower(strings.TrimSpace(c.Env))
	if env == "" {
		return "prod"
	}
	return env
}

func cloneSQLChangeRequest(in *SQLChangeRequest) SQLChangeRequest {
	out := *in
	if in.Result != nil {
		out.Result = make(map[string]any, len(in.Result))
		for k, v := range in.Result {
			out.Result[k] = v
		}
	}
	return out
}

func (m *sqlChangeRequestManager) Create(c MySQLConnection, sqlText, reason, proposer string, now time.Time) SQLChangeRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	cr := &SQLChangeRequest{
		ID:           termID(),
		ConnectionID: c.ID,
		Connection:   c.Name,
		Environment:  sqlConnectionEnv(c),
		SQL:          strings.TrimSpace(sqlText),
		Reason:       strings.TrimSpace(reason),
		Status:       "pending",
		Proposer:     proposer,
		CreatedAt:    now.Unix(),
	}
	m.items[cr.ID] = cr
	return cloneSQLChangeRequest(cr)
}

func (m *sqlChangeRequestManager) List(now time.Time) []SQLChangeRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SQLChangeRequest, 0, len(m.items))
	for _, cr := range m.items {
		m.expireLocked(cr, now)
		out = append(out, cloneSQLChangeRequest(cr))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

func (m *sqlChangeRequestManager) Approve(id, approver string, now time.Time) (SQLChangeRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cr, ok := m.items[id]
	if !ok {
		return SQLChangeRequest{}, fmt.Errorf("change request not found")
	}
	if cr.Status != "pending" {
		return SQLChangeRequest{}, fmt.Errorf("change request is %s", cr.Status)
	}
	cr.Status = "approved"
	cr.Approver = approver
	cr.ApprovedAt = now.Unix()
	cr.ExpiresAt = now.Add(m.ttl).Unix()
	return cloneSQLChangeRequest(cr), nil
}

func (m *sqlChangeRequestManager) Reject(id, approver string, now time.Time) (SQLChangeRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cr, ok := m.items[id]
	if !ok {
		return SQLChangeRequest{}, fmt.Errorf("change request not found")
	}
	m.expireLocked(cr, now)
	if cr.Status != "pending" && cr.Status != "approved" {
		return SQLChangeRequest{}, fmt.Errorf("change request is %s", cr.Status)
	}
	cr.Status = "rejected"
	cr.Approver = approver
	cr.ExpiresAt = 0
	return cloneSQLChangeRequest(cr), nil
}

// BeginExecute atomically consumes an approval before database I/O. A failed
// database execution remains consumed, preventing unsafe retries with one ticket.
func (m *sqlChangeRequestManager) BeginExecute(id, connectionID, sqlText, executor string, now time.Time) (SQLChangeRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cr, ok := m.items[id]
	if !ok {
		return SQLChangeRequest{}, fmt.Errorf("change request not found")
	}
	m.expireLocked(cr, now)
	if cr.Status != "approved" {
		return SQLChangeRequest{}, fmt.Errorf("change request is %s", cr.Status)
	}
	if cr.ConnectionID != connectionID || cr.SQL != strings.TrimSpace(sqlText) {
		return SQLChangeRequest{}, fmt.Errorf("ticket connection or SQL does not match")
	}
	cr.Status = "executing"
	cr.Executor = executor
	return cloneSQLChangeRequest(cr), nil
}

func (m *sqlChangeRequestManager) FinishExecute(id string, result map[string]any, execErr error, now time.Time) SQLChangeRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	cr := m.items[id]
	if cr == nil {
		return SQLChangeRequest{}
	}
	cr.ExecutedAt = now.Unix()
	if execErr != nil {
		cr.Status = "failed"
		cr.Error = execErr.Error()
	} else {
		cr.Status = "executed"
		cr.Result = result
	}
	return cloneSQLChangeRequest(cr)
}

func (m *sqlChangeRequestManager) expireLocked(cr *SQLChangeRequest, now time.Time) {
	if cr.Status == "approved" && cr.ExpiresAt > 0 && now.Unix() >= cr.ExpiresAt {
		cr.Status = "expired"
	}
}

func (s *Server) auditSQLChange(r *http.Request, action string, cr SQLChangeRequest, level string) {
	detail, _ := json.Marshal(map[string]any{
		"action": action, "ticket_id": cr.ID, "connection_id": cr.ConnectionID,
		"environment": cr.Environment, "status": cr.Status, "sql": truncateRunes(cr.SQL, 200),
	})
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: level, Actor: s.actorName(r), IP: s.clientIP(r),
		Message: "sql_change_request " + string(detail)})
}

func (s *Server) handleCreateSQLChangeRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConnectionID string `json:"connection_id"`
		SQL          string `json:"sql"`
		Reason       string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	c, ok := s.cfg.GetMySQLConnection(strings.TrimSpace(req.ConnectionID))
	if !ok || !c.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection not found or disabled"})
		return
	}
	sqlText := strings.TrimSpace(req.SQL)
	if sqlText == "" || len(sqlText) > 16<<10 || !sqltoolkit.IsAllowedIndexDDL(sqlText) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "仅允许 16KB 内的 CREATE/ALTER 索引类 DDL"})
		return
	}
	if len(req.Reason) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason 过长"})
		return
	}
	cr := s.sqlChanges.Create(c, sqlText, req.Reason, s.actorName(r), time.Now())
	s.auditSQLChange(r, "create", cr, "warning")
	writeJSON(w, http.StatusCreated, cr)
}

func (s *Server) handleListSQLChangeRequests(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"change_requests": s.sqlChanges.List(time.Now())})
}

func (s *Server) handleApproveSQLChangeRequest(w http.ResponseWriter, r *http.Request) {
	cr, err := s.sqlChanges.Approve(strings.TrimSpace(r.PathValue("id")), s.actorName(r), time.Now())
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.auditSQLChange(r, "approve", cr, "warning")
	writeJSON(w, http.StatusOK, cr)
}

func (s *Server) handleRejectSQLChangeRequest(w http.ResponseWriter, r *http.Request) {
	cr, err := s.sqlChanges.Reject(strings.TrimSpace(r.PathValue("id")), s.actorName(r), time.Now())
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.auditSQLChange(r, "reject", cr, "warning")
	writeJSON(w, http.StatusOK, cr)
}

func (s *Server) executeSQLChangeRequest(w http.ResponseWriter, r *http.Request, ticketID, expectedConnectionID, expectedSQL string, timeoutSec int, verifySQL string) {
	now := time.Now()
	list := s.sqlChanges.List(now)
	var ticket *SQLChangeRequest
	for i := range list {
		if list[i].ID == ticketID {
			ticket = &list[i]
			break
		}
	}
	if ticket == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "change request not found"})
		return
	}
	if expectedConnectionID != "" && (ticket.ConnectionID != expectedConnectionID || ticket.SQL != strings.TrimSpace(expectedSQL)) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "ticket connection or SQL does not match"})
		return
	}
	c, ok := s.cfg.GetMySQLConnection(ticket.ConnectionID)
	if !ok || !c.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection not found or disabled"})
		return
	}
	cr, err := s.sqlChanges.BeginExecute(ticketID, c.ID, ticket.SQL, s.actorName(r), now)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.auditSQLChange(r, "execute_start", cr, "warning")
	result, execErr := s.execDDLWithExplain(c, cr.SQL, verifySQL, timeoutSec)
	cr = s.sqlChanges.FinishExecute(ticketID, result, execErr, time.Now())
	if execErr != nil {
		s.auditSQLChange(r, "execute_failed", cr, "error")
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": execErr.Error()})
		return
	}
	s.auditSQLChange(r, "execute_success", cr, "warning")
	writeJSON(w, http.StatusOK, cr)
}

func (s *Server) handleExecuteSQLChangeRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TimeoutSec int    `json:"timeout_sec"`
		VerifySQL  string `json:"verify_sql"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	s.executeSQLChangeRequest(w, r, strings.TrimSpace(r.PathValue("id")), "", "", req.TimeoutSec, req.VerifySQL)
}
