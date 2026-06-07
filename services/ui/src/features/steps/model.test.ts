import assert from 'node:assert/strict';
import { test } from 'node:test';
import { normalizeSource, splitIdentifier, validateStepYaml } from './model.js';

test('validates reusable step include contracts', () => {
  const invalid = validateStepYaml('name: deploy\ninclude: pipelines/deploy');
  assert.equal(invalid.errors[0]?.message, "Include steps must reference a reusable step using the 'step:' prefix.");

  const valid = validateStepYaml('name: deploy\ninclude: step:shared/deploy');
  assert.deepEqual(valid.errors, []);
});

test('validates task names, modes, and dependencies', () => {
  const duplicate = validateStepYaml(`
name: build
tasks:
  - name: compile
    script: npm run build
  - name: compile
    goal: Build the app
`);
  assert.match(duplicate.errors[0]?.message ?? '', /Duplicate task name 'compile'/);

  const missingDependency = validateStepYaml(`
name: build
tasks:
  - name: compile
    script: npm run build
    depends_on:
      - missing
`);
  assert.match(missingDependency.errors[0]?.message ?? '', /depends on undefined task 'missing'/);
});

test('normalizes step identifiers and source labels', () => {
  assert.deepEqual(splitIdentifier('platform/payments/deploy%20api'), {
    name: 'deploy api',
    path: 'platform/payments',
  });
  assert.equal(normalizeSource('GitOps'), 'git');
  assert.equal(normalizeSource('draft'), 'draft');
  assert.equal(normalizeSource(''), 'database');
});
