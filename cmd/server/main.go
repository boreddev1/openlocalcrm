package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
	"github.com/openlocalcrm/openlocalcrm/internal/storage"
)

func loadOrGenerateKey(keyPath string, isDemoMode bool) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	if keyPath == "" {
		keyPath = "./data/storage/keys/ed25519.key"
	}

	if data, err := os.ReadFile(keyPath); err == nil && len(data) == ed25519.PrivateKeySize {
		privKey := ed25519.PrivateKey(data)
		pubKey := privKey.Public().(ed25519.PublicKey)
		log.Printf("[Crypto] Loaded persistent Ed25519 keypair from %s", keyPath)
		return pubKey, privKey, nil
	}

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	keyDir := filepath.Dir(keyPath)
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		if !isDemoMode {
			return nil, nil, fmt.Errorf("failed creating key directory %s: %w", keyDir, err)
		}
	} else if err := os.WriteFile(keyPath, privKey, 0600); err != nil {
		if !isDemoMode {
			return nil, nil, fmt.Errorf("failed persisting Ed25519 key at %s: %w", keyPath, err)
		}
	} else {
		log.Printf("[Crypto] Generated and persisted new Ed25519 keypair to %s", keyPath)
		return pubKey, privKey, nil
	}

	if !isDemoMode {
		return nil, nil, fmt.Errorf("production mode requires persistent key storage at %s, cannot fallback to RAM", keyPath)
	}

	log.Printf("[Crypto] [DEMO ONLY] Using in-memory Ed25519 keypair")
	return pubKey, privKey, nil
}

func bootstrapAdminUser(ctx context.Context, querier db.Querier) {
	count, err := querier.CountUsers(ctx)
	if err == nil && count == 0 {
		password := os.Getenv("INITIAL_ADMIN_PASSWORD")
		adminEmail := os.Getenv("INITIAL_ADMIN_EMAIL")
		if adminEmail == "" {
			adminEmail = "admin@openlocalcrm.local"
		}
		adminEmail = strings.TrimSpace(strings.ToLower(adminEmail))

		if password == "" {
			pwBytes := make([]byte, 16)
			_, _ = rand.Read(pwBytes)
			password = hex.EncodeToString(pwBytes)
			log.Printf("==================================================================")
			log.Printf("[BOOTSTRAP] GENERATED SECURE INITIAL ADMIN PASSWORD:")
			log.Printf("User:     %s", adminEmail)
			log.Printf("Password: %s", password)
			log.Printf("PLEASE CHANGE THIS PASSWORD IMMEDIATELY UPON FIRST LOGIN!")
			log.Printf("==================================================================")
		} else {
			log.Printf("[Bootstrap] Using INITIAL_ADMIN_PASSWORD from environment for %s", adminEmail)
		}

		hash, hashErr := auth.HashPassword(password)
		if hashErr != nil {
			log.Fatalf("[FATAL] Failed hashing admin password: %v", hashErr)
		}

		firstName := os.Getenv("INITIAL_ADMIN_FIRST_NAME")
		if firstName == "" {
			firstName = "Admin"
		}
		lastName := os.Getenv("INITIAL_ADMIN_LAST_NAME")
		if lastName == "" {
			lastName = "User"
		}

		_, createErr := querier.CreateUser(ctx, db.CreateUserParams{
			Email:        adminEmail,
			PasswordHash: hash,
			FirstName:    firstName,
			LastName:     lastName,
			Role:         "ADMIN",
			Status:       "ACTIVE",
		})
		if createErr != nil {
			log.Fatalf("[FATAL] Failed initializing default admin user: %v", createErr)
		}
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://crm_user:crm_pass@localhost:5432/crm_db?sslmode=disable"
	}
	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = os.Getenv("STORAGE_LOCAL_DIR")
	}
	if storagePath == "" {
		storagePath = "./data/storage"
	}
	jwtKeyPath := os.Getenv("JWT_SECRET_KEY_PATH")
	if jwtKeyPath == "" {
		jwtKeyPath = os.Getenv("JWT_PRIVATE_KEY_PATH")
	}
	if jwtKeyPath == "" {
		jwtKeyPath = filepath.Join(storagePath, "keys", "ed25519.key")
	}

	isDemoMode := strings.EqualFold(os.Getenv("DEMO_MODE"), "true")
	if isDemoMode {
		log.Println("******************************************************************")
		log.Println("  [DEMO MODE ACTIVE] Running with in-memory sample database.")
		log.Println("  No external PostgreSQL required. Pre-seeded with CRM test data.")
		log.Println("  Demo Login: admin@openlocalcrm.local / demo123 (or vertrieb@openlocalcrm.local / demo123)")
		log.Println("******************************************************************")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// A9: verify the configured embedding model matches the fixed pgvector(1024)
	// schema before serving. A dimension mismatch is a hard configuration error
	// (resizing requires an explicit migration); an unreachable backend or a
	// provider without an embeddings API — e.g. CI/E2E without a local Ollama —
	// only warns and disables semantic search, every later embedding call
	// returns the same honest error.
	// 30 s fixed cap: cold Ollama model loads (multi-second first download/load)
	// must not fail the check falsely, while an unbounded generation timeout
	// (OLLAMA_TIMEOUT_SECONDS) must never block boot. Timeout is warning-only;
	// a dimension mismatch remains fatal.
	embeddingCtx, cancelEmbeddingCheck := context.WithTimeout(ctx, 30*time.Second)
	embeddingErr := ai.ValidateEmbeddingModelFromEnv(embeddingCtx)
	cancelEmbeddingCheck()
	if embeddingErr != nil {
		if errors.Is(embeddingErr, ai.ErrEmbeddingDimensionMismatch) {
			log.Fatalf("[FATAL] Embedding-Konfiguration ungültig: %v", embeddingErr)
		}
		log.Printf("[WARN] Embedding-Modell nicht erreichbar oder ohne Embedding-Support, semantische Suche deaktiviert: %v", embeddingErr)
	}

	// Ed25519 Keys
	pubKey, privKey, err := loadOrGenerateKey(jwtKeyPath, isDemoMode)
	if err != nil {
		log.Fatalf("failed initializing cryptographic keys: %v", err)
	}

	// Initialize Storage & SSE Hub
	localStorage := storage.NewLocalStorage(storagePath)
	sseHub := sse.NewHub()

	var querier db.Querier
	if isDemoMode {
		querier = demo.NewInMemoryQuerier()
	} else {
		dbPool, err := db.ConnectWithRetry(ctx, dbURL, 15, 1*time.Second)
		if err != nil {
			log.Fatalf("[FATAL] PostgreSQL database connection failed: %v", err)
		}

		if migErr := db.RunMigrations(ctx, dbPool); migErr != nil {
			log.Fatalf("[FATAL] Database migrations failed: %v", migErr)
		}

		querier = db.New(dbPool)
		bootstrapAdminUser(ctx, querier)
		defer dbPool.Close()
	}

	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sseHub,
		Storage:  localStorage,
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: isDemoMode,
		Context:  ctx,
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // Allow SSE streaming
	}

	go func() {
		log.Printf("openlocalcrm crm-server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down crm-server gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[SHUTDOWN_WARNING] Error shutting down HTTP server: %v", err)
	}
	log.Println("crm-server stopped.")
}
