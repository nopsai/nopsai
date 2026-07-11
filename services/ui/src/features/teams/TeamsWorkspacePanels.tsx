import {
  ArrowUpRight,
  Bell,
  BookOpen,
  Bot,
  Boxes,
  ChevronRight,
  GitBranch,
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
import { formatTeamTimestamp, getTeamDirectChildren, teamKindLabel } from './model';
import type { TeamDetailTabID } from './workspaceModel';

export function TeamActivityCard({
  team,
  stats,
  directChildren,
}: {
  team: Team | null;
  stats: { teams: number; applications: number; repositories: number; recentRuns: number };
  directChildren: Team[];
}) {
  const rows = [
    { label: 'Scoped teams', value: stats.teams, tone: 'blue' as const },
    { label: 'Applications', value: stats.applications, tone: 'purple' as const },
    { label: 'Repositories', value: stats.repositories, tone: 'green' as const },
    { label: 'Recent run signals', value: stats.recentRuns, tone: 'cyan' as const },
  ];
  return (
    <article className="teams-card teams-activity-card">
      <div className="teams-card-heading">
        <div>
          <h3>{team ? 'Team Activity' : 'Organization Activity'}</h3>
          <p>{directChildren.length} direct child resources in the current scope.</p>
        </div>
        <select aria-label="Activity range">
          <option>Last 30 days</option>
        </select>
      </div>
      <div className="teams-activity-list">
        {rows.map(row => (
          <div key={row.label} className="teams-activity-row">
            <span className={`teams-activity-dot teams-tone-${row.tone}`} aria-hidden="true">
              <ShieldCheck className="h-3.5 w-3.5" />
            </span>
            <span>{row.label}</span>
            <strong>{row.value}</strong>
          </div>
        ))}
      </div>
    </article>
  );
}

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
              Back to root
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
  scopedApplications,
  onOpenConfig,
}: {
  activeTab: Exclude<TeamDetailTabID, 'overview'>;
  team: Team | null;
  teams: Team[];
  stats: { applications: number; repositories: number; recentRuns: number; teams: number; totalItems: number };
  scopedApplications: Team[];
  onOpenConfig: (team: Team) => void;
}) {
  if (activeTab === 'applications') {
    return (
      <section className="teams-tab-panel" role="tabpanel" id="teams-tabpanel-applications" aria-labelledby="teams-tab-applications">
        <div className="teams-detail-grid">
          <TeamApplicationsCard applications={scopedApplications} teams={teams} stats={stats} />
          <TeamActivityCard team={team} stats={stats} directChildren={scopedApplications} />
        </div>
      </section>
    );
  }

  return (
    <section className="teams-tab-panel" role="tabpanel" id={`teams-tabpanel-${activeTab}`} aria-labelledby={`teams-tab-${activeTab}`}>
      <TeamOperationsCard activeTab={activeTab} team={team} teams={teams} stats={stats} onOpenConfig={onOpenConfig} />
    </section>
  );
}

function TeamApplicationsCard({
  applications,
  teams,
  stats,
}: {
  applications: Team[];
  teams: Team[];
  stats: { applications: number; repositories: number; recentRuns: number };
}) {
  const latest = applications.find(application => application.last_run_at);
  return (
    <article className="teams-card teams-focus-card">
      <div className="teams-card-heading">
        <div>
          <h3>Application Scope</h3>
          <p>{applications.length} applications with {stats.repositories} connected repositories.</p>
        </div>
        <span className="teams-resource-icon teams-tone-purple" aria-hidden="true">
          <Boxes className="h-5 w-5" />
        </span>
      </div>
      <dl className="teams-kv-list">
        <div>
          <dt>Applications</dt>
          <dd>{stats.applications}</dd>
        </div>
        <div>
          <dt>Repositories</dt>
          <dd>{stats.repositories}</dd>
        </div>
        <div>
          <dt>Recent runs</dt>
          <dd>{stats.recentRuns}</dd>
        </div>
        <div>
          <dt>Latest signal</dt>
          <dd>{latest ? `${teamDisplayName(latest)} · ${formatTeamTimestamp(latest.last_run_at)}` : 'Never'}</dd>
        </div>
        <div>
          <dt>Primary path</dt>
          <dd>{applications[0] ? teamPathForURL(applications[0], teams) : '-'}</dd>
        </div>
      </dl>
    </article>
  );
}

function TeamOperationsCard({
  activeTab,
  team,
  teams,
  stats,
  onOpenConfig,
}: {
  activeTab: Exclude<TeamDetailTabID, 'overview' | 'applications'>;
  team: Team | null;
  teams: Team[];
  stats: { applications: number; repositories: number; teams: number; totalItems: number };
  onOpenConfig: (team: Team) => void;
}) {
  const app = team ? isAppTeam(team) : false;
  const canConfigure = Boolean(team && !app && !team.navigation_only);
  const label = team ? teamDisplayName(team) : 'Root';
  const teamPath = team ? teamPathForURL(team, teams) : 'root';
  const settingsCopy = getOperationsCopy(activeTab, label, teamPath, canConfigure);

  return (
    <article className="teams-card teams-focus-card teams-focus-card--wide">
      <div className="teams-focus-hero">
        <span className={`teams-resource-icon teams-tone-${settingsCopy.tone}`} aria-hidden="true">
          {settingsCopy.icon}
        </span>
        <div>
          <h3>{settingsCopy.title}</h3>
          <p>{settingsCopy.description}</p>
        </div>
        <div className="teams-focus-actions">
          {canConfigure && activeTab !== 'access' ? (
            <button type="button" className="teams-secondary-btn" onClick={() => team && onOpenConfig(team)}>
              <Settings className="h-4 w-4" aria-hidden="true" />
              Configure
            </button>
          ) : null}
          {activeTab === 'access' ? (
            <a className="teams-secondary-btn" href="/system/access">
              <ArrowUpRight className="h-4 w-4" aria-hidden="true" />
              Open Access
            </a>
          ) : null}
        </div>
      </div>
      <div className="teams-focus-grid">
        {settingsCopy.metrics.map(metric => (
          <div key={metric.label} className="teams-focus-metric">
            <span>{metric.label}</span>
            <strong>{metric.value}</strong>
          </div>
        ))}
        <div className="teams-focus-metric">
          <span>Scope</span>
          <strong>{teamPath}</strong>
        </div>
        <div className="teams-focus-metric">
          <span>Resources</span>
          <strong>{stats.totalItems}</strong>
        </div>
      </div>
    </article>
  );
}

function getOperationsCopy(
  activeTab: Exclude<TeamDetailTabID, 'overview' | 'applications'>,
  label: string,
  teamPath: string,
  canConfigure: boolean
) {
  if (activeTab === 'gitops') {
    return {
      title: `${label} GitOps`,
      description: canConfigure ? 'Configuration repository, sync, drift, and write-back controls.' : 'Select a regular team to manage GitOps configuration.',
      tone: 'green' as const,
      icon: <GitBranch className="h-5 w-5" />,
      metrics: [
        { label: 'Repository', value: canConfigure ? 'Configurable' : 'Inherited' },
        { label: 'Drift', value: canConfigure ? 'Available' : '-' },
      ],
    };
  }
  if (activeTab === 'ai') {
    return {
      title: `${label} AI Profiles`,
      description: canConfigure ? 'Team LLM, agent, and MCP profile configuration.' : 'Team AI profile editing is available for regular teams.',
      tone: 'purple' as const,
      icon: <Bot className="h-5 w-5" />,
      metrics: [
        { label: 'LLM profiles', value: 'Team' },
        { label: 'MCP profiles', value: 'Tools' },
      ],
    };
  }
  if (activeTab === 'notifications') {
    return {
      title: `${label} Notifications`,
      description: canConfigure ? 'Pipeline notification routes and GitOps managed policies.' : 'Notification policy editing is available for regular teams.',
      tone: 'cyan' as const,
      icon: <Bell className="h-5 w-5" />,
      metrics: [
        { label: 'Delivery', value: 'Mail' },
        { label: 'GitOps target', value: teamPath === 'root' ? '-' : 'Routes' },
      ],
    };
  }
  return {
    title: `${label} Access`,
    description: 'AAA role assignments, policies, and team-scoped access catalog entries.',
    tone: 'blue' as const,
    icon: <ShieldCheck className="h-5 w-5" />,
    metrics: [
      { label: 'Resource type', value: teamPath === 'root' ? 'Platform' : 'Team' },
      { label: 'Inheritance', value: teamPath === 'root' ? 'Root' : 'Team path' },
    ],
  };
}

function TeamTableRow({
  team,
  teams,
  onSelect,
  onDelete,
  onOpenConfig,
}: {
  team: Team;
  teams: Team[];
  onSelect: () => void;
  onDelete: () => void;
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
          {!app && !team.navigation_only ? (
            <button type="button" className="teams-icon-btn" title={`GitOps and AI for ${label}`} aria-label={`GitOps and AI for ${label}`} onClick={onOpenConfig}>
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
