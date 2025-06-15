package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"nopsai/sharedtypes"
	"os"
	"os/exec"
	"strings"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var cmd sharedtypes.AgentCommand
		var result sharedtypes.AgentExecResult

		if err := json.Unmarshal([]byte(line), &cmd); err != nil {
			result.Error = fmt.Sprintf("agent failed to unmarshal command: %v", err)
			printResult(result)
			continue
		}

		result.StepName = cmd.StepName

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

func printResult(result sharedtypes.AgentExecResult) {
	resultBytes, err := json.Marshal(result)
	if err != nil {
		fmt.Printf(`{"step_name": "%s", "error": "agent failed to marshal result: %v"}`+"\n", result.StepName, err)
		return
	}
	fmt.Println(string(resultBytes))
}
