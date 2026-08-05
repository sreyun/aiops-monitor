package main

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestPersistAndLatestEvalRun(t *testing.T) {
	p, mock := mockPgStore(t)
	sum := evalRunSummary{
		RunID: "eval_test1", Ts: 100, Model: "m", Mode: "online", EvalSetVersion: "v1.6",
		CaseCount: 6, PassedCount: 4, PassRate: 0.66,
		RootCauseHitRate: 0.8, ActionAcceptRate: 0.7, VerifyAgreement: 0.8,
	}
	mock.ExpectExec(`INSERT INTO ai_eval_runs`).WillReturnResult(sqlmock.NewResult(1, 1))
	p.persistEvalRun(sum)

	// latestEvalRun reads back the row.
	mock.ExpectQuery(`SELECT id, ts, model`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "ts", "model", "mode", "eval_set_version", "case_count", "passed_count",
		"pass_rate", "root_cause_hit_rate", "action_accept_rate", "verify_agreement", "detail",
	}).AddRow("eval_test1", 100, "m", "online", "v1.6", 6, 4, 0.66, 0.8, 0.7, 0.8, []byte("{}")))

	got, err := p.latestEvalRun()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if got.RunID != "eval_test1" || got.CaseCount != 6 || got.PassRate != 0.66 {
		t.Fatalf("unexpected: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestLatestEvalRunEmpty(t *testing.T) {
	p, mock := mockPgStore(t)
	mock.ExpectQuery(`SELECT id, ts, model`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "ts", "model", "mode", "eval_set_version", "case_count", "passed_count",
		"pass_rate", "root_cause_hit_rate", "action_accept_rate", "verify_agreement", "detail",
	})) // empty -> sql.ErrNoRows

	got, err := p.latestEvalRun()
	if err != nil {
		t.Fatalf("empty should not error, got %v", err)
	}
	if got.RunID != "" {
		t.Fatalf("expected empty RunID, got %+v", got)
	}
}
