package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"nopsai/pkg/compatibility"
)

const (
	DefaultGitHubRepository = "nopsai/nopsai"
	checksumAssetName       = "SHA256SUMS"
	maxArchiveBytes         = 512 << 20
	maxBinaryBytes          = 512 << 20
	maxChecksumBytes        = 2 << 20
	defaultDownloadTimeout  = 5 * time.Minute
)

type Updater struct {
	HTTPClient *http.Client
	Token      string
	Executable func() (string, error)
	GOOS       string
	GOARCH     string
}

type Options struct {
	Version      string
	Repository   string
	AssetBaseURL string
	InstallPath  string
	DryRun       bool
}

type Plan struct {
	Version     string
	Repository  string
	GOOS        string
	GOARCH      string
	AssetName   string
	AssetURL    string
	ChecksumURL string
	InstallPath string
}

type Result struct {
	Plan    Plan
	Updated bool
	Bytes   int64
	Digest  string
}

func (u Updater) Plan(options Options) (Plan, error) {
	version, err := normalizeReleaseVersion(options.Version)
	if err != nil {
		return Plan{}, err
	}
	goos := valueOrDefault(u.GOOS, runtime.GOOS)
	goarch := valueOrDefault(u.GOARCH, runtime.GOARCH)
	assetName, err := archiveName(version, goos, goarch)
	if err != nil {
		return Plan{}, err
	}
	repository := valueOrDefault(options.Repository, DefaultGitHubRepository)
	if err := validateRepository(repository); err != nil {
		return Plan{}, err
	}
	assetURL, checksumURL, err := releaseAssetURLs(version, repository, options.AssetBaseURL, assetName)
	if err != nil {
		return Plan{}, err
	}
	installPath, err := u.installPath(options.InstallPath)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		Version:     version,
		Repository:  repository,
		GOOS:        goos,
		GOARCH:      goarch,
		AssetName:   assetName,
		AssetURL:    assetURL,
		ChecksumURL: checksumURL,
		InstallPath: installPath,
	}, nil
}

func (u Updater) Update(ctx context.Context, options Options) (Result, error) {
	plan, err := u.Plan(options)
	if err != nil {
		return Result{}, err
	}
	result := Result{Plan: plan}
	if options.DryRun {
		return result, nil
	}
	archiveBytes, err := u.download(ctx, plan.AssetURL, maxArchiveBytes)
	if err != nil {
		return Result{}, err
	}
	checksumBytes, err := u.download(ctx, plan.ChecksumURL, maxChecksumBytes)
	if err != nil {
		return Result{}, err
	}
	expectedDigest, err := checksumForAsset(checksumBytes, plan.AssetName)
	if err != nil {
		return Result{}, err
	}
	actualDigest := sha256Hex(archiveBytes)
	if actualDigest != expectedDigest {
		return Result{}, fmt.Errorf("verify %s: checksum mismatch: got %s, expected %s", plan.AssetName, actualDigest, expectedDigest)
	}
	binary, err := extractBinary(plan.AssetName, archiveBytes)
	if err != nil {
		return Result{}, err
	}
	if err := installBinary(plan.InstallPath, binary); err != nil {
		return Result{}, err
	}
	result.Updated = true
	result.Bytes = int64(len(binary))
	result.Digest = actualDigest
	return result, nil
}

func normalizeReleaseVersion(raw string) (string, error) {
	version, err := compatibility.ParseVersion(raw)
	if err != nil {
		return "", fmt.Errorf("invalid update version: %w", err)
	}
	if len(version.Prerelease) > 0 || len(version.Build) > 0 {
		return "", errors.New("update version must be an exact major.minor.patch release without prerelease or build metadata")
	}
	return version.String(), nil
}

func archiveName(version, goos, goarch string) (string, error) {
	switch goos {
	case "linux", "darwin":
		if goarch != "amd64" && goarch != "arm64" {
			return "", fmt.Errorf("unsupported CLI update target %s/%s", goos, goarch)
		}
		return fmt.Sprintf("nopsai-cli_%s_%s_%s.tar.gz", version, goos, goarch), nil
	case "windows":
		if goarch != "amd64" {
			return "", fmt.Errorf("unsupported CLI update target %s/%s", goos, goarch)
		}
		return fmt.Sprintf("nopsai-cli_%s_%s_%s.zip", version, goos, goarch), nil
	default:
		return "", fmt.Errorf("unsupported CLI update target %s/%s", goos, goarch)
	}
}

func releaseAssetURLs(version, repository, baseURL, assetName string) (string, string, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = fmt.Sprintf("https://github.com/%s/releases/download/v%s", repository, version)
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", "", fmt.Errorf("parse update asset base URL: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", errors.New("update asset base URL must be an absolute https URL without query or fragment")
	}
	return baseURL + "/" + url.PathEscape(assetName), baseURL + "/" + checksumAssetName, nil
}

func validateRepository(repository string) error {
	parts := strings.Split(strings.TrimSpace(repository), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("update repository must use owner/repository")
	}
	for _, part := range parts {
		if strings.Contains(part, "..") {
			return errors.New("update repository cannot contain parent traversal")
		}
		for _, character := range part {
			valid := character >= 'a' && character <= 'z' ||
				character >= 'A' && character <= 'Z' ||
				character >= '0' && character <= '9' ||
				character == '.' || character == '_' || character == '-'
			if !valid {
				return errors.New("update repository may contain only letters, numbers, periods, underscores, and dashes")
			}
		}
	}
	return nil
}

func (u Updater) download(ctx context.Context, source string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, fmt.Errorf("build update download request: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	if token := strings.TrimSpace(u.Token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := u.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultDownloadTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("download update asset %s timed out before response headers; retry or pass a larger --timeout: %w", source, err)
		}
		return nil, fmt.Errorf("download update asset %s: %w", source, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download update asset %s: HTTP %d", source, response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, fmt.Errorf("update asset %s exceeds %s: content length is %s", source, formatByteLimit(limit), formatByteLimit(response.ContentLength))
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read update asset %s: %w", source, err)
	}
	if int64(len(contents)) > limit {
		return nil, fmt.Errorf("update asset %s exceeds %s", source, formatByteLimit(limit))
	}
	return contents, nil
}

func checksumForAsset(contents []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		candidate := strings.TrimPrefix(fields[1], "*")
		candidate = strings.TrimPrefix(candidate, "./")
		if candidate != assetName {
			continue
		}
		digest := strings.ToLower(fields[0])
		if len(digest) != 64 {
			return "", fmt.Errorf("invalid checksum for %s", assetName)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return "", fmt.Errorf("invalid checksum for %s: %w", assetName, err)
		}
		return digest, nil
	}
	return "", fmt.Errorf("%s is missing from %s", assetName, checksumAssetName)
}

func extractBinary(assetName string, archiveBytes []byte) ([]byte, error) {
	if strings.HasSuffix(assetName, ".zip") {
		return extractZipBinary(archiveBytes)
	}
	if strings.HasSuffix(assetName, ".tar.gz") {
		return extractTarGzipBinary(archiveBytes)
	}
	return nil, fmt.Errorf("unsupported update archive %s", assetName)
}

func extractTarGzipBinary(archiveBytes []byte) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		return nil, fmt.Errorf("open update tar.gz: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read update tar.gz: %w", err)
		}
		if filepath.Base(header.Name) != "nopsai" || header.Typeflag != tar.TypeReg {
			continue
		}
		return readBoundedBinary(tarReader, header.Size)
	}
	return nil, errors.New("update archive does not contain nopsai binary")
}

func extractZipBinary(archiveBytes []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return nil, fmt.Errorf("open update zip: %w", err)
	}
	for _, file := range reader.File {
		if filepath.Base(file.Name) != "nopsai.exe" || file.FileInfo().IsDir() {
			continue
		}
		readCloser, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s in update zip: %w", file.Name, err)
		}
		contents, readErr := readBoundedBinary(readCloser, int64(file.UncompressedSize64))
		closeErr := readCloser.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close %s in update zip: %w", file.Name, closeErr)
		}
		return contents, nil
	}
	return nil, errors.New("update archive does not contain nopsai.exe binary")
}

func readBoundedBinary(reader io.Reader, declaredSize int64) ([]byte, error) {
	if declaredSize < 0 || declaredSize > maxBinaryBytes {
		return nil, fmt.Errorf("update binary exceeds %s: declared size is %s", formatByteLimit(maxBinaryBytes), formatByteLimit(declaredSize))
	}
	contents, err := io.ReadAll(io.LimitReader(reader, maxBinaryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read update binary: %w", err)
	}
	if len(contents) == 0 {
		return nil, errors.New("update binary is empty")
	}
	if int64(len(contents)) > maxBinaryBytes {
		return nil, fmt.Errorf("update binary exceeds %s", formatByteLimit(maxBinaryBytes))
	}
	return contents, nil
}

func (u Updater) installPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		executable := u.Executable
		if executable == nil {
			executable = os.Executable
		}
		resolved, err := executable()
		if err != nil {
			return "", fmt.Errorf("resolve current executable: %w", err)
		}
		path = resolved
	}
	if path == "" {
		return "", errors.New("update install path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve update install path: %w", err)
	}
	if evaluated, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = evaluated
	}
	return absolute, nil
}

func installBinary(path string, contents []byte) error {
	mode := fs.FileMode(0o755)
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("update install path %s is a directory", path)
		}
		mode = info.Mode().Perm() | 0o111
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect update install path %s: %w", path, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create update install directory %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create update temporary file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return fmt.Errorf("set update binary permissions: %w", err)
	}
	if _, err := temp.Write(contents); err != nil {
		temp.Close()
		return fmt.Errorf("write update binary: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync update binary: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close update binary: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func sha256Hex(contents []byte) string {
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}

func valueOrDefault(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func formatByteLimit(bytes int64) string {
	if bytes < 0 {
		return fmt.Sprintf("%d bytes", bytes)
	}
	const mib = int64(1 << 20)
	if bytes%mib == 0 && bytes >= mib {
		return fmt.Sprintf("%d MiB", bytes/mib)
	}
	return fmt.Sprintf("%d bytes", bytes)
}
