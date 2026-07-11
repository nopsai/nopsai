import {
  isAppTeam,
  teamDisplayName,
  teamPathForURL,
  teamRepositoryURL,
  type Team,
} from '../../lib/teamModels.js';

export type TeamTreeNode = {
  team: Team;
  children: TeamTreeNode[];
  depth: number;
  path: string;
};

export type TeamScopeStats = {
  teams: number;
  applications: number;
  repositories: number;
  recentRuns: number;
  directChildren: number;
  totalItems: number;
};

export function compareTeamItems(left: Team, right: Team): number {
  const leftApp = isAppTeam(left);
  const rightApp = isAppTeam(right);
  if (leftApp !== rightApp) return leftApp ? 1 : -1;
  return teamDisplayName(left).localeCompare(teamDisplayName(right), undefined, { sensitivity: 'base' });
}

export function buildTeamTree(teams: Team[]): TeamTreeNode[] {
  const byParent = new Map<number | null, Team[]>();
  teams.forEach(team => {
    const parentID = team.parent_id ?? null;
    byParent.set(parentID, [...(byParent.get(parentID) || []), team]);
  });

  const build = (parentID: number | null, depth: number, seen: Set<number>): TeamTreeNode[] => {
    return [...(byParent.get(parentID) || [])]
      .sort(compareTeamItems)
      .filter(team => !seen.has(team.id))
      .map(team => {
        const nextSeen = new Set(seen);
        nextSeen.add(team.id);
        return {
          team,
          depth,
          path: teamPathForURL(team, teams),
          children: build(team.id, depth + 1, nextSeen),
        };
      });
  };

  return build(null, 0, new Set());
}

export function getTeamDirectChildren(teams: Team[], activeTeamID: number | null): Team[] {
  return teams
    .filter(team => (team.parent_id ?? null) === activeTeamID)
    .sort(compareTeamItems);
}

export function getVisibleTeamItems(teams: Team[], activeTeamID: number | null, searchTerm: string): Team[] {
  const term = searchTerm.trim().toLowerCase();
  const candidates = term ? teams : getTeamDirectChildren(teams, activeTeamID);
  if (!term) return candidates;
  return candidates
    .filter(team => {
      const searchable = [
        teamDisplayName(team),
        team.name,
        team.path,
        team.team_path,
        team.description,
        team.repository_full_name,
        team.repo_url,
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase();
      return searchable.includes(term);
    })
    .sort(compareTeamItems);
}

export function getTeamSubtree(teams: Team[], activeTeamID: number | null): Team[] {
  if (activeTeamID == null) return [...teams].sort(compareTeamItems);
  const byParent = new Map<number | null, Team[]>();
  teams.forEach(team => {
    const parentID = team.parent_id ?? null;
    byParent.set(parentID, [...(byParent.get(parentID) || []), team]);
  });

  const selected = teams.find(team => team.id === activeTeamID);
  const result: Team[] = selected ? [selected] : [];
  const seen = new Set<number>(selected ? [selected.id] : []);
  const visit = (parentID: number) => {
    (byParent.get(parentID) || []).forEach(child => {
      if (seen.has(child.id)) return;
      seen.add(child.id);
      result.push(child);
      visit(child.id);
    });
  };
  if (selected) visit(selected.id);
  return result.sort(compareTeamItems);
}

export function buildTeamScopeStats(teams: Team[], activeTeamID: number | null): TeamScopeStats {
  const scoped = getTeamSubtree(teams, activeTeamID);
  const directChildren = getTeamDirectChildren(teams, activeTeamID);
  return {
    teams: scoped.filter(team => !isAppTeam(team)).length,
    applications: scoped.filter(isAppTeam).length,
    repositories: scoped.filter(team => Boolean(teamRepositoryURL(team))).length,
    recentRuns: scoped.filter(team => Boolean(team.last_run_at)).length,
    directChildren: directChildren.length,
    totalItems: scoped.length,
  };
}

export function getTeamParent(team: Team | null | undefined, teams: Team[]): Team | null {
  if (!team?.parent_id) return null;
  return teams.find(item => item.id === team.parent_id) || null;
}

export function teamKindLabel(team: Team | null | undefined): string {
  if (!team) return 'Organization';
  return isAppTeam(team) ? 'Application' : 'Team';
}

export function formatTeamTimestamp(value?: string): string {
  if (!value) return 'Never';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}
