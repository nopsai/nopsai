package servicelog

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/pkg/correlation"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestHTTPMiddlewarePropagatesCorrelationAndLogsRequest(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() { log.Logger = previous })

	handler := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := correlation.RequestIDFromContext(r.Context()); got != "req-http" {
			t.Fatalf("request id in context = %q, want req-http", got)
		}
		if got := correlation.TraceparentFromContext(r.Context()); got != "trace-http" {
			t.Fatalf("traceparent in context = %q, want trace-http", got)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/test", strings.NewReader("body"))
	req.Header.Set(correlation.RequestIDHeader, "req-http")
	req.Header.Set(correlation.TraceparentHeader, "trace-http")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get(correlation.RequestIDHeader) != "req-http" {
		t.Fatalf("response request id = %q, want req-http", rec.Header().Get(correlation.RequestIDHeader))
	}
	if !strings.Contains(logs.String(), "http_request") || !strings.Contains(logs.String(), "req-http") {
		t.Fatalf("logs = %q, want http_request with request id", logs.String())
	}
}
