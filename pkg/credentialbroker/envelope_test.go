package credentialbroker

import (
	"bytes"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	plaintext := []byte(`{"private_key":"secret"}`)
	sealed, err := Seal("service-signing-key", "git-bot-1", plaintext)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	opened, err := Open("service-signing-key", "git-bot-1", sealed)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Fatalf("Open() = %q, want %q", opened, plaintext)
	}
}

func TestOpenRejectsWrongServiceOrKey(t *testing.T) {
	sealed, err := Seal("service-signing-key", "git-bot-1", []byte("secret"))
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	if _, err := Open("service-signing-key", "git-bot-2", sealed); err == nil {
		t.Fatal("Open() accepted the wrong service id")
	}
	if _, err := Open("other-key", "git-bot-1", sealed); err == nil {
		t.Fatal("Open() accepted the wrong signing key")
	}
}
