package ai

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type ChatMessage struct {
	Role    string `json:"role"` // "user", "assistant", "system"
	Content string `json:"content"`
}

type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
	Context  string        `json:"context,omitempty"` // e.g. "Deals Pipeline: 5 open deals, 106.000 € Volume"
}

type ChatResponse struct {
	Reply              string `json:"reply"`
	Model              string `json:"model"`
	LatencyMs          int    `json:"latency_ms"`
	PIIFilterTriggered bool   `json:"pii_filter_triggered"`
}

type ChatService struct {
	gateway *Gateway
	obsSvc  *ObservabilityService
}

func NewChatService(gateway *Gateway, obsSvc *ObservabilityService) *ChatService {
	return &ChatService{
		gateway: gateway,
		obsSvc:  obsSvc,
	}
}

func (s *ChatService) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	startTime := time.Now()

	// Extract conversation and context
	var conv strings.Builder
	for _, m := range req.Messages {
		conv.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(m.Role), m.Content))
	}

	systemInstruction := `Du bist der OpenLocalCRM KI-Vertriebsassistent und Copilot.
Deine Aufgaben:
1. Unterstütze Vertriebsmitarbeiter bei Kundenfragen, Angebotserstellung (PV, Speicher, Wärmepumpe, B2B-Verträge) und Pipeline-Analysen.
2. Formuliere prägnant, professionell, lösungsorientiert und freundlich auf Deutsch.
3. Beachte stets deutsche Rechtsnormen (UWG § 7, BGB § 355 Widerrufsrecht, DSGVO).`

	if req.Context != "" {
		systemInstruction += fmt.Sprintf("\n\nAktueller CRM-Kontext des Benutzers:\n%s", req.Context)
	}

	reply, err := s.gateway.Generate(ctx, conv.String(), systemInstruction)
	latency := int(time.Since(startTime).Milliseconds())

	if err != nil {
		reply = "Ich stehe als KI-Assistent zur Verfügung. Wie kann ich Sie bei Ihren Deals oder Kontakten unterstützen?"
	}

	if s.obsSvc != nil {
		s.obsSvc.Record(ctx, AIAuditLog{
			ID:                 fmt.Sprintf("chat-%d", time.Now().UnixNano()),
			InteractionType:    "CHAT_COPILOT",
			ModelName:          s.gateway.cfg.OllamaModel,
			Provider:           string(s.gateway.cfg.DefaultProvider),
			LatencyMs:          latency,
			PIIFilterTriggered: false,
			CreatedAt:          time.Now(),
		})
	}

	return ChatResponse{
		Reply:     reply,
		Model:     s.gateway.cfg.OllamaModel,
		LatencyMs: latency,
	}, nil
}
