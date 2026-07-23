package registryauth

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/moby/moby/api/pkg/authconfig"
)

func TestRegistryHostsNormalizesDockerConfigHosts(t *testing.T) {
	data := []byte(`{"auths":{"https://index.docker.io/v1/":{"auth":"` + base64.StdEncoding.EncodeToString([]byte("u:p")) + `"},"GHCR.IO":{"username":"gh","password":"token"}}}`)

	hosts, err := RegistryHosts(data)
	if err != nil {
		t.Fatalf("RegistryHosts() error = %v", err)
	}
	got := strings.Join(hosts, ",")
	if got != "ghcr.io,index.docker.io" {
		t.Fatalf("hosts = %q, want normalized docker and ghcr hosts", got)
	}
}

func TestDockerConfigResolverUsesLocalConfigWithoutAPI(t *testing.T) {
	data := []byte(`{"auths":{"ghcr.io":{"username":"robot","password":"token"}}}`)

	resolver, hosts, err := NewDockerConfigResolver(data, nil)
	if err != nil {
		t.Fatalf("NewDockerConfigResolver() error = %v", err)
	}
	if !resolver.Configured() || strings.Join(hosts, ",") != "ghcr.io" {
		t.Fatalf("resolver configured=%v hosts=%v, want ghcr.io", resolver.Configured(), hosts)
	}

	encoded, err := resolver.Resolve(context.Background(), "ghcr.io/acme/app:1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if encoded == "" {
		t.Fatal("Resolve() returned empty auth for matching registry")
	}

	if encoded, err := resolver.Resolve(context.Background(), "registry.company.example/acme/app:1"); err != nil || encoded != "" {
		t.Fatalf("Resolve() for unconfigured host = (%q, %v), want empty auth", encoded, err)
	}
}

func TestDockerConfigResolverFromEnvUsesBase64Config(t *testing.T) {
	data := []byte(`{"auths":{"ghcr.io":{"username":"robot","password":"token"}}}`)
	env := map[string]string{
		DockerConfigBase64Env: base64.StdEncoding.EncodeToString(data),
	}

	resolver, hosts, err := DockerConfigResolverFromEnv(func(key string) string {
		return env[key]
	})
	if err != nil {
		t.Fatalf("DockerConfigResolverFromEnv() error = %v", err)
	}
	if !resolver.Configured() || strings.Join(hosts, ",") != "ghcr.io" {
		t.Fatalf("resolver configured=%v hosts=%v, want ghcr.io", resolver.Configured(), hosts)
	}
	encoded, err := resolver.Resolve(context.Background(), "ghcr.io/acme/app:1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if encoded == "" {
		t.Fatal("Resolve() returned empty auth for matching registry")
	}
}

func TestDockerConfigResolverEmptyConfigIsNoop(t *testing.T) {
	resolver, hosts, err := NewDockerConfigResolver(nil, nil)
	if err != nil {
		t.Fatalf("NewDockerConfigResolver() error = %v", err)
	}
	if resolver.Configured() || len(hosts) != 0 {
		t.Fatalf("empty resolver configured=%v hosts=%v, want noop", resolver.Configured(), hosts)
	}
	encoded, err := resolver.Resolve(context.Background(), "ghcr.io/acme/app:1")
	if err != nil || encoded != "" {
		t.Fatalf("Resolve() = (%q, %v), want empty auth", encoded, err)
	}
}

func TestMergeDockerConfigsRejectsDuplicateNormalizedHosts(t *testing.T) {
	first := []byte(`{"auths":{"docker.io":{"username":"a","password":"one"}}}`)
	second := []byte(`{"auths":{"registry-1.docker.io":{"username":"b","password":"two"}}}`)

	if _, _, err := MergeDockerConfigs(first, second); err == nil {
		t.Fatal("MergeDockerConfigs() succeeded with duplicate Docker Hub hosts")
	}
}

func TestFilterDockerConfigJSONOnlyReturnsSelectedHosts(t *testing.T) {
	data := []byte(`{"auths":{"ghcr.io":{"username":"gh","password":"token"},"registry.company.com":{"username":"svc","password":"secret"}}}`)

	filtered, matched, err := FilterDockerConfigJSON(data, []string{"ghcr.io"})
	if err != nil {
		t.Fatalf("FilterDockerConfigJSON() error = %v", err)
	}
	if strings.Join(matched, ",") != "ghcr.io" {
		t.Fatalf("matched = %#v", matched)
	}
	if strings.Contains(string(filtered), "registry.company.com") || !strings.Contains(string(filtered), "ghcr.io") {
		t.Fatalf("filtered config leaked or omitted hosts: %s", string(filtered))
	}
}

func TestEncodedAuthForImageMatchesRegistryHost(t *testing.T) {
	data := []byte(`{"auths":{"ghcr.io":{"username":"gh","password":"token"},"registry.company.com:5000":{"auth":"` + base64.StdEncoding.EncodeToString([]byte("svc:secret")) + `"}}}`)

	encoded, host, ok, err := EncodedAuthForImage(data, "registry.company.com:5000/team/build:1.2", []string{"registry.company.com:5000"})
	if err != nil {
		t.Fatalf("EncodedAuthForImage() error = %v", err)
	}
	if !ok || host != "registry.company.com:5000" {
		t.Fatalf("match = (%v, %q), want registry.company.com:5000", ok, host)
	}
	decoded, err := authconfig.Decode(encoded)
	if err != nil {
		t.Fatalf("decode auth: %v", err)
	}
	if decoded.Username != "svc" || decoded.Password != "secret" {
		t.Fatalf("auth = %#v, want decoded basic credentials", decoded)
	}

	if _, _, ok, err := EncodedAuthForImage(data, "ghcr.io/team/build:1.2", []string{"registry.company.com:5000"}); err != nil || ok {
		t.Fatalf("unselected host match = (%v, %v), want no match", ok, err)
	}
}

func TestImageRegistryHostDefaultsDockerHubImages(t *testing.T) {
	for _, image := range []string{"alpine:3.20", "library/alpine:3.20", "docker.io/library/alpine:3.20"} {
		if got := ImageRegistryHost(image); got != DefaultDockerRegistryHost {
			t.Fatalf("ImageRegistryHost(%q) = %q, want %q", image, got, DefaultDockerRegistryHost)
		}
	}
}
