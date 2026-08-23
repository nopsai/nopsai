package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func testKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	// Generated per test run: the fixture is self-contained and depends on no
	// checked-in key material.
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	return public, private
}

func validClaims() Claims {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	return Claims{
		Licensee:          "Acme BV",
		LicenseID:         "lic-001",
		Tier:              TierEnterprise,
		IssuedAt:          now,
		ExpiresAt:         now.AddDate(1, 0, 0),
		MaxUsers:          50,
		MaxTeams:          10,
		MaxConcurrentRuns: 8,
		Features:          []string{"sso", "Kubernetes-Runner"},
	}
}

func TestSignedKeyRoundTrips(t *testing.T) {
	public, private := testKeypair(t)
	claims := validClaims()

	key, err := Sign(claims, private)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if !strings.Contains(string(key), ".") {
		t.Fatalf("key %q must be payload.signature", key)
	}

	got, err := Verify(key, public, claims.IssuedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got.Licensee != claims.Licensee || got.MaxUsers != claims.MaxUsers || got.Tier != claims.Tier {
		t.Fatalf("round-tripped claims = %#v, want %#v", got, claims)
	}
}

func TestVerifyRejectsUntrustworthyKeys(t *testing.T) {
	public, private := testKeypair(t)
	otherPublic, _ := testKeypair(t)
	claims := validClaims()
	key, err := Sign(claims, private)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	tampered := Key(strings.Replace(string(key), "a", "b", 1))

	cases := []struct {
		name   string
		key    Key
		public ed25519.PublicKey
		now    time.Time
		want   error
	}{
		{"empty key", "", public, claims.IssuedAt, ErrEmptyKey},
		{"no verifying key", key, nil, claims.IssuedAt, ErrNoVerifyingKey},
		{"not two parts", "notasignedkey", public, claims.IssuedAt, ErrMalformedKey},
		{"signed by another issuer", key, otherPublic, claims.IssuedAt, ErrBadSignature},
		{"expired", key, public, claims.ExpiresAt.Add(time.Hour), ErrExpired},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Verify(testCase.key, testCase.public, testCase.now); !errors.Is(err, testCase.want) {
				t.Fatalf("Verify() error = %v, want %v", err, testCase.want)
			}
		})
	}

	// Tampering must fail, whichever byte changed.
	if _, err := Verify(tampered, public, claims.IssuedAt); err == nil {
		t.Fatal("a tampered key must not verify")
	}
}

func TestVerifyRejectsAKeyThatIsNotYetValid(t *testing.T) {
	public, private := testKeypair(t)
	claims := validClaims()
	claims.NotBefore = claims.IssuedAt.AddDate(0, 1, 0)
	key, err := Sign(claims, private)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if _, err := Verify(key, public, claims.IssuedAt); !errors.Is(err, ErrNotYetValid) {
		t.Fatalf("Verify() error = %v, want %v", err, ErrNotYetValid)
	}
}

func TestSignRefusesAKeyThatNamesNobody(t *testing.T) {
	_, private := testKeypair(t)
	claims := validClaims()
	claims.Licensee = "   "
	if _, err := Sign(claims, private); !errors.Is(err, ErrKeyMissingScope) {
		t.Fatalf("Sign() error = %v, want %v", err, ErrKeyMissingScope)
	}
}

// An untrustworthy key must never grant more than no key at all.
func TestResolveFallsBackToEvaluationRatherThanTrustingABadKey(t *testing.T) {
	public, private := testKeypair(t)
	otherPublic, _ := testKeypair(t)
	claims := validClaims()
	key, err := Sign(claims, private)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	evaluation := EvaluationClaims()

	cases := []struct {
		name   string
		key    string
		public ed25519.PublicKey
		now    time.Time
	}{
		{"no key configured", "", public, claims.IssuedAt},
		{"no verification key in this build", string(key), nil, claims.IssuedAt},
		{"signature does not verify", string(key), otherPublic, claims.IssuedAt},
		{"key has expired", string(key), public, claims.ExpiresAt.Add(time.Hour)},
		{"key is unreadable", "garbage", public, claims.IssuedAt},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := Resolve(testCase.key, testCase.public, testCase.now)
			if got.Licensed {
				t.Fatal("an unverified key must never be reported as licensed")
			}
			if got.Claims.MaxUsers != evaluation.MaxUsers || got.Claims.Tier != TierEvaluation {
				t.Fatalf("claims = %#v, want evaluation limits", got.Claims)
			}
			if strings.TrimSpace(got.Reason) == "" {
				t.Fatal("a non-licensed entitlement must explain itself to the operator")
			}
		})
	}
}

func TestResolveGrantsTheClaimsOfAValidKey(t *testing.T) {
	public, private := testKeypair(t)
	claims := validClaims()
	key, err := Sign(claims, private)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	got := Resolve(string(key), public, claims.IssuedAt.Add(time.Hour))
	if !got.Licensed {
		t.Fatalf("a valid key must be licensed, got reason %q", got.Reason)
	}
	if got.Claims.MaxUsers != 50 || got.Claims.Licensee != "Acme BV" {
		t.Fatalf("claims = %#v, want the signed claims", got.Claims)
	}
}

// A key that omits a limit must not be read as forbidding everything, which
// would make a paid key more restrictive than no key.
func TestOmittedLimitsMeanUnlimited(t *testing.T) {
	entitlement := Entitlement{Licensed: true, Claims: Claims{Licensee: "Acme BV"}}
	for _, current := range []int{0, 1, 1000} {
		if !entitlement.AllowsUsers(current) || !entitlement.AllowsTeams(current) || !entitlement.AllowsConcurrentRuns(current) {
			t.Fatalf("omitted limits must allow any count, blocked at %d", current)
		}
	}
	if !Unlimited(0) || !Unlimited(-1) || Unlimited(1) {
		t.Fatal("Unlimited must treat zero and negative as no ceiling")
	}
}

func TestLimitsBlockAtTheCeilingNotBeforeIt(t *testing.T) {
	entitlement := Entitlement{Licensed: true, Claims: Claims{Licensee: "Acme BV", MaxUsers: 3}}
	if !entitlement.AllowsUsers(2) {
		t.Fatal("a third user must be allowed under a limit of 3")
	}
	if entitlement.AllowsUsers(3) {
		t.Fatal("a fourth user must be refused under a limit of 3")
	}
}

func TestEvaluationLimitsAreUsableNotCrippling(t *testing.T) {
	claims := EvaluationClaims()
	if claims.MaxUsers < 2 || claims.MaxTeams < 1 || claims.MaxConcurrentRuns < 1 {
		t.Fatalf("evaluation limits %#v must allow a real trial", claims)
	}
	if claims.Tier != TierEvaluation {
		t.Fatalf("tier = %q, want %q", claims.Tier, TierEvaluation)
	}
}

func TestExpiredReportsAKeyThatLapsedWhileRunning(t *testing.T) {
	claims := validClaims()
	entitlement := Entitlement{Licensed: true, Claims: claims}
	if entitlement.Expired(claims.IssuedAt) {
		t.Fatal("a current key must not report expired")
	}
	if !entitlement.Expired(claims.ExpiresAt.Add(time.Hour)) {
		t.Fatal("a lapsed key must report expired without a restart")
	}
	unlicensed := Entitlement{Claims: EvaluationClaims()}
	if unlicensed.Expired(time.Now()) {
		t.Fatal("evaluation mode never expires into something worse")
	}
}

func TestHasFeatureIgnoresCasingAndSpacing(t *testing.T) {
	claims := validClaims()
	for _, feature := range []string{"sso", "SSO", " kubernetes-runner "} {
		if !claims.HasFeature(feature) {
			t.Fatalf("HasFeature(%q) = false, want true", feature)
		}
	}
	if claims.HasFeature("air-gapped") || claims.HasFeature("") {
		t.Fatal("HasFeature must not invent entitlements")
	}
}

func TestParsePublicKeyAcceptsTheUsualEncodings(t *testing.T) {
	public, _ := testKeypair(t)
	for name, encoded := range map[string]string{
		"std":     base64.StdEncoding.EncodeToString(public),
		"rawstd":  base64.RawStdEncoding.EncodeToString(public),
		"url":     base64.URLEncoding.EncodeToString(public),
		"rawurl":  base64.RawURLEncoding.EncodeToString(public),
		"padded ": " " + base64.StdEncoding.EncodeToString(public) + " ",
	} {
		t.Run(name, func(t *testing.T) {
			parsed, ok := ParsePublicKey(encoded)
			if !ok || !parsed.Equal(public) {
				t.Fatalf("ParsePublicKey(%s) did not round-trip", name)
			}
		})
	}
	if _, ok := ParsePublicKey("not-base64"); ok {
		t.Fatal("ParsePublicKey must reject a value that is not a key")
	}
	if _, ok := ParsePublicKey(""); ok {
		t.Fatal("ParsePublicKey must reject an empty value")
	}
}
