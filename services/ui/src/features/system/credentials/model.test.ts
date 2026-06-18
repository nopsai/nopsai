import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildCredentialReference,
  credentialNamespaces,
  credentialPayloadFromForm,
  credentialReferenceRoute,
  credentialSummary,
  filterCredentials,
  groupCredentials,
  isCredentialReference,
  normalizeCredential,
  normalizeCredentialsPayload,
  parseCredentialReference,
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
    name: ' /LLM/Main/ ',
    kind: 'api_key',
    description: ' Primary key ',
    value: 'secret',
    expires_at: '',
  });

  assert.deepEqual(result, {
    reference: 'credential://system/llm/main',
    kind: 'api_key',
    description: 'Primary key',
    value: 'secret',
    expires_at: undefined,
  });
});

test('builds and parses references for compact credential presentation', () => {
  assert.equal(buildCredentialReference(' Platform ', '/GitHub/App-Key/'), 'credential://platform/github/app-key');
  assert.deepEqual(parseCredentialReference('credential://system/oidc/keycloak/client-secret'), {
    namespace: 'system',
    name: 'oidc/keycloak/client-secret',
    category: 'oidc',
    displayName: 'client-secret',
    parentPath: 'keycloak',
  });
});

test('builds deep links for credential references', () => {
  assert.equal(isCredentialReference('credential://system/llm/openai'), true);
  assert.equal(isCredentialReference('not-a-credential'), false);
  assert.equal(
    credentialReferenceRoute('credential://system/llm/openai'),
    '/system/credentials?credential=credential%3A%2F%2Fsystem%2Fllm%2Fopenai'
  );
});

test('summarizes, filters, and groups credentials by namespace and integration category', () => {
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
    groupCredentials(credentials).map(group => [group.key, group.credentials.length]),
    [['system/llm', 1], ['system/mail', 1], ['tenant/llm', 1]]
  );
});
