import type { Team } from '../../lib/teamModels.js';

export type TeamDetailTabID = 'overview' | 'gitops' | 'notifications' | 'access';

export const teamDetailTabs: Array<{ id: TeamDetailTabID; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'gitops', label: 'GitOps' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'access', label: 'Access' },
];

export function visibleTeamDetailTabs(team: Team | null): Array<{ id: TeamDetailTabID; label: string }> {
  if (team) return teamDetailTabs;
  return teamDetailTabs.filter(tab => tab.id !== 'notifications');
}

export function getTeamTableItems({
  directChildren,
  searching,
  visibleItems,
}: {
  directChildren: Team[];
  searching: boolean;
  visibleItems: Team[];
}) {
  if (searching) return visibleItems;
  return directChildren;
}

export function getTeamTableCopy({
  activeLabel,
  searching,
}: {
  activeLabel: string;
  searching: boolean;
}) {
  return {
    title: searching ? 'Matching Resources' : activeLabel === 'Global' ? 'Global Resources' : 'Child Resources',
    emptyTitle: undefined,
    emptyMessage: undefined,
    showBackToRoot: undefined,
  };
}
