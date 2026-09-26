package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrTransactionsUnsupported is returned when the underlying handle cannot open
// a transaction. The demo querier has no pool, so restore and drill must report
// the operation as unavailable rather than pretend to succeed.
var ErrTransactionsUnsupported = errors.New("db: transactions are unsupported")

// beginner is the optional transaction capability a DBTX handle may provide.
// *pgxpool.Pool and *pgx.Conn implement it; the in-memory demo querier does not.
type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Begin opens a transaction on the underlying handle. It lets services run
// transactional restores without reaching for the pool themselves, and it
// returns ErrTransactionsUnsupported instead of panicking when no pool exists.
func (q *Queries) Begin(ctx context.Context) (pgx.Tx, error) {
	beginner, ok := q.db.(beginner)
	if !ok {
		return nil, ErrTransactionsUnsupported
	}
	return beginner.Begin(ctx)
}
