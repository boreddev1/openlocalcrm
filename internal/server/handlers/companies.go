package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/company"
)

type CompanyHandler struct {
	service *company.Service
}

func NewCompanyHandler(service *company.Service) *CompanyHandler {
	return &CompanyHandler{service: service}
}

type CreateCompanyRequest struct {
	Name           string `json:"name"`
	Domain         string `json:"domain"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`
	AddressStreet  string `json:"address_street"`
	AddressZip     string `json:"address_zip"`
	AddressCity    string `json:"address_city"`
	AddressCountry string `json:"address_country"`
}

func (h *CompanyHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	var actorID pgtype.UUID
	if claims != nil {
		_ = actorID.Scan(claims.UserID.String())
	}

	var req CreateCompanyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	comp, err := h.service.Create(r.Context(), actorID, company.CreateCompanyInput{
		Name:           req.Name,
		Domain:         req.Domain,
		Phone:          req.Phone,
		Email:          req.Email,
		AddressStreet:  req.AddressStreet,
		AddressZip:     req.AddressZip,
		AddressCity:    req.AddressCity,
		AddressCountry: req.AddressCountry,
	})

	if err != nil {
		if err == company.ErrInvalidCompany {
			http.Error(w, `{"error":"validation_error","message":"name is required"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":"server_error","message":"failed to create company"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(comp)
}

func (h *CompanyHandler) List(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	companies, err := h.service.List(r.Context(), int32(limit), int32(offset))
	if err != nil {
		http.Error(w, `{"error":"failed to list companies"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(companies)
}

func (h *CompanyHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	comp, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"company_not_found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(comp)
}

func (h *CompanyHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	var actorID pgtype.UUID
	if claims != nil {
		_ = actorID.Scan(claims.UserID.String())
	}

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	var req CreateCompanyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	updated, err := h.service.Update(r.Context(), actorID, company.UpdateCompanyInput{
		ID:             id,
		Name:           req.Name,
		Domain:         req.Domain,
		Phone:          req.Phone,
		Email:          req.Email,
		AddressStreet:  req.AddressStreet,
		AddressZip:     req.AddressZip,
		AddressCity:    req.AddressCity,
		AddressCountry: req.AddressCountry,
	})
	if err != nil {
		http.Error(w, `{"error":"failed to update company: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func (h *CompanyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil || claims.Role != "ADMIN" {
		http.Error(w, `{"error":"forbidden","message":"Löschen von Unternehmen erfordert Administrator-Rechte"}`, http.StatusForbidden)
		return
	}
	var actorID pgtype.UUID
	_ = actorID.Scan(claims.UserID.String())

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	if err := h.service.Delete(r.Context(), actorID, id); err != nil {
		http.Error(w, `{"error":"failed to delete company"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

