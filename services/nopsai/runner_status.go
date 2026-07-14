package nopsai

import (
	"strings"

	"nopsai/pkg/proto"
)

const (
	runnerConnectionStatusOnline      = "online"
	runnerConnectionStatusUnreachable = "unreachable"
)

func runnerConnectionStatus(metadata map[string]string) string {
	statusText := strings.ToLower(strings.TrimSpace(metadata["connection_status"]))
	if statusText == "" {
		if runnerReachable(metadata) {
			return runnerConnectionStatusOnline
		}
		return runnerConnectionStatusUnreachable
	}
	if !runnerReachable(metadata) {
		return runnerConnectionStatusUnreachable
	}
	return statusText
}

func runnerReachable(metadata map[string]string) bool {
	reachable := strings.ToLower(strings.TrimSpace(metadata["reachable"]))
	switch reachable {
	case "false", "0", "no", "unreachable", "offline", "disconnected":
		return false
	case "true", "1", "yes", "reachable", "online", "connected":
		return true
	}

	switch strings.ToLower(strings.TrimSpace(metadata["connection_status"])) {
	case "unreachable", "offline", "disconnected":
		return false
	default:
		return true
	}
}

func runnerUnreachableCount(status *proto.DispatcherStatus) int {
	if status == nil {
		return 0
	}
	count := 0
	for _, runner := range status.GetRunners() {
		if !runnerReachable(runner.GetMetadata()) {
			count++
		}
	}
	return count
}
