import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildKnowledgeTree,
  decodeKnowledgeRouteID,
  splitKnowledgeContentForPreview,
  validateKnowledgeIdentity,
  type KnowledgeContextListItem,
} from './model.js';

const documents: KnowledgeContextListItem[] = [
  {
    id: 'runbook/platform/restart',
    kind: 'runbook',
    team: 'platform',
    name: 'restart',
    visibility: 'team',
    source: 'database',
  },
];

test('builds knowledge trees with empty enterprise team teams', () => {
  const tree = buildKnowledgeTree(documents, ['platform/security']);
  const runbooks = tree.children.find(child => child.name === 'runbook');
  assert.equal(runbooks?.children.find(child => child.name === 'platform')?.docs[0]?.name, 'restart');
  assert.equal(
    runbooks?.children.find(child => child.name === 'platform')?.children.find(child => child.name === 'security')?.docs.length,
    0
  );
});

test('extracts preview content and document parameters', () => {
  assert.deepEqual(
    splitKnowledgeContentForPreview('---\nname: restart\nkind: runbook\ndescription: Recovery\n---\n# Restart\nSteps'),
    {
      content: '# Restart\nSteps',
      parameters: { name: 'restart', kind: 'runbook', description: 'Recovery' },
    }
  );

  assert.deepEqual(splitKnowledgeContentForPreview('name: restart\nkind: runbook\n\nBody'), {
    content: 'Body',
    parameters: { name: 'restart', kind: 'runbook' },
  });
});

test('validates knowledge identities and route ids', () => {
  assert.equal(validateKnowledgeIdentity('runbook', 'platform', 'restart', documents), 'A knowledge context with that identifier already exists.');
  assert.match(validateKnowledgeIdentity('runbook', '../platform', 'new', documents), /invalid path segments/);
  assert.equal(decodeKnowledgeRouteID('/knowledge-context/runbook/platform/restart%20service'), 'runbook/platform/restart service');
});
