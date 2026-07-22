package servicelog

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type splitLevelWriter struct {
	stdout io.Writer
	stderr io.Writer
}

func (w splitLevelWriter) Write(payload []byte) (int, error) {
	return w.stdout.Write(payload)
}

func (w splitLevelWriter) WriteLevel(level zerolog.Level, payload []byte) (int, error) {
	switch level {
	case zerolog.WarnLevel, zerolog.ErrorLevel, zerolog.FatalLevel, zerolog.PanicLevel:
		return w.stderr.Write(payload)
	default:
		return w.stdout.Write(payload)
	}
}

// Configure installs the shared service logger and applies the configured level.
func Configure(rawLevel, format string) error {
	level, err := ParseLevel(rawLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	ConfigureLevel(level, format)
	return err
}

// ConfigureLevel routes routine events to stdout and warnings or errors to stderr.
func ConfigureLevel(level zerolog.Level, format string) {
	context := zerolog.New(newSplitLevelWriter(format, os.Stdout, os.Stderr)).With().Timestamp()
	if serviceName := detectServiceName(); serviceName != "" {
		context = context.Str("service", serviceName)
	}
	if environment := detectEnvironment(); environment != "" {
		context = context.Str("environment", environment)
	}
	log.Logger = context.Logger()
	zerolog.SetGlobalLevel(level)
}

func newSplitLevelWriter(format string, stdout, stderr io.Writer) splitLevelWriter {
	if strings.EqualFold(strings.TrimSpace(format), "console") {
		stdout = zerolog.ConsoleWriter{Out: stdout, TimeFormat: time.Kitchen}
		stderr = zerolog.ConsoleWriter{Out: stderr, TimeFormat: time.Kitchen}
	}
	return splitLevelWriter{stdout: stdout, stderr: stderr}
}

func detectServiceName() string {
	for _, key := range []string{"NOPSAI_SERVICE_NAME", "SERVICE_NAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	if len(os.Args) == 0 {
		return ""
	}
	return strings.TrimSpace(filepath.Base(os.Args[0]))
}

func detectEnvironment() string {
	for _, key := range []string{"NOPSAI_ENV", "NOPSAI_ENVIRONMENT", "ENVIRONMENT", "APP_ENV"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
