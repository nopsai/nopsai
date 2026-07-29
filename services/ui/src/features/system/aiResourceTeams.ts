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

export function decodeAIResourceRouteID(pathname: string, baseSegment: string): string {
  const segments = pathname.split('/').filter(Boolean);
  if (segments[0] !== baseSegment || segments.length < 2) return '';
  return normalizeAIResourceTeamPath(segments.slice(1).map(decodeAIResourceRouteSegment).join('/'));
}

export function encodeAIResourceRouteID(resourceID: string): string {
  return normalizeAIResourceTeamPath(resourceID)
    .split('/')
    .filter(Boolean)
    .map(encodeURIComponent)
    .join('/');
}

export function aiResourceRoute(basePath: string, resourceID: string, searchParams?: URLSearchParams): string {
  const params = new URLSearchParams(searchParams);
  const encodedResourceID = encodeAIResourceRouteID(resourceID);
  const normalizedBasePath = `/${basePath.replace(/^\/+|\/+$/g, '')}`;
  const route = encodedResourceID ? `${normalizedBasePath}/${encodedResourceID}` : normalizedBasePath;
  const query = params.toString();
  return query ? `${route}?${query}` : route;
}

export function aiResourceSearchParamsForTeamFilter(
  searchParams: URLSearchParams,
  teamFilter: string
): URLSearchParams {
  const params = new URLSearchParams(searchParams);
  const normalizedTeamFilter = normalizeAIResourceTeamPath(teamFilter);
  params.delete('team_path');

  if (!normalizedTeamFilter || normalizedTeamFilter === AI_RESOURCE_TEAM_FILTER_ALL) {
    params.delete('team');
  } else if (normalizedTeamFilter === AI_RESOURCE_TEAM_FILTER_GLOBAL) {
    params.set('team', 'global');
  } else {
    params.set('team', normalizedTeamFilter);
  }

  return params;
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

export function collectAIResourceTeamPaths(_resourceIDs: string[], knownTeamPaths: string[] = []) {
  const teams = new Set<string>();
  knownTeamPaths.map(normalizeAIResourceTeamPath).filter(Boolean).forEach(path => teams.add(path));
  return Array.from(teams).sort((a, b) => a.localeCompare(b));
}

export function selectableAIResourceTeamPath(value: string, knownTeamPaths: string[] = []) {
  const normalizedValue = normalizeAIResourceTeamPath(value);
  if (!normalizedValue) return '';
  const known = new Set(knownTeamPaths.map(normalizeAIResourceTeamPath).filter(Boolean));
  return known.has(normalizedValue) ? normalizedValue : '';
}

export function countAIResourceTeams(resourceIDs: string[]) {
  return new Set(resourceIDs.map(id => aiResourceTeamScope(id).teamPath).filter(Boolean)).size;
}

export function formatAIResourceTeamLabel(resourceID: string) {
  return aiResourceTeamScope(resourceID).displayTeam;
}

function decodeAIResourceRouteSegment(segment: string) {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}
