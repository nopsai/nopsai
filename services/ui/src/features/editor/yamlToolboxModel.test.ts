import assert from 'node:assert/strict';
import { test } from 'node:test';
import { getYamlToolboxSpec, insertYamlSnippetAtCursor } from './yamlToolboxModel.js';

test('builds resource-specific YAML toolbox specs', () => {
  const pipeline = getYamlToolboxSpec('pipeline');
  assert.equal(pipeline.title, 'Pipeline Toolbox');
  assert.ok(pipeline.parameterGroups.some(group => group.title === 'Pipeline Parameters'));
  assert.ok(pipeline.snippetGroups.some(group => group.snippets.some(snippet => snippet.id === 'script-step')));

  const trigger = getYamlToolboxSpec('trigger');
  assert.ok(trigger.parameterGroups.some(group => group.parameters.some(parameter => parameter.key === 'provider')));
  assert.ok(trigger.snippetGroups.some(group => group.snippets.some(snippet => snippet.id === 'push-trigger')));
});

test('inserts YAML snippets at the cursor without corrupting indentation', () => {
  const source = ['steps:', '  ', ''].join('\n');
  const cursor = source.indexOf('  ') + 2;
  const result = insertYamlSnippetAtCursor(source, cursor, cursor, '- name: build\n  script: make build');

  assert.equal(result.nextValue, 'steps:\n  - name: build\n    script: make build\n');
  assert.equal(result.nextCursor, 'steps:\n  - name: build\n    script: make build'.length);
});

test('separates inserted snippets from non-empty YAML lines', () => {
  const source = 'name: demo\nsteps: []';
  const cursor = source.indexOf('steps');
  const result = insertYamlSnippetAtCursor(source, cursor, cursor, 'variables:\n  - ENVIRONMENT');

  assert.equal(result.nextValue, 'name: demo\nvariables:\n  - ENVIRONMENT\nsteps: []');
});
