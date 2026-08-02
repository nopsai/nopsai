package apicatalog

import (
	"reflect"
	"testing"

	"nopsai/internal/cli/apicatalog/internal/discovery"
)

func TestGeneratedCatalogMatchesEveryRegisteredServerRoute(t *testing.T) {
	registrations, err := discovery.Discover("../../..")
	if err != nil {
		t.Fatal(err)
	}
	routes := Routes()
	if len(routes) != len(registrations) {
		t.Fatalf("catalog has %d routes; server registers %d", len(routes), len(registrations))
	}
	if len(routes) < 250 {
		t.Fatalf("catalog unexpectedly small: %d", len(routes))
	}
	for index, registration := range registrations {
		if routes[index].Method != registration.Method || routes[index].Path != registration.Path {
			t.Fatalf("route %d = %s %s; registration = %s %s", index, routes[index].Method, routes[index].Path, registration.Method, registration.Path)
		}
	}
}

func TestRouteMetadataAndExpansion(t *testing.T) {
	route, ok := Find("get", "/v1/pipelines/{pipelineName...}")
	if !ok {
		t.Fatal("pipeline route missing")
	}
	if route.Domain != "pipelines" || len(route.PathParameters) != 1 || !route.PathParameters[0].CatchAll {
		t.Fatalf("pipeline metadata = %#v", route)
	}
	path, err := route.Expand(map[string]string{"pipelineName": "delivery/release candidate"})
	if err != nil || path != "/v1/pipelines/delivery/release%20candidate" {
		t.Fatalf("Expand = %q, %v", path, err)
	}
	if _, err := route.Expand(nil); err == nil {
		t.Fatal("missing path parameter succeeded")
	}
	if _, err := route.Expand(map[string]string{"pipelineName": "release", "extra": "value"}); err == nil {
		t.Fatal("unknown path parameter succeeded")
	}
	if _, err := route.Expand(map[string]string{"pipelineName": "delivery//release"}); err == nil {
		t.Fatal("empty catch-all segment succeeded")
	}

	userRoute, ok := Find("PUT", "/v1/admin/users/{userID}")
	if !ok {
		t.Fatal("user route missing")
	}
	if _, err := userRoute.Expand(map[string]string{"userID": "parent/child"}); err == nil {
		t.Fatal("slash in single-segment parameter succeeded")
	}
	repositoryRoute, ok := Find("PUT", "/v1/repositories/{repoOwner}/{repoName}/variables/{variableName}")
	if !ok || len(repositoryRoute.PathParameters) != 3 {
		t.Fatalf("repository path parameters = %#v, found %v", repositoryRoute.PathParameters, ok)
	}
}

func TestCatalogClassifiesSpecialRoutesAndReturnsCopies(t *testing.T) {
	tests := []struct {
		method   string
		path     string
		verify   func(Route) bool
		property string
	}{
		{"GET", "/livez", func(route Route) bool { return route.Public && route.Domain == "platform" }, "public platform"},
		{"POST", "/v1/auth/login", func(route Route) bool { return route.Public }, "public"},
		{"POST", "/v1/internal/config/sync", func(route Route) bool { return route.Internal }, "internal"},
		{"GET", "/v1/system/logs/sources/{sourceID}/stream", func(route Route) bool { return route.Streaming }, "streaming"},
		{"GET", "/v1/runs/{runID}/outputs/{outputID}/download", func(route Route) bool { return route.Download }, "download"},
	}
	for _, test := range tests {
		route, ok := Find(test.method, test.path)
		if !ok || !test.verify(route) {
			t.Errorf("%s route classification = %#v, found %v", test.property, route, ok)
		}
	}

	routes := Routes()
	original := Routes()
	routes[0].Path = "/changed"
	if len(routes[0].PathParameters) > 0 {
		routes[0].PathParameters[0].Name = "changed"
	}
	if reflect.DeepEqual(routes, original) {
		t.Fatal("test did not mutate catalog copy")
	}
	again := Routes()
	if !reflect.DeepEqual(again, original) {
		t.Fatal("Routes exposed generated catalog storage")
	}
	if len(Domains()) < 25 {
		t.Fatalf("too few domains: %#v", Domains())
	}
}
