import assert from 'node:assert/strict';
import { test } from 'node:test';
import { buildSuggestionItems, detectSuggestionContext, suggestionCopyForContext, validatePipelineYamlStrict } from './lab.js';

test('suggests runtime pools for Lab runtime_pool directive values', () => {
  const yaml = 'name: deploy\nruntime_pool: h';
  const context = detectSuggestionContext(yaml, yaml.length, yaml.length);

  assert.equal(context?.type, 'runtime_pool');
  assert.equal(context?.title, 'Runtime Pools');

  const items = buildSuggestionItems(context!, yaml, {
    secrets: [],
    variables: [],
    agentProfiles: [],
    llmProfiles: [],
    mcpProfiles: [],
    runtimePools: ['default', 'high-memory'],
    reusableSteps: [],
    pipelineIds: [],
  });

  assert.deepEqual(items, [{ value: 'high-memory', label: 'high-memory' }]);
  assert.equal(suggestionCopyForContext(context).title, 'Runtime pools');
});

test('accepts the two supported display_option values', () => {
  for (const value of ['list', 'graph']) {
    const yaml = `name: build\ncontainer_image: alpine:3.20\ndisplay_option: ${value}\nsteps:\n  - name: run\n    tasks:\n      - name: echo\n        script: echo ok\n`;
    const result = validatePipelineYamlStrict(yaml);
    assert.deepEqual(result.errors, [], `expected display_option: ${value} to validate`);
  }
});

test('rejects display_option values that are no longer supported', () => {
  for (const value of ['mermaid', 'tree', 'flat', 'Graph']) {
    const yaml = `name: build\ncontainer_image: alpine:3.20\ndisplay_option: ${value}\nsteps:\n  - name: run\n    tasks:\n      - name: echo\n        script: echo ok\n`;
    const result = validatePipelineYamlStrict(yaml);
    assert.equal(result.errors.length, 1, `expected display_option: ${value} to be rejected`);
    assert.match(result.errors[0].message, /'display_option' must be 'list' or 'graph'/);
  }
});

test('rejects the removed nested display_options block', () => {
  const yaml = 'name: build\ncontainer_image: alpine:3.20\ndisplay_options:\n  github_view: mermaid\nsteps:\n  - name: run\n    tasks:\n      - name: echo\n        script: echo ok\n';
  const result = validatePipelineYamlStrict(yaml);
  assert.equal(result.errors.length, 1);
  assert.match(result.errors[0].message, /Unknown field 'display_options'/);
});
