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

	// Licensed is true only when a commercial key verified. It is false for an
	// installation running under the non-commercial licence, and false when a
	// key was present but could not be trusted.
	Licensed bool

	// Reason explains a non-commercial state in operator language. It is shown
	// in the UI banner and in `nopsai license status`. It describes a licence
	// position, not a fault: running non-commercially is a supported state.
	Reason string
}

// Resolve turns a configured key into an entitlement.
//
// It fails closed in the sense that matters: a key that cannot be verified
// never grants more than no key at all. It does not fail closed by refusing to
// run. NopsAI is free for non-commercial use, so an installation with no usable
// key is entitled to the product; a commercial key adds a named licensee and
// whatever scope was agreed, and nothing else.
func Resolve(rawKey string, publicKey ed25519.PublicKey, now time.Time) Entitlement {
	nonCommercial := func(reason string) Entitlement {
		return Entitlement{Claims: NonCommercialClaims(), Licensed: false, Reason: reason}
	}

	if strings.TrimSpace(rawKey) == "" {
		return nonCommercial("No commercial licence key is configured. Running under the non-commercial licence, which is free and uncapped for any non-commercial purpose. Commercial use requires a written agreement: contact@nopsai.com.")
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return nonCommercial("This build has no licence verification key, so the configured key cannot be checked. Running under the non-commercial licence.")
	}

	claims, err := Verify(Key(rawKey), publicKey, now)
	if err != nil {
		switch {
		case errorIs(err, ErrExpired):
			return nonCommercial("The commercial licence key expired. Running under the non-commercial licence until it is renewed: contact@nopsai.com.")
		case errorIs(err, ErrNotYetValid):
			return nonCommercial("The commercial licence key is not valid yet. Running under the non-commercial licence.")
		case errorIs(err, ErrBadSignature):
			return nonCommercial("The commercial licence key signature does not verify. Running under the non-commercial licence.")
		default:
			return nonCommercial("The commercial licence key could not be read. Running under the non-commercial licence.")
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

// Expired reports whether a commercial entitlement has passed its expiry. A key
// that expires while the process is running stops being valid without a
// restart, and the installation falls back to the non-commercial licence.
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
