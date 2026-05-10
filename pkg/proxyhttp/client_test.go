package proxyhttp

import "testing"

func TestShouldBypassProxy(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "", want: false},
		{host: "github.com", want: false},
		{host: "api.github.com", want: false},
		{host: "localhost", want: true},
		{host: "127.0.0.1", want: true},
		{host: "192.168.1.143", want: true},
		{host: "10.0.0.5", want: true},
		{host: "aaa", want: true},
		{host: "nopsai-git-bot", want: true},
		{host: "service.internal", want: false},
	}

	for _, tt := range tests {
		if got := shouldBypassProxy(tt.host); got != tt.want {
			t.Fatalf("shouldBypassProxy(%q) = %t, want %t", tt.host, got, tt.want)
		}
	}
}
