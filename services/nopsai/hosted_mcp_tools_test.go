package nopsai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nopsai/config"
	"nopsai/pkg/proto"
	"nopsai/services/aaa/pkg/model"
)

func TestHostedMCPToolsAreFilteredByAAA(t *testing.T) {
	allowPipelineList := stubAAAAuthorizer{
		checkFn: func(_ context.Context, _ model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
			allowed := action == "pipeline.list" && resource.Type == "pipeline" && resource.ID == "*"
			return model.Decision{Allowed: allowed}, nil
		},
	}
	app := &App{aaaLocal: allowPipelineList}

	tools := app.hostedMCPToolsForSubject(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"})
	if len(tools) != 3 {
		t.Fatalf("tools len = %d, want 3: %#v", len(tools), tools)
	}
	for _, tool := range tools {
		if tool.AuthenticatedOnly {
			t.Fatalf("unexpected authenticated-only tool escaped filter: %#v", tool)
		}
		if tool.Action != "pipeline.list" || tool.Resource.Type != "pipeline" || tool.Resource.ID != "*" {
			t.Fatalf("unexpected tool escaped filter: %#v", tool)
		}
	}

	enabled := true
	app = &App{
		cfg:      &config.Config{Assistant: config.AssistantConfig{Features: config.AssistantFeaturesConfig{ActionExecution: &enabled}}},
		aaaLocal: allowPipelineList,
	}
	tools = app.hostedMCPToolsForSubject(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"})
	authenticatedBridgeFound := false
	for _, tool := range tools {
		if tool.Name == "nopsai.call_api" && tool.AuthenticatedOnly {
			authenticatedBridgeFound = true
		}
	}
	if !authenticatedBridgeFound {
		t.Fatal("authenticated API bridge tool missing")
	}
}

func TestHostedMCPEmptyCurlDefaultsToToolsList(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp", nil)
	decoded, err := decodeHostedMCPRequest(req)
	if err != nil {
		t.Fatalf("decodeHostedMCPRequest() error = %v", err)
	}
	if decoded.JSONRPC != "2.0" || decoded.Method != "tools/list" {
		t.Fatalf("decoded request = %#v, want tools/list default", decoded)
	}
}

func TestHostedMCPDecodeRejectsTrailingPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"tools/list"} {}`))
	if _, err := decodeHostedMCPRequest(req); err == nil {
		t.Fatal("decodeHostedMCPRequest() should reject trailing JSON")
	}
}

func TestHostedMCPAPIBridgeChecksRoutePermissionForCurrentSubject(t *testing.T) {
	subject := model.Subject{Type: model.SubjectTypeUser, Sub: "viewer@example.com"}
	var checkedSubject model.Subject
	var checkedAction string
	var checkedResource model.ResourceRef
	app := &App{aaaLocal: stubAAAAuthorizer{
		checkFn: func(_ context.Context, seen model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
			checkedSubject = seen
			checkedAction = action
			checkedResource = resource
			return model.Decision{Allowed: seen.Sub == subject.Sub && action == "system.read" && resource.Type == "system" && resource.ID == "config"}, nil
		},
	}}

	err := app.authorizeHostedMCPAPICall(context.Background(), subject, map[string]any{
		"method": "GET",
		"path":   "/v1/system/config",
	})
	if err != nil {
		t.Fatalf("authorizeHostedMCPAPICall() error = %v", err)
	}
	if checkedSubject.Sub != subject.Sub || checkedAction != "system.read" || checkedResource.Type != "system" || checkedResource.ID != "config" {
		t.Fatalf("check = subject %#v action %q resource %#v", checkedSubject, checkedAction, checkedResource)
	}

	err = app.authorizeHostedMCPAPICall(context.Background(), subject, map[string]any{
		"method": "GET",
		"path":   "/v1/system/mcp/servers",
	})
	if err == nil {
		t.Fatal("authorizeHostedMCPAPICall() allowed route without permission")
	}
}

func TestHostedMCPAPIBridgeRequiresConfirmationForMutations(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("system.update")}

	result, err := app.hostedMCPCallAPI(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, map[string]any{
		"method": "POST",
		"path":   "/v1/system/config/sync",
	})
	if err != nil {
		t.Fatalf("hostedMCPCallAPI() error = %v", err)
	}
	if result["requires_confirmation"] != true || result["applied"] != false {
		t.Fatalf("confirmation result = %#v", result)
	}
}

func TestHostedMCPAPIBridgeBlocksSensitiveReadsByDefault(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("secret.read_value")}

	err := app.authorizeHostedMCPAPICall(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, map[string]any{
		"method": "GET",
		"path":   "/v1/secrets/prod-token",
	})
	if err == nil {
		t.Fatal("authorizeHostedMCPAPICall() allowed plaintext secret read by default")
	}
	if !strings.Contains(err.Error(), "sensitive response blocked") {
		t.Fatalf("error = %v, want sensitive response block", err)
	}
}

func TestHostedMCPAPIBridgeRejectsPublicIngressRoutes(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("system.read")}

	err := app.authorizeHostedMCPAPICall(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, map[string]any{
		"method": "POST",
		"path":   "/v1/git/events",
	})
	if err == nil {
		t.Fatal("authorizeHostedMCPAPICall() allowed public ingress route")
	}
}

func TestHostedMCPFeatureCapabilitiesAreCurrentSubjectScoped(t *testing.T) {
	subject := model.Subject{Type: model.SubjectTypeUser, Sub: "viewer@example.com"}
	seenSubjects := []model.Subject{}
	app := &App{aaaLocal: stubAAAAuthorizer{
		checkFn: func(_ context.Context, seen model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
			seenSubjects = append(seenSubjects, seen)
			allowed := seen.Sub == subject.Sub &&
				((action == "system.read" && resource.Type == "system" && resource.ID == "mcp") ||
					(action == "pipeline.list" && resource.Type == "pipeline" && resource.ID == "*"))
			return model.Decision{Allowed: allowed}, nil
		},
	}}

	result, err := app.hostedMCPGetFeatureCapabilities(context.Background(), subject, map[string]any{
		"area":               "Pipeline authoring",
		"include_api_routes": false,
	})
	if err != nil {
		t.Fatalf("hostedMCPGetFeatureCapabilities() error = %v", err)
	}
	if len(seenSubjects) == 0 {
		t.Fatal("expected AAA checks")
	}
	for _, seen := range seenSubjects {
		if seen.Sub != subject.Sub {
			t.Fatalf("AAA check used subject %#v, want %#v", seen, subject)
		}
	}
	permissionModel, ok := result["permission_model"].(map[string]any)
	if !ok || permissionModel["mode"] != "current_authenticated_subject" {
		t.Fatalf("permission model = %#v", result["permission_model"])
	}
	areas, ok := result["areas"].([]map[string]any)
	if !ok || len(areas) != 1 {
		t.Fatalf("areas = %#v", result["areas"])
	}
	userAccess, ok := areas[0]["user_access"].(map[string]any)
	if !ok {
		t.Fatalf("user_access missing: %#v", areas[0])
	}
	if userAccess["tools"] != "partial" {
		t.Fatalf("tool access = %#v, want partial", userAccess["tools"])
	}
	checks, ok := userAccess["permission_checks"].([]map[string]any)
	if !ok || len(checks) == 0 {
		t.Fatalf("permission checks missing: %#v", userAccess["permission_checks"])
	}
	allowedPipelineList := false
	deniedPipelineRead := false
	for _, check := range checks {
		if check["action"] == "pipeline.list" && check["allowed"] == true {
			allowedPipelineList = true
		}
		if check["action"] == "pipeline.read" && check["allowed"] == false {
			deniedPipelineRead = true
		}
	}
	if !allowedPipelineList || !deniedPipelineRead {
		t.Fatalf("permission checks do not reflect scoped user access: %#v", checks)
	}
}

func TestHostedMCPToolCallChecksSpecificPipelineResource(t *testing.T) {
	var checked model.ResourceRef
	app := &App{aaaLocal: stubAAAAuthorizer{
		checkFn: func(_ context.Context, _ model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
			checked = resource
			return model.Decision{Allowed: action == "pipeline.read" && resource.Type == "pipeline" && resource.ID == "team/build"}, nil
		},
	}}
	tool := toolDef("nopsai.get_pipeline", "Read", "pipeline.read", "pipeline", "*", objectSchema(map[string]any{}))

	if err := app.authorizeHostedMCPToolCall(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, tool, map[string]any{"pipeline": "team/build"}); err != nil {
		t.Fatalf("authorizeHostedMCPToolCall() error = %v", err)
	}
	if checked.ID != "team/build" {
		t.Fatalf("checked resource = %#v, want pipeline team/build", checked)
	}

	err := app.authorizeHostedMCPToolCall(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, tool, map[string]any{"pipeline": "other/build"})
	if err == nil {
		t.Fatal("authorizeHostedMCPToolCall() allowed unexpected pipeline")
	}
}

func TestHostedMCPValidatePipelineReturnsStructuredValidation(t *testing.T) {
	result, err := hostedMCPValidatePipeline(map[string]any{"yaml": "name: bad pipeline\nsteps: []"})
	if err != nil {
		t.Fatalf("hostedMCPValidatePipeline() error = %v", err)
	}
	if result["valid"] != false {
		t.Fatalf("valid = %#v, want false", result["valid"])
	}
	if result["error"] == "" {
		t.Fatalf("validation error missing: %#v", result)
	}
}

func TestHostedMCPGeneratePipelineUsesGolangAWSECSTemplate(t *testing.T) {
	result := hostedMCPGeneratePipeline(map[string]any{
		"name": "deploy-api",
		"goal": "create steps to build, test, and deploy a Golang app to AWS ECS",
	})
	raw := assistantOutputString(result, "yaml")
	if result["template_id"] != "golang-aws-ecs" {
		t.Fatalf("template_id = %#v, want golang-aws-ecs", result["template_id"])
	}
	for _, want := range []string{
		"name: deploy-api",
		"name: go-test",
		"name: docker-build-push",
		"name: production-approval",
		"name: update-ecs",
		"AWS_ACCESS_KEY_ID",
		"ECS_SERVICE",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("generated yaml missing %q:\n%s", want, raw)
		}
	}
	if strings.Count(raw, "goal:") != 0 {
		t.Fatalf("domain template should use executable scripts, not generic LLM goals:\n%s", raw)
	}
	validation := hostedMCPValidatePipelineYAML(raw)
	if validation["valid"] != true {
		t.Fatalf("generated yaml should validate, got %#v\n%s", validation, raw)
	}
	if len(assistantStringSlice(result["required_variables"])) == 0 || len(assistantStringSlice(result["required_secrets"])) == 0 {
		t.Fatalf("required vars/secrets missing: %#v", result)
	}
}

func TestHostedMCPPipelineSearchMatchFieldsAndSnippet(t *testing.T) {
	fields := hostedMCPPipelineMatchFields(
		"deploy",
		"team-1/services/api",
		"release",
		"latest",
		"database",
		"group",
		"steps:\n  - name: deploy\n",
	)
	if len(fields) != 1 || fields[0] != "definition" {
		t.Fatalf("fields = %#v, want definition only", fields)
	}
	if !onlyPipelineDefinitionMatched(fields) {
		t.Fatalf("onlyPipelineDefinitionMatched(%#v) = false", fields)
	}
	snippet := hostedMCPSnippet("name: release\nsteps:\n  - name: deploy\n    script: ./deploy.sh\n", "deploy", 32)
	if snippet == "" || !strings.Contains(snippet, "deploy") {
		t.Fatalf("snippet missing: %q", snippet)
	}
}

func TestHostedMCPPipelineSearchTokenizesApprovalStepQuery(t *testing.T) {
	definition := "name: release\nsteps:\n  - name: production-gate\n    approval:\n      type: manual\n"
	fields := hostedMCPPipelineMatchFields(
		"approval step",
		"platform",
		"release",
		"latest",
		"git",
		"group",
		definition,
	)
	if len(fields) != 1 || fields[0] != "definition" {
		t.Fatalf("fields = %#v, want definition", fields)
	}
	snippet := hostedMCPSnippet(definition, "approval step", 48)
	if snippet == "" || !strings.Contains(snippet, "approval") {
		t.Fatalf("snippet = %q, want approval context", snippet)
	}
	patterns := hostedMCPSearchPatterns("approval step")
	if len(patterns) != 1 || patterns[0] != "%approval%" {
		t.Fatalf("patterns = %#v, want approval-only pattern", patterns)
	}
}

func TestHostedMCPFeatureCapabilitiesNormalizeNaturalLanguagePolicyQuery(t *testing.T) {
	area, query := hostedMCPNormalizeFeatureCapabilityFilters("", "do we have any policy to prevent showing envs?")
	if area != "secrets" || query != "" {
		t.Fatalf("filters = area %q query %q, want secrets with broad query", area, query)
	}

	area, query = hostedMCPNormalizeFeatureCapabilityFilters("", "What features can I use with the assistant right now?")
	if area != "" || query != "" {
		t.Fatalf("filters = area %q query %q, want broad capability catalog", area, query)
	}
}

func TestHostedMCPGetDispatcherStatusReturnsRunnerSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	app := &App{
		cfg: &config.Config{
			AAAAPIURL:          server.URL,
			NopsaiGitBotAPIURL: server.URL,
		},
		httpClient: server.Client(),
		dispatcher: &fakeDispatcherClient{status: &proto.DispatcherStatus{
			QueuedJobs: 2,
			Runners: []*proto.RunnerInfo{{
				RunnerId:          "runner-a",
				Capacity:          3,
				ActiveJobs:        1,
				InflightJobs:      1,
				LastHeartbeatUnix: time.Now().Unix(),
				AllowDispatch:     true,
				Metadata:          map[string]string{"runtime": "docker"},
			}},
		}},
	}

	result, err := app.hostedMCPGetDispatcherStatus(context.Background())
	if err != nil {
		t.Fatalf("hostedMCPGetDispatcherStatus() error = %v", err)
	}
	dispatcher, ok := result["dispatcher"].(map[string]any)
	if !ok || dispatcher["status"] != "ok" {
		t.Fatalf("dispatcher status = %#v", result["dispatcher"])
	}
	summary, ok := result["runner_summary"].(monitoringRunnerSummary)
	if !ok {
		t.Fatalf("runner summary missing: %#v", result["runner_summary"])
	}
	if summary.Total != 1 || summary.Online != 1 || summary.QueuedJobs != 2 || summary.Capacity != 3 {
		t.Fatalf("summary = %#v", summary)
	}
	runners, ok := result["runners"].([]monitoringRunnerStatus)
	if !ok || len(runners) != 1 || runners[0].RunnerID != "runner-a" {
		t.Fatalf("runners = %#v", result["runners"])
	}
}

func TestHostedMCPProposePipelineCreateReturnsGitOpsPlan(t *testing.T) {
	result, err := hostedMCPProposePipelineWrite(map[string]any{
		"path": "team-1/services/api",
		"yaml": "name: deploy\ncontainer_image: alpine:3.20\nsteps:\n  - name: plan\n    tasks:\n      - name: draft\n        goal: Draft deployment\n",
	}, "create")
	if err != nil {
		t.Fatalf("hostedMCPProposePipelineWrite() error = %v", err)
	}
	if result["applies"] != false || result["valid"] != true {
		t.Fatalf("write plan flags = %#v", result)
	}
	if result["pipeline_id"] != "team-1/services/api/deploy" {
		t.Fatalf("pipeline id = %#v", result["pipeline_id"])
	}
	gitops, ok := result["gitops"].(map[string]any)
	if !ok {
		t.Fatalf("gitops plan missing: %#v", result)
	}
	files, ok := gitops["files"].([]map[string]any)
	if !ok || len(files) != 1 {
		t.Fatalf("gitops files = %#v", gitops["files"])
	}
	if files[0]["path"] != "pipelines/team-1/services/api/deploy.yaml" {
		t.Fatalf("gitops path = %#v", files[0]["path"])
	}
}

func TestHostedMCPProposeScheduleCreateReturnsGitOpsPlan(t *testing.T) {
	app := &App{}
	result, err := app.hostedMCPProposeScheduleWrite(context.Background(), map[string]any{
		"path":            "team-1",
		"name":            "nightly",
		"pipeline":        "team-1/deploy",
		"cron_expression": "0 2 * * *",
		"timezone":        "UTC",
		"enabled":         true,
	}, "create")
	if err != nil {
		t.Fatalf("hostedMCPProposeScheduleWrite() error = %v", err)
	}
	if result["applies"] != false || result["valid"] != true {
		t.Fatalf("schedule plan flags = %#v", result)
	}
	gitops, ok := result["gitops"].(map[string]any)
	if !ok {
		t.Fatalf("gitops plan missing: %#v", result)
	}
	files, ok := gitops["files"].([]map[string]any)
	if !ok || len(files) != 1 {
		t.Fatalf("gitops files = %#v", gitops["files"])
	}
	if files[0]["path"] != "schedules/team-1/nightly.yaml" {
		t.Fatalf("gitops path = %#v", files[0]["path"])
	}
	if content, _ := files[0]["content"].(string); !strings.Contains(content, "pipeline: team-1/deploy") {
		t.Fatalf("gitops content = %q, want pipeline reference", content)
	}
}

func TestHostedMCPProposeKnowledgeContextCreateReturnsGitOpsPlan(t *testing.T) {
	app := &App{}
	result, err := app.hostedMCPProposeKnowledgeContext(context.Background(), map[string]any{
		"kind":        "runbook",
		"group":       "team-1",
		"name":        "release",
		"description": "Release runbook",
		"content":     "Ship carefully.",
	}, "create")
	if err != nil {
		t.Fatalf("hostedMCPProposeKnowledgeContext() error = %v", err)
	}
	if result["applies"] != false || result["valid"] != true || result["id"] != "runbook/team-1/release" {
		t.Fatalf("knowledge plan = %#v", result)
	}
	gitops, ok := result["gitops"].(map[string]any)
	if !ok {
		t.Fatalf("gitops plan missing: %#v", result)
	}
	files, ok := gitops["files"].([]map[string]any)
	if !ok || len(files) != 1 {
		t.Fatalf("gitops files = %#v", gitops["files"])
	}
	if files[0]["path"] != "knowledge/runbook/team-1/release.md" {
		t.Fatalf("gitops path = %#v", files[0]["path"])
	}
}

func TestHostedMCPHighImpactToolRequiresConfirmation(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("system.update")}
	result, err := app.executeHostedMCPTool(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "ops"}, "nopsai.delete_data_backup", map[string]any{
		"backup_id": "backup-1",
	})
	if err != nil {
		t.Fatalf("executeHostedMCPTool() error = %v", err)
	}
	if result["requires_confirmation"] != true || result["applied"] != false || result["high_impact"] != true {
		t.Fatalf("confirmation result = %#v", result)
	}
}

func TestHostedMCPAPIBridgeAllowsMonitoringDeferredRoutes(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline_run.list")}
	err := app.authorizeHostedMCPAPICall(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, map[string]any{
		"method": "GET",
		"path":   "/v1/monitoring/views",
	})
	if err != nil {
		t.Fatalf("authorizeHostedMCPAPICall() error = %v", err)
	}
}

func TestHostedMCPProposePipelineUpdateRejectsTargetNameMismatch(t *testing.T) {
	result, err := hostedMCPProposePipelineWrite(map[string]any{
		"pipeline": "team-1/expected",
		"yaml":     "name: actual\ncontainer_image: alpine:3.20\nsteps:\n  - name: plan\n    tasks:\n      - name: draft\n        goal: Draft deployment\n",
	}, "update")
	if err != nil {
		t.Fatalf("hostedMCPProposePipelineWrite() error = %v", err)
	}
	if result["valid"] != false {
		t.Fatalf("valid = %#v, want false", result["valid"])
	}
	validation, ok := result["validation"].(map[string]any)
	if !ok || validation["error"] == "" {
		t.Fatalf("validation error missing: %#v", result)
	}
}

func TestHostedMCPPipelineWriteAuthorizationUsesYAMLName(t *testing.T) {
	var checked model.ResourceRef
	app := &App{aaaLocal: stubAAAAuthorizer{
		checkFn: func(_ context.Context, _ model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
			checked = resource
			return model.Decision{Allowed: action == "pipeline.create" && resource.Type == "pipeline" && resource.ID == "team-1/deploy"}, nil
		},
	}}
	tool := toolDef("nopsai.propose_pipeline_create", "Plan", "pipeline.create", "pipeline", "*", objectSchema(map[string]any{}))

	err := app.authorizeHostedMCPToolCall(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, tool, map[string]any{
		"path": "team-1",
		"yaml": "name: deploy\ncontainer_image: alpine:3.20\nsteps:\n  - name: plan\n    tasks:\n      - name: draft\n        goal: Draft deployment\n",
	})
	if err != nil {
		t.Fatalf("authorizeHostedMCPToolCall() error = %v", err)
	}
	if checked.ID != "team-1/deploy" {
		t.Fatalf("checked resource = %#v, want team-1/deploy", checked)
	}
}

func TestHostedMCPGetPipelineKnowledgeContextReportsRepoLocalRefs(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.read")}
	result, err := app.hostedMCPGetPipelineKnowledgeContext(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, map[string]any{
		"yaml": "name: docs\ncontainer_image: alpine:3.20\nknowledge_context:\n  - kind: architecture\n    path: .nopsai/docs/backend.md\nsteps:\n  - name: plan\n    tasks:\n      - name: draft\n        goal: Draft deployment\n",
	})
	if err != nil {
		t.Fatalf("hostedMCPGetPipelineKnowledgeContext() error = %v", err)
	}
	if result["valid"] != true {
		t.Fatalf("valid = %#v", result["valid"])
	}
	if result["unresolved_count"] != 1 {
		t.Fatalf("unresolved_count = %#v, want 1: %#v", result["unresolved_count"], result)
	}
	unresolved, ok := result["unresolved"].([]map[string]any)
	if !ok || unresolved[0]["status"] != "repo_local" {
		t.Fatalf("unresolved = %#v", result["unresolved"])
	}
}

func TestHostedMCPProposeReusableStepCreateReturnsGitOpsPlan(t *testing.T) {
	result, err := hostedMCPProposeReusableStep(map[string]any{
		"path": "shared",
		"yaml": "name: checkout\nscript: git checkout \"$BRANCH\"\n",
	}, "create")
	if err != nil {
		t.Fatalf("hostedMCPProposeReusableStep() error = %v", err)
	}
	if result["valid"] != true || result["id"] != "shared/checkout" {
		t.Fatalf("reusable step plan = %#v", result)
	}
	gitops := result["gitops"].(map[string]any)
	files := gitops["files"].([]map[string]any)
	if files[0]["path"] != "steps/shared/checkout.yaml" {
		t.Fatalf("path = %#v, want steps/shared/checkout.yaml", files[0]["path"])
	}
}

func TestHostedMCPProposeReusableStepRejectsNameMismatch(t *testing.T) {
	result, err := hostedMCPProposeReusableStep(map[string]any{
		"step": "shared/expected",
		"yaml": "name: actual\nscript: echo ok\n",
	}, "update")
	if err != nil {
		t.Fatalf("hostedMCPProposeReusableStep() error = %v", err)
	}
	if result["valid"] != false || !strings.Contains(result["error"].(string), "must match") {
		t.Fatalf("mismatch result = %#v", result)
	}
}

func TestHostedMCPProposeScopedSecretGitOpsWriteUsesEncryptedValue(t *testing.T) {
	result, err := hostedMCPProposeScopedValueGitOps(map[string]any{
		"scope":           "team-1/dev",
		"repository":      "acme/api",
		"secret_name":     "DEPLOY_TOKEN",
		"encrypted_value": "encrypted-secret",
	}, "secrets", "write")
	if err != nil {
		t.Fatalf("hostedMCPProposeScopedValueGitOps() error = %v", err)
	}
	if result["valid"] != true {
		t.Fatalf("secret plan = %#v", result)
	}
	gitops := result["gitops"].(map[string]any)
	files := gitops["files"].([]map[string]any)
	if files[0]["path"] != "scopes/team-1/dev/scope.yaml" {
		t.Fatalf("path = %#v", files[0]["path"])
	}
	content := files[0]["content"].(string)
	if !strings.Contains(content, "secrets:") || !strings.Contains(content, "acme/api/DEPLOY_TOKEN: encrypted-secret") {
		t.Fatalf("content = %q", content)
	}
}

func TestHostedMCPProposeScopedVariableGitOpsDeleteUsesRemoveKey(t *testing.T) {
	result, err := hostedMCPProposeScopedValueGitOps(map[string]any{
		"scope":         "prod",
		"variable_name": "API_URL",
	}, "variables", "delete")
	if err != nil {
		t.Fatalf("hostedMCPProposeScopedValueGitOps() error = %v", err)
	}
	gitops := result["gitops"].(map[string]any)
	files := gitops["files"].([]map[string]any)
	removeKey, ok := files[0]["remove_key"].([]string)
	if !ok || len(removeKey) != 2 || removeKey[0] != "variables" || removeKey[1] != "API_URL" {
		t.Fatalf("remove_key = %#v", files[0]["remove_key"])
	}
}

func TestHostedMCPProposeCredentialGitOpsPendingMetadata(t *testing.T) {
	result, err := hostedMCPProposeCredentialGitOps(map[string]any{
		"reference":   "credential://system/llm/openai-primary",
		"kind":        "api_key",
		"description": "OpenAI production key",
	})
	if err != nil {
		t.Fatalf("hostedMCPProposeCredentialGitOps() error = %v", err)
	}
	if result["valid"] != true || result["id"] != "credential://system/llm/openai-primary" {
		t.Fatalf("credential plan = %#v", result)
	}
	gitops := result["gitops"].(map[string]any)
	files := gitops["files"].([]map[string]any)
	if files[0]["path"] != configRepositoryCredentialsPath {
		t.Fatalf("path = %#v", files[0]["path"])
	}
	if content := files[0]["content"].(string); !strings.Contains(content, "status: pending") {
		t.Fatalf("content = %q", content)
	}
}

func TestHostedMCPSetupBootstrapRequiresConfirmation(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("system.update")}
	result, err := app.executeHostedMCPTool(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "ops"}, "nopsai.bootstrap_first_install_setup", map[string]any{
		"profile": "team",
	})
	if err != nil {
		t.Fatalf("executeHostedMCPTool() error = %v", err)
	}
	if result["requires_confirmation"] != true || result["applied"] != false || result["high_impact"] != true {
		t.Fatalf("confirmation result = %#v", result)
	}
}

func TestHostedMCPRunnerBootstrapCommandBlocksSensitiveResponseByDefault(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("system.update")}
	result, err := app.executeHostedMCPTool(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "ops"}, "nopsai.generate_runner_bootstrap_command", map[string]any{
		"runner_id": "runner-1",
	})
	if err != nil {
		t.Fatalf("executeHostedMCPTool() error = %v", err)
	}
	if !strings.Contains(result["error"].(string), "sensitive response blocked") {
		t.Fatalf("result = %#v, want sensitive response block", result)
	}
}

func TestHostedMCPAuditRedactsSensitiveDedicatedTools(t *testing.T) {
	input := hostedMCPAuditInput("nopsai.write_secret_value", map[string]any{
		"name":  "TOKEN",
		"value": "super-secret",
	})
	if input["value"] != "[redacted]" {
		t.Fatalf("input = %#v, want redacted value", input)
	}

	output := hostedMCPAuditOutput("nopsai.propose_secret_gitops_write", map[string]any{
		"gitops": map[string]any{
			"files": []map[string]any{{"path": "scopes/prod/scope.yaml", "content": "secret"}},
		},
	})
	gitops := output["gitops"].(map[string]any)
	if gitops["files"] != "[redacted]" {
		t.Fatalf("output = %#v, want redacted files", output)
	}
}

func TestHostedMCPMonitoringAnalyticsPathUsesAliases(t *testing.T) {
	path := hostedMCPMonitoringAnalyticsPath("nopsai.get_monitoring_summary", map[string]any{
		"group_id":                   "42",
		"pipeline_path":              "platform",
		"llm_profile":                "standard",
		"step_name":                  "plan",
		"task_name":                  "summarize",
		"min_duration_seconds":       5,
		"include_sensitive_response": true,
	})
	if !strings.Contains(path, "groupId=42") || !strings.Contains(path, "pipelinePath=platform") || !strings.Contains(path, "llmProfile=standard") || !strings.Contains(path, "stepName=plan") || !strings.Contains(path, "taskName=summarize") || !strings.Contains(path, "minDurationSeconds=5") {
		t.Fatalf("path = %q", path)
	}
	if strings.Contains(path, "include_sensitive_response") {
		t.Fatalf("path leaked control argument: %q", path)
	}
}

func TestHostedMCPUIContextIsContextualOnly(t *testing.T) {
	result := hostedMCPUIContext(map[string]any{"area": "monitoring"})
	if result["applies"] != false || result["rendering"] != "intentionally_excluded_from_mcp_mutation" {
		t.Fatalf("ui context = %#v", result)
	}
	surfaces := result["surfaces"].([]map[string]any)
	if len(surfaces) != 1 || surfaces[0]["area"] != "monitoring" {
		t.Fatalf("surfaces = %#v", surfaces)
	}
}
