import assert from 'node:assert/strict';
import { test } from 'node:test';
import { initialConfig } from './model.js';
import {
  buildSystemSettingsSummary,
  filterSystemSettingsSections,
  getSystemSettingsSectionCount,
} from './settingsPresentation.js';

test('filters settings sections by field label and ownership keyword', () => {
  assert.deepEqual(
    filterSystemSettingsSections('smtp').map(section => section.id),
    ['notifications']
  );
  assert.deepEqual(
    filterSystemSettingsSections('dispatcher').map(section => section.id),
    ['execution', 'networking']
  );
});

test('counts section-owned fields for navigation badges', () => {
  assert.equal(getSystemSettingsSectionCount('platform'), 9);
  assert.equal(getSystemSettingsSectionCount('source'), 8);
});

test('builds config summary cards from runtime, mail, and GitOps state', () => {
  const cards = buildSystemSettingsSummary({
    config: {
      ...initialConfig,
      environment: 'production',
      log_level: 'info',
      log_format: 'json',
      runtime_pools: {
        default: {
          node_selector: {},
          resources: {
            requests: {},
            limits: {},
          },
        },
      },
    },
    envFilePath: '.env.production',
    globalConfigRepo: {
      id: 1,
      scope_type: 'system',
      scope_id: 'global',
      provider: 'github',
      repo_url: 'https://github.com/acme/nopsai-config',
      branch: 'main',
      base_path: '',
      credential_ref: '',
      enabled: true,
      write_enabled: true,
      write_branch: 'nopsai/ui-changes',
      last_sync_status: 'success',
    },
    mailSettings: {
      enabled: true,
      from: 'nopsai@example.com',
      smtp: {
        host: 'smtp.example.com',
        port: 587,
        start_tls: true,
        username: 'nopsai@example.com',
        password_credential_ref: 'credential://system/mail/smtp-password',
      },
      managed_by_config_repo: true,
    },
    canViewGlobalConfigRepo: true,
  });

  assert.deepEqual(cards.map(card => [card.id, card.value, card.detail, card.tone]), [
    ['environment', 'production', 'Logging info / json', 'success'],
    ['runtime-pools', '1', 'Scheduling profile', 'success'],
    ['mail', 'Enabled', 'GitOps managed', 'success'],
    ['gitops', 'Enabled', 'success', 'success'],
  ]);
});
