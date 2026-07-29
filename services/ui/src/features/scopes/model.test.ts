import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildScopePipelineMeta,
  buildScopeTree,
  canonicalizeTriggerEvent,
  countScopesRecursive,
  createInitialScopeData,
  decodeScopeFromRoute,
  encodeScopeForRoute,
  extractPipelineSecrets,
  extractScopeVariables,
  extractTriggerPipelines,
  filterVisibleScopeList,
  formatScopeDisplay,
  getScopeTreeNode,
  teamScopedItems,
  isEditableScopeSource,
  normalizeItemListPayload,
  normalizeRepositorySlug,
  normalizeScopePipelineList,
  normalizeScopeLabel,
  normalizeTriggerOverrideSlugs,
  parentScopeTeam,
  parseScopedIdentity,
  parseScopeYamlSafe,
  scopeSourceLabel,
  suggestCloneName,
} from './model.js';

test('normalizes scope routes and repository identities', () => {
  assert.equal(normalizeScopeLabel('/Default/'), '');
  assert.equal(encodeScopeForRoute('teams/platform'), 'teams/platform');
  assert.equal(decodeScopeFromRoute(['teams', 'platform']), 'teams/platform');
  assert.equal(normalizeRepositorySlug('/acme/control-plane/'), 'acme/control-plane');
  assert.deepEqual(parseScopedIdentity('acme/control-plane/token'), {
    repoOwner: 'acme',
    repoName: 'control-plane',
    repoSlug: 'acme/control-plane',
    name: 'token',
    fullName: 'acme/control-plane/token',
  });
});

test('normalizes scoped item metadata and clone names', () => {
  const result = normalizeItemListPayload([
    'GLOBAL_TOKEN',
    { name: 'acme/app/API_KEY', source: 'git repository', updated_at: '2026-06-07T10:00:00Z' },
    'GLOBAL_TOKEN',
  ]);
  assert.deepEqual(result.names, ['acme/app/API_KEY', 'GLOBAL_TOKEN']);
  assert.equal(result.meta['acme/app/API_KEY']?.source, 'git');
  assert.equal(suggestCloneName(['acme/app/token_copy'], 'acme/app', 'token'), 'token_copy_2');
});

test('builds scope trees with empty enterprise team teams', () => {
  const root = buildScopeTree(
    [{ scope: '', label: 'Default', teamPath: '', description: '', secretCountHint: 0 }],
    ['teams/platform']
  );
  assert.deepEqual(root.scopes, ['']);
  assert.equal(root.children[0]?.fullPath, 'teams');
  assert.equal(root.children[0]?.children[0]?.fullPath, 'teams/platform');
  assert.equal(countScopesRecursive(root), 1);
  assert.equal(getScopeTreeNode(root, 'teams/platform')?.fullPath, 'teams/platform');
  assert.equal(parentScopeTeam('teams/platform'), 'teams');
});

test('filters visible scopes by team and global search like resource collections', () => {
  const scopes = [
    { scope: '', label: 'Default Scope', teamPath: '', description: 'Fallback', secretCountHint: 0 },
    { scope: 'platform/dev', label: 'dev', teamPath: 'platform/dev', description: 'Development', secretCountHint: 1 },
    { scope: 'platform/prod', label: 'prod', teamPath: 'platform/prod', description: 'Production', secretCountHint: 2 },
    { scope: 'data/prod', label: 'prod', teamPath: 'data/prod', description: 'Analytics', secretCountHint: 3 },
  ];

  assert.deepEqual(filterVisibleScopeList(scopes, '', '').map(scope => scope.scope), [
    '',
    'data/prod',
    'platform/dev',
    'platform/prod',
  ]);
  assert.deepEqual(filterVisibleScopeList(scopes, '', 'platform').map(scope => scope.scope), [
    'platform/dev',
    'platform/prod',
  ]);
  assert.deepEqual(filterVisibleScopeList(scopes, 'analytics', 'platform').map(scope => scope.scope), [
    'data/prod',
  ]);
});

test('teams scoped items and preserves GitOps source behavior', () => {
  assert.deepEqual(teamScopedItems(['GLOBAL_TOKEN', 'acme/api/API_KEY', 'acme/api/DB_URL']), {
    global: [{ full: 'GLOBAL_TOKEN', display: 'GLOBAL_TOKEN' }],
    repositories: [
      {
        repo: 'acme/api',
        items: [
          { full: 'acme/api/API_KEY', display: 'API_KEY' },
          { full: 'acme/api/DB_URL', display: 'DB_URL' },
        ],
      },
    ],
  });
  assert.equal(isEditableScopeSource('git'), true);
  assert.equal(isEditableScopeSource('database'), true);
  assert.equal(scopeSourceLabel('git'), 'GitOps');
  assert.equal(formatScopeDisplay('teams/platform'), '/teams/platform');
  assert.deepEqual(createInitialScopeData(), {
    variables: [],
    variableMeta: {},
    variablesLoaded: false,
    variablesLoading: false,
    secrets: [],
    secretMeta: {},
    secretsLoaded: false,
    secretsLoading: false,
  });
});

test('builds scope usage indexes from pipeline and trigger payloads', () => {
  const manifest = parseScopeYamlSafe(`
name: deploy
version: v2
variables:
  REGION: eu-west-1
steps:
  - name: publish
    secrets: [REGISTRY_TOKEN]
    variables: [IMAGE_TAG]
    tasks:
      - name: notify
        variables:
          CHANNEL: alerts
`);
  assert.deepEqual(extractPipelineSecrets(manifest), ['REGISTRY_TOKEN']);
  assert.deepEqual(extractScopeVariables(manifest).sort(), ['CHANNEL', 'IMAGE_TAG', 'REGION', 'alerts', 'eu-west-1']);
  assert.deepEqual(extractTriggerPipelines(['pipelines/team/deploy.yaml', { path: 'pipelines/team/test.yml' }]), [
    'team/deploy',
    'team/test',
  ]);
  assert.equal(canonicalizeTriggerEvent('pull-request'), 'pull_request');

  const pipelines = normalizeScopePipelineList([
    { id: 'team/deploy', path: 'pipelines/team/deploy.yaml', version: 'v2', source: 'git' },
    'team/test',
  ]);
  assert.deepEqual(pipelines.identifiers, ['team/deploy', 'team/test']);
  assert.deepEqual(buildScopePipelineMeta('team/deploy', manifest, pipelines.seeds.get('team/deploy')), {
    identifier: 'team/deploy',
    name: 'deploy',
    description: '',
    path: 'team/deploy',
    version: 'v2',
    source: 'GitOps',
  });
  assert.deepEqual(normalizeTriggerOverrideSlugs([
    'acme/api',
    { repo_owner: 'acme', repository: 'web' },
  ]), ['acme/api', 'acme/web']);
});
