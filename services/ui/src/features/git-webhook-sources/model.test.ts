import assert from 'node:assert/strict';
import test from 'node:test';
import {
  deliveryStatusClass,
  formatGitWebhookDate,
  gitWebhookSourceForm,
  gitWebhookSourceRequest,
  sourceStatusLabel,
} from './model.js';

test('builds a normalized Git webhook source request', () => {
  const request = gitWebhookSourceRequest({
    ...gitWebhookSourceForm(),
    id: 'gitlab-platform',
    name: 'GitLab Platform',
    provider: 'gitlab',
    authMode: 'static_token',
    credentialRef: 'credential://system/webhooks/gitlab-platform',
    repositoryAllowlistText: 'Platform/API\nplatform/*\nplatform/api',
    rateLimitPerMinute: '120',
  });

  assert.deepEqual(request.repository_allowlist, ['platform/api', 'platform/*']);
  assert.deepEqual(request.rate_limit, { per_minute: 120 });
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
