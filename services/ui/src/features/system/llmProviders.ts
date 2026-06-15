export type LLMAPIKeyMode = 'required' | 'optional' | 'none';
export type LLMBaseURLMode = 'hidden' | 'optional' | 'required';

export type LLMProviderDefinition = {
  id: string;
  label: string;
  defaultModel: string;
  defaultBaseURL: string;
  defaultSecretName: string;
  baseURLMode: LLMBaseURLMode;
  apiKeyMode: LLMAPIKeyMode;
  supportsReasoning?: boolean;
  supportsThinking?: boolean;
  supportsMaxTokens: boolean;
  supportsTemperature: boolean;
  temperatureMax: number;
  generationOptionsNote: string;
};

export const LLM_PROVIDERS: LLMProviderDefinition[] = [
  {
    id: 'lmstudio',
    label: 'LM Studio',
    defaultModel: 'qwen3-coder',
    defaultBaseURL: 'http://lmstudio:1234',
    defaultSecretName: 'LLM_API_KEY',
    baseURLMode: 'required',
    apiKeyMode: 'optional',
    supportsReasoning: true,
    supportsThinking: true,
    supportsMaxTokens: true,
    supportsTemperature: true,
    temperatureMax: 1,
    generationOptionsNote: 'Uses native max output tokens, temperature (0-1), and reasoning controls.',
  },
  {
    id: 'gemini',
    label: 'Google Gemini',
    defaultModel: 'gemini-2.5-flash',
    defaultBaseURL: '',
    defaultSecretName: 'GEMINI_API_KEY',
    baseURLMode: 'hidden',
    apiKeyMode: 'required',
    supportsMaxTokens: true,
    supportsTemperature: true,
    temperatureMax: 2,
    generationOptionsNote: 'Max tokens and temperature use generationConfig. Thinking is model-specific and is not controlled by the generic thinking field.',
  },
  {
    id: 'openai',
    label: 'OpenAI / ChatGPT',
    defaultModel: 'gpt-4.1-mini',
    defaultBaseURL: 'https://api.openai.com/v1',
    defaultSecretName: 'OPENAI_API_KEY',
    baseURLMode: 'optional',
    apiKeyMode: 'required',
    supportsMaxTokens: true,
    supportsTemperature: true,
    temperatureMax: 2,
    generationOptionsNote: 'Uses max_completion_tokens. Some reasoning models reject temperature, so leave it empty unless the selected model supports it.',
  },
  {
    id: 'anthropic',
    label: 'Anthropic Claude',
    defaultModel: 'claude-sonnet-4-6',
    defaultBaseURL: 'https://api.anthropic.com',
    defaultSecretName: 'ANTHROPIC_API_KEY',
    baseURLMode: 'optional',
    apiKeyMode: 'required',
    supportsMaxTokens: true,
    supportsTemperature: true,
    temperatureMax: 1,
    generationOptionsNote: 'Supports max tokens and temperature (0-1). Extended thinking requires model-specific configuration.',
  },
  {
    id: 'groq',
    label: 'Groq',
    defaultModel: 'llama-3.3-70b-versatile',
    defaultBaseURL: 'https://api.groq.com/openai/v1',
    defaultSecretName: 'GROQ_API_KEY',
    baseURLMode: 'optional',
    apiKeyMode: 'required',
    supportsMaxTokens: true,
    supportsTemperature: true,
    temperatureMax: 2,
    generationOptionsNote: 'Uses max_completion_tokens and temperature (0-2). Reasoning controls remain model-specific.',
  },
  {
    id: 'mistral',
    label: 'Mistral',
    defaultModel: 'mistral-large-latest',
    defaultBaseURL: 'https://api.mistral.ai/v1',
    defaultSecretName: 'MISTRAL_API_KEY',
    baseURLMode: 'optional',
    apiKeyMode: 'required',
    supportsMaxTokens: true,
    supportsTemperature: true,
    temperatureMax: 2,
    generationOptionsNote: 'Supports max tokens and temperature; Mistral recommends conservative temperatures, commonly 0-0.7.',
  },
  {
    id: 'openrouter',
    label: 'OpenRouter',
    defaultModel: 'openai/gpt-4.1-mini',
    defaultBaseURL: 'https://openrouter.ai/api/v1',
    defaultSecretName: 'OPENROUTER_API_KEY',
    baseURLMode: 'optional',
    apiKeyMode: 'required',
    supportsMaxTokens: true,
    supportsTemperature: true,
    temperatureMax: 2,
    generationOptionsNote: 'Uses max_completion_tokens. Effective temperature and reasoning support depends on the routed model.',
  },
  {
    id: 'ollama',
    label: 'Ollama',
    defaultModel: 'qwen2.5-coder:14b',
    defaultBaseURL: 'http://ollama:11434/v1',
    defaultSecretName: 'OLLAMA_API_KEY',
    baseURLMode: 'required',
    apiKeyMode: 'optional',
    supportsMaxTokens: true,
    supportsTemperature: true,
    temperatureMax: 2,
    generationOptionsNote: 'Uses OpenAI-compatible max tokens and temperature; effective support depends on the local model and Ollama version.',
  },
  {
    id: 'azure-openai',
    label: 'Azure OpenAI',
    defaultModel: 'gpt-4.1-mini',
    defaultBaseURL: '',
    defaultSecretName: 'AZURE_OPENAI_API_KEY',
    baseURLMode: 'required',
    apiKeyMode: 'required',
    supportsMaxTokens: true,
    supportsTemperature: true,
    temperatureMax: 2,
    generationOptionsNote: 'Uses max_completion_tokens. Azure reasoning deployments may reject temperature, so leave it empty unless supported.',
  },
];

export function getLLMProvider(provider: string): LLMProviderDefinition {
  return LLM_PROVIDERS.find(candidate => candidate.id === provider.trim()) ?? LLM_PROVIDERS[0]!;
}

export function defaultLLMSecretName(provider: string): string {
  return getLLMProvider(provider).defaultSecretName;
}

export function replaceProviderDefault(
  current: string,
  previousDefault: string,
  nextDefault: string
): string {
  return !current.trim() || current === previousDefault ? nextDefault : current;
}
