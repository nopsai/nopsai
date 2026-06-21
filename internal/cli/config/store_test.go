package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStoreContextAndCredentialLifecycle(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if store.Dir() == "" || filepath.Base(store.ConfigPath()) != configFileName || filepath.Base(store.CredentialsPath()) != credentialsFileName {
		t.Fatalf("unexpected store paths: %#v", store)
	}
	if cfg, err := store.Load(); err != nil || cfg.Version != currentVersion || len(cfg.Contexts) != 0 {
		t.Fatalf("Load empty = %#v, %v", cfg, err)
	}

	ctx, err := store.AddContext("prod-eu.1", "https://api.example.com/root/")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.API != "https://api.example.com/root" {
		t.Fatalf("API = %q", ctx.API)
	}
	if _, err := store.AddContext("staging", "http://staging.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := store.UseContext("staging"); err != nil {
		t.Fatal(err)
	}
	name, resolved, err := store.ResolveContext("")
	if err != nil || name != "staging" || resolved.API != "http://staging.example.com" {
		t.Fatalf("ResolveContext = %q, %#v, %v", name, resolved, err)
	}
	if err := store.SaveToken("staging", "nopat_secret"); err != nil {
		t.Fatal(err)
	}
	if token, err := store.Token("staging"); err != nil || token != "nopat_secret" {
		t.Fatalf("Token = %q, %v", token, err)
	}
	configBytes, err := os.ReadFile(store.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configBytes), "nopat_secret") {
		t.Fatal("config file leaked credential")
	}
	assertPrivateFile(t, store.ConfigPath())
	assertPrivateFile(t, store.CredentialsPath())

	if err := store.DeleteContext("staging"); err != nil {
		t.Fatal(err)
	}
	if token, err := store.Token("staging"); err != nil || token != "" {
		t.Fatalf("deleted token = %q, %v", token, err)
	}
	if _, _, err := store.ResolveContext(""); err == nil {
		t.Fatal("expected no-current-context error")
	}
	if err := store.DeleteToken("prod-eu.1"); err != nil {
		t.Fatal(err)
	}
}

func TestStoreValidationAndMalformedFiles(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "-bad", "has space", strings.Repeat("a", 64)} {
		if err := ValidateContextName(name); err == nil {
			t.Errorf("ValidateContextName(%q) succeeded", name)
		}
	}
	if _, err := store.AddContext("valid", "ftp://example.com"); err == nil {
		t.Fatal("expected invalid API URL error")
	}
	if err := store.UseContext("missing"); err == nil {
		t.Fatal("expected missing context error")
	}
	if err := store.DeleteContext("missing"); err == nil {
		t.Fatal("expected delete error")
	}
	if err := store.SaveToken("valid", ""); err == nil {
		t.Fatal("expected empty token error")
	}
	if err := store.SaveToken("valid", "one\ntwo"); err == nil {
		t.Fatal("expected multiline token error")
	}

	if err := os.WriteFile(store.ConfigPath(), []byte("version: 2\ncontexts: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("expected unsupported config error")
	}
	if err := os.WriteFile(store.ConfigPath(), []byte("version: 1\nunknown: true\ncontexts: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("expected unknown field error")
	}
	if err := os.WriteFile(store.CredentialsPath(), []byte("version: 1\ntokens: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Token("valid"); err == nil {
		t.Fatal("expected broad credential permissions error")
	}
}

func TestStoreRejectsNonRegularFiles(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.ConfigPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("expected non-regular config error")
	}
}

func TestStoreAllowsConfigSymlinkButRejectsCredentialSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions are platform-dependent")
	}
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "store"))
	if err := os.MkdirAll(store.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	configTarget := filepath.Join(dir, "config-target.yaml")
	if err := os.WriteFile(configTarget, []byte("version: 1\ncontexts: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(configTarget, store.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err != nil {
		t.Fatalf("config symlink: %v", err)
	}
	credentialsTarget := filepath.Join(dir, "credentials-target.yaml")
	if err := os.WriteFile(credentialsTarget, []byte("version: 1\ntokens: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(credentialsTarget, store.CredentialsPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Token("valid"); err == nil {
		t.Fatal("credential symlink was accepted")
	}
}

func TestDefaultDirAndNewStore(t *testing.T) {
	t.Setenv("NOPSAI_CONFIG_DIR", "relative-config")
	dir, err := DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(dir) || filepath.Base(dir) != "relative-config" {
		t.Fatalf("DefaultDir = %q", dir)
	}
	store, err := NewStore("")
	if err != nil || store.Dir() != dir {
		t.Fatalf("NewStore = %#v, %v", store, err)
	}
}

func TestNormalizeAPIURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{" https://example.com/ ", "https://example.com", true},
		{"http://localhost:8080/prefix/", "http://localhost:8080/prefix", true},
		{"ssh://example.com", "", false},
		{"https:///missing", "", false},
		{"https://user@example.com", "", false},
		{"https://example.com?q=1", "", false},
		{"https://example.com/#fragment", "", false},
	}
	for _, test := range tests {
		got, err := NormalizeAPIURL(test.input)
		if (err == nil) != test.ok || got != test.want {
			t.Errorf("NormalizeAPIURL(%q) = %q, %v", test.input, got, err)
		}
	}
}

func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("%s mode = %o", path, info.Mode().Perm())
	}
}
