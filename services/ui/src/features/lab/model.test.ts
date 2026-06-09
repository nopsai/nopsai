import assert from 'node:assert/strict';
import { test } from 'node:test';
import { parseLabIncludedDependencies } from './model.js';

test('extracts unique included dependencies from Lab YAML', () => {
  assert.deepEqual(
    parseLabIncludedDependencies(`
name: release
steps:
  - name: build
    include: step:build-image
  - name: deploy
    include: pipeline:platform/deploy
  - name: repeat
    include: step:build-image
`),
    {
      status: 'ok',
      items: ['pipeline:platform/deploy', 'step:build-image'],
    }
  );
});

test('reports empty and invalid Lab dependency states', () => {
  assert.deepEqual(parseLabIncludedDependencies('name: empty'), { status: 'no-steps', items: [] });
  assert.deepEqual(parseLabIncludedDependencies('[]'), { status: 'invalid', items: [] });
  assert.deepEqual(parseLabIncludedDependencies('name: ['), { status: 'parse-error', items: [] });
});
