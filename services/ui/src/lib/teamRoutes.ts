export const TEAM_ROUTE_SEGMENT = 'team';

export function normalizeTeamRoutePath(value?: string | null) {
  return (value || '').trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
}

export function encodeTeamRoutePath(value?: string | null) {
  const normalized = normalizeTeamRoutePath(value);
  if (!normalized) return '';
  return normalized.split('/').filter(Boolean).map(encodeURIComponent).join('/');
}

export function decodeTeamRouteSegments(segments: string[]) {
  return normalizeTeamRoutePath(
    segments
      .filter(Boolean)
      .map(segment => {
        try {
          return decodeURIComponent(segment);
        } catch {
          return segment;
        }
      })
      .join('/')
  );
}

export function splitRoutePath(pathname: string) {
  return pathname.split('/').filter(Boolean);
}

export function teamScopedRoute(basePath: string, teamPath?: string | null) {
  const base = basePath.replace(/\/+$/g, '') || '/';
  const encodedTeamPath = encodeTeamRoutePath(teamPath);
  if (!encodedTeamPath) return base;
  return `${base}/${TEAM_ROUTE_SEGMENT}/${encodedTeamPath}`;
}

export function extractTeamPathFromRoute(pathname: string, rootSegment: string) {
  const segments = splitRoutePath(pathname);
  if (segments[0] !== rootSegment) return '';
  const teamIndex = segments.indexOf(TEAM_ROUTE_SEGMENT, 1);
  if (teamIndex < 0) return '';
  return decodeTeamRouteSegments(segments.slice(teamIndex + 1));
}

export function hasTeamRouteSegment(pathname: string, rootSegment: string) {
  const segments = splitRoutePath(pathname);
  return segments[0] === rootSegment && segments.indexOf(TEAM_ROUTE_SEGMENT, 1) >= 0;
}

export function buildPipelineRunsRoute(tab: string, teamPath?: string | null) {
  const normalizedTab = tab === 'recent' || tab === 'events' ? tab : 'main';
  return teamScopedRoute(`/pipelineruns/${normalizedTab}`, teamPath);
}

