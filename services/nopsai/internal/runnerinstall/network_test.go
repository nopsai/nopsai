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

func TestExternalDispatcherAddressAdaptsUIServiceHostToDispatcherServiceHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://nopsai-ui.pre-nopsai.orb.local/v1/system/dispatcher/kubernetes-runner-bootstrap-command", nil)

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
	if got != "nopsai-dispatcher.pre-nopsai.orb.local:9090" {
		t.Fatalf("address = %q, want dispatcher service host", got)
	}
}

func TestExternalDispatcherAddressUsesRequestOverride(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://nopsai-ui.pre-nopsai.orb.local/v1/system/dispatcher/kubernetes-runner-bootstrap-command?dispatcher_grpc_address=nopsai-dispatcher.pre-nopsai.orb.local%3A9443", nil)

	got, adapted, warnings := ExternalDispatcherAddress(config.Config{
		DispatcherAddress:       "dispatcher:9090",
		DispatcherListenAddress: ":9090",
	}, req)

	if adapted {
		t.Fatal("adapted = true, want false for explicit override")
	}
	if len(warnings) == 0 {
		t.Fatal("warnings should explain explicit dispatcher override")
	}
	if got != "nopsai-dispatcher.pre-nopsai.orb.local:9443" {
		t.Fatalf("address = %q, want request override", got)
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
