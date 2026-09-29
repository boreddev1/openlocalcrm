package server

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/connectors"
	"github.com/openlocalcrm/openlocalcrm/internal/core/appointment"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/core/backup"
	"github.com/openlocalcrm/openlocalcrm/internal/core/company"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/core/email"
	"github.com/openlocalcrm/openlocalcrm/internal/core/emailtemplate"
	"github.com/openlocalcrm/openlocalcrm/internal/core/export"
	"github.com/openlocalcrm/openlocalcrm/internal/core/invite"
	"github.com/openlocalcrm/openlocalcrm/internal/core/note"
	"github.com/openlocalcrm/openlocalcrm/internal/core/notification"
	"github.com/openlocalcrm/openlocalcrm/internal/core/reports"
	"github.com/openlocalcrm/openlocalcrm/internal/core/telephony"
	"github.com/openlocalcrm/openlocalcrm/internal/core/todo"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
	"github.com/openlocalcrm/openlocalcrm/internal/storage"
	"github.com/openlocalcrm/openlocalcrm/web"
)

type Config struct {
	DB db.Querier
	// Pool is the real PostgreSQL pool. It stays nil in demo mode, which is how
	// the restore and the restore drill honestly report 501 instead of
	// simulating a success.
	Pool       *pgxpool.Pool
	SSEHub     *sse.Hub
	Storage    storage.StorageService
	PubKey     ed25519.PublicKey
	PrivKey    ed25519.PrivateKey
	DemoMode   bool
	AIProvider string
	AIModel    string
	AIBaseURL  string
	Context    context.Context
}

func NewRouter(cfg Config) http.Handler {
	if cfg.PubKey == nil {
		panic("server: cryptographic public key (PubKey) is required")
	}

	aiProvider := cfg.AIProvider
	if aiProvider == "" {
		aiProvider = os.Getenv("AI_PROVIDER")
	}
	if aiProvider == "" {
		aiProvider = "ollama"
	}

	aiBaseURL := cfg.AIBaseURL
	if aiBaseURL == "" {
		aiBaseURL = os.Getenv("OLLAMA_BASE_URL")
	}
	if aiBaseURL == "" {
		aiBaseURL = os.Getenv("AI_BASE_URL")
	}
	if aiBaseURL == "" {
		aiBaseURL = "http://localhost:11434"
	}

	aiModel := cfg.AIModel
	if aiModel == "" {
		aiModel = os.Getenv("OLLAMA_MODEL")
	}
	if aiModel == "" {
		aiModel = os.Getenv("AI_MODEL")
	}
	if aiModel == "" {
		aiModel = "mistral"
	}
	aiAPIKey := os.Getenv("AI_API_KEY")

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	// F-20: chi/middleware.RealIP removed to prevent header spoofing; ClientIP handles trusted proxies
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Defense-in-depth app layer security headers (F-37)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	})

	// Global request body size limit: 5MB default, 50MB for uploads/CSV, 100MB for backup restore
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
				var limit int64 = 5 * 1024 * 1024
				if strings.HasPrefix(r.URL.Path, "/api/v1/backup/restore") {
					limit = 100 * 1024 * 1024
				} else if strings.HasPrefix(r.URL.Path, "/api/v1/storage") || strings.HasPrefix(r.URL.Path, "/api/v1/export/csv") {
					limit = 50 * 1024 * 1024
				}
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	})

	// Public API health endpoint with status and demo_mode (F-40: no internal AI config leaks)
	r.Get("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "healthy",
			"system":    "openlocalcrm-v3",
			"version":   "3.0.0",
			"demo_mode": cfg.DemoMode,
		})
	})

	// Server-Sent Events stream: Protected with token authentication
	// Server-Sent Events stream: Protected with token authentication (Cookie or Authorization: Bearer only)
	if cfg.SSEHub != nil {
		r.Get("/events/stream", func(w http.ResponseWriter, r *http.Request) {
			token := ""
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			} else if cookie, err := r.Cookie("access_token"); err == nil && cookie.Value != "" {
				token = cookie.Value
			}

			if token == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			claims, err := auth.ValidateAccessToken(token, cfg.PubKey)
			if err != nil || claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			if cfg.DB != nil {
				stored, verr := cfg.DB.CurrentTokenVersion(r.Context(), pgtype.UUID{Bytes: claims.UserID, Valid: true})
				if verr != nil || claims.TokenVersion == nil || *claims.TokenVersion != stored {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}
			}

			cfg.SSEHub.ServeHTTPFor(w, r, claims.UserID)
		})
	}

	// Initialize Services & Handlers
	var authH *handlers.AuthHandler
	var contactH *handlers.ContactHandler
	var companyH *handlers.CompanyHandler
	var dealH *handlers.DealHandler
	var todoH *handlers.TodoHandler
	var emailH *handlers.EmailHandler
	var aiH *handlers.AIHandler
	var connectorH *handlers.ConnectorHandler
	var notificationH *handlers.NotificationHandler
	var exportH *handlers.ExportHandler
	var noteH *handlers.NoteHandler
	var userH *handlers.UserHandler
	var inviteH *handlers.InviteHandler
	var emailTemplateH *handlers.EmailTemplateHandler
	var backupH *handlers.BackupHandler
	var settingsH *handlers.SettingsHandler
	var automationH *handlers.AutomationHandler
	var sessionH *handlers.SessionHandler
	var ownershipAdminH *handlers.OwnershipAdminHandler
	var authLimiter *auth.RateLimiter
	var aiLimiter *auth.RateLimiter

	if cfg.DB != nil {
		auditSvc := audit.NewService(cfg.DB)
		contactSvc := contact.NewService(cfg.DB, auditSvc)
		companySvc := company.NewService(cfg.DB, auditSvc)
		dealSvc := deal.NewService(cfg.DB, auditSvc)
		todoSvc := todo.NewService(cfg.DB, auditSvc)
		noteSvc := note.NewService(cfg.DB, cfg.SSEHub)
		notificationSvc := notification.NewService(cfg.DB, cfg.SSEHub)
		automationSvc := automation.NewService(cfg.DB)
		automationSvc.SetNotifier(notificationSvc)
		emailSvc := email.NewService(cfg.DB, auditSvc, cfg.Storage, cfg.SSEHub, automationSvc)
		exportSvc := export.NewService(cfg.DB, contactSvc)

		var dbtx db.DBTX
		if provider, ok := cfg.DB.(interface{ DB() db.DBTX }); ok {
			dbtx = provider.DB()
		}
		obsSvc := ai.NewObservabilityService(dbtx)
		aiGateway := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.Provider(aiProvider),
			OllamaBaseURL:   aiBaseURL,
			AIBaseURL:       os.Getenv("AI_BASE_URL"),
			OllamaModel:     aiModel,
			EmbeddingModel:  os.Getenv("AI_EMBEDDING_MODEL"),
			APIKey:          aiAPIKey,
		})
		triageSvc := ai.NewTriageService(aiGateway, obsSvc)
		researchSvc := ai.NewResearchService(aiGateway, obsSvc)
		chatSvc := ai.NewChatService(aiGateway, obsSvc, researchSvc, cfg.DB)
		aiDrafter := automation.NewAIDrafter(cfg.DB, aiGateway)
		automationSvc.SetEmailDrafter(aiDrafter)

		connectorToken := os.Getenv("CONNECTOR_API_TOKEN")
		if connectorToken == "" && cfg.DemoMode {
			connectorToken = "demo-connector-token"
		}
		connectorEngine := connectors.NewEngine(contactSvc, dealSvc, connectorToken, automationSvc)

		inviteSvc := invite.NewService(cfg.DB, auditSvc)
		emailTemplateSvc := emailtemplate.NewService(cfg.DB, aiGateway)

		authLimiter = auth.NewRateLimiter(20, time.Minute, 5*time.Minute)
		aiLimiter = auth.NewRateLimiter(30, time.Minute, 2*time.Minute)

		if cfg.Context != nil {
			go func() {
				<-cfg.Context.Done()
				authLimiter.Stop()
				aiLimiter.Stop()
			}()
		}

		authSvc := auth.NewAuthService(cfg.DB, cfg.PrivKey, cfg.PubKey, authLimiter)
		if cfg.SSEHub != nil {
			authSvc = authSvc.WithSessionRevoker(cfg.SSEHub)
		}
		if localStorage, ok := cfg.Storage.(*storage.LocalStorage); ok && localStorage != nil {
			authSvc = authSvc.WithBootstrapSecretPath(filepath.Join(localStorage.BaseDir(), "bootstrap-admin.txt"))
		}
		authSvc = authSvc.WithAuditLogger(auditSvc)
		authH = handlers.NewAuthHandlerWithService(authSvc)
		contactH = handlers.NewContactHandler(contactSvc)
		companyH = handlers.NewCompanyHandler(companySvc)
		dealH = handlers.NewDealHandler(dealSvc, automationSvc)
		todoH = handlers.NewTodoHandler(todoSvc)
		noteH = handlers.NewNoteHandler(noteSvc, aiGateway)
		emailH = handlers.NewEmailHandler(emailSvc, cfg.DemoMode)
		aiH = handlers.NewAIHandler(triageSvc, chatSvc, researchSvc, obsSvc, aiGateway, cfg.DB)
		connectorH = handlers.NewConnectorHandler(connectorEngine, auditSvc)
		notificationH = handlers.NewNotificationHandler(notificationSvc)
		exportH = handlers.NewExportHandler(exportSvc)
		inviteH = handlers.NewInviteHandler(inviteSvc)
		userH = handlers.NewUserHandler(cfg.DB, inviteSvc)
		emailTemplateH = handlers.NewEmailTemplateHandler(emailTemplateSvc)
		// Both *db.Queries and the demo querier implement the streaming row
		// source, so the router never has to know which one it holds. A
		// Querier that does not gets a service without a row source, and
		// backup.CanExport then reports false so the handler answers 501
		// before it commits a 200 it could never fill.
		rowSource, hasRowSource := cfg.DB.(backup.RowSource)
		if !hasRowSource {
			rowSource = nil
		}
		backupH = handlers.NewBackupHandler(backup.NewService(rowSource, auditSvc, cfg.Pool, cfg.DB))
		settingsH = handlers.NewSettingsHandler()
		automationH = handlers.NewAutomationHandler(automationSvc)
		sessionH = handlers.NewSessionHandler(authSvc, cfg.SSEHub)
		ownershipAdminH = handlers.NewOwnershipAdminHandler(cfg.DB, auditSvc)
	}

	if authLimiter == nil {
		authLimiter = auth.NewRateLimiter(20, time.Minute, 5*time.Minute)
		if cfg.Context != nil {
			go func() {
				<-cfg.Context.Done()
				authLimiter.Stop()
			}()
		}
	}
	if aiLimiter == nil {
		aiLimiter = auth.NewRateLimiter(30, time.Minute, 2*time.Minute)
		if cfg.Context != nil {
			go func() {
				<-cfg.Context.Done()
				aiLimiter.Stop()
			}()
		}
	}

	loginLimiter := auth.NewRateLimiter(5, time.Minute, 5*time.Minute)
	connectorLimiter := auth.NewRateLimiter(60, time.Minute, 5*time.Minute)
	heavyLimiter := auth.NewRateLimiter(10, time.Minute, 5*time.Minute)
	if cfg.Context != nil {
		go func() {
			<-cfg.Context.Done()
			loginLimiter.Stop()
			connectorLimiter.Stop()
			heavyLimiter.Stop()
		}()
	}

	// API v1 group
	r.Route("/api/v1", func(api chi.Router) {
		// Public Auth routes
		if authH != nil {
			api.With(auth.RateLimitMiddleware(loginLimiter)).Post("/auth/login", authH.Login)
			api.With(CSRFProtectionMiddleware, auth.RateLimitMiddleware(authLimiter)).Post("/auth/refresh", authH.Refresh)
			api.With(CSRFProtectionMiddleware).Post("/auth/logout", authH.Logout)
		}

		// Public invite acceptance, registered next to login and before the
		// CSRF-protected group. The invitee has no session and therefore no CSRF
		// cookie, so CSRF protection here would reject the only request they can
		// make. The login rate limiter still applies, because both endpoints take
		// a guessable secret and are unauthenticated.
		if inviteH != nil {
			api.With(auth.RateLimitMiddleware(loginLimiter)).Get("/auth/accept-invite", inviteH.Validate)
			api.With(auth.RateLimitMiddleware(loginLimiter)).Post("/auth/accept-invite", inviteH.Accept)
		}

		// Public Webhook intake (secured via Connector Bearer Token, outside CSRF group)
		if connectorH != nil {
			api.With(auth.RateLimitMiddleware(connectorLimiter)).Post("/connectors/lead-intake", connectorH.LeadIntake)
		}

		// Protected routes (Always enforced JWT authentication, then CSRF protection)
		api.Group(func(protected chi.Router) {
			protected.Use(auth.AuthMiddleware(cfg.PubKey, cfg.DB))
			protected.Use(CSRFProtectionMiddleware)

			// /me returns the JWT claims plus the real 2FA state read from the
			// users table, so the settings UI never has to guess whether TOTP is
			// active. totp_enabled is null only when the user row cannot be read,
			// because "unknown" must not be reported as "disabled".
			protected.Get("/me", func(w http.ResponseWriter, r *http.Request) {
				claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
				if !ok || claims == nil {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}
				body := map[string]any{
					"user_id":              claims.UserID,
					"email":                claims.Email,
					"role":                 claims.Role,
					"sub":                  claims.Subject,
					"iss":                  claims.Issuer,
					"aud":                  claims.Audience,
					"iat":                  claims.IssuedAt,
					"exp":                  claims.ExpiresAt,
					"totp_enabled":         nil,
					"must_change_password": false,
				}
				var userID pgtype.UUID
				if cfg.DB != nil {
					if err := userID.Scan(claims.UserID.String()); err == nil {
						if user, dbErr := cfg.DB.GetUserByID(r.Context(), userID); dbErr == nil {
							enabled := user.TotpEnabled
							body["totp_enabled"] = enabled
							body["must_change_password"] = user.MustChangePassword
							body["first_name"] = user.FirstName
							body["last_name"] = user.LastName
						}
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(body)
			})

			// Protected user account & security endpoints (with rate limiting)
			if authH != nil {
				protected.Group(func(secRouter chi.Router) {
					secRouter.Use(auth.RateLimitMiddleware(authLimiter))
					secRouter.Post("/auth/change-password", authH.ChangePassword)
					secRouter.Post("/auth/totp/setup", authH.SetupTOTP)
					secRouter.Post("/auth/totp/verify", authH.VerifyTOTP)
					secRouter.Post("/auth/totp/disable", authH.DisableTOTP)
					if sessionH != nil {
						secRouter.Post("/auth/logout-all", sessionH.LogoutAll)
					}
				})
			}

			// Protected User Management (Admin team endpoints)
			if userH != nil {
				protected.Route("/users", func(ur chi.Router) {
					ur.Use(auth.RequirePermission(auth.PermUsersAdmin))
					ur.Get("/", userH.List)
					ur.Post("/invite", userH.Invite)
					ur.Put("/{id}/role", userH.UpdateRole)
					ur.Put("/{id}/status", userH.UpdateStatus)
				})
			}

			// Protected Backup Endpoints (Drill & Export)
			if backupH != nil {
				protected.Route("/backup", func(br chi.Router) {
					br.Use(auth.RequirePermission(auth.PermBackup))
					br.Use(auth.RateLimitMiddleware(heavyLimiter))
					br.Post("/drill", backupH.Drill)
					br.Get("/export", backupH.Export)
					br.Post("/restore", backupH.Restore)
				})
			}

			// Protected Settings Endpoints (Export & Import without DB dump)
			if settingsH != nil {
				protected.Route("/settings", func(sr chi.Router) {
					sr.Use(auth.RequirePermission(auth.PermSettingsExport))
					sr.Get("/export", settingsH.ExportSettings)
					sr.Post("/import", settingsH.ImportSettings)
				})
			}

			// Protected Ownership Bulk-Assignment (Admin only)
			if ownershipAdminH != nil {
				protected.Route("/ownership", func(or chi.Router) {
					or.Use(auth.RequirePermission(auth.PermUsersAdmin))
					or.Post("/bulk-assign", ownershipAdminH.BulkAssign)
				})
			}

			// Protected E-Mail template CRUD (admin only). The path segment is the
			// template key, not its UUID: every CRUD query is keyed by `key`, so a
			// second lookup query would exist only to rename a parameter.
			if emailTemplateH != nil {
				protected.Route("/email-templates", func(etr chi.Router) {
					etr.Use(auth.RequirePermission(auth.PermEmailTemplates))
					etr.Get("/", emailTemplateH.List)
					etr.Post("/", emailTemplateH.Create)
					etr.Post("/generate", emailTemplateH.Generate)
					etr.Put("/{key}", emailTemplateH.Update)
					etr.Delete("/{key}", emailTemplateH.Delete)
				})
			}

			if contactH != nil {
				protected.Route("/contacts", func(cr chi.Router) {
					cr.Use(auth.RequirePermission(auth.PermContactsRead))
					cr.Post("/", contactH.Create)
					cr.Get("/", contactH.List)
					cr.Get("/{id}", contactH.Get)
					cr.Put("/{id}", contactH.Update)
					cr.Delete("/{id}", contactH.Delete)
				})
			}

			if companyH != nil {
				protected.Route("/companies", func(cr chi.Router) {
					cr.Use(auth.RequirePermission(auth.PermContactsRead))
					cr.Post("/", companyH.Create)
					cr.Get("/", companyH.List)
					cr.Get("/{id}", companyH.Get)
					cr.Put("/{id}", companyH.Update)
					cr.Delete("/{id}", companyH.Delete)
				})
			}

			if dealH != nil {
				protected.Route("/deals", func(dr chi.Router) {
					dr.Use(auth.RequirePermission(auth.PermDealsRead))
					dr.Post("/", dealH.Create)
					dr.Get("/", dealH.List)
					dr.Get("/{id}", dealH.Get)
					dr.Put("/{id}", dealH.Update)
					dr.With(auth.RequirePermission(auth.PermDealsDelete)).Delete("/{id}", dealH.Delete)
					dr.With(auth.RequirePermission(auth.PermDealsRevoke)).Post("/{id}/revoke", dealH.Revoke)
					dr.Post("/{id}/solar-calculation", dealH.AttachSolarCalculation)
				})
			}

			if todoH != nil {
				protected.Route("/todos", func(tr chi.Router) {
					tr.Use(auth.RequirePermission(auth.PermContactsRead))
					tr.Post("/", todoH.Create)
					tr.Get("/", todoH.List)
					tr.Get("/{id}", todoH.Get)
					tr.Put("/{id}", todoH.Update)
					tr.Delete("/{id}", todoH.Delete)
				})
			}

			if noteH != nil {
				protected.Route("/notes", func(nr chi.Router) {
					nr.Use(auth.RequirePermission(auth.PermContactsRead))
					nr.Post("/", noteH.Create)
					nr.Get("/", noteH.List)
					nr.Get("/{id}", noteH.Get)
					nr.Put("/{id}", noteH.Update)
					nr.Delete("/{id}", noteH.Delete)
				})
			}

			if emailH != nil {
				protected.Route("/emails", func(er chi.Router) {
					er.Use(auth.RequirePermission(auth.PermContactsRead))
					er.Get("/messages", emailH.ListMessages)
					er.Get("/threads/{threadID}", emailH.GetThread)
					er.Post("/demo-ingest", emailH.IngestDemo)
					er.Post("/send", emailH.SendEmail)
					er.Patch("/{id}/tags", emailH.TagMessage)
					er.Get("/accounts", emailH.ListAccounts)
					er.Post("/accounts", emailH.CreateAccount)
					er.Patch("/accounts/{id}", emailH.UpdateAccount)
					er.Delete("/accounts/{id}", emailH.DeleteAccount)
					er.Post("/accounts/{id}/test", emailH.TestAccountConnection)
					er.Post("/accounts/{id}/sync", emailH.SyncAccount)
				})
			}

			if aiH != nil {
				protected.Route("/ai", func(air chi.Router) {
					air.Use(auth.RequirePermission(auth.PermAI))
					air.Use(auth.RateLimitMiddleware(aiLimiter))
					air.Post("/triage", aiH.TriageEmail)
					air.Post("/chat", aiH.Chat)
					air.Post("/research/company", aiH.ResearchCompany)
					air.With(auth.RequirePermission(auth.PermAIObservability)).Get("/observability", aiH.GetObservability)
					air.Post("/parse-bill", aiH.ParseBill)
					air.Get("/kb/search", aiH.SearchKB)
					air.Get("/kb", aiH.ListKB)
					air.Post("/kb", aiH.CreateKB)
					air.Delete("/kb/{id}", aiH.DeleteKB)
					air.Get("/research/jobs", aiH.ListResearchJobs)
					air.Post("/research/jobs", aiH.CreateResearchJob)
					if noteH != nil {
						air.Post("/synthesize-notes", noteH.Synthesize)
					}
				})
			}

			calcH := handlers.NewCalculatorHandler()
			protected.With(auth.RequirePermission(auth.PermDealsWrite)).Post("/calculator/solar", calcH.CalculateSolar)

			appointmentSvc := appointment.NewService(cfg.DB, cfg.SSEHub)
			telephonySvc := telephony.NewService(cfg.DB, cfg.SSEHub)
			reportsSvc := reports.NewService(cfg.DB)

			appointmentH := handlers.NewAppointmentHandler(appointmentSvc)
			telephonyH := handlers.NewTelephonyHandler(telephonySvc)
			reportsH := handlers.NewReportsHandler(reportsSvc)
			if automationH == nil {
				automationSvc := automation.NewService(cfg.DB)
				automationH = handlers.NewAutomationHandler(automationSvc)
			}

			if appointmentH != nil {
				protected.Route("/appointments", func(appr chi.Router) {
					appr.Use(auth.RequirePermission(auth.PermContactsRead))
					appr.Get("/", appointmentH.List)
					appr.Post("/", appointmentH.Create)
					appr.Put("/{id}", appointmentH.Update)
					appr.Delete("/{id}", appointmentH.Delete)
					appr.Get("/{id}/ics", appointmentH.DownloadICS)
				})
			}

			if telephonyH != nil {
				protected.Route("/telephony", func(telr chi.Router) {
					telr.Use(auth.RequirePermission(auth.PermTelephony))
					telr.Post("/calls", telephonyH.LogCall)
				})
			}

			if reportsH != nil {
				protected.Route("/reports", func(repr chi.Router) {
					repr.Use(auth.RequirePermission(auth.PermReportsView))
					repr.Get("/sales", reportsH.GetSalesReport)
				})
			}

			if automationH != nil {
				protected.Route("/automations", func(autr chi.Router) {
					autr.Use(auth.RequirePermission(auth.PermAutomationsRead))
					autr.Get("/", automationH.ListWorkflows)
					autr.Get("/runs", automationH.ListRuns)
					autr.With(auth.RequirePermission(auth.PermAutomationsWrite)).Post("/", automationH.CreateWorkflow)
					autr.With(auth.RequirePermission(auth.PermAutomationsRun)).Post("/{id}/test-run", automationH.TestRunWorkflow)
					autr.With(auth.RequirePermission(auth.PermAutomationsRun)).Post("/runs/{id}/approve", automationH.ApproveStep)
				})
			}

			if notificationH != nil {
				protected.Route("/notifications", func(nr chi.Router) {
					nr.Use(auth.RequirePermission(auth.PermContactsRead))
					nr.Get("/unread", notificationH.ListUnread)
					nr.Post("/{id}/read", notificationH.MarkRead)
					nr.Post("/read-all", notificationH.MarkAllRead)
				})
			}

			if exportH != nil {
				protected.Group(func(exportRouter chi.Router) {
					exportRouter.Use(auth.RateLimitMiddleware(heavyLimiter))
					exportRouter.With(auth.RequirePermission(auth.PermReportsExport)).Get("/export/contacts.csv", exportH.ExportContactsCSV)
					exportRouter.With(auth.RequirePermission(auth.PermContactsImport)).Post("/import/contacts", exportH.ImportContactsCSV)
				})
			}
		})
	})

	// Mount embedded frontend SPA for all other routes
	r.Mount("/", web.DistHandler())

	return r
}
