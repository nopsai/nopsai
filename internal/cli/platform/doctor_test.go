package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/internal/cli/client"
)

func TestDoctorHealthyAndWarnings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/setup/preflight":
			_, _ = w.Write([]byte(`{"ready":true,"mode":"ready"}`))
		case "/metrics":
			_, _ = w.Write([]byte("nopsai_up 1\n"))
		case "/v1/auth/me":
			if r.Header.Get("Authorization") != "Bearer token" {
				w.WriteHeader(http.StatusUnauthorized)
			}
		case "/v1/system/dispatcher":
			_, _ = w.Write([]byte(`{"runners":[{}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	api, err := client.New(client.Options{BaseURL: server.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	doctor := Doctor{
		Client:          api,
		TokenConfigured: true,
		LookPath: func(tool string) (string, error) {
			if tool == "docker" {
				return "", context.Canceled
			}
			return "/usr/bin/" + tool, nil
		},
		RunCommand: func(context.Context, string, ...string) error { return nil },
	}
	checks := doctor.Run(context.Background())
	if HasErrors(checks) {
		t.Fatalf("unexpected errors: %#v", checks)
	}
	assertCheck(t, checks, "tool/docker", SeverityWarning, "not found")
	assertCheck(t, checks, "api/preflight", SeverityOK, "ready")
	assertCheck(t, checks, "aaa/authentication", SeverityOK, "accepted")
	assertCheck(t, checks, "monitoring/dispatcher", SeverityOK, "1 runner")
}

func TestDoctorWithoutTokenSkipsProtectedChecks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/setup/preflight" {
			_, _ = w.Write([]byte(`{"ready":true}`))
			return
		}
		if r.URL.Path == "/metrics" {
			return
		}
		t.Fatalf("unexpected protected request: %s", r.URL.Path)
	}))
	defer server.Close()
	api, _ := client.New(client.Options{BaseURL: server.URL})
	checks := (Doctor{Client: api, LookPath: func(string) (string, error) { return "/tool", nil }, RunCommand: func(context.Context, string, ...string) error { return nil }}).Run(context.Background())
	if HasErrors(checks) {
		t.Fatalf("unexpected errors: %#v", checks)
	}
	assertCheck(t, checks, "aaa/authentication", SeverityWarning, "no token")
	assertCheck(t, checks, "monitoring/dispatcher", SeverityWarning, "skipped")
}

func TestDoctorReportsRemoteFailures(t *testing.T) {
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		checkName string
		severity  Severity
		message   string
	}{
		{"not ready", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/setup/preflight" {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"ready":false,"mode":"preflight_only"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}, "api/preflight", SeverityError, "not ready"},
		{"invalid preflight", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/setup/preflight" {
				_, _ = w.Write([]byte(`nope`))
				return
			}
			_, _ = w.Write([]byte(`{}`))
		}, "api/preflight", SeverityError, "invalid"},
		{"unauthorized", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/setup/preflight" {
				_, _ = w.Write([]byte(`{"ready":true}`))
				return
			}
			if r.URL.Path == "/v1/auth/me" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{}`))
		}, "aaa/authentication", SeverityError, "rejected"},
		{"dispatcher forbidden", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/setup/preflight" {
				_, _ = w.Write([]byte(`{"ready":true}`))
				return
			}
			if r.URL.Path == "/v1/system/dispatcher" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{}`))
		}, "monitoring/dispatcher", SeverityWarning, "lacks"},
		{"dispatcher degraded", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/setup/preflight" {
				_, _ = w.Write([]byte(`{"ready":true}`))
				return
			}
			if r.URL.Path == "/v1/system/dispatcher" {
				_, _ = w.Write([]byte(`{"dispatcher_error":"unavailable"}`))
				return
			}
			_, _ = w.Write([]byte(`{}`))
		}, "monitoring/dispatcher", SeverityWarning, "unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			api, _ := client.New(client.Options{BaseURL: server.URL, Token: "token"})
			checks := (Doctor{Client: api, TokenConfigured: true, LookPath: func(string) (string, error) { return "/tool", nil }, RunCommand: func(context.Context, string, ...string) error { return nil }}).Run(context.Background())
			assertCheck(t, checks, test.checkName, test.severity, test.message)
		})
	}
}

func TestDoctorWithoutClient(t *testing.T) {
	checks := (Doctor{LookPath: func(string) (string, error) { return "/tool", nil }}).Run(context.Background())
	if !HasErrors(checks) {
		t.Fatal("expected client configuration error")
	}
	assertCheck(t, checks, "api/connectivity", SeverityError, "not configured")
}

func TestDoctorReportsPlatformConnectivityWarnings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/setup/preflight" {
			_, _ = w.Write([]byte(`{"ready":true}`))
		}
	}))
	defer server.Close()
	api, _ := client.New(client.Options{BaseURL: server.URL})
	checks := (Doctor{
		Client:   api,
		LookPath: func(tool string) (string, error) { return "/bin/" + tool, nil },
		RunCommand: func(_ context.Context, command string, _ ...string) error {
			if command == "kubectl" {
				return context.DeadlineExceeded
			}
			return nil
		},
	}).Run(context.Background())
	assertCheck(t, checks, "kubernetes/connectivity", SeverityWarning, "unreachable")
	assertCheck(t, checks, "docker/connectivity", SeverityOK, "reachable")
}

func assertCheck(t *testing.T, checks []Check, name string, severity Severity, message string) {
	t.Helper()
	for _, check := range checks {
		if check.Name == name {
			if check.Severity != severity || !strings.Contains(check.Message, message) {
				t.Fatalf("check %s = %#v", name, check)
			}
			return
		}
	}
	t.Fatalf("check %s not found in %#v", name, checks)
}
