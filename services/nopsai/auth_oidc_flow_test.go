package nopsai

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestSafeReturnToRejectsUnsafeRedirects(t *testing.T) {
	tests := map[string]string{
		"":                         "/pipelineruns/main",
		"/system/access":           "/system/access",
		"//evil.example":           "/pipelineruns/main",
		"https://evil.example/app": "/pipelineruns/main",
		"/login?session_code=x":    "/pipelineruns/main",
		"/system\nHeader: value":   "/pipelineruns/main",
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
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/nopsai/callback?error=invalid_scope&error_description=Invalid+scopes%3A+openid+email+profile+groups&state=state", nil)
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

func TestNormalizeOIDCGroupMapping(t *testing.T) {
	got := normalizeOIDCGroupMapping(map[string]string{
		" nopsai-admins ": " SSO Admins ",
		" ":               "ignored",
		"ignored":         " ",
	})
	want := map[string]string{"nopsai-admins": "SSO Admins"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeOIDCGroupMapping() = %#v, want %#v", got, want)
	}
}

func TestNormalizeOIDCBasicRoleMapping(t *testing.T) {
	got := normalizeOIDCBasicRoleMapping(map[string]oidcBasicRoleGrantMapping{
		" team-1-owner ": {
			Role:     " Owner ",
			Resource: " folder:team-1 ",
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
			Resource:     "folder:team-1",
			ResourceType: "folder",
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
