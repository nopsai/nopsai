package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	DefaultOCIPackage       = "ghcr.io/nopsai/nopsai-cli"
	checksumAssetName       = "SHA256SUMS"
	maxArchiveBytes         = 512 << 20
	maxBinaryBytes          = 512 << 20
	maxChecksumBytes        = 2 << 20
	defaultDownloadTimeout  = 5 * time.Minute
)

const (
	ociImageManifestMediaType    = "application/vnd.oci.image.manifest.v1+json"
	ociDockerManifestMediaType   = "application/vnd.docker.distribution.manifest.v2+json"
	ociArtifactManifestMediaType = "application/vnd.oci.artifact.manifest.v1+json"
	ociLayerTitleAnnotation      = "org.opencontainers.image.title"
	ociAssetURLScheme            = "oci"
	ociPackageReferenceURLScheme = "oci://"
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
	PackageRef   string
	AssetBaseURL string
	InstallPath  string
	DryRun       bool
}

type Plan struct {
	Version     string
	Repository  string
	PackageRef  string
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
	assetURL, checksumURL, packageRef, err := updateAssetURLs(version, options.Repository, options.PackageRef, options.AssetBaseURL, assetName)
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
		PackageRef:  packageRef,
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
		if err := validateRepository(repository); err != nil {
			return "", "", err
		}
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

func updateAssetURLs(version, repository, packageRef, baseURL, assetName string) (string, string, string, error) {
	repository = strings.TrimSpace(repository)
	if strings.TrimSpace(baseURL) != "" {
		assetURL, checksumURL, err := releaseAssetURLs(version, valueOrDefault(repository, DefaultGitHubRepository), baseURL, assetName)
		return assetURL, checksumURL, "", err
	}
	if strings.TrimSpace(packageRef) == "" && repository != "" {
		assetURL, checksumURL, err := releaseAssetURLs(version, repository, "", assetName)
		return assetURL, checksumURL, "", err
	}
	normalizedPackage, err := normalizeOCIPackageRef(valueOrDefault(packageRef, DefaultOCIPackage))
	if err != nil {
		return "", "", "", err
	}
	base := fmt.Sprintf("%s%s:%s", ociPackageReferenceURLScheme, normalizedPackage, version)
	return base + "#" + assetName, base + "#" + checksumAssetName, normalizedPackage, nil
}

func normalizeOCIPackageRef(raw string) (string, error) {
	ref := strings.TrimSpace(raw)
	ref = strings.TrimPrefix(ref, ociPackageReferenceURLScheme)
	ref = strings.TrimRight(ref, "/")
	if ref == "" {
		return "", errors.New("update package must be an OCI package reference")
	}
	if strings.ContainsAny(ref, "?#") {
		return "", errors.New("update package reference cannot include query or fragment")
	}
	if strings.Contains(ref, "@") {
		return "", errors.New("update package reference must not include a digest")
	}
	firstSlash := strings.Index(ref, "/")
	if firstSlash <= 0 || firstSlash == len(ref)-1 {
		return "", errors.New("update package must use registry/repository")
	}
	host := ref[:firstSlash]
	repository := ref[firstSlash+1:]
	if strings.Contains(host, "://") || strings.Contains(host, "..") || strings.ContainsAny(host, " \t\r\n") {
		return "", errors.New("update package registry is invalid")
	}
	for _, part := range strings.Split(repository, "/") {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, " \t\r\n") {
			return "", errors.New("update package repository is invalid")
		}
		if strings.Contains(part, ":") {
			return "", errors.New("update package reference must not include a tag")
		}
	}
	return ref, nil
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
	if isOCIAssetURL(source) {
		return u.downloadOCIAsset(ctx, source, limit)
	}
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

func isOCIAssetURL(source string) bool {
	parsed, err := url.Parse(source)
	return err == nil && parsed.Scheme == ociAssetURLScheme
}

type ociAssetRef struct {
	Registry   string
	Repository string
	Tag        string
	AssetName  string
}

type ociManifest struct {
	Layers []ociDescriptor `json:"layers"`
	Blobs  []ociDescriptor `json:"blobs"`
}

type ociDescriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations"`
}

type registryAuthChallenge struct {
	Realm   string
	Service string
	Scope   string
}

func (u Updater) downloadOCIAsset(ctx context.Context, source string, limit int64) ([]byte, error) {
	ref, err := parseOCIAssetURL(source)
	if err != nil {
		return nil, err
	}
	manifestBytes, _, err := u.ociRequest(ctx, ref.Registry, "/v2/"+ref.Repository+"/manifests/"+url.PathEscape(ref.Tag), limit, true)
	if err != nil {
		return nil, fmt.Errorf("download OCI update manifest %s: %w", source, err)
	}
	var manifest ociManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("decode OCI update manifest %s: %w", source, err)
	}
	descriptor, ok := ociFindAsset(manifest, ref.AssetName)
	if !ok {
		return nil, fmt.Errorf("OCI update package %s:%s is missing %s", ref.Registry+"/"+ref.Repository, ref.Tag, ref.AssetName)
	}
	if descriptor.Size > limit {
		return nil, fmt.Errorf("update asset %s exceeds %s: manifest size is %s", source, formatByteLimit(limit), formatByteLimit(descriptor.Size))
	}
	blobPath := "/v2/" + ref.Repository + "/blobs/" + descriptor.Digest
	blobBytes, _, err := u.ociRequest(ctx, ref.Registry, blobPath, limit, false)
	if err != nil {
		return nil, fmt.Errorf("download OCI update asset %s: %w", source, err)
	}
	return blobBytes, nil
}

func parseOCIAssetURL(source string) (ociAssetRef, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return ociAssetRef{}, fmt.Errorf("parse OCI update asset URL: %w", err)
	}
	if parsed.Scheme != ociAssetURLScheme || parsed.Host == "" || parsed.RawQuery != "" {
		return ociAssetRef{}, errors.New("OCI update asset URL must use oci://registry/repository:tag#asset without query")
	}
	assetName := strings.TrimSpace(parsed.Fragment)
	if assetName == "" || strings.Contains(assetName, "/") || strings.Contains(assetName, "..") {
		return ociAssetRef{}, errors.New("OCI update asset URL must include a file asset fragment")
	}
	repositoryWithTag := strings.Trim(strings.TrimSpace(parsed.Path), "/")
	lastSlash := strings.LastIndex(repositoryWithTag, "/")
	lastColon := strings.LastIndex(repositoryWithTag, ":")
	if repositoryWithTag == "" || lastColon <= lastSlash || lastColon == len(repositoryWithTag)-1 {
		return ociAssetRef{}, errors.New("OCI update asset URL must include an exact tag")
	}
	repository := repositoryWithTag[:lastColon]
	tag := repositoryWithTag[lastColon+1:]
	if repository == "" || tag == "" || strings.Contains(tag, "/") {
		return ociAssetRef{}, errors.New("OCI update asset URL has an invalid repository or tag")
	}
	return ociAssetRef{
		Registry:   parsed.Host,
		Repository: repository,
		Tag:        tag,
		AssetName:  assetName,
	}, nil
}

func ociFindAsset(manifest ociManifest, assetName string) (ociDescriptor, bool) {
	descriptors := manifest.Layers
	if len(descriptors) == 0 {
		descriptors = manifest.Blobs
	}
	for _, descriptor := range descriptors {
		if descriptor.Annotations[ociLayerTitleAnnotation] == assetName {
			return descriptor, true
		}
	}
	return ociDescriptor{}, false
}

func (u Updater) ociRequest(ctx context.Context, registry, path string, limit int64, manifest bool) ([]byte, http.Header, error) {
	headers := http.Header{}
	if manifest {
		headers.Set("Accept", strings.Join([]string{
			ociImageManifestMediaType,
			ociDockerManifestMediaType,
			ociArtifactManifestMediaType,
		}, ", "))
	} else {
		headers.Set("Accept", "application/octet-stream")
	}
	if token := strings.TrimSpace(u.Token); token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	body, responseHeaders, statusCode, challenge, err := u.doOCIRequest(ctx, registry, path, headers, limit)
	if err != nil {
		return nil, nil, err
	}
	if statusCode != http.StatusUnauthorized || challenge.Realm == "" || headers.Get("Authorization") != "" {
		if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
			return nil, nil, fmt.Errorf("HTTP %d", statusCode)
		}
		return body, responseHeaders, nil
	}
	token, err := u.fetchRegistryBearerToken(ctx, challenge)
	if err != nil {
		return nil, nil, err
	}
	headers.Set("Authorization", "Bearer "+token)
	body, responseHeaders, statusCode, _, err = u.doOCIRequest(ctx, registry, path, headers, limit)
	if err != nil {
		return nil, nil, err
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return nil, nil, fmt.Errorf("HTTP %d", statusCode)
	}
	return body, responseHeaders, nil
}

func (u Updater) doOCIRequest(ctx context.Context, registry, path string, headers http.Header, limit int64) ([]byte, http.Header, int, registryAuthChallenge, error) {
	source := "https://" + registry + path
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, nil, 0, registryAuthChallenge{}, fmt.Errorf("build OCI update request: %w", err)
	}
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	client := u.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultDownloadTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, nil, 0, registryAuthChallenge{}, fmt.Errorf("download OCI update asset %s timed out before response headers; retry or pass a larger --timeout: %w", source, err)
		}
		return nil, nil, 0, registryAuthChallenge{}, fmt.Errorf("download OCI update asset %s: %w", source, err)
	}
	defer response.Body.Close()
	challenge := parseRegistryAuthChallenge(response.Header.Get("WWW-Authenticate"))
	if response.StatusCode == http.StatusUnauthorized {
		return nil, response.Header.Clone(), response.StatusCode, challenge, nil
	}
	if response.ContentLength > limit {
		return nil, response.Header.Clone(), response.StatusCode, challenge, fmt.Errorf("update asset %s exceeds %s: content length is %s", source, formatByteLimit(limit), formatByteLimit(response.ContentLength))
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, response.Header.Clone(), response.StatusCode, challenge, fmt.Errorf("read OCI update asset %s: %w", source, err)
	}
	if int64(len(contents)) > limit {
		return nil, response.Header.Clone(), response.StatusCode, challenge, fmt.Errorf("update asset %s exceeds %s", source, formatByteLimit(limit))
	}
	return contents, response.Header.Clone(), response.StatusCode, challenge, nil
}

func (u Updater) fetchRegistryBearerToken(ctx context.Context, challenge registryAuthChallenge) (string, error) {
	realm, err := url.Parse(challenge.Realm)
	if err != nil {
		return "", fmt.Errorf("parse OCI auth challenge: %w", err)
	}
	if realm.Scheme != "https" || realm.Host == "" {
		return "", errors.New("OCI auth challenge realm must be an absolute https URL")
	}
	query := realm.Query()
	if challenge.Service != "" {
		query.Set("service", challenge.Service)
	}
	if challenge.Scope != "" {
		query.Set("scope", challenge.Scope)
	}
	realm.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build OCI auth token request: %w", err)
	}
	client := u.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultDownloadTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("request OCI auth token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("request OCI auth token: HTTP %d", response.StatusCode)
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxChecksumBytes+1))
	if err != nil {
		return "", fmt.Errorf("read OCI auth token: %w", err)
	}
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(contents, &payload); err != nil {
		return "", fmt.Errorf("decode OCI auth token: %w", err)
	}
	token := strings.TrimSpace(payload.Token)
	if token == "" {
		token = strings.TrimSpace(payload.AccessToken)
	}
	if token == "" {
		return "", errors.New("OCI auth token response did not include a bearer token")
	}
	return token, nil
}

func parseRegistryAuthChallenge(header string) registryAuthChallenge {
	header = strings.TrimSpace(header)
	if header == "" || !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return registryAuthChallenge{}
	}
	values := map[string]string{}
	for _, part := range strings.Split(header[len("Bearer "):], ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if key != "" {
			values[key] = value
		}
	}
	return registryAuthChallenge{
		Realm:   values["realm"],
		Service: values["service"],
		Scope:   values["scope"],
	}
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
