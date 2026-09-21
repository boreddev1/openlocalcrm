package server

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/connectors"
	"github.com/openlocalcrm/openlocalcrm/internal/core/appointment"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/core/company"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/core/email"
	"github.com/openlocalcrm/openlocalcrm/internal/core/export"
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
	DB         db.Querier
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

	// Defense-in-depth app layer security headers
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			next.ServeHTTP(w, r)
		})
	})

	// Public API health endpoint with demo_mode, go_version and AI config indicators
	r.Get("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":      "healthy",
			"system":      "openlocalcrm-v3",
			"version":     "3.0.0",
			"go_version":  "1.26.0",
			"demo_mode":   cfg.DemoMode,
			"ai_provider": aiProvider,
			"ai_model":    aiModel,
			"ai_base_url": aiBaseURL,
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

			if auth.IsTokenRevoked(token) {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			claims, err := auth.ValidateAccessToken(token, cfg.PubKey)
			if err != nil || claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			cfg.SSEHub.ServeHTTP(w, r)
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
	var backupH *handlers.BackupHandler
	var settingsH *handlers.SettingsHandler
	var authLimiter *auth.RateLimiter
	var aiLimiter *auth.RateLimiter

	if cfg.DB != nil {
		auditSvc := audit.NewService(cfg.DB)
		contactSvc := contact.NewService(cfg.DB, auditSvc)
		companySvc := company.NewService(cfg.DB, auditSvc)
		dealSvc := deal.NewService(cfg.DB, auditSvc)
		todoSvc := todo.NewService(cfg.DB, auditSvc)
		noteSvc := note.NewService(cfg.DB, cfg.SSEHub)
		emailSvc := email.NewService(cfg.DB, auditSvc, cfg.Storage, cfg.SSEHub)
		notificationSvc := notification.NewService(cfg.DB, cfg.SSEHub)
		exportSvc := export.NewService(cfg.DB, contactSvc)

		var dbtx db.DBTX
		if provider, ok := cfg.DB.(interface{ DB() db.DBTX }); ok {
			dbtx = provider.DB()
		}
		obsSvc := ai.NewObservabilityService(dbtx)
		aiGateway := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.Provider(aiProvider),
			OllamaBaseURL:   aiBaseURL,
			OllamaModel:     aiModel,
			APIKey:          aiAPIKey,
		})
		triageSvc := ai.NewTriageService(aiGateway, obsSvc)
		researchSvc := ai.NewResearchService(aiGateway, obsSvc)
		chatSvc := ai.NewChatService(aiGateway, obsSvc, researchSvc, cfg.DB)

		connectorToken := os.Getenv("CONNECTOR_API_TOKEN")
		if connectorToken == "" && cfg.DemoMode {
			connectorToken = "demo-connector-token"
		}
		connectorEngine := connectors.NewEngine(contactSvc, dealSvc, connectorToken)

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
		authH = handlers.NewAuthHandlerWithService(authSvc)
		contactH = handlers.NewContactHandler(contactSvc)
		companyH = handlers.NewCompanyHandler(companySvc)
		dealH = handlers.NewDealHandler(dealSvc)
		todoH = handlers.NewTodoHandler(todoSvc)
		noteH = handlers.NewNoteHandler(noteSvc, aiGateway)
		emailH = handlers.NewEmailHandler(emailSvc, cfg.DemoMode)
		aiH = handlers.NewAIHandler(triageSvc, chatSvc, researchSvc, obsSvc, aiGateway, cfg.DB)
		connectorH = handlers.NewConnectorHandler(connectorEngine)
		notificationH = handlers.NewNotificationHandler(notificationSvc)
		exportH = handlers.NewExportHandler(exportSvc)
		userH = handlers.NewUserHandler(cfg.DB)
		backupH = handlers.NewBackupHandler(cfg.DB)
		settingsH = handlers.NewSettingsHandler()
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
	if cfg.Context != nil {
		go func() {
			<-cfg.Context.Done()
			loginLimiter.Stop()
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

		// Public Webhook intake (secured via Connector Bearer Token, outside CSRF group)
		if connectorH != nil {
			api.Post("/connectors/lead-intake", connectorH.LeadIntake)
		}

		// Protected routes (Always enforced JWT authentication, then CSRF protection)
		api.Group(func(protected chi.Router) {
			protected.Use(auth.AuthMiddleware(cfg.PubKey))
			protected.Use(CSRFProtectionMiddleware)

			protected.Get("/me", func(w http.ResponseWriter, r *http.Request) {
				claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
				if !ok || claims == nil {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(claims)
			})

			// Protected user account & security endpoints (with rate limiting)
			if authH != nil {
				protected.Group(func(secRouter chi.Router) {
					secRouter.Use(auth.RateLimitMiddleware(authLimiter))
					secRouter.Post("/auth/change-password", authH.ChangePassword)
					secRouter.Post("/auth/totp/setup", authH.SetupTOTP)
					secRouter.Post("/auth/totp/verify", authH.VerifyTOTP)
					secRouter.Post("/auth/totp/disable", authH.DisableTOTP)
				})
			}

			// Protected User Management (Admin team endpoints)
			if userH != nil {
				protected.Route("/users", func(ur chi.Router) {
					ur.Use(auth.RequireRole("ADMIN"))
					ur.Get("/", userH.List)
					ur.Post("/invite", userH.Invite)
					ur.Put("/{id}/role", userH.UpdateRole)
					ur.Put("/{id}/status", userH.UpdateStatus)
				})
			}

			// Protected Backup Endpoints (Drill & Export)
			if backupH != nil {
				protected.Route("/backup", func(br chi.Router) {
					br.Use(auth.RequireRole("ADMIN"))
					br.Post("/drill", backupH.Drill)
					br.Get("/export", backupH.Export)
				})
			}

			// Protected Settings Endpoints (Export & Import without DB dump)
			if settingsH != nil {
				protected.Route("/settings", func(sr chi.Router) {
					sr.Use(auth.RequireRole("ADMIN"))
					sr.Get("/export", settingsH.ExportSettings)
					sr.Post("/import", settingsH.ImportSettings)
				})
			}

			if contactH != nil {
				protected.Route("/contacts", func(cr chi.Router) {
					cr.Use(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE", "BENUTZER"))
					cr.Post("/", contactH.Create)
					cr.Get("/", contactH.List)
					cr.Get("/{id}", contactH.Get)
					cr.Put("/{id}", contactH.Update)
					cr.Delete("/{id}", contactH.Delete)
				})
			}

			if companyH != nil {
				protected.Route("/companies", func(cr chi.Router) {
					cr.Use(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE", "BENUTZER"))
					cr.Post("/", companyH.Create)
					cr.Get("/", companyH.List)
					cr.Get("/{id}", companyH.Get)
					cr.Put("/{id}", companyH.Update)
					cr.Delete("/{id}", companyH.Delete)
				})
			}

			if dealH != nil {
				protected.Route("/deals", func(dr chi.Router) {
					dr.Use(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE", "BENUTZER"))
					dr.Post("/", dealH.Create)
					dr.Get("/", dealH.List)
					dr.Get("/{id}", dealH.Get)
					dr.Put("/{id}", dealH.Update)
					dr.Delete("/{id}", dealH.Delete)
					dr.Post("/{id}/solar-calculation", dealH.AttachSolarCalculation)
				})
			}

			if todoH != nil {
				protected.Route("/todos", func(tr chi.Router) {
					tr.Use(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE", "BENUTZER"))
					tr.Post("/", todoH.Create)
					tr.Get("/", todoH.List)
					tr.Get("/{id}", todoH.Get)
					tr.Put("/{id}", todoH.Update)
					tr.Delete("/{id}", todoH.Delete)
				})
			}

			if noteH != nil {
				protected.Route("/notes", func(nr chi.Router) {
					nr.Use(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE", "BENUTZER"))
					nr.Post("/", noteH.Create)
					nr.Get("/", noteH.List)
					nr.Get("/{id}", noteH.Get)
					nr.Put("/{id}", noteH.Update)
					nr.Delete("/{id}", noteH.Delete)
				})
			}

			if emailH != nil {
				protected.Route("/emails", func(er chi.Router) {
					er.Use(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE", "BENUTZER"))
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
					air.Use(auth.RateLimitMiddleware(aiLimiter))
					air.Post("/triage", aiH.TriageEmail)
					air.Post("/chat", aiH.Chat)
					air.Post("/research/company", aiH.ResearchCompany)
					air.Get("/observability", aiH.GetObservability)
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
			protected.Post("/calculator/solar", calcH.CalculateSolar)

			appointmentSvc := appointment.NewService(cfg.DB, cfg.SSEHub)
			telephonySvc := telephony.NewService(cfg.DB, cfg.SSEHub)
			reportsSvc := reports.NewService(cfg.DB)
			automationSvc := automation.NewService(cfg.DB)

			appointmentH := handlers.NewAppointmentHandler(appointmentSvc)
			telephonyH := handlers.NewTelephonyHandler(telephonySvc)
			reportsH := handlers.NewReportsHandler(reportsSvc)
			automationH := handlers.NewAutomationHandler(automationSvc)

			if appointmentH != nil {
				protected.Route("/appointments", func(appr chi.Router) {
					appr.Use(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE", "BENUTZER"))
					appr.Get("/", appointmentH.List)
					appr.Post("/", appointmentH.Create)
					appr.Put("/{id}", appointmentH.Update)
					appr.Delete("/{id}", appointmentH.Delete)
					appr.Post("/{id}/push-external", appointmentH.PushExternal)
					appr.Get("/{id}/ics", appointmentH.DownloadICS)
				})
			}

			if telephonyH != nil {
				protected.Route("/telephony", func(telr chi.Router) {
					telr.Post("/calls", telephonyH.LogCall)
				})
			}

			if reportsH != nil {
				protected.Route("/reports", func(repr chi.Router) {
					repr.Use(auth.RequireAnyRole("ADMIN", "BACKOFFICE"))
					repr.Get("/sales", reportsH.GetSalesReport)
				})
			}

			if automationH != nil {
				protected.Route("/automations", func(autr chi.Router) {
					autr.Get("/", automationH.ListWorkflows)
					autr.Get("/runs", automationH.ListRuns)
					autr.With(auth.RequireRole("ADMIN")).Post("/", automationH.CreateWorkflow)
					autr.With(auth.RequireRole("ADMIN")).Post("/runs/{id}/approve", automationH.ApproveStep)
				})
			}

			if notificationH != nil {
				protected.Route("/notifications", func(nr chi.Router) {
					nr.Get("/unread", notificationH.ListUnread)
					nr.Post("/{id}/read", notificationH.MarkRead)
					nr.Post("/read-all", notificationH.MarkAllRead)
				})
			}

			if exportH != nil {
				protected.Group(func(exportRouter chi.Router) {
					exportRouter.With(auth.RequireRole("ADMIN")).Get("/export/contacts.csv", exportH.ExportContactsCSV)
					exportRouter.With(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE")).Post("/import/contacts", exportH.ImportContactsCSV)
				})
			}
		})
	})

	// Mount embedded frontend SPA for all other routes
	r.Mount("/", web.DistHandler())

	return r
}
