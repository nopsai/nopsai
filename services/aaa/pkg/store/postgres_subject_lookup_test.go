package store

import (
	"testing"

	"nopsai/services/aaa/pkg/model"
)

func TestBuildUserSubjectLookupPrefersSubjectOverEmail(t *testing.T) {
	query, args, err := buildUserSubjectLookup("SELECT * FROM users WHERE %s", model.Subject{
		Sub:   "subject-123",
		Email: "user@example.com",
	})
	if err != nil {
		t.Fatalf("buildUserSubjectLookup() error = %v", err)
	}
	if query != "SELECT * FROM users WHERE sub = $1" {
		t.Fatalf("query = %q, want subject lookup", query)
	}
	if len(args) != 1 || args[0] != "subject-123" {
		t.Fatalf("args = %#v, want subject arg", args)
	}
}

func TestBuildUserSubjectLookupUsesEmailOnlyWhenSubjectMissing(t *testing.T) {
	query, args, err := buildUserSubjectLookup("SELECT * FROM users WHERE %s", model.Subject{
		Email: "user@example.com",
	})
	if err != nil {
		t.Fatalf("buildUserSubjectLookup() error = %v", err)
	}
	if query != "SELECT * FROM users WHERE email = $1" {
		t.Fatalf("query = %q, want email lookup", query)
	}
	if len(args) != 1 || args[0] != "user@example.com" {
		t.Fatalf("args = %#v, want email arg", args)
	}
}
