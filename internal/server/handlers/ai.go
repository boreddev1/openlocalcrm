package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
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
	querier      db.Querier
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
	querier db.Querier,
) *AIHandler {
	return &AIHandler{
		triageSvc:    triageSvc,
		chatSvc:      chatSvc,
		researchSvc:  researchSvc,
		obsSvc:       obsSvc,
		gateway:      gateway,
		querier:      querier,
		kbArticles:   []KBArticle{},
		researchJobs: []ResearchJob{},
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
		if errors.Is(err, ai.ErrUpstreamUnavailable) {
			http.Error(w, `{"error":"ai_unavailable","message":"KI-Dienst nicht erreichbar"}`, http.StatusBadGateway)
			return
		}
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
		if errors.Is(err, ai.ErrUpstreamUnavailable) {
			http.Error(w, `{"error":"ai_unavailable","message":"KI-Dienst nicht erreichbar"}`, http.StatusBadGateway)
			return
		}
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
		if errors.Is(err, ai.ErrUpstreamUnavailable) {
			http.Error(w, `{"error":"ai_unavailable","message":"KI-Dienst nicht erreichbar"}`, http.StatusBadGateway)
			return
		}
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
		DocumentText string `json:"document_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.DocumentText) == "" {
		http.Error(w, `{"error":"document_text_required","message":"Es wurde kein Rechnungs- oder Zählertext übermittelt"}`, http.StatusBadRequest)
		return
	}

	if h.gateway == nil {
		http.Error(w, `{"error":"ai_unavailable","message":"KI-Dienst nicht erreichbar"}`, http.StatusBadGateway)
		return
	}

	prompt := fmt.Sprintf(`Extrahiere aus folgendem Energierechnungs-Text die Daten für einen PV-Angebotsrechner im JSON-Format:
Text: %s
Erwartetes JSON-Format:
{
  "customer_name": "Name",
  "yearly_consumption": 6500,
  "current_electricity_price": 0.385,
  "meter_number": "1EMH...",
  "roof_area_sqm": 75,
  "roof_orientation": "Süd (35° Dachneigung)",
  "recommended_kwp": 14.5,
  "recommended_storage_kwh": 12.0
}`, req.DocumentText)

	out, err := h.gateway.Generate(r.Context(), prompt, "Du bist ein präziser OCR-Parser für Energierechnungen. Antworte ausschließlich mit gültigem JSON.")
	if err != nil {
		http.Error(w, `{"error":"ai_unavailable","message":"KI-Dienst nicht erreichbar"}`, http.StatusBadGateway)
		return
	}

	cleaned := strings.TrimSpace(out)
	if idx := strings.Index(cleaned, "{"); idx >= 0 {
		if endIdx := strings.LastIndex(cleaned, "}"); endIdx > idx {
			cleaned = cleaned[idx : endIdx+1]
		}
	}
	var parsedData map[string]any
	if err := json.Unmarshal([]byte(cleaned), &parsedData); err != nil {
		http.Error(w, `{"error":"ai_invalid_response","message":"KI-Antwort konnte nicht als JSON verarbeitet werden"}`, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"simulated": false,
		"mode":      "ai_ocr_extraction",
		"extracted": parsedData,
	})
}

func (h *AIHandler) ListKB(w http.ResponseWriter, r *http.Request) {
	if h.querier != nil {
		dbArticles, err := h.querier.ListKBArticles(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		res := make([]KBArticle, len(dbArticles))
		for i, a := range dbArticles {
			res[i] = KBArticle{
				ID:          uuid.UUID(a.ID.Bytes).String(),
				Title:       a.Title,
				Category:    a.Category,
				Content:     a.Content,
				Source:      a.Author,
				ChunksCount: 4,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	articles := h.kbArticles
	if articles == nil {
		articles = []KBArticle{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(articles)
}

func (h *AIHandler) CreateKB(w http.ResponseWriter, r *http.Request) {
	var doc KBArticle
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if doc.ChunksCount == 0 {
		words := len(strings.Fields(doc.Content))
		doc.ChunksCount = words/150 + 1
	}

	if h.querier != nil {
		created, err := h.querier.CreateKBArticle(r.Context(), db.CreateKBArticleParams{
			Title:    doc.Title,
			Category: doc.Category,
			Content:  doc.Content,
			Tags:     []string{doc.Category},
			Author:   "System",
		})
		if err == nil {
			doc.ID = uuid.UUID(created.ID.Bytes).String()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(doc)
			return
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	doc.ID = fmt.Sprintf("kb-%d", time.Now().UnixNano())
	h.kbArticles = append([]KBArticle{doc}, h.kbArticles...)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(doc)
}

func (h *AIHandler) DeleteKB(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	u, err := uuid.Parse(id)
	if err != nil {
		http.Error(w, `{"error":"invalid_id","message":"Ungültige UUID für KB-Artikel"}`, http.StatusBadRequest)
		return
	}

	if h.querier != nil {
		if err := h.querier.DeleteKBArticle(r.Context(), pgtype.UUID{Bytes: u, Valid: true}); err != nil {
			http.Error(w, `{"error":"failed to delete kb article: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
	}

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
	if h.querier != nil {
		dbJobs, err := h.querier.ListAIResearchJobs(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		res := make([]ResearchJob, len(dbJobs))
		for i, j := range dbJobs {
			res[i] = ResearchJob{
				ID:          uuid.UUID(j.ID.Bytes).String(),
				Query:       j.Domain,
				CompanyName: j.CompanyName,
				Category:    "Technik",
				Depth:       "DEEP",
				Status:      j.Status,
				Result: &ResearchResult{
					SiteTitle:      j.CompanyName + " - Analyse",
					Summary:        "Automatisch erstellte Firmenrecherche.",
					DecisionMakers: []string{"Geschäftsführung (" + j.Domain + ")"},
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	jobs := h.researchJobs
	if jobs == nil {
		jobs = []ResearchJob{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jobs)
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

	siteTitle := domain
	summary := ""
	decisionMakers := []string{}
	status := "COMPLETED"

	if h.researchSvc != nil {
		res, err := h.researchSvc.ResearchCompany(r.Context(), domain)
		if err != nil {
			status = "FAILED"
			summary = fmt.Sprintf("Recherche für %s fehlgeschlagen: %v", domain, err)
		} else {
			if res.Title != "" {
				siteTitle = res.Title
			}
			if res.Summary != "" {
				summary = res.Summary
			}
			if len(res.IndustryKeywords) > 0 {
				decisionMakers = res.IndustryKeywords
			}
		}
	} else {
		status = "FAILED"
		summary = "Recherche-Dienst nicht konfiguriert."
	}

	jobID := fmt.Sprintf("job-%d", time.Now().UnixNano())
	if h.querier != nil {
		created, err := h.querier.CreateAIResearchJob(r.Context(), db.CreateAIResearchJobParams{
			CompanyName: domain,
			Domain:      domain,
			Status:      status,
		})
		if err == nil {
			jobID = uuid.UUID(created.ID.Bytes).String()
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	job := ResearchJob{
		ID:          jobID,
		Query:       domain,
		CompanyName: domain,
		Category:    req.Category,
		Depth:       req.Depth,
		Status:      status,
		Result: &ResearchResult{
			SiteTitle:      siteTitle,
			Summary:        summary,
			DecisionMakers: decisionMakers,
		},
	}
	h.researchJobs = append([]ResearchJob{job}, h.researchJobs...)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(job)
}
