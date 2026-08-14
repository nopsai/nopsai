import { useCallback, useMemo, useState, type CSSProperties, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  Bell,
  Boxes,
  BrainCircuit,
  ChevronRight,
  FolderTree,
  MoreHorizontal,
  Pencil,
  Plus,
  Search,
  Trash2,
  UsersRound,
} from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import { AIResourceExpandableSearch } from '../system/AIResourcePanel';
import type { ObjectIconType } from '../../components/objectIconRegistry';
import { TreeColumnResizeHandle } from '../../components/resizableTreeColumn';
import { useResizableTreeColumn } from '../../components/resizableTreeColumnState';
import {
  isAppTeam,
  teamDisplayName,
  teamPathForURL,
  teamRepositoryLabel,
  type Team,
} from '../../lib/teamModels';
import { teamScopedRoute } from '../../lib/teamRoutes';
import type { CurrentUser } from '../../app/types';
import type { NewTeamItemKind } from './TeamSettingsModals';
import {
  buildTeamScopeStats,
  buildTeamTree,
  getTeamDirectChildren,
  getTeamSubtree,
  getVisibleTeamItems,
  teamKindLabel,
  type TeamScopeStats,
  type TeamTreeNode,
} from './model';
import {
  TeamApplicationOverviewCard,
  TeamChildrenTable,
  TeamOverviewCard,
  TeamTabPanel,
} from './TeamsWorkspacePanels';
import { AnalysisModal } from '../analysis/AnalysisModal';
import { buildTeamResourceAnalysis } from '../analysis/model';
import { RunnerAssignmentsPanel } from '../system/dispatcher/RunnerAssignmentsPanel';
import type { DispatcherStatusState } from '../system/dispatcher/model';
import { buildTeamAnalysisPromptContext } from './teamAnalysisEvidence';
import {
  getTeamTableCopy,
  getTeamTableItems,
  visibleTeamDetailTabs,
  type TeamDetailTabID,
} from './workspaceModel';
import type { TeamOperationsSummaryState } from './hooks/useTeamOperationsSummary';
import {
  APPLICATION_RELATED_RESOURCE_KINDS,
  TEAM_LINKED_RESOURCE_LABELS,
  filterApplicationLinkedResources,
  type TeamLinkedResource,
  type TeamLinkedResourceKind,
  type TeamResourceCatalogState,
} from './resourceCatalogModel';
import type { TeamDefaultsPayload } from './api';
import './teams.css';

export function TeamsWorkspace({
  teams,
  activeTeam,
  activeTeamPath,
  searchTerm,
  onSearchTermChange,
  onSelectTeam,
  onCreate,
  onEditTeam,
  onDeleteTeam,
  onOpenConfig,
  onSaveTeamDefaults,
  operationsSummary,
  resourceCatalog,
  runnerStatus,
  runnerStatusLoading = false,
  runnerStatusError = null,
  currentUser,
}: {
  teams: Team[];
  activeTeam: Team | null;
  activeTeamPath: Team[];
  searchTerm: string;
  onSearchTermChange: (value: string) => void;
  onSelectTeam: (id: number | null) => void;
  onCreate: (kind?: NewTeamItemKind) => void;
  onEditTeam: (team: Team) => void;
  onDeleteTeam: (team: Team) => void;
  onOpenConfig: (team: Team, tab?: 'sync' | 'notifications') => void;
  onSaveTeamDefaults?: (teamPath: string, defaults: TeamDefaultsPayload) => Promise<void>;
  operationsSummary: TeamOperationsSummaryState;
  resourceCatalog: TeamResourceCatalogState;
  runnerStatus?: DispatcherStatusState | null;
  runnerStatusLoading?: boolean;
  runnerStatusError?: string | null;
  currentUser?: CurrentUser | null;
}) {
  const activeTeamID = activeTeam?.id ?? null;
  const activeTeamIsApp = Boolean(activeTeam && isAppTeam(activeTeam));
  const defaultDetailTab: TeamDetailTabID = 'overview';
  const [detailTabSelection, setDetailTabSelection] = useState<{ teamID: number | null; tab: TeamDetailTabID }>({
    teamID: activeTeamID,
    tab: defaultDetailTab,
  });
  const detailTabs = visibleTeamDetailTabs(activeTeam);
  const selectedDetailTab = detailTabSelection.teamID === activeTeamID ? detailTabSelection.tab : defaultDetailTab;
  const activeDetailTab = detailTabs.some(tab => tab.id === selectedDetailTab) ? selectedDetailTab : defaultDetailTab;
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
  const activeRunnerScope = activeTeam ? teamPathForURL(activeTeam, teams) : '';
  const runnerAssignmentDescription = activeTeamIsApp
    ? 'Routes available to this application scope.'
    : activeTeam
      ? 'Routes for this team scope and subgroup scopes.'
      : 'All effective runner routes.';
  const emptySelection = activeDetailTab === 'overview' && Boolean(activeTeam) && !activeTeamIsApp && !searching && directChildren.length === 0;
  const showChildrenTable = activeDetailTab === 'overview' && (!activeTeamIsApp || searching);
  const tableCopy = getTeamTableCopy({ activeLabel, searching });
  const selectDetailTab = (tab: TeamDetailTabID) => setDetailTabSelection({ teamID: activeTeamID, tab });
  const treeResize = useResizableTreeColumn({
    storageKey: 'teams',
    defaultWidth: 340,
    minWidth: 300,
    maxWidth: 560,
  });

  return (
    <div className="teams-page-shell">
      <header className="teams-page-toolbar">
        <div className="teams-title-block">
          <TeamBreadcrumb path={activeTeamPath} activeTeamID={activeTeamID} onSelectTeam={onSelectTeam} />
        </div>
        <div className="teams-toolbar-actions">
          <AIResourceExpandableSearch
            label="Search teams"
            placeholder="Search teams, apps, repositories"
            value={searchTerm}
            onChange={onSearchTermChange}
            className="teams-search"
          />
          <button type="button" className="teams-primary-btn teams-create-btn" aria-label="New team" title="New team" onClick={() => onCreate('team')}>
            <Plus className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </header>

      <div className="teams-master-detail" style={treeResize.gridStyle}>
        <TeamTreePanel
          teams={teams}
          activeTeamID={activeTeamID}
          searchTerm={searchTerm}
          onSearchTermChange={onSearchTermChange}
          onSelectTeam={onSelectTeam}
        />
        <TreeColumnResizeHandle {...treeResize} label="Resize team tree" />
        <section className="teams-detail-stack" aria-label={`${activeLabel} details`}>
          <TeamDetailHeader
            team={activeTeam}
            teams={teams}
            stats={stats}
            onDeleteTeam={onDeleteTeam}
            onEditTeam={onEditTeam}
            detailTabs={detailTabs}
            activeTab={activeDetailTab}
            onTabChange={selectDetailTab}
          />
          {activeDetailTab === 'overview' ? (
            <>
              {activeTeamIsApp && activeTeam ? (
                <div className="teams-tab-panel" role="tabpanel" id="teams-tabpanel-overview" aria-labelledby="teams-tab-overview">
                  <TeamApplicationOverviewCard team={activeTeam} teams={teams} />
                </div>
              ) : (
                <div className="teams-tab-panel" role="tabpanel" id="teams-tabpanel-overview" aria-labelledby="teams-tab-overview">
                  <TeamOverviewCard team={activeTeam} teams={teams} stats={stats} operationsSummary={operationsSummary} />
                </div>
              )}
              <TeamResourcesPanel
                team={activeTeam}
                teams={teams}
                stats={stats}
                scopedApplications={scopedApplications}
                operationsSummary={operationsSummary}
                resourceCatalog={resourceCatalog}
                onTabChange={selectDetailTab}
              />
              <RunnerAssignmentsPanel
                description={runnerAssignmentDescription}
                targetScope={activeRunnerScope}
                includeDescendantScopes={!activeTeamIsApp}
                status={runnerStatus}
                loading={runnerStatusLoading}
                error={runnerStatusError}
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
              onSaveTeamDefaults={onSaveTeamDefaults}
            />
          )}
          {showChildrenTable ? (
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
              onEditTeam={onEditTeam}
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
  const [expandedIDs, setExpandedIDs] = useState<Set<number>>(() => new Set());
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
  const toggleExpanded = (teamID: number) => {
    setExpandedIDs(current => {
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
              expandedIDs={expandedIDs}
              onToggleExpanded={toggleExpanded}
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
  expandedIDs,
  onToggleExpanded,
  onSelectTeam,
}: {
  node: TeamTreeNode;
  activeTeamID: number | null;
  activeLineageIDs: Set<number>;
  expandedIDs: Set<number>;
  onToggleExpanded: (teamID: number) => void;
  onSelectTeam: (id: number | null) => void;
}) {
  const app = isAppTeam(node.team);
  const active = node.team.id === activeTeamID;
  const hasChildren = node.children.length > 0;
  const expanded = hasChildren && (activeLineageIDs.has(node.team.id) || expandedIDs.has(node.team.id));
  const depthStyle = { '--team-tree-depth': node.depth } as CSSProperties;
  return (
    <li>
      <div className={`teams-tree-row ${active ? 'active' : ''}`} style={depthStyle}>
        {hasChildren ? (
          <button
            type="button"
            className={`teams-tree-toggle ${expanded ? 'expanded' : ''}`}
            aria-label={`${expanded ? 'Collapse' : 'Expand'} ${teamDisplayName(node.team)}`}
            aria-expanded={expanded}
            onClick={() => onToggleExpanded(node.team.id)}
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
      {expanded ? (
        <ul className="teams-tree-list">
          {node.children.map(child => (
            <TeamTreeRow
              key={child.team.id}
              node={child}
              activeTeamID={activeTeamID}
              activeLineageIDs={activeLineageIDs}
              expandedIDs={expandedIDs}
              onToggleExpanded={onToggleExpanded}
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
  onEditTeam,
  detailTabs,
  activeTab,
  onTabChange,
}: {
  team: Team | null;
  teams: Team[];
  stats: { totalItems: number };
  onDeleteTeam: (team: Team) => void;
  onEditTeam: (team: Team) => void;
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
          <>
            <button type="button" className="teams-icon-btn" title={`Edit ${label}`} aria-label={`Edit ${label}`} onClick={() => onEditTeam(team)}>
              <Pencil className="h-4 w-4" aria-hidden="true" />
            </button>
            <button type="button" className="teams-icon-btn teams-icon-btn--danger" title={`Delete ${label}`} aria-label={`Delete ${label}`} onClick={() => onDeleteTeam(team)}>
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </button>
          </>
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

function applicationResourceTiles(
  resourceCounts: Record<TeamLinkedResourceKind, number>,
  activeResourceKind: TeamLinkedResourceKind,
  setSelectedResourceKind: (kind: TeamLinkedResourceKind) => void
) {
  const visibleKinds = APPLICATION_RELATED_RESOURCE_KINDS.filter(kind => resourceCounts[kind] > 0);
  return visibleKinds.map(kind => ({
    label: TEAM_LINKED_RESOURCE_LABELS[kind],
    value: resourceCounts[kind],
    icon: linkedResourceIcon(kind),
    tone: linkedResourceTone(kind),
    active: activeResourceKind === kind,
    onClick: () => setSelectedResourceKind(kind),
  }));
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
  stats: TeamScopeStats;
  scopedApplications: Team[];
  operationsSummary: TeamOperationsSummaryState;
  resourceCatalog: TeamResourceCatalogState;
  onTabChange: (tab: TeamDetailTabID) => void;
}) {
  const notificationCount = operationsSummary.notificationRoute?.definition?.routes?.length;
  const [selectedResourceKind, setSelectedResourceKind] = useState<TeamLinkedResourceKind | null>(null);
  const [analysisRequest, setAnalysisRequest] = useState<{ resource: TeamLinkedResource | null } | null>(null);
  const teamPath = team ? teamPathForURL(team, teams) : '';
  const app = team ? isAppTeam(team) : false;
  const subjectLabel = team ? teamDisplayName(team) : 'Global';
  const localResources = useMemo(
    () => app
      ? []
      : [
          ...buildApplicationLinkedResources(scopedApplications, teams),
          ...buildLLMProfileLinkedResources(operationsSummary.llmProfiles?.profiles ?? []),
          ...buildAgentProfileLinkedResources(operationsSummary.agentProfiles?.profiles ?? []),
          ...buildMCPProfileLinkedResources(operationsSummary.mcpProfiles?.profiles ?? []),
        ],
    [
      app,
      operationsSummary.agentProfiles?.profiles,
      operationsSummary.llmProfiles?.profiles,
      operationsSummary.mcpProfiles?.profiles,
      scopedApplications,
      teams,
    ]
  );
  const catalogResources = useMemo(
    () => (app && team
      ? filterApplicationLinkedResources(resourceCatalog.resources, {
          appPath: teamPathForURL(team, teams),
          appName: teamDisplayName(team),
          repository: teamRepositoryLabel(team),
          repoURL: team.repo_url,
        })
      : resourceCatalog.resources),
    [app, resourceCatalog.resources, team, teams]
  );
  const allResources = useMemo(
    () => [...localResources, ...catalogResources],
    [catalogResources, localResources]
  );
  const analysisResult = useMemo(
    () => analysisRequest
      ? buildTeamResourceAnalysis({
          subjectId: team ? String(team.id) : 'global',
          subjectLabel,
          scopePath: teamPath,
          resources: allResources,
          activeResource: analysisRequest.resource,
        })
      : null,
    [allResources, analysisRequest, subjectLabel, team, teamPath]
  );
  const loadAnalysisPromptContext = useCallback(
    async () => buildTeamAnalysisPromptContext({
      team,
      teams,
      stats,
      subjectLabel,
      scopePath: teamPath,
      resources: allResources,
      activeResource: analysisRequest?.resource || null,
      operationsSummary,
      resourceCatalog,
    }),
    [allResources, analysisRequest?.resource, operationsSummary, resourceCatalog, stats, subjectLabel, team, teamPath, teams]
  );
  const resourceCounts = useMemo(() => countLinkedResourcesByKind(allResources), [allResources]);
  const firstResourceKind = firstLinkedResourceKind(resourceCounts);
  const activeResourceKind = selectedResourceKind && resourceCounts[selectedResourceKind] > 0
    ? selectedResourceKind
    : firstResourceKind ?? (app ? 'trigger' : 'application');
  const resourceCountLabel = (kind: TeamLinkedResourceKind) => {
    if (AI_RESOURCE_KINDS.has(kind) && operationsSummary.loading) return '-';
    if (CATALOG_RESOURCE_KINDS.has(kind) && resourceCatalog.loading) return '-';
    return resourceCounts[kind];
  };
  const description = app
    ? 'Application-specific automation and configuration linked by app path or repository identity.'
    : team
      ? 'Team-scoped product objects and automation configuration.'
      : 'Organization resources and global automation configuration.';
  const resources = app
    ? applicationResourceTiles(resourceCounts, activeResourceKind, setSelectedResourceKind)
    : [
        { label: 'Teams', value: stats.teams, icon: <ObjectIcon type="team" />, tone: 'blue' as const, onClick: () => onTabChange('overview') },
        { label: 'Applications', value: resourceCountLabel('application'), icon: <ObjectIcon type="application" />, tone: 'purple' as const, active: activeResourceKind === 'application', onClick: () => setSelectedResourceKind('application') },
        { label: 'LLM Profiles', value: resourceCountLabel('model'), icon: <ObjectIcon type="llm-profile" />, tone: 'purple' as const, active: activeResourceKind === 'model', onClick: () => setSelectedResourceKind('model') },
        { label: 'Agent Profiles', value: resourceCountLabel('agent_role'), icon: <ObjectIcon type="agent-profile" />, tone: 'blue' as const, active: activeResourceKind === 'agent_role', onClick: () => setSelectedResourceKind('agent_role') },
        { label: 'MCP Profiles', value: resourceCountLabel('mcp_profile'), icon: <ObjectIcon type="mcp-profile" />, tone: 'green' as const, active: activeResourceKind === 'mcp_profile', onClick: () => setSelectedResourceKind('mcp_profile') },
        ...(team ? [{ label: 'Notifications', value: typeof notificationCount === 'number' ? notificationCount : '-', icon: <Bell className="h-4 w-4" />, tone: 'cyan' as const, onClick: () => onTabChange('notifications') }] : []),
        { label: 'Pipelines', value: resourceCountLabel('pipeline'), icon: <ObjectIcon type="pipeline" />, tone: 'green' as const, active: activeResourceKind === 'pipeline', onClick: () => setSelectedResourceKind('pipeline') },
        { label: 'Steps', value: resourceCountLabel('step'), icon: <ObjectIcon type="step" />, tone: 'blue' as const, active: activeResourceKind === 'step', onClick: () => setSelectedResourceKind('step') },
        { label: 'Triggers', value: resourceCountLabel('trigger'), icon: <ObjectIcon type="trigger" />, tone: 'cyan' as const, active: activeResourceKind === 'trigger', onClick: () => setSelectedResourceKind('trigger') },
        { label: 'External Triggers', value: resourceCountLabel('external_trigger'), icon: <ObjectIcon type="external-trigger" />, tone: 'cyan' as const, active: activeResourceKind === 'external_trigger', onClick: () => setSelectedResourceKind('external_trigger') },
        { label: 'Git Webhook Sources', value: resourceCountLabel('git_webhook_source'), icon: <ObjectIcon type="git-webhook-source" />, tone: 'green' as const, active: activeResourceKind === 'git_webhook_source', onClick: () => setSelectedResourceKind('git_webhook_source') },
        { label: 'Schedules', value: resourceCountLabel('schedule'), icon: <ObjectIcon type="schedule" />, tone: 'purple' as const, active: activeResourceKind === 'schedule', onClick: () => setSelectedResourceKind('schedule') },
        { label: 'Knowledge Context', value: resourceCountLabel('knowledge_context'), icon: <ObjectIcon type="knowledge-context" />, tone: 'blue' as const, active: activeResourceKind === 'knowledge_context', onClick: () => setSelectedResourceKind('knowledge_context') },
        { label: 'Scopes', value: resourceCountLabel('scope'), icon: <ObjectIcon type="scope" />, tone: 'purple' as const, active: activeResourceKind === 'scope', onClick: () => setSelectedResourceKind('scope') },
        { label: 'Credentials', value: resourceCountLabel('credential'), icon: <ObjectIcon type="credential" />, tone: 'blue' as const, active: activeResourceKind === 'credential', onClick: () => setSelectedResourceKind('credential') },
      ];
  return (
    <article className="teams-card teams-resources-card">
      <div className="teams-card-heading">
        <div>
          <h3>Resources</h3>
          <p>{description}</p>
        </div>
        <button type="button" className="teams-secondary-btn" onClick={() => setAnalysisRequest({ resource: null })}>
          <BrainCircuit className="h-4 w-4" aria-hidden="true" />
          Analyse Resources
        </button>
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
        scopeLabel={app ? 'application' : teamPath ? 'team' : 'global'}
        onAnalyseResource={resource => setAnalysisRequest({ resource })}
      />
      {analysisRequest && analysisResult ? (
        <AnalysisModal
          result={analysisResult}
          loadAiPromptContext={loadAnalysisPromptContext}
          onClose={() => setAnalysisRequest(null)}
        />
      ) : null}
    </article>
  );
}

function TeamLinkedResourcesBrowser({
  resources,
  activeKind,
  loading,
  error,
  scopeLabel,
  onAnalyseResource,
}: {
  resources: TeamLinkedResource[];
  activeKind: TeamLinkedResourceKind;
  loading: boolean;
  error: string | null;
  scopeLabel: 'application' | 'team' | 'global';
  onAnalyseResource: (resource: TeamLinkedResource) => void;
}) {
  const activeResources = useMemo(
    () => resources.filter(resource => resource.kind === activeKind),
    [activeKind, resources]
  );
  const activeLabel = TEAM_LINKED_RESOURCE_LABELS[activeKind];
  const emptyLabel = scopeLabel === 'global'
    ? `No global ${activeLabel.toLowerCase()} found.`
    : `No ${activeLabel.toLowerCase()} found for this ${scopeLabel}.`;

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
              <TeamLinkedResourceRow key={resource.id} resource={resource} onAnalyseResource={onAnalyseResource} />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function TeamLinkedResourceRow({
  resource,
  onAnalyseResource,
}: {
  resource: TeamLinkedResource;
  onAnalyseResource: (resource: TeamLinkedResource) => void;
}) {
  return (
    <div className="teams-profile-summary__row teams-linked-resource-row">
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
        <Link className="runner-pill runner-pill--muted" to={resource.href} aria-label={`Open ${resource.label}`}>Open</Link>
        <button
          type="button"
          className="runner-pill runner-pill--muted"
          onClick={() => onAnalyseResource(resource)}
          aria-label={`Analyse ${resource.label}`}
        >
          Analyse
        </button>
      </div>
    </div>
  );
}

const TEAM_LINKED_RESOURCE_KINDS: TeamLinkedResourceKind[] = [
  'application',
  'model',
  'agent_role',
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

const AI_RESOURCE_KINDS = new Set<TeamLinkedResourceKind>(['model', 'agent_role', 'mcp_profile']);
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
      id: `model:${teamPath || 'global'}:${profile.name}`,
      kind: 'model',
      label: profile.name,
      description: [profile.provider, profile.model, profile.credential_ref].filter(Boolean).join(' / ') || 'LLM profile',
      href: `/models${aiResourceQuery(teamPath)}`,
      teamPath,
      source: profile.status || undefined,
    };
  });
}

function buildAgentProfileLinkedResources(profiles: NonNullable<TeamOperationsSummaryState['agentProfiles']>['profiles'] = []): TeamLinkedResource[] {
  return profiles.map(profile => {
    const teamPath = profile.team_path || '';
    return {
      id: `agent_role:${teamPath || 'global'}:${profile.id}`,
      kind: 'agent_role',
      label: profile.display_name || profile.id,
      description: [profile.id, profile.role, profile.description].filter(Boolean).join(' / ') || 'Agent profile',
      href: `/agent-roles${aiResourceQuery(teamPath)}`,
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
      href: `/mcp/profiles${aiResourceQuery(teamPath)}`,
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
    model: 0,
    agent_role: 0,
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
  return <ObjectIcon type={linkedResourceIconType(kind)} />;
}

function linkedResourceIconType(kind: TeamLinkedResourceKind): ObjectIconType {
  if (kind === 'application') return 'application';
  if (kind === 'model') return 'llm-profile';
  if (kind === 'agent_role') return 'agent-profile';
  if (kind === 'mcp_profile') return 'mcp-profile';
  if (kind === 'external_trigger') return 'external-trigger';
  if (kind === 'git_webhook_source') return 'git-webhook-source';
  if (kind === 'knowledge_context') return 'knowledge-context';
  return kind;
}

function linkedResourceTone(kind: TeamLinkedResourceKind): 'blue' | 'purple' | 'green' | 'cyan' {
  if (kind === 'pipeline' || kind === 'git_webhook_source' || kind === 'mcp_profile') return 'green';
  if (kind === 'trigger' || kind === 'external_trigger') return 'cyan';
  if (kind === 'schedule' || kind === 'scope' || kind === 'application' || kind === 'model') return 'purple';
  return 'blue';
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
