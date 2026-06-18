import assert from 'node:assert/strict';
import { test } from 'node:test';
import { buildSuggestionItems, detectSuggestionContext, suggestionCopyForContext } from './lab.js';

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
