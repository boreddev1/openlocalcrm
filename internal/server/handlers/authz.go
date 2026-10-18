package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

// claimsFrom returns the authenticated claims or ok=false. It is the only
// supported way for a handler to read claims: a missing value must deny.
func claimsFrom(r *http.Request) (*auth.AccessClaims, bool) {
	claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if !ok || claims == nil {
		return nil, false
	}
	return claims, true
}

// canModify applies the ownership rule for team resources (contacts, companies, deals,
// notes, appointments, todos).
//   - ADMIN always has write access.
//   - VERTRIEB and BACKOFFICE have team-wide write access for team resources per their domain permissions.
//   - BENUTZER is strictly restricted to its own assigned items.
//     A NULL owner is never writable by BENUTZER.
func canModify(claims *auth.AccessClaims, owner pgtype.UUID) bool {
	if claims == nil {
		return false
	}
	if claims.Role == "ADMIN" || claims.Role == "VERTRIEB" || claims.Role == "BACKOFFICE" {
		return true
	}
	if !owner.Valid {
		return false
	}
	return uuid.UUID(owner.Bytes) == claims.UserID
}

// canReassign enforces that only ADMIN or the current owner can change owner_user_id or assigned_to.
func canReassign(claims *auth.AccessClaims, currentOwner pgtype.UUID) bool {
	if claims == nil {
		return false
	}
	if claims.Role == "ADMIN" {
		return true
	}
	if !currentOwner.Valid {
		return claims.Role == "VERTRIEB" || claims.Role == "BACKOFFICE"
	}
	return uuid.UUID(currentOwner.Bytes) == claims.UserID
}

// canModifyPersonal strictly enforces that only ADMIN or the record owner can modify personal resources.
func canModifyPersonal(claims *auth.AccessClaims, owner pgtype.UUID) bool {
	if claims == nil {
		return false
	}
	if claims.Role == "ADMIN" {
		return true
	}
	if !owner.Valid {
		return false
	}
	return uuid.UUID(owner.Bytes) == claims.UserID
}

// canRead enforces read scoping: ADMIN, VERTRIEB, BACKOFFICE may read any team resource;
// BENUTZER may only read records assigned to it.
func canRead(claims *auth.AccessClaims, owner pgtype.UUID) bool {
	if claims == nil {
		return false
	}
	if claims.Role == "ADMIN" || claims.Role == "VERTRIEB" || claims.Role == "BACKOFFICE" {
		return true
	}
	if !owner.Valid {
		return false
	}
	return uuid.UUID(owner.Bytes) == claims.UserID
}
