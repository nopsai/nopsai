import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildGitWebhookSourceTreeItems,
  buildGitWebhookSourceMetrics,
  deliveryStatusClass,
  filterGitWebhookSources,
  formatGitWebhookDate,
  gitWebhookSourceBelongsToTeam,
  gitWebhookSourceForm,
  gitWebhookSourceRequest,
  gitWebhookSourceTeamLabel,
  gitWebhookSourceTeamPath,
  sourceStatusLabel,
} from './model.js';

test('builds a normalized Git webhook source request', () => {
  const request = gitWebhookSourceRequest({
    ...gitWebhookSourceForm(),
    id: 'gitlab-platform',
    name: 'GitLab Platform',
    provider: 'gitlab',
    authMode: 'static_token',
    teamPath: 'platform/prod',
    credentialRef: 'credential://system/webhooks/gitlab-platform',
    repositoryAllowlistText: 'Platform/API\nplatform/*\nplatform/api',
    rateLimitPerMinute: '120',
  });

  assert.deepEqual(request.repository_allowlist, ['platform/api', 'platform/*']);
  assert.deepEqual(request.rate_limit, { per_minute: 120 });
  assert.equal(request.team_path, 'platform/prod');
  assert.equal(request.credential_ref, 'credential://system/webhooks/gitlab-platform');
});

test('requires an allowlist and credential reference for authenticated sources', () => {
  assert.throws(
    () => gitWebhookSourceRequest({ ...gitWebhookSourceForm(), id: 'source' }),
    /owner\/repository/
  );
  assert.throws(
    () => gitWebhookSourceRequest({
      ...gitWebhookSourceForm(),
      id: 'source',
      repositoryAllowlistText: 'owner/repo',
      credentialRef: '',
    }),
    /credential:\/\//
  );
  assert.throws(
    () => gitWebhookSourceRequest({
      ...gitWebhookSourceForm(),
      id: 'invalid/source',
      repositoryAllowlistText: 'owner/repo',
    }),
    /letters, numbers/
  );
  assert.throws(
    () => gitWebhookSourceRequest({
      ...gitWebhookSourceForm(),
      id: 'source',
      repositoryAllowlistText: 'owner/repo',
      credentialRef: 'credential://System/webhooks/source',
    }),
    /credential:\/\//
  );
  assert.throws(
    () => gitWebhookSourceRequest({
      ...gitWebhookSourceForm(),
      id: 'source',
      repositoryAllowlistText: 'owner/repo',
      credentialRef: 'credential://system/webhooks/source',
      rateLimitPerMinute: '1.5',
    }),
    /positive whole number/
  );
});

test('omits credentials for internal unauthenticated sources and maps delivery status tones', () => {
  const request = gitWebhookSourceRequest({
    ...gitWebhookSourceForm(),
    id: 'internal',
    authMode: 'none',
    credentialRef: 'credential://system/ignored',
    repositoryAllowlistText: 'owner/repo',
  });

  assert.equal(request.credential_ref, undefined);
  assert.equal(deliveryStatusClass('processed'), 'runner-pill--ok');
  assert.equal(deliveryStatusClass('failed'), 'runner-pill--error');
  assert.equal(deliveryStatusClass('no_match'), 'runner-pill--muted');
});

test('maps source state, stored forms, dates, and remaining delivery tones', () => {
  const source = {
    id: 'source',
    name: 'Source',
    description: '',
    provider: 'generic' as const,
    enabled: true,
    auth_mode: 'hmac' as const,
    repository_allowlist: ['owner/repo'],
    rate_limit: { per_minute: 30 },
  };

  assert.deepEqual(gitWebhookSourceForm(source), {
    ...gitWebhookSourceForm(),
    id: 'source',
    name: 'Source',
    teamPath: 'root',
    repositoryAllowlistText: 'owner/repo',
    rateLimitPerMinute: '30',
  });
  assert.equal(sourceStatusLabel(source), 'Credential required');
  assert.equal(sourceStatusLabel({ ...source, enabled: false }), 'Disabled');
  assert.equal(sourceStatusLabel({ ...source, auth_mode: 'none' }), 'Enabled');
  assert.equal(deliveryStatusClass('partial'), 'runner-pill--warning');
  assert.equal(deliveryStatusClass('pending'), 'runner-pill--warning');
  assert.equal(formatGitWebhookDate(), 'Never');
  assert.equal(formatGitWebhookDate('not-a-date'), 'not-a-date');
  assert.notEqual(formatGitWebhookDate('2026-06-15T10:00:00Z'), '2026-06-15T10:00:00Z');
});

test('builds Git webhook source workspace metrics and filters by source fields', () => {
  const sources = [
    {
      id: 'gitlab-platform',
      name: 'GitLab Platform',
      description: 'Primary GitLab source',
      provider: 'gitlab' as const,
      enabled: true,
      auth_mode: 'static_token' as const,
      credential_ref: 'credential://system/webhooks/gitlab-platform',
      repository_allowlist: ['platform/*'],
      rate_limit: {},
      managed_by_config_repo: true,
    },
    {
      id: 'internal',
      name: 'Internal',
      description: '',
      provider: 'generic' as const,
      enabled: false,
      auth_mode: 'none' as const,
      repository_allowlist: ['internal/*'],
      rate_limit: {},
    },
  ];

  assert.deepEqual(buildGitWebhookSourceMetrics(sources), {
    total: 2,
    enabled: 1,
    gitManaged: 1,
    secured: 1,
  });
  assert.deepEqual(filterGitWebhookSources(sources, 'platform/*').map(source => source.id), ['gitlab-platform']);
  assert.deepEqual(filterGitWebhookSources(sources, 'disabled').map(source => source.id), ['internal']);
});

test('builds Git webhook source team tree items with global fallback', () => {
  const sources = [
    {
      id: 'global',
      name: 'Global',
      description: '',
      provider: 'generic' as const,
      enabled: true,
      auth_mode: 'none' as const,
      repository_allowlist: ['global/*'],
      rate_limit: {},
    },
    {
      id: 'platform',
      name: 'Platform',
      description: '',
      provider: 'gitlab' as const,
      enabled: true,
      auth_mode: 'hmac' as const,
      repository_allowlist: ['platform/*'],
      rate_limit: {},
      team_path: 'platform/prod',
      source: 'git',
    },
  ];

  assert.equal(gitWebhookSourceTeamPath(sources[0]), '');
  assert.equal(gitWebhookSourceTeamLabel(sources[0]), 'Global');
  assert.equal(gitWebhookSourceTeamPath(sources[1]), 'platform/prod');
  assert.equal(gitWebhookSourceForm(sources[1]).teamPath, 'platform/prod');
  assert.equal(gitWebhookSourceRequest({
    ...gitWebhookSourceForm(sources[1]),
    credentialRef: 'credential://system/webhooks/platform',
  }).team_path, 'platform/prod');
  assert.deepEqual(buildGitWebhookSourceTreeItems(sources), [
    { id: 'global', label: 'Global', path: '', source: undefined },
    { id: 'platform', label: 'Platform', path: 'platform/prod', source: 'git' },
  ]);
  assert.deepEqual(
    sources.filter(source => gitWebhookSourceBelongsToTeam(source, 'platform')).map(source => source.id),
    ['platform']
  );
});
