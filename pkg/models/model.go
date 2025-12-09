package models

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Manifest represents the structure of the .nopsai.yaml manifest file.
type Manifest struct {
	Triggers []Trigger `yaml:"triggers" json:"triggers"`
}

// Trigger defines a rule for when a pipeline should be run.
type Trigger struct {
	On           string           `yaml:"on" json:"on"`
	Branches     []string         `yaml:"branches,omitempty" json:"branches,omitempty"`
	SkipBranches []string         `yaml:"skip_branches,omitempty" json:"skip_branches,omitempty"`
	Tags         []string         `yaml:"tags,omitempty" json:"tags,omitempty"`
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
	GetVariables() map[string]string

	// Type assertion helpers
	AsIncludeStep() (*IncludeStep, bool)
	AsTaskStep() (*TaskStep, bool)
	AsGoalStep() (*GoalStep, bool)
	AsScriptStep() (*ScriptStep, bool)
}

// BaseStep contains all common fields shared by all step types.
type BaseStep struct {
	Name             string            `yaml:"name" json:"name"`
	Image            string            `yaml:"image,omitempty" json:"image,omitempty"`
	Secrets          []string          `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	Volumes          []string          `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	DependsOn        []string          `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Condition        string            `yaml:"condition,omitempty" json:"condition,omitempty"`
	IgnoreFailure    bool              `yaml:"ignore_failure,omitempty" json:"ignore_failure,omitempty"`
	LlmOutputSharing *bool             `yaml:"llm_output_sharing,omitempty" json:"llm_output_sharing,omitempty"`
	Variables        map[string]string `yaml:"variables,omitempty" json:"variables,omitempty"`
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

// GetVariables returns the step's inline variables.
func (s *BaseStep) GetVariables() map[string]string { return s.Variables }

// Default type assertion implementations
func (s *BaseStep) AsIncludeStep() (*IncludeStep, bool) { return nil, false }
func (s *BaseStep) AsTaskStep() (*TaskStep, bool)       { return nil, false }
func (s *BaseStep) AsGoalStep() (*GoalStep, bool)       { return nil, false }
func (s *BaseStep) AsScriptStep() (*ScriptStep, bool)   { return nil, false }

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

	stepName := "unknown"
	if name, ok := raw["name"].(string); ok {
		stepName = name
	}

	if modes == 0 {
		return fmt.Errorf("step '%s' must contain one of 'include', 'tasks', 'goal', or 'script'", stepName)
	}
	if modes > 1 {
		return fmt.Errorf("step '%s' cannot mix 'include', 'tasks', 'goal', or 'script'", stepName)
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

	var stepName string
	if rawName, ok := raw["name"]; ok {
		_ = json.Unmarshal(rawName, &stepName)
	}
	if stepName == "" {
		stepName = "unknown"
	}

	if modes == 0 {
		return fmt.Errorf("step '%s' must contain one of 'include', 'tasks', 'goal', or 'script'", stepName)
	}
	if modes > 1 {
		return fmt.Errorf("step '%s' cannot mix 'include', 'tasks', 'goal', or 'script'", stepName)
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

func (ps *PipelineStep) SetVariables(env map[string]string) {
	if base := ps.baseStep(); base != nil {
		base.Variables = env
	}
}

type Pipeline struct {
	Name              string         `yaml:"name" json:"name"`
	Version           string         `yaml:"version,omitempty" json:"version,omitempty"`
	Description       string         `yaml:"description" json:"description"`
	ContainerImage    string         `yaml:"container_image" json:"container_image"`
	DisplayOptions    DisplayOptions `yaml:"display_options" json:"display_options"`
	WorkingDirectory  string         `yaml:"working_directory,omitempty" json:"working_directory,omitempty"`
	Variables         []string       `yaml:"variables" json:"variables"`
	Steps             []PipelineStep `yaml:"steps" json:"steps"`
	Timeout           string         `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	LlmContentSharing *bool          `yaml:"llm_content_sharing,omitempty" json:"llm_content_sharing,omitempty"`
	LlmOutputSharing  *bool          `yaml:"llm_output_sharing,omitempty" json:"llm_output_sharing,omitempty"`
	LlmContentIgnore  []string       `yaml:"llm_content_ignore,omitempty" json:"llm_content_ignore,omitempty"`
}

func (p *Pipeline) UnmarshalYAML(value *yaml.Node) error {
	type rawPipeline Pipeline
	aux := struct {
		rawPipeline `yaml:",inline"`
		Environment []string `yaml:"environment"`
	}{}
	if err := value.Decode(&aux); err != nil {
		return err
	}
	if len(aux.Environment) > 0 {
		return fmt.Errorf("the 'environment' key is deprecated; use 'variables'")
	}
	*p = Pipeline(aux.rawPipeline)
	return nil
}

func (p *Pipeline) UnmarshalJSON(data []byte) error {
	type rawPipeline Pipeline
	aux := struct {
		rawPipeline
		Environment []string `json:"environment"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.Environment) > 0 {
		return fmt.Errorf("the 'environment' key is deprecated; use 'variables'")
	}
	*p = Pipeline(aux.rawPipeline)
	return nil
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
	Name             string            `yaml:"name" json:"name"`
	Goal             string            `yaml:"goal" json:"goal"`
	Script           string            `yaml:"script,omitempty" json:"script,omitempty"`
	DependsOn        []string          `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	IgnoreFailure    bool              `yaml:"ignore_failure,omitempty" json:"ignore_failure,omitempty"`
	LlmOutputSharing *bool             `yaml:"llm_output_sharing,omitempty" json:"llm_output_sharing,omitempty"`
	Variables        map[string]string `yaml:"variables,omitempty" json:"variables,omitempty"`
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

// Action is the structured command returned by the LLM Agent to the Agent.
type Action struct {
	Type          string         `json:"type"`
	CommandAction *CommandAction `json:"command_action,omitempty"`
	FileAction    *FileAction    `json:"file_action,omitempty"`
	AnswerAction  *AnswerAction  `json:"answer_action,omitempty"`
}

const (
	ActionTypeExecuteCommand string = "EXECUTE_COMMAND"
	ActionTypeReplaceFile    string = "REPLACE_FILE"
	ActionTypeReturnAnswer   string = "RETURN_ANSWER"
)

// ActionResult is sent from the Agent back to the LLM Agent after an action is performed.
type ActionResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// Gemini API specific structures
type GeminiRequest struct {
	Contents []Content `json:"contents"`
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
}
