package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"nopsai/sharedtypes" // Import the new shared package
	"os"
	"os/exec"
	"strings"
)

func main() {
	// The agent runs in a simple loop: read a command, execute it, print result, repeat.
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var cmd sharedtypes.AgentCommand       // Use type from shared package
		var result sharedtypes.AgentExecResult // Use type from shared package

		if err := json.Unmarshal([]byte(line), &cmd); err != nil {
			result.Error = fmt.Sprintf("agent failed to unmarshal command: %v", err)
			printResult(result)
			continue
		}

		result.ActionName = cmd.ActionName

		bashCmd := exec.Command("/bin/bash", "-c", cmd.Script)

		var stdout, stderr strings.Builder
		bashCmd.Stdout = &stdout
		bashCmd.Stderr = &stderr

		err := bashCmd.Run()

		result.Stdout = stdout.String()
		result.Stderr = stderr.String()

		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitError.ExitCode()
			} else {
				result.ExitCode = -1
				result.Error = fmt.Sprintf("agent failed to run script: %v", err)
			}
		} else {
			result.ExitCode = 0
		}

		printResult(result)
	}
}

// printResult marshals the result to JSON and prints it to stdout
// for the Nopsai host to consume.
func printResult(result sharedtypes.AgentExecResult) {
	resultBytes, err := json.Marshal(result)
	if err != nil {
		fmt.Printf(`{"action_name": "%s", "error": "agent failed to marshal result: %v"}`+"\n", result.ActionName, err)
		return
	}
	fmt.Println(string(resultBytes))
}
