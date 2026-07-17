import assert from 'node:assert/strict';
import test from 'node:test';
import { normalizeTeamCatalogPayload } from './api.js';
import { buildAccessResourceCatalog } from './resourceCatalog.js';

function emptyCatalogSources(teams: unknown[]) {
  return {
    teams,
    pipelines: [],
    triggers: [],
    externalTriggers: [],
    gitWebhookSources: [],
    credentials: [],
    secretScopes: [],
    variableScopes: [],
  };
}

test('normalizes Access team catalog without application records', () => {
  const teams = normalizeTeamCatalogPayload({
    teams: [
      { id: 1, slug: 'platform' },
      { id: 2, name: 'payments', parent_team_id: 1 },
    ],
    applications: [
      { id: 3, name: 'checkout-api', repository_full_name: 'acme/checkout-api', team_id: 2 },
    ],
  });

  const catalog = buildAccessResourceCatalog(emptyCatalogSources(teams));

  assert.deepEqual(catalog.teamOptions.map(option => option.value), [
    'platform',
    'platform/payments',
  ]);
});

test('drops application-shaped records from legacy team catalog arrays', () => {
  const teams = normalizeTeamCatalogPayload([
    { id: 1, name: 'platform' },
    { id: 2, name: 'checkout-api', kind: 'app', parent_id: 1 },
    { id: 3, name: 'acme/docs', parent_id: 1 },
    { id: 4, name: 'docs', repository_full_name: 'acme/docs', parent_id: 1 },
  ]);

  const catalog = buildAccessResourceCatalog(emptyCatalogSources(teams));

  assert.deepEqual(catalog.teamOptions.map(option => option.value), ['platform']);
});
