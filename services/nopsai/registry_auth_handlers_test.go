package nopsai

import (
	"testing"

	"nopsai/pkg/serviceauth"
	"nopsai/services/nopsai/pkg/auth"
)

func TestRegistryAuthCallerBoundToRunner(t *testing.T) {
	tests := []struct {
		name     string
		claims   *auth.Claims
		runnerID string
		want     bool
	}{
		{
			name:     "specific runner subject matches",
			claims:   &auth.Claims{Sub: "runner-prod-1", Provider: serviceauth.ProviderInternalService, Roles: []string{serviceauth.RoleRunner}},
			runnerID: "runner-prod-1",
			want:     true,
		},
		{
			name:     "specific runner subject cannot claim another runner",
			claims:   &auth.Claims{Sub: "runner-prod-1", Provider: serviceauth.ProviderInternalService, Roles: []string{serviceauth.RoleRunner}},
			runnerID: "runner-prod-2",
			want:     false,
		},
		{
			name:     "prefixed runner subject matches suffix",
			claims:   &auth.Claims{Sub: "runner:runner-prod-1", Provider: serviceauth.ProviderInternalService, Roles: []string{serviceauth.RoleRunner}},
			runnerID: "runner-prod-1",
			want:     true,
		},
		{
			name:     "legacy generic runner subject stays compatible",
			claims:   &auth.Claims{Sub: "runner", Provider: serviceauth.ProviderInternalService, Roles: []string{serviceauth.RoleRunner}},
			runnerID: "runner-prod-1",
			want:     true,
		},
		{
			name:     "legacy generic agent subject stays compatible",
			claims:   &auth.Claims{Sub: "agent", Provider: serviceauth.ProviderInternalService, Roles: []string{serviceauth.RoleAgent}},
			runnerID: "runner-prod-1",
			want:     true,
		},
		{
			name:     "generic runner subject without runner role is rejected",
			claims:   &auth.Claims{Sub: "runner", Provider: serviceauth.ProviderInternalService, Roles: []string{serviceauth.RoleAgent}},
			runnerID: "runner-prod-1",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registryAuthCallerBoundToRunner(tt.claims, tt.runnerID); got != tt.want {
				t.Fatalf("registryAuthCallerBoundToRunner() = %v, want %v", got, tt.want)
			}
		})
	}
}
