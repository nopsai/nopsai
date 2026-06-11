import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildInlineSuggestionPreview,
  normalizeAgentProfileSuggestionList,
  normalizeLLMProfileSuggestionList,
  normalizeLabScopeLabel,
  normalizeLabSuggestionList,
  normalizeMCPProfileSuggestionList,
  normalizeVariableSuggestionList,
} from './suggestions.js';

test('normalizes Lab scopes and list payloads', () => {
  assert.equal(normalizeLabScopeLabel(' default '), '');
  assert.equal(normalizeLabScopeLabel('/team/dev/'), 'team/dev');
  assert.deepEqual(normalizeLabSuggestionList([' TOKEN ', { name: 'REGION' }, { id: 'ignored' }]), ['TOKEN', 'REGION']);
  assert.deepEqual(normalizeVariableSuggestionList(['owner/repo/TOKEN', 'TOKEN', 'owner/repo/REGION']), ['REGION', 'TOKEN']);
});

test('filters unavailable LLM and MCP profiles', () => {
  assert.deepEqual(
    normalizeAgentProfileSuggestionList({
      profiles: [{ id: 'sre' }, { id: 'disabled', enabled: false }],
    }),
    ['sre']
  );
  assert.deepEqual(
    normalizeLLMProfileSuggestionList({
      profiles: [{ name: 'standard' }, { name: 'blocked', allowed_in_scope: false }],
    }),
    ['standard']
  );
  assert.deepEqual(
    normalizeMCPProfileSuggestionList({
      profiles: [{ name: 'github' }, { name: 'disabled', enabled: false }],
    }),
    ['github']
  );
});

test('builds an inline completion only when it matches the typed prefix', () => {
  const item = { label: 'pipeline', value: 'pipelines/release.yaml' };
  assert.equal(
    buildInlineSuggestionPreview(item, {
      type: 'pipeline-key',
      prefix: 'pipe',
      title: 'Pipeline',
      rangeStart: 0,
      rangeEnd: 4,
    }),
    'lines/release.yaml'
  );
  assert.equal(
    buildInlineSuggestionPreview(item, {
      type: 'pipeline-key',
      prefix: 'step',
      title: 'Pipeline',
      rangeStart: 0,
      rangeEnd: 4,
    }),
    ''
  );
});
