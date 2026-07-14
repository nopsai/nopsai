import { useState } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { EditTeamItemModal, NewTeamItemModal, TeamConfigRepositoryModal } from './TeamSettingsModals';
import { createEmptyNotificationRouteForm, defaultNotificationRouteDefinition } from './notificationRoutes';

const configRepo = {
  id: 7,
  scope_type: 'team',
  scope_id: 'platform',
  repo_url: 'https://github.com/acme/platform-config',
  branch: 'main',
  base_path: 'teams/platform',
  enabled: true,
  write_enabled: true,
  write_branch: 'nopsai/team-updates',
  last_sync_status: 'success',
  last_sync_message: 'Synced',
  last_sync_started_at: '2026-07-10T10:00:00Z',
  last_sync_completed_at: '2026-07-10T10:01:00Z',
  last_sync_commit_sha: 'abc123',
};

function createSettingsHandlers() {
  return {
    onSave: vi.fn().mockResolvedValue(undefined),
    onDelete: vi.fn().mockResolvedValue(undefined),
    onSync: vi.fn().mockResolvedValue(undefined),
    onCheckDrift: vi.fn().mockResolvedValue(undefined),
    onSaveNotification: vi.fn().mockResolvedValue(undefined),
    onDeleteNotification: vi.fn().mockResolvedValue(undefined),
    onClose: vi.fn(),
  };
}

function ConfigModalHarness({ handlers }: { handlers: ReturnType<typeof createSettingsHandlers> }) {
  const [form, setForm] = useState({
    repo_url: configRepo.repo_url,
    branch: configRepo.branch,
    base_path: configRepo.base_path,
    enabled: configRepo.enabled,
    write_enabled: configRepo.write_enabled,
    write_branch: configRepo.write_branch,
  });
  const [notificationForm, setNotificationForm] = useState(createEmptyNotificationRouteForm());

  return (
    <TeamConfigRepositoryModal
      teamLabel="platform"
      repo={configRepo}
      form={form}
      loading={false}
      saving={false}
      syncing={false}
      error={null}
      driftLoading={false}
      notificationRoute={{
        id: 22,
        team_id: 1,
        team_path: 'platform',
        definition: defaultNotificationRouteDefinition(),
        source: 'database',
        managed_by_config_repo: false,
      }}
      notificationForm={notificationForm}
      notificationLoading={false}
      notificationSaving={false}
      notificationError={null}
      canManage
      canSync
      onChange={setForm}
      onNotificationChange={setNotificationForm}
      onSave={() => handlers.onSave(form)}
      onDelete={handlers.onDelete}
      onSync={handlers.onSync}
      onCheckDrift={handlers.onCheckDrift}
      onSaveNotification={() => handlers.onSaveNotification(notificationForm)}
      onDeleteNotification={handlers.onDeleteNotification}
      onClose={handlers.onClose}
    />
  );
}

describe('TeamSettingsModals', () => {
  it('submits new team payloads from the creation modal', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn().mockResolvedValue(undefined);

    render(
      <NewTeamItemModal
        open
        parentLabel="platform"
        parentOptions={[
          { id: null, label: 'Global' },
          { id: 1, label: '/platform' },
          { id: 2, label: '/security' },
        ]}
        defaultParentID={1}
        error={null}
        pending={false}
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />
    );

    await user.type(screen.getByLabelText('Team Name'), 'security');
    await user.selectOptions(screen.getByLabelText('Parent team'), '2');
    await user.type(screen.getByLabelText(/Description/), 'Security engineering');
    await user.click(screen.getByRole('button', { name: 'Create' }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith({
      kind: 'team',
      name: 'security',
      description: 'Security engineering',
      repoURL: '',
      parentID: 2,
    }));
  });

  it('submits edited application details and placement', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn().mockResolvedValue(undefined);

    render(
      <EditTeamItemModal
        open
        team={{
          id: 3,
          name: 'checkout-api',
          kind: 'app',
          parent_id: 1,
          path: 'platform/checkout-api',
          team_path: 'platform',
          repo_url: 'https://github.com/acme/checkout-api',
          repository_full_name: 'acme/checkout-api',
        }}
        parentOptions={[
          { id: null, label: 'Global' },
          { id: 1, label: '/platform' },
          { id: 2, label: '/payments' },
        ]}
        error={null}
        pending={false}
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />
    );

    await waitFor(() => expect(screen.getByLabelText('Application Name')).toHaveValue('checkout-api'));
    await user.clear(screen.getByLabelText('Application Name'));
    await user.type(screen.getByLabelText('Application Name'), 'checkout-worker');
    await user.selectOptions(screen.getByLabelText('Parent team'), '2');
    await user.clear(screen.getByLabelText('Repository URL'));
    await user.type(screen.getByLabelText('Repository URL'), 'https://github.com/acme/checkout-worker');
    await user.click(screen.getByRole('button', { name: 'Save Changes' }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith({
      name: 'checkout-worker',
      description: '',
      repoURL: 'https://github.com/acme/checkout-worker',
      parentID: 2,
    }));
  });

  it('keeps GitOps and notification settings tabs interactive', async () => {
    const user = userEvent.setup();
    const handlers = createSettingsHandlers();
    render(<ConfigModalHarness handlers={handlers} />);

    await user.clear(screen.getByLabelText('Repository URL'));
    await user.type(screen.getByLabelText('Repository URL'), 'https://github.com/acme/new-config');
    await user.click(screen.getByRole('button', { name: 'Save Repository' }));
    expect(handlers.onSave).toHaveBeenCalledWith(expect.objectContaining({
      repo_url: 'https://github.com/acme/new-config',
    }));

    await user.click(screen.getByRole('tab', { name: 'Notifications' }));
    await user.click(screen.getByRole('button', { name: 'Add route' }));
    await user.clear(screen.getByLabelText('Route name'));
    await user.type(screen.getByLabelText('Route name'), 'release failures');
    await user.click(screen.getByRole('button', { name: 'Save Notifications' }));
    expect(handlers.onSaveNotification).toHaveBeenCalledWith(expect.objectContaining({
      routeName: 'release failures',
    }));

    expect(screen.queryByRole('tab', { name: 'AI profiles' })).not.toBeInTheDocument();
  });
});
