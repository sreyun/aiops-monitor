package main

import (
	"net/http"
	"testing"
	"time"
)

func TestSQLChangeRequestApprovalTTLAndOneTimeExecution(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	m := newSQLChangeRequestManager()
	m.ttl = 30 * time.Minute
	conn := MySQLConnection{ID: "prod-db", Name: "Production", Env: "", Enabled: true}
	sqlText := "CREATE INDEX idx_users_email ON users(email)"

	cr := m.Create(conn, sqlText, "speed up lookup", "operator", now)
	if cr.Status != "pending" || cr.Environment != "prod" {
		t.Fatalf("created request = status %q env %q", cr.Status, cr.Environment)
	}
	cr, err := m.Approve(cr.ID, "admin", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if cr.Status != "approved" || cr.ExpiresAt != now.Add(31*time.Minute).Unix() {
		t.Fatalf("approved request = %#v", cr)
	}
	if _, err := m.BeginExecute(cr.ID, conn.ID, sqlText+" ", "operator", now.Add(2*time.Minute)); err != nil {
		t.Fatalf("first execution should consume approval: %v", err)
	}
	m.FinishExecute(cr.ID, map[string]any{"ok": true}, nil, now.Add(3*time.Minute))
	if _, err := m.BeginExecute(cr.ID, conn.ID, sqlText, "operator", now.Add(4*time.Minute)); err == nil {
		t.Fatal("approved ticket was reusable")
	}

	expired := m.Create(conn, sqlText, "", "operator", now)
	if _, err := m.Approve(expired.ID, "admin", now); err != nil {
		t.Fatal(err)
	}
	if _, err := m.BeginExecute(expired.ID, conn.ID, sqlText, "operator", now.Add(30*time.Minute)); err == nil {
		t.Fatal("expired approval was executable")
	}
}

func TestSQLChangeRequestRBAC(t *testing.T) {
	s := &Server{}
	if s.routeAllowed(newRoleRequest(http.MethodPost, "/api/v1/sql/change-requests/t1/approve"), RoleOperator) {
		t.Fatal("operator could approve a SQL change request")
	}
	if !s.routeAllowed(newRoleRequest(http.MethodPost, "/api/v1/sql/change-requests/t1/approve"), RoleAdmin) {
		t.Fatal("admin could not approve a SQL change request")
	}
	if !s.routeAllowed(newRoleRequest(http.MethodPost, "/api/v1/sql/change-requests"), RoleOperator) {
		t.Fatal("operator could not propose a SQL change request")
	}
	if s.routeAllowed(newRoleRequest(http.MethodGet, "/api/v1/sql/change-requests"), RoleViewer) {
		t.Fatal("viewer could read SQL change requests")
	}
}
