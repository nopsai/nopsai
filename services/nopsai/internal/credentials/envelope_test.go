package credentials

import (
	"bytes"
	"testing"
)

func TestEnvelopeCodecRoundTrip(t *testing.T) {
	codec, err := NewEnvelopeCodec("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("NewEnvelopeCodec() error = %v", err)
	}
	ref, _ := ParseReference("credential://system/llm/openai-primary")
	envelope, err := codec.Encrypt(ref, "api_key", 1, []byte("secret-value"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	plaintext, err := codec.Decrypt(ref, "api_key", 1, envelope)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got, want := string(plaintext), "secret-value"; got != want {
		t.Fatalf("Decrypt() = %q, want %q", got, want)
	}
}

func TestEnvelopeCodecRejectsTamperingAndWrongContext(t *testing.T) {
	codec, _ := NewEnvelopeCodec("01234567890123456789012345678901")
	ref, _ := ParseReference("credential://system/llm/openai-primary")
	envelope, _ := codec.Encrypt(ref, "api_key", 1, []byte("secret-value"))

	tampered := envelope
	tampered.Ciphertext = append([]byte(nil), envelope.Ciphertext...)
	tampered.Ciphertext[len(tampered.Ciphertext)-1] ^= 0xff
	if _, err := codec.Decrypt(ref, "api_key", 1, tampered); err == nil {
		t.Fatal("Decrypt() unexpectedly accepted tampered ciphertext")
	}

	otherRef, _ := ParseReference("credential://system/llm/other")
	if _, err := codec.Decrypt(otherRef, "api_key", 1, envelope); err == nil {
		t.Fatal("Decrypt() unexpectedly accepted the wrong reference")
	}
	if _, err := codec.Decrypt(ref, "password", 1, envelope); err == nil {
		t.Fatal("Decrypt() unexpectedly accepted the wrong kind")
	}
	if _, err := codec.Decrypt(ref, "api_key", 2, envelope); err == nil {
		t.Fatal("Decrypt() unexpectedly accepted the wrong version")
	}
}

func TestEnvelopeCodecUsesUniqueDataAndNonces(t *testing.T) {
	codec, _ := NewEnvelopeCodec("01234567890123456789012345678901")
	ref, _ := ParseReference("credential://system/llm/openai-primary")
	first, _ := codec.Encrypt(ref, "api_key", 1, []byte("secret-value"))
	second, _ := codec.Encrypt(ref, "api_key", 1, []byte("secret-value"))
	if bytes.Equal(first.Ciphertext, second.Ciphertext) {
		t.Fatal("Encrypt() reused credential ciphertext")
	}
	if bytes.Equal(first.WrappedDataKey, second.WrappedDataKey) {
		t.Fatal("Encrypt() reused wrapped data key ciphertext")
	}
}

func TestEnvelopeCodecRejectsWrongMasterKeyAndFormat(t *testing.T) {
	codec, _ := NewEnvelopeCodec("01234567890123456789012345678901")
	other, _ := NewEnvelopeCodec("abcdefghijklmnopqrstuvwxyzABCDEF")
	ref, _ := ParseReference("credential://system/llm/openai-primary")
	envelope, _ := codec.Encrypt(ref, "api_key", 1, []byte("secret-value"))
	if _, err := other.Decrypt(ref, "api_key", 1, envelope); err == nil {
		t.Fatal("Decrypt() unexpectedly accepted the wrong master key")
	}
	envelope.EncryptionFormatVersion = 99
	if _, err := codec.Decrypt(ref, "api_key", 1, envelope); err == nil {
		t.Fatal("Decrypt() unexpectedly accepted an unknown format")
	}
}
