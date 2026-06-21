package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSendsAuthenticatedPrefixedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/v1/runs" || r.URL.RawQuery != "limit=5" {
			t.Errorf("URL = %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "nopsai-cli/test" {
			t.Errorf("User-Agent = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"name":"release"}` {
			t.Errorf("body = %q", body)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	api, err := New(Options{BaseURL: server.URL + "/prefix", Token: " token ", UserAgent: "nopsai-cli/test"})
	if err != nil {
		t.Fatal(err)
	}
	request, err := api.NewRequest(http.MethodPost, "/v1/runs?limit=5", strings.NewReader(`{"name":"release"}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := api.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestClientValidatesBaseURLAndRequestPath(t *testing.T) {
	for _, raw := range []string{"", "relative", "ftp://example.com", "https://user@example.com", "https://example.com?q=1", "https://example.com/#x"} {
		if _, err := New(Options{BaseURL: raw}); err == nil {
			t.Errorf("New(%q) succeeded", raw)
		}
	}
	if _, err := New(Options{BaseURL: "https://example.com", Token: "one\ntwo"}); err == nil {
		t.Fatal("multiline token accepted")
	}
	api, err := New(Options{BaseURL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"relative", "://bad"} {
		if _, err := api.NewRequest(http.MethodGet, path, nil); err == nil {
			t.Errorf("NewRequest(%q) succeeded", path)
		}
	}
	absolute, _ := http.NewRequest(http.MethodGet, "https://other.example/v1", nil)
	if _, err := api.Do(absolute); err == nil {
		t.Fatal("absolute request URL was accepted")
	}
	traversal, _ := http.NewRequest(http.MethodGet, "/v1/../admin", nil)
	if _, err := api.Do(traversal); err == nil {
		t.Fatal("parent traversal was accepted")
	}
}

func TestClientRejectsCrossOriginRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("cross-origin redirect target should not be called")
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, target.URL, http.StatusFound)
	}))
	defer source.Close()
	api, err := New(Options{BaseURL: source.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := api.NewRequest(http.MethodGet, "/redirect", nil)
	if _, err := api.Do(request); err == nil || !strings.Contains(err.Error(), "across API origins") {
		t.Fatalf("redirect error = %v", err)
	}
}

func TestClientUsesDefaultUserAgentAndCallerRedirectPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusFound)
			return
		}
		if got := r.Header.Get("User-Agent"); got != defaultUserAgent {
			t.Errorf("User-Agent = %q", got)
		}
	}))
	defer server.Close()
	httpClient := server.Client()
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	api, err := New(Options{BaseURL: server.URL, HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := api.NewRequest(http.MethodGet, "/start", nil)
	response, err := api.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestClientCanExplicitlyOmitAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "" {
			t.Errorf("Authorization = %q", authorization)
		}
	}))
	defer server.Close()
	api, err := New(Options{BaseURL: server.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := api.NewRequest(http.MethodGet, "/public", nil)
	request.Header.Set("Authorization", "Bearer caller-value")
	response, err := api.DoUnauthenticated(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}
