import { useMemo, useState } from 'react';
import { EventAutomationToolbar } from '../event-automation/EventAutomationToolbar';
import { GitWebhookSourceForm } from './GitWebhookSourceForm';
import { GitWebhookSourceMetricGrid, GitWebhookSourcesWorkspace } from './GitWebhookSourcesWorkspace';
import {
  buildGitWebhookSourceMetrics,
  buildGitWebhookSourceTreeItems,
  filterGitWebhookSources,
  gitWebhookSourceBelongsToTeam,
  gitWebhookSourceTeamPath,
} from './model';
import type { GitWebhookSourcesController } from './useGitWebhookSources';

export function GitWebhookSourcesView({
  controller,
  canWrite,
  canDelete,
}: {
  controller: GitWebhookSourcesController;
  canWrite: boolean;
  canDelete: boolean;
}) {
  const { selected } = controller;
  const [searchTerm, setSearchTerm] = useState('');
  const [activeTeamPath, setActiveTeamPath] = useState('');
  const filteredSources = useMemo(() => {
    return filterGitWebhookSources(controller.sources, searchTerm);
  }, [controller.sources, searchTerm]);
  const workspaceTeamPath = selected ? gitWebhookSourceTeamPath(selected) : activeTeamPath;
  const visibleSources = useMemo(() => {
    if (searchTerm.trim()) return filteredSources;
    return filteredSources.filter(source => gitWebhookSourceBelongsToTeam(source, workspaceTeamPath));
  }, [filteredSources, searchTerm, workspaceTeamPath]);
  const treeItems = useMemo(() => buildGitWebhookSourceTreeItems(controller.sources), [controller.sources]);
  const metrics = useMemo(() => buildGitWebhookSourceMetrics(controller.sources), [controller.sources]);
  const openTeam = (path: string) => {
    setActiveTeamPath(path);
    controller.onSelect('');
  };
  const selectSource = (sourceID: string) => {
    const source = controller.sources.find(item => item.id === sourceID);
    if (source) setActiveTeamPath(gitWebhookSourceTeamPath(source));
    controller.onSelect(sourceID);
  };

  return (
    <div data-page="git-webhook-sources" className="active h-full flex flex-col">
      <EventAutomationToolbar
        active="git-webhook-sources"
        searchLabel="Search webhook sources"
        searchTerm={searchTerm}
        canCreate={canWrite}
        createLabel="New source"
        createDisabledReason="You have read-only access to Git webhook sources"
        showCreateWhenDisabled
        onSearchTermChange={setSearchTerm}
        onCreate={controller.startCreate}
        filters={!canWrite ? <span className="runner-pill runner-pill--muted">Read-only</span> : null}
        summary={<GitWebhookSourceMetricGrid metrics={metrics} />}
      />
      <div className="flex-1 overflow-auto px-4 pb-6 triggers-content">
        {controller.error && !controller.editorOpen ? (
          <div className="mb-4 rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500">
            {controller.error}
          </div>
        ) : null}
        <GitWebhookSourcesWorkspace
          visibleSources={visibleSources}
          treeItems={treeItems}
          activeTeamPath={workspaceTeamPath}
          selected={selected}
          deliveries={controller.deliveries}
          loading={controller.loading}
          detailLoading={controller.detailLoading}
          saving={controller.saving}
          searchTerm={searchTerm}
          canWrite={canWrite}
          canDelete={canDelete}
          onOpenTeam={openTeam}
          onSelect={selectSource}
          onEdit={controller.startEdit}
          onToggle={source => void controller.setEnabled(source, !source.enabled)}
          onDelete={source => void controller.remove(source)}
        />
      </div>

      {controller.editorOpen ? (
        <GitWebhookSourceForm
          source={controller.editing || null}
          form={controller.form}
          saving={controller.saving}
          error={controller.error}
          teamPaths={controller.teamPaths}
          teamPathsLoading={controller.teamPathsLoading}
          onChange={controller.setForm}
          onClose={controller.closeEditor}
          onSubmit={controller.submit}
        />
      ) : null}
    </div>
  );
}
