import { GLOBAL_RESOURCE_TEAM_LABEL, isGlobalResourceTeamPath } from '../../lib/resourceTeams.js';
import type { StepUsageItem } from './api.js';
import { normalizeSource, splitIdentifier, type StepDetail } from './model.js';

export type StepDetailSourceState = {
  label: string;
  tone: 'success' | 'warning' | 'neutral';
  description: string;
};

export type StepUsageSourceSummary = {
  total: number;
  gitOps: number;
  database: number;
  drafts: number;
};

export function formatStepDetailSource(source?: string): StepDetailSourceState {
  const normalized = normalizeSource(source);
  if (normalized === 'git') {
    return {
      label: 'GitOps',
      tone: 'success',
      description: 'Synced from configuration repository',
    };
  }
  if (normalized === 'draft') {
    return {
      label: 'Draft',
      tone: 'warning',
      description: 'Local draft, save before use',
    };
  }
  return {
    label: 'Database',
    tone: 'neutral',
    description: 'Stored as database definition or override',
  };
}

export function formatStepDetailPath(detail: Pick<StepDetail, 'path'>): string {
  const path = detail.path?.trim() || '';
  return !path || isGlobalResourceTeamPath(path) ? GLOBAL_RESOURCE_TEAM_LABEL : path;
}

export function formatStepUsageTeam(item: StepUsageItem): string {
  const explicitPath = item.path?.trim() || '';
  const path = explicitPath || splitIdentifier(item.identifier).path;
  return !path || isGlobalResourceTeamPath(path) ? GLOBAL_RESOURCE_TEAM_LABEL : path;
}

export function summarizeStepUsageSources(usage: StepUsageItem[]): StepUsageSourceSummary {
  return usage.reduce<StepUsageSourceSummary>(
    (summary, item) => {
      const source = normalizeSource(item.source);
      summary.total += 1;
      if (source === 'git') summary.gitOps += 1;
      else if (source === 'draft') summary.drafts += 1;
      else summary.database += 1;
      return summary;
    },
    { total: 0, gitOps: 0, database: 0, drafts: 0 }
  );
}
