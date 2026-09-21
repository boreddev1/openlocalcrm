package ai

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type AIAuditLog struct {
	ID                 string    `json:"id"`
	InteractionType    string    `json:"interaction_type"` // TRIAGE, RESEARCH, CHAT_COPILOT, DRAFT_REPLY
	ModelName          string    `json:"model_name"`
	Provider           string    `json:"provider"`
	PromptTokens       int       `json:"prompt_tokens"`
	CompletionTokens   int       `json:"completion_tokens"`
	LatencyMs          int       `json:"latency_ms"`
	PIIFilterTriggered bool      `json:"pii_filter_triggered"`
	PIIRedactionsCount int       `json:"pii_redactions_count"`
	HumanApproved      *bool     `json:"human_approved,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type ObservabilityStats struct {
	TotalInteractions  int     `json:"total_interactions"`
	TotalPIIBlocked    int     `json:"total_pii_blocked"`
	AverageLatencyMs   float64 `json:"average_latency_ms"`
	ComplianceStandard string  `json:"compliance_standard"` // "EU AI Act (Transparenz & Auditierbarkeit nach Art. 50/52)"
	ActiveModel        string  `json:"active_model"`
	Provider           string  `json:"provider"`
}

type ObservabilityService struct {
	mu   sync.RWMutex
	logs []AIAuditLog
	dbtx db.DBTX
}

func NewObservabilityService(dbtx ...db.DBTX) *ObservabilityService {
	s := &ObservabilityService{
		logs: make([]AIAuditLog, 0),
	}
	if len(dbtx) > 0 {
		s.dbtx = dbtx[0]
	}
	return s
}

func (s *ObservabilityService) Record(ctx context.Context, log AIAuditLog) {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	if log.ID == "" {
		log.ID = uuid.New().String()
	}

	// Finding #3: Persist to Postgres ai_audit_logs if DB connection is available
	if s.dbtx != nil {
		idUUID, err := uuid.Parse(log.ID)
		if err != nil {
			idUUID = uuid.New()
			log.ID = idUUID.String()
		}
		query := `INSERT INTO ai_audit_logs (
			id, interaction_type, model_name, provider,
			prompt_tokens, completion_tokens, latency_ms,
			pii_filter_triggered, pii_redactions_count, human_approved, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
		_, _ = s.dbtx.Exec(ctx, query,
			idUUID, log.InteractionType, log.ModelName, log.Provider,
			log.PromptTokens, log.CompletionTokens, log.LatencyMs,
			log.PIIFilterTriggered, log.PIIRedactionsCount, log.HumanApproved, log.CreatedAt,
		)
	}

	// Always retain in memory for quick dashboard querying and demo mode
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append([]AIAuditLog{log}, s.logs...)
	if len(s.logs) > 500 {
		s.logs = s.logs[:500]
	}
}

func (s *ObservabilityService) GetRecentLogs(limit int) []AIAuditLog {
	// Finding #39: Prevent crash/panic on negative or zero limit
	if limit <= 0 {
		return []AIAuditLog{}
	}

	// Try fetching from database first if available
	if s.dbtx != nil {
		query := `SELECT id, interaction_type, model_name, provider,
		                 prompt_tokens, completion_tokens, latency_ms,
		                 pii_filter_triggered, pii_redactions_count, human_approved, created_at
		          FROM ai_audit_logs
		          ORDER BY created_at DESC
		          LIMIT $1`
		rows, err := s.dbtx.Query(context.Background(), query, limit)
		if err == nil {
			defer rows.Close()
			var dbLogs []AIAuditLog
			for rows.Next() {
				var l AIAuditLog
				var idUUID pgtype.UUID
				var humanApproved *bool
				if scanErr := rows.Scan(
					&idUUID, &l.InteractionType, &l.ModelName, &l.Provider,
					&l.PromptTokens, &l.CompletionTokens, &l.LatencyMs,
					&l.PIIFilterTriggered, &l.PIIRedactionsCount, &humanApproved, &l.CreatedAt,
				); scanErr == nil {
					l.ID = uuid.UUID(idUUID.Bytes).String()
					l.HumanApproved = humanApproved
					dbLogs = append(dbLogs, l)
				}
			}
			if len(dbLogs) > 0 {
				return dbLogs
			}
		}
	}

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
