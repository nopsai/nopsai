package nopsai

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"nopsai/internal/cli/apicatalog"
)

var wikiRoutePattern = regexp.MustCompile(`method: '([A-Z]+)',\s*\n?\s*path: '([^']+)'`)

// The wiki's REST index is authored, while the CLI catalogue is generated from
// the router. Adding a route without documenting it should fail the build rather
// than ship an endpoint nobody can find.
func TestWikiDocumentsEveryRegisteredRoute(t *testing.T) {
	documented := map[string]bool{}
	apiDir := filepath.Join("services", "ui", "src", "features", "product-docs", "content", "fields", "api")
	entries, err := os.ReadDir(apiDir)
	if err != nil {
		t.Fatalf("read %s: %v", apiDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ts") || entry.Name() == "index.ts" {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(apiDir, entry.Name()))
		if readErr != nil {
			t.Fatalf("read %s: %v", entry.Name(), readErr)
		}
		for _, match := range wikiRoutePattern.FindAllStringSubmatch(string(raw), -1) {
			documented[match[1]+" "+match[2]] = true
		}
	}
	if len(documented) == 0 {
		t.Fatal("found no documented routes; the wiki API modules moved or changed shape")
	}

	var undocumented []string
	registered := map[string]bool{}
	for _, route := range apicatalog.Routes() {
		key := route.Method + " " + route.Path
		registered[key] = true
		if !documented[key] {
			undocumented = append(undocumented, key)
		}
	}
	sort.Strings(undocumented)
	if len(undocumented) > 0 {
		t.Fatalf("routes registered but missing from the product wiki:\n  %s", strings.Join(undocumented, "\n  "))
	}

	// The generated catalogue only sees method-prefixed patterns. A route
	// registered as a bare path prefix is real but invisible to it, so it is
	// listed here with the registration that proves it exists.
	catalogueBlindSpots := map[string]string{
		"ANY /v1/resources/...": `services/nopsai/routes.go registers the prefix pattern "/v1/resources/"`,
	}

	var stale []string
	for key := range documented {
		if registered[key] {
			continue
		}
		if _, known := catalogueBlindSpots[key]; known {
			continue
		}
		stale = append(stale, key)
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("routes documented in the product wiki that no longer exist:\n  %s", strings.Join(stale, "\n  "))
	}
}
