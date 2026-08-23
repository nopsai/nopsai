package nopsai

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nopsai/config"
	"nopsai/pkg/buildinfo"
	"nopsai/pkg/license"
	"nopsai/pkg/licensenotice"
)

// The notice has to be readable before anyone can be asked to accept it, but
// recording that acceptance must stay an authenticated administrator action.
func TestSetupLicenseReadIsPublicButAcceptIsNot(t *testing.T) {
	if !isPublicPath("/v1/setup/license") {
		t.Error("GET /v1/setup/license must be public so the notice can be read before authentication")
	}
	if isPublicPath("/v1/setup/license/accept") {
		t.Error("POST /v1/setup/license/accept must not be public")
	}
}

func TestGetSetupLicenseServesTheShippedNotice(t *testing.T) {
	app := &App{}
	// No database is configured, so acceptance cannot be evaluated.
	recorder := httptest.NewRecorder()
	app.handleGetSetupLicense(recorder, httptest.NewRequest(http.MethodGet, "/v1/setup/license", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d when acceptance state cannot be read", recorder.Code, http.StatusInternalServerError)
	}
}

func TestAcceptSetupLicenseRejectsBeforeRecordingAnything(t *testing.T) {
	// Each case must be refused before the handler reaches the database, so a
	// nil-database App is enough and proves nothing was written.
	app := &App{}

	cases := []struct {
		name string
		body string
		want int
	}{
		{
			name: "acceptance must be explicit",
			body: `{"accept":false,"document_sha256":"` + licensenotice.SHA256() + `"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "a stale document digest is refused",
			body: `{"accept":true,"document_sha256":"0000000000000000000000000000000000000000000000000000000000000000"}`,
			want: http.StatusConflict,
		},
		{
			name: "an unidentified caller cannot accept",
			body: `{"accept":true,"document_sha256":"` + licensenotice.SHA256() + `"}`,
			want: http.StatusForbidden,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/setup/license/accept", strings.NewReader(testCase.body))
			recorder := httptest.NewRecorder()
			app.handleAcceptSetupLicense(recorder, request)
			if recorder.Code != testCase.want {
				t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, testCase.want, recorder.Body.String())
			}
		})
	}
}

// An installation that cannot evaluate acceptance is not an installation that
// may skip it.
func TestLicenseAcceptanceFailsClosedWithoutDatabase(t *testing.T) {
	app := &App{}
	accepted, _, _, _, err := app.licenseAcceptance(t.Context())
	if err == nil {
		t.Fatal("licenseAcceptance must report an error when it cannot be evaluated")
	}
	if accepted {
		t.Fatal("licenseAcceptance must never report accepted when it could not be evaluated")
	}
}

func TestSetupLicenseDocumentIdentityIsRecordable(t *testing.T) {
	if len(licensenotice.SHA256()) != 64 {
		t.Fatalf("document digest = %q, want a 64-character hex sha256", licensenotice.SHA256())
	}
	if !strings.Contains(licensenotice.Text, "NopsAI Proprietary Software Notice") {
		t.Fatal("the served notice must be the proprietary notice")
	}
}

// The production gate must distinguish "this operator has not licensed the
// installation" from "this build cannot check licences at all".
func TestLicenseEntitlementGateOnlyBlocksBuildsThatCanVerify(t *testing.T) {
	original := buildinfo.LicensePublicKey
	t.Cleanup(func() { buildinfo.LicensePublicKey = original })

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	encodedPublic := base64.StdEncoding.EncodeToString(publicKey)

	validKey, err := license.Sign(license.Claims{
		Licensee:  "Acme BV",
		Tier:      license.TierEnterprise,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().AddDate(1, 0, 0),
	}, privateKey)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	cases := []struct {
		name         string
		buildKey     string
		licenseKey   string
		strict       bool
		wantRequired bool
		wantStatus   string
	}{
		{
			name:       "a build with no verification key never blocks, even in production",
			buildKey:   "",
			strict:     true,
			wantStatus: "warning",
		},
		{
			name:       "outside production an unlicensed install is advisory",
			buildKey:   encodedPublic,
			strict:     false,
			wantStatus: "warning",
		},
		{
			name:         "in production a verifying build with no key blocks",
			buildKey:     encodedPublic,
			strict:       true,
			wantRequired: true,
			wantStatus:   "error",
		},
		{
			name:         "in production a valid key passes",
			buildKey:     encodedPublic,
			licenseKey:   string(validKey),
			strict:       true,
			wantRequired: true,
			wantStatus:   "success",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			buildinfo.LicensePublicKey = testCase.buildKey
			check := licenseEntitlementCheck(&config.Config{LicenseKey: testCase.licenseKey}, testCase.strict)
			if check.Status != testCase.wantStatus {
				t.Errorf("status = %q, want %q (message: %s)", check.Status, testCase.wantStatus, check.Message)
			}
			if check.Required != testCase.wantRequired {
				t.Errorf("required = %v, want %v", check.Required, testCase.wantRequired)
			}
			if strings.TrimSpace(check.Message) == "" {
				t.Error("the gate must always explain itself")
			}
		})
	}
}
