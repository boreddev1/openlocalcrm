package server_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
	"github.com/stretchr/testify/require"
)

// TestRouteMatrixAcrossRoles is the machine-checked enforcement of the approved
// role matrix. It drives the real router, not the handlers in isolation, so a
// middleware that is not actually mounted fails the test.
func TestRouteMatrixAcrossRoles(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	querier := demo.NewInMemoryQuerier()
	hub := sse.NewHub()
	router := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   hub,
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	ctx := context.Background()

	// Seed one user per role in the querier with token_version = 0
	roles := []string{"ADMIN", "VERTRIEB", "BACKOFFICE", "BENUTZER"}
	tokens := make(map[string]string)
	userIDs := make(map[string]uuid.UUID)

	for _, role := range roles {
		email := role + "@example.de"
		created, seedErr := querier.CreateUser(ctx, db.CreateUserParams{
			Email:        email,
			PasswordHash: "$argon2id$v=19$m=65536,t=3,p=2$demo$fakehash",
			FirstName:    role,
			LastName:     "Tester",
			Role:         role,
		})
		require.NoError(t, seedErr)

		uID := uuid.UUID(created.ID.Bytes)
		userIDs[role] = uID

		token, seedErr := auth.GenerateAccessToken(uID, email, role, 0, privKey, 15*time.Minute)
		require.NoError(t, seedErr)
		tokens[role] = token
	}

	// Seed a contact and deal for telephony and deal-delete tests
	auditSvc := audit.NewService(querier)
	contactSvc := contact.NewService(querier, auditSvc)
	dealSvc := deal.NewService(querier, auditSvc)

	adminUUID := pgtype.UUID{Bytes: userIDs["ADMIN"], Valid: true}
	c, err := contactSvc.Create(ctx, adminUUID, contact.CreateContactInput{
		FirstName: "Max",
		LastName:  "Mustermann",
	})
	require.NoError(t, err)
	contactIDStr := uuid.UUID(c.ID.Bytes).String()

	d, err := dealSvc.Create(ctx, adminUUID, deal.CreateDealInput{
		Title:     "Solar Test Deal",
		ContactID: &c.ID,
	})
	require.NoError(t, err)
	dealIDStr := uuid.UUID(d.ID.Bytes).String()

	doAs := func(t *testing.T, role, method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var bodyReader io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			require.NoError(t, err)
			bodyReader = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, bodyReader)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+tokens[role])
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	cases := []struct {
		method, path string
		body         any
		want         map[string]int
	}{
		{
			method: http.MethodGet,
			path:   "/api/v1/users",
			want:   map[string]int{"ADMIN": 200, "VERTRIEB": 403, "BACKOFFICE": 403, "BENUTZER": 403},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/backup/export",
			want:   map[string]int{"ADMIN": 200, "VERTRIEB": 403, "BACKOFFICE": 403, "BENUTZER": 403},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/reports/sales",
			want:   map[string]int{"ADMIN": 200, "VERTRIEB": 200, "BACKOFFICE": 200, "BENUTZER": 403},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/export/contacts.csv",
			want:   map[string]int{"ADMIN": 200, "VERTRIEB": 403, "BACKOFFICE": 200, "BENUTZER": 403},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/telephony/calls",
			body: map[string]any{
				"contact_id":       contactIDStr,
				"duration_seconds": 120,
				"disposition":      "REACHED",
			},
			want: map[string]int{"ADMIN": 201, "VERTRIEB": 201, "BACKOFFICE": 201, "BENUTZER": 403},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/automations/",
			want:   map[string]int{"ADMIN": 200, "VERTRIEB": 200, "BACKOFFICE": 200, "BENUTZER": 403},
		},
		{
			method: http.MethodPost,
			path:   "/api/v1/automations",
			body: map[string]any{
				"name":        "Test Workflow",
				"description": "Auto test",
				"trigger":     "CONTACT_CREATED",
				"actions":     []any{},
			},
			want: map[string]int{"ADMIN": 201, "VERTRIEB": 403, "BACKOFFICE": 403, "BENUTZER": 403},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/ai/observability",
			want:   map[string]int{"ADMIN": 200, "VERTRIEB": 403, "BACKOFFICE": 403, "BENUTZER": 403},
		},
		{
			method: http.MethodDelete,
			path:   "/api/v1/deals/" + dealIDStr,
			want:   map[string]int{"ADMIN": 200, "VERTRIEB": 200, "BACKOFFICE": 403, "BENUTZER": 403},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/notifications/unread",
			want:   map[string]int{"ADMIN": 200, "VERTRIEB": 200, "BACKOFFICE": 200, "BENUTZER": 200},
		},
	}

	for _, tc := range cases {
		for role, want := range tc.want {
			t.Run(role+" "+tc.method+" "+tc.path, func(t *testing.T) {
				path := tc.path
				if tc.method == http.MethodDelete {
					freshDeal, err := dealSvc.Create(ctx, adminUUID, deal.CreateDealInput{
						Title:     "Fresh Deal For " + role,
						ContactID: &c.ID,
					})
					require.NoError(t, err)
					path = "/api/v1/deals/" + uuid.UUID(freshDeal.ID.Bytes).String()
				}
				rec := doAs(t, role, tc.method, path, tc.body)
				require.Equal(t, want, rec.Code, "unexpected status code for %s %s with role %s: body=%s", tc.method, tc.path, role, rec.Body.String())
			})
		}
	}
}
