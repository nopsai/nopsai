package servicelog

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"nopsai/pkg/correlation"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestGRPCUnaryServerInterceptorPropagatesCorrelation(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() { log.Logger = previous })

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		correlation.RequestIDMetadata, "req-grpc",
		correlation.TraceparentMetadata, "trace-grpc",
	))
	interceptor := GRPCUnaryServerInterceptor()
	called := false

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/proto.DispatcherService/GetStatus"}, func(ctx context.Context, req any) (any, error) {
		called = true
		if got := correlation.RequestIDFromContext(ctx); got != "req-grpc" {
			t.Fatalf("request id in context = %q, want req-grpc", got)
		}
		if got := correlation.TraceparentFromContext(ctx); got != "trace-grpc" {
			t.Fatalf("traceparent in context = %q, want trace-grpc", got)
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
	if !strings.Contains(logs.String(), "grpc_request") || !strings.Contains(logs.String(), "req-grpc") {
		t.Fatalf("logs = %q, want grpc_request with request id", logs.String())
	}
}
