package demo_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/stretchr/testify/require"
)

func auditLogParams(action string) db.CreateAuditLogParams {
	return db.CreateAuditLogParams{
		UserID:     pgtype.UUID{},
		Action:     action,
		EntityType: "test",
		EntityID:   pgtype.UUID{},
		Changes:    []byte(`{}`),
		IpAddress:  pgtype.Text{String: "127.0.0.1", Valid: true},
		UserAgent:  pgtype.Text{String: "test", Valid: true},
	}
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// TestDemoRunTxRollsBackTheAuditLogSlice pins the slice case. auditLogs is the
// one non-map collection, and it is the field a map-only snapshot would miss.
func TestDemoRunTxRollsBackTheAuditLogSlice(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	before := len(q.AuditLogsForTest())

	err := q.RunTx(context.Background(), func(txq db.Querier) error {
		if _, err := txq.CreateAuditLog(context.Background(), auditLogParams("written inside a failed transaction")); err != nil {
			return err
		}
		return errors.New("abort")
	})
	require.Error(t, err)
	require.Equal(t, before, len(q.AuditLogsForTest()), "a rolled-back transaction must not leave an audit row behind")

	require.NoError(t, q.RunTx(context.Background(), func(txq db.Querier) error {
		_, err := txq.CreateAuditLog(context.Background(), auditLogParams("committed"))
		return err
	}))
	require.Equal(t, before+1, len(q.AuditLogsForTest()), "a committed transaction keeps its audit row")
}

// TestEveryDemoCollectionIsCloned guards the shadow clone. A new map or slice
// field that is missing from cloneForTransaction and adoptFrom would be shared
// between the shadow and the original, so it would escape rollback silently.
// Both map and slice kinds are counted, not just maps.
func TestEveryDemoCollectionIsCloned(t *testing.T) {
	typ := reflect.TypeOf(demo.InMemoryQuerier{})
	want := 0
	for i := 0; i < typ.NumField(); i++ {
		switch typ.Field(i).Type.Kind() {
		case reflect.Map, reflect.Slice, reflect.Pointer, reflect.Interface, reflect.Array:
			want++
		}
	}
	covered := demo.TransactionCollectionFields()
	require.Equal(t, want, len(covered),
		"every map or slice field on InMemoryQuerier needs a clone and adopt entry")
	require.Len(t, covered, len(uniqueStrings(covered)), "TransactionCollectionFields must not list a field twice")
}
