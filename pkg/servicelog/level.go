package servicelog

import (
	"strings"

	"github.com/rs/zerolog"
)

// ParseLevel resolves an omitted service log level to the operational default.
func ParseLevel(raw string) (zerolog.Level, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return zerolog.InfoLevel, nil
	}
	return zerolog.ParseLevel(normalized)
}
