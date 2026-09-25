package audit

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type DiffMap map[string]any

type Service struct {
	queries db.Querier
}

func NewService(queries db.Querier) *Service {
	return &Service{queries: queries}
}

// SanitizeDiffValue masks sensitive PII (email, phone, auth tokens) while preserving
// exact legal evidence fields (consent_phone, consent_email, consent_updated_at).
func SanitizeDiffValue(key string, val any) any {
	if val == nil {
		return nil
	}
	lower := strings.ToLower(key)

	// Preserve legal compliance fields in full
	if strings.HasPrefix(lower, "consent_") {
		return val
	}

	// Mask phone numbers
	if strings.Contains(lower, "phone") || strings.Contains(lower, "mobile") || strings.Contains(lower, "telefon") {
		str, ok := val.(string)
		if !ok || len(str) < 4 {
			return "***"
		}
		return str[:2] + " *** " + str[len(str)-2:]
	}

	// Mask email addresses
	if strings.Contains(lower, "email") || strings.Contains(lower, "mail") {
		str, ok := val.(string)
		if !ok || !strings.Contains(str, "@") {
			return "***"
		}
		parts := strings.SplitN(str, "@", 2)
		if len(parts[0]) <= 1 {
			return "*@" + parts[1]
		}
		return parts[0][:1] + "***@" + parts[1]
	}

	// Redact security credentials
	if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") {
		return "[REDACTED]"
	}

	return val
}

func sanitizeMap(m DiffMap) DiffMap {
	if m == nil {
		return DiffMap{}
	}
	res := make(DiffMap, len(m))
	for k, v := range m {
		res[k] = SanitizeDiffValue(k, v)
	}
	return res
}

func (s *Service) LogDiff(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, action string, oldValues, newValues DiffMap, ip, userAgent string) error {
	sanitizedOld := sanitizeMap(oldValues)
	sanitizedNew := sanitizeMap(newValues)

	oldBytes, err := json.Marshal(sanitizedOld)
	if err != nil {
		oldBytes = []byte("{}")
	}
	newBytes, err := json.Marshal(sanitizedNew)
	if err != nil {
		newBytes = []byte("{}")
	}

	// changes summary contains the delta keys
	diffKeys := make([]string, 0)
	for k := range sanitizedNew {
		diffKeys = append(diffKeys, k)
	}
	changesBytes, _ := json.Marshal(map[string]any{
		"action":       action,
		"changed_keys": diffKeys,
	})

	_, err = s.queries.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		Changes:    changesBytes,
		OldValues:  oldBytes,
		NewValues:  newBytes,
		IpAddress:  pgtype.Text{String: ip, Valid: ip != ""},
		UserAgent:  pgtype.Text{String: userAgent, Valid: userAgent != ""},
	})
	return err
}

func (s *Service) Log(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, action string, changes any, ip, userAgent string) error {
	var changesBytes []byte
	var err error
	if changes != nil {
		changesBytes, err = json.Marshal(changes)
		if err != nil {
			changesBytes = []byte("{}")
		}
	} else {
		changesBytes = []byte("{}")
	}

	_, err = s.queries.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		Changes:    changesBytes,
		OldValues:  []byte("{}"),
		NewValues:  []byte("{}"),
		IpAddress:  pgtype.Text{String: ip, Valid: ip != ""},
		UserAgent:  pgtype.Text{String: userAgent, Valid: userAgent != ""},
	})
	return err
}
