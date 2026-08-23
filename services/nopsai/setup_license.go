package nopsai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/licensenotice"
	"nopsai/services/nopsai/pkg/audit"
)

// Acceptance is recorded in setup_state, which is already a key/value table, so
// the licence gate needs no schema migration.
const (
	setupStateKeyLicenseAcceptedAt      = "license_accepted_at"
	setupStateKeyLicenseAcceptedBy      = "license_accepted_by"
	setupStateKeyLicenseDocumentVersion = "license_document_version"
	setupStateKeyLicenseDocumentSHA256  = "license_document_sha256"
)

// errSetupLicenseNotAccepted blocks setup completion. It is a distinct error so
// the handler can answer with a precondition failure rather than a generic 500.
var errSetupLicenseNotAccepted = errors.New("the proprietary licence notice must be accepted before setup can complete")

type setupLicenseResponse struct {
	Text            string `json:"text"`
	DocumentVersion string `json:"document_version"`
	DocumentSHA256  string `json:"document_sha256"`
	Accepted        bool   `json:"accepted"`
	AcceptedAt      string `json:"accepted_at,omitempty"`
	AcceptedBy      string `json:"accepted_by,omitempty"`
	// AcceptedVersion is the notice the installation previously agreed to. It
	// differs from DocumentVersion after an upgrade that changed the wording.
	AcceptedVersion string `json:"accepted_version,omitempty"`
}

type setupLicenseAcceptRequest struct {
	// DocumentSHA256 must match the notice the server is serving. It stops an
	// administrator from accepting a stale copy held by a cached browser tab.
	DocumentSHA256 string `json:"document_sha256"`
	Accept         bool   `json:"accept"`
}

// licenseAcceptance reports whether the current notice has been accepted. An
// installation that accepted an earlier notice version is not accepted now: the
// wording it agreed to no longer matches what it is running.
func (a *App) licenseAcceptance(ctx context.Context) (accepted bool, at string, by string, version string, err error) {
	if a == nil || a.db == nil {
		return false, "", "", "", errors.New("database is not configured")
	}
	rows, err := a.db.Query(ctx, `
		SELECT key, value
		FROM setup_state
		WHERE key = ANY($1)
	`, []string{
		setupStateKeyLicenseAcceptedAt,
		setupStateKeyLicenseAcceptedBy,
		setupStateKeyLicenseDocumentVersion,
		setupStateKeyLicenseDocumentSHA256,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, "", "", "", err
	}
	state := map[string]string{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var key, value string
			if scanErr := rows.Scan(&key, &value); scanErr != nil {
				return false, "", "", "", scanErr
			}
			state[key] = strings.TrimSpace(value)
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			return false, "", "", "", rowsErr
		}
	}

	at = state[setupStateKeyLicenseAcceptedAt]
	by = state[setupStateKeyLicenseAcceptedBy]
	version = state[setupStateKeyLicenseDocumentVersion]
	accepted = at != "" && state[setupStateKeyLicenseDocumentSHA256] == licensenotice.SHA256()
	return accepted, at, by, version, nil
}

func (a *App) handleGetSetupLicense(w http.ResponseWriter, r *http.Request) {
	accepted, at, by, version, err := a.licenseAcceptance(r.Context())
	if err != nil {
		http.Error(w, "failed to read licence acceptance state", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, setupLicenseResponse{
		Text:            licensenotice.Text,
		DocumentVersion: licensenotice.Version,
		DocumentSHA256:  licensenotice.SHA256(),
		Accepted:        accepted,
		AcceptedAt:      at,
		AcceptedBy:      by,
		AcceptedVersion: version,
	})
}

func (a *App) handleAcceptSetupLicense(w http.ResponseWriter, r *http.Request) {
	var req setupLicenseAcceptRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if !req.Accept {
		http.Error(w, "licence acceptance requires accept=true", http.StatusBadRequest)
		return
	}
	// Fail closed on a digest mismatch rather than recording acceptance of
	// whatever the server happens to hold.
	if strings.TrimSpace(req.DocumentSHA256) != licensenotice.SHA256() {
		http.Error(w, "licence document has changed; reload the notice and accept the current version", http.StatusConflict)
		return
	}

	acceptedBy := actorIDFromRequest(r)
	if acceptedBy == "" {
		http.Error(w, "licence acceptance requires an identified administrator", http.StatusForbidden)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := a.db.Exec(r.Context(), `
		INSERT INTO setup_state (key, value, updated_at)
		VALUES ($1, $2, NOW()), ($3, $4, NOW()), ($5, $6, NOW()), ($7, $8, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`,
		setupStateKeyLicenseAcceptedAt, now,
		setupStateKeyLicenseAcceptedBy, acceptedBy,
		setupStateKeyLicenseDocumentVersion, licensenotice.Version,
		setupStateKeyLicenseDocumentSHA256, licensenotice.SHA256(),
	); err != nil {
		http.Error(w, "failed to record licence acceptance", http.StatusInternalServerError)
		return
	}

	a.auditLicenseAcceptance(r, acceptedBy)

	writeJSON(w, http.StatusOK, setupLicenseResponse{
		Text:            licensenotice.Text,
		DocumentVersion: licensenotice.Version,
		DocumentSHA256:  licensenotice.SHA256(),
		Accepted:        true,
		AcceptedAt:      now,
		AcceptedBy:      acceptedBy,
		AcceptedVersion: licensenotice.Version,
	})
}

// auditLicenseAcceptance records who agreed to which notice. The acceptance row
// in setup_state is the authoritative record; the audit entry is what makes it
// visible in the normal audit trail.
func (a *App) auditLicenseAcceptance(r *http.Request, acceptedBy string) {
	if a == nil || a.auditLogger == nil {
		return
	}
	_ = a.auditLogger.Write(r.Context(), audit.Entry{
		Action:   "system.license.accept",
		Resource: "system:license",
		Result:   "success",
		ActorSub: acceptedBy,
		Metadata: map[string]any{
			"document_version": licensenotice.Version,
			"document_sha256":  licensenotice.SHA256(),
		},
	})
}
