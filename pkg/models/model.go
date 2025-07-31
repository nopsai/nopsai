package models

type Pipeline struct {
	Name           string            `yaml:"name"`
	Description    string            `yaml:"description"`
	ContainerImage string            `yaml:"container_image"`
	Environment    map[string]string `yaml:"environment"`
	Steps          []PipelineStep    `yaml:"steps"`
}

type PipelineStep struct {
	Name string `yaml:"name"`
	Goal string `yaml:"goal"`
}

type CommandAction struct {
	Command string `json:"command"`
}

type FileAction struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type AnswerAction struct {
	Answer string `json:"answer"`
}

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

type ActionResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}
