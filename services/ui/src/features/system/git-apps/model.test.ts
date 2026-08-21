import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildGitHubAppMetrics,
  filterGitHubAppInstallations,
  formatGitHubAppDate,
  gitHubAppForm,
  gitHubAppInstallationForm,
  gitHubAppInstallationPayloadFromForm,
  gitHubAppPayloadFromForm,
  gitHubInstallationStatusLabel,
  gitHubInstallationApprovalForm,
  gitHubInstallationStatusTone,
  installationDisplayName,
  normalizeGitHubAccountType,
  normalizeGitHubAppPayload,
} from './model.js';

test('normalizes GitHub App payloads and metrics', () => {
  const app = normalizeGitHubAppPayload({
    app_id: ' 123456 ',
    private_key_credential_ref: ' credential://system/github/app-private-key ',
    webhook_credential_ref: ' credential://system/github/webhook-secret ',
    webhook_url: ' https://nopsai.example.com/webhook ',
    webhook_endpoint: ' https://nopsai.example.com/webhook ',
    installations: [
      {
        installation_id: ' 987654 ',
        account_login: ' nopsai ',
        account_type: ' org ',
        enabled: true,
        accessible_repositories: '3',
        connected_triggers: 2,
        repositories: [{ id: 1, full_name: 'nopsai/api', private: true, used_by_nopsai: true }],
      },
      { installation_id: '', account_login: 'ignored' },
    ],
  });

  assert.equal(app.app_id, '123456');
  assert.equal(app.webhook_endpoint, 'https://nopsai.example.com/webhook');
  assert.equal(app.installations.length, 1);
  assert.equal(app.installations[0]?.account_type, 'organization');
  assert.equal(app.installations[0]?.repositories?.[0]?.full_name, 'nopsai/api');
  assert.deepEqual(buildGitHubAppMetrics(app), {
    installations: 1,
    enabled: 1,
    disabled: 0,
    pending: 0,
    repositories: 3,
    connectedTriggers: 2,
  });
});

test('builds GitHub App and installation requests without legacy scalar fields', () => {
  const app = normalizeGitHubAppPayload({
    app_id: '123456',
    private_key_credential_ref: 'credential://system/github/app-private-key',
    webhook_credential_ref: 'credential://system/github/webhook-secret',
    webhook_url: 'https://nopsai.example.com/webhook',
    installations: [{
      installation_id: '987654',
      account_login: 'nopsai',
      account_type: 'organization',
      enabled: true,
      accessible_repositories: 3,
      repositories: [{ id: 1, full_name: 'nopsai/api' }],
    }],
  });

  assert.deepEqual(gitHubAppForm(app), {
    appID: '123456',
    privateKeyCredentialRef: 'credential://system/github/app-private-key',
    webhookCredentialRef: 'credential://system/github/webhook-secret',
    webhookURL: 'https://nopsai.example.com/webhook',
  });
  const payload = gitHubAppPayloadFromForm(gitHubAppForm(app), app.installations);
  assert.equal(payload.provider, 'github');
  assert.equal(payload.app_id, '123456');
  assert.equal(Object.hasOwn(payload, 'github_installation_id'), false);
  assert.equal(payload.installations[0]?.installation_id, '987654');
  assert.equal(payload.installations[0]?.accessible_repositories, 3);
  assert.equal(payload.installations[0]?.repositories, undefined);

  assert.deepEqual(gitHubAppInstallationForm(app.installations[0]), {
    installationID: '987654',
    accountLogin: 'nopsai',
    accountType: 'organization',
    enabled: true,
  });
  assert.deepEqual(gitHubAppInstallationPayloadFromForm({
    installationID: ' 987654 ',
    accountLogin: ' nopsai ',
    accountType: 'org' as 'organization',
    enabled: false,
  }), {
    installation_id: '987654',
    account_login: 'nopsai',
    account_type: 'organization',
    enabled: false,
    pending_approval: false,
    accessible_repositories: 0,
    connected_triggers: 0,
    status: 'disabled',
  });
});

test('validates GitHub App identifiers, credentials, and owner routing metadata', () => {
  assert.throws(
    () => gitHubAppPayloadFromForm({
      appID: 'abc',
      privateKeyCredentialRef: '',
      webhookCredentialRef: '',
      webhookURL: '',
    }, []),
    /App ID/
  );
  assert.throws(
    () => gitHubAppPayloadFromForm({
      appID: '123456',
      privateKeyCredentialRef: 'credential://System/github/key',
      webhookCredentialRef: '',
      webhookURL: '',
    }, []),
    /credential:\/\//
  );
  assert.throws(
    () => gitHubAppInstallationPayloadFromForm({
      installationID: 'abc',
      accountLogin: 'nopsai',
      accountType: 'organization',
      enabled: true,
    }),
    /Installation ID/
  );
  assert.throws(
    () => gitHubAppInstallationPayloadFromForm({
      installationID: '987654',
      accountLogin: 'bad_owner',
      accountType: 'organization',
      enabled: true,
    }),
    /GitHub owner/
  );
});

test('filters, formats, and labels GitHub App installations', () => {
  const installations = normalizeGitHubAppPayload({
    installations: [
      { installation_id: '1', account_login: 'acme', account_type: 'organization', enabled: true },
      { installation_id: '2', account_login: 'beta', account_type: 'user', enabled: false },
      { installation_id: '3', enabled: true, last_error: 'missing token' },
    ],
  }).installations;

  assert.deepEqual(filterGitHubAppInstallations(installations, 'beta').map(item => item.installation_id), ['2']);
  assert.equal(installationDisplayName(installations[0]!), 'acme');
  assert.equal(gitHubInstallationStatusLabel(installations[0]!), 'Connected');
  assert.equal(gitHubInstallationStatusTone(installations[0]!), 'ok');
  assert.equal(gitHubInstallationStatusLabel(installations[1]!), 'Disabled');
  assert.equal(gitHubInstallationStatusTone(installations[1]!), 'muted');
  assert.equal(gitHubInstallationStatusLabel(installations[2]!), 'Error');
  assert.equal(gitHubInstallationStatusTone(installations[2]!), 'error');

  // A held installation reads differently from one the operator switched off:
  // it is waiting on a decision, not the result of one.
  const held = normalizeGitHubAppPayload({
    installations: [{ installation_id: '4', account_login: 'stranger', enabled: false, pending_approval: true }],
  }).installations[0]!;
  assert.equal(gitHubInstallationStatusLabel(held), 'Pending approval');
  assert.equal(gitHubInstallationStatusTone(held), 'warning');
  assert.equal(gitHubInstallationApprovalForm(held).enabled, true);
  assert.equal(gitHubInstallationApprovalForm(held).installationID, '4');
  assert.equal(normalizeGitHubAccountType('ORG'), 'organization');
  assert.equal(formatGitHubAppDate(), 'Never');
  assert.equal(formatGitHubAppDate('not-a-date'), 'not-a-date');
  assert.notEqual(formatGitHubAppDate('2026-06-15T10:00:00Z'), '2026-06-15T10:00:00Z');
});
