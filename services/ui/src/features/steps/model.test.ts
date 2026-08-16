import assert from 'node:assert/strict';
import { test } from 'node:test';
import { filterVisibleStepList, normalizeSource, splitIdentifier, validateStepYaml } from './model.js';

test('validates reusable step include contracts', () => {
  const invalid = validateStepYaml('name: deploy\ninclude: pipelines/deploy');
  assert.equal(invalid.errors[0]?.message, "Include steps must reference a reusable step using the 'step:' prefix.");

  const valid = validateStepYaml('name: deploy\nruntime_pool: high-memory\ninclude: step:shared/deploy');
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

test('validates reusable step variables and runtime outputs', () => {
  const valid = validateStepYaml(`
name: variable-defaults
variables:
  RELEASE_MANIFEST: default-release-manifest
script: |
  printf manifest > /nopsai/outputs/release_manifest
outputs:
  - release_manifest
  - name: access_token
    sensitive: true
`);
  assert.deepEqual(valid.errors, []);

  const badVariable = validateStepYaml(`
name: variable-defaults
variables:
  BAD/NAME: value
script: echo ok
`);
  assert.match(badVariable.errors[0]?.message ?? '', /BAD\/NAME/);

  const duplicateOutput = validateStepYaml(`
name: variable-defaults
script: echo ok
outputs:
  - image_tag
  - image_tag
`);
  assert.match(duplicateOutput.errors[0]?.message ?? '', /declared more than once/);
});

test('rejects a reusable step or task governance level that is not advisory or strict', () => {
  const stepLevel = validateStepYaml(`
name: governed-step
governance_level: guarded
script: make check
`);
  assert.equal(stepLevel.errors.length, 1);
  assert.match(stepLevel.errors[0]?.message ?? '', /'advisory' or 'strict'/);

  const taskLevel = validateStepYaml(`
name: governed-step
tasks:
  - name: inspect
    governance_level: exception_based
    goal: Inspect workspace readiness.
`);
  assert.equal(taskLevel.errors.length, 1);
  assert.match(taskLevel.errors[0]?.message ?? '', /'advisory' or 'strict'/);
});

test('validates reusable step governance level directives', () => {
  const result = validateStepYaml(`
name: governed-step
governance_level: strict
tasks:
  - name: inspect
    governance_level: advisory
    goal: Inspect workspace readiness.
`);

  assert.deepEqual(result.errors, []);
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

test('filters visible steps by all teams, selected team subtree, and search', () => {
  const items = [
    { id: 'setup' },
    { id: 'library/docker/build-image' },
    { id: 'library/deploy' },
    { id: 'sandbox/lint' },
  ];

  assert.deepEqual(
    filterVisibleStepList(items, '', '').map(item => item.id),
    ['library/deploy', 'library/docker/build-image', 'sandbox/lint', 'setup']
  );
  assert.deepEqual(
    filterVisibleStepList(items, '', 'global').map(item => item.id),
    ['setup']
  );
  assert.deepEqual(
    filterVisibleStepList(items, '', 'library').map(item => item.id),
    ['library/deploy', 'library/docker/build-image']
  );
  assert.deepEqual(
    filterVisibleStepList(items, 'build', 'sandbox').map(item => item.id),
    ['library/docker/build-image']
  );
});
