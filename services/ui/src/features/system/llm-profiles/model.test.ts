import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  emptyLLMProfileForm,
  llmProfileFormFromRecord,
  llmProfilePayloadFromForm,
  normalizeLLMProfilesPayload,
  type LLMProfileRecord,
} from './model.js';

test('normalizes LLM profile payloads and selects a stable default', () => {
  const payload = normalizeLLMProfilesPayload({
    default_profile: '',
    profiles: [
      {
        name: 'z-reasoning',
        provider: 'lmstudio',
        model: 'qwen3-coder',
        allowed_scopes: ['dev', ''],
        thinking: true,
        timeout_seconds: 30,
        max_tokens: 4096,
        temperature: 0.2,
        extra: { deployment: 'review' },
        references: ['pipeline:build'],
      },
      {
        name: 'alpha',
        provider: 'gemini',
        model: 'gemini-2.5-pro',
        status: 'valid',
      },
      { name: '   ' },
    ],
  });

  assert.equal(payload.default_profile, 'alpha');
  assert.deepEqual(
    payload.profiles.map(profile => profile.name),
    ['alpha', 'z-reasoning']
  );
  assert.deepEqual(payload.profiles[1]?.allowed_scopes, ['dev']);
  assert.equal(payload.profiles[1]?.thinking, true);
  assert.equal(payload.profiles[1]?.timeout_seconds, 30);
  assert.equal(payload.profiles[1]?.temperature, 0.2);
  assert.deepEqual(payload.profiles[1]?.extra, { deployment: 'review' });
  assert.deepEqual(payload.profiles[1]?.references, ['pipeline:build']);
});

test('converts LLM profile records into editable form state', () => {
  const profile: LLMProfileRecord = {
    name: 'local',
    provider: 'lmstudio',
    model: 'qwen3-coder',
    base_url: 'http://lmstudio:1234',
    api_key_secret: '',
    allowed_scopes: ['dev', 'internal'],
    reasoning: 'medium',
    thinking: false,
    timeout_seconds: 30,
    max_tokens: 4096,
    temperature: 0.1,
    extra: { api_version: '2024-10-21', deployment: 'local' },
    status: 'valid',
  };

  assert.deepEqual(llmProfileFormFromRecord(profile), {
    name: 'local',
    provider: 'lmstudio',
    model: 'qwen3-coder',
    base_url: 'http://lmstudio:1234',
    api_key_secret: '',
    allowed_scopes: 'dev, internal',
    reasoning: 'medium',
    thinking: 'false',
    timeout_seconds: '30',
    max_tokens: '4096',
    temperature: '0.1',
    extra: 'api_version=2024-10-21\ndeployment=local',
  });
});

test('builds API payloads from LLM profile form state', () => {
  const payload = llmProfilePayloadFromForm({
    name: ' local ',
    provider: ' lmstudio ',
    model: ' qwen3-coder ',
    base_url: ' http://lmstudio:1234 ',
    api_key_secret: ' LOCAL_KEY ',
    allowed_scopes: 'dev, internal, ',
    reasoning: ' high ',
    thinking: 'true',
    timeout_seconds: '20',
    max_tokens: '2048',
    temperature: '0.25',
    extra: 'x_title=NopsAI\nhttp_referer=https://nopsai.example.com',
  });

  assert.deepEqual(payload, {
    name: 'local',
    provider: 'lmstudio',
    model: 'qwen3-coder',
    base_url: 'http://lmstudio:1234',
    api_key_secret: 'LOCAL_KEY',
    allowed_scopes: ['dev', 'internal'],
    reasoning: 'high',
    thinking: true,
    timeout_seconds: 20,
    max_tokens: 2048,
    temperature: 0.25,
    extra: {
      http_referer: 'https://nopsai.example.com',
      x_title: 'NopsAI',
    },
  });

  assert.equal(
    llmProfilePayloadFromForm({
      name: 'standard',
      provider: 'gemini',
      model: 'gemini-2.5-pro',
      base_url: '',
      api_key_secret: 'GEMINI_API_KEY',
      allowed_scopes: '',
      reasoning: '',
      thinking: 'false',
      timeout_seconds: '',
      max_tokens: '',
      temperature: '',
      extra: '',
    }).thinking,
    undefined
  );
});

test('removes LM Studio-only options from other providers', () => {
  const payload = llmProfilePayloadFromForm({
    ...emptyLLMProfileForm,
    name: 'hosted',
    provider: 'openai',
    model: 'gpt-test',
    api_key_secret: 'OPENAI_API_KEY',
    reasoning: 'high',
    thinking: 'true',
  });

  assert.equal(payload.reasoning, '');
  assert.equal(payload.thinking, undefined);
});
