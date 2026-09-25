package audit

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
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
	if strings.HasPrefix(lower, "consent_") {
		return val
	}

	value := reflect.ValueOf(val)
	switch value.Kind() {
	case reflect.Map, reflect.Struct, reflect.Slice, reflect.Array, reflect.Ptr, reflect.Interface:
		normalized, err := normalizeJSONValue(val)
		if err != nil {
			return nil
		}
		return sanitizeNormalizedValue("", normalized)
	default:
		return sanitizeScalarValue(lower, val)
	}
}

func sanitizeNormalizedValue(key string, val any) any {
	if val == nil {
		return nil
	}
	lower := strings.ToLower(key)
	if strings.HasPrefix(lower, "consent_") {
		return val
	}
	switch typed := val.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for nestedKey, nestedValue := range typed {
			result[nestedKey] = sanitizeNormalizedValue(nestedKey, nestedValue)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, nestedValue := range typed {
			result[i] = sanitizeNormalizedValue(key, nestedValue)
		}
		return result
	default:
		return sanitizeScalarValue(lower, val)
	}
}

func sanitizeScalarValue(key string, val any) any {
	lower := strings.ToLower(key)
	if strings.HasPrefix(lower, "consent_") {
		return val
	}
	if strings.Contains(lower, "phone") || strings.Contains(lower, "mobile") || strings.Contains(lower, "telefon") {
		str, ok := val.(string)
		if !ok || len(str) < 4 {
			return "***"
		}
		return str[:2] + " *** " + str[len(str)-2:]
	}
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
	if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") {
		return "[REDACTED]"
	}
	return val
}

func normalizeJSONValue(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func sanitizeMap(m DiffMap) DiffMap {
	if m == nil {
		return DiffMap{}
	}
	result := make(DiffMap, len(m))
	for key, value := range m {
		result[key] = SanitizeDiffValue(key, value)
	}
	return result
}

func changedKeys(oldValues, newValues DiffMap) []string {
	keys := make(map[string]struct{}, len(oldValues)+len(newValues))
	for key := range oldValues {
		keys[key] = struct{}{}
	}
	for key := range newValues {
		keys[key] = struct{}{}
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		oldValue, oldExists := oldValues[key]
		newValue, newExists := newValues[key]
		if !oldExists || !newExists || !reflect.DeepEqual(oldValue, newValue) {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
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

	changesBytes, _ := json.Marshal(map[string]any{
		"action":       action,
		"changed_keys": changedKeys(oldValues, newValues),
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
	changesBytes := []byte("{}")
	if changes != nil {
		normalized, err := normalizeJSONValue(changes)
		if err == nil {
			changesBytes, err = json.Marshal(sanitizeNormalizedValue("", normalized))
		}
		if err != nil {
			changesBytes = []byte("{}")
		}
	}

	_, err := s.queries.CreateAuditLog(ctx, db.CreateAuditLogParams{
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
