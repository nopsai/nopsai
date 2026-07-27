package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewServerAppliesProductionTimeouts(t *testing.T) {
	server := NewServer(":8080", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	if server.ReadHeaderTimeout != DefaultReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %s, want %s", server.ReadHeaderTimeout, DefaultReadHeaderTimeout)
	}
	if server.ReadTimeout != DefaultReadTimeout {
		t.Fatalf("ReadTimeout = %s, want %s", server.ReadTimeout, DefaultReadTimeout)
	}
	if server.WriteTimeout != DefaultWriteTimeout {
		t.Fatalf("WriteTimeout = %s, want %s", server.WriteTimeout, DefaultWriteTimeout)
	}
	if server.IdleTimeout != DefaultIdleTimeout {
		t.Fatalf("IdleTimeout = %s, want %s", server.IdleTimeout, DefaultIdleTimeout)
	}
}

func TestReadRequestBodyReportsTooLarge(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("abcdef"))
	rec := httptest.NewRecorder()

	_, err := ReadRequestBody(rec, req, 3)
	if err == nil {
		t.Fatal("ReadRequestBody() error = nil, want max body error")
	}
	if !IsRequestBodyTooLarge(err) {
		t.Fatalf("IsRequestBodyTooLarge(%v) = false", err)
	}
}
