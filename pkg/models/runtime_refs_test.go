package models

import "testing"

func TestParseScopedRuntimeRefBareUsesCurrentScope(t *testing.T) {
	ref, err := ParseScopedRuntimeRef(" TEST_ENV ", "dev")
	if err != nil {
		t.Fatalf("ParseScopedRuntimeRef() error = %v", err)
	}
	if ref.Name != "TEST_ENV" || ref.Scope != "dev" || ref.ExplicitScope {
		t.Fatalf("ref = %#v, want name TEST_ENV, scope dev, implicit scope", ref)
	}
	if got := ref.Key(); got != "TEST_ENV" {
		t.Fatalf("Key() = %q, want TEST_ENV", got)
	}
}

func TestParseScopedRuntimeRefExplicitScope(t *testing.T) {
	ref, err := ParseScopedRuntimeRef("team-1/dev:TEST_ENV", "prod")
	if err != nil {
		t.Fatalf("ParseScopedRuntimeRef() error = %v", err)
	}
	if ref.Name != "TEST_ENV" || ref.Scope != "team-1/dev" || !ref.ExplicitScope {
		t.Fatalf("ref = %#v, want name TEST_ENV, explicit scope team-1/dev", ref)
	}
	if got := ref.Key(); got != "team-1/dev:TEST_ENV" {
		t.Fatalf("Key() = %q, want team-1/dev:TEST_ENV", got)
	}
}

func TestParseScopedRuntimeRefExplicitDefaultScope(t *testing.T) {
	ref, err := ParseScopedRuntimeRef("default:TEST_ENV", "prod")
	if err != nil {
		t.Fatalf("ParseScopedRuntimeRef() error = %v", err)
	}
	if ref.Scope != "" || ref.DisplayScope() != "default" {
		t.Fatalf("default scope ref = %#v", ref)
	}
	if got := ref.Key(); got != "default:TEST_ENV" {
		t.Fatalf("Key() = %q, want default:TEST_ENV", got)
	}
}

func TestParseScopedRuntimeRefRejectsInvalidScopedRefs(t *testing.T) {
	for _, raw := range []string{"", ":TEST_ENV", "dev:", "dev:TEST:ENV"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseScopedRuntimeRef(raw, "prod"); err == nil {
				t.Fatal("ParseScopedRuntimeRef() error = nil, want error")
			}
		})
	}
}

func TestIsValidRuntimeReferenceName(t *testing.T) {
	for _, name := range []string{"API_VERSION", "release.channel", "team-1", "1_VALUE"} {
		if !IsValidRuntimeReferenceName(name) {
			t.Fatalf("IsValidRuntimeReferenceName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", "BAD/NAME", "SCOPE:NAME", "NAME WITH SPACE"} {
		if IsValidRuntimeReferenceName(name) {
			t.Fatalf("IsValidRuntimeReferenceName(%q) = true, want false", name)
		}
	}
}
