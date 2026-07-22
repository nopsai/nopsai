package correlation

import (
	"context"
	"net/http"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestFromHTTPHeadersPreservesRequestIDAndTraceparent(t *testing.T) {
	headers := http.Header{}
	headers.Set(RequestIDHeader, " req-1 ")
	headers.Set(TraceparentHeader, " trace-1 ")

	ctx, requestID, traceparent := FromHTTPHeaders(context.Background(), headers)

	if requestID != "req-1" || RequestIDFromContext(ctx) != "req-1" {
		t.Fatalf("request id = %q / %q, want req-1", requestID, RequestIDFromContext(ctx))
	}
	if traceparent != "trace-1" || TraceparentFromContext(ctx) != "trace-1" {
		t.Fatalf("traceparent = %q / %q, want trace-1", traceparent, TraceparentFromContext(ctx))
	}
}

func TestFromHTTPHeadersGeneratesRequestID(t *testing.T) {
	ctx, requestID, _ := FromHTTPHeaders(context.Background(), http.Header{})

	if requestID == "" {
		t.Fatal("request id is empty")
	}
	if RequestIDFromContext(ctx) != requestID {
		t.Fatalf("context request id = %q, want %q", RequestIDFromContext(ctx), requestID)
	}
}

func TestOutgoingGRPCContextCarriesCorrelationMetadata(t *testing.T) {
	ctx := WithTraceparent(WithRequestID(context.Background(), "req-2"), "trace-2")
	ctx = OutgoingGRPCContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("outgoing metadata missing")
	}
	if got := md.Get(RequestIDMetadata); len(got) != 1 || got[0] != "req-2" {
		t.Fatalf("request id metadata = %#v, want req-2", got)
	}
	if got := md.Get(TraceparentMetadata); len(got) != 1 || got[0] != "trace-2" {
		t.Fatalf("traceparent metadata = %#v, want trace-2", got)
	}
}

func TestFromIncomingGRPCUsesMetadata(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		RequestIDMetadata, "req-3",
		TraceparentMetadata, "trace-3",
	))

	ctx, requestID, traceparent := FromIncomingGRPC(ctx)

	if requestID != "req-3" || RequestIDFromContext(ctx) != "req-3" {
		t.Fatalf("request id = %q / %q, want req-3", requestID, RequestIDFromContext(ctx))
	}
	if traceparent != "trace-3" || TraceparentFromContext(ctx) != "trace-3" {
		t.Fatalf("traceparent = %q / %q, want trace-3", traceparent, TraceparentFromContext(ctx))
	}
}
