import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ExternalTriggersPage from '../../pages/ExternalTriggers';
import { ExternalTriggerFormModal } from './ExternalTriggerFormModal';
import type { ExternalTriggerForm } from './model';

const mocks = vi.hoisted(() => ({
  fetch: vi.fn(),
  fetchPipelineRunGroupPaths: vi.fn(),
}));

vi.mock('../../lib/api', () => ({
  apiClient: { fetch: mocks.fetch },
  buildApiUrl: (path: string) => `http://localhost${path}`,
}));

vi.mock('../../lib/resourceGroups', () => ({
  fetchPipelineRunGroupPaths: mocks.fetchPipelineRunGroupPaths,
}));

describe('ExternalTriggersPage create action', () => {
  beforeEach(() => {
    mocks.fetch.mockResolvedValue(Response.json([]));
    mocks.fetchPipelineRunGroupPaths.mockResolvedValue([]);
  });

  it('opens an accessible feature-owned form for writable users', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <ExternalTriggersPage canWriteExternalTriggers canDeleteExternalTriggers />
      </MemoryRouter>
    );

    const opener = screen.getByRole('button', { name: 'New trigger' });
    await user.click(opener);

    expect(screen.getByRole('dialog', { name: 'New authenticated endpoint' })).toBeVisible();
    expect(screen.getByLabelText('Name')).toHaveFocus();
    expect(screen.getByRole('button', { name: 'Create trigger' })).toBeVisible();
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

describe('ExternalTriggerFormModal', () => {
  const form: ExternalTriggerForm = {
    id: 'deploy-prod',
    name: 'Deploy production',
    description: 'Authenticated deployment endpoint',
    pipeline: 'platform/deploy',
    scope: '',
    runGroupPath: 'root',
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
        runGroupOptions={['root', 'platform']}
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
    fireEvent.change(screen.getByLabelText('Run group'), { target: { value: 'platform' } });
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
        modal={{ mode: 'edit', trigger: { ...form, run_group_path: 'root' } }}
        form={form}
        formError="Variable mapping must be valid JSON."
        saving={false}
        pipelineOptions={[form.pipeline]}
        scopeOptions={['']}
        runGroupOptions={['root']}
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
        runGroupOptions={['root']}
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
