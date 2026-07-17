import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useState } from 'react';
import { expect, test, vi } from 'vitest';

import {
  DashboardModal,
  RefreshModal,
  RefreshScheduleModal,
  SectionModal,
  SourceModal,
  type SectionModalState,
  type SourceModalState,
} from './DashboardModals';
import {
  createDashboardForm,
  createRefreshForm,
  createRefreshScheduleForm,
  createSectionForm,
  createSourceForm,
  type DashboardFormState,
  type DashboardSection,
  type DashboardSectionFormState,
  type DashboardSourceFormState,
} from './model';
import type { DashboardPipelineOutputOption } from './sourceOptions';

test('new dashboard modal uses an existing-team dropdown and dashboard-output pipeline picker', () => {
  const onChange = vi.fn();
  const form = {
    ...createDashboardForm('team-1'),
    slug: 'ops-dashboard',
    title: 'Ops Dashboard',
    description: 'Operational view.',
  };

  render(
    <DashboardModal
      modal={{ mode: 'create' }}
      form={form}
      teams={['team-1', 'platform']}
      pipelineOptions={dashboardPipelineOptions()}
      scopeOptions={['', 'prod']}
      saving={false}
      error={null}
      onChange={onChange}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />
  );

  const dialog = screen.getByRole('dialog', { name: 'New dashboard' });
  expect(dialog).toHaveClass('pipelines-modal-card', 'workflow-form-dialog', 'workflow-form-dialog--xwide');
  const teamSelect = screen.getByLabelText('Team');
  expect(teamSelect.tagName).toBe('SELECT');
  expect(screen.getByRole('option', { name: 'platform' })).toBeInTheDocument();
  expect(screen.queryByLabelText('Access')).not.toBeInTheDocument();
  expect(screen.getByPlaceholderText('One sentence for the dashboard catalog')).toBeVisible();
  expect(screen.queryByLabelText('Section')).not.toBeInTheDocument();
  expect(screen.getByText('Pipeline sources')).toBeVisible();
  expect(screen.getByText('service-metrics')).toBeVisible();

  fireEvent.change(teamSelect, { target: { value: 'platform' } });
  expect(onChange).toHaveBeenCalledWith({ ...form, teamPath: 'platform' });
  fireEvent.click(screen.getByRole('checkbox', { name: /team-1\/dashboard-sample/ }));
  expect(onChange).toHaveBeenCalledWith({
    ...form,
    pipelineIDs: ['team-1/dashboard-sample'],
    pipelineScopes: { 'team-1/dashboard-sample': '' },
  });
});

test('edit dashboard modal leaves access management to the dashboard access card', () => {
  const onEditSection = vi.fn();
  const onDeleteSection = vi.fn();
  const form: DashboardFormState = {
    ...createDashboardForm('team-1'),
    slug: 'ops-dashboard',
    title: 'Ops Dashboard',
    visibility: 'restricted',
    pipelineIDs: ['team-1/dashboard-sample'],
    pipelineScopes: { 'team-1/dashboard-sample': 'prod' },
  };
  const onChange = vi.fn();

  render(
    <DashboardModal
      modal={{
        mode: 'edit',
        dashboard: {
          id: 'dashboard-1',
          team_path: 'team-1',
          ref: 'team-1/ops-dashboard',
          slug: 'ops-dashboard',
          title: 'Ops Dashboard',
          visibility: 'restricted',
        },
      }}
      form={form}
      teams={['team-1']}
      pipelineOptions={dashboardPipelineOptions()}
      scopeOptions={['', 'prod', 'staging']}
      sections={[
        {
          id: 'section-1',
          section_key: 'overview',
          title: 'Overview',
          description: 'Primary operational signals.',
          display_order: 0,
        },
      ]}
      saving={false}
      error={null}
      onChange={onChange}
      onEditSection={onEditSection}
      onDeleteSection={onDeleteSection}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />
  );

  expect(screen.getByRole('dialog', { name: 'Edit dashboard' })).toBeVisible();
  expect(screen.queryByLabelText('Access')).not.toBeInTheDocument();
  expect(screen.getByText('Sections')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'New section' })).not.toBeInTheDocument();
  expect(screen.getByRole('checkbox', { name: /team-1\/dashboard-sample/ })).toBeChecked();
  expect(screen.getByLabelText('Run scope for team-1/dashboard-sample')).toHaveValue('prod');
  fireEvent.change(screen.getByLabelText('Run scope for team-1/dashboard-sample'), { target: { value: 'staging' } });
  expect(onChange).toHaveBeenCalledWith({
    ...form,
    pipelineScopes: { 'team-1/dashboard-sample': 'staging' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Edit section Overview' }));
  expect(onEditSection).toHaveBeenCalledWith(expect.objectContaining({ section_key: 'overview' }));
  fireEvent.click(screen.getByRole('button', { name: 'Delete section Overview' }));
  expect(onDeleteSection).toHaveBeenCalledWith(expect.objectContaining({ section_key: 'overview' }));
});

test('section modal edits section title, description, and order without changing existing keys', () => {
  render(<SectionModalHarness />);

  expect(screen.getByRole('dialog', { name: 'Edit section' })).toHaveClass('workflow-form-dialog--xwide');
  expect(screen.getByLabelText('Section')).toBeDisabled();
  expect(screen.getByLabelText('Section')).toHaveValue('overview');
  fireEvent.change(screen.getByLabelText('Title'), { target: { value: 'Live overview' } });
  fireEvent.change(screen.getByLabelText('Description'), { target: { value: 'Current service status.' } });
  fireEvent.change(screen.getByLabelText('Order'), { target: { value: '20' } });
  expect(screen.getByLabelText('Title')).toHaveValue('Live overview');
  expect(screen.getByLabelText('Description')).toHaveValue('Current service status.');
  expect(screen.getByLabelText('Order')).toHaveValue(20);
});

test('source modal loads pipeline outputs and maps section, output, and entry with dropdowns', async () => {
  const loadPipelineOutputs = vi.fn(async (_pipelineID: string): Promise<DashboardPipelineOutputOption[]> => (
    [
      {
        name: 'Service metrics',
        type: 'dashboard',
        when: 'success',
        dashboardRef: 'team-1/ops-dashboard',
        sectionKey: 'service-metrics',
        entryKey: 'dashboard-sample',
        mode: 'replace',
        preset: 'metrics',
        ttl: '24h',
      },
      {
        name: 'Other dashboard',
        type: 'dashboard',
        when: 'success',
        dashboardRef: 'platform/ops',
        sectionKey: 'overview',
        entryKey: 'other',
        mode: 'replace',
        preset: 'status',
        ttl: '',
      },
    ]
  ));
  const sections: DashboardSection[] = [
    {
      id: 'section-1',
      section_key: 'overview',
      title: 'Overview',
      display_order: 0,
    },
    {
      id: 'section-2',
      section_key: 'service-metrics',
      title: 'Service Metrics',
      display_order: 10,
    },
  ];

  render(
    <SourceModalHarness
      sections={sections}
      loadPipelineOutputs={loadPipelineOutputs}
    />
  );

  expect(screen.getByRole('dialog', { name: 'New source' })).toHaveClass('workflow-form-dialog--xwide');
  expect(screen.getByText('Mapping review')).toBeVisible();
  fireEvent.change(screen.getByLabelText('Pipeline'), { target: { value: 'team-1/dashboard-sample' } });
  await waitFor(() => expect(loadPipelineOutputs).toHaveBeenCalledWith('team-1/dashboard-sample'));
  expect(await screen.findByRole('option', { name: 'Service metrics' })).toBeInTheDocument();
  expect(screen.queryByRole('option', { name: 'Other dashboard' })).not.toBeInTheDocument();

  fireEvent.change(screen.getByLabelText('Output'), { target: { value: 'Service metrics' } });
  fireEvent.change(screen.getByLabelText('Run scope'), { target: { value: 'prod' } });

  await waitFor(() => {
    expect(screen.getByLabelText('Section')).toHaveValue('service-metrics');
    expect(screen.getByLabelText('Entry')).toHaveValue('dashboard-sample');
  });
  expect(screen.getByText('Pipeline output')).toBeVisible();
  expect(screen.getAllByText('Run scope').length).toBeGreaterThan(0);
  expect(screen.getAllByText('prod').length).toBeGreaterThan(0);
  expect(screen.getByText('24h')).toBeVisible();
});

test('refresh modal explains scope and execution guardrails in a wider dialog', () => {
  render(
    <RefreshModal
      title="Refresh overview"
      form={createRefreshForm('overview')}
      sections={[
        {
          id: 'section-1',
          section_key: 'overview',
          title: 'Overview',
          display_order: 0,
        },
      ]}
      sources={[]}
      saving={false}
      error={null}
      onChange={vi.fn()}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />
  );

  expect(screen.getByRole('dialog', { name: 'Refresh overview' })).toHaveClass('workflow-form-dialog--xwide');
  expect(screen.getByText('Refresh target')).toBeVisible();
  expect(screen.getByText('Execution guardrails')).toBeVisible();
  expect(screen.getByText(/Strict fails when required sources cannot complete/)).toBeVisible();
});

test('refresh schedule modal captures cadence, target, and guardrails', () => {
  const onChange = vi.fn();
  const form = {
    ...createRefreshScheduleForm(),
    name: 'hourly-health',
    description: 'Refresh health before review.',
  };

  render(
    <RefreshScheduleModal
      modal={{ mode: 'create' }}
      form={form}
      sections={[
        {
          id: 'section-1',
          section_key: 'overview',
          title: 'Overview',
          display_order: 0,
        },
      ]}
      sources={[
        {
          id: 'source-1',
          section_key: 'overview',
          pipeline_id: 'team-1/dashboard-sample',
          output_name: 'Service metrics',
          enabled: true,
          required_for_refresh: true,
          refresh_order: 0,
        },
      ]}
      saving={false}
      error={null}
      onChange={onChange}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />
  );

  expect(screen.getByRole('dialog', { name: 'Schedule refresh' })).toHaveClass('workflow-form-dialog--xwide');
  expect(screen.getByText('Cadence')).toBeVisible();
  expect(screen.getByText('Refresh target')).toBeVisible();
  expect(screen.getByLabelText('Frequency')).toHaveValue('daily');
  expect(screen.getByLabelText('Cron preview')).toHaveValue('0 2 * * *');
  fireEvent.change(screen.getByLabelText('Frequency'), { target: { value: 'minutes' } });
  expect(onChange).toHaveBeenCalledWith({ ...form, cronMode: 'minutes', cron_expression: '*/15 * * * *' });
  fireEvent.change(screen.getByLabelText('Scope'), { target: { value: 'source' } });
  expect(onChange).toHaveBeenCalledWith({ ...form, scopeType: 'source' });
});

function SourceModalHarness({
  sections,
  loadPipelineOutputs,
}: {
  sections: DashboardSection[];
  loadPipelineOutputs: (pipelineID: string) => Promise<DashboardPipelineOutputOption[]>;
}) {
  const [form, setForm] = useState<DashboardSourceFormState>(() => createSourceForm('overview'));
  const modal: SourceModalState = { mode: 'create', sectionKey: 'overview' };

  return (
    <SourceModal
      modal={modal}
      form={form}
      dashboardRef="team-1/ops-dashboard"
      sections={sections}
      sources={[]}
      publications={[]}
      pipelines={['team-1/dashboard-sample']}
      scopeOptions={['', 'prod']}
      saving={false}
      error={null}
      loadPipelineOutputs={loadPipelineOutputs}
      onChange={setForm}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />
  );
}

function dashboardPipelineOptions() {
  return [
    {
      id: 'team-1/dashboard-sample',
      outputs: [
        {
          name: 'Service metrics',
          type: 'dashboard',
          when: 'success',
          dashboardRef: 'team-1/ops-dashboard',
          sectionKey: 'service-metrics',
          entryKey: 'dashboard-sample',
          mode: 'replace',
          preset: 'metrics',
          ttl: '24h',
        },
      ],
    },
  ];
}

function SectionModalHarness() {
  const [form, setForm] = useState<DashboardSectionFormState>(() => ({
    ...createSectionForm(0),
    sectionKey: 'overview',
    title: 'Overview',
  }));
  const modal: SectionModalState = {
    mode: 'edit',
    section: {
      id: 'section-1',
      section_key: 'overview',
      title: 'Overview',
      display_order: 0,
    },
  };

  return (
    <SectionModal
      modal={modal}
      form={form}
      saving={false}
      error={null}
      onChange={setForm}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />
  );
}
