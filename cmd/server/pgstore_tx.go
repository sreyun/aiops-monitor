package main

import (
	"context"
	"database/sql"
	"fmt"
)

// ErrPgConflict is returned by optimistic-lock guarded updates when the row
// version does not match (i.e. someone else already moved the state).
var ErrPgConflict = fmt.Errorf("pg: optimistic lock conflict")

// withPgTx runs fn inside a read-committed transaction. It is the single
// standardized entry point for multi-statement writes that must be atomic;
// new business write paths should use this instead of inlining db.Begin().
func (p *pgStore) withPgTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	if p == nil || p.db == nil {
		return sql.ErrConnDone
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// bumpVersion performs an optimistic-lock update: only succeeds when the row's
// current version equals want, and increments version by one. Rows==0 means a
// stale reader lost the race and ErrPgConflict is returned. This is the
// building block for multi-replica-safe state transitions.
func bumpVersion(ctx context.Context, q queryer, table, idCol string, id any, want int64) error {
	res, err := q.ExecContext(ctx,
		fmt.Sprintf(`UPDATE %s SET version=version+1 WHERE %s=$1 AND version=$2`, table, idCol),
		id, want)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrPgConflict
	}
	return nil
}

// queryer abstracts *sql.DB and *sql.Tx so helpers work inside and outside
// transactions.
type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}
