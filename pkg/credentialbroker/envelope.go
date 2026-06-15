package credentialbroker

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const keyPurpose = "nopsai-credential-broker-v1"

func Seal(signingKey, serviceID string, plaintext []byte) (string, error) {
	key, aad, err := derive(signingKey, serviceID)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, aad)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func Open(signingKey, serviceID, encoded string) ([]byte, error) {
	key, aad, err := derive(signingKey, serviceID)
	if err != nil {
		return nil, err
	}
	sealed, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("credential broker envelope is too short")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, aad)
}

func derive(signingKey, serviceID string) ([]byte, []byte, error) {
	signingKey = strings.TrimSpace(signingKey)
	serviceID = strings.TrimSpace(serviceID)
	if signingKey == "" {
		return nil, nil, errors.New("service signing key is required")
	}
	if serviceID == "" {
		return nil, nil, errors.New("service id is required")
	}
	sum := sha256.Sum256([]byte(keyPurpose + "\x00" + signingKey))
	return sum[:], []byte(keyPurpose + "\x00" + serviceID), nil
}
