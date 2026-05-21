package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleEncryptSecretForGitOpsRequiresOnlyValue(t *testing.T) {
	app := &App{encKey: []byte("12345678901234567890123456789012")}
	req := httptest.NewRequest(http.MethodPost, "/v1/secrets/encrypt", strings.NewReader(`{"value":"super-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.handleEncryptSecretForGitOps(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}

	var payload struct {
		EncryptedValue string `json:"encrypted_value"`
		Algorithm      string `json:"algorithm"`
		Encoding       string `json:"encoding"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.EncryptedValue == "" {
		t.Fatal("encrypted_value is empty")
	}
	if payload.Algorithm != "aes-256-gcm" {
		t.Fatalf("algorithm = %q, want aes-256-gcm", payload.Algorithm)
	}
	if payload.Encoding != "hex" {
		t.Fatalf("encoding = %q, want hex", payload.Encoding)
	}

	decrypted, err := app.decrypt(payload.EncryptedValue)
	if err != nil {
		t.Fatalf("decrypt encrypted value: %v", err)
	}
	if decrypted != "super-secret" {
		t.Fatalf("decrypted value = %q, want super-secret", decrypted)
	}
}
