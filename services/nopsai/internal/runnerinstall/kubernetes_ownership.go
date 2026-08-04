package runnerinstall

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	"nopsai/config"
)

const (
	PlatformIDEnv   = "NOPSAI_PLATFORM_ID"
	PlatformIDLabel = "nopsai.io/platform-id"

	KubernetesPlatformIDEnv   = PlatformIDEnv
	KubernetesPlatformIDLabel = PlatformIDLabel
)

var platformIDInvalidRegex = regexp.MustCompile(`[^a-z0-9-]+`)

func PlatformID(cfg config.Config) string {
	if platformID := NormalizePlatformID(cfg.PlatformID); platformID != "" {
		return platformID
	}
	return derivedPlatformID(cfg)
}

func KubernetesPlatformID(cfg config.Config) string {
	return PlatformID(cfg)
}

func NormalizePlatformID(value string) string {
	platformID := strings.ToLower(strings.TrimSpace(value))
	platformID = platformIDInvalidRegex.ReplaceAllString(platformID, "-")
	platformID = strings.Trim(platformID, "-")
	if platformID == "" {
		return ""
	}
	if len(platformID) <= 63 {
		return platformID
	}
	sum := sha256.Sum256([]byte(platformID))
	prefix := strings.Trim(platformID[:52], "-")
	if prefix == "" {
		prefix = "platform"
	}
	return prefix + "-" + fmt.Sprintf("%x", sum[:5])
}

func derivedPlatformID(cfg config.Config) string {
	material := strings.TrimSpace(cfg.MasterKey)
	if material == "" {
		material = strings.Join(nonEmptyStrings(
			cfg.EffectiveServiceJWTSigningKey(),
			cfg.EffectiveServiceJWTIssuer(),
			cfg.EffectiveServiceJWTAudience(),
			cfg.EffectiveNopsaiAPIURL(),
			cfg.PublicURL,
		), "\x00")
	}
	if strings.TrimSpace(material) == "" {
		material = "nopsai"
	}
	sum := sha256.Sum256([]byte(material))
	return "p-" + fmt.Sprintf("%x", sum[:8])
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
