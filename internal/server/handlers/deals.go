package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type DealHandler struct {
	service *deal.Service
}

func NewDealHandler(service *deal.Service) *DealHandler {
	return &DealHandler{service: service}
}

type CreateDealRequest struct {
	Title       string `json:"title"`
	Stage       string `json:"stage"`
	Currency    string `json:"currency"`
	Probability int32  `json:"probability"`
}

func (h *DealHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	var actorID pgtype.UUID
	if claims != nil {
		_ = actorID.Scan(claims.UserID.String())
	}

	var req CreateDealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	newDeal, err := h.service.Create(r.Context(), actorID, deal.CreateDealInput{
		Title:       req.Title,
		Stage:       req.Stage,
		Currency:    req.Currency,
		Probability: req.Probability,
	})

	if err != nil {
		if err == deal.ErrInvalidDeal {
			http.Error(w, `{"error":"validation_error","message":"title is required"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":"server_error","message":"failed to create deal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(newDeal)
}

func (h *DealHandler) List(w http.ResponseWriter, r *http.Request) {
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
		deals, err = h.service.ListByStage(r.Context(), stage, int32(limit), int32(offset))
	} else {
		deals, err = h.service.List(r.Context(), int32(limit), int32(offset))
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
	idStr := chi.URLParam(r, "id")
	var dealID pgtype.UUID
	if err := dealID.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	dealItem, err := h.service.GetByID(r.Context(), dealID)
	if err != nil {
		http.Error(w, `{"error":"deal_not_found"}`, http.StatusNotFound)
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
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	var actorID pgtype.UUID
	if claims != nil {
		_ = actorID.Scan(claims.UserID.String())
	}

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
		http.Error(w, `{"error":"deal_not_found"}`, http.StatusNotFound)
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
		if err := val.Scan(req.Value); err != nil {
			http.Error(w, `{"error":"invalid_value","message":"Ungültiges Zahlenformat für Deal-Volumen"}`, http.StatusBadRequest)
			return
		}
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
		http.Error(w, `{"error":"failed to update deal: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func (h *DealHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil || claims.Role != "ADMIN" {
		http.Error(w, `{"error":"forbidden","message":"Löschen von Deals erfordert Administrator-Rechte"}`, http.StatusForbidden)
		return
	}
	var actorID pgtype.UUID
	_ = actorID.Scan(claims.UserID.String())

	idStr := chi.URLParam(r, "id")
	var dealID pgtype.UUID
	if err := dealID.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	if err := h.service.Delete(r.Context(), actorID, dealID); err != nil {
		http.Error(w, `{"error":"failed to delete deal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *DealHandler) AttachSolarCalculation(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	var actorID pgtype.UUID
	if claims != nil {
		_ = actorID.Scan(claims.UserID.String())
	}

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
		http.Error(w, `{"error":"deal_not_found"}`, http.StatusNotFound)
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
		http.Error(w, `{"error":"failed to update deal with calculation"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "calculation_attached",
		"deal":        updated,
		"calculation": calc,
	})
}
