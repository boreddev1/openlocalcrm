package auth_test

import (
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/stretchr/testify/require"
)

// TestRoleMatrixMatchesTheApprovedDesign is the machine-checked copy of
// docs/superpowers/specs/2026-09-21-fake-feature-elimination-design.md §12.1.
func TestRoleMatrixMatchesTheApprovedDesign(t *testing.T) {
	matrix := []struct {
		perm                                  auth.Permission
		admin, vertrieb, backoffice, benutzer bool
	}{
		{auth.PermUsersAdmin, true, false, false, false},
		{auth.PermBackup, true, false, false, false},
		{auth.PermSettingsExport, true, false, false, false},
		{auth.PermEmailTemplates, true, false, false, false},
		{auth.PermContactsRead, true, true, true, true},
		{auth.PermContactsWrite, true, true, true, false},
		{auth.PermContactsDelete, true, false, false, false},
		{auth.PermDealsRead, true, true, true, true},
		{auth.PermDealsWrite, true, true, true, false},
		{auth.PermDealsDelete, true, true, false, false},
		{auth.PermDealsRevoke, true, true, true, false},
		{auth.PermContactsImport, true, true, true, false},
		{auth.PermNotesWrite, true, true, true, false},
		{auth.PermEmailsTeam, true, true, true, false},
		{auth.PermEmailsSend, true, true, true, false},
		{auth.PermTelephony, true, true, true, false},
		{auth.PermAI, true, true, true, false},
		{auth.PermAIObservability, true, false, false, false},
		{auth.PermReportsView, true, true, true, false},
		{auth.PermReportsExport, true, false, true, false},
		{auth.PermAutomationsRead, true, true, true, false},
		{auth.PermAutomationsWrite, true, false, false, false},
		{auth.PermAutomationsRun, true, true, false, false},
	}
	for _, row := range matrix {
		require.Equal(t, row.admin, auth.HasPermission("ADMIN", row.perm), "ADMIN/%s", row.perm)
		require.Equal(t, row.vertrieb, auth.HasPermission("VERTRIEB", row.perm), "VERTRIEB/%s", row.perm)
		require.Equal(t, row.backoffice, auth.HasPermission("BACKOFFICE", row.perm), "BACKOFFICE/%s", row.perm)
		require.Equal(t, row.benutzer, auth.HasPermission("BENUTZER", row.perm), "BENUTZER/%s", row.perm)
	}
}
