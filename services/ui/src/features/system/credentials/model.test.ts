import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildCredentialReference,
  credentialCatalogGroups,
  credentialNamespaces,
  credentialPayloadFromForm,
  credentialReferenceRoute,
  credentialReferenceDisplay,
  credentialSummary,
  filterCredentials,
  teamCredentials,
  isCredentialReference,
  normalizeCredential,
  normalizeCredentialsPayload,
  parseCredentialReference,
  recentlyUpdatedCredentials,
} from './model.js';

test('normalizes credential metadata without exposing encrypted fields', () => {
  const result = normalizeCredentialsPayload({
    credentials: [
      {
        id: '1',
        reference: 'credential://system/mail/smtp',
        kind: 'password',
        status: 'active',
        active_version: 2,
        has_value: true,
        ciphertext: 'must-not-appear',
      },
    ],
  });

  assert.equal(result.length, 1);
  assert.equal(result[0]?.reference, 'credential://system/mail/smtp');
  assert.equal(result[0]?.active_version, 2);
  assert.equal('ciphertext' in (result[0] || {}), false);
});

test('normalizes and sorts credential versions newest first', () => {
  const result = normalizeCredential({
    id: '1',
    reference: 'credential://system/llm/main',
    versions: [{ version: 1 }, { version: 3 }, { version: 0 }],
  });

  assert.deepEqual(result?.versions.map(version => version.version), [3, 1]);
});

test('builds a normalized credential create payload', () => {
  const result = credentialPayloadFromForm({
    namespace: ' SYSTEM ',
    team_path: '',
    name: ' /LLM/Main/ ',
    kind: 'api_key',
    description: ' Primary key ',
    value: 'secret',
    expires_at: '',
  });

  assert.deepEqual(result, {
    reference: 'credential://system/llm/main',
    team_path: undefined,
    kind: 'api_key',
    description: 'Primary key',
    value: 'secret',
    expires_at: undefined,
  });
});

test('builds a team-scoped credential create payload', () => {
  const result = credentialPayloadFromForm({
    namespace: 'system',
    team_path: ' /Platform/ML/ ',
    name: ' /LLM/Main/ ',
    kind: 'api_key',
    description: ' Team key ',
    value: 'secret',
    expires_at: '',
  });

  assert.deepEqual(result, {
    reference: 'credential://team/platform/ml/llm/main',
    team_path: 'platform/ml',
    kind: 'api_key',
    description: 'Team key',
    value: 'secret',
    expires_at: undefined,
  });

  assert.equal(
    buildCredentialReference('system', 'platform/ml/llm/main', 'platform/ml'),
    'credential://team/platform/ml/llm/main'
  );
});

test('builds and parses references for compact credential presentation', () => {
  assert.equal(buildCredentialReference(' Platform ', '/GitHub/App-Key/'), 'credential://platform/github/app-key');
  assert.equal(buildCredentialReference('system', 'llm/openai', 'platform/ml'), 'credential://team/platform/ml/llm/openai');
  assert.deepEqual(parseCredentialReference('credential://system/oidc/keycloak/client-secret'), {
    namespace: 'system',
    name: 'oidc/keycloak/client-secret',
    category: 'oidc',
    displayName: 'client-secret',
    parentPath: 'keycloak',
  });
});

test('derives display grouping for team credentials with known team paths', () => {
  assert.deepEqual(
    credentialReferenceDisplay('credential://team/platform/ml/mail/smtp-primary', ['platform/ml']),
    {
      namespace: 'team',
      name: 'platform/ml/mail/smtp-primary',
      category: 'mail',
      displayName: 'smtp-primary',
      parentPath: '',
      scopeKind: 'team',
      scopePath: 'platform/ml',
      scopeLabel: 'platform/ml',
    }
  );
  assert.deepEqual(
    credentialReferenceDisplay('credential://team/platform/openai', []),
    {
      namespace: 'team',
      name: 'platform/openai',
      category: 'general',
      displayName: 'openai',
      parentPath: '',
      scopeKind: 'team',
      scopePath: 'platform',
      scopeLabel: 'platform',
    }
  );
  assert.deepEqual(
    credentialReferenceDisplay('credential://team/team-1/team-1/test', []),
    {
      namespace: 'team',
      name: 'team-1/team-1/test',
      category: 'general',
      displayName: 'test',
      parentPath: '',
      scopeKind: 'team',
      scopePath: 'team-1',
      scopeLabel: 'team-1',
    }
  );
});

test('builds deep links for credential references', () => {
  assert.equal(isCredentialReference('credential://system/llm/openai'), true);
  assert.equal(isCredentialReference('not-a-credential'), false);
  assert.equal(
    credentialReferenceRoute('credential://system/llm/openai'),
    '/credentials?credential=credential%3A%2F%2Fsystem%2Fllm%2Fopenai'
  );
});

test('summarizes, filters, and teams credentials by namespace and integration category', () => {
  const credentials = normalizeCredentialsPayload({
    credentials: [
      { id: '1', reference: 'credential://system/llm/openai', kind: 'api_key', status: 'active' },
      { id: '2', reference: 'credential://system/mail/smtp', kind: 'password', status: 'disabled' },
      { id: '3', reference: 'credential://tenant/llm/anthropic', kind: 'api_key', status: 'pending' },
    ],
  });

  assert.deepEqual(credentialSummary(credentials), { total: 3, active: 1, disabled: 1, pending: 1 });
  assert.deepEqual(credentialNamespaces(credentials), ['system', 'tenant']);
  assert.deepEqual(
    filterCredentials(credentials, 'smtp', 'disabled', 'system').map(credential => credential.id),
    ['2']
  );
  assert.deepEqual(
    teamCredentials(credentials).map(team => [team.key, team.credentials.length]),
    [['system/llm', 1], ['system/mail', 1], ['tenant/llm', 1]]
  );
});

test('groups catalog cards and sorts recently updated credentials', () => {
  const credentials = normalizeCredentialsPayload({
    credentials: [
      { id: '1', reference: 'credential://system/llm/openai', kind: 'api_key', status: 'active', updated_at: '2026-01-01T00:00:00Z' },
      { id: '2', reference: 'credential://team/platform/ml/mail/smtp', kind: 'password', status: 'active', updated_at: '2026-01-03T00:00:00Z' },
      { id: '3', reference: 'credential://global/github/app', kind: 'private_key', status: 'pending', updated_at: '2026-01-02T00:00:00Z' },
    ],
  });

  assert.deepEqual(
    credentialCatalogGroups(credentials, ['platform/ml']).map(group => [
      group.key,
      group.scopeKind,
      group.scopePath,
      group.categories.map(category => [category.category, category.credentials.map(credential => credential.id)]),
      group.credentials.map(credential => credential.id),
    ]),
    [
      ['team/platform/ml', 'team', 'platform/ml', [['mail', ['2']]], ['2']],
      ['shared/global', 'shared', 'global', [['github', ['3']]], ['3']],
      ['shared/system', 'shared', 'system', [['llm', ['1']]], ['1']],
    ]
  );
  assert.deepEqual(recentlyUpdatedCredentials(credentials, 2).map(credential => credential.id), ['2', '3']);
  assert.deepEqual(filterCredentials(credentials, '', 'all', 'shared').map(credential => credential.id), ['3', '1']);
});
