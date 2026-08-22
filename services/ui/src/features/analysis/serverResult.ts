import { asRecord, normalizeNumber, readString } from '../system/data.js';
import {
  TEAM_RESOURCE_ANALYSIS_TABS,
  PIPELINE_ANALYSIS_TABS,
  RUN_ANALYSIS_TABS,
  type AnalysisCategory,
  type AnalysisEvidence,
  type AnalysisFinding,
  type AnalysisResult,
  type AnalysisScore,
  type AnalysisSeverity,
  type AnalysisSubjectType,
} from './model.js';

const SEVERITIES: AnalysisSeverity[] = ['critical', 'high', 'medium', 'low', 'opportunity'];
const CATEGORIES: AnalysisCategory[] = [
  'security', 'reliability', 'organization', 'efficiency', 'monitoring', 'maintainability', 'bug', 'cost',
];

/**
 * Maps the server analysis contract onto the shape the modal renders.
 *
 * The server is the engine; this only translates. Two differences it has to
 * reconcile: the server states confidence as 0-1 where the UI shows a percentage,
 * and the server returns no score at all when it could not read enough evidence,
 * which must not render as a score of zero.
 */
export function analysisResultFromServer(payload: unknown, fallback: {
  subjectType: AnalysisSubjectType;
  subjectId: string;
  subjectLabel: string;
  scopePath?: string;
  title: string;
}): AnalysisResult {
  const record = asRecord(payload) || {};
  const subject = asRecord(record.subject) || {};
  const window = asRecord(record.window) || {};
  const basis = asRecord(record.score_basis) || {};
  const findings = readFindings(record, fallback);
  const counts = countBySeverity(findings);
  const scored = record.health_score !== null && record.health_score !== undefined;
  const limitations = readStrings(record.limitations);

  return {
    title: fallback.title,
    subjectType: fallback.subjectType,
    subjectId: readString(subject.id) || fallback.subjectId,
    subjectLabel: readString(subject.label) || fallback.subjectLabel,
    scopePath: readString(subject.path) || fallback.scopePath,
    generatedAt: new Date().toISOString(),
    // The revision has to change whenever the analysis does, because cached AI
    // reviews are keyed on it.
    snapshotRevision: [
      readString(window.to) || 'unknown-window',
      scored ? String(normalizeNumber(record.health_score)) : 'unscored',
      String(findings.length),
    ].join('-'),
    summary: readString(record.summary),
    healthScore: scored ? clamp(Math.round(normalizeNumber(record.health_score)), 0, 100) : 0,
    scoreBasis: {
      baseline: normalizeNumber(basis.baseline) || 100,
      formula: readString(basis.formula),
      severityWeights: readSeverityWeights(basis.severity_weights),
      findingCount: findings.length,
      totalDeduction: normalizeNumber(basis.total_deduction),
      severityCounts: counts,
      inputs: readStrings(record.data_sources),
      limitations: scored
        ? limitations
        : ['No health score was produced: the evidence this analysis needs could not be read.', ...limitations],
    },
    scores: readScores(record, findings),
    findings,
    counts,
    tabs: tabsForSubject(fallback.subjectType),
    safeguards: [
      'Read-only analysis computed on the server from evidence the current user is allowed to read.',
      'Findings, severities, and scores are deterministic. Credential and secret values are never shown.',
      ...(scored ? [] : ['This result is incomplete; treat the absent score as unknown rather than healthy.']),
    ],
    ...(readDiagnosis(record.primary_diagnosis) || {}),
  };
}

function readFindings(record: Record<string, unknown>, fallback: { subjectType: AnalysisSubjectType; subjectId: string }): AnalysisFinding[] {
  const generatedAt = new Date().toISOString();
  return readArray(record.findings).map((entry, index) => {
    const finding = asRecord(entry) || {};
    return {
      id: readString(finding.id) || `${fallback.subjectType}-${fallback.subjectId}-${index}`,
      subjectType: fallback.subjectType,
      subjectId: fallback.subjectId,
      category: readCategory(finding.category),
      severity: readSeverity(finding.severity),
      title: readString(finding.title),
      summary: readString(finding.summary),
      evidence: readEvidence(finding.evidence),
      affectedResources: [],
      recommendations: readArray(finding.recommendations).map(item => {
        const recommendation = asRecord(item) || {};
        return { title: readString(recommendation.title), detail: readString(recommendation.detail) };
      }),
      // The server states confidence as a fraction; the modal renders a percentage.
      confidence: clamp(Math.round(normalizeNumber(finding.confidence) * 100), 0, 100),
      generatedAt,
      snapshotRevision: '',
    };
  });
}

function readScores(record: Record<string, unknown>, findings: AnalysisFinding[]): AnalysisScore[] {
  return readArray(record.scores).map(entry => {
    const score = asRecord(entry) || {};
    const category = readCategory(score.category);
    return {
      category,
      label: category,
      score: clamp(Math.round(normalizeNumber(score.score)), 0, 100),
      findingCount: normalizeNumber(score.finding_count) || findings.filter(item => item.category === category).length,
      deduction: normalizeNumber(score.deduction),
      basis: readString(score.basis),
    };
  });
}

function readEvidence(value: unknown): AnalysisEvidence[] {
  return readArray(value).map(entry => {
    const item = asRecord(entry) || {};
    const kind = readString(item.kind);
    return {
      label: readString(item.label),
      value: readString(item.value),
      kind: kind === 'metric' || kind === 'inference' || kind === 'redacted' ? kind : 'fact',
    };
  });
}

function readDiagnosis(value: unknown): Pick<AnalysisResult, 'primaryDiagnosis'> | null {
  const record = asRecord(value);
  if (!record) return null;
  const domain = readString(record.domain);
  if (!domain) return null;
  return {
    primaryDiagnosis: {
      domain: domain as AnalysisResult['primaryDiagnosis'] extends { domain: infer D } ? D : never,
      confidence: clamp(Math.round(normalizeNumber(record.confidence) * 100), 0, 100),
    },
  };
}

function readSeverityWeights(value: unknown): Record<AnalysisSeverity, number> {
  const record = asRecord(value) || {};
  return SEVERITIES.reduce((weights, severity) => {
    weights[severity] = normalizeNumber(record[severity]);
    return weights;
  }, {} as Record<AnalysisSeverity, number>);
}

function countBySeverity(findings: AnalysisFinding[]): Record<AnalysisSeverity, number> {
  return SEVERITIES.reduce((counts, severity) => {
    counts[severity] = findings.filter(finding => finding.severity === severity).length;
    return counts;
  }, {} as Record<AnalysisSeverity, number>);
}

function tabsForSubject(subjectType: AnalysisSubjectType) {
  if (subjectType === 'pipeline') return PIPELINE_ANALYSIS_TABS;
  if (subjectType === 'run') return RUN_ANALYSIS_TABS;
  return TEAM_RESOURCE_ANALYSIS_TABS;
}

function readCategory(value: unknown): AnalysisCategory {
  const category = readString(value) as AnalysisCategory;
  return CATEGORIES.includes(category) ? category : 'organization';
}

function readSeverity(value: unknown): AnalysisSeverity {
  const severity = readString(value) as AnalysisSeverity;
  return SEVERITIES.includes(severity) ? severity : 'low';
}

function readArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function readStrings(value: unknown): string[] {
  return readArray(value).map(readString).filter(Boolean);
}

function clamp(value: number, min: number, max: number): number {
  if (!Number.isFinite(value)) return min;
  return Math.min(Math.max(value, min), max);
}
