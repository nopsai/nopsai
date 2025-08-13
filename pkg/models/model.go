package models

// Pipeline represents the structure of a .nopsai.yml file.
type Pipeline struct {
	Name              string            `yaml:"name"`
	Description       string            `yaml:"description"`
	ContainerImage    string            `yaml:"container_image"`
	WorkingDirectory  string            `yaml:"working_directory,omitempty"`
	Environment       map[string]string `yaml:"environment"`
	Steps             []PipelineStep    `yaml:"steps"`
	Timeout           string            `yaml:"timeout,omitempty"`
	LlmContentSharing *bool             `yaml:"llm_content_sharing,omitempty"`
	LlmOutputSharing  *bool             `yaml:"llm_output_sharing,omitempty"`
}

// PipelineStep is a single step within a pipeline definition.
type PipelineStep struct {
	Name             string   `yaml:"name"`
	Goal             string   `yaml:"goal"`
	Script           string   `yaml:"script,omitempty"`
	DependsOn        []string `yaml:"depends_on,omitempty"` // Added for step dependencies
	IgnoreFailure    bool     `yaml:"ignore_failure,omitempty"`
	LlmOutputSharing *bool    `yaml:"llm_output_sharing,omitempty"`
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
