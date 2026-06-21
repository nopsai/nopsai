package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverFindsLiteralRoutesAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "services", "nopsai")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := `package sample
func routes(mux interface{ HandleFunc(string, any) }) {
	mux.HandleFunc("GET /v1/z", nil)
	mux.HandleFunc("POST /v1/a/{id}", nil)
	mux.HandleFunc("GET /v1/z", nil)
}`
	if err := os.WriteFile(filepath.Join(dir, "routes.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "routes_test.go"), []byte(`package sample`), 0o600); err != nil {
		t.Fatal(err)
	}
	routes, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 || routes[0].Path != "/v1/a/{id}" || routes[1].Path != "/v1/z" {
		t.Fatalf("routes = %#v", routes)
	}
}

func TestDiscoverReportsMissingOrInvalidSources(t *testing.T) {
	if _, err := Discover(t.TempDir()); err == nil {
		t.Fatal("missing service directory succeeded")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "services", "nopsai")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("not go"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(root); err == nil {
		t.Fatal("invalid Go source succeeded")
	}
}
