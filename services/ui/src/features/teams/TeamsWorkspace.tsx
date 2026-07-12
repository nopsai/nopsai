import { useMemo, useState, type CSSProperties, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  ArrowUpRight,
  Bell,
  Bot,
  Boxes,
  ChevronRight,
  FolderTree,
  GitBranch,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Route,
  Search,
  Settings,
  ShieldCheck,
  Trash2,
  UsersRound,
} from 'lucide-react';
import {
  isAppTeam,
  teamDisplayName,
  teamPathForURL,
  teamRepositoryLabel,
  teamRepositoryURL,
  type Team,
} from '../../lib/teamModels';
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
    activeDetailTab,
    directChildren,
    scopedApplications,
    searching,
    visibleItems,
  });
  const activeLabel = activeTeam ? teamDisplayName(activeTeam) : 'Root';
  const emptySelection = activeDetailTab === 'overview' && Boolean(activeTeam) && !searching && directChildren.length === 0;
  const tableCopy = getTeamTableCopy({ activeDetailTab, activeLabel, searching });
  const selectDetailTab = (tab: TeamDetailTabID) => setDetailTabSelection({ teamID: activeTeamID, tab });

  return (
    <div className="teams-page-shell">
      <header className="teams-page-toolbar">
        <div className="teams-title-block">
          <p className="teams-eyebrow">Enterprise organization</p>
          <p className="teams-page-description">
            Organize teams, applications, GitOps repositories, notifications, access, and team-owned AI profiles.
          </p>
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

      <section className="teams-stat-grid" aria-label="Team summary">
        <MetricCard label="Total Teams" value={stats.teams} icon={<UsersRound className="h-4 w-4" />} tone="blue" />
        <MetricCard label="Applications" value={stats.applications} icon={<Boxes className="h-4 w-4" />} tone="purple" />
        <MetricCard label="Repositories" value={stats.repositories} icon={<GitBranch className="h-4 w-4" />} tone="green" />
        <MetricCard label="Recent Runs" value={stats.recentRuns} icon={<ShieldCheck className="h-4 w-4" />} tone="cyan" />
      </section>

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
            onOpenConfig={onOpenConfig}
            detailTabs={detailTabs}
            activeTab={activeDetailTab}
            onTabChange={selectDetailTab}
          />
          {activeDetailTab === 'overview' ? (
            <>
              <div className="teams-detail-grid" role="tabpanel" id="teams-tabpanel-overview" aria-labelledby="teams-tab-overview">
                <TeamOverviewCard team={activeTeam} teams={teams} stats={stats} onOpenConfig={onOpenConfig} />
                <TeamActivityCard team={activeTeam} stats={stats} directChildren={directChildren} />
              </div>
              <TeamResourcesPanel
                team={activeTeam}
                teams={teams}
                stats={stats}
                operationsSummary={operationsSummary}
                onTabChange={selectDetailTab}
              />
            </>
          ) : (
            <TeamTabPanel
              activeTab={activeDetailTab}
              team={activeTeam}
              teams={teams}
              stats={stats}
              scopedApplications={scopedApplications}
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
        Root
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

function MetricCard({
  label,
  value,
  icon,
  tone,
}: {
  label: string;
  value: number;
  icon: ReactNode;
  tone: 'blue' | 'purple' | 'green' | 'cyan';
}) {
  return (
    <article className="teams-stat-card">
      <div>
        <p>{label}</p>
        <strong>{value}</strong>
      </div>
      <span className={`teams-stat-card__icon teams-tone-${tone}`} aria-hidden="true">
        {icon}
      </span>
    </article>
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
  const tree = buildTeamTree(teams);
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
          <span>Root</span>
          <span className="teams-tree-count">{tree.length}</span>
        </button>
        <ul className="teams-tree-list">
          {tree.map(node => (
            <TeamTreeRow key={node.team.id} node={node} activeTeamID={activeTeamID} onSelectTeam={onSelectTeam} />
          ))}
        </ul>
      </nav>
    </aside>
  );
}

function TeamTreeRow({
  node,
  activeTeamID,
  onSelectTeam,
}: {
  node: TeamTreeNode;
  activeTeamID: number | null;
  onSelectTeam: (id: number | null) => void;
}) {
  const app = isAppTeam(node.team);
  const active = node.team.id === activeTeamID;
  const depthStyle = { '--team-tree-depth': node.depth } as CSSProperties;
  return (
    <li>
      <button
        type="button"
        className={`teams-tree-row ${active ? 'active' : ''}`}
        style={depthStyle}
        onClick={() => onSelectTeam(node.team.id)}
      >
        {app ? <Boxes className="h-4 w-4" aria-hidden="true" /> : <FolderTree className="h-4 w-4" aria-hidden="true" />}
        <span className="truncate">{teamDisplayName(node.team)}</span>
        {node.children.length > 0 ? <span className="teams-tree-count">{node.children.length}</span> : <MoreHorizontal className="ml-auto h-4 w-4" aria-hidden="true" />}
      </button>
      {node.children.length > 0 ? (
        <ul className="teams-tree-list">
          {node.children.map(child => (
            <TeamTreeRow key={child.team.id} node={child} activeTeamID={activeTeamID} onSelectTeam={onSelectTeam} />
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
  onOpenConfig,
  detailTabs,
  activeTab,
  onTabChange,
}: {
  team: Team | null;
  teams: Team[];
  stats: { totalItems: number };
  onDeleteTeam: (team: Team) => void;
  onOpenConfig: (team: Team, tab?: 'sync' | 'notifications') => void;
  detailTabs: Array<{ id: TeamDetailTabID; label: string }>;
  activeTab: TeamDetailTabID;
  onTabChange: (tab: TeamDetailTabID) => void;
}) {
  const app = team ? isAppTeam(team) : false;
  const label = team ? teamDisplayName(team) : 'Root';
  const path = team ? teamPathForURL(team, teams) : 'root';
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
        {team && !app ? (
          <button type="button" className="teams-secondary-btn" onClick={() => onOpenConfig(team)}>
            <Settings className="h-4 w-4" aria-hidden="true" />
            GitOps & Notifications
          </button>
        ) : null}
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
  onOpenConfig,
}: {
  team: Team | null;
  teams: Team[];
  stats: { teams: number; applications: number; repositories: number; directChildren: number };
  onOpenConfig: (team: Team, tab?: 'sync' | 'notifications') => void;
}) {
  const parent = getTeamParent(team, teams);
  const app = team ? isAppTeam(team) : false;
  const repoURL = team ? teamRepositoryURL(team) : '';
  const rows = [
    ['Kind', teamKindLabel(team)],
    ['Path', team ? teamPathForURL(team, teams) : 'root'],
    ['Parent', parent ? teamDisplayName(parent) : 'Root'],
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
        {team && !app ? (
          <button type="button" className="teams-icon-btn" title="Open team settings" aria-label="Open team settings" onClick={() => onOpenConfig(team)}>
            <Settings className="h-4 w-4" aria-hidden="true" />
          </button>
        ) : null}
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
  operationsSummary,
  onTabChange,
}: {
  team: Team | null;
  teams: Team[];
  stats: { teams: number; applications: number; repositories: number };
  operationsSummary: TeamOperationsSummaryState;
  onTabChange: (tab: TeamDetailTabID) => void;
}) {
  const llmCount = operationsSummary.llmProfiles?.profiles?.length;
  const agentCount = operationsSummary.agentProfiles?.profiles?.length;
  const mcpCount = operationsSummary.mcpProfiles?.profiles?.length;
  const notificationCount = operationsSummary.notificationRoute?.definition?.routes?.length;
  const teamPath = team ? teamPathForURL(team, teams) : '';
  const encodedTeam = encodeURIComponent(teamPath);
  const teamQuery = teamPath ? `?team=${encodedTeam}` : '?team=global';
  const mcpQuery = teamPath ? `?team=${encodedTeam}&view=profiles` : '?team=global&view=profiles';
  const description = team
    ? 'Team-scoped product objects and automation configuration.'
    : 'Organization resources and global automation configuration.';
  const resources = [
    { label: 'Teams', value: stats.teams, icon: <UsersRound className="h-4 w-4" />, tone: 'blue' as const, onClick: () => onTabChange('overview') },
    { label: 'Applications', value: stats.applications, icon: <Boxes className="h-4 w-4" />, tone: 'purple' as const, onClick: () => onTabChange('applications') },
    { label: 'Repositories', value: stats.repositories, icon: <GitBranch className="h-4 w-4" />, tone: 'green' as const, onClick: () => onTabChange('gitops') },
    { label: 'LLM Profiles', value: typeof llmCount === 'number' ? llmCount : '-', icon: <Bot className="h-4 w-4" />, tone: 'purple' as const, to: `/llm-profiles${teamQuery}` },
    { label: 'Agent Profiles', value: typeof agentCount === 'number' ? agentCount : '-', icon: <ShieldCheck className="h-4 w-4" />, tone: 'blue' as const, to: `/agent-profiles${teamQuery}` },
    { label: 'MCP Profiles', value: typeof mcpCount === 'number' ? mcpCount : '-', icon: <Route className="h-4 w-4" />, tone: 'green' as const, to: `/mcp${mcpQuery}` },
    ...(team ? [{ label: 'Notifications', value: typeof notificationCount === 'number' ? notificationCount : '-', icon: <Bell className="h-4 w-4" />, tone: 'cyan' as const, onClick: () => onTabChange('notifications') }] : []),
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
    </article>
  );
}

function TeamResourceTile({
  resource,
}: {
  resource: {
    label: string;
    value: number | string;
    icon: ReactNode;
    tone: 'blue' | 'purple' | 'green' | 'cyan';
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
      <Link className="teams-resource-mini" to={resource.to} aria-label={label}>
        {content}
      </Link>
    );
  }
  return (
    <button type="button" className="teams-resource-mini" onClick={resource.onClick} aria-label={label}>
      {content}
    </button>
  );
}
