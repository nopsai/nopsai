import {
  ArrowUpRight,
  Bell,
  BookOpen,
  Boxes,
  ChevronRight,
  GitBranch,
  Pencil,
  Settings,
  ShieldCheck,
  Trash2,
  UsersRound,
} from 'lucide-react';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import {
  formatConfigRepoTimestamp,
  isAppTeam,
  teamDisplayName,
  teamPathForURL,
  teamRepositoryLabel,
  teamRepositoryURL,
  type Team,
} from '../../lib/teamModels';
import { buildPipelineRunsRoute } from '../../lib/teamRoutes';
import type { CurrentUser } from '../../app/types';
import { accessPresetIDForRole } from '../system/access/presentation';
import { formatTeamTimestamp, getLatestRunApplication, getTeamDirectChildren, getTeamParent, teamKindLabel } from './model';
import { teamNotificationGitOpsTarget, type NotificationRouteRecord } from './notificationRoutes';
import type { TeamOperationsSummaryState } from './hooks/useTeamOperationsSummary';
import type { TeamDetailTabID } from './workspaceModel';

export function TeamChildrenTable({
  title,
  items,
  teams,
  emptySelection,
  activeLabel,
  emptyTitle,
  emptyMessage,
  showBackToRoot,
  onBackToRoot,
  onSelectTeam,
  onDeleteTeam,
  onEditTeam,
  onOpenConfig,
}: {
  title: string;
  items: Team[];
  teams: Team[];
  emptySelection: boolean;
  activeLabel: string;
  emptyTitle?: string;
  emptyMessage?: string;
  showBackToRoot?: boolean;
  onBackToRoot: () => void;
  onSelectTeam: (id: number | null) => void;
  onDeleteTeam: (team: Team) => void;
  onEditTeam: (team: Team) => void;
  onOpenConfig: (team: Team) => void;
}) {
  return (
    <article className="teams-card teams-table-card">
      <div className="teams-table-heading">
        <h3>{title}</h3>
        <span>{items.length} items</span>
      </div>
      {items.length === 0 ? (
        <div className="teams-empty-detail">
          <BookOpen className="h-5 w-5" aria-hidden="true" />
          <div>
            <h2>{emptyTitle || (emptySelection ? 'No child items' : 'No matching resources')}</h2>
            <p>{emptyMessage || (emptySelection ? `${activeLabel} has no child teams or applications.` : 'Adjust search or create a new team item.')}</p>
          </div>
          {showBackToRoot ?? emptySelection ? (
            <button type="button" className="teams-secondary-btn" onClick={onBackToRoot}>
              Back to global
            </button>
          ) : null}
        </div>
      ) : (
        <div className="teams-table-scroll">
          <table className="teams-table">
            <thead>
              <tr>
                <th>Resource</th>
                <th>Type</th>
                <th>Repository / GitOps</th>
                <th>Last Run</th>
                <th>Children</th>
                <th aria-label="Actions"></th>
              </tr>
            </thead>
            <tbody>
              {items.map(item => (
                <TeamTableRow
                  key={item.id}
                  team={item}
                  teams={teams}
                  onSelect={() => onSelectTeam(item.id)}
                  onDelete={() => onDeleteTeam(item)}
                  onEdit={() => onEditTeam(item)}
                  onOpenConfig={() => onOpenConfig(item)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </article>
  );
}

export function TeamTabPanel({
  activeTab,
  team,
  teams,
  stats,
  operationsSummary,
  currentUser,
  onOpenConfig,
}: {
  activeTab: Exclude<TeamDetailTabID, 'overview'>;
  team: Team | null;
  teams: Team[];
  stats: { totalItems: number };
  operationsSummary: TeamOperationsSummaryState;
  currentUser?: CurrentUser | null;
  onOpenConfig: (team: Team, tab?: 'sync' | 'notifications') => void;
}) {
  if (activeTab === 'gitops') {
    return (
      <section className="teams-tab-panel" role="tabpanel" id="teams-tabpanel-gitops" aria-labelledby="teams-tab-gitops">
        <TeamGitOpsCard team={team} teams={teams} stats={stats} summary={operationsSummary} onOpenConfig={onOpenConfig} />
      </section>
    );
  }

  if (activeTab === 'notifications') {
    return (
      <section className="teams-tab-panel" role="tabpanel" id="teams-tabpanel-notifications" aria-labelledby="teams-tab-notifications">
        <TeamNotificationsCard team={team} teams={teams} summary={operationsSummary} onOpenConfig={onOpenConfig} />
      </section>
    );
  }

  return (
    <section className="teams-tab-panel" role="tabpanel" id="teams-tabpanel-access" aria-labelledby="teams-tab-access">
      <TeamAccessSummaryCard team={team} teams={teams} summary={operationsSummary} currentUser={currentUser} />
    </section>
  );
}

export function TeamOverviewCard({
  team,
  teams,
  stats,
  operationsSummary,
}: {
  team: Team | null;
  teams: Team[];
  stats: { applications: number; directChildren: number };
  operationsSummary: TeamOperationsSummaryState;
}) {
  const parent = getTeamParent(team, teams);
  const currentTeamPath = team ? teamPathForURL(team, teams) : '';
  const profileQuery = team ? `?team=${encodeURIComponent(currentTeamPath)}` : '?team=global';
  const latestRunApplication = getLatestRunApplication(teams, team?.id ?? null);
  const ownerLabels = teamOwnerLabels(operationsSummary);
  const ownerSummary = operationsSummary.loading
    ? 'Loading...'
    : operationsSummary.accessGrantsError
      ? 'Unable to load owners'
      : ownerLabels.length
        ? ownerLabels.slice(0, 3).join(', ') + (ownerLabels.length > 3 ? ` +${ownerLabels.length - 3}` : '')
        : 'No direct owners';
  const latestRunLabel = latestRunApplication
    ? `${teamDisplayName(latestRunApplication)} / ${formatTeamTimestamp(latestRunApplication.last_run_at)}`
    : 'No app runs';
  const llmDefaultProfile = operationsSummary.loading ? 'Loading...' : operationsSummary.llmProfiles?.default_profile || '';
  const agentDefaultProfile = operationsSummary.loading ? 'Loading...' : operationsSummary.agentProfiles?.default_profile || '';
  const rows: TeamOverviewRow[] = [
    { label: 'Kind', value: teamKindLabel(team) },
    { label: 'Path', value: currentTeamPath || 'global' },
    { label: 'Parent', value: parent ? teamDisplayName(parent) : 'Global' },
    {
      label: 'Default LLM profile',
      value: profileDefaultLink(llmDefaultProfile, `/llm-profiles${profileQuery}`),
      title: llmDefaultProfile || 'Not configured',
    },
    {
      label: 'Default agent profile',
      value: profileDefaultLink(agentDefaultProfile, `/agent-profiles${profileQuery}`),
      title: agentDefaultProfile || 'Not configured',
    },
    { label: 'Direct children', value: String(stats.directChildren) },
    { label: 'Applications', value: String(stats.applications) },
    { label: 'Owners', value: ownerSummary },
    { label: 'Latest run app', value: latestRunLabel },
  ];
  return (
    <article className="teams-card teams-overview-card">
      <div className="teams-card-heading">
        <div>
          <h3>{team ? 'Team Overview' : 'Organization Overview'}</h3>
          <p>{team?.description || 'Ownership, hierarchy, and latest application run context for this scope.'}</p>
        </div>
      </div>
      <dl className="teams-kv-list">
        {rows.map(({ label, value, title }) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd title={title || (typeof value === 'string' ? value : undefined)}>{value}</dd>
          </div>
        ))}
      </dl>
    </article>
  );
}

type TeamOverviewRow = {
  label: string;
  value: ReactNode;
  title?: string;
};

function profileDefaultLink(defaultProfile: string, href: string) {
  const label = defaultProfile || 'Not configured';
  return (
    <Link className="teams-inline-link" to={href}>
      {label}
    </Link>
  );
}

function teamOwnerLabels(summary: TeamOperationsSummaryState) {
  const ownerLabels = summary.accessGrants
    .filter(grant => accessPresetIDForRole(grant.role) === 'owner')
    .map(grant => grant.subjectDisplay || grant.subjectID || grant.subjectType)
    .filter(Boolean);
  return Array.from(new Set(ownerLabels)).sort((left, right) => left.localeCompare(right, undefined, { sensitivity: 'base' }));
}

export function TeamApplicationOverviewCard({
  team,
  teams,
}: {
  team: Team;
  teams: Team[];
}) {
  const parent = getTeamParent(team, teams);
  const applicationPath = teamPathForURL(team, teams);
  const repositoryURL = teamRepositoryURL(team);
  const repositoryLabel = teamRepositoryLabel(team);
  const applicationDescription =
    team.description ||
    `${teamDisplayName(team)} application details: owner team, repository, app path, and latest run context.`;
  const rows = [
    ['Type', 'Application'],
    ['Application name', teamDisplayName(team)],
    ['Repository', repositoryLabel || '-'],
    ['Owner team', parent ? teamDisplayName(parent) : 'Global'],
    ['Owner path', team.team_path || (parent ? teamPathForURL(parent, teams) : 'global')],
    ['Application path', applicationPath],
    ['Last run', formatTeamTimestamp(team.last_run_at)],
  ];

  return (
    <article className="teams-card teams-focus-card teams-focus-card--wide">
      <div className="teams-focus-hero teams-focus-hero--plain">
        <div className="teams-focus-copy teams-focus-copy--standalone">
          <p>{applicationDescription}</p>
        </div>
        <div className="teams-focus-actions">
          <Link className="teams-secondary-btn" to={buildPipelineRunsRoute('main', applicationPath)}>
            <GitBranch className="h-4 w-4" aria-hidden="true" />
            Related Runs
          </Link>
          {repositoryURL ? (
            <a className="teams-secondary-btn" href={repositoryURL} target="_blank" rel="noreferrer">
              <ArrowUpRight className="h-4 w-4" aria-hidden="true" />
              Open Repository
            </a>
          ) : null}
        </div>
      </div>
      <dl className="teams-kv-list teams-kv-list--compact">
        {rows.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd title={value}>{value}</dd>
          </div>
        ))}
        <div>
          <dt>Repository URL</dt>
          <dd title={team.repo_url || repositoryURL || ''}>
            {repositoryURL ? (
              <a href={repositoryURL} target="_blank" rel="noreferrer" className="teams-inline-link">
                {team.repo_url || repositoryURL}
                <ArrowUpRight className="h-3.5 w-3.5" aria-hidden="true" />
              </a>
            ) : (
              'Not connected'
            )}
          </dd>
        </div>
      </dl>
    </article>
  );
}

function TeamGitOpsCard({
  team,
  teams,
  stats,
  summary,
  onOpenConfig,
}: {
  team: Team | null;
  teams: Team[];
  stats: { totalItems: number };
  summary: TeamOperationsSummaryState;
  onOpenConfig: (team: Team, tab?: 'sync' | 'notifications') => void;
}) {
  const isRoot = !team;
  const app = team ? isAppTeam(team) : false;
  const canConfigure = Boolean(team && !app);
  const label = team ? teamDisplayName(team) : 'Global';
  const teamPath = team ? teamPathForURL(team, teams) : 'system/global';
  const repo = summary.configRepo;
  const teamDescription = canConfigure
    ? 'Configuration repository, sync, drift, and write-back controls.'
    : 'Select a regular team to manage GitOps configuration.';
  const description = isRoot
    ? 'Global uses the system config repository for platform-wide GitOps settings.'
    : teamDescription;
  const teamRepositoryStatus = repo?.repo_url ? 'Connected' : 'Not connected';
  const repositoryStatus = isRoot ? 'Global repository' : teamRepositoryStatus;
  const repositoryTitle = isRoot ? 'Managed in System Config' : repo?.repo_url || '';
  const branchStatus = isRoot ? 'Managed in System' : repo?.branch || '-';
  const syncStatus = isRoot ? 'Open system config' : repo?.last_sync_status || 'Not synced';
  const teamWriteBackStatus = repo?.write_enabled ? 'Enabled' : 'Disabled';
  const writeBackStatus = isRoot ? 'System owned' : teamWriteBackStatus;

  return (
    <article className="teams-card teams-focus-card teams-focus-card--wide">
      <div className="teams-focus-hero">
        <span className="teams-resource-icon teams-tone-green" aria-hidden="true">
          <GitBranch className="h-5 w-5" />
        </span>
        <div>
          <h3>{label} GitOps</h3>
          <p>{description}</p>
        </div>
        <div className="teams-focus-actions">
          {isRoot ? (
            <Link className="teams-secondary-btn" to="/system/config">
              <ArrowUpRight className="h-4 w-4" aria-hidden="true" />
              Open Global Config
            </Link>
          ) : canConfigure ? (
            <button type="button" className="teams-secondary-btn" onClick={() => team && onOpenConfig(team, 'sync')}>
              <Settings className="h-4 w-4" aria-hidden="true" />
              Configure
            </button>
          ) : null}
        </div>
      </div>
      {!isRoot && summary.loading ? <div className="teams-inline-status">Loading GitOps configuration...</div> : null}
      {!isRoot && summary.configRepoError ? <div className="teams-inline-status teams-inline-status--error">{summary.configRepoError}</div> : null}
      <div className="teams-focus-grid">
        <div className="teams-focus-metric">
          <span>Repository</span>
          <strong title={repositoryTitle}>{repositoryStatus}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Branch</span>
          <strong>{branchStatus}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Status</span>
          <strong>{syncStatus}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Write-back</span>
          <strong>{writeBackStatus}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Scope</span>
          <strong>{teamPath}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Resources</span>
          <strong>{stats.totalItems}</strong>
        </div>
      </div>
      {isRoot ? (
        <div className="teams-inline-status">
          Global config repository details are managed in{' '}
          <Link className="teams-inline-link" to="/system/config">
            System Config
            <ArrowUpRight className="h-3.5 w-3.5" aria-hidden="true" />
          </Link>
          .
        </div>
      ) : (
        <dl className="teams-kv-list teams-kv-list--compact">
          <div>
            <dt>Repository URL</dt>
            <dd title={repo?.repo_url || ''}>{repo?.repo_url || '-'}</dd>
          </div>
          <div>
            <dt>Base path</dt>
            <dd>{repo?.base_path || '-'}</dd>
          </div>
          <div>
            <dt>Push branch</dt>
            <dd>{repo?.write_branch || '-'}</dd>
          </div>
          <div>
            <dt>Last sync</dt>
            <dd>{formatConfigRepoTimestamp(repo?.last_sync_completed_at)}</dd>
          </div>
          <div>
            <dt>Last commit</dt>
            <dd title={repo?.last_sync_commit_sha || ''}>{repo?.last_sync_commit_sha || '-'}</dd>
          </div>
        </dl>
      )}
    </article>
  );
}

function TeamNotificationsCard({
  team,
  teams,
  summary,
  onOpenConfig,
}: {
  team: Team | null;
  teams: Team[];
  summary: TeamOperationsSummaryState;
  onOpenConfig: (team: Team, tab?: 'sync' | 'notifications') => void;
}) {
  const app = team ? isAppTeam(team) : false;
  const canConfigure = Boolean(team && !app);
  const label = team ? teamDisplayName(team) : 'Global';
  const teamPath = team ? teamPathForURL(team, teams) : 'global';
  const route = summary.notificationRoute;
  const routes = notificationRoutes(route);
  const repo = summary.configRepo;
  const gitOpsTarget = repo ? teamNotificationGitOpsTarget(repo.base_path) : '';
  const source = route?.managed_by_config_repo ? 'GitOps' : route?.id ? 'Database' : 'Default';

  return (
    <article className="teams-card teams-focus-card teams-focus-card--wide">
      <div className="teams-focus-hero">
        <span className="teams-resource-icon teams-tone-cyan" aria-hidden="true">
          <Bell className="h-5 w-5" />
        </span>
        <div>
          <h3>{label} Notifications</h3>
          <p>{canConfigure ? 'Pipeline notification routes and GitOps managed policies.' : 'Notification policy editing is available for regular teams.'}</p>
        </div>
        <div className="teams-focus-actions">
          {canConfigure ? (
            <button type="button" className="teams-secondary-btn" onClick={() => team && onOpenConfig(team, 'notifications')}>
              <Settings className="h-4 w-4" aria-hidden="true" />
              Configure
            </button>
          ) : null}
        </div>
      </div>
      {summary.loading ? <div className="teams-inline-status">Loading notification policy...</div> : null}
      {summary.notificationError ? <div className="teams-inline-status teams-inline-status--error">{summary.notificationError}</div> : null}
      <div className="teams-focus-grid">
        <div className="teams-focus-metric">
          <span>Routes</span>
          <strong>{routes.length}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Enabled</span>
          <strong>{routes.filter(item => item.enabled).length}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Source</span>
          <strong>{source}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Scope</span>
          <strong>{teamPath}</strong>
        </div>
      </div>
      {gitOpsTarget ? <div className="teams-inline-status">GitOps target: <span>{gitOpsTarget}</span></div> : null}
      <div className="teams-summary-list" aria-label="Notification routes">
        {routes.length === 0 ? (
          <div className="teams-summary-list__empty">No notification routes configured.</div>
        ) : (
          routes.map(routeItem => (
            <div key={routeItem.name} className="teams-summary-list__row">
              <div className="min-w-0">
                <strong>{routeItem.name}</strong>
                <p>{notificationRouteRecipients(routeItem)}</p>
              </div>
              <span className={`runner-pill ${routeItem.enabled ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
                {routeItem.enabled ? 'Enabled' : 'Disabled'}
              </span>
              <span className="runner-pill runner-pill--muted">{notificationRouteEventCount(routeItem)} events</span>
            </div>
          ))
        )}
      </div>
    </article>
  );
}

function currentUserAccessRoles(currentUser?: CurrentUser | null) {
  const roles = (currentUser?.roles || [])
    .map(role => role.trim())
    .filter(Boolean);
  return Array.from(new Set(roles)).sort((left, right) => left.localeCompare(right, undefined, { sensitivity: 'base' }));
}

function currentUserAccessLabel(currentUser?: CurrentUser | null) {
  return currentUser?.displayName?.trim() || currentUser?.email?.trim() || currentUser?.sub?.trim() || 'Current user';
}

function currentUserScopedGrantRoles(currentUser: CurrentUser | null | undefined, grants: TeamOperationsSummaryState['accessGrants']) {
  const identities = [currentUser?.sub, currentUser?.email, currentUser?.displayName]
    .map(value => value?.trim().toLowerCase())
    .filter(Boolean) as string[];
  if (identities.length === 0) return [];
  const identitySet = new Set(identities);
  const roles = grants
    .filter(grant => {
      if (grant.subjectType.trim().toLowerCase() !== 'user') return false;
      return [grant.subjectID, grant.subjectDisplay]
        .map(value => (value || '').trim().toLowerCase())
        .some(value => identitySet.has(value));
    })
    .map(grant => `${grant.role}${grant.inherit ? ' + child scopes' : ''}`);
  return Array.from(new Set(roles)).sort((left, right) => left.localeCompare(right, undefined, { sensitivity: 'base' }));
}

function TeamAccessSummaryCard({
  team,
  teams,
  summary,
  currentUser,
}: {
  team: Team | null;
  teams: Team[];
  summary: TeamOperationsSummaryState;
  currentUser?: CurrentUser | null;
}) {
  const isRoot = !team;
  const label = team ? teamDisplayName(team) : 'Global';
  const teamPath = team ? teamPathForURL(team, teams) : 'global';
  const accessURL = isRoot
    ? '/system/access?resource_type=platform&resource_id=platform'
    : `/system/access?resource_type=team&resource_id=${encodeURIComponent(teamPath)}`;
  const accessDescription = isRoot
    ? 'Platform-wide administrator grants and current-user effective checks.'
    : 'Direct team grants, current-user effective checks, and child-scope coverage.';
  const grantsLabel = isRoot ? 'Global admins' : 'Direct grants';
  const scopeLabel = isRoot ? 'Platform-wide' : 'Team + children';
  const grantsHeading = isRoot ? 'Global Admins' : 'Users & Roles';
  const emptyGrantsMessage = isRoot ? 'No platform-wide admin grants visible.' : 'No direct grants visible for this team.';
  const accessRoles = currentUserAccessRoles(currentUser);
  const scopedGrantRoles = currentUserScopedGrantRoles(currentUser, summary.accessGrants);
  const allowedChecks = summary.permissions.filter(item => item.allowed).length;
  const effectiveChecksLabel = summary.permissions.length > 0 ? `${allowedChecks}/${summary.permissions.length}` : '-';
  const currentUserLabel = currentUserAccessLabel(currentUser);

  return (
    <article className="teams-card teams-focus-card teams-focus-card--wide">
      <div className="teams-focus-hero">
        <span className="teams-resource-icon teams-tone-blue" aria-hidden="true">
          <ShieldCheck className="h-5 w-5" />
        </span>
        <div>
          <h3>{label} Access</h3>
          <p>{accessDescription}</p>
        </div>
        <div className="teams-focus-actions">
          <Link className="teams-secondary-btn" to={accessURL}>
            <ArrowUpRight className="h-4 w-4" aria-hidden="true" />
            Open Access
          </Link>
        </div>
      </div>
      {summary.loading ? <div className="teams-inline-status">Loading access grants...</div> : null}
      {summary.accessGrantsError ? <div className="teams-inline-status teams-inline-status--error">{summary.accessGrantsError}</div> : null}
      {summary.permissionsError ? <div className="teams-inline-status teams-inline-status--error">{summary.permissionsError}</div> : null}
      <div className="teams-focus-grid">
        <div className="teams-focus-metric">
          <span>{grantsLabel}</span>
          <strong>{summary.accessGrants.length}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Access roles</span>
          <strong title={accessRoles.join(', ') || 'No access roles assigned'}>{accessRoles.length || 'None'}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Effective checks</span>
          <strong>{effectiveChecksLabel}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Scope</span>
          <strong>{scopeLabel}</strong>
        </div>
      </div>
      <div className="teams-access-grid">
        <section className="teams-summary-list" aria-label="Current user access">
          <div className="teams-table-heading">
            <h3>Current User Access</h3>
            <span>{accessRoles.length ? `${accessRoles.length} roles` : 'No roles'}</span>
          </div>
          <div className="teams-summary-list__row teams-summary-list__row--stacked">
            <div className="min-w-0">
              <strong>{currentUserLabel}</strong>
              <p>Current session</p>
            </div>
            <div className="teams-access-role-list" aria-label="Current access roles">
              {accessRoles.length ? (
                accessRoles.map(role => (
                  <span key={role} className="runner-pill runner-pill--muted">{role}</span>
                ))
              ) : (
                <span className="runner-pill runner-pill--muted">No access roles</span>
              )}
            </div>
          </div>
          <div className="teams-summary-list__row teams-summary-list__row--stacked">
            <div className="min-w-0">
              <strong>Scoped Basic Roles</strong>
              <p>{isRoot ? 'Platform scope' : 'Team scope'}</p>
            </div>
            <div className="teams-access-role-list" aria-label="Current scoped basic roles">
              {scopedGrantRoles.length ? (
                scopedGrantRoles.map(role => (
                  <span key={role} className="runner-pill runner-pill--ok">{role}</span>
                ))
              ) : (
                <span className="runner-pill runner-pill--muted">No direct scoped basic roles</span>
              )}
            </div>
          </div>
          <div className="teams-table-heading teams-table-heading--subtle">
            <h3>Effective Checks</h3>
            <span>{summary.permissions.length ? `${allowedChecks} allowed` : 'No checks'}</span>
          </div>
          {summary.permissions.length === 0 ? (
            <div className="teams-summary-list__empty">No effective checks available for this scope.</div>
          ) : (
            summary.permissions.map(permission => (
              <div key={permission.action} className="teams-summary-list__row">
                <div className="min-w-0">
                  <strong>{permission.label}</strong>
                  <p>{permission.action}</p>
                </div>
                <span className={`runner-pill ${permission.allowed ? 'runner-pill--ok' : 'runner-pill--muted'}`}>
                  {permission.allowed ? 'Allowed' : 'Not allowed'}
                </span>
              </div>
            ))
          )}
        </section>
        <section className="teams-summary-list" aria-label="Access grants">
          <div className="teams-table-heading">
            <h3>{grantsHeading}</h3>
            <span>{summary.accessGrants.length} grants</span>
          </div>
          {summary.accessGrants.length === 0 ? (
            <div className="teams-summary-list__empty">{emptyGrantsMessage}</div>
          ) : (
            summary.accessGrants.map(grant => (
              <div key={grant.id} className="teams-summary-list__row">
                <div className="min-w-0">
                  <strong>{accessSubjectLabel(grant)}</strong>
                  <p>{grant.subjectType}{grant.inherit ? ' / inherits to child resources' : ''}</p>
                </div>
                <span className="runner-pill runner-pill--ok">{grant.role}</span>
                {grant.managedByConfigRepo ? <span className="runner-pill runner-pill--muted">GitOps</span> : null}
              </div>
            ))
          )}
        </section>
      </div>
    </article>
  );
}

function notificationRoutes(route: NotificationRouteRecord | null) {
  return route?.definition.routes?.length ? route.definition.routes : [];
}

function notificationRouteRecipients(route: ReturnType<typeof notificationRoutes>[number]) {
  const include = route.recipients.include;
  const users = include?.users?.filter(Boolean) ?? [];
  const teams = include?.teams?.filter(Boolean) ?? [];
  const sameTeam = teams.includes('same_team') ? ['same team'] : [];
  return [...sameTeam, ...users, ...teams.filter(team => team !== 'same_team')].join(', ') || 'No recipients';
}

function notificationRouteEventCount(route: ReturnType<typeof notificationRoutes>[number]) {
  return Object.values(route.events).filter(Boolean).length;
}

function accessSubjectLabel(grant: TeamOperationsSummaryState['accessGrants'][number]) {
  return grant.subjectDisplay || grant.subjectID || grant.subjectType || 'Subject';
}

function TeamTableRow({
  team,
  teams,
  onSelect,
  onDelete,
  onEdit,
  onOpenConfig,
}: {
  team: Team;
  teams: Team[];
  onSelect: () => void;
  onDelete: () => void;
  onEdit: () => void;
  onOpenConfig: () => void;
}) {
  const app = isAppTeam(team);
  const repoURL = teamRepositoryURL(team);
  const children = getTeamDirectChildren(teams, team.id);
  const label = teamDisplayName(team);
  return (
    <tr>
      <td>
        <button type="button" className="teams-table-resource" onClick={onSelect}>
          <span className={`teams-table-resource__icon ${app ? 'teams-tone-purple' : 'teams-tone-blue'}`} aria-hidden="true">
            {app ? <Boxes className="h-4 w-4" /> : <UsersRound className="h-4 w-4" />}
          </span>
          <span>
            <strong>{label}</strong>
            {team.description ? <small>{team.description}</small> : null}
          </span>
        </button>
      </td>
      <td>
        <span className="runner-pill runner-pill--muted">{teamKindLabel(team)}</span>
      </td>
      <td>
        {repoURL ? (
          <a href={repoURL} target="_blank" rel="noreferrer" className="teams-inline-link">
            {teamRepositoryLabel(team)}
            <ArrowUpRight className="h-3.5 w-3.5" aria-hidden="true" />
          </a>
        ) : app ? (
          'No repository'
        ) : (
          'Team settings'
        )}
      </td>
      <td>{formatTeamTimestamp(team.last_run_at)}</td>
      <td>{children.length}</td>
      <td>
        <div className="teams-table-actions">
          {!team.navigation_only ? (
            <button type="button" className="teams-icon-btn" title={`Edit ${label}`} aria-label={`Edit ${label}`} onClick={onEdit}>
              <Pencil className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
          {!app ? (
            <button type="button" className="teams-icon-btn" title={`GitOps and notifications for ${label}`} aria-label={`GitOps and notifications for ${label}`} onClick={onOpenConfig}>
              <Settings className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
          {!team.navigation_only ? (
            <button type="button" className="teams-icon-btn teams-icon-btn--danger" title={`Delete ${label}`} aria-label={`Delete ${label}`} onClick={onDelete}>
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
          <button type="button" className="teams-icon-btn" title={`Open ${label}`} aria-label={`Open ${label}`} onClick={onSelect}>
            <ChevronRight className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </td>
    </tr>
  );
}
