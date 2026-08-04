package runnerinstall

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"nopsai/config"
)

func TestDockerInstallResourceNameIncludesPlatformOwnership(t *testing.T) {
	req := httptest.NewRequest("GET", "http://nopsai.example.com/v1/system/dispatcher/runner-compose?runner_id=shared-runner&runner_uid=sameuid", nil)
	cfgA := config.Config{
		MasterKey:               "platform-master-a",
		ServiceJWTSigningKey:    "service-secret-a",
		ServiceJWTIssuer:        "issuer",
		ServiceJWTAudience:      "audience",
		DispatcherAddress:       "dispatcher:9090",
		DispatcherListenAddress: ":9090",
		DispatcherTLSSecret:     "tls-secret",
		DispatcherTLSMode:       "mtls",
	}
	cfgB := cfgA
	cfgB.MasterKey = "platform-master-b"
	cfgB.ServiceJWTSigningKey = "service-secret-b"

	respA, err := BuildComposeResponse(cfgA, req)
	if err != nil {
		t.Fatalf("BuildComposeResponse(A) error = %v", err)
	}
	respB, err := BuildComposeResponse(cfgB, req)
	if err != nil {
		t.Fatalf("BuildComposeResponse(B) error = %v", err)
	}

	if respA.PlatformID == respB.PlatformID || respA.ResourceName == respB.ResourceName {
		t.Fatalf("platform/resource names should differ: A=%s/%s B=%s/%s", respA.PlatformID, respA.ResourceName, respB.PlatformID, respB.ResourceName)
	}
	for _, want := range []string{
		respA.ResourceName + ":",
		"RUNNER_ID: " + strconv.Quote("shared-runner-sameuid"),
		"RUNNER_NAME: " + strconv.Quote("shared-runner"),
		"RUNNER_CONTAINER_NAME: " + strconv.Quote(respA.ResourceName),
		PlatformIDEnv + ": " + strconv.Quote(respA.PlatformID),
		PlatformIDLabel + ": " + strconv.Quote(respA.PlatformID),
	} {
		if !strings.Contains(respA.Compose, want) {
			t.Fatalf("compose missing %q:\n%s", want, respA.Compose)
		}
	}
	if strings.Contains(respA.Compose, respB.PlatformID) || strings.Contains(respA.Compose, respB.ResourceName) {
		t.Fatalf("compose A should not target platform B resources:\n%s", respA.Compose)
	}
}
