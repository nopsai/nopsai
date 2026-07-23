package registryauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/moby/moby/api/pkg/authconfig"
	"github.com/moby/moby/api/types/registry"
)

const (
	CredentialKindDockerConfigJSON = "docker_config_json"
	DefaultDockerRegistryHost      = "index.docker.io"
	DockerConfigBase64Env          = "NOPSAI_REGISTRY_DOCKER_CONFIG_B64"
	DockerConfigPathEnv            = "NOPSAI_REGISTRY_DOCKER_CONFIG_PATH"
)

type DockerConfig struct {
	Auths map[string]DockerAuthConfig `json:"auths"`
}

type DockerAuthConfig struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Auth          string `json:"auth,omitempty"`
	Email         string `json:"email,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
	RegistryToken string `json:"registrytoken,omitempty"`
}

type DockerConfigResolver struct {
	dockerConfigJSON []byte
	allowedHosts     []string
}

func NewDockerConfigResolver(data []byte, allowedHosts []string) (DockerConfigResolver, []string, error) {
	if len(data) == 0 {
		return DockerConfigResolver{}, nil, nil
	}
	hosts, err := RegistryHosts(data)
	if err != nil {
		return DockerConfigResolver{}, nil, err
	}
	if len(allowedHosts) == 0 {
		allowedHosts = hosts
	}
	return DockerConfigResolver{
		dockerConfigJSON: append([]byte(nil), data...),
		allowedHosts:     append([]string(nil), allowedHosts...),
	}, hosts, nil
}

func DockerConfigResolverFromEnv(getenv func(string) string) (DockerConfigResolver, []string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if raw := strings.TrimSpace(getenv(DockerConfigBase64Env)); raw != "" {
		data, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return DockerConfigResolver{}, nil, fmt.Errorf("decode docker registry auth env %s: %w", DockerConfigBase64Env, err)
		}
		return NewDockerConfigResolver(data, nil)
	}
	path := strings.TrimSpace(getenv(DockerConfigPathEnv))
	if path == "" {
		if dockerConfigDir := strings.TrimSpace(getenv("DOCKER_CONFIG")); dockerConfigDir != "" {
			path = filepath.Join(dockerConfigDir, "config.json")
		}
	}
	if path == "" {
		return DockerConfigResolver{}, nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return DockerConfigResolver{}, nil, fmt.Errorf("read docker registry auth config %q: %w", path, err)
	}
	return NewDockerConfigResolver(data, nil)
}

func (r DockerConfigResolver) Configured() bool {
	return len(r.dockerConfigJSON) > 0
}

func (r DockerConfigResolver) Resolve(_ context.Context, imageName string) (string, error) {
	if len(r.dockerConfigJSON) == 0 || strings.TrimSpace(imageName) == "" {
		return "", nil
	}
	encoded, _, ok, err := EncodedAuthForImage(r.dockerConfigJSON, imageName, r.allowedHosts)
	if err != nil || !ok {
		return "", err
	}
	return encoded, nil
}

func ParseDockerConfigJSON(data []byte) (DockerConfig, error) {
	var cfg DockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DockerConfig{}, fmt.Errorf("parse docker config json: %w", err)
	}
	if len(cfg.Auths) == 0 {
		return DockerConfig{}, errors.New("docker config json must contain auths")
	}
	normalized := make(map[string]DockerAuthConfig, len(cfg.Auths))
	for rawHost, auth := range cfg.Auths {
		host := NormalizeRegistryHost(rawHost)
		if host == "" {
			return DockerConfig{}, errors.New("docker config contains an empty registry host")
		}
		auth = normalizeDockerAuth(auth)
		if !auth.hasCredentialMaterial() {
			return DockerConfig{}, fmt.Errorf("docker config registry %q has no auth material", rawHost)
		}
		normalized[rawHost] = auth
	}
	cfg.Auths = normalized
	return cfg, nil
}

func RegistryHosts(data []byte) ([]string, error) {
	cfg, err := ParseDockerConfigJSON(data)
	if err != nil {
		return nil, err
	}
	hosts := make([]string, 0, len(cfg.Auths))
	seen := map[string]struct{}{}
	for host := range cfg.Auths {
		normalized := NormalizeRegistryHost(host)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		hosts = append(hosts, normalized)
	}
	sort.Strings(hosts)
	return hosts, nil
}

func MergeDockerConfigs(configs ...[]byte) ([]byte, []string, error) {
	merged := DockerConfig{Auths: map[string]DockerAuthConfig{}}
	hostToRaw := map[string]string{}
	for _, data := range configs {
		cfg, err := ParseDockerConfigJSON(data)
		if err != nil {
			return nil, nil, err
		}
		for rawHost, auth := range cfg.Auths {
			host := NormalizeRegistryHost(rawHost)
			if existingRaw, exists := hostToRaw[host]; exists {
				return nil, nil, fmt.Errorf("registry host %q is present in both %q and %q", host, existingRaw, rawHost)
			}
			hostToRaw[host] = rawHost
			merged.Auths[rawHost] = auth
		}
	}
	hosts := make([]string, 0, len(hostToRaw))
	for host := range hostToRaw {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return data, hosts, nil
}

func FilterDockerConfigJSON(data []byte, allowedHosts []string) ([]byte, []string, error) {
	cfg, err := ParseDockerConfigJSON(data)
	if err != nil {
		return nil, nil, err
	}
	allowed := normalizedHostSet(allowedHosts)
	if len(allowed) == 0 {
		return nil, nil, errors.New("at least one registry host must be selected")
	}
	filtered := DockerConfig{Auths: map[string]DockerAuthConfig{}}
	matched := []string{}
	for rawHost, auth := range cfg.Auths {
		host := NormalizeRegistryHost(rawHost)
		if _, ok := allowed[host]; !ok {
			continue
		}
		filtered.Auths[rawHost] = auth
		matched = append(matched, host)
	}
	sort.Strings(matched)
	if len(filtered.Auths) == 0 {
		return nil, nil, errors.New("selected registry hosts are not present in docker config")
	}
	data, err = json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return data, matched, nil
}

func EncodedAuthForImage(data []byte, imageName string, allowedHosts []string) (string, string, bool, error) {
	cfg, err := ParseDockerConfigJSON(data)
	if err != nil {
		return "", "", false, err
	}
	imageHost := ImageRegistryHost(imageName)
	allowed := normalizedHostSet(allowedHosts)
	if len(allowed) > 0 {
		if _, ok := allowed[imageHost]; !ok {
			return "", imageHost, false, nil
		}
	}
	for _, candidate := range dockerHostAliases(imageHost) {
		for rawHost, auth := range cfg.Auths {
			if NormalizeRegistryHost(rawHost) != candidate {
				continue
			}
			encoded, err := encodeAuthConfig(candidate, auth)
			return encoded, imageHost, true, err
		}
	}
	return "", imageHost, false, nil
}

func ImageRegistryHost(imageName string) string {
	imageName = strings.TrimSpace(imageName)
	if imageName == "" {
		return ""
	}
	imageName = strings.TrimPrefix(imageName, "docker://")
	first, _, ok := strings.Cut(imageName, "/")
	if !ok || first == "" || (!strings.Contains(first, ".") && !strings.Contains(first, ":") && first != "localhost") {
		return DefaultDockerRegistryHost
	}
	return NormalizeRegistryHost(first)
}

func NormalizeRegistryHost(raw string) string {
	host := strings.ToLower(strings.TrimSpace(raw))
	host = strings.TrimSuffix(host, "/")
	if host == "" {
		return ""
	}
	if strings.Contains(host, "://") {
		if parsed, err := url.Parse(host); err == nil && parsed.Host != "" {
			host = parsed.Host
		}
	}
	host = strings.TrimSuffix(host, "/v1")
	host = strings.TrimSuffix(host, "/v2")
	host = strings.TrimSuffix(host, "/")
	if host == "docker.io" || host == "registry-1.docker.io" {
		return DefaultDockerRegistryHost
	}
	return host
}

func normalizedHostSet(hosts []string) map[string]struct{} {
	out := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		normalized := NormalizeRegistryHost(host)
		if normalized == "" {
			continue
		}
		out[normalized] = struct{}{}
	}
	return out
}

func dockerHostAliases(host string) []string {
	host = NormalizeRegistryHost(host)
	if host == DefaultDockerRegistryHost {
		return []string{DefaultDockerRegistryHost, "docker.io", "registry-1.docker.io"}
	}
	return []string{host}
}

func normalizeDockerAuth(auth DockerAuthConfig) DockerAuthConfig {
	auth.Username = strings.TrimSpace(auth.Username)
	auth.Password = strings.TrimSpace(auth.Password)
	auth.Auth = strings.TrimSpace(auth.Auth)
	auth.Email = strings.TrimSpace(auth.Email)
	auth.IdentityToken = strings.TrimSpace(auth.IdentityToken)
	auth.RegistryToken = strings.TrimSpace(auth.RegistryToken)
	if auth.Auth != "" && (auth.Username == "" || auth.Password == "") {
		decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
		if err == nil {
			username, password, ok := strings.Cut(string(decoded), ":")
			if ok {
				if auth.Username == "" {
					auth.Username = username
				}
				if auth.Password == "" {
					auth.Password = password
				}
			}
		}
	}
	return auth
}

func (auth DockerAuthConfig) hasCredentialMaterial() bool {
	return auth.Auth != "" ||
		auth.IdentityToken != "" ||
		auth.RegistryToken != "" ||
		(auth.Username != "" && auth.Password != "")
}

func encodeAuthConfig(serverAddress string, auth DockerAuthConfig) (string, error) {
	return authconfig.Encode(registry.AuthConfig{
		Username:      auth.Username,
		Password:      auth.Password,
		Auth:          auth.Auth,
		ServerAddress: serverAddress,
		IdentityToken: auth.IdentityToken,
		RegistryToken: auth.RegistryToken,
	})
}
