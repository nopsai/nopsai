import { useState, type ComponentProps } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import SystemConfigWorkspace from './SystemConfigWorkspace';
import {
  emptyConfigRepositoryForm,
  emptyNotificationMailSettingsForm,
  initialConfig,
  type ConfigFormState,
  type ConfigRepository,
  type ConfigRepositoryFormState,
  type NotificationMailSettingsFormState,
} from './model';

function renderWorkspace(overrides: Partial<ComponentProps<typeof SystemConfigWorkspace>> = {}) {
  const actions = {
    onChange: vi.fn(),
    onReload: vi.fn(async () => undefined),
    onSave: vi.fn(async () => undefined),
    onSaveMailSettings: vi.fn(async () => undefined),
    onTestMailSettings: vi.fn(async () => undefined),
    onSaveGlobalConfigRepo: vi.fn(async () => undefined),
    onDeleteGlobalConfigRepo: vi.fn(async () => undefined),
    onSyncGlobalConfigRepo: vi.fn(async () => undefined),
    onCheckGlobalConfigRepoDrift: vi.fn(async () => undefined),
  };

  function Harness() {
    const [config, setConfig] = useState<ConfigFormState>({
      ...initialConfig,
      environment: 'production',
      log_level: 'info',
      log_format: 'json',
      runner_id: 'runner-general',
      runner_scopes: 'dev,prod',
      runner_capacity: '2',
      runtime_pools: {
        default: {
          node_selector: { workload: 'nopsai' },
          resources: {
            requests: {},
            limits: {},
          },
        },
      },
    });
    const [mailSettingsForm, setMailSettingsForm] = useState<NotificationMailSettingsFormState>({
      ...emptyNotificationMailSettingsForm,
      enabled: true,
      from: 'nopsai@example.com',
      smtp_host: 'smtp.example.com',
      smtp_password_credential_ref: 'credential://system/mail/smtp-password',
    });
    const [globalConfigRepoForm, setGlobalConfigRepoForm] = useState<ConfigRepositoryFormState>({
      ...emptyConfigRepositoryForm,
      repo_url: 'https://github.com/acme/nopsai-config',
      credential_ref: '',
      write_enabled: true,
      write_branch: 'nopsai/ui-changes',
    });
    const globalConfigRepo: ConfigRepository = {
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
      last_sync_commit_sha: '1234567890abcdef',
    };

    return (
      <SystemConfigWorkspace
        config={config}
        envFilePath=".env"
        fieldMetadata={{
          environment: { scope: 'runtime_live', label: 'Environment', section: 'General', apply: 'Applied immediately' },
        }}
        configError={null}
        configLoading={false}
        saving={false}
        globalConfigRepo={globalConfigRepo}
        globalConfigRepoForm={globalConfigRepoForm}
        globalConfigRepoLoading={false}
        globalConfigRepoSaving={false}
        globalConfigRepoSyncing={false}
        globalConfigRepoError={null}
        mailSettings={{
          enabled: true,
          from: 'nopsai@example.com',
          smtp: {
            host: 'smtp.example.com',
            port: 587,
            start_tls: true,
            username: 'nopsai@example.com',
            password_credential_ref: 'credential://system/mail/smtp-password',
          },
        }}
        mailSettingsForm={mailSettingsForm}
        mailSettingsLoading={false}
        mailSettingsSaving={false}
        mailSettingsTesting={false}
        mailSettingsError={null}
        onChange={next => {
          actions.onChange(next);
          setConfig(next);
        }}
        onReload={actions.onReload}
        onSave={actions.onSave}
        onMailSettingsChange={setMailSettingsForm}
        onSaveMailSettings={actions.onSaveMailSettings}
        onTestMailSettings={actions.onTestMailSettings}
        onGlobalConfigRepoChange={setGlobalConfigRepoForm}
        onSaveGlobalConfigRepo={actions.onSaveGlobalConfigRepo}
        onDeleteGlobalConfigRepo={actions.onDeleteGlobalConfigRepo}
        onSyncGlobalConfigRepo={actions.onSyncGlobalConfigRepo}
        onCheckGlobalConfigRepoDrift={actions.onCheckGlobalConfigRepoDrift}
        globalConfigRepoDriftLoading={false}
        globalConfigRepoPushing={false}
        canViewRuntimeConfig
        canManageRuntimeConfig
        canViewGlobalConfigRepo
        canManageGlobalConfigRepo
        canViewDispatcher
        {...overrides}
      />
    );
  }

  render(
    <MemoryRouter>
      <Harness />
    </MemoryRouter>
  );

  return actions;
}

test('renders the settings hub and filters sections by search', async () => {
  const user = userEvent.setup();
  renderWorkspace();

  expect(screen.getByText('Settings Config')).toBeVisible();
  expect(screen.getByText('Platform Identity')).toBeVisible();
  expect(screen.getByRole('tab', { name: /Platform/ })).toHaveAttribute('aria-selected', 'true');
  expect(screen.getByRole('tab', { name: /Config Source/ })).toBeVisible();
  expect(screen.getByRole('link', { name: 'Open Dispatcher' })).toHaveAttribute('href', '/system/dispatcher');

  await user.click(screen.getByRole('tab', { name: /Config Source/ }));
  expect(screen.getByText('Global Config Repository')).toBeVisible();

  await user.type(screen.getByRole('searchbox', { name: 'Search settings' }), 'smtp');

  expect(screen.getByText('Mail Notifications')).toBeVisible();
  expect(screen.queryByText('Platform Identity')).not.toBeInTheDocument();
  expect(screen.queryByText('Global Config Repository')).not.toBeInTheDocument();

  await user.clear(screen.getByRole('searchbox', { name: 'Search settings' }));
  await user.type(screen.getByRole('searchbox', { name: 'Search settings' }), 'no-result');

  expect(screen.getByText('No matching settings')).toBeVisible();
});

test('updates runtime config fields and saves the form', async () => {
  const user = userEvent.setup();
  const actions = renderWorkspace();

  await user.selectOptions(screen.getByLabelText(/Environment/), 'staging');
  expect(actions.onChange).toHaveBeenLastCalledWith(expect.objectContaining({ environment: 'staging' }));

  await user.click(screen.getByLabelText(/Require production gates/));
  expect(actions.onChange).toHaveBeenLastCalledWith(expect.objectContaining({ require_production_gates: true }));

  await user.click(screen.getAllByRole('button', { name: /Save settings/ })[0]);
  expect(actions.onSave).toHaveBeenCalled();
});

test('keeps mail actions wired to the notification handlers', async () => {
  const user = userEvent.setup();
  const actions = renderWorkspace();

  await user.click(screen.getByRole('tab', { name: /Notifications/ }));
  await user.type(screen.getByLabelText('Test recipient'), 'operator@example.com');
  await user.click(screen.getByRole('button', { name: 'Send test' }));
  await user.click(screen.getByRole('button', { name: 'Save mail' }));

  expect(actions.onTestMailSettings).toHaveBeenCalled();
  expect(actions.onSaveMailSettings).toHaveBeenCalled();
});

test('keeps GitOps repository actions wired through the config repo handlers', async () => {
  const user = userEvent.setup();
  const actions = renderWorkspace();

  await user.click(screen.getByRole('tab', { name: /Config Source/ }));
  await user.click(screen.getByRole('button', { name: 'Check drift' }));
  await user.click(screen.getByRole('button', { name: 'Review & push' }));
  await user.click(screen.getByRole('button', { name: 'Sync' }));
  await user.click(screen.getByRole('button', { name: 'Save repository' }));
  await user.click(screen.getByRole('button', { name: 'Remove' }));

  expect(actions.onCheckGlobalConfigRepoDrift).toHaveBeenCalledTimes(2);
  expect(actions.onSyncGlobalConfigRepo).toHaveBeenCalled();
  expect(actions.onSaveGlobalConfigRepo).toHaveBeenCalled();
  expect(actions.onDeleteGlobalConfigRepo).toHaveBeenCalled();
});

test('disables runtime edits when AAA grants read-only access', async () => {
  const user = userEvent.setup();
  renderWorkspace({ canManageRuntimeConfig: false, canManageGlobalConfigRepo: false });

  expect(screen.getByLabelText(/Environment/)).toBeDisabled();
  expect(screen.getAllByRole('button', { name: /Save settings/ })[0]).toBeDisabled();

  await user.click(screen.getByRole('tab', { name: /Config Source/ }));
  expect(screen.getByText('Read-only')).toBeVisible();
});
