import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ExternalTriggersPage from '../../pages/ExternalTriggers';
import { ExternalTriggerCards } from './ExternalTriggerCards';
import { ExternalTriggerFormModal } from './ExternalTriggerFormModal';
import type { ExternalTrigger, ExternalTriggerForm } from './model';

const mocks = vi.hoisted(() => ({
  fetch: vi.fn(),
  fetchPipelineRunTeamPaths: vi.fn(),
}));

vi.mock('../../lib/api', () => ({
  apiClient: { fetch: mocks.fetch },
  buildApiUrl: (path: string) => `http://localhost${path}`,
}));

vi.mock('../../lib/resourceTeams', () => ({
  fetchPipelineRunTeamPaths: mocks.fetchPipelineRunTeamPaths,
}));

function LocationProbe() {
  const location = useLocation();
  return <span data-testid="location">{location.pathname}{location.search}</span>;
}

describe('ExternalTriggersPage create action', () => {
  beforeEach(() => {
    mocks.fetch.mockResolvedValue(Response.json([]));
    mocks.fetchPipelineRunTeamPaths.mockResolvedValue([]);
  });

  it('opens an accessible feature-owned form for writable users', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <ExternalTriggersPage canWriteExternalTriggers canDeleteExternalTriggers />
      </MemoryRouter>
    );

    const opener = screen.getByRole('button', { name: 'New trigger' });
    expect(screen.getByRole('button', { name: 'Search external triggers' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Refresh External API triggers' })).toBeVisible();
    expect(screen.queryByText('Event automation')).not.toBeInTheDocument();
    expect(screen.queryByText('External API triggers')).not.toBeInTheDocument();
    expect(screen.queryByText('Authenticated invocation endpoints with caller policy, payload mapping, and audit history.')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Git webhooks' })).toHaveAttribute('href', '/git-webhook-sources');
    await user.click(opener);

    const dialog = screen.getByRole('dialog', { name: 'New authenticated endpoint' });
    expect(dialog).toBeVisible();
    expect(dialog).toHaveClass(
      'pipelines-modal-card',
      'workflow-form-dialog',
      'workflow-form-dialog--wide'
    );
    expect(dialog.querySelector('.pipelines-modal-header')).not.toBeNull();
    expect(dialog.querySelector('.pipelines-modal-body')).not.toBeNull();
    expect(dialog.querySelector('.pipelines-modal-footer')).not.toBeNull();
    expect(screen.getByLabelText('Name')).toHaveFocus();
    expect(screen.getByRole('button', { name: 'Create trigger' })).toBeVisible();
  });

  it('renders workspace metrics and opens details after selecting a row', async () => {
    const user = userEvent.setup();
    const trigger: ExternalTrigger = {
      id: 'deploy/prod',
      name: 'Deploy production',
      description: 'ServiceNow approval endpoint',
      enabled: true,
      pipeline: 'platform/deploy',
      scope: 'production',
      run_team_path: 'platform/prod',
      allowed_callers: [{ type: 'service_account', id: 'deployer' }],
      last_used_at: '2026-07-12T09:30:00Z',
      source: 'database',
    };
    mocks.fetch.mockImplementation(async path => {
      const requestPath = String(path);
      if (requestPath === '/v1/external-triggers') return Response.json([trigger]);
      if (requestPath === '/v1/external-triggers/deploy%2Fprod') return Response.json(trigger);
      if (requestPath === '/v1/external-triggers/deploy%2Fprod/invocations?limit=20') return Response.json([]);
      return Response.json([]);
    });

    render(
      <MemoryRouter initialEntries={['/external-triggers']}>
        <ExternalTriggersPage canWriteExternalTriggers canDeleteExternalTriggers />
        <LocationProbe />
      </MemoryRouter>
    );

    expect(await screen.findByText('Deploy production')).toBeVisible();
    expect(screen.getByText('API endpoints')).toBeVisible();
    expect(screen.getByText('Caller policies')).toBeVisible();
    expect(screen.getByRole('complementary', { name: 'Team tree' })).toBeVisible();
    expect(screen.getByRole('separator', { name: 'Resize team tree' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'All teams (1)' })).toBeVisible();
    expect(screen.queryByText('Select an external trigger')).not.toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Last used' })).toBeVisible();
    expect(screen.getByRole('cell', { name: 'platform/deploy' })).toBeVisible();
    expect(screen.getByRole('cell', { name: 'platform/prod' })).toBeVisible();
    expect(screen.getByRole('cell', { name: 'production' })).toBeVisible();
    expect(screen.queryByText('Allowed callers')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Deploy production' }));

    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/external-triggers/deploy/prod'));
    expect(await screen.findByText('Allowed callers')).toBeVisible();
    expect(screen.getByRole('button', { name: 'List' })).toBeVisible();
    expect(screen.getByText('service_account:deployer')).toBeVisible();
    expect(screen.getAllByText(/external-triggers\/deploy%2Fprod\/invoke/)).toHaveLength(2);
  });

  it('keeps the action visible but disabled when AAA grants read-only access', () => {
    render(
      <MemoryRouter>
        <ExternalTriggersPage canWriteExternalTriggers={false} canDeleteExternalTriggers={false} />
      </MemoryRouter>
    );

    expect(screen.getByText('Read-only')).toBeVisible();
    expect(screen.getByRole('button', { name: 'New trigger' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'New trigger' })).toHaveAttribute(
      'title',
      'You have read-only access to external triggers'
    );
  });
});

describe('ExternalTriggerCards', () => {
  it('renders compact selectable cards with GitOps and disabled states', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const triggers: ExternalTrigger[] = [
      {
        id: 'platform/deploy-prod',
        name: 'Deploy production',
        description: 'ServiceNow approval endpoint',
        enabled: false,
        pipeline: 'platform/deploy',
        scope: 'production',
        run_team_path: 'platform/prod',
        allowed_callers: [
          { type: 'service_account', id: 'deployer' },
          { type: 'auth_team', id: 'operators' },
        ],
        managed_by_config_repo: true,
      },
      {
        id: 'deploy-dev',
        name: '',
        enabled: true,
        pipeline: 'platform/deploy-dev',
      },
    ];

    render(<ExternalTriggerCards triggers={triggers} selectedID={triggers[0].id} onSelect={onSelect} />);

    const list = screen.getByTestId('external-trigger-card-list');
    expect(list).toHaveClass('compact-resource-grid');
    const cards = Array.from(list.querySelectorAll('.compact-resource-card'));
    expect(cards).toHaveLength(2);
    cards.forEach(card => {
      expect(card).toHaveClass('compact-resource-card--bordered', 'external-trigger-card');
    });
    expect(screen.getByText('Disabled')).toBeVisible();
    expect(screen.getByText('GitOps')).toBeVisible();
    expect(screen.getByText('production · service_account, auth_team')).toBeVisible();
    expect(screen.getAllByText('deploy-dev')).toHaveLength(2);
    expect(screen.getByText('default · none')).toBeVisible();
    const selector = screen.getByRole('button', { name: 'Select external trigger Deploy production' });
    expect(selector).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'Select external trigger deploy-dev' })).toHaveAttribute(
      'aria-pressed',
      'false'
    );

    await user.click(selector);
    expect(onSelect).toHaveBeenCalledWith('platform/deploy-prod');
  });
});

describe('ExternalTriggerFormModal', () => {
  const form: ExternalTriggerForm = {
    id: 'deploy-prod',
    name: 'Deploy production',
    description: 'Authenticated deployment endpoint',
    pipeline: 'platform/deploy',
    scope: '',
    runTeamPath: 'root',
    enabled: true,
    allowedCallers: [{ type: 'service_account', id: 'deployer' }],
    variableMappingText: '{"VERSION":"payload.version"}',
    payloadSchemaText: '{"type":"object"}',
    rateLimitPerMinute: '30',
  };

  it('routes create form interactions through feature callbacks', () => {
    const callbacks = {
      onClose: vi.fn(),
      onSubmit: vi.fn((event: React.FormEvent<HTMLFormElement>) => event.preventDefault()),
      onFormChange: vi.fn(),
      onPipelineChange: vi.fn(),
      onCallerTypeChange: vi.fn(),
      onCallerIDChange: vi.fn(),
      onAddCaller: vi.fn(),
      onRemoveCaller: vi.fn(),
    };

    render(
      <ExternalTriggerFormModal
        modal={{ mode: 'create' }}
        form={form}
        formError=""
        saving={false}
        pipelineOptions={['platform/deploy', 'platform/rollback']}
        scopeOptions={['', 'production']}
        runTeamOptions={['root', 'platform']}
        callerDraft={{ type: 'service_account', id: 'deployer' }}
        activeCallerOptions={[{ value: 'deployer', label: 'deployer' }]}
        {...callbacks}
      />
    );

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Deploy' } });
    fireEvent.change(screen.getByLabelText('ID'), { target: { value: 'deploy' } });
    fireEvent.change(screen.getByLabelText('Description'), { target: { value: 'Deploy endpoint' } });
    fireEvent.change(screen.getByLabelText('Pipeline'), { target: { value: 'platform/rollback' } });
    fireEvent.change(screen.getByLabelText('Scope'), { target: { value: 'production' } });
    fireEvent.change(screen.getByLabelText('Run team'), { target: { value: 'platform' } });
    fireEvent.click(screen.getByLabelText('Enabled'));
    fireEvent.change(screen.getByLabelText('Caller type'), { target: { value: 'user' } });
    fireEvent.change(screen.getByLabelText('Caller'), { target: { value: 'deployer' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    fireEvent.click(screen.getByRole('button', { name: 'service_account:deployer' }));
    fireEvent.change(screen.getByLabelText('Variable mapping'), { target: { value: '{}' } });
    fireEvent.change(screen.getByLabelText('Payload schema'), { target: { value: '{}' } });
    fireEvent.change(screen.getByLabelText('Rate limit per minute'), { target: { value: '60' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create trigger' }));
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));

    expect(callbacks.onFormChange).toHaveBeenCalledWith({ name: 'Deploy' });
    expect(callbacks.onFormChange).toHaveBeenCalledWith({ id: 'deploy' });
    expect(callbacks.onFormChange).toHaveBeenCalledWith({ enabled: false });
    expect(callbacks.onPipelineChange).toHaveBeenCalledWith('platform/rollback');
    expect(callbacks.onCallerTypeChange).toHaveBeenCalledWith('user');
    expect(callbacks.onCallerIDChange).toHaveBeenCalledWith('deployer');
    expect(callbacks.onAddCaller).toHaveBeenCalledOnce();
    expect(callbacks.onRemoveCaller).toHaveBeenCalledWith(0);
    expect(callbacks.onSubmit).toHaveBeenCalledOnce();
    expect(callbacks.onClose).toHaveBeenCalledOnce();
  });

  it('renders edit, error, and saving states', () => {
    const onClose = vi.fn();
    const { rerender } = render(
      <ExternalTriggerFormModal
        modal={{ mode: 'edit', trigger: { ...form, run_team_path: 'root' } }}
        form={form}
        formError="Variable mapping must be valid JSON."
        saving={false}
        pipelineOptions={[form.pipeline]}
        scopeOptions={['']}
        runTeamOptions={['root']}
        callerDraft={{ type: 'service_account', id: '' }}
        activeCallerOptions={[]}
        onClose={onClose}
        onSubmit={event => event.preventDefault()}
        onFormChange={() => undefined}
        onPipelineChange={() => undefined}
        onCallerTypeChange={() => undefined}
        onCallerIDChange={() => undefined}
        onAddCaller={() => undefined}
        onRemoveCaller={() => undefined}
      />
    );

    expect(screen.getByRole('dialog', { name: 'Deploy production' })).toHaveAccessibleDescription(
      'Variable mapping must be valid JSON.'
    );
    expect(screen.getByLabelText('ID')).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Save trigger' })).toBeVisible();
    expect(screen.getByText('No callers available')).toBeVisible();

    rerender(
      <ExternalTriggerFormModal
        modal={{ mode: 'edit' }}
        form={{ ...form, name: '' }}
        formError=""
        saving
        pipelineOptions={[form.pipeline]}
        scopeOptions={['']}
        runTeamOptions={['root']}
        callerDraft={{ type: 'service_account', id: '' }}
        activeCallerOptions={[]}
        onClose={onClose}
        onSubmit={event => event.preventDefault()}
        onFormChange={() => undefined}
        onPipelineChange={() => undefined}
        onCallerTypeChange={() => undefined}
        onCallerIDChange={() => undefined}
        onAddCaller={() => undefined}
        onRemoveCaller={() => undefined}
      />
    );

    expect(screen.getByRole('dialog', { name: 'deploy-prod' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled();
  });
});
