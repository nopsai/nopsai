package runnerinstall

import (
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"
)

func TestKubernetesPlatformIDIsStableAndResourceScoped(t *testing.T) {
	cfg := config.Config{
		MasterKey:            "platform-master-a",
		ServiceJWTSigningKey: "service-secret",
		ServiceJWTIssuer:     "issuer",
		ServiceJWTAudience:   "audience",
	}
	if got, again := KubernetesPlatformID(cfg), KubernetesPlatformID(cfg); got == "" || got != again {
		t.Fatalf("KubernetesPlatformID() = %q then %q, want stable non-empty ID", got, again)
	}
	other := cfg
	other.MasterKey = "platform-master-b"
	if KubernetesPlatformID(cfg) == KubernetesPlatformID(other) {
		t.Fatal("KubernetesPlatformID() should differ for different platform master keys")
	}
}

func TestPlatformIDHonorsConfiguredValue(t *testing.T) {
	cfg := config.Config{
		MasterKey:  "platform-master-a",
		PlatformID: " Platform Prod_A ",
	}
	if got := PlatformID(cfg); got != "platform-prod-a" {
		t.Fatalf("PlatformID() = %q, want normalized configured platform ID", got)
	}
}

func TestKubernetesManifestResourceNameIncludesPlatformOwnership(t *testing.T) {
	req := httptest.NewRequest("GET", "http://nopsai.example.com/v1/system/dispatcher/kubernetes-runner-manifest?runner_id=shared-runner&runner_uid=sameuid&namespace=shared-runners", nil)
	cfgA := config.Config{
		MasterKey:               "platform-master-a",
		ServiceJWTSigningKey:    "service-secret-a",
		ServiceJWTIssuer:        "issuer",
		ServiceJWTAudience:      "audience",
		DispatcherAddress:       "dispatcher:9090",
		DispatcherListenAddress: ":9090",
		DispatcherTLSSecret:     "tls-secret",
		DispatcherTLSServerName: "dispatcher.example.com",
		DispatcherTLSMode:       "mtls",
	}
	cfgB := cfgA
	cfgB.MasterKey = "platform-master-b"
	cfgB.ServiceJWTSigningKey = "service-secret-b"

	respA, err := BuildKubernetesManifestResponse(cfgA, req)
	if err != nil {
		t.Fatalf("BuildKubernetesManifestResponse(A) error = %v", err)
	}
	respB, err := BuildKubernetesManifestResponse(cfgB, req)
	if err != nil {
		t.Fatalf("BuildKubernetesManifestResponse(B) error = %v", err)
	}

	if respA.PlatformID == respB.PlatformID || respA.ResourceName == respB.ResourceName {
		t.Fatalf("platform/resource names should differ: A=%s/%s B=%s/%s", respA.PlatformID, respA.ResourceName, respB.PlatformID, respB.ResourceName)
	}
	for _, want := range []string{
		"name: " + respA.ResourceName,
		"app.kubernetes.io/instance: " + respA.ResourceName,
		KubernetesPlatformIDLabel + ": " + respA.PlatformID,
		KubernetesPlatformIDEnv + ": " + respA.PlatformID,
		"RUNNER_ID: shared-runner-sameuid",
		"RUNNER_NAME: shared-runner",
		"KUBERNETES_RUNNER_LABEL_SELECTOR: app.kubernetes.io/name=nopsai-k8s-runner,app.kubernetes.io/instance=" + respA.ResourceName + ",nopsai.io/runner-id=" + respA.RunnerID + ",nopsai.io/platform-id=" + respA.PlatformID,
	} {
		if !strings.Contains(respA.Manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, respA.Manifest)
		}
	}
	if strings.Contains(respA.Manifest, respB.PlatformID) || strings.Contains(respA.Manifest, respB.ResourceName) {
		t.Fatalf("manifest A should not target platform B resources:\n%s", respA.Manifest)
	}
}

func TestKubernetesManifestHonorsExplicitAllScopes(t *testing.T) {
	req := httptest.NewRequest("GET", "http://nopsai.example.com/v1/system/dispatcher/kubernetes-runner-manifest?runner_id=shared-runner&runner_uid=sameuid&runner_scopes=", nil)
	resp, err := BuildKubernetesManifestResponse(config.Config{
		RunnerScopes:            "dev,prod",
		ServiceJWTSigningKey:    "service-secret",
		ServiceJWTIssuer:        "issuer",
		ServiceJWTAudience:      "audience",
		DispatcherAddress:       "dispatcher:9090",
		DispatcherListenAddress: ":9090",
	}, req)
	if err != nil {
		t.Fatalf("BuildKubernetesManifestResponse() error = %v", err)
	}
	if resp.RunnerScopes != "" {
		t.Fatalf("runner scopes = %q, want explicit all-scopes value", resp.RunnerScopes)
	}
	if !strings.Contains(resp.Manifest, "RUNNER_SCOPES: \"\"") {
		t.Fatalf("manifest should keep RUNNER_SCOPES empty for all scopes:\n%s", resp.Manifest)
	}
}
