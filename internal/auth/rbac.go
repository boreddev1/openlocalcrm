package auth

import (
	"net/http"
)

type Permission string

const (
	PermUsersAdmin       Permission = "users.admin"
	PermBackup           Permission = "backup.admin"
	PermSettingsExport   Permission = "settings.export"
	PermEmailTemplates   Permission = "email_templates.admin"
	PermContactsRead     Permission = "contacts.read"
	PermContactsWrite    Permission = "contacts.write"
	PermContactsDelete   Permission = "contacts.delete"
	PermDealsRead        Permission = "deals.read"
	PermDealsWrite       Permission = "deals.write"
	PermDealsDelete      Permission = "deals.delete"
	PermDealsRevoke      Permission = "deals.revoke"
	PermContactsImport   Permission = "contacts.import"
	PermNotesWrite       Permission = "notes.write"
	PermEmailsTeam       Permission = "emails.team"
	PermEmailsSend       Permission = "emails.send"
	PermTelephony        Permission = "telephony.use"
	PermAI               Permission = "ai.use"
	PermAIObservability  Permission = "ai.observability"
	PermReportsView      Permission = "reports.view"
	PermReportsExport    Permission = "reports.export"
	PermAutomationsRead  Permission = "automations.read"
	PermAutomationsWrite Permission = "automations.write"
	PermAutomationsRun   Permission = "automations.run"
	PermOwnOnly          Permission = "own.only"
)

var rolePermissions = map[string]map[Permission]bool{
	"ADMIN": {
		PermUsersAdmin:       true,
		PermBackup:           true,
		PermSettingsExport:   true,
		PermEmailTemplates:   true,
		PermContactsRead:     true,
		PermContactsWrite:    true,
		PermContactsDelete:   true,
		PermDealsRead:        true,
		PermDealsWrite:       true,
		PermDealsDelete:      true,
		PermDealsRevoke:      true,
		PermContactsImport:   true,
		PermNotesWrite:       true,
		PermEmailsTeam:       true,
		PermEmailsSend:       true,
		PermTelephony:        true,
		PermAI:               true,
		PermAIObservability:  true,
		PermReportsView:      true,
		PermReportsExport:    true,
		PermAutomationsRead:  true,
		PermAutomationsWrite: true,
		PermAutomationsRun:   true,
	},
	"VERTRIEB": {
		PermContactsRead:    true,
		PermContactsWrite:   true,
		PermDealsRead:       true,
		PermDealsWrite:      true,
		PermDealsDelete:     true,
		PermDealsRevoke:     true,
		PermContactsImport:  true,
		PermNotesWrite:      true,
		PermEmailsTeam:      true,
		PermEmailsSend:      true,
		PermTelephony:       true,
		PermAI:              true,
		PermReportsView:     true,
		PermAutomationsRead: true,
		PermAutomationsRun:  true,
	},
	"BACKOFFICE": {
		PermContactsRead:    true,
		PermContactsWrite:   true,
		PermDealsRead:       true,
		PermDealsWrite:      true,
		PermDealsRevoke:     true,
		PermContactsImport:  true,
		PermNotesWrite:      true,
		PermEmailsTeam:      true,
		PermEmailsSend:      true,
		PermTelephony:       true,
		PermAI:              true,
		PermReportsView:     true,
		PermReportsExport:   true,
		PermAutomationsRead: true,
	},
	"BENUTZER": {
		PermContactsRead: true,
		PermDealsRead:    true,
	},
}

func HasPermission(role string, p Permission) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	return perms[p]
}

func RequirePermission(p Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(UserContextKey).(*AccessClaims)
			if !ok || claims == nil {
				http.Error(w, `{"error":"forbidden","message":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			if !HasPermission(claims.Role, p) {
				http.Error(w, `{"error":"forbidden","message":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
