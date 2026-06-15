package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const EncryptionFormatVersion = 1

type Envelope struct {
	Ciphertext              []byte
	WrappedDataKey          []byte
	EncryptionKeyID         string
	EncryptionFormatVersion int
}

type EnvelopeCodec struct {
	key   []byte
	keyID string
}

func NewEnvelopeCodec(masterKey string) (*EnvelopeCodec, error) {
	masterKey = strings.TrimSpace(masterKey)
	if masterKey == "" {
		return nil, errors.New("credential master key is required")
	}
	key := sha256.Sum256([]byte(masterKey))
	keyIDHash := sha256.Sum256(key[:])
	return &EnvelopeCodec{
		key:   append([]byte(nil), key[:]...),
		keyID: hex.EncodeToString(keyIDHash[:8]),
	}, nil
}

func (c *EnvelopeCodec) KeyID() string {
	if c == nil {
		return ""
	}
	return c.keyID
}

func (c *EnvelopeCodec) Encrypt(ref Reference, kind string, version int, plaintext []byte) (Envelope, error) {
	if c == nil || len(c.key) != 32 {
		return Envelope{}, errors.New("credential envelope codec is not configured")
	}
	if ref.String() == "" {
		return Envelope{}, ErrInvalidReference
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return Envelope{}, errors.New("credential kind is required")
	}
	if version <= 0 {
		return Envelope{}, errors.New("credential version must be positive")
	}
	if len(plaintext) == 0 {
		return Envelope{}, errors.New("credential value is required")
	}

	dataKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dataKey); err != nil {
		return Envelope{}, fmt.Errorf("generate credential data key: %w", err)
	}
	defer clear(dataKey)

	valueAAD := envelopeAAD("value", ref, kind, version)
	ciphertext, err := seal(dataKey, plaintext, valueAAD)
	if err != nil {
		return Envelope{}, fmt.Errorf("encrypt credential value: %w", err)
	}
	keyAAD := envelopeAAD("data-key", ref, kind, version)
	wrappedKey, err := seal(c.key, dataKey, keyAAD)
	if err != nil {
		return Envelope{}, fmt.Errorf("wrap credential data key: %w", err)
	}
	return Envelope{
		Ciphertext:              ciphertext,
		WrappedDataKey:          wrappedKey,
		EncryptionKeyID:         c.keyID,
		EncryptionFormatVersion: EncryptionFormatVersion,
	}, nil
}

func (c *EnvelopeCodec) Decrypt(ref Reference, kind string, version int, envelope Envelope) ([]byte, error) {
	if c == nil || len(c.key) != 32 {
		return nil, errors.New("credential envelope codec is not configured")
	}
	if envelope.EncryptionFormatVersion != EncryptionFormatVersion {
		return nil, fmt.Errorf("unsupported credential encryption format version %d", envelope.EncryptionFormatVersion)
	}
	if envelope.EncryptionKeyID != c.keyID {
		return nil, fmt.Errorf("credential encryption key %q is not available", envelope.EncryptionKeyID)
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	keyAAD := envelopeAAD("data-key", ref, kind, version)
	dataKey, err := open(c.key, envelope.WrappedDataKey, keyAAD)
	if err != nil {
		return nil, fmt.Errorf("unwrap credential data key: %w", err)
	}
	defer clear(dataKey)
	valueAAD := envelopeAAD("value", ref, kind, version)
	plaintext, err := open(dataKey, envelope.Ciphertext, valueAAD)
	if err != nil {
		return nil, fmt.Errorf("decrypt credential value: %w", err)
	}
	return plaintext, nil
}

func envelopeAAD(purpose string, ref Reference, kind string, version int) []byte {
	return []byte(strings.Join([]string{
		"nopsai-credential",
		"v" + strconv.Itoa(EncryptionFormatVersion),
		purpose,
		ref.String(),
		strings.ToLower(strings.TrimSpace(kind)),
		strconv.Itoa(version),
	}, "|"))
}

func seal(key, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, aad), nil
}

func open(key, sealed, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("ciphertext is too short")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, aad)
}
