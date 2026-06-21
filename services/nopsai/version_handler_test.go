package nopsai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/pkg/buildinfo"
)

func TestVersionEndpointIsPublicAndNonSensitive(t *testing.T) {
	original := buildinfo.Current()
	buildinfo.Version = "2.7.0"
	buildinfo.Commit = "abc123"
	buildinfo.BuildDate = "2026-06-21T12:00:00Z"
	buildinfo.ReleaseManifestDigest = "sha256:release"
	t.Cleanup(func() {
		buildinfo.Version = original.Version
		buildinfo.Commit = original.Commit
		buildinfo.BuildDate = original.BuildDate
		buildinfo.ReleaseManifestDigest = original.ReleaseManifestDigest
	})

	recorder := httptest.NewRecorder()
	handleVersion(recorder, httptest.NewRequest(http.MethodGet, "/version", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("status/headers = %d %#v", recorder.Code, recorder.Header())
	}
	var response buildinfo.PublicInfo
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ProductVersion != "2.7.0" || response.APIVersion != "v1" || response.ReleaseManifestDigest != "sha256:release" || len(response.Capabilities) == 0 {
		t.Fatalf("response = %#v", response)
	}
	if !isPublicPath("/version") {
		t.Fatal("/version must bypass authentication and AAA")
	}
}

func TestVersionInfoMap(t *testing.T) {
	result := versionInfoMap()
	if result["productVersion"] == nil || result["apiVersion"] != "v1" {
		t.Fatalf("version map = %#v", result)
	}
}
