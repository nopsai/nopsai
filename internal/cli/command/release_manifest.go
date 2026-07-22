package command

import (
	"net/http"
	"strings"

	"nopsai/internal/cli/platform"
)

func releaseManifestResolver(root *rootOptions, httpClient *http.Client) platform.ManifestResolver {
	token, hostSuffixes := releaseManifestAuth(root)
	urlTemplate := ""
	if root != nil && root.dependencies.Getenv != nil {
		urlTemplate = strings.TrimSpace(root.dependencies.Getenv("NOPSAI_RELEASE_MANIFEST_URL_TEMPLATE"))
	}
	return platform.ManifestResolver{
		HTTPClient:            httpClient,
		URLTemplate:           urlTemplate,
		AuthToken:             token,
		AuthTokenHostSuffixes: hostSuffixes,
	}
}

func releaseManifestAuth(root *rootOptions) (string, []string) {
	if root == nil || root.dependencies.Getenv == nil {
		return "", nil
	}
	if token := strings.TrimSpace(root.dependencies.Getenv("NOPSAI_RELEASE_MANIFEST_TOKEN")); token != "" {
		return token, nil
	}
	for _, key := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if token := strings.TrimSpace(root.dependencies.Getenv(key)); token != "" {
			return token, []string{"github.com"}
		}
	}
	return "", nil
}
