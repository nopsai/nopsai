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
	for _, expected := range []string{"NopsAI Licence", "PolyForm Noncommercial License 1.0.0", "Commercial use is not granted by this licence"} {
		if !strings.Contains(licensenotice.Text, expected) {
			t.Fatalf("the served notice must be the shipped licence; missing %q", expected)
		}
	}
}

// The licence check states a position; it never blocks. NopsAI is free for any
// non-commercial purpose, so an installation with no commercial key is entitled
// to run in production, and whether a purpose is commercial is something the
// software cannot observe and never asks.
func TestLicenseEntitlementCheckNeverBlocksStartup(t *testing.T) {
	original := buildinfo.LicensePublicKey
	t.Cleanup(func() { buildinfo.LicensePublicKey = original })

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	encodedPublic := base64.StdEncoding.EncodeToString(publicKey)

	validKey, err := license.Sign(license.Claims{
		Licensee:  "Acme BV",
		Tier:      license.TierCommercial,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().AddDate(1, 0, 0),
	}, privateKey)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	cases := []struct {
		name        string
		buildKey    string
		licenseKey  string
		strict      bool
		wantStatus  string
		wantMessage string
	}{
		{
			name:        "outside production the non-commercial licence needs no prompting",
			buildKey:    encodedPublic,
			strict:      false,
			wantStatus:  "success",
			wantMessage: "non-commercial licence",
		},
		{
			name:        "in production the commercial boundary is named, not enforced",
			buildKey:    encodedPublic,
			strict:      true,
			wantStatus:  "warning",
			wantMessage: "contact@nopsai.com",
		},
		{
			name:        "a build with no verification key still reports its position",
			buildKey:    "",
			strict:      true,
			wantStatus:  "warning",
			wantMessage: "non-commercial licence",
		},
		{
			name:        "a valid commercial key names its licensee",
			buildKey:    encodedPublic,
			licenseKey:  string(validKey),
			strict:      true,
			wantStatus:  "success",
			wantMessage: "Acme BV",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			buildinfo.LicensePublicKey = testCase.buildKey
			check := licenseEntitlementCheck(&config.Config{LicenseKey: testCase.licenseKey}, testCase.strict)
			if check.Status != testCase.wantStatus {
				t.Errorf("status = %q, want %q (message: %s)", check.Status, testCase.wantStatus, check.Message)
			}
			if check.Required {
				t.Error("the licence check must never be required, so it can never block startup")
			}
			if !strings.Contains(check.Message, testCase.wantMessage) {
				t.Errorf("message = %q, want it to mention %q", check.Message, testCase.wantMessage)
			}
		})
	}
}
