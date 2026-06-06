package runnerinstall

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nopsai/config"
)

func TestExternalDispatcherAddressAdaptsInternalHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://nopsai.example.com/v1/system/dispatcher/runner-compose", nil)

	got, adapted, warnings := ExternalDispatcherAddress(config.Config{
		DispatcherAddress:       "dispatcher:9090",
		DispatcherListenAddress: ":9090",
	}, req)

	if !adapted {
		t.Fatal("adapted = false, want true")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if got != "nopsai.example.com:9090" {
		t.Fatalf("address = %q, want request host with dispatcher port", got)
	}
}

func TestExternalDispatcherAddressKeepsExternalHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://nopsai.example.com/v1/system/dispatcher/runner-compose", nil)

	got, adapted, _ := ExternalDispatcherAddress(config.Config{
		DispatcherAddress: "dispatcher.internal.example.com:9443",
	}, req)

	if adapted {
		t.Fatal("adapted = true, want false")
	}
	if got != "dispatcher.internal.example.com:9443" {
		t.Fatalf("address = %q, want configured external host", got)
	}
}
