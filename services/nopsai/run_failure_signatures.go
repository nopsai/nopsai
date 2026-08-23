package nopsai

import "strings"

// Run failure signatures.
//
// The failure analysis surface has to answer "why did this fail", not hand the
// operator the logs it just read. A signature turns one decisive log line into
// the conclusion it implies and the single action that clears it. Matching is
// deterministic, so the same run always produces the same answer, and an
// unrecognised failure still reports the first error line as evidence rather
// than restating the run status.
type runFailureSignature struct {
	// needles are matched case-insensitively against the log line. The first
	// signature with any needle present wins, so order them decisive first.
	needles []string
	cause   string
	action  string
}

var runFailureSignatures = []runFailureSignature{
	{
		needles: []string{
			"inappropriate ioctl for device",
			"the input device is not a tty",
			"is not a terminal",
			"cannot read from stdin",
		},
		cause:  "A command asked for interactive input and the runner has no terminal, so the read failed before the command could do any work. This is almost always a credential flag that never reached the command, leaving it to prompt.",
		action: "Give the command its input non-interactively — a credential flag such as --password-stdin, or the value on the command line — and check that the flags survive any line continuations in the step script.",
	},
	{
		needles: []string{
			"unauthorized",
			"authentication required",
			"denied: permission_denied",
			"401 unauthorized",
			"403 forbidden",
			"invalid username or password",
			"bad credentials",
		},
		cause:  "The registry or API rejected the credentials the step presented.",
		action: "Check the secret this step reads: that it is set for this scope, that it has not expired, and that its account can write to the target.",
	},
	{
		needles: []string{"no space left on device", "disk quota exceeded"},
		cause:   "The runner ran out of disk while the step was writing.",
		action:  "Free space on the runner or give this step a larger workspace, then re-run.",
	},
	{
		needles: []string{"oomkilled", "out of memory", "cannot allocate memory", "exit code 137", "signal: killed"},
		cause:   "The step was killed for exceeding its memory limit.",
		action:  "Raise the memory limit for this step, or reduce what it holds in memory, then re-run.",
	},
	{
		needles: []string{"command not found", "executable file not found", "no such file or directory: /usr/bin", "exit code 127"},
		cause:   "The step called a binary that is not in its image.",
		action:  "Install the missing tool in the step image, or switch to an image that ships it.",
	},
	{
		needles: []string{
			"connection refused",
			"i/o timeout",
			"could not resolve host",
			"temporary failure in name resolution",
			"no such host",
			"network is unreachable",
			"tls handshake timeout",
		},
		cause:  "The step could not reach a host it depends on.",
		action: "Check that the host is reachable from the runner's network and that any proxy or firewall rule allows it.",
	},
	{
		needles: []string{"permission denied", "operation not permitted", "read-only file system"},
		cause:   "The step was refused access to a path it tried to use.",
		action:  "Check the ownership and mode of that path in the step image, and whether the step needs a writable volume.",
	},
	{
		needles: []string{"--- fail", "tests failed", "test failure", "assertion failed", "npm err!"},
		cause:   "The step's own tests or build checks failed; the command reported the failure itself.",
		action:  "Read the first failing test or check in this step's output and fix it, rather than re-running.",
	},
}

// runFailureLogLine is the shape the log tools already return for one line.
type runFailureLogLine = map[string]any

// analyzeRunFailureLogs picks the decisive line out of a run's log excerpt and
// returns the conclusion it supports. The second return value is the evidence
// the conclusion rests on, so a caller can show its work; both are empty when
// there is nothing in the excerpt to reason from.
func analyzeRunFailureLogs(logs []runFailureLogLine) (cause string, action string, evidence map[string]any) {
	var firstError map[string]any
	for _, entry := range logs {
		line, _ := entry["line"].(string)
		if strings.TrimSpace(line) == "" {
			continue
		}
		lowered := strings.ToLower(line)
		for _, signature := range runFailureSignatures {
			for _, needle := range signature.needles {
				if strings.Contains(lowered, needle) {
					return signature.cause, signature.action, runFailureEvidence(entry)
				}
			}
		}
		if firstError == nil && runFailureLineLooksLikeError(entry, lowered) {
			firstError = entry
		}
	}
	if firstError == nil {
		return "", "", nil
	}
	return "The step reported an error that does not match a known failure signature; the line below is the first one it printed.",
		"Read this step's log from this line onward — the command that printed it is the one that failed.",
		runFailureEvidence(firstError)
}

// runFailureLineLooksLikeError keeps the fallback honest: an error level or the
// stderr stream is a real signal, and so is a line that names its own failure.
func runFailureLineLooksLikeError(entry map[string]any, lowered string) bool {
	if level, _ := entry["level"].(string); strings.EqualFold(level, "error") {
		return true
	}
	if stream, _ := entry["stream"].(string); strings.EqualFold(stream, "stderr") {
		return true
	}
	return strings.Contains(lowered, "error:") ||
		strings.Contains(lowered, "failed") ||
		strings.Contains(lowered, "fatal")
}

func runFailureEvidence(entry map[string]any) map[string]any {
	evidence := map[string]any{}
	for _, key := range []string{"line", "step_name", "task_name", "stream", "level", "timestamp"} {
		if value, ok := entry[key]; ok {
			if text, isText := value.(string); isText && strings.TrimSpace(text) == "" {
				continue
			}
			evidence[key] = value
		}
	}
	return evidence
}
