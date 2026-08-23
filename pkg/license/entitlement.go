package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"time"
)

// Entitlement is the resolved answer to "what may this installation run right
// now". It is always populated: there is no nil entitlement and no state in
// which enforcement has nothing to consult.
type Entitlement struct {
	Claims Claims

	// Licensed is true only when a key verified. It is false for an
	// unlicensed installation and false when a key was present but could not
	// be trusted.
	Licensed bool

	// Reason explains a non-licensed state in operator language. It is shown
	// in the UI banner and in `nopsai license status`.
	Reason string
}

// Resolve turns a configured key into an entitlement.
//
// It fails closed in the sense that matters: a key that cannot be verified
// never grants more than no key at all. It does not fail closed by refusing to
// run, because an installation that cannot read its key must still be able to
// start, so an operator can log in and fix it. Refusing to boot is the job of
// the production startup gate, where an administrator has explicitly declared
// the installation to be production.
func Resolve(rawKey string, publicKey ed25519.PublicKey, now time.Time) Entitlement {
	evaluation := func(reason string) Entitlement {
		return Entitlement{Claims: EvaluationClaims(), Licensed: false, Reason: reason}
	}

	if strings.TrimSpace(rawKey) == "" {
		return evaluation("No licence key is configured. Running under evaluation limits.")
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return evaluation("This build has no licence verification key, so the configured key cannot be checked. Running under evaluation limits.")
	}

	claims, err := Verify(Key(rawKey), publicKey, now)
	if err != nil {
		switch {
		case errorIs(err, ErrExpired):
			return evaluation("The licence key expired. Running under evaluation limits until it is renewed.")
		case errorIs(err, ErrNotYetValid):
			return evaluation("The licence key is not valid yet. Running under evaluation limits.")
		case errorIs(err, ErrBadSignature):
			return evaluation("The licence key signature does not verify. Running under evaluation limits.")
		default:
			return evaluation("The licence key could not be read. Running under evaluation limits.")
		}
	}
	return Entitlement{Claims: claims, Licensed: true}
}

// AllowsUsers, AllowsTeams, and AllowsConcurrentRuns answer the three limits
// enforcement consults. Each takes the current count and reports whether one
// more is permitted.
func (e Entitlement) AllowsUsers(current int) bool { return Allows(e.Claims.MaxUsers, current) }

func (e Entitlement) AllowsTeams(current int) bool { return Allows(e.Claims.MaxTeams, current) }

func (e Entitlement) AllowsConcurrentRuns(current int) bool {
	return Allows(e.Claims.MaxConcurrentRuns, current)
}

// Expired reports whether a licensed entitlement has passed its expiry. A key
// that expires while the process is running stops being valid without a
// restart.
func (e Entitlement) Expired(now time.Time) bool {
	if !e.Licensed || e.Claims.ExpiresAt.IsZero() {
		return false
	}
	return now.After(e.Claims.ExpiresAt)
}

// ParsePublicKey reads a base64 Ed25519 public key, as compiled into the binary
// or supplied by configuration.
func ParsePublicKey(encoded string) (ed25519.PublicKey, bool) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, false
	}
	for _, decoder := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := decoder.DecodeString(trimmed); err == nil && len(decoded) == ed25519.PublicKeySize {
			return ed25519.PublicKey(decoded), true
		}
	}
	return nil, false
}

func errorIs(err, target error) bool { return err == target }
