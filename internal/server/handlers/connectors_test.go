package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/connectors"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/stretchr/testify/require"
)

type mockContactDB struct {
	db.Querier
}

func (m *mockContactDB) CreateContact(ctx context.Context, arg db.CreateContactParams) (db.Contact, error) {
	var id pgtype.UUID
	_ = id.Scan("44444444-4444-4444-4444-444444444444")
	return db.Contact{
		ID:        id,
		FirstName: arg.FirstName,
		LastName:  arg.LastName,
		Email:     arg.Email,
	}, nil
}

func TestConnectorHandler_LeadIntake(t *testing.T) {
	contactSvc := contact.NewService(&mockContactDB{}, nil)
	engine := connectors.NewEngine(contactSvc, nil, "valid-token")
	h := handlers.NewConnectorHandler(engine)

	t.Run("missing bearer header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/connectors/lead-intake", bytes.NewReader([]byte("{}")))
		rec := httptest.NewRecorder()

		h.LeadIntake(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		payload := connectors.LeadIntakePayload{LastName: "Schmidt"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/connectors/lead-intake", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer wrong-token")
		rec := httptest.NewRecorder()

		h.LeadIntake(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid json body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/connectors/lead-intake", bytes.NewReader([]byte("{invalid-json")))
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		h.LeadIntake(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid token and payload", func(t *testing.T) {
		payload := connectors.LeadIntakePayload{
			FirstName: "Klaus",
			LastName:  "Schmidt",
			Email:     "klaus@schmidt.de",
			City:      "Berlin",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/connectors/lead-intake", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		h.LeadIntake(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		require.Equal(t, "success", res["status"])
	})
}
