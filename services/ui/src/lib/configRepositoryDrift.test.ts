import assert from 'node:assert/strict';
import test from 'node:test';
import { buildConfigRepositoryContentDiff } from './configRepositoryDrift.js';

test('buildConfigRepositoryContentDiff marks only changed lines in modified drift content', () => {
  const diff = buildConfigRepositoryContentDiff({
    path: 'setting/system/runner.yaml',
    status: 'modified',
    git_content: 'runner:\n  capacity: 1\n  scopes: dev\n',
    desired_content: 'runner:\n  capacity: 2\n  scopes: dev\n',
  });

  assert.equal(diff.changedLines, 2);
  assert.deepEqual(diff.git.map(line => line.kind), ['context', 'removed', 'context']);
  assert.deepEqual(diff.desired.map(line => line.kind), ['context', 'added', 'context']);
  assert.deepEqual(
    { number: diff.git[1]?.number, text: diff.git[1]?.text },
    { number: 2, text: '  capacity: 1' }
  );
  assert.deepEqual(
    { number: diff.desired[1]?.number, text: diff.desired[1]?.text },
    { number: 2, text: '  capacity: 2' }
  );
});

test('buildConfigRepositoryContentDiff highlights whole added and deleted files', () => {
  const added = buildConfigRepositoryContentDiff({
    path: 'pipelines/deploy.yaml',
    status: 'added',
    desired_content: 'name: deploy\nsteps: []\n',
  });
  const deleted = buildConfigRepositoryContentDiff({
    path: 'pipelines/old.yaml',
    status: 'deleted',
    delete: true,
    git_content: 'name: old\nsteps: []\n',
  });

  assert.equal(added.git.length, 0);
  assert.deepEqual(added.desired.map(line => line.kind), ['added', 'added']);
  assert.deepEqual(deleted.git.map(line => line.kind), ['removed', 'removed']);
  assert.equal(deleted.desired.length, 0);
});
