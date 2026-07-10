import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
  ArrowUpRight,
  Boxes,
  ChevronRight,
  Folder,
  GitBranch,
  Plus,
  RefreshCw,
  Search,
  Settings,
  Trash2,
  X,
} from 'lucide-react';
import { useSearchParams } from 'react-router-dom';
import { ConfigRepositoryDriftModal } from '../components/ConfigRepositoryDriftModal';
import { fetchTeamGroups, requestTeamsJson } from '../features/teams/api';
import { NewTeamItemModal, TeamConfigRepositoryModal } from '../features/teams/TeamSettingsModals';
import { useTeamConfigRepositoryController } from '../features/teams/hooks/useTeamConfigRepositoryController';
import {
  buildGroupPath,
  groupDisplayName,
  groupRepositoryLabel,
  groupRepositoryURL,
  isAppGroup,
  type Group,
} from '../lib/teamGroups';

type TeamItemPayload = {
  kind: 'group' | 'app';
  name: string;
  description: string;
  repoURL: string;
};

function isReservedRootGroupName(name: string) {
  const normalized = name.trim().replace(/^\/+|\/+$/g, '').toLowerCase();
  return normalized === 'root' || normalized === '__general__';
}

export default function TeamsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [groups, setGroups] = useState<Group[]>([]);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupsError, setGroupsError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [createPending, setCreatePending] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  const activeTeamId = useMemo(() => {
    const raw = searchParams.get('team');
    if (!raw) return null;
    const parsed = Number(raw);
    return Number.isFinite(parsed) ? parsed : null;
  }, [searchParams]);

  const activeTeamPath = useMemo(() => buildGroupPath(activeTeamId, groups), [activeTeamId, groups]);
  const activeTeam = activeTeamPath.length ? activeTeamPath[activeTeamPath.length - 1] : null;
  const selectedTeamId = activeTeam?.id ?? null;
  const activeTeamLabel = activeTeam ? groupDisplayName(activeTeam) : 'Root';

  const fetchJson = useCallback(
    async <T,>(path: string, options?: RequestInit): Promise<T> => requestTeamsJson<T>(path, options),
    []
  );

  const checkAccessPermission = useCallback(async (action: string, resourceType: string, resourceID: string) => {
    const params = new URLSearchParams({
      action,
      resource_type: resourceType,
      resource_id: resourceID,
    });
    try {
      const payload = await fetchJson<{ allowed?: boolean }>(`/v1/access/effective-permissions?${params.toString()}`);
      return Boolean(payload?.allowed);
    } catch {
      return false;
    }
  }, [fetchJson]);

  const config = useTeamConfigRepositoryController({ groups, fetchJson, checkAccessPermission });

  const loadGroups = useCallback(async () => {
    setGroupsLoading(true);
    setGroupsError(null);
    try {
      const payload = await fetchTeamGroups();
      setGroups(payload);
    } catch (error) {
      setGroupsError(error instanceof Error ? error.message : 'Unable to load teams');
    } finally {
      setGroupsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadGroups();
  }, [loadGroups]);

  const selectTeam = useCallback(
    (id: number | null) => {
      const params = new URLSearchParams(searchParams);
      if (id == null) params.delete('team');
      else params.set('team', String(id));
      setSearchParams(params, { replace: true });
    },
    [searchParams, setSearchParams]
  );

  const currentChildren = useMemo(
    () => groups.filter(group => (group.parent_id ?? null) === selectedTeamId),
    [groups, selectedTeamId]
  );

  const visibleGroups = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    const base = term ? groups : currentChildren;
    if (!term) return base;
    return base.filter(group =>
      [group.name, group.description, group.repository_full_name, group.repo_url]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
        .includes(term)
    );
  }, [currentChildren, groups, searchTerm]);

  const teams = useMemo(() => visibleGroups.filter(group => !isAppGroup(group)), [visibleGroups]);
  const applications = useMemo(() => visibleGroups.filter(group => isAppGroup(group)), [visibleGroups]);
  const stats = useMemo(() => buildTeamStats(groups, selectedTeamId), [groups, selectedTeamId]);
  const showTeamWorkspace = groups.length > 0 || searchTerm.trim().length > 0;
  const showEmptySelectionState = showTeamWorkspace && !groupsLoading && !groupsError && !searchTerm.trim() && currentChildren.length === 0;

  const submitNewTeamItem = useCallback(
    async ({ kind, name, description, repoURL }: TeamItemPayload) => {
      const trimmedName = name.trim();
      const trimmedDescription = description.trim();
      const trimmedRepoURL = repoURL.trim();
      if (!trimmedName) {
        setCreateError(kind === 'app' ? 'Application name is required.' : 'Team name is required.');
        return;
      }
      if (kind === 'group' && isReservedRootGroupName(trimmedName)) {
        setCreateError('Root is reserved and cannot be used as a team name.');
        return;
      }
      if (kind === 'app' && !trimmedRepoURL) {
        setCreateError('Repository URL is required for applications.');
        return;
      }
      setCreatePending(true);
      setCreateError(null);
      try {
        const endpoint = kind === 'group'
          ? '/v1/teams'
          : `/v1/teams/${encodeURIComponent(selectedTeamId == null ? 'root' : String(selectedTeamId))}/applications`;
        await fetchJson(endpoint, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name: trimmedName,
            description: kind === 'group' ? trimmedDescription || undefined : undefined,
            repo_url: kind === 'app' ? trimmedRepoURL : undefined,
            parent_team_id: kind === 'group' ? selectedTeamId : undefined,
          }),
        });
        setCreateOpen(false);
        await loadGroups();
        window.dispatchEvent(new Event('nopsai-resource-groups-changed'));
      } catch (error) {
        setCreateError(error instanceof Error ? error.message : 'Unable to create team item');
      } finally {
        setCreatePending(false);
      }
    },
    [fetchJson, loadGroups, selectedTeamId]
  );

  const deleteTeamItem = useCallback(
    async (group: Group) => {
      const label = groupDisplayName(group);
      const noun = isAppGroup(group) ? 'application' : 'team';
      if (!window.confirm(`Delete ${noun} "${label}"? Runs remain in history.`)) return;
      try {
        const endpoint = isAppGroup(group)
          ? `/v1/teams/${encodeURIComponent(group.parent_id == null ? 'root' : String(group.parent_id))}/applications/${encodeURIComponent(String(group.id))}`
          : `/v1/teams/${encodeURIComponent(String(group.id))}`;
        await fetchJson<void>(endpoint, { method: 'DELETE' });
        if (selectedTeamId === group.id) selectTeam(null);
        await loadGroups();
        window.dispatchEvent(new Event('nopsai-resource-groups-changed'));
      } catch (error) {
        alert(error instanceof Error ? error.message : `Unable to delete ${noun}`);
      }
    },
    [fetchJson, loadGroups, selectTeam, selectedTeamId]
  );

  return (
    <div data-page="teams" className="active min-h-full p-6 space-y-5">
      <header className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-[var(--text-primary)]">Teams</h1>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-[var(--text-secondary)]">
            <button type="button" className="runner-pill runner-pill--muted" onClick={() => selectTeam(null)}>
              Root
            </button>
            {activeTeamPath.map(group => (
              <div key={group.id} className="flex items-center gap-2">
                <ChevronRight className="h-3.5 w-3.5 text-[var(--border-primary)]" aria-hidden="true" />
                <button
                  type="button"
                  className={`runner-pill ${group.id === activeTeamId ? 'runner-pill--muted' : 'runner-pill--ghost'}`}
                  onClick={() => selectTeam(group.id)}
                >
                  {groupDisplayName(group)}
                </button>
              </div>
            ))}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="pipelines-search-shell open">
            <button
              type="button"
              className="pipelines-search-toggle"
              aria-label="Search teams"
              onClick={() => requestAnimationFrame(() => searchInputRef.current?.focus())}
            >
              <Search className="h-4 w-4" aria-hidden="true" />
            </button>
            <input
              ref={searchInputRef}
              type="text"
              placeholder="Search teams"
              className="pipelines-search-input"
              value={searchTerm}
              onChange={event => setSearchTerm(event.target.value)}
            />
            {searchTerm && (
              <button type="button" className="pipelines-search-clear" onClick={() => setSearchTerm('')} aria-label="Clear search">
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            )}
          </div>
          <button type="button" className="pipelines-icon-only" title="Refresh" aria-label="Refresh teams" onClick={() => void loadGroups()}>
            <RefreshCw className={`h-4 w-4 ${groupsLoading ? 'animate-spin' : ''}`} aria-hidden="true" />
          </button>
          <button
            type="button"
            className="glass-button-primary inline-flex items-center gap-2"
            onClick={() => {
              setCreateError(null);
              setCreateOpen(true);
            }}
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            New
          </button>
        </div>
      </header>

      <section className="grid gap-4 md:grid-cols-4">
        <MetricCard label="Teams" value={stats.teams} icon={<Folder className="h-4 w-4" />} />
        <MetricCard label="Applications" value={stats.applications} icon={<Boxes className="h-4 w-4" />} />
        <MetricCard label="Direct Children" value={currentChildren.length} icon={<GitBranch className="h-4 w-4" />} />
        <MetricCard label="Selected" value={activeTeam ? 1 : 0} icon={<Settings className="h-4 w-4" />} />
      </section>

      {groupsError && (
        <TeamsStatusPanel
          tone="error"
          icon={<RefreshCw className="h-5 w-5" aria-hidden="true" />}
          title="Teams could not load"
          message={groupsError}
          actionLabel="Retry"
          onAction={() => void loadGroups()}
        />
      )}

      {groupsLoading && groups.length === 0 && !groupsError && (
        <TeamsStatusPanel
          icon={<RefreshCw className="h-5 w-5 animate-spin" aria-hidden="true" />}
          title="Loading teams"
          message="Fetching the team structure and visible applications for your account."
        />
      )}

      {!groupsLoading && !groupsError && groups.length === 0 && (
        <TeamsStatusPanel
          icon={<Folder className="h-5 w-5" aria-hidden="true" />}
          title="No visible teams"
          message="Teams appear here after a team is created or when your account has folder.list access to existing teams."
          actionLabel="Create team"
          onAction={() => {
            setCreateError(null);
            setCreateOpen(true);
          }}
        />
      )}

      {showEmptySelectionState && (
        <TeamsStatusPanel
          icon={activeTeam && isAppGroup(activeTeam) ? <Boxes className="h-5 w-5" aria-hidden="true" /> : <Folder className="h-5 w-5" aria-hidden="true" />}
          title={activeTeam ? 'No child items' : 'No root items'}
          message={activeTeam ? `${activeTeamLabel} has no child teams or applications.` : 'There are no teams or applications at the root level.'}
          actionLabel={activeTeam ? 'Back to root' : 'Create team'}
          onAction={() => {
            if (activeTeam) {
              selectTeam(null);
              return;
            }
            setCreateError(null);
            setCreateOpen(true);
          }}
        />
      )}

      {showTeamWorkspace && (
        <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
          <div className="space-y-5">
            <TeamSection title={searchTerm.trim() ? 'Matching Teams' : 'Teams'} count={teams.length} emptyLabel="No teams.">
              <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
                {teams.map(group => (
                  <TeamCard
                    key={group.id}
                    group={group}
                    groups={groups}
                    active={selectedTeamId === group.id}
                    onSelect={() => selectTeam(group.id)}
                    onDelete={() => void deleteTeamItem(group)}
                    onOpenConfig={() => config.openFolderConfigRepository(group)}
                  />
                ))}
              </div>
            </TeamSection>

            <TeamSection title={searchTerm.trim() ? 'Matching Applications' : 'Applications'} count={applications.length} emptyLabel="No applications.">
              <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
                {applications.map(group => (
                  <TeamCard
                    key={group.id}
                    group={group}
                    groups={groups}
                    active={selectedTeamId === group.id}
                    onSelect={() => selectTeam(group.id)}
                    onDelete={() => void deleteTeamItem(group)}
                    onOpenConfig={() => config.openFolderConfigRepository(group)}
                  />
                ))}
              </div>
            </TeamSection>
          </div>

          <aside className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 h-fit">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
                  {activeTeam && isAppGroup(activeTeam) ? 'Application' : 'Team'}
                </p>
                <h2 className="mt-1 truncate text-lg font-semibold text-[var(--text-primary)]">{activeTeamLabel}</h2>
              </div>
              {activeTeam && !activeTeam.navigation_only && (
                <button
                  type="button"
                  className="pipelines-icon-only"
                  title="Delete"
                  aria-label={`Delete ${activeTeamLabel}`}
                  onClick={() => void deleteTeamItem(activeTeam)}
                >
                  <Trash2 className="h-4 w-4" aria-hidden="true" />
                </button>
              )}
            </div>
            {activeTeam?.description && <p className="mt-3 text-sm text-[var(--text-secondary)]">{activeTeam.description}</p>}
            {activeTeam && isAppGroup(activeTeam) && (
              <RepositoryLink group={activeTeam} className="mt-3" />
            )}
            <div className="mt-4 grid grid-cols-2 gap-3 text-sm">
              <div className="rounded-md bg-[var(--bg-primary)] p-3 border border-[var(--border-primary)]">
                <p className="text-xs text-[var(--text-secondary)]">Teams</p>
                <p className="mt-1 text-lg font-semibold text-[var(--text-primary)]">{stats.teams}</p>
              </div>
              <div className="rounded-md bg-[var(--bg-primary)] p-3 border border-[var(--border-primary)]">
                <p className="text-xs text-[var(--text-secondary)]">Applications</p>
                <p className="mt-1 text-lg font-semibold text-[var(--text-primary)]">{stats.applications}</p>
              </div>
            </div>
            {activeTeam && !isAppGroup(activeTeam) && !activeTeam.navigation_only && (
              <button
                type="button"
                className="mt-4 glass-button-primary w-full inline-flex items-center justify-center gap-2"
                onClick={() => config.openFolderConfigRepository(activeTeam)}
              >
                <Settings className="h-4 w-4" aria-hidden="true" />
                GitOps & Notifications
              </button>
            )}
          </aside>
        </div>
      )}

      {createOpen && (
        <NewTeamItemModal
          open={createOpen}
          parentLabel={activeTeamLabel}
          error={createError}
          pending={createPending}
          onClose={() => {
            setCreateOpen(false);
            setCreateError(null);
            setCreatePending(false);
          }}
          onSubmit={submitNewTeamItem}
        />
      )}

      {config.configRepoFolder && (
        <TeamConfigRepositoryModal
          folderLabel={config.configRepoFolder.folderPath}
          repo={config.configRepo}
          form={config.configRepoForm}
          loading={config.configRepoLoading}
          saving={config.configRepoSaving}
          syncing={config.configRepoSyncing}
          error={config.configRepoError}
          driftLoading={config.configRepoDriftLoading}
          notificationRoute={config.notificationRoute}
          notificationForm={config.notificationRouteForm}
          notificationLoading={config.notificationRouteLoading}
          notificationSaving={config.notificationRouteSaving}
          notificationError={config.notificationRouteError}
          llmProfiles={config.teamLLMProfiles}
          agentProfiles={config.teamAgentProfiles}
          mcpProfiles={config.teamMCPProfiles}
          aiProfilesLoading={config.teamProfilesLoading}
          aiProfilesSaving={config.teamProfilesSaving}
          aiProfilesError={config.teamProfilesError}
          canManage={config.configRepoManageAllowed}
          canSync={config.configRepoSyncAllowed}
          canManageProfiles={config.teamProfileManageAllowed}
          onChange={config.setConfigRepoForm}
          onNotificationChange={config.setNotificationRouteForm}
          onSave={config.saveFolderConfigRepository}
          onDelete={config.deleteFolderConfigRepository}
          onSync={config.syncFolderConfigRepository}
          onCheckDrift={config.checkFolderConfigRepositoryDrift}
          onSaveNotification={config.saveFolderNotificationRoute}
          onDeleteNotification={config.deleteFolderNotificationRoute}
          onSaveLLMProfile={config.saveTeamLLMProfile}
          onSetDefaultLLMProfile={config.saveTeamDefaultLLMProfile}
          onDeleteLLMProfile={config.removeTeamLLMProfile}
          onSaveAgentProfile={config.saveTeamAgentProfile}
          onSetDefaultAgentProfile={config.saveTeamDefaultAgentProfile}
          onDeleteAgentProfile={config.removeTeamAgentProfile}
          onSaveMCPProfile={config.saveTeamMCPProfile}
          onDeleteMCPProfile={config.removeTeamMCPProfile}
          onClose={config.closeFolderConfigRepository}
        />
      )}

      {config.configRepoFolder && config.configRepoDriftOpen && (
        <ConfigRepositoryDriftModal
          title={`${config.configRepoFolder.folderPath} config repository`}
          drift={config.configRepoDrift}
          loading={config.configRepoDriftLoading}
          error={config.configRepoDriftError}
          pushing={config.configRepoPushing}
          pushResult={config.configRepoPushResult}
          canPush={config.configRepoManageAllowed && Boolean(config.configRepoDrift?.can_push)}
          onClose={() => config.setConfigRepoDriftOpen(false)}
          onRefresh={config.checkFolderConfigRepositoryDrift}
          onPush={config.pushFolderConfigRepositoryDrift}
        />
      )}
    </div>
  );
}

function buildTeamStats(groups: Group[], activeTeamId: number | null) {
  const descendants = new Set<number>();
  const visit = (parentId: number | null) => {
    groups.forEach(group => {
      if ((group.parent_id ?? null) !== parentId) return;
      descendants.add(group.id);
      visit(group.id);
    });
  };
  visit(activeTeamId);
  const scoped = activeTeamId == null ? groups : groups.filter(group => descendants.has(group.id));
  return {
    teams: scoped.filter(group => !isAppGroup(group)).length,
    applications: scoped.filter(group => isAppGroup(group)).length,
  };
}

function MetricCard({ label, value, icon }: { label: string; value: number; icon: ReactNode }) {
  return (
    <div className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm text-[var(--text-secondary)]">{label}</span>
        <span className="text-[var(--text-secondary)]" aria-hidden="true">{icon}</span>
      </div>
      <p className="mt-2 text-2xl font-semibold text-[var(--text-primary)]">{value}</p>
    </div>
  );
}

function TeamsStatusPanel({
  tone = 'default',
  icon,
  title,
  message,
  actionLabel,
  onAction,
}: {
  tone?: 'default' | 'error';
  icon: ReactNode;
  title: string;
  message: string;
  actionLabel?: string;
  onAction?: () => void;
}) {
  const accentClass = tone === 'error' ? 'text-red-500' : 'text-[var(--text-secondary)]';
  return (
    <section className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <span className={`mt-0.5 inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] ${accentClass}`}>
            {icon}
          </span>
          <div className="min-w-0">
            <h2 className="text-sm font-semibold text-[var(--text-primary)]">{title}</h2>
            <p className="mt-1 text-sm text-[var(--text-secondary)]">{message}</p>
          </div>
        </div>
        {actionLabel && onAction && (
          <button type="button" className="glass-button-primary shrink-0" onClick={onAction}>
            {actionLabel}
          </button>
        )}
      </div>
    </section>
  );
}

function TeamSection({
  title,
  count,
  emptyLabel,
  children,
}: {
  title: string;
  count: number;
  emptyLabel: string;
  children: ReactNode;
}) {
  return (
    <section className="pipeline-dashboard-section">
      <header className="pipeline-dashboard-section-header">
        <div className="pipeline-dashboard-section-title">
          <h2>{title}</h2>
        </div>
        <span className="runner-pill runner-pill--muted">{count}</span>
      </header>
      {count > 0 ? children : <div className="pipeline-dashboard-empty-state">{emptyLabel}</div>}
    </section>
  );
}

function TeamCard({
  group,
  groups,
  active,
  onSelect,
  onDelete,
  onOpenConfig,
}: {
  group: Group;
  groups: Group[];
  active: boolean;
  onSelect: () => void;
  onDelete: () => void;
  onOpenConfig: () => void;
}) {
  const app = isAppGroup(group);
  const displayName = groupDisplayName(group);
  const childTeams = groups.filter(child => (child.parent_id ?? null) === group.id && !isAppGroup(child)).length;
  const childApplications = groups.filter(child => (child.parent_id ?? null) === group.id && isAppGroup(child)).length;
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={event => {
        if (event.key === 'Enter') onSelect();
      }}
      className={`pipeline-folder-card border border-[var(--border-primary)] ${active ? 'run-link-highlight' : ''}`}
    >
      <div className="pipeline-folder-card-header">
        <span className="pipeline-folder-icon">
          {app ? <Boxes className="h-6 w-6" aria-hidden="true" /> : <Folder className="h-6 w-6" aria-hidden="true" />}
        </span>
        <h3 className="pipeline-folder-title" title={displayName}>{displayName}</h3>
        <div className="pipeline-folder-actions">
          {!app && !group.navigation_only && (
            <button
              className="pipelines-delete-button pipeline-folder-delete-btn"
              type="button"
              title="GitOps and notifications"
              aria-label={`GitOps and notifications for ${displayName}`}
              onClick={event => {
                event.stopPropagation();
                onOpenConfig();
              }}
            >
              <Settings className="h-4 w-4" aria-hidden="true" />
            </button>
          )}
          {!group.navigation_only && (
            <button
              className="pipelines-delete-button pipeline-folder-delete-btn"
              type="button"
              title="Delete"
              aria-label={`Delete ${displayName}`}
              onClick={event => {
                event.stopPropagation();
                onDelete();
              }}
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </button>
          )}
        </div>
      </div>
      {group.description && <p className="pipeline-folder-description" title={group.description}>{group.description}</p>}
      {app ? (
        <RepositoryLink group={group} className="mt-3" />
      ) : (
        <div className="pipeline-folder-meta">
          <div className="pipeline-folder-meta-row">
            <span className="pipeline-folder-meta-label">Teams:</span>
            <span className="pipeline-folder-meta-value">{childTeams}</span>
          </div>
          <div className="pipeline-folder-meta-row">
            <span className="pipeline-folder-meta-label">Applications:</span>
            <span className="pipeline-folder-meta-value">{childApplications}</span>
          </div>
        </div>
      )}
      {group.last_run_at && <p className="mt-2 text-[11px] text-[var(--text-secondary)]">Last run {new Date(group.last_run_at).toLocaleString()}</p>}
    </div>
  );
}

function RepositoryLink({ group, className = '' }: { group: Group; className?: string }) {
  const repoURL = groupRepositoryURL(group);
  const repoLabel = groupRepositoryLabel(group);
  if (!repoURL) return <p className={`text-sm text-[var(--text-secondary)] ${className}`}>No repository URL.</p>;
  return (
    <a
      href={repoURL}
      target="_blank"
      rel="noreferrer"
      className={`inline-flex min-w-0 items-center gap-1.5 text-xs font-medium text-[var(--text-accent)] hover:underline ${className}`}
      title={repoURL}
      onClick={event => event.stopPropagation()}
    >
      <ArrowUpRight className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
      <span className="truncate">{repoLabel || repoURL}</span>
    </a>
  );
}
