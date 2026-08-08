import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  PIPELINE_DIRECTIVES as PIPELINE_YAML_DIRECTIVES,
  STEP_DIRECTIVES as PIPELINE_STEP_DIRECTIVES,
  TASK_DIRECTIVES as PIPELINE_TASK_DIRECTIVES,
} from '../pipelines/model.js';
import {
  STEP_DIRECTIVES as REUSABLE_STEP_DIRECTIVES,
  TASK_DIRECTIVES as REUSABLE_TASK_DIRECTIVES,
} from '../steps/model.js';
import {
  PIPELINE_DIRECTIVES as LAB_PIPELINE_DIRECTIVES,
  STEP_DIRECTIVES as LAB_STEP_DIRECTIVES,
  TASK_DIRECTIVES as LAB_TASK_DIRECTIVES,
} from '../../lib/lab.js';
import { getYamlToolboxSpec, insertYamlSnippetAtCursor } from './yamlToolboxModel.js';

function findParameterKeys(resourceKind: 'pipeline' | 'step' | 'trigger', groupTitle: string) {
  const spec = getYamlToolboxSpec(resourceKind);
  const group = spec.parameterGroups.find(entry => entry.title === groupTitle);
  assert.ok(group, `${groupTitle} should exist`);
  return group.parameters.map(parameter => parameter.key);
}

function assertIncludesAll(actual: string[], expected: string[], context: string) {
  const missing = expected.filter(key => !actual.includes(key));
  assert.deepEqual(missing, [], `${context} missing parameters: ${missing.join(', ')}`);
}

function labDirectiveKeys(directives: Array<{ key: string }>) {
  return directives.map(directive => directive.key);
}

test('builds resource-specific YAML toolbox specs', () => {
  const pipeline = getYamlToolboxSpec('pipeline');
  assert.equal(pipeline.title, 'Pipeline Toolbox');
  assert.ok(pipeline.parameterGroups.some(group => group.title === 'Pipeline Parameters'));
  assert.ok(pipeline.snippetGroups.some(group => group.snippets.some(snippet => snippet.id === 'script-step')));

  const trigger = getYamlToolboxSpec('trigger');
  assert.ok(trigger.parameterGroups.some(group => group.parameters.some(parameter => parameter.key === 'provider')));
  assert.ok(trigger.snippetGroups.some(group => group.snippets.some(snippet => snippet.id === 'push-trigger')));
});

test('keeps parameter help aligned with editor autocomplete directive sets', () => {
  assertIncludesAll(
    findParameterKeys('pipeline', 'Pipeline Parameters'),
    PIPELINE_YAML_DIRECTIVES,
    'Pipeline parameter help'
  );
  assertIncludesAll(
    findParameterKeys('pipeline', 'Step Parameters'),
    PIPELINE_STEP_DIRECTIVES,
    'Pipeline step parameter help'
  );
  assertIncludesAll(
    findParameterKeys('pipeline', 'Task Parameters'),
    PIPELINE_TASK_DIRECTIVES,
    'Pipeline task parameter help'
  );
  assertIncludesAll(
    findParameterKeys('pipeline', 'Pipeline Parameters'),
    labDirectiveKeys(LAB_PIPELINE_DIRECTIVES),
    'Lab pipeline parameter help'
  );
  assertIncludesAll(
    findParameterKeys('pipeline', 'Step Parameters'),
    labDirectiveKeys(LAB_STEP_DIRECTIVES),
    'Lab step parameter help'
  );
  assertIncludesAll(
    findParameterKeys('pipeline', 'Task Parameters'),
    labDirectiveKeys(LAB_TASK_DIRECTIVES),
    'Lab task parameter help'
  );
  assertIncludesAll(
    findParameterKeys('step', 'Step Parameters'),
    REUSABLE_STEP_DIRECTIVES,
    'Reusable step parameter help'
  );
  assertIncludesAll(
    findParameterKeys('step', 'Task Parameters'),
    REUSABLE_TASK_DIRECTIVES,
    'Reusable task parameter help'
  );
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
