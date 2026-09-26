package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/backup"
)

// MaxRestoreBytes caps the restore upload at 512 MiB. It is a variable purely
// so the limit itself can be exercised in tests without materializing half a
// gigabyte of JSON.
var MaxRestoreBytes int64 = 512 << 20

// BackupHandler exposes the streaming export, the transactional restore and the
// real scratch-schema restore drill. It owns authentication, response headers,
// the upload limit and the streaming lifecycle only; all data work lives in
// internal/core/backup.
type BackupHandler struct {
	service *backup.Service
}

func NewBackupHandler(service *backup.Service) *BackupHandler {
	return &BackupHandler{
		service: service,
	}
}

// requestMeta carries the real request identity into the audit trail.
func requestMeta(r *http.Request, claims *auth.AccessClaims) backup.RequestMeta {
	var actorID pgtype.UUID
	if claims != nil {
		_ = actorID.Scan(claims.UserID.String())
	}
	return backup.RequestMeta{
		ActorID:   actorID,
		IPAddress: auth.ClientIP(r),
		UserAgent: r.UserAgent(),
	}
}

func adminClaims(r *http.Request) (*auth.AccessClaims, bool) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	return claims, claims != nil && claims.Role == "ADMIN"
}

// Export streams the full backup envelope straight to the client. Rows are
// written one at a time, so memory stays flat regardless of instance size.
//
// Availability is checked before the status line is committed. A deployment
// whose db.Querier cannot stream rows gets an honest 501 instead of a 200 with
// an empty body.
//
// The response status is committed before the first row, which means a failure
// mid-stream cannot be turned into a 500. Instead the handler aborts the
// response: the client sees a truncated body and rejects the invalid JSON. A
// partial export is never presented as a successful one.
func (h *BackupHandler) Export(w http.ResponseWriter, r *http.Request) {
	claims, ok := adminClaims(r)
	if !ok {
		writeJSONError(w, http.StatusForbidden, "forbidden", "Nur Administratoren können Daten-Backups exportieren")
		return
	}
	if h.service == nil || !h.service.CanExport() {
		writeJSONError(w, http.StatusNotImplemented, "backup_unavailable", backupExportUnavailableMessage)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="openlocalcrm-backup.json"`)
	w.WriteHeader(http.StatusOK)

	if err := h.service.Export(r.Context(), w, requestMeta(r, claims)); err != nil {
		panic(http.ErrAbortHandler)
	}
}

// Drill runs a real restore round trip: the live public tables are cloned into
// an isolated scratch schema and the rows are streamed through the same static
// upserts a restore uses. It accepts no request body and no uploaded backup.
//
// In demo mode there is no pool, so the drill reports 501 instead of a
// reassuring "passed".
func (h *BackupHandler) Drill(w http.ResponseWriter, r *http.Request) {
	claims, ok := adminClaims(r)
	if !ok {
		writeJSONError(w, http.StatusForbidden, "forbidden", "Nur Administratoren können Backup-Drills durchführen")
		return
	}
	if h.service == nil {
		writeJSONError(w, http.StatusNotImplemented, "backup_unavailable", backupUnavailableMessage)
		return
	}

	result, err := h.service.Drill(r.Context(), requestMeta(r, claims))
	if err != nil {
		h.writeDrillFailure(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

func (h *BackupHandler) writeDrillFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, backup.ErrBackupUnavailable) {
		writeJSONError(w, http.StatusNotImplemented, "backup_unavailable", backupUnavailableMessage)
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "restore_drill_failed",
		"Restore-Drill fehlgeschlagen: "+err.Error())
}

// Restore imports an uploaded backup envelope. Merge is the default;
// ?mode=replace is the only way to truncate first. The body is capped before
// the service sees it, and the committed flag in the response is the only
// thing the client may trust about the outcome.
func (h *BackupHandler) Restore(w http.ResponseWriter, r *http.Request) {
	claims, ok := adminClaims(r)
	if !ok {
		writeJSONError(w, http.StatusForbidden, "forbidden", "Nur Administratoren können Backups wiederherstellen")
		return
	}
	if h.service == nil {
		writeJSONError(w, http.StatusNotImplemented, "backup_unavailable", backupUnavailableMessage)
		return
	}

	mode := backup.RestoreModeMerge
	switch requested := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode"))); requested {
	case "":
		// merge is the default and needs no flag
	case string(backup.RestoreModeMerge):
		mode = backup.RestoreModeMerge
	case string(backup.RestoreModeReplace):
		mode = backup.RestoreModeReplace
	default:
		writeJSONError(w, http.StatusBadRequest, "restore_invalid_mode",
			"Unbekannter Wiederherstellungsmodus. Erlaubt sind 'merge' und 'replace'.")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxRestoreBytes)

	result, err := h.service.Restore(r.Context(), r.Body, mode, requestMeta(r, claims))
	if err != nil {
		h.writeRestoreFailure(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"mode":      string(result.Mode),
		"counts":    result.Counts,
		"committed": result.Committed,
	})
}

func (h *BackupHandler) writeRestoreFailure(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")

	var missingTables *backup.MissingTablesError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &missingTables):
		// Nothing was committed and no transaction was opened. The response
		// names the tables so the operator can see which live data an
		// uncompleted replace would have deleted.
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":          "restore_incomplete_backup",
			"message":        "Der Ersetzungsmodus verlangt eine Sicherung mit allen 24 Tabellen. Es fehlen: " + strings.Join(missingTables.Tables, ", "),
			"missing_tables": missingTables.Tables,
			"missing_count":  len(missingTables.Tables),
			"committed":      false,
		})
	case errors.As(err, &tooLarge):
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":     "restore_too_large",
			"message":   "Die hochgeladene Sicherung überschreitet das zulässige Größenlimit.",
			"committed": false,
		})
	case errors.Is(err, backup.ErrRestoreAuditFailed):
		// The data restore committed. Saying committed:false here would invite
		// a re-run of a non-idempotent replace.
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":     "restore_committed_audit_failed",
			"message":   "Die Wiederherstellung wurde committet, das Audit-Log konnte aber nicht geschrieben werden.",
			"committed": true,
		})
	case errors.Is(err, backup.ErrRestoreRolledBack):
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":     "restore_rolled_back",
			"message":   "Die Wiederherstellung wurde vollständig zurückgerollt. Es wurden keine Änderungen übernommen.",
			"committed": false,
		})
	case errors.Is(err, backup.ErrInvalidBackup):
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":     "restore_invalid_backup",
			"message":   fmt.Sprintf("Die hochgeladene Sicherung ist ungültig: %s", err.Error()),
			"committed": false,
		})
	case errors.Is(err, backup.ErrBackupUnavailable):
		writeJSONError(w, http.StatusNotImplemented, "backup_unavailable", backupUnavailableMessage)
	default:
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":     "restore_failed",
			"message":   err.Error(),
			"committed": false,
		})
	}
}

const backupUnavailableMessage = "Daten-Backup-Wiederherstellung benötigt eine PostgreSQL-Datenbank und ist im Demo-Modus nicht verfügbar."

const backupExportUnavailableMessage = "Der Daten-Backup-Export benötigt einen zeilenweise lesbaren Datenbankzugriff, der auf dieser Instanz nicht verfügbar ist."
