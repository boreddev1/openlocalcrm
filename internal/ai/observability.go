package ai

import (
	"context"
	"sync"
	"time"
)

type AIAuditLog struct {
	ID                  string    `json:"id"`
	InteractionType     string    `json:"interaction_type"` // TRIAGE, RESEARCH, CHAT_COPILOT, DRAFT_REPLY
	ModelName           string    `json:"model_name"`
	Provider            string    `json:"provider"`
	PromptTokens        int       `json:"prompt_tokens"`
	CompletionTokens    int       `json:"completion_tokens"`
	LatencyMs           int       `json:"latency_ms"`
	PIIFilterTriggered  bool      `json:"pii_filter_triggered"`
	PIIRedactionsCount  int       `json:"pii_redactions_count"`
	HumanApproved       *bool     `json:"human_approved,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

type ObservabilityStats struct {
	TotalInteractions   int     `json:"total_interactions"`
	TotalPIIBlocked     int     `json:"total_pii_blocked"`
	AverageLatencyMs    float64 `json:"average_latency_ms"`
	ComplianceStandard  string  `json:"compliance_standard"` // "EU AI Act (Transparenz & Auditierbarkeit nach Art. 50/52)"
	ActiveModel         string  `json:"active_model"`
	Provider            string  `json:"provider"`
}

type ObservabilityService struct {
	mu   sync.RWMutex
	logs []AIAuditLog
}

func NewObservabilityService() *ObservabilityService {
	return &ObservabilityService{
		logs: make([]AIAuditLog, 0),
	}
}

func (s *ObservabilityService) Record(ctx context.Context, log AIAuditLog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	s.logs = append([]AIAuditLog{log}, s.logs...)
	if len(s.logs) > 500 {
		s.logs = s.logs[:500]
	}
}

func (s *ObservabilityService) GetRecentLogs(limit int) []AIAuditLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit > len(s.logs) {
		limit = len(s.logs)
	}
	res := make([]AIAuditLog, limit)
	copy(res, s.logs[:limit])
	return res
}

func (s *ObservabilityService) GetStats() ObservabilityStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.logs)
	var totalLatency int
	var piiCount int

	for _, l := range s.logs {
		totalLatency += l.LatencyMs
		if l.PIIFilterTriggered {
			piiCount += l.PIIRedactionsCount
		}
	}

	var avgLatency float64
	if total > 0 {
		avgLatency = float64(totalLatency) / float64(total)
	}

	return ObservabilityStats{
		TotalInteractions:  total,
		TotalPIIBlocked:    piiCount,
		AverageLatencyMs:   avgLatency,
		ComplianceStandard: "EU AI Act konform (Art. 50/52 Transparenz & Auditierbarkeit)",
		ActiveModel:        "gemma4:12b",
		Provider:           "Ollama (Local On-Premise)",
	}
}
