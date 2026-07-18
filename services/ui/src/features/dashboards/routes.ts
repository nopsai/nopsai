export const DASHBOARD_ROUTE_DASHBOARD_PARAM = 'dashboard';
export const DASHBOARD_ROUTE_TAB_PARAM = 'tab';

export function normalizeDashboardRouteValue(value?: string | null): string {
  return (value || '').trim();
}

export function dashboardTabSearchParams(
  currentParams: URLSearchParams,
  dashboardID: string,
  sectionKey: string
): URLSearchParams {
  const params = new URLSearchParams(currentParams);
  const normalizedDashboardID = normalizeDashboardRouteValue(dashboardID);
  const normalizedSectionKey = normalizeDashboardRouteValue(sectionKey);

  if (normalizedDashboardID) params.set(DASHBOARD_ROUTE_DASHBOARD_PARAM, normalizedDashboardID);
  else params.delete(DASHBOARD_ROUTE_DASHBOARD_PARAM);

  if (normalizedSectionKey) params.set(DASHBOARD_ROUTE_TAB_PARAM, normalizedSectionKey);
  else params.delete(DASHBOARD_ROUTE_TAB_PARAM);

  return params;
}

export function dashboardTabHref(
  currentParams: URLSearchParams,
  dashboardID: string,
  sectionKey: string
): string {
  const params = dashboardTabSearchParams(currentParams, dashboardID, sectionKey);
  const query = params.toString();
  return query ? `/dashboards?${query}` : '/dashboards';
}
