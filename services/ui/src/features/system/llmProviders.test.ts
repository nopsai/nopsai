import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  LLM_PROVIDERS,
  defaultLLMSecretName,
  getLLMProvider,
  replaceProviderDefault,
} from './llmProviders.js';

test('defines the first-wave LLM provider catalog', () => {
  assert.deepEqual(
    LLM_PROVIDERS.map(provider => provider.id),
    ['lmstudio', 'gemini', 'openai', 'anthropic', 'groq', 'mistral', 'openrouter', 'ollama', 'azure-openai']
  );
  assert.equal(getLLMProvider('openai').label, 'OpenAI / ChatGPT');
  assert.equal(defaultLLMSecretName('anthropic'), 'ANTHROPIC_API_KEY');
  assert.equal(getLLMProvider('ollama').apiKeyMode, 'optional');
  assert.equal(defaultLLMSecretName('ollama'), 'OLLAMA_API_KEY');
  assert.equal(getLLMProvider('anthropic').defaultModel, 'claude-sonnet-4-6');
  assert.equal(getLLMProvider('lmstudio').supportsReasoning, true);
  assert.equal(getLLMProvider('openai').supportsReasoning, undefined);
  assert.equal(getLLMProvider('lmstudio').temperatureMax, 1);
  assert.equal(getLLMProvider('anthropic').temperatureMax, 1);
  assert.equal(getLLMProvider('openai').temperatureMax, 2);
  assert.equal(LLM_PROVIDERS.every(provider => provider.supportsMaxTokens), true);
  assert.equal(LLM_PROVIDERS.every(provider => provider.supportsTemperature), true);
});

test('replaces only empty or provider-owned defaults', () => {
  assert.equal(replaceProviderDefault('', 'old', 'next'), 'next');
  assert.equal(replaceProviderDefault('old', 'old', 'next'), 'next');
  assert.equal(replaceProviderDefault('custom', 'old', 'next'), 'custom');
});
