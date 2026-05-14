package servicetls

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestMutualTLSCredentialsAllowSharedSecretClient(t *testing.T) {
	addr, stop := startHealthServer(t, Config{Mode: ModeMTLS, Secret: "shared-secret"})
	defer stop()

	clientCreds, err := ClientCredentials(Config{
		Mode:       ModeMTLS,
		Secret:     "shared-secret",
		Role:       "runner",
		ServiceID:  "runner-1",
		ServerName: "dispatcher",
	})
	if err != nil {
		t.Fatalf("ClientCredentials() error = %v", err)
	}

	conn := dialHealthServer(t, addr, clientCreds)
	defer conn.Close()

	if _, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatalf("health check over mTLS failed: %v", err)
	}
}

func TestMutualTLSCredentialsRejectDifferentSecretClient(t *testing.T) {
	addr, stop := startHealthServer(t, Config{Mode: ModeMTLS, Secret: "server-secret"})
	defer stop()

	clientCreds, err := ClientCredentials(Config{
		Mode:       ModeMTLS,
		Secret:     "client-secret",
		Role:       "runner",
		ServiceID:  "runner-1",
		ServerName: "dispatcher",
	})
	if err != nil {
		t.Fatalf("ClientCredentials() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(clientCreds), grpc.WithBlock())
	if err == nil {
		_ = conn.Close()
		t.Fatal("DialContext() succeeded with a different TLS secret")
	}
}

func TestTLSModeDoesNotRequireClientCertificate(t *testing.T) {
	addr, stop := startHealthServer(t, Config{Mode: ModeTLS, Secret: "shared-secret"})
	defer stop()

	clientCreds, err := ClientCredentials(Config{
		Mode:       ModeTLS,
		Secret:     "shared-secret",
		ServerName: "dispatcher",
	})
	if err != nil {
		t.Fatalf("ClientCredentials() error = %v", err)
	}

	conn := dialHealthServer(t, addr, clientCreds)
	defer conn.Close()

	if _, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatalf("health check over TLS failed: %v", err)
	}
}

func TestNormalizeMode(t *testing.T) {
	tests := map[string]string{
		"":           ModeMTLS,
		"auto":       ModeMTLS,
		"mutual-tls": ModeMTLS,
		"server-tls": ModeTLS,
		"plaintext":  ModeDisabled,
	}

	for raw, want := range tests {
		if got := NormalizeMode(raw); got != want {
			t.Fatalf("NormalizeMode(%q) = %q, want %q", raw, got, want)
		}
	}
}

func startHealthServer(t *testing.T, cfg Config) (string, func()) {
	t.Helper()
	serverCreds, err := ServerCredentials(cfg)
	if err != nil {
		t.Fatalf("ServerCredentials() error = %v", err)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	opts := []grpc.ServerOption{}
	if serverCreds != nil {
		opts = append(opts, grpc.Creds(serverCreds))
	}
	srv := grpc.NewServer(opts...)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(srv, healthServer)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(lis)
	}()

	stop := func() {
		srv.Stop()
		<-done
	}
	return lis.Addr().String(), stop
}

func dialHealthServer(t *testing.T, addr string, creds credentials.TransportCredentials) *grpc.ClientConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		t.Fatalf("DialContext() error = %v", err)
	}
	return conn
}
