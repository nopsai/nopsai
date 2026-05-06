package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAuthLogoutRejectsInvalidJSON(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", strings.NewReader("{"))
	rec := httptest.NewRecorder()

	app.handleAuthLogout(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("handleAuthLogout() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAuthLogoutRejectsEmptyRefreshToken(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", strings.NewReader(`{"refresh_token":"   "}`))
	rec := httptest.NewRecorder()

	app.handleAuthLogout(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("handleAuthLogout() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
