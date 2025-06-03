// executor/command.go
package executor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func ExecuteCommand(workingDir string, command string, verbose bool, args ...string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("command cannot be empty")
	}
	cmd := exec.Command(command, args...)
	effectiveDir := workingDir
	if effectiveDir == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			fmt.Println("  Warning: could not get current working directory, using default behavior for command execution path.")
		} else {
			effectiveDir = currentDir
		}
	}
	cmd.Dir = effectiveDir
	if verbose {
		if cmd.Dir != "" {
			fmt.Printf("  Executing in dir '%s': %s %s\n", cmd.Dir, command, strings.Join(args, " "))
		} else {
			fmt.Printf("  Executing: %s %s\n", command, strings.Join(args, " "))
		}
	}
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	err := cmd.Run()
	stdout := outb.String()
	stderr := errb.String()
	displayDir := cmd.Dir
	if displayDir == "" {
		displayDir = "current"
	}
	if err != nil {
		return stdout, fmt.Errorf("command execution failed in dir '%s': %w. Stderr: %s", displayDir, err, stderr)
	}
	if stderr != "" && verbose {
		fmt.Printf("  Stderr (non-fatal) from dir '%s':\n%s\n", displayDir, stderr)
	}
	return stdout, nil
}

func ExecuteDirectAction(actionName string, details string) (string, error) {
	fmt.Printf("  Simulating direct action: %s, Details: %s\n", actionName, details)
	return fmt.Sprintf("Successfully simulated direct action: %s", actionName), nil
}
