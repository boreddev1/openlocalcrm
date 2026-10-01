package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type DealEventTrigger interface {
	TriggerEvent(ctx context.Context, triggerType, targetID, targetType, targetName string) ([]automation.WorkflowRun, error)
}

type DealHandler struct {
	service *deal.Service
	trigger DealEventTrigger
}

func NewDealHandler(service *deal.Service, trigger ...DealEventTrigger) *DealHandler {
	h := &DealHandler{service: service}
	if len(trigger) > 0 {
		h.trigger = trigger[0]
	}
	return h
}

func writeDealError(w http.ResponseWriter, err error, action string) {
	switch {
	case errors.Is(err, deal.ErrDealNotFound):
		http.Error(w, `{"error":"deal_not_found"}`, http.StatusNotFound)
	case errors.Is(err, deal.ErrDealRevoked):
		http.Error(w, `{"error":"deal_conflict","message":"revoked deals are terminal"}`, http.StatusConflict)
	case errors.Is(err, deal.ErrConcurrencyConflict):
		http.Error(w, `{"error":"deal_conflict","message":"deal was modified by another user"}`, http.StatusConflict)
	case errors.Is(err, deal.ErrDirectRevocation):
		http.Error(w, `{"error":"validation_error","message":"Direct transition to WIDERRUFEN is prohibited; use POST /deals/{id}/revoke"}`, http.StatusBadRequest)
	case errors.Is(err, deal.ErrInvalidDeal), errors.Is(err, deal.ErrInvalidDealValue), errors.Is(err, deal.ErrInvalidProbability):
		http.Error(w, `{"error":"validation_error","message":"invalid deal data"}`, http.StatusBadRequest)
	default:
		log.Printf("[deal] failed to %s deal: %v", action, err)
		http.Error(w, `{"error":"server_error","message":"failed to process deal"}`, http.StatusInternalServerError)
	}
}

type CreateDealRequest struct {
	Title       string  `json:"title"`
	Stage       string  `json:"stage"`
	Currency    string  `json:"currency"`
	Probability int32   `json:"probability"`
	Value       any     `json:"value"`
	CompanyID   *string `json:"company_id"`
	ContactID   *string `json:"contact_id"`
}

// parseDealValue converts a JSON number or numeric string into a NUMERIC.
// A missing value is stored as 0.00 because deals.value is NOT NULL.
func parseDealValue(raw any) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	switch v := raw.(type) {
	case nil:
		err := n.Scan("0.00")
		return n, err
	case string:
		if strings.TrimSpace(v) == "" {
			err := n.Scan("0.00")
			return n, err
		}
		err := n.Scan(strings.TrimSpace(v))
		return n, err
	case float64:
		err := n.Scan(strconv.FormatFloat(v, 'f', 2, 64))
		return n, err
	default:
		return n, fmt.Errorf("unsupported value type %T", raw)
	}
}

// parseOptionalUUID returns nil for a missing or empty id and an error for a malformed one.
func parseOptionalUUID(s *string) (*pgtype.UUID, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil, nil
	}
	var id pgtype.UUID
	if err := id.Scan(strings.TrimSpace(*s)); err != nil {
		return nil, err
	}
	return &id, nil
}

type RevokeDealRequest struct {
	WiderrufenAt  *time.Time `json:"widerrufen_at"`
	WiderrufGrund string     `json:"widerruf_grund"`
}

func (h *DealHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	actorID := pgtype.UUID{Bytes: claims.UserID, Valid: true}

	var req CreateDealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if strings.EqualFold(req.Stage, "WIDERRUFEN") || strings.EqualFold(req.Stage, "REVOKED") {
		http.Error(w, `{"error":"validation_error","message":"Deals cannot be created directly in WIDERRUFEN stage; use POST /deals/{id}/revoke"}`, http.StatusBadRequest)
		return
	}

	value, err := parseDealValue(req.Value)
	if err != nil {
		http.Error(w, `{"error":"invalid_value","message":"Ungültiges Zahlenformat für Deal-Volumen"}`, http.StatusBadRequest)
		return
	}
	companyID, err := parseOptionalUUID(req.CompanyID)
	if err != nil {
		http.Error(w, `{"error":"invalid_id","message":"Ungültige company_id"}`, http.StatusBadRequest)
		return
	}
	contactID, err := parseOptionalUUID(req.ContactID)
	if err != nil {
		http.Error(w, `{"error":"invalid_id","message":"Ungültige contact_id"}`, http.StatusBadRequest)
		return
	}

	newDeal, err := h.service.Create(r.Context(), actorID, deal.CreateDealInput{
		Title:       req.Title,
		Stage:       req.Stage,
		Currency:    req.Currency,
		Probability: req.Probability,
		Value:       value,
		CompanyID:   companyID,
		ContactID:   contactID,
		AssignedTo:  &actorID,
	})

	if err != nil {
		writeDealError(w, err, "create")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(newDeal)
}

func (h *DealHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var assignedTo pgtype.UUID
	if claims.Role == "BENUTZER" {
		_ = assignedTo.Scan(claims.UserID.String())
	}

	stage := r.URL.Query().Get("stage")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var deals []db.Deal
	var err error
	if stage != "" {
		deals, err = h.service.ListByStage(r.Context(), stage, assignedTo, int32(limit), int32(offset))
	} else {
		deals, err = h.service.List(r.Context(), int32(limit), int32(offset), assignedTo)
	}
	if err != nil {
		http.Error(w, `{"error":"failed to list deals"}`, http.StatusInternalServerError)
		return
	}

	if deals == nil {
		deals = []db.Deal{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(deals)
}

func (h *DealHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	var dealID pgtype.UUID
	if err := dealID.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	dealItem, err := h.service.GetByID(r.Context(), dealID)
	if err != nil {
		writeDealError(w, err, "get")
		return
	}

	if !canRead(claims, dealItem.AssignedTo) {
		http.Error(w, `{"error":"forbidden","message":"insufficient permissions to view deal"}`, http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dealItem)
}

type UpdateDealRequest struct {
	Title       string `json:"title"`
	Stage       string `json:"stage"`
	Currency    string `json:"currency"`
	Probability int32  `json:"probability"`
	Value       any    `json:"value"`
}

func (h *DealHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	actorID := pgtype.UUID{Bytes: claims.UserID, Valid: true}

	idStr := chi.URLParam(r, "id")
	var dealID pgtype.UUID
	if err := dealID.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	var req UpdateDealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	existing, err := h.service.GetByID(r.Context(), dealID)
	if err != nil {
		writeDealError(w, err, "get for update")
		return
	}

	if !canModify(claims, existing.AssignedTo) {
		writeJSONError(w, http.StatusForbidden, "forbidden", "Keine Berechtigung für diesen Datensatz")
		return
	}

	title := req.Title
	if title == "" {
		title = existing.Title
	}
	stage := req.Stage
	if stage == "" {
		stage = existing.Stage
	}
	prob := req.Probability
	if prob == 0 && stage != existing.Stage {
		switch stage {
		case "LEAD":
			prob = 20
		case "QUALIFIED":
			prob = 40
		case "OFFER_SENT":
			prob = 60
		case "NEGOTIATION":
			prob = 80
		case "WON":
			prob = 100
		case "LOST", "REVOKED":
			prob = 0
		default:
			prob = existing.Probability
		}
	} else if prob == 0 {
		prob = existing.Probability
	}

	val := existing.Value
	if req.Value != nil {
		parsed, perr := parseDealValue(req.Value)
		if perr != nil {
			http.Error(w, `{"error":"invalid_value","message":"Ungültiges Zahlenformat für Deal-Volumen"}`, http.StatusBadRequest)
			return
		}
		val = parsed
	}

	updated, err := h.service.Update(r.Context(), actorID, deal.UpdateDealInput{
		ID:          dealID,
		Title:       title,
		Stage:       stage,
		Probability: prob,
		Currency:    existing.Currency,
		Value:       val,
	})
	if err != nil {
		writeDealError(w, err, "update")
		return
	}

	if existing.Stage != "WON" && updated.Stage == "WON" && h.trigger != nil {
		if _, trigErr := h.trigger.TriggerEvent(r.Context(), "DEAL_WON", idStr, "DEAL", updated.Title); trigErr != nil {
			log.Printf("[deal_warning] automation trigger DEAL_WON failed for deal %s: %v", idStr, trigErr)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func (h *DealHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	actorID := pgtype.UUID{Bytes: claims.UserID, Valid: true}

	idStr := chi.URLParam(r, "id")
	var dealID pgtype.UUID
	if err := dealID.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	existing, err := h.service.GetByID(r.Context(), dealID)
	if err != nil {
		writeDealError(w, err, "get for delete")
		return
	}
	if !canModify(claims, existing.AssignedTo) {
		writeJSONError(w, http.StatusForbidden, "forbidden", "Keine Berechtigung für diesen Datensatz")
		return
	}

	if err := h.service.Delete(r.Context(), actorID, dealID); err != nil {
		writeDealError(w, err, "delete")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *DealHandler) AttachSolarCalculation(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	actorID := pgtype.UUID{Bytes: claims.UserID, Valid: true}

	idStr := chi.URLParam(r, "id")
	var dealID pgtype.UUID
	if err := dealID.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	var calc SolarCalcResponse
	if err := json.NewDecoder(r.Body).Decode(&calc); err != nil {
		http.Error(w, `{"error":"invalid solar calculation payload"}`, http.StatusBadRequest)
		return
	}

	existing, err := h.service.GetByID(r.Context(), dealID)
	if err != nil {
		writeDealError(w, err, "get for update")
		return
	}

	if !canModify(claims, existing.AssignedTo) {
		writeJSONError(w, http.StatusForbidden, "forbidden", "Keine Berechtigung für diesen Datensatz")
		return
	}

	val := existing.Value
	if calc.SystemCostGrossEuro > 0 {
		_ = val.Scan(fmt.Sprintf("%d.00", calc.SystemCostGrossEuro))
	}

	updated, err := h.service.Update(r.Context(), actorID, deal.UpdateDealInput{
		ID:          dealID,
		Title:       existing.Title,
		Stage:       existing.Stage,
		Probability: existing.Probability,
		Currency:    existing.Currency,
		Value:       val,
	})
	if err != nil {
		writeDealError(w, err, "attach solar calculation to")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "calculation_attached",
		"deal":        updated,
		"calculation": calc,
	})
}

func (h *DealHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	actorID := pgtype.UUID{Bytes: claims.UserID, Valid: true}

	idStr := chi.URLParam(r, "id")
	var dealID pgtype.UUID
	if err := dealID.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	existing, err := h.service.GetByID(r.Context(), dealID)
	if err != nil {
		writeDealError(w, err, "get for revoke")
		return
	}
	if !canModify(claims, existing.AssignedTo) {
		writeJSONError(w, http.StatusForbidden, "forbidden", "Keine Berechtigung für diesen Datensatz")
		return
	}

	var req RevokeDealRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
			return
		}
	}

	revokedAt := time.Now().UTC()
	if req.WiderrufenAt != nil && !req.WiderrufenAt.IsZero() {
		revokedAt = *req.WiderrufenAt
	}

	revoked, err := h.service.Revoke(r.Context(), actorID, dealID, revokedAt, req.WiderrufGrund)
	if err != nil {
		writeDealError(w, err, "revoke")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(revoked)
}
