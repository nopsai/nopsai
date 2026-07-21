import { dashboardTabHref } from '../dashboards/routes.js';
import type { PipelineDefinition, PipelineOutputDashboardTarget, PipelineRunFinalOutput, RunFinalOutputStatus } from './contracts.js';

export type FinalOutputDashboardTarget = {
  ref: string;
  section: string;
  entryKey: string;
  mode: string;
  preset: string;
  ttl: string;
};

export type FinalOutputDashboardLink = FinalOutputDashboardTarget & {
  href: string;
  label: string;
};

export type RunFinalOutputStatusPresentation = {
  label: string;
  detail: string;
  title: string;
  className: string;
};

export function runFinalOutputStatusPresentation(status?: RunFinalOutputStatus | null): RunFinalOutputStatusPresentation | null {
  if (!status || (status.configured <= 0 && status.total <= 0)) return null;
  const generatedText = status.generated > 0 ? `${status.generated} generated` : '';
  const activeText = status.generating > 0
    ? `${status.generating} generating`
    : status.pending > 0
      ? `${status.pending} pending`
      : '';
  const failedText = status.failed > 0 ? `${status.failed} failed` : '';
  const cancelledText = status.cancelled > 0 ? `${status.cancelled} cancelled` : '';
  const countText = [generatedText, activeText, failedText, cancelledText].filter(Boolean).join(', ');
  const configuredText = `${status.configured || status.total} configured`;
  const totalText = status.total > 0 ? countText || `${status.total} output${status.total === 1 ? '' : 's'}` : configuredText;

  switch (normalizeStatusKey(status.status)) {
    case 'success':
      return outputStatusPresentation('Output generated', totalText, 'runner-pill--ok');
    case 'generating':
      return outputStatusPresentation('Output generating', totalText, 'runner-pill--warning');
    case 'pending':
      return outputStatusPresentation('Output pending', totalText, 'runner-pill--warning');
    case 'waiting':
      return outputStatusPresentation('Output waiting', configuredText, 'runner-pill--warning');
    case 'failure':
      return outputStatusPresentation('Output failed', totalText, 'runner-pill--error');
    case 'partial_failure':
      return outputStatusPresentation('Output partial', totalText, 'runner-pill--error');
    case 'cancelled':
      return outputStatusPresentation('Output cancelled', totalText, 'runner-pill--error');
    case 'partial_cancelled':
      return outputStatusPresentation('Output partial', totalText, 'runner-pill--warning');
    case 'not_generated':
      return outputStatusPresentation('Output not generated', configuredText, 'runner-pill--muted');
    default:
      return outputStatusPresentation('Output status', totalText, 'runner-pill--muted');
  }
}

export function finalOutputDashboardTarget(
  output: PipelineRunFinalOutput,
  pipelineDefinition?: PipelineDefinition | null
): FinalOutputDashboardTarget | null {
  if (normalizeOutputType(output.type) !== 'dashboard') return null;
  const stored = normalizeDashboardTarget(output.dashboard_target, output.name);
  if (stored) return stored;
  const item = (pipelineDefinition?.output?.items || []).find(candidate =>
    normalizeOutputType(candidate.type) === 'dashboard' &&
    candidate.name.trim() === output.name.trim()
  );
  return normalizeDashboardTarget(item?.dashboard, output.name);
}

export function finalOutputDashboardLink(
  output: PipelineRunFinalOutput,
  pipelineDefinition?: PipelineDefinition | null
): FinalOutputDashboardLink | null {
  const target = finalOutputDashboardTarget(output, pipelineDefinition);
  if (!target?.ref) return null;
  const href = dashboardTabHref(new URLSearchParams(), target.ref, target.section);
  const label = target.section ? `${target.ref} / ${target.section}` : target.ref;
  return { ...target, href, label };
}

function normalizeDashboardTarget(
  target: PipelineOutputDashboardTarget | undefined,
  outputName: string
): FinalOutputDashboardTarget | null {
  if (!target) return null;
  const normalized: FinalOutputDashboardTarget = {
    ref: trimSlashes(target.ref),
    section: trimValue(target.section),
    entryKey: trimValue(target.entry_key) || trimValue(outputName),
    mode: trimValue(target.mode),
    preset: trimValue(target.preset),
    ttl: trimValue(target.ttl),
  };
  if (!normalized.ref && !normalized.section && !normalized.entryKey && !normalized.mode && !normalized.preset && !normalized.ttl) {
    return null;
  }
  return normalized;
}

function normalizeOutputType(value: string): string {
  return trimValue(value).toLowerCase();
}

function outputStatusPresentation(label: string, detail: string, className: string): RunFinalOutputStatusPresentation {
  return { label, detail, className, title: detail ? `${label}: ${detail}` : label };
}

function normalizeStatusKey(value?: string): string {
  return trimValue(value).toLowerCase().replace(/\s+/g, '_');
}

function trimSlashes(value?: string): string {
  return trimValue(value).replace(/^\/+|\/+$/g, '');
}

function trimValue(value?: string): string {
  return String(value || '').trim();
}
