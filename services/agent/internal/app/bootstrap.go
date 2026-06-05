package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	appconfig "nopsai/config"
	"nopsai/pkg/models"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const (
	ResumeCheckpointIDEnv = "RESUME_CHECKPOINT_ID"

	defaultLLMTimeout = 2 * time.Minute
)

type EnvLookup func(string) string

type RuntimeConfig struct {
	RunID                    string
	PipelineName             string
	TriggerEventID           string
	PipelineDefinitionBase64 string
	PipelineDefinitionYAML   []byte
	ParentHistoryBase64      string
	SharedVolumeName         string
	PipelineTimeout          string
	DockerNetworkName        string
	LLMTimeout               time.Duration
	Secrets                  map[string]string
	Variables                map[string]string
	ResumeCheckpointID       string
	RunScope                 string
	RuntimeMode              string
	Pipeline                 models.Pipeline
}

type Warning struct {
	Kind WarningKind
	Err  error
}

type WarningKind string

const (
	WarningDecodeSecrets      WarningKind = "decode_secrets"
	WarningUnmarshalSecrets   WarningKind = "unmarshal_secrets"
	WarningDecodeVariables    WarningKind = "decode_variables"
	WarningUnmarshalVariables WarningKind = "unmarshal_variables"
)

type FailureKind string

const (
	FailureInvalidLLMTimeout           FailureKind = "invalid_llm_timeout"
	FailureMissingRequiredRuntimeValue FailureKind = "missing_required_runtime_value"
	FailureDecodePipelineDefinition    FailureKind = "decode_pipeline_definition"
	FailureUnmarshalPipelineDefinition FailureKind = "unmarshal_pipeline_definition"
)

type LoadError struct {
	Kind FailureKind
	Err  error
}

func (e LoadError) Error() string {
	if e.Err == nil {
		return string(e.Kind)
	}
	return e.Err.Error()
}

func (e LoadError) Unwrap() error {
	return e.Err
}

func ConfigureLogging(logFormat string) {
	if strings.EqualFold(strings.TrimSpace(logFormat), "console") {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	} else {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	}
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
}

func LoadRuntimeConfig(lookup EnvLookup) (RuntimeConfig, []Warning, error) {
	if lookup == nil {
		lookup = os.Getenv
	}

	config := RuntimeConfig{
		RunID:                    lookup("RUN_ID"),
		PipelineName:             lookup("PIPELINE_NAME"),
		TriggerEventID:           firstNonEmpty(lookup("GIT_TRIGGER_EVENT_ID"), "N/A"),
		PipelineDefinitionBase64: lookup("PIPELINE_DEFINITION"),
		ParentHistoryBase64:      lookup("PARENT_EXECUTION_HISTORY"),
		SharedVolumeName:         lookup("SHARED_VOLUME_NAME"),
		PipelineTimeout:          lookup("PIPELINE_TIMEOUT"),
		DockerNetworkName:        lookup("DOCKER_NETWORK_NAME"),
		ResumeCheckpointID:       lookup(ResumeCheckpointIDEnv),
		RunScope:                 lookup("SCOPE"),
		RuntimeMode:              appconfig.NormalizeRuntime(lookup("NOPSAI_RUNTIME")),
	}

	var warnings []Warning
	secrets, decodeFailure, err := decodeStringMapPayload(lookup("NOPSAI_SECRETS"))
	if err != nil {
		warnings = append(warnings, Warning{Kind: payloadWarningKind("secrets", decodeFailure), Err: err})
	} else {
		config.Secrets = secrets
	}

	variables, decodeFailure, err := decodeStringMapPayload(lookup("NOPSAI_VARIABLES"))
	if err != nil {
		warnings = append(warnings, Warning{Kind: payloadWarningKind("variables", decodeFailure), Err: err})
	} else {
		config.Variables = variables
	}
	if config.Variables == nil {
		config.Variables = map[string]string{}
	}

	llmTimeoutRaw := strings.TrimSpace(lookup("LLM_AGENT_TIMEOUT"))
	if llmTimeoutRaw == "" {
		config.LLMTimeout = defaultLLMTimeout
	} else {
		llmTimeout, err := time.ParseDuration(llmTimeoutRaw)
		if err != nil {
			return config, warnings, LoadError{Kind: FailureInvalidLLMTimeout, Err: err}
		}
		config.LLMTimeout = llmTimeout
	}

	if config.RunID == "" || config.PipelineDefinitionBase64 == "" || config.PipelineName == "" || config.SharedVolumeName == "" {
		return config, warnings, LoadError{
			Kind: FailureMissingRequiredRuntimeValue,
			Err:  fmt.Errorf("missing one or more required runtime variables"),
		}
	}

	pipelineDefBytes, err := base64.StdEncoding.DecodeString(config.PipelineDefinitionBase64)
	if err != nil {
		return config, warnings, LoadError{Kind: FailureDecodePipelineDefinition, Err: err}
	}
	config.PipelineDefinitionYAML = pipelineDefBytes

	if err := yaml.Unmarshal(pipelineDefBytes, &config.Pipeline); err != nil {
		return config, warnings, LoadError{Kind: FailureUnmarshalPipelineDefinition, Err: err}
	}

	return config, warnings, nil
}

func LoadFailureLogMessage(err error) string {
	var loadErr LoadError
	if !errors.As(err, &loadErr) {
		return "Failed to load agent runtime configuration"
	}
	switch loadErr.Kind {
	case FailureInvalidLLMTimeout:
		return "Invalid LLM timeout duration"
	case FailureMissingRequiredRuntimeValue:
		return "Missing one or more required runtime variables"
	case FailureDecodePipelineDefinition:
		return "Failed to decode pipeline definition"
	case FailureUnmarshalPipelineDefinition:
		return "Failed to unmarshal pipeline definition"
	default:
		return "Failed to load agent runtime configuration"
	}
}

func (w Warning) LogMessage() string {
	switch w.Kind {
	case WarningDecodeSecrets:
		return "Failed to decode secrets payload"
	case WarningUnmarshalSecrets:
		return "Failed to unmarshal secrets payload"
	case WarningDecodeVariables:
		return "Failed to decode variables payload"
	case WarningUnmarshalVariables:
		return "Failed to unmarshal variables payload"
	default:
		return "Failed to load agent runtime payload"
	}
}

func decodeStringMapPayload(raw string) (map[string]string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, false, nil
	}
	payloadJSON, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, true, err
	}
	var payload map[string]string
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, false, err
	}
	return payload, false, nil
}

func payloadWarningKind(name string, decodeFailure bool) WarningKind {
	switch name {
	case "secrets":
		if decodeFailure {
			return WarningDecodeSecrets
		}
		return WarningUnmarshalSecrets
	case "variables":
		if decodeFailure {
			return WarningDecodeVariables
		}
		return WarningUnmarshalVariables
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
