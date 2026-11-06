package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// uniqueViolationCode is SQLSTATE 23505, raised by a unique index or unique
// constraint.
const uniqueViolationCode = "23505"

// IsUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation.
//
// It is deliberately narrow. Only a real *pgconn.PgError carrying SQLSTATE
// 23505 counts, so a handler that maps this to HTTP 409 is reporting something
// the database actually said. The in-memory demo querier returns an untyped
// error for a duplicate key, so in demo mode this returns false and the caller
// must fall back to its generic failure path rather than claiming a conflict it
// cannot detect.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == uniqueViolationCode
	}
	return false
}
