package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

type KBArticle struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Source      string `json:"source"`
	Content     string `json:"content"`
	ChunksCount int    `json:"chunks_count"`
}

type ResearchJob struct {
	ID          string          `json:"id"`
	Query       string          `json:"query"`
	CompanyName string          `json:"company_name"`
	Category    string          `json:"category"`
	Depth       string          `json:"depth"`
	Status      string          `json:"status"`
	Result      *ResearchResult `json:"result,omitempty"`
}

type ResearchResult struct {
	SiteTitle      string   `json:"site_title"`
	Summary        string   `json:"summary"`
	DecisionMakers []string `json:"decision_makers"`
}

type AIHandler struct {
	triageSvc    *ai.TriageService
	chatSvc      *ai.ChatService
	researchSvc  *ai.ResearchService
	obsSvc       *ai.ObservabilityService
	gateway      *ai.Gateway
	mu           sync.RWMutex
	kbArticles   []KBArticle
	researchJobs []ResearchJob
}

func NewAIHandler(
	triageSvc *ai.TriageService,
	chatSvc *ai.ChatService,
	researchSvc *ai.ResearchService,
	obsSvc *ai.ObservabilityService,
	gateway *ai.Gateway,
) *AIHandler {
	return &AIHandler{
		triageSvc:   triageSvc,
		chatSvc:     chatSvc,
		researchSvc: researchSvc,
		obsSvc:      obsSvc,
		gateway:     gateway,
		kbArticles: []KBArticle{
			{
				ID:          "kb-1",
				Title:       "Technisches Handbuch Solarsysteme 2026",
				Category:    "Photovoltaik & Speicher",
				Source:      "Technisches_Handbuch_2026.pdf",
				Content:     "Richtlinien zur Auslegung von PV-Anlagen nach EEG 2026 und DIN VDE AR-N 4105. Glas-Glas TOPCon Module erreichen 30 Jahre Leistungsgarantie.",
				ChunksCount: 4,
			},
			{
				ID:          "kb-2",
				Title:       "Vergütungssätze & Einspeisemanagement 2026",
				Category:    "Recht & Compliance",
				Source:      "Bundesnetzagentur_EEG_2026.pdf",
				Content:     "Überschusseinspeisung bis 10 kWp mit 8,1 Cent/kWh. Keine Rundsteuerempfängerpflicht mehr für Anlagen bis 25 kWp mit iMSys.",
				ChunksCount: 3,
			},
		},
		researchJobs: []ResearchJob{
			{
				ID:          "job-1",
				Query:       "energie-dach.de",
				CompanyName: "Energie Dach GmbH",
				Category:    "Gewerbesolar & Hallendach",
				Depth:       "deep",
				Status:      "COMPLETED",
				Result: &ResearchResult{
					SiteTitle:      "Energie Dach GmbH - Solartechnik & Hallenbau",
					Summary:        "Führender Anbieter für Aufdach-Photovoltaik im Gewerbesegment in Süddeutschland. Große Hallendachflächen, eigene Montagekolonnen und über 15 MWp realisierte Leistung.",
					DecisionMakers: []string{"Dr. Michael Weber (Geschäftsführer)", "Markus Huber (Technischer Leiter)"},
				},
			},
		},
	}
}

type TriageRequest struct {
	Sender  string `json:"sender"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (h *AIHandler) TriageEmail(w http.ResponseWriter, r *http.Request) {
	var req TriageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	result, err := h.triageSvc.TriageEmail(r.Context(), req.Sender, req.Subject, req.Body)
	if err != nil {
		http.Error(w, `{"error":"ai triage failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *AIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	var req ai.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid chat request"}`, http.StatusBadRequest)
		return
	}

	res, err := h.chatSvc.Chat(r.Context(), req)
	if err != nil {
		http.Error(w, `{"error":"ai chat failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

type CompanyResearchReq struct {
	Domain string `json:"domain"`
}

func (h *AIHandler) ResearchCompany(w http.ResponseWriter, r *http.Request) {
	var req CompanyResearchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid research request"}`, http.StatusBadRequest)
		return
	}

	res, err := h.researchSvc.ResearchCompany(r.Context(), req.Domain)
	if err != nil {
		http.Error(w, `{"error":"company research failed: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *AIHandler) GetObservability(w http.ResponseWriter, r *http.Request) {
	stats := h.obsSvc.GetStats()
	recentLogs := h.obsSvc.GetRecentLogs(20)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"stats":       stats,
		"recent_logs": recentLogs,
	})
}

func (h *AIHandler) ParseBill(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CustomerName string `json:"customer_name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	custName := req.CustomerName
	if custName == "" {
		custName = "Familie Müller"
	}

	res := map[string]any{
		"extracted": map[string]any{
			"customer_name":             custName,
			"yearly_consumption":        6500,
			"current_electricity_price": 0.385,
			"meter_number":              "1EMH0012948291",
			"roof_area_sqm":             75,
			"roof_orientation":          "Süd (35° Dachneigung)",
			"recommended_kwp":           14.5,
			"recommended_storage_kwh":   12.0,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *AIHandler) ListKB(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.kbArticles)
}

func (h *AIHandler) CreateKB(w http.ResponseWriter, r *http.Request) {
	var doc KBArticle
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	doc.ID = fmt.Sprintf("kb-%d", time.Now().UnixNano())
	if doc.ChunksCount == 0 {
		doc.ChunksCount = 4
	}
	h.kbArticles = append([]KBArticle{doc}, h.kbArticles...)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(doc)
}

func (h *AIHandler) DeleteKB(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	h.mu.Lock()
	defer h.mu.Unlock()

	filtered := make([]KBArticle, 0, len(h.kbArticles))
	for _, a := range h.kbArticles {
		if a.ID != id {
			filtered = append(filtered, a)
		}
	}
	h.kbArticles = filtered

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *AIHandler) ListResearchJobs(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.researchJobs)
}

func (h *AIHandler) CreateResearchJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain   string `json:"domain"`
		Depth    string `json:"depth"`
		Category string `json:"category"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	domain := req.Domain
	if domain == "" {
		domain = "energie-dach.de"
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	job := ResearchJob{
		ID:          fmt.Sprintf("job-%d", time.Now().UnixNano()),
		Query:       domain,
		CompanyName: domain,
		Category:    req.Category,
		Depth:       req.Depth,
		Status:      "COMPLETED",
		Result: &ResearchResult{
			SiteTitle:      domain + " - Gewerbliche Photovoltaik",
			Summary:        "Geprüftes Gewerbeunternehmen mit hoher Dachflächen-Eignung für Photovoltaik-Großanlagen.",
			DecisionMakers: []string{"Geschäftsführung (" + domain + ")"},
		},
	}
	h.researchJobs = append([]ResearchJob{job}, h.researchJobs...)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(job)
}
