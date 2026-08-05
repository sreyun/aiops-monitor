package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// mockPgStore builds a pgStore backed by sqlmock (no real PG, matching the
// repo's offline test convention).
func mockPgStore(t *testing.T) (*pgStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &pgStore{db: db}, mock
}

func TestWithPgTxCommit(t *testing.T) {
	p, mock := mockPgStore(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO ai_call_events`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := p.withPgTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO ai_call_events(...)`)
		return err
	})
	if err != nil {
		t.Fatalf("commit path: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestWithPgTxRollbackOnError(t *testing.T) {
	p, mock := mockPgStore(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	sentinel := errors.New("boom")
	err := p.withPgTx(context.Background(), func(tx *sql.Tx) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestWithPgTxNilDB(t *testing.T) {
	var p *pgStore
	if err := p.withPgTx(context.Background(), func(tx *sql.Tx) error { return nil }); err == nil {
		t.Fatal("expected error for nil pgStore")
	}
}

func TestBumpVersionSuccess(t *testing.T) {
	p, mock := mockPgStore(t)
	mock.ExpectExec(`UPDATE ai_runs SET version=version\+1 WHERE id=\$1 AND version=\$2`).
		WithArgs("run1", int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := bumpVersion(context.Background(), p.db, "ai_runs", "id", "run1", 3)
	if err != nil {
		t.Fatalf("bump success: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestBumpVersionConflict(t *testing.T) {
	p, mock := mockPgStore(t)
	mock.ExpectExec(`UPDATE ai_runs SET version=version\+1 WHERE id=\$1 AND version=\$2`).
		WithArgs("run1", int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := bumpVersion(context.Background(), p.db, "ai_runs", "id", "run1", 3)
	if !errors.Is(err, ErrPgConflict) {
		t.Fatalf("expected ErrPgConflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
