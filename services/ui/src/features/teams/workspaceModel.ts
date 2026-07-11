import { isAppTeam, type Team } from '../../lib/teamModels.js';

export type TeamDetailTabID = 'overview' | 'applications' | 'gitops' | 'ai' | 'notifications' | 'access';

export const teamDetailTabs: Array<{ id: TeamDetailTabID; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'applications', label: 'Applications' },
  { id: 'gitops', label: 'GitOps' },
  { id: 'ai', label: 'AI Profiles' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'access', label: 'Access' },
];

export function getTeamTableItems({
  activeDetailTab,
  directChildren,
  scopedApplications,
  searching,
  visibleItems,
}: {
  activeDetailTab: TeamDetailTabID;
  directChildren: Team[];
  scopedApplications: Team[];
  searching: boolean;
  visibleItems: Team[];
}) {
  if (activeDetailTab === 'applications') {
    return searching ? visibleItems.filter(isAppTeam) : scopedApplications;
  }
  if (searching) return visibleItems;
  return directChildren;
}

export function getTeamTableCopy({
  activeDetailTab,
  activeLabel,
  searching,
}: {
  activeDetailTab: TeamDetailTabID;
  activeLabel: string;
  searching: boolean;
}) {
  if (activeDetailTab === 'applications') {
    return {
      title: searching ? 'Matching Applications' : 'Scoped Applications',
      emptyTitle: searching ? 'No matching applications' : 'No scoped applications',
      emptyMessage: searching ? 'Adjust search or create a new application.' : `${activeLabel} has no visible applications in this scope.`,
      showBackToRoot: false,
    };
  }
  return {
    title: searching ? 'Matching Resources' : activeLabel === 'Root' ? 'Root Resources' : 'Child Resources',
    emptyTitle: undefined,
    emptyMessage: undefined,
    showBackToRoot: undefined,
  };
}
