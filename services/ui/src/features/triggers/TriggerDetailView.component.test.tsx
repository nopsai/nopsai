import { createRef } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { TriggerDetailView } from './TriggerDetailView';

const triggerDetail = {
  slug: 'platform/checkout',
  source: 'gitops',
  provider: 'gitlab',
  teamPath: 'platform',
  management: 'nopsai',
  webhookSourceID: 'corporate-gitlab',
  webhookSourceName: 'Corporate GitLab',
  ingress: 'Corporate GitLab',
  allowlistStatus: 'allowed',
  repositoryForWebhook: 'acme/checkout',
  rawYaml: 'triggers:\n  - on: push\n    branches:\n      - main\n    pipelines:\n      - pipelines/platform/deploy.yaml\n    scope: production\n',
  summary: {
    triggerCount: 1,
    pipelines: [{ identifier: 'platform/deploy', display: 'deploy', pathLabel: 'platform' }],
    events: ['push'],
    branches: ['main'],
    skipBranches: [],
    tags: [],
    scopes: ['production'],
  },
};

test('renders trigger detail sections on one page and delegates actions', async () => {
  const user = userEvent.setup();
  const callbacks = {
    onBack: vi.fn(),
    onOpenScope: vi.fn(),
    onOpenPipeline: vi.fn(),
    onOpenRun: vi.fn(),
    onRecentRunsScroll: vi.fn(),
    onCopy: vi.fn(),
    onDownload: vi.fn(),
    onEdit: vi.fn(),
    onClone: vi.fn(),
    onDelete: vi.fn(),
    onDiscard: vi.fn(),
    onSave: vi.fn(),
    onTriggerDetailsChange: vi.fn(),
    onEditorTextChange: vi.fn(),
    onOpenSuggestion: vi.fn(),
    onMoveSuggestion: vi.fn(),
    onDismissSuggestion: vi.fn(),
    onSelectSuggestion: vi.fn(),
    onEditorScroll: vi.fn(),
    onIndentTab: vi.fn(),
    onAutoIndentEnter: vi.fn(),
  };

  render(
    <TriggerDetailView
      detail={triggerDetail}
      isEditing={false}
      editorValue={triggerDetail.rawYaml}
      validationErrors={[]}
      validationErrorLines={new Set()}
      editorSuggestion={null}
      autocompleteLoading={false}
      editorRef={createRef<HTMLTextAreaElement>()}
      highlightContentRef={createRef<HTMLPreElement>()}
      lineNumbersRef={createRef<HTMLDivElement>()}
      canUpdateSelectedTrigger
      canCreateTriggerHere
      canDeleteSelectedTrigger
      saving={false}
      triggerDetails={{
        provider: 'gitlab',
        teamPath: 'platform',
        management: 'nopsai',
        webhookSourceID: 'corporate-gitlab',
      }}
      teamPaths={['root', 'platform']}
      webhookSources={[
        { id: 'corporate-gitlab', name: 'Corporate GitLab', provider: 'gitlab', teamPath: 'platform', visibility: 'workspace' },
      ]}
      linkedPipelines={triggerDetail.summary.pipelines}
      pipelineMetadata={new Map([['platform/deploy', { version: 'v1', sourceKey: 'git', sourceLabel: 'Git' }]])}
      pipelineSourceIndex={new Map([['platform/deploy', 'git']])}
      recentRuns={[
        {
          run_id: 'run-123456',
          pipeline_name: 'deploy',
          status: 'success',
          git_ref: 'refs/heads/main',
          started_at: new Date().toISOString(),
          trigger_event_id: 'event-123456',
        },
      ]}
      runsLoading={false}
      runsError={null}
      runsScrollable={false}
      recentRunsListRef={createRef<HTMLUListElement>()}
      {...callbacks}
    />
  );

  expect(screen.getByRole('heading', { name: 'checkout' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Overview' })).toBeVisible();
  expect(screen.getByText('Owner')).toBeVisible();
  expect(screen.getByText('Corporate GitLab')).toBeVisible();
  expect(screen.getByText('Same-team resources')).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Trigger definition' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Recent runs' })).toBeVisible();
  expect(screen.queryByRole('tablist')).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: '/production' }));
  expect(callbacks.onOpenScope).toHaveBeenCalledWith('production');

  expect(screen.getByText(/pipelines\/platform\/deploy.yaml/)).toBeVisible();

  await user.click(screen.getByTitle('Open run run-123456'));
  expect(callbacks.onOpenRun).toHaveBeenCalledWith('run-123456');

  await user.click(screen.getByRole('button', { name: 'Copy YAML' }));
  expect(callbacks.onCopy).toHaveBeenCalledOnce();

  await user.click(screen.getByRole('button', { name: 'Delete' }));
  expect(callbacks.onDelete).toHaveBeenCalledOnce();
});

test('keeps edit and clone reachable before action-time authorization', async () => {
  const user = userEvent.setup();
  const callbacks = {
    onBack: vi.fn(),
    onOpenScope: vi.fn(),
    onOpenPipeline: vi.fn(),
    onOpenRun: vi.fn(),
    onRecentRunsScroll: vi.fn(),
    onCopy: vi.fn(),
    onDownload: vi.fn(),
    onEdit: vi.fn(),
    onClone: vi.fn(),
    onDelete: vi.fn(),
    onDiscard: vi.fn(),
    onSave: vi.fn(),
    onTriggerDetailsChange: vi.fn(),
    onEditorTextChange: vi.fn(),
    onOpenSuggestion: vi.fn(),
    onMoveSuggestion: vi.fn(),
    onDismissSuggestion: vi.fn(),
    onSelectSuggestion: vi.fn(),
    onEditorScroll: vi.fn(),
    onIndentTab: vi.fn(),
    onAutoIndentEnter: vi.fn(),
  };

  render(
    <TriggerDetailView
      detail={triggerDetail}
      isEditing={false}
      editorValue={triggerDetail.rawYaml}
      validationErrors={[]}
      validationErrorLines={new Set()}
      editorSuggestion={null}
      autocompleteLoading={false}
      editorRef={createRef<HTMLTextAreaElement>()}
      highlightContentRef={createRef<HTMLPreElement>()}
      lineNumbersRef={createRef<HTMLDivElement>()}
      canUpdateSelectedTrigger={false}
      canCreateTriggerHere={false}
      canDeleteSelectedTrigger={false}
      saving={false}
      triggerDetails={{
        provider: 'gitlab',
        teamPath: 'platform',
        management: 'nopsai',
        webhookSourceID: 'corporate-gitlab',
      }}
      teamPaths={['root', 'platform']}
      webhookSources={[
        { id: 'corporate-gitlab', name: 'Corporate GitLab', provider: 'gitlab', teamPath: 'platform', visibility: 'workspace' },
      ]}
      linkedPipelines={triggerDetail.summary.pipelines}
      pipelineMetadata={new Map()}
      pipelineSourceIndex={new Map()}
      recentRuns={[]}
      runsLoading={false}
      runsError={null}
      runsScrollable={false}
      recentRunsListRef={createRef<HTMLUListElement>()}
      {...callbacks}
    />
  );

  await user.click(screen.getByRole('button', { name: 'Edit' }));
  expect(callbacks.onEdit).toHaveBeenCalledOnce();

  await user.click(screen.getByRole('button', { name: 'Clone' }));
  expect(callbacks.onClone).toHaveBeenCalledOnce();
});

test('renders editable trigger detail fields while editing', async () => {
  const callbacks = {
    onBack: vi.fn(),
    onOpenScope: vi.fn(),
    onOpenPipeline: vi.fn(),
    onOpenRun: vi.fn(),
    onRecentRunsScroll: vi.fn(),
    onCopy: vi.fn(),
    onDownload: vi.fn(),
    onEdit: vi.fn(),
    onClone: vi.fn(),
    onDelete: vi.fn(),
    onDiscard: vi.fn(),
    onSave: vi.fn(),
    onTriggerDetailsChange: vi.fn(),
    onEditorTextChange: vi.fn(),
    onOpenSuggestion: vi.fn(),
    onMoveSuggestion: vi.fn(),
    onDismissSuggestion: vi.fn(),
    onSelectSuggestion: vi.fn(),
    onEditorScroll: vi.fn(),
    onIndentTab: vi.fn(),
    onAutoIndentEnter: vi.fn(),
  };

  render(
    <TriggerDetailView
      detail={triggerDetail}
      isEditing
      editorValue={triggerDetail.rawYaml}
      validationErrors={[]}
      validationErrorLines={new Set()}
      editorSuggestion={null}
      autocompleteLoading={false}
      editorRef={createRef<HTMLTextAreaElement>()}
      highlightContentRef={createRef<HTMLPreElement>()}
      lineNumbersRef={createRef<HTMLDivElement>()}
      canUpdateSelectedTrigger
      canCreateTriggerHere
      canDeleteSelectedTrigger
      saving={false}
      triggerDetails={{
        provider: 'gitlab',
        teamPath: 'platform',
        management: 'nopsai',
        webhookSourceID: 'corporate-gitlab',
      }}
      teamPaths={['root', 'platform']}
      webhookSources={[
        { id: 'corporate-gitlab', name: 'Corporate GitLab', provider: 'gitlab', teamPath: 'platform', visibility: 'workspace' },
      ]}
      linkedPipelines={triggerDetail.summary.pipelines}
      pipelineMetadata={new Map()}
      pipelineSourceIndex={new Map()}
      recentRuns={[]}
      runsLoading={false}
      runsError={null}
      runsScrollable={false}
      recentRunsListRef={createRef<HTMLUListElement>()}
      {...callbacks}
    />
  );

  expect(screen.getByLabelText('Provider')).toHaveValue('gitlab');
  expect(screen.getByLabelText('Team')).toHaveValue('platform');
  expect(screen.getByLabelText('Webhook source')).toHaveValue('corporate-gitlab');
});
