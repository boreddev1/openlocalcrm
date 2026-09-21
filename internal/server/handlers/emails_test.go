package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/core/email"
	"github.com/openlocalcrm/openlocalcrm/internal/crypto"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
	"github.com/stretchr/testify/require"
)

func TestEmailHandler(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	hub := sse.NewHub()
	svc := email.NewService(q, nil, nil, hub)
	h := handlers.NewEmailHandler(svc, true)

	t.Run("IngestDemo valid", func(t *testing.T) {
		reqBody := handlers.IngestDemoEmailRequest{
			SenderEmail: "kunde@solar-example.de",
			SenderName:  "Herr Solar",
			Subject:     "Anfrage zu 10 kWp Anlage",
			BodyText:    "Bitte um ein unverbindliches Angebot für unser Einfamilienhaus.",
			Recipients:  []string{"inbox@openlocalcrm.local"},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/emails/demo-ingest", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.IngestDemo(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var msg db.EmailMessage
		err := json.NewDecoder(rec.Body).Decode(&msg)
		require.NoError(t, err)
		require.Equal(t, reqBody.Subject, msg.Subject)
	})

	t.Run("IngestDemo invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/emails/demo-ingest", bytes.NewReader([]byte("{invalid-json")))
		rec := httptest.NewRecorder()

		h.IngestDemo(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("IngestDemo forbidden in production mode", func(t *testing.T) {
		hProd := handlers.NewEmailHandler(svc, false)
		req := httptest.NewRequest(http.MethodPost, "/api/emails/demo-ingest", bytes.NewReader([]byte("{}")))
		rec := httptest.NewRecorder()

		hProd.IngestDemo(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ListMessages", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/emails?limit=10&offset=0", nil)
		rec := httptest.NewRecorder()

		h.ListMessages(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var messages []db.EmailMessage
		err := json.NewDecoder(rec.Body).Decode(&messages)
		require.NoError(t, err)
		require.NotEmpty(t, messages)
	})

	t.Run("GetThread missing param", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/emails/threads/", nil)
		rec := httptest.NewRecorder()

		h.GetThread(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("GetThread with threadID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/emails/threads/test-thread", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("threadID", "test-thread")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()

		h.GetThread(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestEmailHandler_AccountCRUDAndSend(t *testing.T) {
	crypto.SetMasterKeyForTest([]byte("0123456789abcdef0123456789abcdef"))
	t.Cleanup(func() { crypto.SetMasterKeyForTest(nil) })

	q := demo.NewEmptyInMemoryQuerier()
	svc := email.NewService(q, nil, nil, nil)
	h := handlers.NewEmailHandler(svc, true)

	// 1. Create account
	createBody, _ := json.Marshal(handlers.EmailAccountRequest{
		Name:         "Vertrieb Postfach",
		EmailAddress: "vertrieb@openlocalcrm.local",
		Provider:     "IMAP",
		ImapHost:     "imap.example.com",
		ImapPort:     993,
		SmtpHost:     "smtp.example.com",
		SmtpPort:     587,
		Username:     "vertrieb@openlocalcrm.local",
		Password:     "s3cret-password",
	})
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/emails/accounts", bytes.NewReader(createBody))
	recCreate := httptest.NewRecorder()
	h.CreateAccount(recCreate, reqCreate)
	require.Equal(t, http.StatusCreated, recCreate.Code)

	var created handlers.EmailAccountResponse
	require.NoError(t, json.NewDecoder(recCreate.Body).Decode(&created))
	require.True(t, created.HasPassword)
	require.NotContains(t, recCreate.Body.String(), "s3cret-password")
	accIDStr := uuid.UUID(created.ID.Bytes).String()

	// 2. List accounts never leaks the password
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/emails/accounts", nil)
	recList := httptest.NewRecorder()
	h.ListAccounts(recList, reqList)
	require.Equal(t, http.StatusOK, recList.Code)
	require.NotContains(t, recList.Body.String(), "s3cret-password")
	require.NotContains(t, recList.Body.String(), "password_encrypted")

	// 3. Update account
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", accIDStr)
	updateBody, _ := json.Marshal(handlers.EmailAccountRequest{
		Name:     "Vertrieb Postfach (neu)",
		ImapHost: "imap.example.com",
		ImapPort: 993,
		SmtpHost: "smtp.example.com",
		SmtpPort: 587,
		Username: "vertrieb@openlocalcrm.local",
		IsActive: true,
	})
	reqUpdate := httptest.NewRequest(http.MethodPatch, "/api/v1/emails/accounts/"+accIDStr, bytes.NewReader(updateBody)).
		WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
	recUpdate := httptest.NewRecorder()
	h.UpdateAccount(recUpdate, reqUpdate)
	require.Equal(t, http.StatusOK, recUpdate.Code)

	var updated handlers.EmailAccountResponse
	require.NoError(t, json.NewDecoder(recUpdate.Body).Decode(&updated))
	require.Equal(t, "Vertrieb Postfach (neu)", updated.Name)

	// 4. Test connection against an unreachable host fails honestly with 502
	reqTest := httptest.NewRequest(http.MethodPost, "/api/v1/emails/accounts/"+accIDStr+"/test", nil).
		WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
	recTest := httptest.NewRecorder()
	h.TestAccountConnection(recTest, reqTest)
	require.Equal(t, http.StatusBadGateway, recTest.Code)

	// 5. Manual sync against an unreachable host fails honestly with 502
	reqSync := httptest.NewRequest(http.MethodPost, "/api/v1/emails/accounts/"+accIDStr+"/sync", nil).
		WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
	recSync := httptest.NewRecorder()
	h.SyncAccount(recSync, reqSync)
	require.Equal(t, http.StatusBadGateway, recSync.Code)

	// 6. Send email fails honestly against an unreachable SMTP host
	sendBody, _ := json.Marshal(handlers.SendEmailRequest{
		AccountID: accIDStr,
		To:        []string{"kunde@example.com"},
		Subject:   "Test",
		TextBody:  "Test",
	})
	reqSend := httptest.NewRequest(http.MethodPost, "/api/v1/emails/send", bytes.NewReader(sendBody))
	recSend := httptest.NewRecorder()
	h.SendEmail(recSend, reqSend)
	require.Equal(t, http.StatusBadGateway, recSend.Code)

	// 7. Delete account
	reqDelete := httptest.NewRequest(http.MethodDelete, "/api/v1/emails/accounts/"+accIDStr, nil).
		WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
	recDelete := httptest.NewRecorder()
	h.DeleteAccount(recDelete, reqDelete)
	require.Equal(t, http.StatusOK, recDelete.Code)
}

func TestEmailHandler_SendEmail_ValidatesInput(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	svc := email.NewService(q, nil, nil, nil)
	h := handlers.NewEmailHandler(svc, true)

	body, _ := json.Marshal(handlers.SendEmailRequest{AccountID: uuid.New().String()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/emails/send", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.SendEmail(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEmailHandler_TagMessage(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := email.NewService(q, nil, nil, nil)
	h := handlers.NewEmailHandler(svc, true)

	msg, err := svc.IngestMessage(context.Background(), email.IngestEmailInput{
		MessageID:   "<msg-tag@example.com>",
		SenderEmail: "kunde@example.com",
		Subject:     "Anfrage",
	})
	require.NoError(t, err)
	msgIDStr := uuid.UUID(msg.ID.Bytes).String()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", msgIDStr)
	body, _ := json.Marshal(handlers.TagMessageRequest{Tags: []string{"wichtig", "angebot"}})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/emails/"+msgIDStr+"/tags", bytes.NewReader(body)).
		WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.TagMessage(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var updated db.EmailMessage
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&updated))
	require.JSONEq(t, `["wichtig","angebot"]`, string(updated.Tags))
}
