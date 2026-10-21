package ai

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type AIAuditLog struct {
	ID                 string    `json:"id"`
	UserID             *string   `json:"user_id,omitempty"`
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
	// gateway is the serving gateway whose model name the stats report. Wired
	// once at startup before any request (issue #72); nil keeps the legacy
	// environment-based resolution for standalone use and tests.
	gateway *Gateway
}

// WithGateway binds the gateway that actually serves inferences, so
// GetStats reports the model the instance really uses — including the
// DemoStubMarker in demo mode — instead of re-guessing from the environment.
func (s *ObservabilityService) WithGateway(g *Gateway) *ObservabilityService {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gateway = g
	return s
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
	if log.UserID == nil && ctx != nil {
		if claims, ok := ctx.Value(auth.UserContextKey).(*auth.AccessClaims); ok && claims != nil {
			uStr := claims.UserID.String()
			log.UserID = &uStr
		}
	}

	// Finding #3 / F-35: Persist to Postgres ai_audit_logs if DB connection is available
	if s.dbtx != nil {
		idUUID, err := uuid.Parse(log.ID)
		if err != nil {
			idUUID = uuid.New()
			log.ID = idUUID.String()
		}
		var userUUID *uuid.UUID
		if log.UserID != nil && *log.UserID != "" {
			if u, err := uuid.Parse(*log.UserID); err == nil {
				userUUID = &u
			}
		}
		query := `INSERT INTO ai_audit_logs (
			id, interaction_type, model_name, provider,
			prompt_tokens, completion_tokens, latency_ms,
			pii_filter_triggered, pii_redactions_count, human_approved, user_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
		_, _ = s.dbtx.Exec(ctx, query,
			idUUID, log.InteractionType, log.ModelName, log.Provider,
			log.PromptTokens, log.CompletionTokens, log.LatencyMs,
			log.PIIFilterTriggered, log.PIIRedactionsCount, log.HumanApproved, userUUID, log.CreatedAt,
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
		                 pii_filter_triggered, pii_redactions_count, human_approved, user_id, created_at
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
				var userUUID pgtype.UUID
				if scanErr := rows.Scan(
					&idUUID, &l.InteractionType, &l.ModelName, &l.Provider,
					&l.PromptTokens, &l.CompletionTokens, &l.LatencyMs,
					&l.PIIFilterTriggered, &l.PIIRedactionsCount, &humanApproved, &userUUID, &l.CreatedAt,
				); scanErr == nil {
					l.ID = uuid.UUID(idUUID.Bytes).String()
					l.HumanApproved = humanApproved
					if userUUID.Valid {
						uStr := uuid.UUID(userUUID.Bytes).String()
						l.UserID = &uStr
					}
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
	var gatewayModel string
	if s.gateway != nil {
		gatewayModel = s.gateway.ServingModel()
	}
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

	// Issue #72: the gateway is the authority on which model is actually
	// served (demo marker included). Only an unwired service may fall back to
	// environment guessing.
	activeModel := gatewayModel
	if activeModel == "" {
		activeModel = os.Getenv("OLLAMA_MODEL")
	}
	if activeModel == "" {
		activeModel = os.Getenv("AI_MODEL")
	}
	if activeModel == "" {
		activeModel = DefaultOllamaModel
	}

	provider := os.Getenv("AI_PROVIDER")
	if provider == "" {
		provider = "Ollama (Local On-Premise)"
	}

	return ObservabilityStats{
		TotalInteractions:  total,
		TotalPIIBlocked:    piiCount,
		AverageLatencyMs:   avgLatency,
		ComplianceStandard: "EU AI Act konform (Art. 50/52 Transparenz & Auditierbarkeit)",
		ActiveModel:        activeModel,
		Provider:           provider,
	}
}
