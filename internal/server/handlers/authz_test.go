package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/stretchr/testify/require"
)

// TestCanModifyEnforcesRoleMatrixAndOwnership asserts that ADMIN, VERTRIEB and BACKOFFICE
// may modify team resources, while BENUTZER is restricted to its own assigned items.
func TestCanModifyEnforcesRoleMatrixAndOwnership(t *testing.T) {
	testUserID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	otherUserID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	sales := &auth.AccessClaims{UserID: testUserID, Role: "VERTRIEB"}
	backoffice := &auth.AccessClaims{UserID: testUserID, Role: "BACKOFFICE"}
	user := &auth.AccessClaims{UserID: testUserID, Role: "BENUTZER"}
	admin := &auth.AccessClaims{UserID: otherUserID, Role: "ADMIN"}

	nilOwner := pgtype.UUID{}
	require.True(t, canModify(admin, nilOwner))
	require.True(t, canModify(sales, nilOwner))
	require.True(t, canModify(backoffice, nilOwner))
	require.False(t, canModify(user, nilOwner), "a NULL owner must not grant write access to BENUTZER")

	ownedByAdmin := pgtype.UUID{Bytes: admin.UserID, Valid: true}
	require.True(t, canModify(admin, ownedByAdmin))
	require.True(t, canModify(sales, ownedByAdmin))
	require.True(t, canModify(backoffice, ownedByAdmin))
	require.False(t, canModify(user, ownedByAdmin))

	ownedByUser := pgtype.UUID{Bytes: user.UserID, Valid: true}
	require.True(t, canModify(user, ownedByUser))
}

// TestCanReassignRequiresOwnerOrAdmin: a team member cannot transfer away another user's owner assignment.
func TestCanReassignRequiresOwnerOrAdmin(t *testing.T) {
	testUserID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	otherUserID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	owner := &auth.AccessClaims{UserID: testUserID, Role: "VERTRIEB"}
	colleague := &auth.AccessClaims{UserID: otherUserID, Role: "VERTRIEB"}
	admin := &auth.AccessClaims{UserID: uuid.New(), Role: "ADMIN"}

	ownerUUID := pgtype.UUID{Bytes: owner.UserID, Valid: true}
	require.True(t, canReassign(owner, ownerUUID))
	require.True(t, canReassign(admin, ownerUUID))
	require.False(t, canReassign(colleague, ownerUUID))
}

// TestClaimsFromIsFailClosed: a claim-less context must not authorize anything.
func TestClaimsFromIsFailClosed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	claims, ok := claimsFrom(req)
	require.False(t, ok)
	require.Nil(t, claims)
}
