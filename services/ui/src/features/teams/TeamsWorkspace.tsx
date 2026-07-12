import { useMemo, useState, type CSSProperties, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  ArrowUpRight,
  Bell,
  BookOpen,
  Bot,
  Boxes,
  ChevronRight,
  Clock,
  FolderTree,
  GitBranch,
  ListChecks,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Route,
  Search,
  ShieldCheck,
  Trash2,
  UsersRound,
  Webhook,
} from 'lucide-react';
import {
  isAppTeam,
  teamDisplayName,
  teamPathForURL,
  teamRepositoryLabel,
  teamRepositoryURL,
  type Team,
} from '../../lib/teamModels';
import { teamScopedRoute } from '../../lib/teamRoutes';
import type { CurrentUser } from '../../app/types';
import {
  buildTeamScopeStats,
  buildTeamTree,
  formatTeamTimestamp,
  getTeamDirectChildren,
  getTeamParent,
  getTeamSubtree,
  getVisibleTeamItems,
  teamKindLabel,
  type TeamTreeNode,
} from './model';
import {
  TeamActivityCard,
  TeamChildrenTable,
  TeamTabPanel,
} from './TeamsWorkspacePanels';
import {
  getTeamTableCopy,
  getTeamTableItems,
  visibleTeamDetailTabs,
  type TeamDetailTabID,
} from './workspaceModel';
import type { TeamOperationsSummaryState } from './hooks/useTeamOperationsSummary';
import {
  TEAM_LINKED_RESOURCE_LABELS,
  type TeamLinkedResource,
  type TeamLinkedResourceKind,
  type TeamResourceCatalogState,
} from './resourceCatalogModel';
import './teams.css';

export function TeamsWorkspace({
  teams,
  activeTeam,
  activeTeamPath,
  searchTerm,
  teamsLoading,
  onSearchTermChange,
  onSelectTeam,
  onRefresh,
  onCreate,
  onDeleteTeam,
  onOpenConfig,
  operationsSummary,
  resourceCatalog,
  currentUser,
}: {
  teams: Team[];
  activeTeam: Team | null;
  activeTeamPath: Team[];
  searchTerm: string;
  teamsLoading: boolean;
  onSearchTermChange: (value: string) => void;
  onSelectTeam: (id: number | null) => void;
  onRefresh: () => void;
  onCreate: () => void;
  onDeleteTeam: (team: Team) => void;
  onOpenConfig: (team: Team, tab?: 'sync' | 'notifications') => void;
  operationsSummary: TeamOperationsSummaryState;
  resourceCatalog: TeamResourceCatalogState;
  currentUser?: CurrentUser | null;
}) {
  const activeTeamID = activeTeam?.id ?? null;
  const [detailTabSelection, setDetailTabSelection] = useState<{ teamID: number | null; tab: TeamDetailTabID }>({
    teamID: activeTeamID,
    tab: 'overview',
  });
  const detailTabs = visibleTeamDetailTabs(activeTeam);
  const selectedDetailTab = detailTabSelection.teamID === activeTeamID ? detailTabSelection.tab : 'overview';
  const activeDetailTab = detailTabs.some(tab => tab.id === selectedDetailTab) ? selectedDetailTab : 'overview';
  const directChildren = getTeamDirectChildren(teams, activeTeamID);
  const visibleItems = getVisibleTeamItems(teams, activeTeamID, searchTerm);
  const scopedItems = useMemo(() => getTeamSubtree(teams, activeTeamID), [activeTeamID, teams]);
  const scopedApplications = useMemo(() => scopedItems.filter(isAppTeam), [scopedItems]);
  const stats = buildTeamScopeStats(teams, activeTeamID);
  const searching = Boolean(searchTerm.trim());
  const tableItems = getTeamTableItems({
    directChildren,
    searching,
    visibleItems,
  });
  const activeLabel = activeTeam ? teamDisplayName(activeTeam) : 'Global';
  const emptySelection = activeDetailTab === 'overview' && Boolean(activeTeam) && !searching && directChildren.length === 0;
  const tableCopy = getTeamTableCopy({ activeLabel, searching });
  const selectDetailTab = (tab: TeamDetailTabID) => setDetailTabSelection({ teamID: activeTeamID, tab });

  return (
    <div className="teams-page-shell">
      <header className="teams-page-toolbar">
        <div className="teams-title-block">
          <TeamBreadcrumb path={activeTeamPath} activeTeamID={activeTeamID} onSelectTeam={onSelectTeam} />
        </div>
        <div className="teams-toolbar-actions">
          <label className="teams-search" htmlFor="teams-global-search">
            <Search className="h-4 w-4" aria-hidden="true" />
            <input
              id="teams-global-search"
              value={searchTerm}
              onChange={event => onSearchTermChange(event.target.value)}
              placeholder="Search teams, apps, repositories"
            />
          </label>
          <button type="button" className="teams-icon-btn" title="Refresh teams" aria-label="Refresh teams" onClick={onRefresh}>
            <RefreshCw className={`h-4 w-4 ${teamsLoading ? 'animate-spin' : ''}`} aria-hidden="true" />
          </button>
          <button type="button" className="teams-primary-btn" onClick={onCreate}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            New
          </button>
        </div>
      </header>

      <div className="teams-master-detail">
        <TeamTreePanel
          teams={teams}
          activeTeamID={activeTeamID}
          searchTerm={searchTerm}
          onSearchTermChange={onSearchTermChange}
          onSelectTeam={onSelectTeam}
        />
        <section className="teams-detail-stack" aria-label={`${activeLabel} details`}>
          <TeamDetailHeader
            team={activeTeam}
            teams={teams}
            stats={stats}
            onDeleteTeam={onDeleteTeam}
            detailTabs={detailTabs}
            activeTab={activeDetailTab}
            onTabChange={selectDetailTab}
          />
          {activeDetailTab === 'overview' ? (
            <>
              <div className="teams-detail-grid" role="tabpanel" id="teams-tabpanel-overview" aria-labelledby="teams-tab-overview">
                <TeamOverviewCard team={activeTeam} teams={teams} stats={stats} />
                <TeamActivityCard team={activeTeam} stats={stats} directChildren={directChildren} />
              </div>
              <TeamResourcesPanel
                team={activeTeam}
                teams={teams}
                stats={stats}
                scopedApplications={scopedApplications}
                operationsSummary={operationsSummary}
                resourceCatalog={resourceCatalog}
                onTabChange={selectDetailTab}
              />
            </>
          ) : (
            <TeamTabPanel
              activeTab={activeDetailTab}
              team={activeTeam}
              teams={teams}
              stats={stats}
              operationsSummary={operationsSummary}
              currentUser={currentUser}
              onOpenConfig={onOpenConfig}
            />
          )}
          {activeDetailTab === 'overview' ? (
            <TeamChildrenTable
              title={tableCopy.title}
              items={tableItems}
              teams={teams}
              emptySelection={emptySelection}
              activeLabel={activeLabel}
              emptyTitle={tableCopy.emptyTitle}
              emptyMessage={tableCopy.emptyMessage}
              showBackToRoot={tableCopy.showBackToRoot}
              onBackToRoot={() => onSelectTeam(null)}
              onSelectTeam={onSelectTeam}
              onDeleteTeam={onDeleteTeam}
              onOpenConfig={onOpenConfig}
            />
          ) : null}
        </section>
      </div>
    </div>
  );
}

export function TeamsStatusPanel({
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
  return (
    <section className={`teams-status-panel teams-status-panel--${tone}`}>
      <span className="teams-status-panel__icon" aria-hidden="true">
        {icon}
      </span>
      <div className="teams-status-panel__body">
        <h2>{title}</h2>
        <p>{message}</p>
      </div>
      {actionLabel && onAction ? (
        <button type="button" className="teams-primary-btn" onClick={onAction}>
          {actionLabel}
        </button>
      ) : null}
    </section>
  );
}

function TeamBreadcrumb({
  path,
  activeTeamID,
  onSelectTeam,
}: {
  path: Team[];
  activeTeamID: number | null;
  onSelectTeam: (id: number | null) => void;
}) {
  return (
    <nav className="teams-breadcrumb" aria-label="Team path">
      <button type="button" className={`teams-breadcrumb__item ${activeTeamID == null ? 'current' : ''}`} onClick={() => onSelectTeam(null)}>
        Global
      </button>
      {path.map(team => (
        <span key={team.id} className="teams-breadcrumb__segment">
          <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
          <button
            type="button"
            className={`teams-breadcrumb__item ${team.id === activeTeamID ? 'current' : ''}`}
            onClick={() => onSelectTeam(team.id)}
          >
            {teamDisplayName(team)}
          </button>
        </span>
      ))}
    </nav>
  );
}

function TeamTreePanel({
  teams,
  activeTeamID,
  searchTerm,
  onSearchTermChange,
  onSelectTeam,
}: {
  teams: Team[];
  activeTeamID: number | null;
  searchTerm: string;
  onSearchTermChange: (value: string) => void;
  onSelectTeam: (id: number | null) => void;
}) {
  const [collapsedIDs, setCollapsedIDs] = useState<Set<number>>(() => new Set());
  const tree = buildTeamTree(teams);
  const activeLineageIDs = useMemo(() => {
    const lineage = new Set<number>();
    const byID = new Map(teams.map(team => [team.id, team]));
    let currentID = activeTeamID;
    while (currentID != null) {
      const team = byID.get(currentID);
      if (!team) break;
      lineage.add(team.id);
      currentID = team.parent_id ?? null;
    }
    return lineage;
  }, [activeTeamID, teams]);
  const toggleCollapsed = (teamID: number) => {
    setCollapsedIDs(current => {
      const next = new Set(current);
      if (next.has(teamID)) {
        next.delete(teamID);
      } else {
        next.add(teamID);
      }
      return next;
    });
  };

  return (
    <aside className="teams-tree-card">
      <div className="teams-panel-heading">
        <div>
          <h2>Teams</h2>
          <p>{teams.length} resources</p>
        </div>
        <span className="teams-count-badge">{teams.length}</span>
      </div>
      <label className="teams-tree-search" htmlFor="teams-tree-search">
        <Search className="h-4 w-4" aria-hidden="true" />
        <input
          id="teams-tree-search"
          value={searchTerm}
          onChange={event => onSearchTermChange(event.target.value)}
          placeholder="Filter hierarchy"
        />
      </label>
      <nav aria-label="Team hierarchy">
        <button
          type="button"
          className={`teams-tree-row teams-tree-row--root ${activeTeamID == null ? 'active' : ''}`}
          onClick={() => onSelectTeam(null)}
        >
          <UsersRound className="h-4 w-4" aria-hidden="true" />
          <span>Global</span>
          <span className="teams-tree-count">{tree.length}</span>
        </button>
        <ul className="teams-tree-list">
          {tree.map(node => (
            <TeamTreeRow
              key={node.team.id}
              node={node}
              activeTeamID={activeTeamID}
              activeLineageIDs={activeLineageIDs}
              collapsedIDs={collapsedIDs}
              onToggleCollapsed={toggleCollapsed}
              onSelectTeam={onSelectTeam}
            />
          ))}
        </ul>
      </nav>
    </aside>
  );
}

function TeamTreeRow({
  node,
  activeTeamID,
  activeLineageIDs,
  collapsedIDs,
  onToggleCollapsed,
  onSelectTeam,
}: {
  node: TeamTreeNode;
  activeTeamID: number | null;
  activeLineageIDs: Set<number>;
  collapsedIDs: Set<number>;
  onToggleCollapsed: (teamID: number) => void;
  onSelectTeam: (id: number | null) => void;
}) {
  const app = isAppTeam(node.team);
  const active = node.team.id === activeTeamID;
  const hasChildren = node.children.length > 0;
  const collapsed = hasChildren && collapsedIDs.has(node.team.id) && !activeLineageIDs.has(node.team.id);
  const depthStyle = { '--team-tree-depth': node.depth } as CSSProperties;
  return (
    <li>
      <div className={`teams-tree-row ${active ? 'active' : ''}`} style={depthStyle}>
        {hasChildren ? (
          <button
            type="button"
            className={`teams-tree-toggle ${collapsed ? '' : 'expanded'}`}
            aria-label={`${collapsed ? 'Expand' : 'Collapse'} ${teamDisplayName(node.team)}`}
            aria-expanded={!collapsed}
            onClick={() => onToggleCollapsed(node.team.id)}
          >
            <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
          </button>
        ) : (
          <span className="teams-tree-toggle-placeholder" aria-hidden="true" />
        )}
        <button type="button" className="teams-tree-select" onClick={() => onSelectTeam(node.team.id)}>
          {app ? <Boxes className="h-4 w-4" aria-hidden="true" /> : <FolderTree className="h-4 w-4" aria-hidden="true" />}
          <span className="truncate">{teamDisplayName(node.team)}</span>
          {hasChildren ? <span className="teams-tree-count">{node.children.length}</span> : <MoreHorizontal className="teams-tree-more h-4 w-4" aria-hidden="true" />}
        </button>
      </div>
      {hasChildren && !collapsed ? (
        <ul className="teams-tree-list">
          {node.children.map(child => (
            <TeamTreeRow
              key={child.team.id}
              node={child}
              activeTeamID={activeTeamID}
              activeLineageIDs={activeLineageIDs}
              collapsedIDs={collapsedIDs}
              onToggleCollapsed={onToggleCollapsed}
              onSelectTeam={onSelectTeam}
            />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

function TeamDetailHeader({
  team,
  teams,
  stats,
  onDeleteTeam,
  detailTabs,
  activeTab,
  onTabChange,
}: {
  team: Team | null;
  teams: Team[];
  stats: { totalItems: number };
  onDeleteTeam: (team: Team) => void;
  detailTabs: Array<{ id: TeamDetailTabID; label: string }>;
  activeTab: TeamDetailTabID;
  onTabChange: (tab: TeamDetailTabID) => void;
}) {
  const app = team ? isAppTeam(team) : false;
  const label = team ? teamDisplayName(team) : 'Global';
  const path = team ? teamPathForURL(team, teams) : 'global';
  return (
    <header className="teams-detail-header">
      <div className="teams-resource-title">
        <span className={`teams-resource-icon ${app ? 'teams-tone-purple' : 'teams-tone-blue'}`} aria-hidden="true">
          {app ? <Boxes className="h-5 w-5" /> : <UsersRound className="h-5 w-5" />}
        </span>
        <div className="min-w-0">
          <div className="teams-resource-title__line">
            <h2 title={label}>{label}</h2>
            <span className="runner-pill runner-pill--ok">Active</span>
          </div>
          <p>
            {teamKindLabel(team)} · {path} · {stats.totalItems} scoped resources
          </p>
        </div>
      </div>
      <div className="teams-detail-actions">
        {team && !team.navigation_only ? (
          <button type="button" className="teams-icon-btn teams-icon-btn--danger" title={`Delete ${label}`} aria-label={`Delete ${label}`} onClick={() => onDeleteTeam(team)}>
            <Trash2 className="h-4 w-4" aria-hidden="true" />
          </button>
        ) : null}
      </div>
      <div className="teams-tabs" role="tablist" aria-label="Team detail sections">
        {detailTabs.map(tab => (
          <button
            key={tab.id}
            id={`teams-tab-${tab.id}`}
            type="button"
            role="tab"
            aria-selected={tab.id === activeTab}
            aria-controls={`teams-tabpanel-${tab.id}`}
            className={tab.id === activeTab ? 'active' : ''}
            onClick={() => onTabChange(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>
    </header>
  );
}

function TeamOverviewCard({
  team,
  teams,
  stats,
}: {
  team: Team | null;
  teams: Team[];
  stats: { teams: number; applications: number; repositories: number; directChildren: number };
}) {
  const parent = getTeamParent(team, teams);
  const app = team ? isAppTeam(team) : false;
  const repoURL = team ? teamRepositoryURL(team) : '';
  const rows = [
    ['Kind', teamKindLabel(team)],
    ['Path', team ? teamPathForURL(team, teams) : 'global'],
    ['Parent', parent ? teamDisplayName(parent) : 'Global'],
    ['Direct children', String(stats.directChildren)],
    ['Applications', String(stats.applications)],
    ['Repositories', String(stats.repositories)],
    ['Last run', formatTeamTimestamp(team?.last_run_at)],
  ];
  return (
    <article className="teams-card teams-overview-card">
      <div className="teams-card-heading">
        <div>
          <h3>{app ? 'Application Overview' : 'Team Overview'}</h3>
          <p>{team?.description || 'Central place for ownership, access, repositories, and automation resources.'}</p>
        </div>
      </div>
      <dl className="teams-kv-list">
        {rows.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd title={value}>{value}</dd>
          </div>
        ))}
        <div>
          <dt>{app ? 'Repository' : 'GitOps'}</dt>
          <dd>
            {repoURL ? (
              <a href={repoURL} target="_blank" rel="noreferrer" className="teams-inline-link">
                {team ? teamRepositoryLabel(team) || repoURL : repoURL}
                <ArrowUpRight className="h-3.5 w-3.5" aria-hidden="true" />
              </a>
            ) : team && !app ? (
              'Configurable'
            ) : (
              'Not connected'
            )}
          </dd>
        </div>
      </dl>
    </article>
  );
}

function TeamResourcesPanel({
  team,
  teams,
  stats,
  scopedApplications,
  operationsSummary,
  resourceCatalog,
  onTabChange,
}: {
  team: Team | null;
  teams: Team[];
  stats: { teams: number; applications: number; repositories: number };
  scopedApplications: Team[];
  operationsSummary: TeamOperationsSummaryState;
  resourceCatalog: TeamResourceCatalogState;
  onTabChange: (tab: TeamDetailTabID) => void;
}) {
  const notificationCount = operationsSummary.notificationRoute?.definition?.routes?.length;
  const [selectedResourceKind, setSelectedResourceKind] = useState<TeamLinkedResourceKind | null>(null);
  const teamPath = team ? teamPathForURL(team, teams) : '';
  const localResources = useMemo(
    () => [
      ...buildApplicationLinkedResources(scopedApplications, teams),
      ...buildLLMProfileLinkedResources(operationsSummary.llmProfiles?.profiles ?? []),
      ...buildAgentProfileLinkedResources(operationsSummary.agentProfiles?.profiles ?? []),
      ...buildMCPProfileLinkedResources(operationsSummary.mcpProfiles?.profiles ?? []),
    ],
    [
      operationsSummary.agentProfiles?.profiles,
      operationsSummary.llmProfiles?.profiles,
      operationsSummary.mcpProfiles?.profiles,
      scopedApplications,
      teams,
    ]
  );
  const allResources = useMemo(
    () => [...localResources, ...resourceCatalog.resources],
    [localResources, resourceCatalog.resources]
  );
  const resourceCounts = useMemo(() => countLinkedResourcesByKind(allResources), [allResources]);
  const activeResourceKind = selectedResourceKind ?? firstLinkedResourceKind(resourceCounts) ?? 'application';
  const resourceCountLabel = (kind: TeamLinkedResourceKind) => {
    if (AI_RESOURCE_KINDS.has(kind) && operationsSummary.loading) return '-';
    if (CATALOG_RESOURCE_KINDS.has(kind) && resourceCatalog.loading) return '-';
    return resourceCounts[kind];
  };
  const description = team
    ? 'Team-scoped product objects and automation configuration.'
    : 'Organization resources and global automation configuration.';
  const resources = [
    { label: 'Teams', value: stats.teams, icon: <UsersRound className="h-4 w-4" />, tone: 'blue' as const, onClick: () => onTabChange('overview') },
    { label: 'Applications', value: resourceCountLabel('application'), icon: <Boxes className="h-4 w-4" />, tone: 'purple' as const, active: activeResourceKind === 'application', onClick: () => setSelectedResourceKind('application') },
    { label: 'LLM Profiles', value: resourceCountLabel('llm_profile'), icon: <Bot className="h-4 w-4" />, tone: 'purple' as const, active: activeResourceKind === 'llm_profile', onClick: () => setSelectedResourceKind('llm_profile') },
    { label: 'Agent Profiles', value: resourceCountLabel('agent_profile'), icon: <ShieldCheck className="h-4 w-4" />, tone: 'blue' as const, active: activeResourceKind === 'agent_profile', onClick: () => setSelectedResourceKind('agent_profile') },
    { label: 'MCP Profiles', value: resourceCountLabel('mcp_profile'), icon: <Route className="h-4 w-4" />, tone: 'green' as const, active: activeResourceKind === 'mcp_profile', onClick: () => setSelectedResourceKind('mcp_profile') },
    ...(team ? [{ label: 'Notifications', value: typeof notificationCount === 'number' ? notificationCount : '-', icon: <Bell className="h-4 w-4" />, tone: 'cyan' as const, onClick: () => onTabChange('notifications') }] : []),
    { label: 'Pipelines', value: resourceCountLabel('pipeline'), icon: <GitBranch className="h-4 w-4" />, tone: 'green' as const, active: activeResourceKind === 'pipeline', onClick: () => setSelectedResourceKind('pipeline') },
    { label: 'Steps', value: resourceCountLabel('step'), icon: <ListChecks className="h-4 w-4" />, tone: 'blue' as const, active: activeResourceKind === 'step', onClick: () => setSelectedResourceKind('step') },
    { label: 'Triggers', value: resourceCountLabel('trigger'), icon: <Route className="h-4 w-4" />, tone: 'cyan' as const, active: activeResourceKind === 'trigger', onClick: () => setSelectedResourceKind('trigger') },
    { label: 'External Triggers', value: resourceCountLabel('external_trigger'), icon: <Webhook className="h-4 w-4" />, tone: 'cyan' as const, active: activeResourceKind === 'external_trigger', onClick: () => setSelectedResourceKind('external_trigger') },
    { label: 'Git Webhook Sources', value: resourceCountLabel('git_webhook_source'), icon: <Webhook className="h-4 w-4" />, tone: 'green' as const, active: activeResourceKind === 'git_webhook_source', onClick: () => setSelectedResourceKind('git_webhook_source') },
    { label: 'Schedules', value: resourceCountLabel('schedule'), icon: <Clock className="h-4 w-4" />, tone: 'purple' as const, active: activeResourceKind === 'schedule', onClick: () => setSelectedResourceKind('schedule') },
    { label: 'Knowledge Context', value: resourceCountLabel('knowledge_context'), icon: <BookOpen className="h-4 w-4" />, tone: 'blue' as const, active: activeResourceKind === 'knowledge_context', onClick: () => setSelectedResourceKind('knowledge_context') },
    { label: 'Scopes', value: resourceCountLabel('scope'), icon: <FolderTree className="h-4 w-4" />, tone: 'purple' as const, active: activeResourceKind === 'scope', onClick: () => setSelectedResourceKind('scope') },
    { label: 'Credentials', value: resourceCountLabel('credential'), icon: <ShieldCheck className="h-4 w-4" />, tone: 'blue' as const, active: activeResourceKind === 'credential', onClick: () => setSelectedResourceKind('credential') },
  ];
  return (
    <article className="teams-card teams-resources-card">
      <div className="teams-card-heading">
        <div>
          <h3>Resources</h3>
          <p>{description}</p>
        </div>
      </div>
      <div className="teams-resource-grid">
        {resources.map(resource => (
          <TeamResourceTile key={resource.label} resource={resource} />
        ))}
      </div>
      <TeamLinkedResourcesBrowser
        resources={allResources}
        activeKind={activeResourceKind}
        loading={selectedResourceKindLoading(activeResourceKind, operationsSummary.loading, resourceCatalog.loading)}
        error={selectedResourceKindError(activeResourceKind, operationsSummary.aiProfilesError, resourceCatalog.error)}
        teamPath={teamPath}
      />
    </article>
  );
}

function TeamLinkedResourcesBrowser({
  resources,
  activeKind,
  loading,
  error,
  teamPath,
}: {
  resources: TeamLinkedResource[];
  activeKind: TeamLinkedResourceKind;
  loading: boolean;
  error: string | null;
  teamPath: string;
}) {
  const activeResources = useMemo(
    () => resources.filter(resource => resource.kind === activeKind),
    [activeKind, resources]
  );
  const activeLabel = TEAM_LINKED_RESOURCE_LABELS[activeKind];
  const emptyLabel = teamPath
    ? `No ${activeLabel.toLowerCase()} found for this team.`
    : `No global ${activeLabel.toLowerCase()} found.`;

  return (
    <div className="teams-linked-resources" aria-label="Selected resources">
      {error ? <p className="teams-linked-resources__error">{error}</p> : null}
      <section
        id="teams-linked-resource-panel"
        className="teams-profile-summary teams-linked-resource-summary"
        aria-label={`${activeLabel} resources`}
      >
        <div className="teams-table-heading">
          <h3>{activeLabel}</h3>
          <span>{activeResources.length} items</span>
        </div>
        {loading && activeResources.length === 0 ? (
          <div className="teams-profile-summary__empty">Loading {activeLabel.toLowerCase()}...</div>
        ) : activeResources.length === 0 ? (
          <div className="teams-profile-summary__empty">{emptyLabel}</div>
        ) : (
          <div className="teams-profile-summary__list">
            {activeResources.map(resource => (
              <TeamLinkedResourceRow key={resource.id} resource={resource} />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function TeamLinkedResourceRow({ resource }: { resource: TeamLinkedResource }) {
  return (
    <Link className="teams-profile-summary__row teams-linked-resource-row" to={resource.href} aria-label={`Open ${resource.label}`}>
      <span className={`teams-linked-resource-row__icon teams-linked-resource-row__icon--${resource.kind}`} aria-hidden="true">
        {linkedResourceIcon(resource.kind)}
      </span>
      <div className="min-w-0">
        <strong title={resource.label}>{resource.label}</strong>
        <p title={resource.description}>{resource.description}</p>
      </div>
      <div className="teams-profile-summary__tags">
        {resource.teamPath ? <span className="runner-pill runner-pill--muted">{resource.teamPath}</span> : null}
        {resource.source ? <span className="runner-pill runner-pill--muted">{resource.source}</span> : null}
        <span className="runner-pill runner-pill--muted">Open</span>
      </div>
    </Link>
  );
}

const TEAM_LINKED_RESOURCE_KINDS: TeamLinkedResourceKind[] = [
  'application',
  'llm_profile',
  'agent_profile',
  'mcp_profile',
  'pipeline',
  'step',
  'trigger',
  'external_trigger',
  'git_webhook_source',
  'schedule',
  'knowledge_context',
  'scope',
  'credential',
];

const AI_RESOURCE_KINDS = new Set<TeamLinkedResourceKind>(['llm_profile', 'agent_profile', 'mcp_profile']);
const CATALOG_RESOURCE_KINDS = new Set<TeamLinkedResourceKind>([
  'pipeline',
  'step',
  'trigger',
  'external_trigger',
  'git_webhook_source',
  'schedule',
  'knowledge_context',
  'scope',
  'credential',
]);

function buildApplicationLinkedResources(applications: Team[], teams: Team[]): TeamLinkedResource[] {
  return applications.map(application => {
    const path = teamPathForURL(application, teams);
    const repository = teamRepositoryLabel(application);
    const label = teamDisplayName(application);
    return {
      id: `application:${application.id}`,
      kind: 'application',
      label,
      description: [repository, path].filter(Boolean).join(' / ') || 'Application',
      href: teamScopedRoute('/teams', path),
      teamPath: path,
      source: repository ? 'repository' : undefined,
    };
  });
}

function buildLLMProfileLinkedResources(profiles: NonNullable<TeamOperationsSummaryState['llmProfiles']>['profiles'] = []): TeamLinkedResource[] {
  return profiles.map(profile => {
    const teamPath = profile.team_path || '';
    return {
      id: `llm_profile:${teamPath || 'global'}:${profile.name}`,
      kind: 'llm_profile',
      label: profile.name,
      description: [profile.provider, profile.model, profile.credential_ref].filter(Boolean).join(' / ') || 'LLM profile',
      href: `/llm-profiles${aiResourceQuery(teamPath)}`,
      teamPath,
      source: profile.status || undefined,
    };
  });
}

function buildAgentProfileLinkedResources(profiles: NonNullable<TeamOperationsSummaryState['agentProfiles']>['profiles'] = []): TeamLinkedResource[] {
  return profiles.map(profile => {
    const teamPath = profile.team_path || '';
    return {
      id: `agent_profile:${teamPath || 'global'}:${profile.id}`,
      kind: 'agent_profile',
      label: profile.display_name || profile.id,
      description: [profile.id, profile.role, profile.description].filter(Boolean).join(' / ') || 'Agent profile',
      href: `/agent-profiles${aiResourceQuery(teamPath)}`,
      teamPath,
      source: profile.enabled === false ? 'disabled' : profile.source,
    };
  });
}

function buildMCPProfileLinkedResources(profiles: NonNullable<TeamOperationsSummaryState['mcpProfiles']>['profiles'] = []): TeamLinkedResource[] {
  return profiles.map(profile => {
    const teamPath = profile.team_path || '';
    const servers = (profile.servers ?? []).map(ref => ref.server).filter(Boolean);
    return {
      id: `mcp_profile:${teamPath || 'global'}:${profile.name}`,
      kind: 'mcp_profile',
      label: profile.name,
      description: servers.length ? servers.join(', ') : profile.description || 'MCP profile',
      href: `/mcp${aiResourceQuery(teamPath, '&view=profiles')}`,
      teamPath,
      source: profile.enabled === false ? 'disabled' : undefined,
    };
  });
}

function aiResourceQuery(teamPath: string, suffix = '') {
  const query = teamPath ? `?team=${encodeURIComponent(teamPath)}` : '?team=global';
  return `${query}${suffix}`;
}

function countLinkedResourcesByKind(resources: TeamLinkedResource[]): Record<TeamLinkedResourceKind, number> {
  const counts: Record<TeamLinkedResourceKind, number> = {
    application: 0,
    llm_profile: 0,
    agent_profile: 0,
    mcp_profile: 0,
    pipeline: 0,
    step: 0,
    trigger: 0,
    external_trigger: 0,
    git_webhook_source: 0,
    schedule: 0,
    knowledge_context: 0,
    scope: 0,
    credential: 0,
  };
  resources.forEach(resource => {
    counts[resource.kind] += 1;
  });
  return counts;
}

function firstLinkedResourceKind(counts: Record<TeamLinkedResourceKind, number>): TeamLinkedResourceKind | null {
  return TEAM_LINKED_RESOURCE_KINDS.find(kind => counts[kind] > 0) ?? null;
}

function selectedResourceKindLoading(
  kind: TeamLinkedResourceKind,
  aiLoading: boolean,
  catalogLoading: boolean
) {
  return (AI_RESOURCE_KINDS.has(kind) && aiLoading) || (CATALOG_RESOURCE_KINDS.has(kind) && catalogLoading);
}

function selectedResourceKindError(
  kind: TeamLinkedResourceKind,
  aiError: string | null,
  catalogError: string | null
) {
  if (AI_RESOURCE_KINDS.has(kind)) return aiError;
  if (CATALOG_RESOURCE_KINDS.has(kind)) return catalogError;
  return null;
}

function linkedResourceIcon(kind: TeamLinkedResourceKind) {
  if (kind === 'application') return <Boxes className="h-4 w-4" />;
  if (kind === 'llm_profile') return <Bot className="h-4 w-4" />;
  if (kind === 'agent_profile') return <ShieldCheck className="h-4 w-4" />;
  if (kind === 'mcp_profile') return <Route className="h-4 w-4" />;
  if (kind === 'pipeline') return <GitBranch className="h-4 w-4" />;
  if (kind === 'step') return <ListChecks className="h-4 w-4" />;
  if (kind === 'trigger') return <Route className="h-4 w-4" />;
  if (kind === 'external_trigger') return <Webhook className="h-4 w-4" />;
  if (kind === 'git_webhook_source') return <Webhook className="h-4 w-4" />;
  if (kind === 'schedule') return <Clock className="h-4 w-4" />;
  if (kind === 'knowledge_context') return <BookOpen className="h-4 w-4" />;
  if (kind === 'scope') return <FolderTree className="h-4 w-4" />;
  return <ShieldCheck className="h-4 w-4" />;
}

function TeamResourceTile({
  resource,
}: {
  resource: {
    label: string;
    value: number | string;
    icon: ReactNode;
    tone: 'blue' | 'purple' | 'green' | 'cyan';
    active?: boolean;
    to?: string;
    onClick?: () => void;
  };
}) {
  const content = (
    <>
      <span className={`teams-resource-mini__icon teams-tone-${resource.tone}`} aria-hidden="true">
        {resource.icon}
      </span>
      <p>{resource.label}</p>
      <strong>{resource.value}</strong>
    </>
  );
  const label = `Open ${resource.label}`;
  if (resource.to) {
    return (
      <Link className={`teams-resource-mini ${resource.active ? 'active' : ''}`} to={resource.to} aria-label={label}>
        {content}
      </Link>
    );
  }
  return (
    <button type="button" className={`teams-resource-mini ${resource.active ? 'active' : ''}`} onClick={resource.onClick} aria-label={label} aria-pressed={resource.active}>
      {content}
    </button>
  );
}
