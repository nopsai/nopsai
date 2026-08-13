package selfupdate

import (
	"context"
	"fmt"
	"strings"
)

const maxReleaseAssetBytes = 8 << 20

// AssetOptions selects one published release asset. It uses the same sources as
// a CLI self-update: an explicit HTTPS base URL, an OCI package, or the GitHub
// release for the given repository.
type AssetOptions struct {
	Version      string
	AssetName    string
	Repository   string
	PackageRef   string
	AssetBaseURL string
	MaxBytes     int64
}

// ReleaseAsset downloads one release asset and verifies it against the
// SHA256SUMS file published beside it. Callers use it for non-binary assets such
// as the release changelog; binary self-updates keep using Update.
func (u Updater) ReleaseAsset(ctx context.Context, options AssetOptions) ([]byte, string, error) {
	version, err := normalizeReleaseVersion(options.Version)
	if err != nil {
		return nil, "", err
	}
	assetName := strings.TrimSpace(options.AssetName)
	if assetName == "" {
		return nil, "", fmt.Errorf("release asset name is required")
	}
	if strings.ContainsAny(assetName, "/\\") {
		return nil, "", fmt.Errorf("release asset name %q must not contain a path separator", assetName)
	}
	repository := strings.TrimSpace(options.Repository)
	packageRef := strings.TrimSpace(options.PackageRef)
	baseURL := strings.TrimSpace(options.AssetBaseURL)
	if baseURL == "" && packageRef == "" && repository == "" {
		repository = DefaultGitHubRepository
	}
	assetURL, checksumURL, _, err := updateAssetURLs(version, repository, packageRef, baseURL, assetName)
	if err != nil {
		return nil, "", err
	}
	limit := options.MaxBytes
	if limit <= 0 {
		limit = maxReleaseAssetBytes
	}
	contents, err := u.download(ctx, assetURL, limit)
	if err != nil {
		return nil, assetURL, err
	}
	checksums, err := u.download(ctx, checksumURL, maxChecksumBytes)
	if err != nil {
		return nil, assetURL, err
	}
	expectedDigest, err := checksumForAsset(checksums, assetName)
	if err != nil {
		return nil, assetURL, err
	}
	if actualDigest := sha256Hex(contents); actualDigest != expectedDigest {
		return nil, assetURL, fmt.Errorf("verify %s: checksum mismatch: got %s, expected %s", assetName, actualDigest, expectedDigest)
	}
	return contents, assetURL, nil
}
