// Command licenseissuer signs NopsAI licence keys.
//
// It is deliberately not part of any distributed binary: the release pipeline
// builds ./cmd/..., so this tool only exists when it is run from a source
// checkout by whoever holds the signing key.
//
// Generate an issuing keypair once:
//
//	go run ./internal/tools/licenseissuer -generate-keypair
//
// Then compile the printed public key into the product with
// -ldflags "-X nopsai/pkg/buildinfo.LicensePublicKey=<public key>" and keep the
// private key in a secret manager. The private key never leaves the issuing
// environment and is never shipped.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"nopsai/pkg/license"
)

func main() {
	var (
		generateKeypair = flag.Bool("generate-keypair", false, "generate a new issuing keypair and exit")
		privateKeyB64   = flag.String("private-key", os.Getenv("NOPSAI_LICENSE_PRIVATE_KEY"), "base64 Ed25519 private key (or NOPSAI_LICENSE_PRIVATE_KEY)")
		licensee        = flag.String("licensee", "", "the organisation the licence is issued to")
		licenseID       = flag.String("license-id", "", "issuer-side identifier for this licence")
		tier            = flag.String("tier", string(license.TierTeam), "evaluation, team, or enterprise")
		days            = flag.Int("days", 365, "validity in days from now")
		maxUsers        = flag.Int("max-users", 0, "user ceiling, 0 for unlimited")
		maxTeams        = flag.Int("max-teams", 0, "team ceiling, 0 for unlimited")
		maxRuns         = flag.Int("max-concurrent-runs", 0, "concurrent run ceiling, 0 for unlimited")
		features        = flag.String("features", "", "comma-separated feature names")
	)
	flag.Parse()

	if *generateKeypair {
		if err := printNewKeypair(); err != nil {
			fail(err)
		}
		return
	}

	privateKey, err := decodePrivateKey(*privateKeyB64)
	if err != nil {
		fail(err)
	}

	now := time.Now().UTC()
	claims := license.Claims{
		Licensee:          strings.TrimSpace(*licensee),
		LicenseID:         strings.TrimSpace(*licenseID),
		Tier:              license.Tier(strings.TrimSpace(*tier)),
		IssuedAt:          now,
		ExpiresAt:         now.AddDate(0, 0, *days),
		MaxUsers:          *maxUsers,
		MaxTeams:          *maxTeams,
		MaxConcurrentRuns: *maxRuns,
		Features:          splitFeatures(*features),
	}

	key, err := license.Sign(claims, privateKey)
	if err != nil {
		fail(err)
	}
	fmt.Println(string(key))
}

func printNewKeypair() error {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	fmt.Printf("public key  (compile into the product): %s\n", base64.StdEncoding.EncodeToString(public))
	fmt.Printf("private key (keep in a secret manager): %s\n", base64.StdEncoding.EncodeToString(private))
	fmt.Println()
	fmt.Println("The private key is the only thing that can mint licences. It must never be")
	fmt.Println("committed, shipped in an artifact, or shared outside the issuing environment.")
	return nil
}

func decodePrivateKey(encoded string) (ed25519.PrivateKey, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, fmt.Errorf("a signing key is required; pass -private-key or set NOPSAI_LICENSE_PRIVATE_KEY")
	}
	for _, decoder := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := decoder.DecodeString(trimmed); err == nil && len(decoded) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(decoded), nil
		}
	}
	return nil, fmt.Errorf("signing key is not a base64 Ed25519 private key")
}

func splitFeatures(raw string) []string {
	var features []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			features = append(features, trimmed)
		}
	}
	return features
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "licenseissuer: %v\n", err)
	os.Exit(1)
}
