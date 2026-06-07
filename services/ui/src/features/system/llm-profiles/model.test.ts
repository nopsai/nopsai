import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
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
    }).thinking,
    undefined
  );
});
