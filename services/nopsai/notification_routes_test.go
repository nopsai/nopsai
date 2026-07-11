package nopsai

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestParseGitOpsNotificationRoutesTeamRepoRootFile(t *testing.T) {
	routes, err := parseGitOpsNotificationRoutes(map[string]string{
		"notifications.yaml": `
enabled: true
recipients:
  include:
    teams: [same_team]
    users:
      - Alice@Example.com
  exclude:
    users:
      - noisy@example.com
events:
  failure: true
  success: false
  approval_requested: true
filters:
  branches:
    include: ["main", "release/*"]
delivery:
  channels: [mail]
  throttle:
    dedupe_window: 15m
    max_per_run: 3
`,
	}, "", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("parseGitOpsNotificationRoutes() error = %v", err)
	}
	route, ok := routes["team-1"]
	if !ok {
		t.Fatalf("missing team-1 route: %#v", routes)
	}
	if !route.definition.Enabled {
		t.Fatal("route enabled = false, want true")
	}
	if route.definition.Recipients.Include.Users[0] != "alice@example.com" {
		t.Fatalf("user recipient = %#v, want normalized email", route.definition.Recipients.Include.Users)
	}
	if !route.definition.Events["failure"] || route.definition.Events["success"] {
		t.Fatalf("events = %#v, want failure only", route.definition.Events)
	}
	if route.definition.Delivery.Throttle.DedupeWindow != "15m" || route.definition.Delivery.Throttle.MaxPerRun != 3 {
		t.Fatalf("throttle = %#v, want GitOps values", route.definition.Delivery.Throttle)
	}
}

func TestParseGitOpsNotificationRoutesConfigRepositoryTeamPath(t *testing.T) {
	routes, err := parseGitOpsConfigRepositoryNotificationRoutes(map[string]string{
		"config-repositories/teams/team-1/platform/notifications.yaml": `
enabled: true
events:
  waiting-approval: true
`,
	}, "config-repositories", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "")
	if err != nil {
		t.Fatalf("parseGitOpsConfigRepositoryNotificationRoutes() error = %v", err)
	}
	route, ok := routes["team-1/platform"]
	if !ok {
		t.Fatalf("missing colocated route: %#v", routes)
	}
	if route.sourcePath != "config-repositories/teams/team-1/platform/notifications.yaml" {
		t.Fatalf("source path = %q, want colocated notification path", route.sourcePath)
	}
	if !route.definition.Events["waiting_approval"] {
		t.Fatalf("events = %#v, want waiting_approval enabled", route.definition.Events)
	}
}

func TestParseConfigSyncPlanSkipsColocatedNotificationAsBinding(t *testing.T) {
	app := &App{}
	binding := models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	plan, err := app.parseConfigSyncPlan(binding, configSyncRepositoryContext{
		dirs: configRepositoryGitDirsForBasePath(""),
	}, configSyncRepositoryFiles{
		configRepositories: map[string]string{
			"config-repositories/teams/team-1/notifications.yaml": `
enabled: true
routes:
  - name: failures
    events:
      failure: true
`,
		},
	})
	if err != nil {
		t.Fatalf("parseConfigSyncPlan() error = %v", err)
	}
	if _, ok := plan.configRepositories["team/team-1/notifications"]; ok {
		t.Fatal("colocated notification policy was parsed as a config repository binding")
	}
	if _, ok := plan.notificationRoutes["team-1"]; !ok {
		t.Fatalf("missing colocated notification route: %#v", plan.notificationRoutes)
	}
}

func TestParseGitOpsNotificationRoutesMultiRoutePolicy(t *testing.T) {
	routes, err := parseGitOpsConfigRepositoryNotificationRoutes(map[string]string{
		"config-repositories/teams/team-1/notifications.yaml": `
enabled: true
routes:
  - name: ops failures
    events:
      failure: true
      success: false
    recipients:
      include:
        users: [ops@example.com]
  - name: approvals
    events:
      approval_requested: true
    recipients:
      include:
        teams: [team-1/release]
`,
	}, "config-repositories", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "")
	if err != nil {
		t.Fatalf("parseGitOpsNotificationRoutes() error = %v", err)
	}
	route := routes["team-1"]
	if len(route.definition.Routes) != 2 {
		t.Fatalf("routes = %#v, want two named routes", route.definition.Routes)
	}
	if route.definition.Routes[0].Name != "ops failures" || route.definition.Routes[1].Name != "approvals" {
		t.Fatalf("route names = %#v, want normalized names", route.definition.Routes)
	}
	if !route.definition.Routes[0].Events["failure"] || route.definition.Routes[0].Events["success"] {
		t.Fatalf("first route events = %#v, want failure only", route.definition.Routes[0].Events)
	}
	if route.definition.Recipients.Include.Users[0] != "ops@example.com" {
		t.Fatalf("legacy recipients = %#v, want first route compatibility fields", route.definition.Recipients)
	}
}

func TestParseGitOpsNotificationRoutesRejectsUnknownEvent(t *testing.T) {
	_, err := parseGitOpsConfigRepositoryNotificationRoutes(map[string]string{
		"config-repositories/teams/team-1/notifications.yaml": `
events:
  maybe: true
`,
	}, "config-repositories", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "")
	if err == nil || !strings.Contains(err.Error(), "unsupported notification event") {
		t.Fatalf("error = %v, want unsupported event", err)
	}
}

func TestNormalizeNotificationRouteDefaults(t *testing.T) {
	route, err := normalizeNotificationRouteDefinition(notificationRouteDefinitionFile{})
	if err != nil {
		t.Fatalf("normalizeNotificationRouteDefinition() error = %v", err)
	}
	if !route.Enabled {
		t.Fatal("default route should be enabled")
	}
	if !route.Events["failure"] || !route.Events["approval_requested"] || route.Events["success"] {
		t.Fatalf("default events = %#v", route.Events)
	}
	if len(route.Delivery.Channels) != 1 || route.Delivery.Channels[0] != "mail" {
		t.Fatalf("default delivery = %#v, want mail", route.Delivery)
	}
	if len(route.Routes) != 1 || route.Routes[0].Name != "default" {
		t.Fatalf("default routes = %#v, want a default route", route.Routes)
	}
}
