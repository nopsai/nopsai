package nopsai

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nopsai/config"
	"nopsai/pkg/buildinfo"
	"nopsai/pkg/license"
)

// newAppWithLicenseKey builds the smallest App that resolves an entitlement:
// a config snapshot carrying the key, and no database, so any count lookup
// fails and the fail-closed path is the one under test.
func newAppWithLicenseKey(key string) *App {
	return &App{cfg: &config.Config{LicenseKey: key}}
}

// A build with no verification key cannot tell a commercially licensed
// installation from any other, so it must not apply ceilings. Otherwise every
// installation that predates licensing is silently capped.
func TestEnforcementIsInertOnBuildsThatCannotVerify(t *testing.T) {
	original := buildinfo.LicensePublicKey
	t.Cleanup(func() { buildinfo.LicensePublicKey = original })
	buildinfo.LicensePublicKey = ""

	// A nil-database App would fail any count lookup, so reaching the count at
	// all would panic or error. Returning nil proves enforcement short-circuits.
	app := &App{}
	if err := app.enforceUserEntitlement(t.Context()); err != nil {
		t.Fatalf("enforceUserEntitlement() = %v, want nil on a non-verifying build", err)
	}
	if err := app.enforceTeamEntitlement(t.Context()); err != nil {
		t.Fatalf("enforceTeamEntitlement() = %v, want nil on a non-verifying build", err)
	}
}

// Where a commercial licence records a limit, a count that cannot be read must
// block rather than pass: being unable to evaluate a limit is not the same as
// being under it.
func TestEnforcementFailsClosedWhenUsageCannotBeRead(t *testing.T) {
	original := buildinfo.LicensePublicKey
	t.Cleanup(func() { buildinfo.LicensePublicKey = original })
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	buildinfo.LicensePublicKey = base64.StdEncoding.EncodeToString(publicKey)

	capped, err := license.Sign(license.Claims{
		Licensee:  "Acme BV",
		Tier:      license.TierCommercial,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().AddDate(1, 0, 0),
		MaxUsers:  5,
	}, privateKey)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	app := newAppWithLicenseKey(string(capped))
	err = app.enforceUserEntitlement(t.Context())
	if err == nil {
		t.Fatal("enforcement must refuse when current usage cannot be read")
	}
	if _, ok := err.(entitlementError); !ok {
		t.Fatalf("error = %T, want entitlementError so the handler answers 402", err)
	}
}

// NopsAI is free and uncapped for non-commercial use, so an installation with
// no commercial key has no ceiling to evaluate. Nothing is read, and nothing is
// refused — not even when the database is unreachable, because there is no
// limit to be unable to check.
func TestNonCommercialUseIsNeverRefused(t *testing.T) {
	original := buildinfo.LicensePublicKey
	t.Cleanup(func() { buildinfo.LicensePublicKey = original })
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	buildinfo.LicensePublicKey = base64.StdEncoding.EncodeToString(publicKey)

	app := newAppWithLicenseKey("")
	if err := app.enforceUserEntitlement(t.Context()); err != nil {
		t.Fatalf("enforceUserEntitlement() = %v, want nil under the non-commercial licence", err)
	}
	if err := app.enforceTeamEntitlement(t.Context()); err != nil {
		t.Fatalf("enforceTeamEntitlement() = %v, want nil under the non-commercial licence", err)
	}
}

func TestEntitlementDenialAnswers402(t *testing.T) {
	app := &App{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/teams", nil)

	handled := app.writeEntitlementError(recorder, request, entitlementError{
		resource: "team",
		message:  "This installation has reached the team limit recorded in its commercial licence.",
	})
	if !handled {
		t.Fatal("an entitlement error must be handled")
	}
	if recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusPaymentRequired)
	}
	if body := recorder.Body.String(); body == "" {
		t.Fatal("the denial must name the limit that was hit")
	}
}

func TestNonEntitlementErrorsAreLeftToTheCaller(t *testing.T) {
	app := &App{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/teams", nil)

	if app.writeEntitlementError(recorder, request, http.ErrBodyNotAllowed) {
		t.Fatal("only entitlement errors may be answered as 402")
	}
}

// A licence that omits a limit must never be read as forbidding everything.
func TestEntitlementLimitsTreatZeroAsUnlimited(t *testing.T) {
	entitlement := license.Entitlement{Licensed: true, Claims: license.Claims{Licensee: "Acme BV"}}
	for _, current := range []int{0, 50, 5000} {
		if !entitlement.AllowsUsers(current) || !entitlement.AllowsTeams(current) {
			t.Fatalf("an omitted limit blocked at %d", current)
		}
	}
}
