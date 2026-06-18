import assert from 'node:assert/strict';
import { test } from 'node:test';
import { buildPipelineEditorSuggestion, type PipelineAutocompleteMetadata } from './editorAutocomplete.js';

const metadata: PipelineAutocompleteMetadata = {
  secrets: [],
  variables: [],
  agentProfiles: [],
  llmProfiles: [],
  mcpProfiles: [],
  runtimePools: ['default', 'high-memory'],
  reusableSteps: [],
  secretScopes: [],
  variableScopes: [],
};

test('suggests configured runtime pools for pipeline and step runtime_pool values', () => {
  const pipelineYaml = 'name: deploy\nruntime_pool: h';
  const pipelineSuggestion = buildPipelineEditorSuggestion({
    text: pipelineYaml,
    cursor: pipelineYaml.length,
    metadata,
    detail: null,
  });

  assert.equal(pipelineSuggestion?.title, 'Runtime pools');
  assert.deepEqual(pipelineSuggestion?.items, ['high-memory']);
  assert.equal(pipelineSuggestion?.appendColon, false);

  const stepYaml = 'steps:\n  - name: build\n    runtime_pool: ';
  const stepSuggestion = buildPipelineEditorSuggestion({
    text: stepYaml,
    cursor: stepYaml.length,
    force: true,
    metadata,
    detail: null,
  });

  assert.equal(stepSuggestion?.title, 'Runtime pools');
  assert.deepEqual(stepSuggestion?.items, ['default', 'high-memory']);
});
