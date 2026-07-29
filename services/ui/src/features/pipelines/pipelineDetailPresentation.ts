import type { AnalysisFinding, AnalysisResult, AnalysisSeverity } from '../analysis/model.js';
import type { PipelineRun, PipelineTrigger } from './api.js';
import { formatPipelineGitRef, normalizePipelineSource, pipelineRunStatusLabel, type PipelineDetail, type PipelineGraphData } from './model.js';

export type PipelineDetailMetric = {
  id: 'steps' | 'tasks' | 'triggers' | 'runs' | 'health';
  label: string;
  value: string;
  detail: string;
  tone: 'neutral' | 'success' | 'warning' | 'danger';
};

export type PipelineDetailSourceState = {
  label: string;
  tone: 'success' | 'warning' | 'neutral';
  description: string;
};

export type PipelineDetailHealthSummary = {
  score: number;
  label: string;
  tone: 'success' | 'warning' | 'danger';
  findingLabel: string;
};

export type PipelineDetailRunSummary = {
  statusLabel: string;
  branchLabel: string;
  runLabel: string;
};

const SEVERITY_WEIGHT: Record<AnalysisSeverity, number> = {
  critical: 5,
  high: 4,
  medium: 3,
  low: 2,
  opportunity: 1,
};

export function formatPipelineDetailSource(source?: string): PipelineDetailSourceState {
  const normalized = normalizePipelineSource(source);
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
      description: 'Local draft, save before execution',
    };
  }
  if (normalized === 'database') {
    return {
      label: 'Database',
      tone: 'neutral',
      description: 'Stored as database definition or override',
    };
  }
  return {
    label: normalized,
    tone: 'neutral',
    description: 'Stored pipeline source',
  };
}

export function countPipelineGraphTasks(graphData: PipelineGraphData): number {
  return graphData.steps.reduce((total, step) => {
    if (step.tasks.length) return total + step.tasks.length;
    if (step.configuration?.script || step.configuration?.goal || step.configuration?.include || step.configuration?.approval) {
      return total + 1;
    }
    return total;
  }, 0);
}

export function buildPipelineDetailMetrics({
  graphData,
  triggers,
  recentRuns,
  analysis,
  validationErrorCount,
}: {
  graphData: PipelineGraphData;
  triggers: PipelineTrigger[];
  recentRuns: PipelineRun[];
  analysis: AnalysisResult | null;
  validationErrorCount: number;
}): PipelineDetailMetric[] {
  const blockingFindings = analysis
    ? analysis.findings.filter(finding => finding.severity === 'critical' || finding.severity === 'high').length
    : 0;
  return [
    {
      id: 'steps',
      label: 'Steps',
      value: String(graphData.steps.length),
      detail: graphData.error ? 'Graph issue' : 'Executable graph nodes',
      tone: graphData.error ? 'danger' : 'neutral',
    },
    {
      id: 'tasks',
      label: 'Tasks',
      value: String(countPipelineGraphTasks(graphData)),
      detail: 'Step and task work units',
      tone: 'neutral',
    },
    {
      id: 'triggers',
      label: 'Trigger Rules',
      value: String(triggers.length),
      detail: triggers.length === 1 ? 'Automation binding' : 'Automation bindings',
      tone: triggers.length ? 'success' : 'warning',
    },
    {
      id: 'runs',
      label: 'Recent Runs',
      value: String(recentRuns.length),
      detail: 'Loaded run history',
      tone: recentRuns.length ? 'success' : 'neutral',
    },
    {
      id: 'health',
      label: 'Health',
      value: analysis ? `${analysis.healthScore}` : '—',
      detail: validationErrorCount
        ? `${validationErrorCount} validation issue${validationErrorCount === 1 ? '' : 's'}`
        : blockingFindings
          ? `${blockingFindings} blocking finding${blockingFindings === 1 ? '' : 's'}`
          : 'Current snapshot',
      tone: validationErrorCount || blockingFindings
        ? 'danger'
        : analysis && analysis.healthScore >= 85
          ? 'success'
          : 'warning',
    },
  ];
}

export function buildPipelineDetailHealthSummary(analysis: AnalysisResult | null): PipelineDetailHealthSummary {
  if (!analysis) {
    return {
      score: 0,
      label: 'Not analysed',
      tone: 'warning',
      findingLabel: 'Open analysis to generate findings',
    };
  }
  const blocking = analysis.findings.filter(finding => finding.severity === 'critical' || finding.severity === 'high').length;
  const topFinding = highestPriorityFinding(analysis.findings);
  if (blocking > 0) {
    return {
      score: analysis.healthScore,
      label: 'Needs attention',
      tone: 'danger',
      findingLabel: `${blocking} high-priority finding${blocking === 1 ? '' : 's'}`,
    };
  }
  if (analysis.healthScore >= 90) {
    return {
      score: analysis.healthScore,
      label: 'Healthy',
      tone: 'success',
      findingLabel: topFinding ? topFinding.title : 'No weighted findings',
    };
  }
  return {
    score: analysis.healthScore,
    label: 'Review recommended',
    tone: 'warning',
    findingLabel: topFinding ? topFinding.title : 'Review lower-priority recommendations',
  };
}

export function summarizePipelineLatestRun(runs: PipelineRun[]): PipelineDetailRunSummary {
  const latest = runs[0];
  if (!latest) {
    return {
      statusLabel: 'No runs',
      branchLabel: '—',
      runLabel: '—',
    };
  }
  const shortRunID = latest.run_id ? latest.run_id.slice(0, 8) : '—';
  return {
    statusLabel: pipelineRunStatusLabel(latest.status),
    branchLabel: formatPipelineGitRef(latest.git_ref),
    runLabel: shortRunID,
  };
}

export function highestPriorityFinding(findings: AnalysisFinding[]): AnalysisFinding | null {
  return [...findings].sort((left, right) => {
    const bySeverity = SEVERITY_WEIGHT[right.severity] - SEVERITY_WEIGHT[left.severity];
    if (bySeverity !== 0) return bySeverity;
    return right.confidence - left.confidence;
  })[0] || null;
}

export function formatPipelineDetailPath(detail: PipelineDetail): string {
  return detail.path?.trim() || 'Root';
}
