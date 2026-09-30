package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/google/uuid"
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

const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// ComputeAuditHash computes a SHA-256 hash over all canonical audit entry fields.
func ComputeAuditHash(prevHash string, userID pgtype.UUID, entityType string, entityID pgtype.UUID, action string, changes, oldVals, newVals []byte, ip, ua string) string {
	h := sha256.New()
	var userStr string
	if userID.Valid {
		userStr = uuid.UUID(userID.Bytes).String()
	}
	var entityStr string
	if entityID.Valid {
		entityStr = uuid.UUID(entityID.Bytes).String()
	}
	parts := []string{
		prevHash,
		userStr,
		entityType,
		entityStr,
		action,
		string(changes),
		string(oldVals),
		string(newVals),
		ip,
		ua,
	}
	h.Write([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Service) getLatestHash(ctx context.Context) string {
	if s.queries == nil {
		return GenesisHash
	}
	latest, err := s.queries.GetLatestAuditLogHash(ctx)
	if err == nil && latest.Valid && latest.String != "" {
		return latest.String
	}
	return GenesisHash
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

	prevHash := s.getLatestHash(ctx)
	entryHash := ComputeAuditHash(prevHash, userID, entityType, entityID, action, changesBytes, oldBytes, newBytes, ip, userAgent)

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
		PrevHash:   pgtype.Text{String: prevHash, Valid: true},
		Hash:       pgtype.Text{String: entryHash, Valid: true},
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

	oldBytes := []byte("{}")
	newBytes := []byte("{}")
	prevHash := s.getLatestHash(ctx)
	entryHash := ComputeAuditHash(prevHash, userID, entityType, entityID, action, changesBytes, oldBytes, newBytes, ip, userAgent)

	_, err := s.queries.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		Changes:    changesBytes,
		OldValues:  oldBytes,
		NewValues:  newBytes,
		IpAddress:  pgtype.Text{String: ip, Valid: ip != ""},
		UserAgent:  pgtype.Text{String: userAgent, Valid: userAgent != ""},
		PrevHash:   pgtype.Text{String: prevHash, Valid: true},
		Hash:       pgtype.Text{String: entryHash, Valid: true},
	})
	return err
}

// VerifyAuditChain verifies the cryptographic integrity of all audit log rows in chronological order.
// Returns (valid, verifiedCount, error). If tampering is detected, returns (false, invalidRowIndex, err).
func (s *Service) VerifyAuditChain(ctx context.Context) (bool, int, error) {
	if s.queries == nil {
		return true, 0, nil
	}
	logs, err := s.queries.ListAuditLogsAsc(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("failed fetching audit logs for verification: %w", err)
	}

	expectedPrev := GenesisHash
	for i, l := range logs {
		// If prev_hash is not set (e.g. legacy log from before migration 21), adopt its hash if valid
		if !l.PrevHash.Valid || l.PrevHash.String == "" {
			if l.Hash.Valid && l.Hash.String != "" {
				expectedPrev = l.Hash.String
			}
			continue
		}

		if l.PrevHash.String != expectedPrev {
			return false, i, fmt.Errorf("audit chain broken at row %d (ID %v): expected prev_hash %s, got %s",
				i, l.ID, expectedPrev, l.PrevHash.String)
		}

		var ip, ua string
		if l.IpAddress.Valid {
			ip = l.IpAddress.String
		}
		if l.UserAgent.Valid {
			ua = l.UserAgent.String
		}

		computed := ComputeAuditHash(
			l.PrevHash.String,
			l.UserID,
			l.EntityType,
			l.EntityID,
			l.Action,
			l.Changes,
			l.OldValues,
			l.NewValues,
			ip,
			ua,
		)

		if !l.Hash.Valid || l.Hash.String != computed {
			return false, i, fmt.Errorf("audit hash mismatch at row %d (ID %v): expected %s, got %s",
				i, l.ID, computed, l.Hash.String)
		}

		expectedPrev = l.Hash.String
	}

	return true, len(logs), nil
}
