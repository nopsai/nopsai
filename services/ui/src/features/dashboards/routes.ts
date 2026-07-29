export const DASHBOARD_ROUTE_DASHBOARD_PARAM = 'dashboard';
export const DASHBOARD_ROUTE_TAB_PARAM = 'tab';

export type DashboardRouteSummary = {
  id?: string;
  team_path?: string;
  ref?: string;
  slug?: string;
};

export type DashboardRouteSection = {
  section_key?: string;
};

export function normalizeDashboardRouteValue(value?: string | null): string {
  return (value || '').trim();
}

export function dashboardRouteIDFromPath(pathname: string): string {
  const segments = pathname.split('/').filter(Boolean);
  if (segments[0] !== 'dashboards' || segments.length < 2) return '';
  return normalizeDashboardRouteValue(segments.slice(1).map(decodeDashboardRouteSegment).join('/'));
}

export function encodeDashboardRouteID(dashboardID: string): string {
  return normalizeDashboardRouteValue(dashboardID)
    .split('/')
    .filter(Boolean)
    .map(encodeURIComponent)
    .join('/');
}

export function dashboardRouteSelectedID(
  dashboards: DashboardRouteSummary[],
  routeDashboardID: string,
  currentID: string
): string {
  const routeMatch = findDashboardRouteMatch(dashboards, routeDashboardID);
  if (routeMatch) return normalizeDashboardRouteValue(routeMatch.id) || routeDashboardID;
  const currentMatch = findDashboardRouteMatch(dashboards, currentID);
  if (currentMatch) return normalizeDashboardRouteValue(currentMatch.id) || currentID;
  return dashboards[0]?.id || '';
}

export function dashboardRouteParamForSelectedID(
  dashboards: DashboardRouteSummary[],
  selectedID: string,
  routeDashboardID: string
): string {
  const normalizedSelectedID = normalizeDashboardRouteValue(selectedID);
  const selectedMatch = findDashboardRouteMatch(dashboards, normalizedSelectedID);
  if (!selectedMatch) return normalizedSelectedID;

  const routeMatch = findDashboardRouteMatch(dashboards, routeDashboardID);
  if (
    routeMatch &&
    normalizeDashboardRouteValue(routeMatch.id) === normalizeDashboardRouteValue(selectedMatch.id)
  ) {
    return normalizeDashboardRouteValue(routeDashboardID);
  }

  return normalizeDashboardRouteValue(selectedMatch.id) || normalizedSelectedID;
}

export function resolveDashboardActiveSectionKey(
  sections: DashboardRouteSection[] | undefined,
  routeSectionKey: string
): string {
  const normalizedRouteSectionKey = normalizeDashboardRouteValue(routeSectionKey);
  const loadedSections = sections || [];
  if (normalizedRouteSectionKey && loadedSections.length === 0) return normalizedRouteSectionKey;
  if (normalizedRouteSectionKey && loadedSections.some(section => normalizeDashboardRouteValue(section.section_key) === normalizedRouteSectionKey)) {
    return normalizedRouteSectionKey;
  }
  return normalizeDashboardRouteValue(loadedSections[0]?.section_key);
}

export function dashboardTabSearchParams(
  currentParams: URLSearchParams,
  sectionKey: string
): URLSearchParams {
  const params = new URLSearchParams(currentParams);
  const normalizedSectionKey = normalizeDashboardRouteValue(sectionKey);
  params.delete(DASHBOARD_ROUTE_DASHBOARD_PARAM);

  if (normalizedSectionKey) params.set(DASHBOARD_ROUTE_TAB_PARAM, normalizedSectionKey);
  else params.delete(DASHBOARD_ROUTE_TAB_PARAM);

  return params;
}

export function dashboardTabHref(
  currentParams: URLSearchParams,
  dashboardID: string,
  sectionKey: string
): string {
  const params = dashboardTabSearchParams(currentParams, sectionKey);
  return dashboardRouteHref(dashboardID, params);
}

export function dashboardRouteHref(dashboardID: string, currentParams?: URLSearchParams): string {
  const params = new URLSearchParams(currentParams);
  params.delete(DASHBOARD_ROUTE_DASHBOARD_PARAM);
  const encodedDashboardID = encodeDashboardRouteID(dashboardID);
  const route = encodedDashboardID ? `/dashboards/${encodedDashboardID}` : '/dashboards';
  const query = params.toString();
  return query ? `${route}?${query}` : route;
}

function findDashboardRouteMatch(
  dashboards: DashboardRouteSummary[],
  routeValue: string
): DashboardRouteSummary | null {
  const normalizedRouteValue = normalizeDashboardRouteValue(routeValue);
  if (!normalizedRouteValue) return null;
  return dashboards.find(dashboard => dashboardRouteValues(dashboard).includes(normalizedRouteValue)) || null;
}

function dashboardRouteValues(dashboard: DashboardRouteSummary): string[] {
  const teamPath = normalizeDashboardRouteValue(dashboard.team_path);
  const slug = normalizeDashboardRouteValue(dashboard.slug);
  return Array.from(new Set([
    normalizeDashboardRouteValue(dashboard.id),
    normalizeDashboardRouteValue(dashboard.ref),
    teamPath ? '' : slug,
    teamPath && slug ? `${teamPath}/${slug}` : '',
  ].filter(Boolean)));
}

function decodeDashboardRouteSegment(segment: string) {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}
