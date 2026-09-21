package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"time"

	"github.com/jackc/pgx/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/email"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/queue"
	"github.com/riverqueue/river"
)

func aiEnv(key, fallbackKey, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if fallbackKey != "" {
		if v := os.Getenv(fallbackKey); v != "" {
			return v
		}
	}
	return def
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://crm_user:crm_pass@localhost:5432/crm_db?sslmode=disable"
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Println("Connecting openlocalcrm crm-worker to PostgreSQL...")
	dbPool, err := db.ConnectWithRetry(ctx, dbURL, 15, 1*time.Second)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer dbPool.Close()

	queries := db.New(dbPool)
	auditSvc := audit.NewService(queries)
	emailSvc := email.NewService(queries, auditSvc, nil, nil)

	obsSvc := ai.NewObservabilityService(dbPool)
	aiGateway := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.Provider(aiEnv("AI_PROVIDER", "", "ollama")),
		OllamaBaseURL:   aiEnv("OLLAMA_BASE_URL", "AI_BASE_URL", "http://localhost:11434"),
		OllamaModel:     aiEnv("OLLAMA_MODEL", "AI_MODEL", "mistral"),
		APIKey:          os.Getenv("AI_API_KEY"),
	})
	triageSvc := ai.NewTriageService(aiGateway, obsSvc)

	deps := queue.Deps{
		Queries:  queries,
		EmailSvc: emailSvc,
		Triager:  triageSvc,
	}

	var client *river.Client[pgx.Tx]
	for attempt := 1; attempt <= 10; attempt++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		client, err = queue.NewClient(ctx, dbPool, deps)
		if err == nil {
			break
		}

		log.Printf("[Worker] River queue initialization attempt %d/10 failed: %v. Retrying in 2s...", attempt, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	if err != nil {
		log.Fatalf("failed to initialize river queue client: %v", err)
	}

	log.Println("Starting openlocalcrm crm-worker job consumers...")
	if err := client.Start(ctx); err != nil {
		log.Fatalf("river queue error: %v", err)
	}

	<-ctx.Done()
	log.Println("Shutting down crm-worker gracefully...")
	_ = client.Stop(context.Background())
	log.Println("crm-worker stopped.")
}
