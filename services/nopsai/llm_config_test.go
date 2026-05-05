package main

import "testing"

func TestContainerReachableLMStudioBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "localhost rewritten", raw: "http://127.0.0.1:1234", want: "http://host.docker.internal:1234"},
		{name: "localhost hostname rewritten", raw: "http://localhost:1234/v1", want: "http://host.docker.internal:1234/v1"},
		{name: "remote host preserved", raw: "http://lmstudio.internal:1234", want: "http://lmstudio.internal:1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containerReachableLMStudioBaseURL(tt.raw); got != tt.want {
				t.Fatalf("containerReachableLMStudioBaseURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
