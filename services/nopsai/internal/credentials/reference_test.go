package credentials

import "testing"

func TestParseReference(t *testing.T) {
	ref, err := ParseReference("credential://system/llm/openai-primary")
	if err != nil {
		t.Fatalf("ParseReference() error = %v", err)
	}
	if got, want := ref.Namespace, "system"; got != want {
		t.Fatalf("namespace = %q, want %q", got, want)
	}
	if got, want := ref.Name, "llm/openai-primary"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	if got, want := ref.String(), "credential://system/llm/openai-primary"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParseReferenceRejectsUnsupportedShapes(t *testing.T) {
	for _, raw := range []string{
		"",
		"OPENAI_API_KEY",
		"credential:///llm/openai",
		"credential://system",
		"credential://system/llm/openai?version=1",
		"credential://system/llm/openai#value",
		"vault://system/llm/openai",
	} {
		if _, err := ParseReference(raw); err == nil {
			t.Errorf("ParseReference(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestPurposeNormalizeRequiresConsumerAndOperation(t *testing.T) {
	purpose, err := (Purpose{ConsumerService: " NOPSAI ", Operation: " LLM.RUNTIME "}).Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if purpose.ConsumerService != "nopsai" || purpose.Operation != "llm.runtime" {
		t.Fatalf("Normalize() = %#v", purpose)
	}
	if _, err := (Purpose{}).Normalize(); err == nil {
		t.Fatal("Normalize() unexpectedly accepted an empty purpose")
	}
}
