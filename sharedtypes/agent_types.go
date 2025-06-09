package sharedtypes

// AgentCommand is the structured command Nopsai host sends to the agent's stdin.
type AgentCommand struct {
	ActionName string `json:"action_name"`
	Script     string `json:"script"`
}

// AgentExecResult is the structured result the agent prints to its stdout
// for the Nopsai host to parse.
type AgentExecResult struct {
	ActionName string `json:"action_name"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	Error      string `json:"error,omitempty"` // For errors within the agent itself (e.g., can't unmarshal command)
}
