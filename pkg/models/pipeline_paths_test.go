package models

import "testing"

func TestNormalizePipelineWorkingDirectory(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty defaults to workspace", input: "", want: "/workspace"},
		{name: "dot defaults to workspace", input: ".", want: "/workspace"},
		{name: "absolute path is cleaned", input: "/tmp/test/../work", want: "/tmp/work"},
		{name: "relative path is under workspace", input: "src/app", want: "/workspace/src/app"},
		{name: "relative dot path is under workspace", input: "./src", want: "/workspace/src"},
		{name: "relative escape is rejected", input: "../src", wantErr: true},
		{name: "root is rejected", input: "/", wantErr: true},
		{name: "colon is rejected", input: "/tmp/a:b", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePipelineWorkingDirectory(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizePipelineWorkingDirectory(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizePipelineWorkingDirectory(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizePipelineWorkingDirectory(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
