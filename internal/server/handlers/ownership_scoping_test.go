package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/appointment"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/company"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/core/note"
	"github.com/openlocalcrm/openlocalcrm/internal/core/todo"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
	"github.com/stretchr/testify/require"
)

func TestBenutzerReadScopingAcrossDomains(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	auditSvc := audit.NewService(q)
	hub := sse.NewHub()

	contactSvc := contact.NewService(q, auditSvc)
	companySvc := company.NewService(q, auditSvc)
	dealSvc := deal.NewService(q, auditSvc)
	todoSvc := todo.NewService(q, auditSvc)
	appSvc := appointment.NewService(q, hub)
	noteSvc := note.NewService(q, hub)

	contactH := handlers.NewContactHandler(contactSvc)
	companyH := handlers.NewCompanyHandler(companySvc)
	dealH := handlers.NewDealHandler(dealSvc)
	todoH := handlers.NewTodoHandler(todoSvc)
	appH := handlers.NewAppointmentHandler(appSvc)
	noteH := handlers.NewNoteHandler(noteSvc, nil)

	userA := uuid.MustParse("aaaaaaa1-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	userB := uuid.MustParse("bbbbbbb2-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	admin := uuid.MustParse("ccccccc3-cccc-cccc-cccc-cccccccccccc")

	userAUUID := pgtype.UUID{Bytes: userA, Valid: true}
	userBUUID := pgtype.UUID{Bytes: userB, Valid: true}

	ctx := context.Background()

	// 1. Seed contacts (1 for A, 1 for B)
	cA, err := contactSvc.Create(ctx, userAUUID, contact.CreateContactInput{
		FirstName: "Alice",
		LastName:  "A-Contact",
	})
	require.NoError(t, err)
	cB, err := contactSvc.Create(ctx, userBUUID, contact.CreateContactInput{
		FirstName: "Bob",
		LastName:  "B-Contact",
	})
	require.NoError(t, err)

	// 2. Seed companies (1 for A, 1 for B)
	compA, err := companySvc.Create(ctx, userAUUID, company.CreateCompanyInput{
		Name: "Company A",
	})
	require.NoError(t, err)
	compB, err := companySvc.Create(ctx, userBUUID, company.CreateCompanyInput{
		Name: "Company B",
	})
	require.NoError(t, err)

	// 3. Seed deals (1 for A, 1 for B)
	dealA, err := dealSvc.Create(ctx, userAUUID, deal.CreateDealInput{
		Title:      "Deal A",
		ContactID:  &cA.ID,
		AssignedTo: &userAUUID,
	})
	require.NoError(t, err)
	dealB, err := dealSvc.Create(ctx, userBUUID, deal.CreateDealInput{
		Title:      "Deal B",
		ContactID:  &cB.ID,
		AssignedTo: &userBUUID,
	})
	require.NoError(t, err)

	// 4. Seed todos (1 for A, 1 for B)
	todoA, err := todoSvc.Create(ctx, userAUUID, todo.CreateTodoInput{
		Title:      "Todo A",
		AssignedTo: &userAUUID,
	})
	require.NoError(t, err)
	todoB, err := todoSvc.Create(ctx, userBUUID, todo.CreateTodoInput{
		Title:      "Todo B",
		AssignedTo: &userBUUID,
	})
	require.NoError(t, err)

	// 5. Seed appointments (1 for A, 1 for B)
	appA, err := appSvc.Create(ctx, userAUUID, appointment.CreateAppointmentInput{
		Title:     "Appointment A",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)
	appB, err := appSvc.Create(ctx, userBUUID, appointment.CreateAppointmentInput{
		Title:     "Appointment B",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	// 6. Seed notes (1 for A, 1 for B)
	cAStr := uuid.UUID(cA.ID.Bytes).String()
	cBStr := uuid.UUID(cB.ID.Bytes).String()
	userAStr := userA.String()
	userBStr := userB.String()
	noteA, err := noteSvc.Create(ctx, note.CreateNoteInput{
		EntityType: "CONTACT",
		EntityID:   cAStr,
		Content:    "Note A",
		UserID:     &userAStr,
	})
	require.NoError(t, err)
	noteB, err := noteSvc.Create(ctx, note.CreateNoteInput{
		EntityType: "CONTACT",
		EntityID:   cBStr,
		Content:    "Note B",
		UserID:     &userBStr,
	})
	require.NoError(t, err)

	buildRouter := func(callerID uuid.UUID, callerRole string) http.Handler {
		r := chi.NewRouter()
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				claims := &auth.AccessClaims{
					UserID: callerID,
					Email:  callerID.String() + "@example.de",
					Role:   callerRole,
				}
				next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, claims)))
			})
		})
		r.Get("/api/v1/contacts", contactH.List)
		r.Get("/api/v1/contacts/{id}", contactH.Get)
		r.Get("/api/v1/companies", companyH.List)
		r.Get("/api/v1/companies/{id}", companyH.Get)
		r.Get("/api/v1/deals", dealH.List)
		r.Get("/api/v1/deals/{id}", dealH.Get)
		r.Get("/api/v1/todos", todoH.List)
		r.Get("/api/v1/todos/{id}", todoH.Get)
		r.Get("/api/v1/appointments", appH.List)
		r.Get("/api/v1/notes", noteH.List)
		r.Get("/api/v1/notes/{id}", noteH.Get)
		return r
	}

	routerA := buildRouter(userA, "BENUTZER")
	routerAdmin := buildRouter(admin, "ADMIN")

	t.Run("BENUTZER sees only own contacts in list, and gets 403 on other contact", func(t *testing.T) {
		rec := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/contacts", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var list []db.Contact
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		require.Len(t, list, 1)
		require.Equal(t, "Alice", list[0].FirstName)

		// Get other contact -> 403
		recOther := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/contacts/"+cBStr, nil)
		require.Equal(t, http.StatusForbidden, recOther.Code)

		// Get own contact -> 200
		recOwn := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/contacts/"+cAStr, nil)
		require.Equal(t, http.StatusOK, recOwn.Code)
	})

	t.Run("BENUTZER sees only own companies in list, and gets 403 on other company", func(t *testing.T) {
		rec := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/companies", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var list []db.Company
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		require.Len(t, list, 1)
		require.Equal(t, "Company A", list[0].Name)

		compBStr := uuid.UUID(compB.ID.Bytes).String()
		recOther := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/companies/"+compBStr, nil)
		require.Equal(t, http.StatusForbidden, recOther.Code)
	})

	t.Run("BENUTZER sees only own deals in list, and gets 403 on other deal", func(t *testing.T) {
		rec := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/deals", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var list []db.Deal
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		require.Len(t, list, 1)
		require.Equal(t, "Deal A", list[0].Title)

		dealBStr := uuid.UUID(dealB.ID.Bytes).String()
		recOther := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/deals/"+dealBStr, nil)
		require.Equal(t, http.StatusForbidden, recOther.Code)
	})

	t.Run("BENUTZER sees only own todos in list, and gets 403 on other todo", func(t *testing.T) {
		rec := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/todos", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var list []db.Todo
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		require.Len(t, list, 1)
		require.Equal(t, "Todo A", list[0].Title)

		todoBStr := uuid.UUID(todoB.ID.Bytes).String()
		recOther := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/todos/"+todoBStr, nil)
		require.Equal(t, http.StatusForbidden, recOther.Code)
	})

	t.Run("BENUTZER sees only own appointments in list", func(t *testing.T) {
		rec := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/appointments", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var list []appointment.Appointment
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		require.Len(t, list, 1)
		require.Equal(t, "Appointment A", list[0].Title)
	})

	t.Run("BENUTZER sees only own notes in list, and gets 403 on other note", func(t *testing.T) {
		rec := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/notes", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var list []note.Note
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		require.Len(t, list, 1)
		require.Equal(t, "Note A", list[0].Content)

		recOther := performHTTPJSON(t, routerA, http.MethodGet, "/api/v1/notes/"+noteB.ID, nil)
		require.Equal(t, http.StatusForbidden, recOther.Code)
	})

	t.Run("ADMIN sees all records across all domains", func(t *testing.T) {
		// Contacts
		rec := performHTTPJSON(t, routerAdmin, http.MethodGet, "/api/v1/contacts", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var contacts []db.Contact
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &contacts))
		require.Len(t, contacts, 2)

		// Companies
		rec = performHTTPJSON(t, routerAdmin, http.MethodGet, "/api/v1/companies", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var comps []db.Company
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &comps))
		require.Len(t, comps, 2)

		// Deals
		rec = performHTTPJSON(t, routerAdmin, http.MethodGet, "/api/v1/deals", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var deals []db.Deal
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &deals))
		require.Len(t, deals, 2)

		// Todos
		rec = performHTTPJSON(t, routerAdmin, http.MethodGet, "/api/v1/todos", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var todos []db.Todo
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &todos))
		require.Len(t, todos, 2)

		// Appointments
		rec = performHTTPJSON(t, routerAdmin, http.MethodGet, "/api/v1/appointments", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var apps []appointment.Appointment
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &apps))
		require.Len(t, apps, 2)

		// Notes
		rec = performHTTPJSON(t, routerAdmin, http.MethodGet, "/api/v1/notes", nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var notes []note.Note
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &notes))
		require.Len(t, notes, 2)
	})

	_ = compA
	_ = appA
	_ = appB
	_ = noteA
	_ = dealA
	_ = todoA
}
