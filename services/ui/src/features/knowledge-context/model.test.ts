import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildKnowledgeConnectionTeamSummaries,
  buildKnowledgeTree,
  collectKnowledgeTeamDocs,
  decodeKnowledgeRouteID,
  documentTeamPath,
  deriveKnowledgeConnectionName,
  knowledgeContentSource,
  knowledgeConnectionProviderLabel,
  knowledgeConnectionStatusLabel,
  knowledgeSyncStatusLabel,
  knowledgeTreePathToTeam,
  matchesKnowledgeSourceFilter,
  normalizeKnowledgeSource,
  normalizeKnowledgeSourceFilter,
  normalizeKnowledgeWorkspaceTab,
  sourceLabel,
  splitKnowledgeContentForPreview,
  summarizeKnowledgeWorkspace,
  validateKnowledgeConnectionDraft,
  validateKnowledgeExternalDraft,
  validateKnowledgeIdentity,
  type KnowledgeConnectionListItem,
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
  assert.ok(runbooks);
  assert.equal(runbooks?.children.find(child => child.name === 'platform')?.docs[0]?.name, 'restart');
  assert.deepEqual(collectKnowledgeTeamDocs(runbooks).map(document => document.id), ['runbook/platform/restart']);
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

test('normalizes source-aware workspace state', () => {
  const items: KnowledgeContextListItem[] = [
    documents[0],
    {
      id: 'guardrail/security/repo-check',
      kind: 'guardrail',
      team: 'security',
      name: 'repo-check',
      visibility: 'restricted',
      source: 'git-repository',
      managed_by_config_repo: true,
      used_by_count: 2,
    },
    {
      id: 'policy/security/source-page',
      kind: 'policy',
      team: 'security',
      name: 'source-page',
      visibility: 'team',
      source: 'confluence-page',
      used_by: ['security/review'],
    },
  ];

  assert.equal(sourceLabel('notion-page'), 'Notion');
  assert.equal(normalizeKnowledgeSource('confluence-page'), 'confluence');
  assert.equal(knowledgeContentSource({ source: 'notion-page' }), 'external');
  assert.equal(knowledgeContentSource({ source: 'database' }), 'inline');
  assert.equal(knowledgeTreePathToTeam('guardrail/security/platform'), 'security/platform');
  assert.equal(documentTeamPath({ id: 'runbook/platform/restart', team: '' }), 'platform');
  assert.equal(normalizeKnowledgeWorkspaceTab('connections'), 'connections');
  assert.equal(normalizeKnowledgeWorkspaceTab('unknown'), 'documents');
  assert.equal(normalizeKnowledgeSourceFilter('gitops'), 'gitops');
  assert.equal(normalizeKnowledgeSourceFilter('invalid'), 'all');
  assert.equal(matchesKnowledgeSourceFilter(items[1], 'gitops'), true);
  assert.equal(matchesKnowledgeSourceFilter(items[2], 'external'), true);
  assert.equal(matchesKnowledgeSourceFilter(items[2], 'confluence'), true);
  assert.equal(matchesKnowledgeSourceFilter(items[0], 'notion'), false);
  assert.deepEqual(summarizeKnowledgeWorkspace(items), {
    documents: 3,
    teams: 2,
    kinds: 3,
    inlineDocuments: 2,
    externalDocuments: 1,
    gitOpsManaged: 1,
    referencedDocuments: 2,
    pipelineReferences: 3,
  });
  assert.deepEqual(buildKnowledgeConnectionTeamSummaries(items, ['platform/empty']), [
    {
      teamPath: 'platform',
      documentCount: 1,
      inlineDocuments: 1,
      externalDocuments: 0,
      gitOpsManaged: 0,
      referencedDocuments: 0,
      providers: [],
      connections: [],
    },
    {
      teamPath: 'platform/empty',
      documentCount: 0,
      inlineDocuments: 0,
      externalDocuments: 0,
      gitOpsManaged: 0,
      referencedDocuments: 0,
      providers: [],
      connections: [],
    },
    {
      teamPath: 'security',
      documentCount: 2,
      inlineDocuments: 1,
      externalDocuments: 1,
      gitOpsManaged: 1,
      referencedDocuments: 2,
      providers: ['confluence'],
      connections: [],
    },
  ]);
});

test('summarizes persisted knowledge connections', () => {
  const connections: KnowledgeConnectionListItem[] = [
    {
      id: 'security/security-notion',
      uuid: '00000000-0000-0000-0000-000000000001',
      team: 'security',
      name: 'security-notion',
      display_name: 'Security Notion',
      provider: 'notion',
      status: 'authentication_required',
      credential_visibility: 'not_configured',
      document_count: 0,
      external_document_count: 0,
    },
  ];

  const summaries = buildKnowledgeConnectionTeamSummaries([
    {
      id: 'policy/security/source-page',
      kind: 'policy',
      team: 'security',
      name: 'source-page',
      visibility: 'team',
      source: 'notion',
      connection_ref: 'security/security-notion',
    },
  ], ['security'], connections);
  assert.equal(summaries[0]?.providers[0], 'notion');
  assert.equal(summaries[0]?.connections[0]?.display_name, 'Security Notion');
  assert.deepEqual(summaries[0]?.connections[0]?.used_by, ['policy/security/source-page']);
  assert.equal(knowledgeConnectionProviderLabel('wiki'), 'Wiki page');
  assert.equal(knowledgeConnectionStatusLabel('authentication_required'), 'Auth required');
  assert.equal(knowledgeConnectionStatusLabel('provider_unavailable'), 'Integration pending');
  assert.equal(deriveKnowledgeConnectionName('Security Notion!'), 'security-notion');
  assert.deepEqual(knowledgeSyncStatusLabel('cached', true), { label: 'Cached', tone: 'green' });
  assert.equal(validateKnowledgeConnectionDraft({
    team: 'security',
    provider: 'notion',
    name: 'security-notion',
    display_name: 'Security Notion',
    base_url: '',
    credential_ref: '',
  }, connections), 'A connection with that identifier already exists.');
  assert.equal(validateKnowledgeConnectionDraft({
    team: 'security',
    provider: 'notion',
    name: 'security-notion',
    display_name: 'Security Notion',
    base_url: '',
    credential_ref: '',
  }, connections, 'security/security-notion'), '');
  assert.equal(validateKnowledgeExternalDraft({
    connection_id: 'security/security-notion',
    external_page_id: '',
    external_page_url: 'https://example.test/page',
    sync_mode: 'manual',
    failure_mode: 'fail',
    content: 'Cached page',
  }, 'security', connections), '');
  assert.match(validateKnowledgeExternalDraft({
    connection_id: 'security/security-notion',
    external_page_id: '',
    external_page_url: 'notaurl',
    sync_mode: 'manual',
    failure_mode: 'fail',
    content: '',
  }, 'security', connections), /valid page URL/);
});
