import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildTriggerSummary,
  buildTriggerCollectionMetrics,
  buildNewTriggerYaml,
  deriveDefaultPipelinePath,
  encodeTriggerSlug,
  filterTriggerListItems,
  parseTriggerOverrideList,
  triggerAllowlistStatusLabel,
  triggerBelongsToOwner,
  triggerIngressLabel,
  triggerManagementLabel,
  triggerDetailsFormFromYaml,
  triggerDetailsWithProvider,
  triggerTeamLabel,
  applyTriggerDetailsToYaml,
  parseTriggerYaml,
  splitTriggerSlug,
  triggerSlugLabel,
  validateTriggerYaml,
} from './model.js';

test('normalizes trigger manifests and linked pipeline summaries', () => {
  const summary = buildTriggerSummary(
    parseTriggerYaml(`
triggers:
  - on: push
    branches: [main]
    pipelines:
      - pipelines/platform/deploy.yaml
    scope: default
`)
  );
  assert.equal(summary.triggerCount, 1);
  assert.deepEqual(summary.pipelines.map(item => item.identifier), ['platform/deploy']);
  assert.deepEqual(summary.scopes, ['']);
});

test('normalizes trigger lists and default pipeline paths', () => {
  assert.deepEqual(parseTriggerOverrideList(['acme/api', { name: 'acme/web', source: 'gitops' }]), [
    { slug: 'acme/api', source: 'database' },
    { slug: 'acme/web', source: 'git' },
  ]);
  assert.equal(deriveDefaultPipelinePath('acme/payment api'), 'pipelines/payment-api.yaml');
  assert.deepEqual(parseTriggerOverrideList([{
    name: 'acme/api',
    source: 'gitops',
    provider: 'gitlab',
    team_path: 'team-1',
    management: 'nopsai',
    webhook_source_id: 'corporate-gitlab',
    webhook_source_name: 'Corporate GitLab',
    allowlist_status: 'allowed',
  }]), [{
    slug: 'acme/api',
    source: 'git',
    provider: 'gitlab',
    teamPath: 'team-1',
    management: 'nopsai',
    webhookSourceID: 'corporate-gitlab',
    webhookSourceName: 'Corporate GitLab',
    allowlistStatus: 'allowed',
  }]);
  assert.equal(triggerManagementLabel('repository'), 'Repository');
  assert.equal(triggerTeamLabel('root'), 'Workspace');
  assert.equal(triggerAllowlistStatusLabel('allowed'), 'Allowed');
  assert.equal(triggerIngressLabel({ provider: 'gitlab', webhookSourceName: 'Corporate GitLab' }), 'Corporate GitLab');
});

test('edits trigger root details through structured YAML helpers', () => {
  const raw = buildNewTriggerYaml('pipelines/api.yaml', {
    provider: 'gitlab',
    teamPath: 'team-1',
    management: 'nopsai',
    webhookSourceID: 'corporate-gitlab',
  });
  const details = triggerDetailsFormFromYaml(raw);
  assert.deepEqual(details, {
    provider: 'gitlab',
    teamPath: 'team-1',
    management: 'nopsai',
    webhookSourceID: 'corporate-gitlab',
  });

  const githubDetails = triggerDetailsWithProvider(details, 'github');
  assert.equal(githubDetails.webhookSourceID, '');
  const updated = applyTriggerDetailsToYaml(raw, githubDetails);
  assert.equal(parseTriggerYaml(updated).provider, 'github');
  assert.equal(parseTriggerYaml(updated).webhook_source, undefined);
});

test('builds owner-scoped trigger collection metrics', () => {
  const triggers = [
    { slug: 'platform/api', source: 'gitops' },
    { slug: 'platform/web', source: 'database' },
    { slug: 'platform/apps/checkout', source: 'git' },
    { slug: 'security/audit', source: 'database' },
  ];

  assert.equal(triggerBelongsToOwner('platform/apps/checkout', 'platform'), true);
  assert.equal(triggerBelongsToOwner('security/audit', 'platform'), false);
  assert.deepEqual(buildTriggerCollectionMetrics(triggers, 'platform'), {
    total: 3,
    gitManaged: 2,
    databaseManaged: 1,
    ownerCount: 2,
  });
});

test('filters trigger list items by query and source label', () => {
  const triggers = [
    { slug: 'platform/api', source: 'gitops' },
    { slug: 'platform/web', source: 'database' },
    { slug: 'security/audit', source: 'database' },
  ];

  assert.deepEqual(filterTriggerListItems(triggers, { query: 'git', source: 'all' }), [
    { slug: 'platform/api', source: 'gitops' },
  ]);
  assert.deepEqual(filterTriggerListItems(triggers, { query: 'platform', source: 'database' }), [
    { slug: 'platform/web', source: 'database' },
  ]);
});

test('validates trigger manifests and repository routes', () => {
  assert.deepEqual(splitTriggerSlug('acme/platform/api'), { owner: 'acme/platform', repo: 'api' });
  assert.equal(encodeTriggerSlug('acme/platform api'), 'acme/platform%20api');
  assert.deepEqual(triggerSlugLabel('acme/platform/api'), { name: 'api', path: 'acme/platform' });

  const valid = validateTriggerYaml(`
provider: gitlab
team: team-1
webhook_source: corporate-gitlab
triggers:
  - on: push
    branches: [main]
    pipelines:
      - pipelines/platform/deploy.yaml
    scope: production
`);
  assert.deepEqual(valid.errors, []);

  const invalid = validateTriggerYaml(`
steps: []
triggers:
  - on: unsupported
    branches: [""]
    pipelines: []
    scope: ""
`);
  assert.ok(invalid.errors.some(error => error.message.includes('appears to be a pipeline')));
  assert.ok(invalid.errors.some(error => error.message.includes("unsupported event 'unsupported'")));
  assert.ok(invalid.errors.some(error => error.message.includes('at least one pipeline reference')));
  assert.ok(invalid.errors.some(error => error.message.includes('empty branches entry')));
  assert.ok(invalid.errors.some(error => error.message.includes("empty 'scope'")));
});
