export const AI_RESOURCE_TEAM_FILTER_ALL = '__all__';
export const AI_RESOURCE_TEAM_FILTER_GLOBAL = '__global__';

export type AIResourceTeamScope = {
  teamPath: string;
  localName: string;
  displayTeam: string;
};

export function normalizeAIResourceTeamPath(value: string) {
  return value.trim().replace(/^\/+|\/+$/g, '');
}

export function aiResourceTeamFilterFromSearch(search: string) {
  const params = new URLSearchParams(search);
  const team = normalizeAIResourceTeamPath(params.get('team') || params.get('team_path') || '');
  if (!team) return AI_RESOURCE_TEAM_FILTER_ALL;
  if (team === AI_RESOURCE_TEAM_FILTER_GLOBAL || team.toLowerCase() === 'global') {
    return AI_RESOURCE_TEAM_FILTER_GLOBAL;
  }
  return team;
}

export function aiResourceTeamScope(resourceID: string): AIResourceTeamScope {
  const normalized = normalizeAIResourceTeamPath(resourceID);
  const parts = normalized.split('/').map(part => part.trim()).filter(Boolean);
  if (parts.length <= 1) {
    return {
      teamPath: '',
      localName: normalized,
      displayTeam: 'Global',
    };
  }
  const localName = parts[parts.length - 1] || normalized;
  const teamPath = parts.slice(0, -1).join('/');
  return {
    teamPath,
    localName,
    displayTeam: `/${teamPath}`,
  };
}

export function aiResourceLocalName(resourceID: string) {
  if (resourceID.trim().endsWith('/')) return '';
  return aiResourceTeamScope(resourceID).localName;
}

export function buildAIResourceScopedID(teamPath: string, localName: string) {
  const normalizedTeam = normalizeAIResourceTeamPath(teamPath);
  const normalizedName = normalizeAIResourceTeamPath(localName);
  if (!normalizedTeam) return normalizedName;
  if (!normalizedName) return normalizedTeam ? `${normalizedTeam}/` : '';
  return `${normalizedTeam}/${normalizedName}`;
}

export function aiResourceMatchesTeamFilter(resourceID: string, teamFilter: string) {
  if (!teamFilter || teamFilter === AI_RESOURCE_TEAM_FILTER_ALL) return true;
  const teamPath = aiResourceTeamScope(resourceID).teamPath;
  if (teamFilter === AI_RESOURCE_TEAM_FILTER_GLOBAL) return !teamPath;
  const normalizedFilter = normalizeAIResourceTeamPath(teamFilter);
  return teamPath === normalizedFilter || teamPath.startsWith(`${normalizedFilter}/`);
}

export function collectAIResourceTeamPaths(resourceIDs: string[], knownTeamPaths: string[] = []) {
  const teams = new Set<string>();
  knownTeamPaths.map(normalizeAIResourceTeamPath).filter(Boolean).forEach(path => teams.add(path));
  resourceIDs
    .map(id => aiResourceTeamScope(id).teamPath)
    .filter(Boolean)
    .forEach(path => teams.add(path));
  return Array.from(teams).sort((a, b) => a.localeCompare(b));
}

export function countAIResourceTeams(resourceIDs: string[]) {
  return new Set(resourceIDs.map(id => aiResourceTeamScope(id).teamPath).filter(Boolean)).size;
}

export function formatAIResourceTeamLabel(resourceID: string) {
  return aiResourceTeamScope(resourceID).displayTeam;
}
