package dockerimage

import (
	"context"
	"errors"
	"testing"
)

func TestPullOptionsFailsClosedOnResolverError(t *testing.T) {
	wantErr := errors.New("resolver down")
	_, _, err := PullOptions(context.Background(), "registry.local/app:latest", failingRegistryAuthResolver{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("PullOptions() error = %v, want wrapped resolver error", err)
	}
}

func TestPullOptionsUsesResolvedAuth(t *testing.T) {
	options, authenticated, err := PullOptions(context.Background(), "registry.local/app:latest", staticRegistryAuthResolver{auth: " encoded-auth "})
	if err != nil {
		t.Fatalf("PullOptions() error = %v", err)
	}
	if !authenticated || options.RegistryAuth != "encoded-auth" {
		t.Fatalf("PullOptions() = auth %q authenticated %v, want trimmed auth", options.RegistryAuth, authenticated)
	}
}

type failingRegistryAuthResolver struct {
	err error
}

func (r failingRegistryAuthResolver) Resolve(context.Context, string) (string, error) {
	return "", r.err
}

type staticRegistryAuthResolver struct {
	auth string
}

func (r staticRegistryAuthResolver) Resolve(context.Context, string) (string, error) {
	return r.auth, nil
}
