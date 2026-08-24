// Package license implements NopsAI's offline licence keys.
//
// Verification is a local Ed25519 signature check. NopsAI never calls home to
// validate a key: the platform is designed to run without a public URL, and an
// air-gapped installation must be able to prove its entitlement with nothing
// but the key and the public key compiled into the binary.
//
// The key is data, not a secret. It states what an installation is entitled to
// run; it is not a credential, and leaking one grants nobody any rights that
// the licence agreement did not already grant.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Tier names the commercial shape of an installation. Non-commercial is the
// floor: it is what an installation gets when it presents no key, and what it
// falls back to when a key cannot be trusted.
//
// The floor is not a restriction. NopsAI is free for any non-commercial purpose
// under the licence shipped with it, so an installation with no key is a
// complete, uncapped product. A key exists to record a commercial entitlement
// and whatever scope was agreed for it.
type Tier string

const (
	TierNonCommercial Tier = "noncommercial"
	TierCommercial    Tier = "commercial"
)

// Claims is the signed payload of a licence key.
type Claims struct {
	Licensee  string `json:"licensee"`
	LicenseID string `json:"license_id"`
	Tier      Tier   `json:"tier"`

	IssuedAt  time.Time `json:"issued_at"`
	NotBefore time.Time `json:"not_before,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`

	// A zero limit means unlimited. Absent limits must never be read as zero
	// allowed, which would make a valid key more restrictive than no key.
	MaxUsers          int `json:"max_users,omitempty"`
	MaxTeams          int `json:"max_teams,omitempty"`
	MaxConcurrentRuns int `json:"max_concurrent_runs,omitempty"`

	Features []string `json:"features,omitempty"`
}

// NonCommercialClaims are what an installation with no commercial key runs
// under. Every limit is zero, which means unlimited: the non-commercial licence
// grants the whole product, so there is nothing here to cap.
//
// Commercial use is a licence question, not a runtime one. The software has no
// way to observe whether a purpose is commercial and never phones home to ask,
// so the boundary is self-certified against the licence rather than enforced by
// a ceiling.
func NonCommercialClaims() Claims {
	return Claims{
		Licensee: "Non-commercial use",
		Tier:     TierNonCommercial,
	}
}

var (
	ErrEmptyKey        = errors.New("licence key is empty")
	ErrMalformedKey    = errors.New("licence key is malformed")
	ErrBadSignature    = errors.New("licence key signature does not verify")
	ErrNotYetValid     = errors.New("licence key is not valid yet")
	ErrExpired         = errors.New("licence key has expired")
	ErrNoVerifyingKey  = errors.New("no licence verification key is configured")
	ErrKeyMissingScope = errors.New("licence key does not name a licensee")
)

// Key is the wire format: base64url(payload) + "." + base64url(signature).
// A single line, so it survives a YAML file, an environment variable, and a
// copy-paste out of an email intact.
type Key string

// Sign produces a key from claims. It lives here rather than in the issuing
// script so that the format has exactly one implementation and the round trip
// is covered by the same tests as verification.
func Sign(claims Claims, privateKey ed25519.PrivateKey) (Key, error) {
	if strings.TrimSpace(claims.Licensee) == "" {
		return "", ErrKeyMissingScope
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("licence signing key must be %d bytes", ed25519.PrivateKeySize)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signature := ed25519.Sign(privateKey, payload)
	return Key(encode(payload) + "." + encode(signature)), nil
}

// Verify checks the signature and the validity window. It returns the claims
// only when the key can be trusted; every failure path returns an error, so a
// caller cannot accidentally treat an unverified key as entitlement.
func Verify(key Key, publicKey ed25519.PublicKey, now time.Time) (Claims, error) {
	raw := strings.TrimSpace(string(key))
	if raw == "" {
		return Claims{}, ErrEmptyKey
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return Claims{}, ErrNoVerifyingKey
	}

	payloadPart, signaturePart, found := strings.Cut(raw, ".")
	if !found {
		return Claims{}, ErrMalformedKey
	}
	payload, err := decode(payloadPart)
	if err != nil {
		return Claims{}, ErrMalformedKey
	}
	signature, err := decode(signaturePart)
	if err != nil {
		return Claims{}, ErrMalformedKey
	}
	if !ed25519.Verify(publicKey, payload, signature) {
		return Claims{}, ErrBadSignature
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrMalformedKey
	}
	if strings.TrimSpace(claims.Licensee) == "" {
		return Claims{}, ErrKeyMissingScope
	}
	if !claims.NotBefore.IsZero() && now.Before(claims.NotBefore) {
		return Claims{}, ErrNotYetValid
	}
	if !claims.ExpiresAt.IsZero() && now.After(claims.ExpiresAt) {
		return Claims{}, ErrExpired
	}
	return claims, nil
}

// HasFeature reports whether the claims name a feature. Comparison is
// case-insensitive so a key issued with different casing still works.
func (c Claims) HasFeature(feature string) bool {
	target := strings.ToLower(strings.TrimSpace(feature))
	if target == "" {
		return false
	}
	for _, candidate := range c.Features {
		if strings.ToLower(strings.TrimSpace(candidate)) == target {
			return true
		}
	}
	return false
}

// Unlimited reports whether a limit means "no ceiling". Zero and negative both
// mean unlimited, so a key that omits a limit never accidentally forbids
// everything.
func Unlimited(limit int) bool { return limit <= 0 }

// Allows reports whether one more of something is permitted at the given
// current count.
func Allows(limit, current int) bool {
	if Unlimited(limit) {
		return true
	}
	return current < limit
}

func encode(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }

func decode(value string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(value) }
