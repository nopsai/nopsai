package nopsai

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"nopsai/config"
)

func TestSafeReturnToRejectsUnsafeRedirects(t *testing.T) {
	tests := map[string]string{
		"":                         "/pipelineruns/main",
		"/system/access":           "/system/access",
		"//evil.example":           "/pipelineruns/main",
		"https://evil.example/app": "/pipelineruns/main",
		"/login?session_code=x":    "/pipelineruns/main",
		"/system\nHeader: value":   "/pipelineruns/main",
		`/\evil.example`:           "/pipelineruns/main",
		`/\/evil.example`:          "/pipelineruns/main",
		`/system\..\..\evil`:       "/pipelineruns/main",
	}
	for input, want := range tests {
		if got := safeReturnTo(input); got != want {
			t.Fatalf("safeReturnTo(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCallbackRedirectURLUsesHashRouterLoginRoute(t *testing.T) {
	got := callbackRedirectURL("/system/access", "exchange-code")
	want := "/#/login?return_to=%2Fsystem%2Faccess&session_code=exchange-code"
	if got != want {
		t.Fatalf("callbackRedirectURL() = %q, want %q", got, want)
	}
}

func TestHandleAuthOIDCCallbackReportsProviderAuthorizationError(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/nopsai/callback?error=invalid_scope&error_description=Invalid+scopes%3A+openid+email+profile+teams&state=state", nil)
	req.SetPathValue("provider", "nopsai")
	rr := httptest.NewRecorder()

	app.handleAuthOIDCCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "oidc authorization failed: invalid_scope") {
		t.Fatalf("body = %q, want provider authorization error", body)
	}
	if strings.Contains(body, "missing oidc code or state") {
		t.Fatalf("body = %q, should not report missing code or state", body)
	}
}

func TestNormalizeOIDCDomainMappings(t *testing.T) {
	got := normalizeOIDCDomainMappings(map[string]string{
		"@Company.COM": " Corporate ",
		"example.com":  "MICROSOFT",
		" ":            "ignored",
	})
	want := map[string]string{
		"company.com": "corporate",
		"example.com": "microsoft",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeOIDCDomainMappings() = %#v, want %#v", got, want)
	}
}

func TestNormalizeOIDCTeamMapping(t *testing.T) {
	got := normalizeOIDCTeamMapping(map[string]string{
		" nopsai-admins ": " SSO Admins ",
		" ":               "ignored",
		"ignored":         " ",
	})
	want := map[string]string{"nopsai-admins": "SSO Admins"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeOIDCTeamMapping() = %#v, want %#v", got, want)
	}
}

func TestNormalizeOIDCBasicRoleMapping(t *testing.T) {
	got := normalizeOIDCBasicRoleMapping(map[string]oidcBasicRoleGrantMapping{
		" team-1-owner ": {
			Role:     " Owner ",
			Resource: " team:team-1 ",
		},
		"platform-admin": {
			Role:     "admin",
			Resource: "platform:default",
		},
		"ignored": {
			Role: "viewer",
		},
	})
	want := map[string]oidcBasicRoleGrantMapping{
		"team-1-owner": {
			Role:         "owner",
			Resource:     "team:team-1",
			ResourceType: "team",
			ResourceID:   "team-1",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeOIDCBasicRoleMapping() = %#v, want %#v", got, want)
	}
}

func TestStringSliceClaimNormalizesCSVAndArrays(t *testing.T) {
	got := stringSliceClaim([]any{"nopsai-admins", "nopsai-admins", "nopsai-viewers"})
	want := []string{"nopsai-admins", "nopsai-viewers"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stringSliceClaim(array) = %#v, want %#v", got, want)
	}
	got = stringSliceClaim("nopsai-admins, nopsai-viewers")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stringSliceClaim(csv) = %#v, want %#v", got, want)
	}
}

func TestEmailDomainAllowedDefaultsToAllowProviderWithoutDomainList(t *testing.T) {
	if !emailDomainAllowed("company.com", nil) {
		t.Fatal("emailDomainAllowed() = false, want true when provider has no domain allowlist")
	}
	if emailDomainAllowed("other.com", []string{"company.com"}) {
		t.Fatal("emailDomainAllowed() = true, want false for non-allowlisted domain")
	}
}

func TestOIDCEmailVerificationStatusFromClaim(t *testing.T) {
	tests := []struct {
		name          string
		email         string
		claim         any
		wantStatus    string
		wantTrust     bool
		wantMalformed bool
	}{
		{name: "verified boolean", email: "alice@example.com", claim: true, wantStatus: oidcEmailVerificationVerified, wantTrust: true},
		{name: "verified string", email: "alice@example.com", claim: "true", wantStatus: oidcEmailVerificationVerified, wantTrust: true},
		{name: "unverified boolean", email: "alice@example.com", claim: false, wantStatus: oidcEmailVerificationUnverified},
		{name: "unverified string", email: "alice@example.com", claim: "false", wantStatus: oidcEmailVerificationUnverified},
		{name: "missing claim", email: "alice@example.com", wantStatus: oidcEmailVerificationUnknown},
		{name: "malformed claim", email: "alice@example.com", claim: "yes", wantStatus: oidcEmailVerificationUnknown, wantMalformed: true},
		{name: "missing email", claim: true, wantStatus: oidcEmailVerificationNotProvided},
		{name: "missing email with malformed claim", claim: "yes", wantStatus: oidcEmailVerificationNotProvided, wantMalformed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotTrust, gotMalformed := oidcEmailVerificationStatusFromClaim(tt.email, tt.claim)
			if gotStatus != tt.wantStatus || gotTrust != tt.wantTrust || gotMalformed != tt.wantMalformed {
				t.Fatalf("oidcEmailVerificationStatusFromClaim() = (%q, %v, %v), want (%q, %v, %v)", gotStatus, gotTrust, gotMalformed, tt.wantStatus, tt.wantTrust, tt.wantMalformed)
			}
		})
	}
}

func TestExternalAuthCallbackBaseURLUsesConfiguredPublicURL(t *testing.T) {
	app := &App{cfg: &config.Config{PublicURL: "https://nopsai.example.com/app/"}}
	req := httptest.NewRequest(http.MethodGet, "http://attacker.example/v1/auth/oidc/nopsai/start", nil)
	req.Header.Set("X-Forwarded-Proto", "https")

	got, err := app.oidcCallbackURL(req, "nopsai")
	if err != nil {
		t.Fatalf("oidcCallbackURL() error = %v", err)
	}
	want := "https://nopsai.example.com/app/v1/auth/oidc/nopsai/callback"
	if got != want {
		t.Fatalf("oidcCallbackURL() = %q, want %q", got, want)
	}
}

func TestExternalAuthCallbackBaseURLRequiresPublicURLInProduction(t *testing.T) {
	app := &App{cfg: &config.Config{Environment: "production"}}
	req := httptest.NewRequest(http.MethodGet, "http://nopsai.example.com/v1/auth/oidc/nopsai/start", nil)

	if _, err := app.oidcCallbackURL(req, "nopsai"); err == nil {
		t.Fatal("oidcCallbackURL() error = nil, want production public_url requirement")
	}
}

func TestOIDCIdentityAuditMetadataFlagsMalformedEmailVerificationClaim(t *testing.T) {
	metadata := oidcIdentityAuditMetadata(oidcVerifiedIdentity{EmailVerificationClaimMalformed: true}, map[string]any{
		"reason": "user_resolution_failed",
	})

	if metadata["email_verification_claim_malformed"] != true {
		t.Fatalf("metadata = %#v, want malformed email verification flag", metadata)
	}
	if metadata["reason"] != "user_resolution_failed" {
		t.Fatalf("metadata reason = %#v", metadata["reason"])
	}
}
