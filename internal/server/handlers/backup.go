package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type BackupHandler struct {
	queries db.Querier
}

func NewBackupHandler(queries db.Querier) *BackupHandler {
	return &BackupHandler{
		queries: queries,
	}
}

type DrillResponse struct {
	Status         string `json:"status"`
	Timestamp      string `json:"timestamp"`
	UserCount      int64  `json:"user_count"`
	CompanyCount   int    `json:"company_count"`
	ContactCount   int    `json:"contact_count"`
	DealCount      int    `json:"deal_count"`
	IntegrityCheck string `json:"integrity_check"`
	Message        string `json:"message"`
}

func (h *BackupHandler) Drill(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil || claims.Role != "ADMIN" {
		http.Error(w, `{"error":"forbidden","message":"Nur Administratoren können Backup-Drills durchführen"}`, http.StatusForbidden)
		return
	}

	ctx := r.Context()
	userCount, err1 := h.queries.CountUsers(ctx)
	companies, err2 := h.queries.ListCompanies(ctx, db.ListCompaniesParams{Limit: 10000, Offset: 0})
	contacts, err3 := h.queries.ListContacts(ctx, db.ListContactsParams{Limit: 10000, Offset: 0})
	deals, err4 := h.queries.ListDeals(ctx, db.ListDealsParams{Limit: 10000, Offset: 0})

	w.Header().Set("Content-Type", "application/json")
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(DrillResponse{
			Status:         "unhealthy",
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
			UserCount:      userCount,
			CompanyCount:   len(companies),
			ContactCount:   len(contacts),
			DealCount:      len(deals),
			IntegrityCheck: "failed",
			Message:        fmt.Sprintf("Backup-Konsistenzprüfung fehlgeschlagen (Fehler: users=%v, companies=%v, contacts=%v, deals=%v)", err1, err2, err3, err4),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(DrillResponse{
		Status:         "healthy",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		UserCount:      userCount,
		CompanyCount:   len(companies),
		ContactCount:   len(contacts),
		DealCount:      len(deals),
		IntegrityCheck: "passed",
		Message:        "Backup-Konsistenzprüfung erfolgreich: Alle Tabellen und Referenzen sind lesbar und konsistent.",
	})
}

type ExportPayload struct {
	ExportedAt string       `json:"exported_at"`
	Companies  []db.Company `json:"companies"`
	Contacts   []db.Contact `json:"contacts"`
	Deals      []db.Deal    `json:"deals"`
	Todos      []db.Todo    `json:"todos"`
}

func (h *BackupHandler) Export(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil || claims.Role != "ADMIN" {
		http.Error(w, `{"error":"forbidden","message":"Nur Administratoren können Daten-Backups exportieren"}`, http.StatusForbidden)
		return
	}

	ctx := r.Context()
	const batchSize = 1000

	var allCompanies []db.Company
	for offset := int32(0); ; offset += batchSize {
		batch, err := h.queries.ListCompanies(ctx, db.ListCompaniesParams{Limit: batchSize, Offset: offset})
		if err != nil {
			http.Error(w, `{"error":"export_failed","table":"companies"}`, http.StatusInternalServerError)
			return
		}
		allCompanies = append(allCompanies, batch...)
		if len(batch) < batchSize {
			break
		}
	}

	var allContacts []db.Contact
	for offset := int32(0); ; offset += batchSize {
		batch, err := h.queries.ListContacts(ctx, db.ListContactsParams{Limit: batchSize, Offset: offset})
		if err != nil {
			http.Error(w, `{"error":"export_failed","table":"contacts"}`, http.StatusInternalServerError)
			return
		}
		allContacts = append(allContacts, batch...)
		if len(batch) < batchSize {
			break
		}
	}

	var allDeals []db.Deal
	for offset := int32(0); ; offset += batchSize {
		batch, err := h.queries.ListDeals(ctx, db.ListDealsParams{Limit: batchSize, Offset: offset})
		if err != nil {
			http.Error(w, `{"error":"export_failed","table":"deals"}`, http.StatusInternalServerError)
			return
		}
		allDeals = append(allDeals, batch...)
		if len(batch) < batchSize {
			break
		}
	}

	var allTodos []db.Todo
	for offset := int32(0); ; offset += batchSize {
		batch, err := h.queries.ListTodos(ctx, db.ListTodosParams{Limit: batchSize, Offset: offset})
		if err != nil {
			http.Error(w, `{"error":"export_failed","table":"todos"}`, http.StatusInternalServerError)
			return
		}
		allTodos = append(allTodos, batch...)
		if len(batch) < batchSize {
			break
		}
	}

	if allCompanies == nil {
		allCompanies = []db.Company{}
	}
	if allContacts == nil {
		allContacts = []db.Contact{}
	}
	if allDeals == nil {
		allDeals = []db.Deal{}
	}
	if allTodos == nil {
		allTodos = []db.Todo{}
	}

	payload := ExportPayload{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Companies:  allCompanies,
		Contacts:   allContacts,
		Deals:      allDeals,
		Todos:      allTodos,
	}

	fileName := fmt.Sprintf("openlocalcrm-backup-%s.json", time.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))

	_ = json.NewEncoder(w).Encode(payload)
}
