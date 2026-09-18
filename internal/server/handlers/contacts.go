package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
)

type ContactHandler struct {
	service *contact.Service
}

func NewContactHandler(service *contact.Service) *ContactHandler {
	return &ContactHandler{service: service}
}

type CreateContactRequest struct {
	FirstName     string   `json:"first_name"`
	LastName      string   `json:"last_name"`
	Email         string   `json:"email"`
	Phone         string   `json:"phone"`
	Mobile        string   `json:"mobile"`
	Position      string   `json:"position"`
	LeadSource    string   `json:"lead_source"`
	AddressStreet string   `json:"address_street"`
	AddressZip    string   `json:"address_zip"`
	AddressCity   string   `json:"address_city"`
	Latitude      *float64 `json:"latitude"`
	Longitude     *float64 `json:"longitude"`
}

func (h *ContactHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	var actorID pgtype.UUID
	if claims != nil {
		_ = actorID.Scan(claims.UserID.String())
	}

	var req CreateContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	res, err := h.service.Create(r.Context(), actorID, contact.CreateContactInput{
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		Email:         req.Email,
		Phone:         req.Phone,
		Mobile:        req.Mobile,
		Position:      req.Position,
		LeadSource:    req.LeadSource,
		AddressStreet: req.AddressStreet,
		AddressZip:    req.AddressZip,
		AddressCity:   req.AddressCity,
		Latitude:      req.Latitude,
		Longitude:     req.Longitude,
	})

	if err != nil {
		if err == contact.ErrInvalidContact {
			http.Error(w, `{"error":"validation_error","message":"first_name and last_name are required"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":"server_error","message":"failed to create contact"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(res)
}

func (h *ContactHandler) List(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	searchQuery := r.URL.Query().Get("q")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	if searchQuery != "" {
		results, err := h.service.Search(r.Context(), searchQuery, int32(limit))
		if err != nil {
			http.Error(w, `{"error":"search_failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(results)
		return
	}

	contacts, err := h.service.List(r.Context(), int32(limit), int32(offset))
	if err != nil {
		http.Error(w, `{"error":"failed to list contacts"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(contacts)
}

func (h *ContactHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	contact, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"contact_not_found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(contact)
}

func (h *ContactHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	var req CreateContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	res, err := h.service.Update(r.Context(), actorID, contact.UpdateContactInput{
		ID:            id,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		Email:         req.Email,
		Phone:         req.Phone,
		Mobile:        req.Mobile,
		Position:      req.Position,
		LeadSource:    req.LeadSource,
		AddressStreet: req.AddressStreet,
		AddressZip:    req.AddressZip,
		AddressCity:   req.AddressCity,
		Latitude:      req.Latitude,
		Longitude:     req.Longitude,
	})
	if err != nil {
		http.Error(w, `{"error":"failed to update contact: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *ContactHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil || claims.Role != "ADMIN" {
		http.Error(w, `{"error":"forbidden","message":"Löschen von Kontakten erfordert Administrator-Rechte"}`, http.StatusForbidden)
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
		http.Error(w, `{"error":"failed to delete contact"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
