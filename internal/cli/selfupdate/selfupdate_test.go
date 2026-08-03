package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdaterDownloadsVerifiesAndInstallsRelease(t *testing.T) {
	version := "2.7.184"
	archiveName, err := archiveName(version, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	binary := []byte("#!/bin/sh\necho updated\n")
	archive := tarGzipArchive(t, "nopsai", binary)
	checksums := []byte(fmt.Sprintf("%s  %s\n", sha256Hex(archive), archiveName))
	var sawToken bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") == "Bearer update-token" {
			sawToken = true
		}
		switch request.URL.Path {
		case "/v2.7.184/" + archiveName:
			_, _ = writer.Write(archive)
		case "/v2.7.184/SHA256SUMS":
			_, _ = writer.Write(checksums)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	installPath := filepath.Join(t.TempDir(), "nopsai")
	if err := os.WriteFile(installPath, []byte("old"), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := (Updater{
		HTTPClient: server.Client(),
		Token:      "update-token",
		GOOS:       "linux",
		GOARCH:     "amd64",
	}).Update(context.Background(), Options{
		Version:      version,
		AssetBaseURL: server.URL + "/v2.7.184",
		InstallPath:  installPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(installPath)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.Plan.AssetName != archiveName || string(contents) != string(binary) || !sawToken {
		t.Fatalf("result=%#v contents=%q sawToken=%v", result, contents, sawToken)
	}
	if info, err := os.Stat(installPath); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed binary mode = %v, %v", info.Mode().Perm(), err)
	}
}

func TestUpdaterDownloadsVerifiesAndInstallsOCIPackage(t *testing.T) {
	version := "2.7.184"
	archiveName, err := archiveName(version, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	binary := []byte("#!/bin/sh\necho updated from oci\n")
	archive := tarGzipArchive(t, "nopsai", binary)
	checksums := []byte(fmt.Sprintf("%s  %s\n", sha256Hex(archive), archiveName))
	archiveDigest := "sha256:" + sha256Hex(archive)
	checksumDigest := "sha256:" + sha256Hex(checksums)
	var sawChallengeToken bool
	var sawBearerToken bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			if request.URL.Query().Get("scope") != "repository:acme/nopsai-cli:pull" {
				t.Fatalf("token scope = %q, want repository pull scope", request.URL.Query().Get("scope"))
			}
			sawChallengeToken = true
			_, _ = writer.Write([]byte(`{"token":"registry-token"}`))
		case "/v2/acme/nopsai-cli/manifests/2.7.184":
			if request.Header.Get("Authorization") == "" {
				writer.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="ghcr.io",scope="repository:acme/nopsai-cli:pull"`, serverURL(request)))
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			if request.Header.Get("Authorization") == "Bearer registry-token" {
				sawBearerToken = true
			}
			writer.Header().Set("Content-Type", ociImageManifestMediaType)
			_, _ = writer.Write([]byte(fmt.Sprintf(`{"schemaVersion":2,"layers":[{"mediaType":"application/gzip","digest":%q,"size":%d,"annotations":{"%s":%q}},{"mediaType":"text/plain","digest":%q,"size":%d,"annotations":{"%s":"SHA256SUMS"}}]}`,
				archiveDigest, len(archive), ociLayerTitleAnnotation, archiveName,
				checksumDigest, len(checksums), ociLayerTitleAnnotation,
			)))
		case "/v2/acme/nopsai-cli/blobs/" + archiveDigest:
			_, _ = writer.Write(archive)
		case "/v2/acme/nopsai-cli/blobs/" + checksumDigest:
			_, _ = writer.Write(checksums)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	installPath := filepath.Join(t.TempDir(), "nopsai")
	result, err := (Updater{
		HTTPClient: server.Client(),
		GOOS:       "linux",
		GOARCH:     "amd64",
	}).Update(context.Background(), Options{
		Version:     version,
		PackageRef:  strings.TrimPrefix(server.URL, "https://") + "/acme/nopsai-cli",
		InstallPath: installPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(installPath)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.Plan.PackageRef == "" || string(contents) != string(binary) || !sawChallengeToken || !sawBearerToken {
		t.Fatalf("result=%#v contents=%q sawChallengeToken=%v sawBearerToken=%v", result, contents, sawChallengeToken, sawBearerToken)
	}
}

func TestUpdaterRejectsChecksumMismatch(t *testing.T) {
	version := "2.7.184"
	archiveName, err := archiveName(version, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	archive := tarGzipArchive(t, "nopsai", []byte("new"))
	checksums := []byte(strings.Repeat("0", 64) + "  " + archiveName + "\n")
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/release/" + archiveName:
			_, _ = writer.Write(archive)
		case "/release/SHA256SUMS":
			_, _ = writer.Write(checksums)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err = (Updater{HTTPClient: server.Client(), GOOS: "linux", GOARCH: "amd64"}).Update(context.Background(), Options{
		Version:      version,
		AssetBaseURL: server.URL + "/release",
		InstallPath:  filepath.Join(t.TempDir(), "nopsai"),
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("checksum mismatch error = %v", err)
	}
}

func TestUpdaterRejectsOversizedAssetFromContentLength(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", "5")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("12345"))
	}))
	defer server.Close()

	_, err := (Updater{HTTPClient: server.Client()}).download(context.Background(), server.URL+"/asset", 4)
	if err == nil || !strings.Contains(err.Error(), "exceeds 4 bytes") || !strings.Contains(err.Error(), "content length is 5 bytes") {
		t.Fatalf("oversized asset error = %v", err)
	}
}

func TestUpdaterReportsDownloadTimeout(t *testing.T) {
	client := &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}),
	}

	_, err := (Updater{HTTPClient: client}).download(context.Background(), "https://example.com/asset", 4)
	if err == nil || !strings.Contains(err.Error(), "timed out before response headers") || !strings.Contains(err.Error(), "--timeout") {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestReadBoundedBinaryRejectsOversizedDeclaredSize(t *testing.T) {
	_, err := readBoundedBinary(strings.NewReader("binary"), maxBinaryBytes+1)
	if err == nil || !strings.Contains(err.Error(), "update binary exceeds 512 MiB") || !strings.Contains(err.Error(), "declared size is") {
		t.Fatalf("oversized binary error = %v", err)
	}
}

func TestUpdaterDryRunPlansDefaultOCIPackageAsset(t *testing.T) {
	plan, err := (Updater{
		GOOS:       "darwin",
		GOARCH:     "arm64",
		Executable: func() (string, error) { return "/usr/local/bin/nopsai", nil },
	}).Plan(Options{Version: "2.7.184"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.AssetName != "nopsai-cli_2.7.184_darwin_arm64.tar.gz" ||
		plan.AssetURL != "oci://ghcr.io/nopsai/nopsai-cli:2.7.184#nopsai-cli_2.7.184_darwin_arm64.tar.gz" ||
		plan.ChecksumURL != "oci://ghcr.io/nopsai/nopsai-cli:2.7.184#SHA256SUMS" ||
		plan.PackageRef != DefaultOCIPackage ||
		plan.InstallPath != "/usr/local/bin/nopsai" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestUpdaterDryRunPlansLegacyGitHubAssetWhenRepositoryIsExplicit(t *testing.T) {
	plan, err := (Updater{
		GOOS:       "darwin",
		GOARCH:     "arm64",
		Executable: func() (string, error) { return "/usr/local/bin/nopsai", nil },
	}).Plan(Options{Version: "2.7.184", Repository: "acme/nopsai"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.AssetURL != "https://github.com/acme/nopsai/releases/download/v2.7.184/nopsai-cli_2.7.184_darwin_arm64.tar.gz" ||
		plan.ChecksumURL != "https://github.com/acme/nopsai/releases/download/v2.7.184/SHA256SUMS" ||
		plan.PackageRef != "" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestUpdaterRejectsUnsupportedOrNonExactVersions(t *testing.T) {
	if _, err := (Updater{GOOS: "plan9", GOARCH: "amd64"}).Plan(Options{Version: "2.7.184"}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported target error = %v", err)
	}
	if _, err := (Updater{GOOS: "linux", GOARCH: "amd64"}).Plan(Options{Version: "2.7.184+build"}); err == nil || !strings.Contains(err.Error(), "without prerelease or build metadata") {
		t.Fatalf("non-exact version error = %v", err)
	}
	if _, err := (Updater{GOOS: "linux", GOARCH: "amd64"}).Plan(Options{Version: "2.7.184", Repository: "../nopsai"}); err == nil || !strings.Contains(err.Error(), "parent traversal") {
		t.Fatalf("repository validation error = %v", err)
	}
	if _, err := (Updater{GOOS: "linux", GOARCH: "amd64"}).Plan(Options{Version: "2.7.184", PackageRef: "ghcr.io/acme/nopsai-cli:latest"}); err == nil || !strings.Contains(err.Error(), "must not include a tag") {
		t.Fatalf("package validation error = %v", err)
	}
}

func tarGzipArchive(t *testing.T, name string, contents []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func serverURL(request *http.Request) string {
	return "https://" + request.Host
}
