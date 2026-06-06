package httpapi

import (
	"net/http"
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
