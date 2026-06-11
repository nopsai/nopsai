package resolver

import (
	"context"
	"strings"
	"testing"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog"
)

type fakeActionSession struct {
	action   *proto.Action
	requests []*proto.GetActionRequest
}

func (s *fakeActionSession) ProfileName() string         { return "unit" }
func (s *fakeActionSession) AgentProfileName() string    { return "devops-engineer" }
func (s *fakeActionSession) MCPEnabled() bool            { return false }
func (s *fakeActionSession) MCPProfiles() []string       { return nil }
func (s *fakeActionSession) MCPToolCount() int           { return 0 }
func (s *fakeActionSession) RequiresMCPToolCall() bool   { return false }
func (s *fakeActionSession) SuccessfulMCPToolCalls() int { return 0 }
func (s *fakeActionSession) GetAction(_ context.Context, req *proto.GetActionRequest) (*proto.Action, error) {
	s.requests = append(s.requests, req)
	return s.action, nil
}

func TestTaskActionResolverUsesDirectScriptWithoutLLM(t *testing.T) {
	logger := zerolog.Nop()
	pipeline := &models.Pipeline{}
	step := &models.PipelineStep{Step: &models.GoalStep{
		BaseStep: models.BaseStep{Name: "deploy"},
		Goal:     "ship release",
	}}
	task := &models.Task{
		Name:   "script",
		Script: "make deploy",
	}

	result := NewTaskActionResolver().Resolve(context.Background(), ActionRequest{
		Logger:     &logger,
		Pipeline:   pipeline,
		Step:       step,
		Task:       task,
		Context:    NewExecutionContext(),
		LLMEnabled: false,
	})

	if result.Failed {
		t.Fatal("script resolver failed; want direct action")
	}
	if result.Goal != "ship release" {
		t.Fatalf("goal = %q, want step goal", result.Goal)
	}
	if result.ActionSummary != "make deploy" {
		t.Fatalf("action summary = %q, want script", result.ActionSummary)
	}
	if cmd := result.Action.GetCommandAction(); cmd == nil || cmd.Command != "make deploy" {
		t.Fatalf("command action = %#v, want make deploy", cmd)
	}
}

func TestTaskActionResolverDisabledLLMReturnsFinalizableFailure(t *testing.T) {
	logger := zerolog.Nop()
	step := &models.PipelineStep{Step: &models.GoalStep{
		BaseStep: models.BaseStep{Name: "review"},
		Goal:     "review changes",
	}}
	task := &models.Task{Name: "goal"}

	result := NewTaskActionResolver().Resolve(context.Background(), ActionRequest{
		Logger:     &logger,
		Pipeline:   &models.Pipeline{},
		Step:       step,
		Task:       task,
		Context:    NewExecutionContext(),
		LLMEnabled: false,
	})

	if !result.Failed {
		t.Fatal("disabled LLM resolver succeeded; want failure")
	}
	if result.FinalizeStatus != "failure" || result.FinalizeExitCode != 1 {
		t.Fatalf("finalize result = %q/%d, want failure/1", result.FinalizeStatus, result.FinalizeExitCode)
	}
	if result.Goal != "review changes" {
		t.Fatalf("goal = %q, want step goal", result.Goal)
	}
}

func TestTaskActionResolverUsesSessionAndMasksPromptContent(t *testing.T) {
	logger := zerolog.Nop()
	session := &fakeActionSession{action: &proto.Action{
		Type:    "EXECUTE_COMMAND",
		Payload: &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: "make test"}},
	}}
	executionContext := NewExecutionContext()
	executionContext.SetValue("API_TOKEN", "secret-token-value", true)

	result := NewTaskActionResolver().Resolve(context.Background(), ActionRequest{
		Logger:       &logger,
		Pipeline:     &models.Pipeline{},
		Step:         &models.PipelineStep{Step: &models.GoalStep{Goal: "test changes"}},
		Task:         &models.Task{Name: "test"},
		Context:      executionContext,
		History:      "token=secret-token-value",
		Secrets:      map[string]string{"API_TOKEN": "secret-token-value"},
		LLMEnabled:   true,
		WorkspaceDir: "/workspace",
		SessionResolver: func(*models.Pipeline, *models.PipelineStep, *models.Task) (ActionSession, error) {
			return session, nil
		},
		DirectoryLister: func(*zerolog.Logger, string, []string, []string) map[string]string {
			return map[string]string{"README.md": "secret-token-value"}
		},
	})

	if result.Failed || result.ActionSummary != "make test" {
		t.Fatalf("result = %#v, want successful command action", result)
	}
	if len(session.requests) != 1 {
		t.Fatalf("session requests = %d, want 1", len(session.requests))
	}
	if strings.Contains(session.requests[0].GetHistory(), "secret-token-value") {
		t.Fatalf("history was not masked: %q", session.requests[0].GetHistory())
	}
	if strings.Contains(session.requests[0].GetDirectoryListing()["README.md"], "secret-token-value") {
		t.Fatalf("directory listing was not masked: %q", session.requests[0].GetDirectoryListing()["README.md"])
	}
}
