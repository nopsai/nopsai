package sharedtypes

type AgentCommand struct {
	StepName string `json:"step_name"`
	Script   string `json:"script"`
}

type AgentExecResult struct {
	StepName string `json:"step_name"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}
