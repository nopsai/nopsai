package llm

import appconfig "nopsai/config"

const (
	ProviderFeatureModeAuto     = "auto"
	ProviderFeatureModeRequired = "required"
	ProviderFeatureModeDisabled = "disabled"

	ExecutionModeStatelessPrompt       = "stateless_prompt"
	ExecutionModeStatelessPromptCached = "stateless_prompt_cached"
	ExecutionModeProviderState         = "provider_state"
)

type ProviderCapabilities struct {
	Provider                 string
	PromptCacheSupported     bool
	PromptCacheControl       bool
	ProviderStateSupported   bool
	ProviderStateDisposable  bool
	CachedTokenUsageReported bool
}

func CapabilitiesForProvider(provider string) ProviderCapabilities {
	normalized := appconfig.NormalizeLLMProvider(provider)
	capabilities := ProviderCapabilities{Provider: normalized}
	switch normalized {
	case appconfig.LLMProviderOpenAI, appconfig.LLMProviderAzureOpenAI:
		capabilities.PromptCacheSupported = true
		capabilities.CachedTokenUsageReported = true
	case appconfig.LLMProviderAnthropic, appconfig.LLMProviderGemini:
		capabilities.PromptCacheSupported = true
		capabilities.PromptCacheControl = true
		capabilities.CachedTokenUsageReported = true
	}
	return capabilities
}
