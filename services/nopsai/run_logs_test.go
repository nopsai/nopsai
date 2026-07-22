package nopsai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"nopsai/pkg/correlation"
	"nopsai/pkg/serviceauth"
	"nopsai/services/nopsai/pkg/auth"
)

func TestNormalizeRunLogIngestPayloadUsesCorrelationAndServiceClaims(t *testing.T) {
	ctx := correlation.WithTraceparent(correlation.WithRequestID(context.Background(), "req-run-log"), "trace-run-log")
	ctx = context.WithValue(ctx, ctxKeyRequestID, "req-run-log")
	ctx = auth.WithClaims(ctx, &auth.Claims{
		Sub:      "dispatcher",
		Provider: serviceauth.ProviderInternalService,
		Roles:    []string{serviceauth.RoleDispatcher},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/00000000-0000-0000-0000-000000000001/logs/ingest", nil).WithContext(ctx)

	payload := normalizeRunLogIngestPayload(req, runLogIngestPayload{
		Lines:     []string{"hello"},
		Source:    " runner ",
		ServiceID: " runner-1 ",
		Metadata:  map[string]any{},
	})

	if payload.Source != "runner" {
		t.Fatalf("Source = %q, want runner", payload.Source)
	}
	if payload.RequestID != "req-run-log" || payload.Traceparent != "trace-run-log" {
		t.Fatalf("correlation = (%q, %q), want req-run-log/trace-run-log", payload.RequestID, payload.Traceparent)
	}
	if payload.ServiceID != "runner-1" {
		t.Fatalf("ServiceID = %q, want runner-1", payload.ServiceID)
	}
	if payload.Metadata["ingested_by"] != "dispatcher" || payload.Metadata["service_id"] != "runner-1" {
		t.Fatalf("metadata = %#v, want ingested_by and service_id", payload.Metadata)
	}
}
