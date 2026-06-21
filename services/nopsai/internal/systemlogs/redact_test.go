package systemlogs

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRedactorMasksCommonSecretShapes(t *testing.T) {
	redactor := NewRedactor(1024)
	line := `password=hunter2 Authorization: Bearer abc.def {"api_key":"key-123"} postgres://user:pass@db:5432/app`
	got := redactor.Redact(line)
	for _, secret := range []string{"hunter2", "abc.def", "key-123", "user:pass"} {
		if strings.Contains(got, secret) {
			t.Fatalf("Redact() leaked %q in %q", secret, got)
		}
	}
	if strings.Count(got, redactionMarker) < 4 {
		t.Fatalf("Redact() = %q, want redaction markers", got)
	}
}

func TestRedactorTruncatesOnUTF8Boundary(t *testing.T) {
	got := NewRedactor(24).Redact(strings.Repeat("界", 20))
	if len(got) > 24 {
		t.Fatalf("Redact() bytes = %d, want <= 24", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("Redact() produced invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...[truncated]") {
		t.Fatalf("Redact() = %q, want truncation suffix", got)
	}
}
