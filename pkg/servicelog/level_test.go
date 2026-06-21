package servicelog

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    zerolog.Level
		wantErr bool
	}{
		{name: "omitted", want: zerolog.InfoLevel},
		{name: "whitespace", raw: " \t\n", want: zerolog.InfoLevel},
		{name: "info", raw: "info", want: zerolog.InfoLevel},
		{name: "case insensitive", raw: " WARN ", want: zerolog.WarnLevel},
		{name: "explicitly disabled", raw: "disabled", want: zerolog.Disabled},
		{name: "invalid", raw: "verbose", want: zerolog.NoLevel, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseLevel(test.raw)
			if (err != nil) != test.wantErr {
				t.Fatalf("ParseLevel(%q) error = %v, wantErr %v", test.raw, err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("ParseLevel(%q) = %s, want %s", test.raw, got, test.want)
			}
		})
	}
}
