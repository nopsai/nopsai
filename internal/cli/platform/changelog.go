package platform

import (
	"context"
	"fmt"
	"strings"
)

// ChangelogAssetName returns the published changelog asset for a release.
func ChangelogAssetName(version string) string {
	return "nopsai-changelog-" + strings.TrimSpace(version) + ".md"
}

// ChangelogFetcher downloads and verifies the changelog for one release
// version. It returns the changelog body and the source it was read from.
type ChangelogFetcher func(ctx context.Context, version string) (string, string, error)

// Changelog is the parsed release changelog used by upgrade planning.
type Changelog struct {
	Version  string
	Source   string
	Body     string
	Breaking []string
}

// ParseChangelog splits a generated release changelog into its body and the
// entries listed under the Breaking heading. scripts/generate-changelog.sh
// writes that heading whenever a commit is marked as a breaking change.
func ParseChangelog(version, source, body string) Changelog {
	changelog := Changelog{
		Version: strings.TrimSpace(version),
		Source:  strings.TrimSpace(source),
		Body:    strings.TrimRight(body, "\n"),
	}
	inBreaking := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inBreaking = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")), "breaking")
			continue
		}
		if !inBreaking || !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		entry := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		if entry != "" {
			changelog.Breaking = append(changelog.Breaking, entry)
		}
	}
	return changelog
}

// RequiredActions turns breaking changelog entries into the operator checklist
// printed before a series upgrade is applied.
func (c Changelog) RequiredActions() []string {
	actions := make([]string, 0, len(c.Breaking))
	for _, entry := range c.Breaking {
		actions = append(actions, "Breaking change: "+entry)
	}
	return actions
}

// ReleaseChangelogFetcher builds a fetcher over a verified release asset
// download, such as the one implemented by the CLI self-update package.
func ReleaseChangelogFetcher(download func(ctx context.Context, assetName string) ([]byte, string, error)) ChangelogFetcher {
	if download == nil {
		return nil
	}
	return func(ctx context.Context, version string) (string, string, error) {
		assetName := ChangelogAssetName(version)
		contents, source, err := download(ctx, assetName)
		if err != nil {
			return "", source, fmt.Errorf("download release changelog %s: %w", assetName, err)
		}
		return string(contents), source, nil
	}
}
