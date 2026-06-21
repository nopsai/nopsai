package servicelog

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestSplitLevelWriterRoutesBySeverity(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	logger := zerolog.New(splitLevelWriter{stdout: &stdout, stderr: &stderr})

	logger.Debug().Msg("diagnostic")
	logger.Info().Msg("ready")
	logger.Log().Msg("unlevelled")
	logger.Warn().Msg("degraded")
	logger.Error().Msg("failed")

	if got := stdout.String(); !strings.Contains(got, "diagnostic") || !strings.Contains(got, "ready") || !strings.Contains(got, "unlevelled") {
		t.Fatalf("stdout = %q, want debug, info, and unlevelled events", got)
	}
	if got := stdout.String(); strings.Contains(got, "degraded") || strings.Contains(got, "failed") {
		t.Fatalf("stdout = %q, contains warning or error event", got)
	}
	if got := stderr.String(); !strings.Contains(got, "degraded") || !strings.Contains(got, "failed") {
		t.Fatalf("stderr = %q, want warning and error events", got)
	}
}

func TestSplitLevelWriterRoutesUnlevelledEventsToStdout(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	writer := splitLevelWriter{stdout: &stdout, stderr: &stderr}
	if _, err := writer.Write([]byte("plain")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if stdout.String() != "plain" || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestSplitLevelWriterPreservesRoutingInConsoleFormat(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	logger := zerolog.New(newSplitLevelWriter(" CONSOLE ", &stdout, &stderr))

	logger.Info().Msg("ready")
	logger.Warn().Msg("degraded")

	if got := stdout.String(); !strings.Contains(got, "ready") || strings.Contains(got, "degraded") {
		t.Fatalf("stdout = %q, want only the info event", got)
	}
	if got := stderr.String(); !strings.Contains(got, "degraded") || strings.Contains(got, "ready") {
		t.Fatalf("stderr = %q, want only the warning event", got)
	}
}
