package db

import "context"

// RunTx executes fn with a transactional Querier: either every statement
// commits or none does. It exists so services can group writes without
// reaching for the pool, and so the same call works against the in-memory demo
// querier, which implements RunTx by snapshot and restore.
func (q *Queries) RunTx(ctx context.Context, fn func(Querier) error) error {
	tx, err := q.Begin(ctx)
	if err != nil {
		return err
	}
	// The rollback is deferred immediately after Begin, before any work, so a
	// failure on any path releases the transaction. After a successful Commit it
	// is a no-op.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if err := fn(New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
