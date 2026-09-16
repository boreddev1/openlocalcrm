package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

type AIHandler struct {
	triageSvc   *ai.TriageService
	chatSvc     *ai.ChatService
	researchSvc *ai.ResearchService
	obsSvc      *ai.ObservabilityService
	gateway     *ai.Gateway
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
