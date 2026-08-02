package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest represents the structure of the .nopsai.yaml manifest file.
type Manifest struct {
	Provider      string    `yaml:"provider,omitempty" json:"provider,omitempty"`
	Team          string    `yaml:"team,omitempty" json:"team,omitempty"`
	TeamPath      string    `yaml:"team_path,omitempty" json:"team_path,omitempty"`
	WebhookSource string    `yaml:"webhook_source,omitempty" json:"webhook_source,omitempty"`
	Management    string    `yaml:"management,omitempty" json:"management,omitempty"`
	Triggers      []Trigger `yaml:"triggers" json:"triggers"`
}

// Trigger defines a rule for when a pipeline should be run.
type Trigger struct {
	On           string           `yaml:"on" json:"on"`
	Branches     []string         `yaml:"branches,omitempty" json:"branches,omitempty"`
	SkipBranches []string         `yaml:"skip_branches,omitempty" json:"skip_branches,omitempty"`
	Tags         []string         `yaml:"tags,omitempty" json:"tags,omitempty"`
	SkipRepos    []string         `yaml:"skip_repos,omitempty" json:"skip_repos,omitempty"`
	IncludePaths []string         `yaml:"include_paths,omitempty" json:"include_paths,omitempty"`
	ExcludePaths []string         `yaml:"exclude_paths,omitempty" json:"exclude_paths,omitempty"`
	Pipelines    []PipelineSource `yaml:"pipelines" json:"pipelines"`
	Scope        string           `yaml:"scope,omitempty" json:"scope,omitempty"`
}

// PipelineSource defines a single pipeline to be run from a local path or stored definition.
type PipelineSource struct {
	Path string `yaml:"path" json:"path"`
}

// UnmarshalYAML allows pipeline sources to be declared as simple strings (path).
func (p *PipelineSource) UnmarshalYAML(value *yaml.Node) error {
	*p = PipelineSource{}
	if value.Kind == yaml.ScalarNode {
		var path string
		if err := value.Decode(&path); err != nil {
			return err
		}
		if path == "" {
			return fmt.Errorf("pipeline path cannot be empty")
		}
		p.Path = path
		return nil
	}
	return fmt.Errorf("invalid pipeline source definition; only scalar paths are supported")
}

// Step is a common interface that all concrete step types must implement.
type Step interface {
	// Common methods for all step types
	GetName() string
	GetDependsOn() []string
	GetCondition() string
	GetSecrets() []string
	GetVolumes() []string
	GetImage() string
	GetIgnoreFailure() bool
	GetLlmOutputSharing() *bool
	GetAgentProfile() string
	GetLLMProfile() string
	GetMCPProfiles() []string
	GetRuntimePool() string
	GetVariables() map[string]string
	GetOutputs() []TaskOutput
	GetKnowledgeContext() []KnowledgeContextRef
	GetPolicyMergeMode() string

	// Type assertion helpers
	AsIncludeStep() (*IncludeStep, bool)
	AsTaskStep() (*TaskStep, bool)
	AsGoalStep() (*GoalStep, bool)
	AsScriptStep() (*ScriptStep, bool)
	AsApprovalStep() (*ApprovalStep, bool)
}

// BaseStep contains all common fields shared by all step types.
type BaseStep struct {
	Name             string                `yaml:"name" json:"name"`
	Image            string                `yaml:"image,omitempty" json:"image,omitempty"`
	Secrets          []string              `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	Volumes          []string              `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	DependsOn        []string              `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Condition        string                `yaml:"condition,omitempty" json:"condition,omitempty"`
	IgnoreFailure    bool                  `yaml:"ignore_failure,omitempty" json:"ignore_failure,omitempty"`
	LlmOutputSharing *bool                 `yaml:"llm_output_sharing,omitempty" json:"llm_output_sharing,omitempty"`
	AgentProfile     string                `yaml:"agent_profile,omitempty" json:"agent_profile,omitempty"`
	LLMProfile       string                `yaml:"llm_profile,omitempty" json:"llm_profile,omitempty"`
	MCPProfiles      []string              `yaml:"mcp_profiles,omitempty" json:"mcp_profiles,omitempty"`
	RuntimePool      string                `yaml:"runtime_pool,omitempty" json:"runtime_pool,omitempty"`
	Variables        map[string]string     `yaml:"variables,omitempty" json:"variables,omitempty"`
	Outputs          []TaskOutput          `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	KnowledgeContext []KnowledgeContextRef `yaml:"knowledge_context,omitempty" json:"knowledge_context,omitempty"`
	PolicyMergeMode  string                `yaml:"policy_merge_mode,omitempty" json:"policy_merge_mode,omitempty"`
}

// GetName returns the step's name.
func (s *BaseStep) GetName() string { return s.Name }

// GetDependsOn returns the step's dependencies.
func (s *BaseStep) GetDependsOn() []string { return s.DependsOn }

// GetCondition returns the step's condition.
func (s *BaseStep) GetCondition() string { return s.Condition }

// GetSecrets returns the step's secret requirements.
func (s *BaseStep) GetSecrets() []string { return s.Secrets }

// GetVolumes returns the step's volume mounts.
func (s *BaseStep) GetVolumes() []string { return s.Volumes }

// GetImage returns the step's container image.
func (s *BaseStep) GetImage() string { return s.Image }

// GetIgnoreFailure returns the step's failure tolerance.
func (s *BaseStep) GetIgnoreFailure() bool { return s.IgnoreFailure }

// GetLlmOutputSharing returns the step's LLM output sharing setting.
func (s *BaseStep) GetLlmOutputSharing() *bool { return s.LlmOutputSharing }

// GetAgentProfile returns the step's AI role/persona override.
func (s *BaseStep) GetAgentProfile() string { return s.AgentProfile }

// GetLLMProfile returns the step's LLM profile override.
func (s *BaseStep) GetLLMProfile() string { return s.LLMProfile }

// GetMCPProfiles returns the step's MCP profile defaults.
func (s *BaseStep) GetMCPProfiles() []string { return s.MCPProfiles }

// GetRuntimePool returns the step's Kubernetes runtime pool override.
func (s *BaseStep) GetRuntimePool() string { return s.RuntimePool }

// GetVariables returns the step's inline variables.
func (s *BaseStep) GetVariables() map[string]string { return s.Variables }

// GetOutputs returns outputs declared on legacy single-task step forms.
func (s *BaseStep) GetOutputs() []TaskOutput { return s.Outputs }

// GetKnowledgeContext returns the step's requested knowledge context.
func (s *BaseStep) GetKnowledgeContext() []KnowledgeContextRef { return s.KnowledgeContext }

// GetPolicyMergeMode returns the step's policy merge mode override.
func (s *BaseStep) GetPolicyMergeMode() string { return s.PolicyMergeMode }

// Default type assertion implementations
func (s *BaseStep) AsIncludeStep() (*IncludeStep, bool)   { return nil, false }
func (s *BaseStep) AsTaskStep() (*TaskStep, bool)         { return nil, false }
func (s *BaseStep) AsGoalStep() (*GoalStep, bool)         { return nil, false }
func (s *BaseStep) AsScriptStep() (*ScriptStep, bool)     { return nil, false }
func (s *BaseStep) AsApprovalStep() (*ApprovalStep, bool) { return nil, false }

// IncludeStep defines a step that includes a reusable step or pipeline.
type IncludeStep struct {
	BaseStep `yaml:",inline"`
	Include  string `yaml:"include" json:"include"`
	Sync     bool   `yaml:"sync,omitempty" json:"sync,omitempty"`
}

func (s *IncludeStep) UnmarshalYAML(value *yaml.Node) error {
	type rawInclude IncludeStep
	aux := struct {
		rawInclude `yaml:",inline"`
	}{
		rawInclude: rawInclude{},
	}
	if err := value.Decode(&aux); err != nil {
		return err
	}
	*s = IncludeStep(aux.rawInclude)
	return nil
}

func (s *IncludeStep) AsIncludeStep() (*IncludeStep, bool) { return s, true }

// TaskStep defines a step that contains one or more sub-tasks.
type TaskStep struct {
	BaseStep `yaml:",inline"`
	Tasks    []Task `yaml:"tasks" json:"tasks"`
}

func (s *TaskStep) AsTaskStep() (*TaskStep, bool) { return s, true }

// GoalStep defines a step driven by a natural language goal.
type GoalStep struct {
	BaseStep `yaml:",inline"`
	Goal     string `yaml:"goal" json:"goal"`
}

func (s *GoalStep) AsGoalStep() (*GoalStep, bool) { return s, true }

// ScriptStep defines a step driven by an explicit script.
type ScriptStep struct {
	BaseStep `yaml:",inline"`
	Script   string `yaml:"script" json:"script"`
}

func (s *ScriptStep) AsScriptStep() (*ScriptStep, bool) { return s, true }

// ApprovalDefinition configures a human approval checkpoint in a pipeline.
type ApprovalDefinition struct {
	Type              string   `yaml:"type,omitempty" json:"type,omitempty"`
	Teams             []string `yaml:"teams" json:"teams"`
	AllowSelfApproval bool     `yaml:"allow_self_approval,omitempty" json:"allow_self_approval,omitempty"`
}

// ApprovalStep defines a durable human approval checkpoint.
type ApprovalStep struct {
	BaseStep `yaml:",inline"`
	Approval ApprovalDefinition `yaml:"approval" json:"approval"`
}

func (s *ApprovalStep) AsApprovalStep() (*ApprovalStep, bool) { return s, true }

// PipelineStep is a wrapper struct that implements yaml.Unmarshaler
// to parse the YAML/JSON into the correct concrete Step type.
type PipelineStep struct {
	Step Step
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
// It inspects the YAML node keys to determine which concrete struct to parse into.
func (ps *PipelineStep) UnmarshalYAML(value *yaml.Node) error {
	// 1. Decode into a raw map to inspect keys
	var raw map[string]interface{}
	if err := value.Decode(&raw); err != nil {
		return err
	}

	// 2. Check for the presence of the mutually exclusive keys
	hasInclude := raw["include"] != nil
	hasTasks := raw["tasks"] != nil
	hasGoal := raw["goal"] != nil
	hasScript := raw["script"] != nil
	hasApproval := raw["approval"] != nil

	// 3. Enforce mutual exclusivity (Validation is now part of parsing)
	modes := 0
	if hasInclude {
		modes++
	}
	if hasTasks {
		modes++
	}
	if hasGoal {
		modes++
	}
	if hasScript {
		modes++
	}
	if hasApproval {
		modes++
	}

	stepName := "unknown"
	if name, ok := raw["name"].(string); ok {
		stepName = name
	}

	if modes == 0 {
		return fmt.Errorf("step '%s' must contain one of 'include', 'tasks', 'goal', 'script', or 'approval'", stepName)
	}
	if modes > 1 {
		return fmt.Errorf("step '%s' cannot mix 'include', 'tasks', 'goal', 'script', or 'approval'", stepName)
	}

	// 4. Decode into the correct concrete struct based on the key
	if hasInclude {
		var step IncludeStep
		if err := value.Decode(&step); err != nil {
			return err
		}
		ps.Step = &step
	} else if hasTasks {
		var step TaskStep
		if err := value.Decode(&step); err != nil {
			return err
		}
		ps.Step = &step
	} else if hasGoal {
		var step GoalStep
		if err := value.Decode(&step); err != nil {
			return err
		}
		ps.Step = &step
	} else if hasScript {
		var step ScriptStep
		if err := value.Decode(&step); err != nil {
			return err
		}
		ps.Step = &step
	} else if hasApproval {
		var step ApprovalStep
		if err := value.Decode(&step); err != nil {
			return err
		}
		ps.Step = &step
	}

	return nil
}

// UnmarshalJSON performs the same detection logic for JSON payloads.
func (ps *PipelineStep) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	hasInclude := raw["include"] != nil
	hasTasks := raw["tasks"] != nil
	hasGoal := raw["goal"] != nil
	hasScript := raw["script"] != nil
	hasApproval := raw["approval"] != nil

	modes := 0
	if hasInclude {
		modes++
	}
	if hasTasks {
		modes++
	}
	if hasGoal {
		modes++
	}
	if hasScript {
		modes++
	}
	if hasApproval {
		modes++
	}

	var stepName string
	if rawName, ok := raw["name"]; ok {
		_ = json.Unmarshal(rawName, &stepName)
	}
	if stepName == "" {
		stepName = "unknown"
	}

	if modes == 0 {
		return fmt.Errorf("step '%s' must contain one of 'include', 'tasks', 'goal', 'script', or 'approval'", stepName)
	}
	if modes > 1 {
		return fmt.Errorf("step '%s' cannot mix 'include', 'tasks', 'goal', 'script', or 'approval'", stepName)
	}

	switch {
	case hasInclude:
		var step IncludeStep
		if err := json.Unmarshal(data, &step); err != nil {
			return err
		}
		ps.Step = &step
	case hasTasks:
		var step TaskStep
		if err := json.Unmarshal(data, &step); err != nil {
			return err
		}
		ps.Step = &step
	case hasGoal:
		var step GoalStep
		if err := json.Unmarshal(data, &step); err != nil {
			return err
		}
		ps.Step = &step
	case hasScript:
		var step ScriptStep
		if err := json.Unmarshal(data, &step); err != nil {
			return err
		}
		ps.Step = &step
	case hasApproval:
		var step ApprovalStep
		if err := json.Unmarshal(data, &step); err != nil {
			return err
		}
		ps.Step = &step
	}

	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
// It unwraps the concrete Step interface, so the YAML output
// doesn't contain an extra "Step:" key, making it compatible
// with the custom UnmarshalYAML function.
func (ps PipelineStep) MarshalYAML() (interface{}, error) {
	if ps.Step == nil {
		return nil, nil
	}
	return ps.Step, nil
}

// MarshalJSON mirrors MarshalYAML for JSON serialization.
func (ps PipelineStep) MarshalJSON() ([]byte, error) {
	if ps.Step == nil {
		return []byte("null"), nil
	}
	return json.Marshal(ps.Step)
}

// Convenience getters to avoid repeated nil checks when interacting with pipeline steps.
func (ps PipelineStep) GetName() string {
	if ps.Step == nil {
		return ""
	}
	return ps.Step.GetName()
}

func (ps PipelineStep) GetDependsOn() []string {
	if ps.Step == nil {
		return nil
	}
	return ps.Step.GetDependsOn()
}

func (ps PipelineStep) GetCondition() string {
	if ps.Step == nil {
		return ""
	}
	return ps.Step.GetCondition()
}

func (ps PipelineStep) GetSecrets() []string {
	if ps.Step == nil {
		return nil
	}
	return ps.Step.GetSecrets()
}

func (ps PipelineStep) GetVolumes() []string {
	if ps.Step == nil {
		return nil
	}
	return ps.Step.GetVolumes()
}

func (ps PipelineStep) GetImage() string {
	if ps.Step == nil {
		return ""
	}
	return ps.Step.GetImage()
}

func (ps PipelineStep) GetIgnoreFailure() bool {
	if ps.Step == nil {
		return false
	}
	return ps.Step.GetIgnoreFailure()
}

func (ps PipelineStep) GetLlmOutputSharing() *bool {
	if ps.Step == nil {
		return nil
	}
	return ps.Step.GetLlmOutputSharing()
}

func (ps PipelineStep) GetAgentProfile() string {
	if ps.Step == nil {
		return ""
	}
	return ps.Step.GetAgentProfile()
}

func (ps PipelineStep) GetLLMProfile() string {
	if ps.Step == nil {
		return ""
	}
	return ps.Step.GetLLMProfile()
}

func (ps PipelineStep) GetMCPProfiles() []string {
	if ps.Step == nil {
		return nil
	}
	return ps.Step.GetMCPProfiles()
}

func (ps PipelineStep) GetRuntimePool() string {
	if ps.Step == nil {
		return ""
	}
	return ps.Step.GetRuntimePool()
}

func (ps PipelineStep) GetInclude() string {
	if include, ok := ps.AsIncludeStep(); ok {
		return include.Include
	}
	return ""
}

func (ps PipelineStep) GetSync() bool {
	if include, ok := ps.AsIncludeStep(); ok {
		return include.Sync
	}
	return false
}

func (ps PipelineStep) GetVariables() map[string]string {
	if base := ps.baseStep(); base != nil {
		return base.Variables
	}
	return nil
}

func (ps PipelineStep) GetOutputs() []TaskOutput {
	if base := ps.baseStep(); base != nil {
		return base.Outputs
	}
	return nil
}

func (ps PipelineStep) GetKnowledgeContext() []KnowledgeContextRef {
	if ps.Step == nil {
		return nil
	}
	return ps.Step.GetKnowledgeContext()
}

func (ps PipelineStep) GetPolicyMergeMode() string {
	if ps.Step == nil {
		return ""
	}
	return ps.Step.GetPolicyMergeMode()
}

func (ps PipelineStep) GetTasks() []Task {
	if taskStep, ok := ps.AsTaskStep(); ok {
		return taskStep.Tasks
	}
	return nil
}

func (ps PipelineStep) GetGoal() string {
	if goalStep, ok := ps.AsGoalStep(); ok {
		return goalStep.Goal
	}
	return ""
}

func (ps PipelineStep) GetScript() string {
	if scriptStep, ok := ps.AsScriptStep(); ok {
		return scriptStep.Script
	}
	return ""
}

func (ps PipelineStep) GetApproval() ApprovalDefinition {
	if approvalStep, ok := ps.AsApprovalStep(); ok {
		return approvalStep.Approval
	}
	return ApprovalDefinition{}
}

func (ps PipelineStep) AsIncludeStep() (*IncludeStep, bool) {
	if ps.Step == nil {
		return nil, false
	}
	return ps.Step.AsIncludeStep()
}

func (ps PipelineStep) AsTaskStep() (*TaskStep, bool) {
	if ps.Step == nil {
		return nil, false
	}
	return ps.Step.AsTaskStep()
}

func (ps PipelineStep) AsGoalStep() (*GoalStep, bool) {
	if ps.Step == nil {
		return nil, false
	}
	return ps.Step.AsGoalStep()
}

func (ps PipelineStep) AsScriptStep() (*ScriptStep, bool) {
	if ps.Step == nil {
		return nil, false
	}
	return ps.Step.AsScriptStep()
}

func (ps PipelineStep) AsApprovalStep() (*ApprovalStep, bool) {
	if ps.Step == nil {
		return nil, false
	}
	return ps.Step.AsApprovalStep()
}

func (ps *PipelineStep) baseStep() *BaseStep {
	if ps == nil || ps.Step == nil {
		return nil
	}
	switch concrete := ps.Step.(type) {
	case *IncludeStep:
		return &concrete.BaseStep
	case *TaskStep:
		return &concrete.BaseStep
	case *GoalStep:
		return &concrete.BaseStep
	case *ScriptStep:
		return &concrete.BaseStep
	case *ApprovalStep:
		return &concrete.BaseStep
	default:
		return nil
	}
}

func (ps *PipelineStep) SetName(name string) {
	if base := ps.baseStep(); base != nil {
		base.Name = name
	}
}

func (ps *PipelineStep) SetDependsOn(dependsOn []string) {
	if base := ps.baseStep(); base != nil {
		base.DependsOn = dependsOn
	}
}

func (ps *PipelineStep) SetCondition(condition string) {
	if base := ps.baseStep(); base != nil {
		base.Condition = condition
	}
}

func (ps *PipelineStep) SetSecrets(secrets []string) {
	if base := ps.baseStep(); base != nil {
		base.Secrets = secrets
	}
}

func (ps *PipelineStep) SetVolumes(volumes []string) {
	if base := ps.baseStep(); base != nil {
		base.Volumes = volumes
	}
}

func (ps *PipelineStep) SetImage(image string) {
	if base := ps.baseStep(); base != nil {
		base.Image = image
	}
}

func (ps *PipelineStep) SetIgnoreFailure(ignore bool) {
	if base := ps.baseStep(); base != nil {
		base.IgnoreFailure = ignore
	}
}

func (ps *PipelineStep) SetLlmOutputSharing(value *bool) {
	if base := ps.baseStep(); base != nil {
		base.LlmOutputSharing = value
	}
}

func (ps *PipelineStep) SetAgentProfile(value string) {
	if base := ps.baseStep(); base != nil {
		base.AgentProfile = value
	}
}

func (ps *PipelineStep) SetLLMProfile(value string) {
	if base := ps.baseStep(); base != nil {
		base.LLMProfile = value
	}
}

func (ps *PipelineStep) SetMCPProfiles(value []string) {
	if base := ps.baseStep(); base != nil {
		base.MCPProfiles = value
	}
}

func (ps *PipelineStep) SetVariables(variables map[string]string) {
	if base := ps.baseStep(); base != nil {
		base.Variables = variables
	}
}

func (ps *PipelineStep) SetOutputs(outputs []TaskOutput) {
	if base := ps.baseStep(); base != nil {
		base.Outputs = outputs
	}
}

func (ps *PipelineStep) SetKnowledgeContext(context []KnowledgeContextRef) {
	if base := ps.baseStep(); base != nil {
		base.KnowledgeContext = context
	}
}

func (ps *PipelineStep) SetPolicyMergeMode(value string) {
	if base := ps.baseStep(); base != nil {
		base.PolicyMergeMode = value
	}
}

type KnowledgeContextRef struct {
	Kind     string `yaml:"kind" json:"kind"`
	Ref      string `yaml:"ref,omitempty" json:"ref,omitempty"`
	Path     string `yaml:"path,omitempty" json:"path,omitempty"`
	Required bool   `yaml:"required,omitempty" json:"required,omitempty"`
}

type KnowledgeContextSnapshot struct {
	ID                    string    `json:"id,omitempty"`
	KnowledgeContextID    string    `json:"knowledge_context_id,omitempty"`
	Kind                  string    `json:"kind"`
	Team                  string    `json:"team,omitempty"`
	Name                  string    `json:"name,omitempty"`
	Description           string    `json:"description,omitempty"`
	Ref                   string    `json:"ref,omitempty"`
	Path                  string    `json:"path,omitempty"`
	Required              bool      `json:"required"`
	Source                string    `json:"source"`
	Content               string    `json:"content"`
	ConfigSourcePath      string    `json:"config_source_path,omitempty"`
	ConfigSourceCommitSHA string    `json:"config_source_commit_sha,omitempty"`
	ResolvedAt            time.Time `json:"resolved_at,omitempty"`
}

type PipelineOutput struct {
	LLMProfile string               `yaml:"llm_profile,omitempty" json:"llm_profile,omitempty"`
	Items      []PipelineOutputItem `yaml:"items,omitempty" json:"items,omitempty"`
}

type PipelineOutputItem struct {
	Name       string                `yaml:"name" json:"name"`
	Type       string                `yaml:"type" json:"type"`
	When       string                `yaml:"when,omitempty" json:"when,omitempty"`
	Prompt     string                `yaml:"prompt" json:"prompt"`
	LLMProfile string                `yaml:"llm_profile,omitempty" json:"llm_profile,omitempty"`
	Dashboard  DashboardOutputTarget `yaml:"dashboard,omitempty" json:"dashboard,omitempty"`
}

type DashboardOutputTarget struct {
	Ref      string `yaml:"ref,omitempty" json:"ref,omitempty"`
	Section  string `yaml:"section,omitempty" json:"section,omitempty"`
	EntryKey string `yaml:"entry_key,omitempty" json:"entry_key,omitempty"`
	Mode     string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Preset   string `yaml:"preset,omitempty" json:"preset,omitempty"`
	TTL      string `yaml:"ttl,omitempty" json:"ttl,omitempty"`
}

const RuntimeOutputsMountPath = "/nopsai/outputs"

// TaskOutput declares a file-based runtime value produced by a task.
type TaskOutput struct {
	Name      string `yaml:"name" json:"name"`
	Sensitive bool   `yaml:"sensitive,omitempty" json:"sensitive,omitempty"`
}

func (o *TaskOutput) UnmarshalYAML(value *yaml.Node) error {
	*o = TaskOutput{}
	if value.Kind == yaml.ScalarNode {
		var name string
		if err := value.Decode(&name); err != nil {
			return err
		}
		o.Name = name
		return nil
	}
	type rawOutput TaskOutput
	var parsed rawOutput
	if err := value.Decode(&parsed); err != nil {
		return err
	}
	*o = TaskOutput(parsed)
	return nil
}

func (o *TaskOutput) UnmarshalJSON(data []byte) error {
	*o = TaskOutput{}
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		o.Name = name
		return nil
	}
	type rawOutput TaskOutput
	var parsed rawOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*o = TaskOutput(parsed)
	return nil
}

type Pipeline struct {
	Name              string                `yaml:"name" json:"name"`
	Version           string                `yaml:"version,omitempty" json:"version,omitempty"`
	Description       string                `yaml:"description" json:"description"`
	ContainerImage    string                `yaml:"container_image" json:"container_image"`
	DisplayOptions    DisplayOptions        `yaml:"display_options" json:"display_options"`
	WorkingDirectory  string                `yaml:"working_directory,omitempty" json:"working_directory,omitempty"`
	Variables         []string              `yaml:"variables" json:"variables"`
	Steps             []PipelineStep        `yaml:"steps" json:"steps"`
	Timeout           string                `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	LLMEnabled        *bool                 `yaml:"llm_enabled,omitempty" json:"llm_enabled,omitempty"`
	AgentProfile      string                `yaml:"agent_profile,omitempty" json:"agent_profile,omitempty"`
	LLMProfile        string                `yaml:"llm_profile,omitempty" json:"llm_profile,omitempty"`
	MCPProfiles       []string              `yaml:"mcp_profiles,omitempty" json:"mcp_profiles,omitempty"`
	RuntimePool       string                `yaml:"runtime_pool,omitempty" json:"runtime_pool,omitempty"`
	AffinityEnabled   *bool                 `yaml:"affinity_enabled,omitempty" json:"affinity_enabled,omitempty"`
	KnowledgeContext  []KnowledgeContextRef `yaml:"knowledge_context,omitempty" json:"knowledge_context,omitempty"`
	PolicyMergeMode   string                `yaml:"policy_merge_mode,omitempty" json:"policy_merge_mode,omitempty"`
	Output            PipelineOutput        `yaml:"output,omitempty" json:"output,omitempty"`
	LlmContentSharing *bool                 `yaml:"llm_content_sharing,omitempty" json:"llm_content_sharing,omitempty"`
	LlmOutputSharing  *bool                 `yaml:"llm_output_sharing,omitempty" json:"llm_output_sharing,omitempty"`
	LlmContentInclude []string              `yaml:"llm_content_include,omitempty" json:"llm_content_include,omitempty"`
	LlmContentIgnore  []string              `yaml:"llm_content_ignore,omitempty" json:"llm_content_ignore,omitempty"`
}

func PipelineLLMEnabled(pipeline *Pipeline) bool {
	if pipeline == nil {
		return true
	}
	if pipeline.LLMEnabled != nil {
		return *pipeline.LLMEnabled
	}
	return true
}

func PipelineRequiresLLMProfiles(pipeline *Pipeline) bool {
	if pipeline == nil || !PipelineLLMEnabled(pipeline) {
		return false
	}
	if len(pipeline.Output.Items) > 0 {
		return true
	}
	for _, step := range pipeline.Steps {
		if strings.TrimSpace(step.GetCondition()) != "" || strings.TrimSpace(step.GetGoal()) != "" {
			return true
		}
		for _, task := range step.GetTasks() {
			if strings.TrimSpace(task.Goal) != "" {
				return true
			}
		}
	}
	return false
}

func PipelineLLMContentSharing(pipeline *Pipeline) bool {
	if pipeline == nil || pipeline.LlmContentSharing == nil {
		return false
	}
	return *pipeline.LlmContentSharing
}

// DisplayOptions defines how the pipeline progress is displayed in integrations like GitHub.
type DisplayOptions struct {
	GitHubView string `yaml:"github_view,omitempty" json:"github_view,omitempty"`
}

// Task is an individual command or goal within a PipelineStep.
//
// **Validation Note**: A Task must define EITHER 'goal' OR 'script', but not both.
// This mutual exclusivity is enforced by the application-layer validation
// (e.g., in services/nopsai/main.go::validatePipeline), not at the YAML unmarshaling level.
type Task struct {
	Name             string                `yaml:"name" json:"name"`
	Goal             string                `yaml:"goal" json:"goal"`
	Script           string                `yaml:"script,omitempty" json:"script,omitempty"`
	DependsOn        []string              `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	IgnoreFailure    bool                  `yaml:"ignore_failure,omitempty" json:"ignore_failure,omitempty"`
	LlmOutputSharing *bool                 `yaml:"llm_output_sharing,omitempty" json:"llm_output_sharing,omitempty"`
	LLMProfile       string                `yaml:"llm_profile,omitempty" json:"llm_profile,omitempty"`
	MCPProfiles      []string              `yaml:"mcp_profiles,omitempty" json:"mcp_profiles,omitempty"`
	Variables        map[string]string     `yaml:"variables,omitempty" json:"variables,omitempty"`
	Outputs          []TaskOutput          `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	KnowledgeContext []KnowledgeContextRef `yaml:"knowledge_context,omitempty" json:"knowledge_context,omitempty"`
	PolicyMergeMode  string                `yaml:"policy_merge_mode,omitempty" json:"policy_merge_mode,omitempty"`
}

func (t *Task) UnmarshalYAML(value *yaml.Node) error {
	var raw map[string]interface{}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	if _, exists := raw["agent_profile"]; exists {
		taskName := "unknown"
		if name, ok := raw["name"].(string); ok && name != "" {
			taskName = name
		}
		return fmt.Errorf("task %q cannot define agent_profile; set agent_profile on the pipeline or step", taskName)
	}
	type rawTask Task
	var parsed rawTask
	if err := value.Decode(&parsed); err != nil {
		return err
	}
	*t = Task(parsed)
	return nil
}

func (t *Task) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if _, exists := raw["agent_profile"]; exists {
		taskName := "unknown"
		if rawName, ok := raw["name"]; ok {
			var name string
			if err := json.Unmarshal(rawName, &name); err == nil && name != "" {
				taskName = name
			}
		}
		return fmt.Errorf("task %q cannot define agent_profile; set agent_profile on the pipeline or step", taskName)
	}
	type rawTask Task
	var parsed rawTask
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*t = Task(parsed)
	return nil
}

// CommandAction defines a command to be executed in the shell.
type CommandAction struct {
	Command string `json:"command"`
}

// FileAction defines a file to be created or replaced.
type FileAction struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// AnswerAction defines a simple text response from the LLM.
type AnswerAction struct {
	Answer string `json:"answer"`
}

// MCPToolAction asks the runtime to call an approved external MCP tool.
type MCPToolAction struct {
	Server    string          `json:"server,omitempty"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// WorkspaceToolAction asks the agent to call a bounded NopsAI workspace tool.
type WorkspaceToolAction struct {
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// Action is the structured command returned by the LLM Agent to the Agent.
type Action struct {
	Type                string               `json:"type"`
	CommandAction       *CommandAction       `json:"command_action,omitempty"`
	FileAction          *FileAction          `json:"file_action,omitempty"`
	AnswerAction        *AnswerAction        `json:"answer_action,omitempty"`
	MCPToolAction       *MCPToolAction       `json:"mcp_tool_action,omitempty"`
	WorkspaceToolAction *WorkspaceToolAction `json:"workspace_tool_action,omitempty"`
}

const (
	ActionTypeExecuteCommand    string = "EXECUTE_COMMAND"
	ActionTypeReplaceFile       string = "REPLACE_FILE"
	ActionTypeReturnAnswer      string = "RETURN_ANSWER"
	ActionTypeCallMCPTool       string = "CALL_MCP_TOOL"
	ActionTypeCallWorkspaceTool string = "CALL_WORKSPACE_TOOL"
)

// ActionResult is sent from the Agent back to the LLM Agent after an action is performed.
type ActionResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// Gemini API specific structures
type GeminiRequest struct {
	Contents          []Content               `json:"contents"`
	SystemInstruction *Content                `json:"systemInstruction,omitempty"`
	GenerationConfig  *GeminiGenerationConfig `json:"generationConfig,omitempty"`
}
type GeminiGenerationConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
}
type Content struct {
	Parts []Part `json:"parts"`
}
type Part struct {
	Text string `json:"text"`
}
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []Part `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int64 `json:"promptTokenCount"`
		CandidatesTokenCount int64 `json:"candidatesTokenCount"`
		TotalTokenCount      int64 `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}
