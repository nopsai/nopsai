import type { AssistantPageContext } from '../assistant/pageContext.js';
import {
  analysisCategoryLabel,
  formatAnalysisReport,
  type AnalysisCategory,
  type AnalysisEvidence,
  type AnalysisResult,
  type AnalysisSeverity,
} from './model.js';

export type AnalysisAiPromptSnapshot = {
  subject: {
    type: AnalysisResult['subjectType'];
    id: string;
    label: string;
    scopePath?: string;
    snapshotRevision: string;
  };
  score: {
    health: number;
    basis: string;
    inputs: string[];
    deductions: number;
    counts: AnalysisResult['counts'];
    categoryScores: Array<{
      category: string;
      score: number;
      basis: string;
    }>;
  };
  primaryDiagnosis?: AnalysisResult['primaryDiagnosis'];
  comparison?: AnalysisResult['comparison'];
  findings: Array<{
    severity: string;
    category: string;
    title: string;
    summary: string;
    confidence: number;
    evidence: AnalysisEvidence[];
    recommendations: string[];
  }>;
  additionalEvidence?: AnalysisAiPromptContext;
};

export type AnalysisAiPromptContext = {
  sections: AnalysisAiPromptContextSection[];
};

export type AnalysisAiPromptContextSection = {
  title: string;
  summary?: string;
  items?: AnalysisEvidence[];
  lines?: string[];
  lineRetention?: 'head' | 'tail';
  limitations?: string[];
};

export type AnalysisAiEvaluationProblem = {
  title: string;
  detail: string;
};

export type AnalysisAiEvaluationScore = {
  health: number | null;
  detail: string;
  drivers: string[];
  findings: AnalysisAiScoredFinding[];
  categoryScores: AnalysisAiCategoryScore[];
};

export type AnalysisAiScoredFinding = {
  title: string;
  severity: AnalysisSeverity;
  category: AnalysisCategory;
  basis: string;
  deduction: number | null;
  confidence: number;
};

export type AnalysisAiCategoryScore = {
  category: AnalysisCategory;
  score: number;
  basis: string;
};

export type AnalysisAiEvaluationFix = {
  title: string;
  detail: string;
  priority: 'now' | 'next' | 'later' | string;
  safeAction: string;
};

export type StructuredAnalysisAiEvaluation = {
  summary: string;
  problem: AnalysisAiEvaluationProblem;
  score: AnalysisAiEvaluationScore;
  fixes: AnalysisAiEvaluationFix[];
  evidenceNeeded: string[];
  confidence: number;
  structured: boolean;
};

export function buildAnalysisAiPrompt(result: AnalysisResult, context?: AnalysisAiPromptContext | null): string {
  const snapshot = buildAnalysisAiPromptSnapshot(result, context);
  const instructions = analysisSubjectPromptInstructions(result);

  return [
    'You are the NOPSAI enterprise operations analysis reviewer.',
    'Use the redacted snapshot below as evidence. You may reason from it, but do not invent logs, secret values, or hidden resources.',
    'Be precise and professional: name the concrete failing step, task, command, error line, or missing evidence. Do not fill gaps with generic advice.',
    'Do not propose mutations, reruns, permission changes, or YAML edits as already executed. Suggestions must be safe, reviewable, and GitOps-compatible.',
    instructions,
    'Return ONLY valid JSON. Do not use Markdown, code fences, prose before JSON, or raw log excerpts.',
    'Do not list evidence_needed for evidence already present in the snapshot.',
    'Use this exact JSON shape:',
    '{"summary":"one-sentence executive result","problem":{"title":"specific problem or risk","detail":"why this is likely based on evidence"},"score":{"reviewed_health":75,"detail":"why the reviewed score changed from the deterministic starting point","drivers":["short scored driver"],"findings":[{"title":"scored finding","severity":"critical|high|medium|low|opportunity","category":"security|reliability|organization|efficiency|monitoring|maintainability|bug|cost","basis":"specific evidence used for this scoring decision","deduction":25,"confidence":90}],"category_scores":[{"category":"security","score":75,"basis":"why this metric changed"}]},"fixes":[{"title":"fix title","detail":"specific safe suggestion","priority":"now|next|later","safe_action":"read-only or reviewable action"}],"evidence_needed":["specific missing evidence"],"confidence":80}',
    'Keep each string concise. Prefer three fixes or fewer unless the evidence requires more.',
    'For score.findings, include only concrete scored findings supported by the provided snapshot; do not include generic best-practice reminders.',
    '',
    'Deterministic reviewer report:',
    formatAnalysisReport(result),
    '',
    'Redacted structured snapshot:',
    JSON.stringify(snapshot, null, 2),
  ].join('\n');
}

export function buildAnalysisAiPromptSnapshot(
  result: AnalysisResult,
  context?: AnalysisAiPromptContext | null
): AnalysisAiPromptSnapshot {
  return {
    subject: {
      type: result.subjectType,
      id: result.subjectId,
      label: result.subjectLabel,
      scopePath: result.scopePath || undefined,
      snapshotRevision: result.snapshotRevision,
    },
    score: {
      health: result.healthScore,
      basis: result.scoreBasis.formula,
      inputs: result.scoreBasis.inputs,
      deductions: result.scoreBasis.totalDeduction,
      counts: result.counts,
      categoryScores: result.scores.map(score => ({
        category: score.label,
        score: score.score,
        basis: score.basis,
      })),
    },
    primaryDiagnosis: result.primaryDiagnosis,
    comparison: result.comparison,
    findings: result.findings.slice(0, 12).map(finding => ({
      severity: finding.severity,
      category: analysisCategoryLabel(finding.category),
      title: finding.title,
      summary: finding.summary,
      confidence: finding.confidence,
      evidence: finding.evidence.slice(0, 6).map(redactEvidenceForPrompt),
      recommendations: finding.recommendations.slice(0, 3).map(recommendation =>
        `${recommendation.title}: ${recommendation.detail}`
      ),
    })),
    additionalEvidence: normalizeAnalysisAiPromptContext(context),
  };
}

export function parseAnalysisAiEvaluation(content: string): StructuredAnalysisAiEvaluation {
  const parsed = parseJSONObject(content);
  if (!parsed) return fallbackStructuredEvaluation(content);
  const summary = readString(parsed.summary) || 'AI evaluation completed.';
  const problemRecord = asRecord(parsed.problem) || {};
  const scoreRecord = asRecord(parsed.score) || {};
  const scoredFindings = readScoredFindings(scoreRecord, parsed);
  const categoryScores = readCategoryScores(scoreRecord);
  const fixes = readArray(parsed.fixes)
    .map(item => {
      const record = asRecord(item) || {};
      return {
        title: readString(record.title) || 'Suggested fix',
        detail: readString(record.detail),
        priority: readString(record.priority) || 'next',
        safeAction: readString(record.safe_action) || readString(record.safeAction),
      };
    })
    .filter(fix => fix.detail || fix.safeAction)
    .slice(0, 6);

  return {
    summary: truncateEvaluationText(summary, 260),
    problem: {
      title: truncateEvaluationText(readString(problemRecord.title) || 'Problem', 120),
      detail: truncateEvaluationText(readString(problemRecord.detail) || summary, 700),
    },
    score: {
      health: readReviewedHealth(scoreRecord),
      detail: truncateEvaluationText(readString(scoreRecord.detail) || 'The deterministic score is based on weighted visible findings.', 700),
      drivers: readStringArray(scoreRecord.drivers).slice(0, 6).map(item => truncateEvaluationText(item, 180)),
      findings: scoredFindings,
      categoryScores,
    },
    fixes: fixes.map(fix => ({
      title: truncateEvaluationText(fix.title, 120),
      detail: truncateEvaluationText(fix.detail, 700),
      priority: truncateEvaluationText(fix.priority, 20),
      safeAction: truncateEvaluationText(fix.safeAction, 260),
    })),
    evidenceNeeded: readStringArray(parsed.evidence_needed || parsed.evidenceNeeded)
      .slice(0, 6)
      .map(item => truncateEvaluationText(item, 180)),
    confidence: clampConfidence(readNumber(parsed.confidence)),
    structured: true,
  };
}

/** The chat opener for an analysis, so moving from a finding to a conversation needs no retyping. */
export function analysisAssistantChatPrompt(result: AnalysisResult): string {
  const blocking = result.counts.critical + result.counts.high;
  const subject = `${result.subjectType} ${result.subjectLabel}`.trim();
  if (blocking > 0) {
    return `Walk me through the analysis of ${subject} (score ${result.healthScore}/100, ${blocking} critical or high findings) and tell me what to fix first.`;
  }
  if (result.findings.length > 0) {
    return `Walk me through the analysis of ${subject} (score ${result.healthScore}/100) and tell me which finding is worth acting on.`;
  }
  return `The analysis of ${subject} scored ${result.healthScore}/100 with no findings. What else should I check?`;
}

export function analysisAssistantPageContext(result: AnalysisResult): Partial<AssistantPageContext> {
  if (result.subjectType === 'run') {
    return {
      title: 'Run analysis',
      path: `/pipelineruns?run=${encodeURIComponent(result.subjectId)}`,
      route: '/pipelineruns/:run_id',
      area: 'pipelineruns',
      resource_type: 'pipeline_run',
      resource_id: result.subjectId,
      resource_name: result.subjectLabel,
      run_id: result.subjectId,
    };
  }

  if (result.subjectType === 'pipeline') {
    return {
      title: 'Pipeline analysis',
      path: `/pipelines/${encodeURIComponent(result.subjectId)}`,
      route: '/pipelines/:pipeline_id',
      area: 'pipelines',
      resource_type: 'pipeline',
      resource_id: result.subjectId,
      resource_name: result.subjectLabel,
      pipeline_id: result.subjectId,
    };
  }

  return {
    title: result.subjectType === 'team' ? 'Team resource analysis' : 'Resource analysis',
    path: '/teams',
    route: '/teams',
    area: 'teams',
    resource_type: result.subjectType,
    resource_id: result.subjectId,
    resource_name: result.subjectLabel,
    scope: result.scopePath || '',
  };
}

function redactEvidenceForPrompt(evidence: AnalysisEvidence): AnalysisEvidence {
  return {
    ...evidence,
    value: evidence.kind === 'redacted' ? evidence.value : truncatePromptValue(evidence.value),
  };
}

function normalizeAnalysisAiPromptContext(context?: AnalysisAiPromptContext | null): AnalysisAiPromptContext | undefined {
  if (!context || !Array.isArray(context.sections)) return undefined;
  const sections = context.sections
    .map(section => ({
      title: truncateEvaluationText(readString(section.title), 120),
      summary: truncateEvaluationText(readString(section.summary), 300),
      items: readArray(section.items)
        .map(item => asRecord(item))
        .filter(Boolean)
        .slice(0, 20)
        .map(item => redactEvidenceForPrompt({
          label: truncateEvaluationText(readString(item?.label), 120),
          value: truncatePromptValue(readString(item?.value)),
          kind: readAnalysisEvidenceKind(item?.kind),
        })),
      lines: retainedContextLines(section)
        .map(line => truncatePromptValue(line)),
      limitations: readStringArray(section.limitations)
        .slice(0, 8)
        .map(item => truncateEvaluationText(item, 180)),
    }))
    .filter(section =>
      section.title &&
      (section.summary || section.items.length > 0 || section.lines.length > 0 || section.limitations.length > 0)
    )
    .slice(0, 6);
  return sections.length > 0 ? { sections } : undefined;
}

function retainedContextLines(section: AnalysisAiPromptContextSection): string[] {
  const lines = readStringArray(section.lines);
  return section.lineRetention === 'tail' ? lines.slice(-60) : lines.slice(0, 60);
}

function readAnalysisEvidenceKind(value: unknown): AnalysisEvidence['kind'] {
  const normalized = readString(value);
  return normalized === 'fact' || normalized === 'metric' || normalized === 'inference' || normalized === 'redacted'
    ? normalized
    : undefined;
}

function truncatePromptValue(value: string): string {
  const normalized = value.replace(/\s+/g, ' ').trim();
  if (normalized.length <= 500) return normalized;
  return `${normalized.slice(0, 500)}...`;
}

function analysisSubjectPromptInstructions(result: AnalysisResult) {
  if (result.subjectType === 'run') {
    return [
      'Subject instructions for Analyse Run:',
      '- Identify the most likely problem domain and distinguish root cause from downstream symptoms.',
      '- Use first failed step/task, exit code, failure reason, failed step/task configuration, approvals, child runs, final outputs, last-success comparison, and provided log excerpts.',
      '- A wrapper or aggregate task name is not the root cause by itself. Name the underlying failed check, file, command, dependency, permission, or timeout when the log excerpt shows it.',
      '- If a log excerpt is present, cite the specific failing signal from it. If logs are missing or insufficient, say the precise cause cannot be determined yet instead of guessing from commit or scope changes alone.',
      '- Suggested fixes must say what to inspect or change first and why; avoid generic rerun, review commit, or check runner advice unless the evidence specifically supports it.',
      '- Call out when missing logs, command output, or hidden external dependency evidence prevents a confident conclusion.',
    ].join('\n');
  }
  if (result.subjectType === 'pipeline') {
    return [
      'Subject instructions for Analyse Pipeline:',
      '- Evaluate YAML correctness, dependency graph safety, credential references, container image policy, approvals, runtime pools, observability, and GitOps-safe maintainability.',
      '- Explain whether findings block pre-execution readiness or are improvement opportunities.',
      '- Suggested fixes must be reviewable changes to pipeline definition, ownership, monitoring, or policy; do not claim execution happened.',
    ].join('\n');
  }
  if (result.subjectType === 'resource') {
    return [
      'Subject instructions for Analyse Resource:',
      '- Evaluate this visible resource metadata for ownership clarity, access boundary, credential exposure risk, GitOps/database drift, reuse, and consumer impact.',
      '- Suggested fixes must preserve existing consumers until usage is verified.',
    ].join('\n');
  }
  return [
    'Subject instructions for Analyse Team Resources:',
    '- Evaluate visible team/application resources for duplicate resources, unclear ownership, excessive privilege metadata, inherited/global exposure, GitOps/database drift, unused candidates, and reuse opportunities.',
    '- Suggested fixes must be safe for enterprise teams: verify ownership and consumers before consolidation or deletion.',
  ].join('\n');
}

function parseJSONObject(content: string): Record<string, unknown> | null {
  const trimmed = content.trim();
  const candidates = [
    trimmed,
    trimmed.replace(/^```(?:json)?\s*/i, '').replace(/\s*```$/i, ''),
    extractJSONObject(trimmed),
  ].filter(Boolean);
  for (const candidate of candidates) {
    try {
      const parsed = JSON.parse(candidate);
      const record = asRecord(parsed);
      if (record) return record;
    } catch {
      // Try the next candidate.
    }
  }
  return null;
}

function extractJSONObject(content: string) {
  const start = content.indexOf('{');
  const end = content.lastIndexOf('}');
  if (start < 0 || end <= start) return '';
  return content.slice(start, end + 1);
}

function fallbackStructuredEvaluation(content: string): StructuredAnalysisAiEvaluation {
  const hasContent = Boolean(content.replace(/\s+/g, ' ').trim());
  return {
    summary: hasContent ? 'AI evaluation returned an unstructured response.' : 'AI evaluation returned an empty response.',
    problem: {
      title: 'AI response needs regeneration',
      detail: 'The model responded, but the reviewer could not separate the result into the expected fields.',
    },
    score: {
      health: null,
      detail: 'Use the deterministic score basis shown in the left rail while regenerating AI Evaluation.',
      drivers: [],
      findings: [],
      categoryScores: [],
    },
    fixes: [],
    evidenceNeeded: ['Regenerate AI Evaluation after confirming the selected LLM profile is reachable.'],
    confidence: 0,
    structured: false,
  };
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

function readArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function readString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function readStringArray(value: unknown): string[] {
  return readArray(value).map(readString).filter(Boolean);
}

function readNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function readOptionalNumber(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value.trim());
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function readReviewedHealth(scoreRecord: Record<string, unknown>) {
  const value = readOptionalNumber(
    scoreRecord.reviewed_health ??
    scoreRecord.reviewedHealth ??
    scoreRecord.health ??
    scoreRecord.overall_health ??
    scoreRecord.overallHealth
  );
  return value == null ? null : clampPercent(value);
}

function readScoredFindings(scoreRecord: Record<string, unknown>, parsed: Record<string, unknown>) {
  return readArray(scoreRecord.findings || scoreRecord.scored_findings || parsed.scored_findings)
    .map(item => {
      const record = asRecord(item) || {};
      const title = readString(record.title);
      const basis = readString(record.basis) || readString(record.detail) || readString(record.evidence);
      if (!title || !basis) return null;
      const deduction = readOptionalNumber(record.deduction ?? record.score_impact ?? record.scoreImpact);
      return {
        title: truncateEvaluationText(title, 120),
        severity: readAnalysisSeverity(record.severity),
        category: readAnalysisCategory(record.category),
        basis: truncateEvaluationText(basis, 280),
        deduction: deduction == null ? null : Math.max(0, Math.round(deduction)),
        confidence: clampConfidence(readOptionalNumber(record.confidence) ?? 0),
      };
    })
    .filter((finding): finding is AnalysisAiScoredFinding => Boolean(finding))
    .slice(0, 10);
}

function readCategoryScores(scoreRecord: Record<string, unknown>) {
  return readArray(scoreRecord.category_scores || scoreRecord.categoryScores)
    .map(item => {
      const record = asRecord(item) || {};
      const score = readOptionalNumber(record.score);
      if (score == null) return null;
      return {
        category: readAnalysisCategory(record.category),
        score: clampPercent(score),
        basis: truncateEvaluationText(readString(record.basis) || readString(record.detail), 240),
      };
    })
    .filter((score): score is AnalysisAiCategoryScore => Boolean(score))
    .slice(0, 8);
}

function readAnalysisSeverity(value: unknown): AnalysisSeverity {
  const normalized = readString(value).toLowerCase().replace(/[^a-z]+/g, '_');
  if (normalized === 'critical' || normalized === 'blocker') return 'critical';
  if (normalized === 'high') return 'high';
  if (normalized === 'medium' || normalized === 'moderate') return 'medium';
  if (normalized === 'low' || normalized === 'info' || normalized === 'informational') return 'low';
  if (normalized === 'opportunity' || normalized === 'optimization') return 'opportunity';
  return 'medium';
}

function readAnalysisCategory(value: unknown): AnalysisCategory {
  const normalized = readString(value).toLowerCase().replace(/[^a-z]+/g, '_').replace(/^_+|_+$/g, '');
  if (normalized === 'security' || normalized === 'governance' || normalized === 'release_governance') return 'security';
  if (normalized === 'reliability' || normalized === 'resilience' || normalized === 'availability') return 'reliability';
  if (normalized === 'organization' || normalized === 'ownership' || normalized === 'gitops' || normalized === 'drift') return 'organization';
  if (normalized === 'efficiency' || normalized === 'performance' || normalized === 'optimization') return 'efficiency';
  if (normalized === 'monitoring' || normalized === 'observability' || normalized === 'telemetry') return 'monitoring';
  if (normalized === 'maintainability' || normalized === 'maintenance') return 'maintainability';
  if (normalized === 'bug' || normalized === 'defect' || normalized === 'failure' || normalized === 'diagnosis') return 'bug';
  if (normalized === 'cost' || normalized === 'spend') return 'cost';
  return 'maintainability';
}

function clampConfidence(value: number): number {
  if (value > 0 && value <= 1) {
    return Math.max(0, Math.min(100, Math.round(value * 100)));
  }
  return clampPercent(value);
}

function clampPercent(value: number): number {
  return Math.max(0, Math.min(100, Math.round(value)));
}

function truncateEvaluationText(value: string, maxLength: number): string {
  const normalized = value.replace(/\s+/g, ' ').trim();
  if (normalized.length <= maxLength) return normalized;
  return `${normalized.slice(0, Math.max(0, maxLength - 3))}...`;
}
