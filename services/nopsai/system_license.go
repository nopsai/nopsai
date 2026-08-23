package nopsai

import (
	"net/http"
	"time"

	"nopsai/pkg/buildinfo"
	"nopsai/pkg/license"
)

// currentEntitlement resolves what this installation may run, from the licence
// key in the live configuration and the verification key compiled into this
// build. It is resolved per call rather than cached, so a key changed through
// GitOps config sync takes effect without a restart, and a key that lapses
// while the process runs stops granting anything.
func (a *App) currentEntitlement() license.Entitlement {
	rawKey := ""
	if a != nil {
		rawKey = a.getConfigSnapshot().LicenseKey
	}
	publicKey, _ := license.ParsePublicKey(buildinfo.LicensePublicKey)
	return license.Resolve(rawKey, publicKey, time.Now().UTC())
}

type systemLicenseLimits struct {
	// Zero means unlimited. The UI renders that as "Unlimited" rather than as
	// a limit of nothing.
	MaxUsers          int `json:"max_users"`
	MaxTeams          int `json:"max_teams"`
	MaxConcurrentRuns int `json:"max_concurrent_runs"`
}

type systemLicenseResponse struct {
	Licensed  bool                `json:"licensed"`
	Tier      string              `json:"tier"`
	Licensee  string              `json:"licensee,omitempty"`
	LicenseID string              `json:"license_id,omitempty"`
	ExpiresAt string              `json:"expires_at,omitempty"`
	Reason    string              `json:"reason,omitempty"`
	Features  []string            `json:"features,omitempty"`
	Limits    systemLicenseLimits `json:"limits"`
	// Usage lets the UI show headroom next to each limit instead of only
	// telling an operator they hit one.
	Usage systemLicenseUsage `json:"usage"`
}

type systemLicenseUsage struct {
	Users int `json:"users"`
	Teams int `json:"teams"`
}

func (a *App) handleGetSystemLicense(w http.ResponseWriter, r *http.Request) {
	entitlement := a.currentEntitlement()

	response := systemLicenseResponse{
		Licensed:  entitlement.Licensed,
		Tier:      string(entitlement.Claims.Tier),
		Licensee:  entitlement.Claims.Licensee,
		LicenseID: entitlement.Claims.LicenseID,
		Reason:    entitlement.Reason,
		Features:  entitlement.Claims.Features,
		Limits: systemLicenseLimits{
			MaxUsers:          entitlement.Claims.MaxUsers,
			MaxTeams:          entitlement.Claims.MaxTeams,
			MaxConcurrentRuns: entitlement.Claims.MaxConcurrentRuns,
		},
	}
	if !entitlement.Claims.ExpiresAt.IsZero() {
		response.ExpiresAt = entitlement.Claims.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if counts, err := a.setupCounts(r.Context()); err == nil {
		response.Usage = systemLicenseUsage{Users: counts.Users, Teams: counts.Teams}
	}

	writeJSON(w, http.StatusOK, response)
}
