package app

import "testing"

func TestExecutionRuntimeCloseAllowsNilDockerClient(t *testing.T) {
	if err := (*ExecutionRuntime)(nil).Close(); err != nil {
		t.Fatalf("nil runtime Close() error = %v", err)
	}
	if err := (&ExecutionRuntime{}).Close(); err != nil {
		t.Fatalf("empty runtime Close() error = %v", err)
	}
}
