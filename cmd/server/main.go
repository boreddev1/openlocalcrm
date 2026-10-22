package main

import (
	"bytes"
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

	"github.com/jackc/pgx/v5/pgxpool"
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

	if data, err := os.ReadFile(keyPath); err == nil && len(data) == ed25519.PrivateKeySize { //nolint:gosec // G703: path comes from operator-controlled configuration or fixed launcher locations, not request input
		privKey := ed25519.PrivateKey(data)
		pubKey := privKey.Public().(ed25519.PublicKey)
		log.Printf("[Crypto] Loaded persistent Ed25519 keypair from %s", keyPath) //nolint:gosec // G706: logged value is operator configuration (env/paths/port) or a server-generated ID, not attacker-controlled free text
		return pubKey, privKey, nil
	}

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	keyDir := filepath.Dir(keyPath)
	if err := os.MkdirAll(keyDir, 0700); err != nil { //nolint:gosec // G703: path comes from operator-controlled configuration or fixed launcher locations, not request input
		if !isDemoMode {
			return nil, nil, fmt.Errorf("failed creating key directory %s: %w", keyDir, err)
		}
	} else if err := os.WriteFile(keyPath, privKey, 0600); err != nil { //nolint:gosec // G703: path comes from operator-controlled configuration or fixed launcher locations, not request input
		if !isDemoMode {
			return nil, nil, fmt.Errorf("failed persisting Ed25519 key at %s: %w", keyPath, err)
		}
	} else {
		log.Printf("[Crypto] Generated and persisted new Ed25519 keypair to %s", keyPath) //nolint:gosec // G706: logged value is operator configuration (env/paths/port) or a server-generated ID, not attacker-controlled free text
		return pubKey, privKey, nil
	}

	if !isDemoMode {
		return nil, nil, fmt.Errorf("production mode requires persistent key storage at %s, cannot fallback to RAM", keyPath)
	}

	log.Printf("[Crypto] [DEMO ONLY] Using in-memory Ed25519 keypair")
	return pubKey, privKey, nil
}

// writeBootstrapSecret stores a generated administrator password in the storage
// directory with owner-only permissions and returns its path. The value is never
// written to the process log.
func writeBootstrapSecret(storagePath, email, password string) error {
	path := filepath.Join(storagePath, "bootstrap-admin.txt")
	body := fmt.Sprintf("email=%s\npassword=%s\nwritten=%s\n", email, password, time.Now().UTC().Format(time.RFC3339))
	return os.WriteFile(path, []byte(body), 0o600) //nolint:gosec // G703: path comes from operator-controlled configuration or fixed launcher locations, not request input
}

func bootstrapAdminUser(ctx context.Context, querier db.Querier, storagePath string) {
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
			secretPath := filepath.Join(storagePath, "bootstrap-admin.txt")
			if err := writeBootstrapSecret(storagePath, adminEmail, password); err != nil {
				log.Printf("[BOOTSTRAP_ERROR] Failed writing bootstrap secret to %s: %v", secretPath, err) //nolint:gosec // G706: logged value is operator configuration (env/paths/port) or a server-generated ID, not attacker-controlled free text
			} else {
				log.Printf("[BOOTSTRAP] Initial admin password written to %s (permissions 0600). Please change this password immediately upon first login!", secretPath) //nolint:gosec // G706: logged value is operator configuration (env/paths/port) or a server-generated ID, not attacker-controlled free text
			}
		} else {
			log.Printf("[Bootstrap] Using INITIAL_ADMIN_PASSWORD from environment for %s", adminEmail) //nolint:gosec // G706: logged value is operator configuration (env/paths/port) or a server-generated ID, not attacker-controlled free text
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

		adminUser, createErr := querier.CreateUser(ctx, db.CreateUserParams{
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

		if err := querier.SetUserMustChangePassword(ctx, db.SetUserMustChangePasswordParams{
			ID:                 adminUser.ID,
			MustChangePassword: true,
		}); err != nil {
			log.Printf("[BOOTSTRAP_WARN] Failed setting must_change_password: %v", err)
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
		dbURL = "postgres://crm_user:crm_pass@localhost:5432/crm_db?sslmode=disable" //nolint:gosec // G101: false positive: local-development default URL / context key name, not a real credential
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
	// Demo mode resolves the same deterministic no-network stub as the
	// runtime gateway (server.Config.DemoMode), so this check passes without
	// a backend.
	// 30 s fixed cap: cold Ollama model loads (multi-second first download/load)
	// must not fail the check falsely, while an unbounded generation timeout
	// (OLLAMA_TIMEOUT_SECONDS) must never block boot. Timeout is warning-only;
	// a dimension mismatch remains fatal.
	embeddingCtx, cancelEmbeddingCheck := context.WithTimeout(ctx, 30*time.Second)
	embeddingErr := ai.NewGateway(ai.GatewayConfig{DemoMode: isDemoMode}).ValidateEmbeddingModel(embeddingCtx)
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

	// The CSRF HMAC key must outlive a restart, otherwise every cookie-bound
	// mutating request answers 403 until the user logs in again. Install it here
	// and treat a configured-but-unusable secret as a fatal misconfiguration:
	// falling back to a random key would look like success and lose the sessions.
	if csrfErr := installCSRFSecret(privKey); csrfErr != nil {
		log.Fatalf("failed initializing CSRF secret: %v", csrfErr)
	}

	// Initialize Storage & SSE Hub
	localStorage := storage.NewLocalStorage(storagePath)
	sseHub := sse.NewHub()

	var querier db.Querier
	// The pool stays nil in demo mode, which is how the restore endpoints
	// report 501 instead of pretending a drill succeeded.
	var dbPool *pgxpool.Pool
	if isDemoMode {
		querier = demo.NewInMemoryQuerier()
	} else {
		dbPool, err = db.ConnectWithRetry(ctx, dbURL, 15, 1*time.Second)
		if err != nil {
			log.Fatalf("[FATAL] PostgreSQL database connection failed: %v", err)
		}

		if migErr := db.RunMigrations(ctx, dbPool); migErr != nil {
			log.Fatalf("[FATAL] Database migrations failed: %v", migErr)
		}

		querier = db.New(dbPool)
		bootstrapAdminUser(ctx, querier, storagePath)
		defer dbPool.Close()
	}

	r := server.NewRouter(server.Config{
		DB:       querier,
		Pool:     dbPool,
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
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("openlocalcrm crm-server listening on :%s", port) //nolint:gosec // G706: logged value is operator configuration (env/paths/port) or a server-generated ID, not attacker-controlled free text
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

// installCSRFSecret resolves the CSRF HMAC key for this process.
//
// CSRF_SECRET_KEY_PATH (optional) wins when set, mirroring JWT_SECRET_KEY_PATH, so
// an operator can rotate or pin the secret independently of the signing key. Without
// it the secret is derived from the signing key, which is already persisted and
// already rotated by the same file - no second secret to forget about.
//
// Every failure is returned, never swallowed: an unreadable file, an empty file and
// a too-short file are all fatal at the call site. Reading is deliberately not
// allowed to fall through to derivation, because that would silently invalidate
// tokens issued under the configured secret after a restart.
func installCSRFSecret(privKey ed25519.PrivateKey) error {
	path := os.Getenv("CSRF_SECRET_KEY_PATH")
	if path == "" {
		return auth.InstallCSRFSecretFromSigningKey(privKey)
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from operator-controlled configuration, not request input
	if err != nil {
		return fmt.Errorf("CSRF_SECRET_KEY_PATH is set but unreadable (%s): %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return fmt.Errorf("CSRF_SECRET_KEY_PATH is set but empty after trimming whitespace (%s)", path)
	}
	if err := auth.SetCSRFSecret(data); err != nil {
		return fmt.Errorf("CSRF_SECRET_KEY_PATH (%s): %w", path, err)
	}
	log.Printf("[Crypto] Installed persistent CSRF secret from %s", path) //nolint:gosec // G706: logged value is operator configuration, not attacker-controlled text
	return nil
}
