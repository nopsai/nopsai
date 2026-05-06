package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsTrailingValues(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"owner":"repo"} {"extra":true}`))

	var payload struct {
		Owner string `json:"owner"`
	}
	if err := DecodeJSON(req, &payload); err == nil {
		t.Fatal("expected trailing JSON to be rejected")
	}
}

func TestDecodeOptionalJSONAllowsEmptyBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(""))

	var payload struct {
		Name string `json:"name"`
	}
	if err := DecodeOptionalJSON(req, &payload); err != nil {
		t.Fatalf("DecodeOptionalJSON() error = %v", err)
	}
}

func TestValidateRequiredFormatsMissingFields(t *testing.T) {
	err := ValidateRequired(
		RequiredString("owner", ""),
		RequiredString("repo", "demo"),
		RequiredString("ref", ""),
	)
	if err == nil {
		t.Fatal("expected missing field validation error")
	}
	if got, want := err.Error(), "owner and ref are required"; got != want {
		t.Fatalf("ValidateRequired() = %q, want %q", got, want)
	}
}
