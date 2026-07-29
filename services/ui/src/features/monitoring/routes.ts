import type { MonitoringTab } from './model.js';

export const MONITORING_TABS: readonly MonitoringTab[] = [
  'overview',
  'runs',
  'pipelines',
  'steps-tasks',
  'triggers',
  'external-triggers',
  'runners',
  'ai-usage',
  'reliability',
  'efficiency',
  'security',
];

export function isMonitoringTab(value: string | null): value is MonitoringTab {
  return MONITORING_TABS.includes(value as MonitoringTab);
}

export function monitoringTabFromPath(pathname: string): MonitoringTab | null {
  const segments = pathname.split('/').filter(Boolean);
  if (segments[0] !== 'monitoring' || segments.length < 2) return null;
  const tab = decodeRouteSegment(segments[1]).trim();
  return isMonitoringTab(tab) ? tab : null;
}

export function monitoringTabFromSearch(search: string): MonitoringTab | null {
  const tab = new URLSearchParams(search).get('tab');
  return isMonitoringTab(tab) ? tab : null;
}

export function monitoringTabRoute(tab: MonitoringTab, searchParams?: URLSearchParams): string {
  const params = new URLSearchParams(searchParams);
  params.delete('tab');
  const query = params.toString();
  const route = `/monitoring/${encodeURIComponent(tab)}`;
  return query ? `${route}?${query}` : route;
}

function decodeRouteSegment(segment: string) {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}
