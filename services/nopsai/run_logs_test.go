package nopsai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRunLogFieldsForLineDerivesStructuredMetadata(t *testing.T) {
	fields := runLogFieldsForLine(runLogIngestPayload{}, `prefix {"level":"warning","step_name":"build","task":"compile","message":"done"}`)

	if fields.Level != "warn" || fields.StepName != "build" || fields.TaskName != "compile" {
		t.Fatalf("fields = %#v, want warn/build/compile", fields)
	}
}

func TestRunLogFieldsForLinePrefersActionOutputLevelAndDoesNotTreatStderrAsSeverity(t *testing.T) {
	fields := runLogFieldsForLine(runLogIngestPayload{Stream: "stdout"}, `{"level":"info","output_level":"debug","step":"test","task_name":"unit","message":"trace line"}`)
	if fields.Level != "debug" || fields.StepName != "test" || fields.TaskName != "unit" {
		t.Fatalf("fields = %#v, want debug/test/unit", fields)
	}

	stderrFields := runLogFieldsForLine(runLogIngestPayload{Stream: "stderr"}, "plain build progress")
	if stderrFields.Level != "info" {
		t.Fatalf("stderr level = %q, want info", stderrFields.Level)
	}

	stderrErrorFields := runLogFieldsForLine(runLogIngestPayload{Stream: "stderr"}, `{"level":"error","message":"failed"}`)
	if stderrErrorFields.Level != "error" {
		t.Fatalf("structured stderr level = %q, want error", stderrErrorFields.Level)
	}

	plainTextErrorFields := runLogFieldsForLine(runLogIngestPayload{Stream: "stderr"}, "ERROR request failed")
	if plainTextErrorFields.Level != "error" {
		t.Fatalf("plain text stderr level = %q, want error", plainTextErrorFields.Level)
	}
}

func TestRunLogFieldsForLineInfersPlainTextSeverity(t *testing.T) {
	fields := runLogFieldsForLine(runLogIngestPayload{}, "WARNING request latency exceeded threshold")
	if fields.Level != "warn" {
		t.Fatalf("level = %q, want warn", fields.Level)
	}
}

func TestFilterRunLogIngestLinesDropsSuccessfulAgentGRPCClientNoise(t *testing.T) {
	payload := normalizeRunLogIngestPayload(nil, runLogIngestPayload{
		Source: serviceauth.RoleAgent,
		Lines: []string{
			`{"level":"info","grpc_code":"OK","message":"grpc_client_request"}`,
			"9 amAGENT grpc_client_request",
			`{"level":"warn","grpc_code":"Unavailable","message":"grpc_client_request"}`,
			"build output",
		},
	})

	got := filterRunLogIngestLines(payload)
	if len(got) != 2 || got[0] != `{"level":"warn","grpc_code":"Unavailable","message":"grpc_client_request"}` || got[1] != "build output" {
		t.Fatalf("filtered lines = %#v, want warning and build output", got)
	}
}

func TestFilterRunLogIngestLinesKeepsNonAgentGRPCTelemetry(t *testing.T) {
	payload := normalizeRunLogIngestPayload(nil, runLogIngestPayload{
		Source: serviceauth.RoleRunner,
		Lines:  []string{`{"level":"info","grpc_code":"OK","message":"grpc_client_request"}`},
	})

	got := filterRunLogIngestLines(payload)
	if len(got) != 1 {
		t.Fatalf("filtered lines = %#v, want runner telemetry retained", got)
	}
}

func TestRedactRunLogLineMasksCredentialsWithoutHidingOperationalEvidence(t *testing.T) {
	line := `{"message":"{\"environment\":\"production\",\"image_reference\":\"ghcr.io/acme/api:2026.08.03-rc1\",\"api_token\":\"secret-token\"}","authorization":"Bearer abc.def","database":"postgres://user:pass@db/app"}`

	got := redactRunLogLine(line)
	for _, want := range []string{"production", "ghcr.io/acme/api:2026.08.03-rc1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("redacted log lost operational evidence %q: %s", want, got)
		}
	}
	for _, forbidden := range []string{"secret-token", "abc.def", "user:pass"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redacted log leaked %q: %s", forbidden, got)
		}
	}
	if strings.Count(got, runLogRedactionMarker) < 3 {
		t.Fatalf("redacted log = %s, want credential redaction markers", got)
	}
}
