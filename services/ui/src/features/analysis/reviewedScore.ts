import type { AnalysisAiEvaluation } from './api.js';
import type { AnalysisAiScoredFinding } from './ai.js';
import {
  analysisCategoryLabel,
  formatAnalysisReport,
  type AnalysisCategory,
  type AnalysisResult,
  type AnalysisScore,
  type AnalysisScoreBasis,
  type AnalysisSeverity,
} from './model.js';

export type AnalysisScoreView = {
  source: 'deterministic' | 'ai-reviewed';
  healthScore: number;
  scoreBasis: AnalysisScoreBasis;
  scores: AnalysisScore[];
  counts: Record<AnalysisSeverity, number>;
  reviewedFindings: AnalysisAiScoredFinding[];
  deterministicHealthScore: number;
  snapshotMatches: boolean;
  cachedSnapshotRevision?: string;
  currentSnapshotRevision: string;
  reviewedAt?: string;
  modelLabel?: string;
  profileName?: string;
};

const SEVERITIES: AnalysisSeverity[] = ['critical', 'high', 'medium', 'low', 'opportunity'];

export function buildAnalysisScoreView(
  result: AnalysisResult,
  evaluation?: AnalysisAiEvaluation | null
): AnalysisScoreView {
  if (!evaluation || !hasUsableReviewedScore(evaluation)) {
    return deterministicScoreView(result);
  }

  const reviewed = evaluation.evaluation;
  const cachedSnapshotRevision = evaluationSnapshotRevision(evaluation);
  const snapshotMatches = !cachedSnapshotRevision || cachedSnapshotRevision === result.snapshotRevision;
  const reviewedFindings = [...reviewed.score.findings].sort(compareScoredFindings);
  const counts = reviewedFindings.length > 0 ? countScoredFindings(reviewedFindings) : result.counts;
  const findingDeduction = reviewedFindings.length > 0
    ? reviewedFindings.reduce((total, finding) => total + scoredFindingDeduction(result, finding), 0)
    : null;
  const totalDeduction = findingDeduction ?? Math.max(0, result.scoreBasis.baseline - (reviewed.score.health ?? result.healthScore));
  const healthScore = reviewedFindings.length > 0
    ? clampScore(result.scoreBasis.baseline - totalDeduction)
    : reviewed.score.health ?? result.healthScore;

  return {
    source: 'ai-reviewed',
    healthScore,
    scoreBasis: {
      ...result.scoreBasis,
      formula: `AI-reviewed score starts at ${result.scoreBasis.baseline}; it uses the same severity weights and the model-returned scored findings from the redacted snapshot.`,
      findingCount: reviewedFindings.length || result.scoreBasis.findingCount,
      totalDeduction,
      severityCounts: counts,
      inputs: uniqueStrings([
        ...result.scoreBasis.inputs,
        'AI-reviewed scored findings returned from the redacted evidence packet',
        'AI score drivers and category score basis from the latest structured evaluation',
      ]),
      limitations: [
        snapshotMatches
          ? 'The AI-reviewed score is cached for this exact snapshot revision and should be regenerated after evidence changes.'
          : 'This AI-reviewed score came from the latest cached review for this subject; regenerate AI Evaluation to rescore the current snapshot evidence.',
        'The deterministic score remains the baseline; the reviewed score is an operator decision aid, not an SLO or uptime metric.',
      ],
    },
    scores: reviewedCategoryScores(result, reviewedFindings, reviewed.score.categoryScores),
    counts,
    reviewedFindings,
    deterministicHealthScore: result.healthScore,
    snapshotMatches,
    cachedSnapshotRevision,
    currentSnapshotRevision: result.snapshotRevision,
    reviewedAt: evaluation.generatedAt,
    modelLabel: evaluation.modelLabel,
    profileName: evaluation.profileName,
  };
}

export function formatAnalysisReportWithScoreView(
  result: AnalysisResult,
  scoreView: AnalysisScoreView,
  evaluation?: AnalysisAiEvaluation | null
) {
  if (scoreView.source !== 'ai-reviewed') return formatAnalysisReport(result);

  const lines = [
    result.title,
    `Subject: ${result.subjectLabel}`,
    `Overall health: ${scoreView.healthScore}/100 (${scoreView.snapshotMatches ? 'AI-reviewed' : 'AI-reviewed from previous snapshot'}; deterministic baseline was ${scoreView.deterministicHealthScore}/100)`,
    `Score basis: ${scoreView.scoreBasis.formula}`,
    `Score inputs: ${scoreView.scoreBasis.inputs.join('; ')}`,
    scoreView.reviewedAt ? `Reviewed: ${scoreView.reviewedAt}` : '',
    !scoreView.snapshotMatches && scoreView.cachedSnapshotRevision ? `Review snapshot: ${scoreView.cachedSnapshotRevision}` : '',
    !scoreView.snapshotMatches ? `Current snapshot: ${scoreView.currentSnapshotRevision}` : '',
    scoreView.profileName ? `AI profile: ${scoreView.profileName}${scoreView.modelLabel ? ` / ${scoreView.modelLabel}` : ''}` : '',
    '',
    evaluation?.evaluation.summary || result.summary,
    '',
    'AI-reviewed scored findings:',
  ].filter(Boolean);

  if (scoreView.reviewedFindings.length === 0) {
    lines.push('- No separate AI-scored findings were returned.');
  } else {
    scoreView.reviewedFindings.forEach((finding, index) => {
      lines.push(`${index + 1}. [${finding.severity.toUpperCase()}] ${finding.title}`);
      lines.push(`   Category: ${analysisCategoryLabel(finding.category)}`);
      lines.push(`   Deduction: ${scoredFindingDeduction(result, finding)}`);
      lines.push(`   Confidence: ${finding.confidence}%`);
      lines.push(`   Basis: ${finding.basis}`);
    });
  }

  lines.push('', 'Deterministic baseline report:', formatAnalysisReport(result));
  return lines.join('\n');
}

function deterministicScoreView(result: AnalysisResult): AnalysisScoreView {
  return {
    source: 'deterministic',
    healthScore: result.healthScore,
    scoreBasis: result.scoreBasis,
    scores: result.scores,
    counts: result.counts,
    reviewedFindings: [],
    deterministicHealthScore: result.healthScore,
    snapshotMatches: true,
    currentSnapshotRevision: result.snapshotRevision,
  };
}

function hasUsableReviewedScore(evaluation: AnalysisAiEvaluation) {
  const review = evaluation.evaluation;
  if (!review.structured) return false;
  return review.score.health != null ||
    review.score.findings.length > 0 ||
    review.score.categoryScores.length > 0;
}

function reviewedCategoryScores(
  result: AnalysisResult,
  findings: AnalysisAiScoredFinding[],
  explicitScores: Array<{ category: AnalysisCategory; score: number; basis: string }>
): AnalysisScore[] {
  const categories = uniqueCategories([
    ...result.scores.map(score => score.category),
    ...findings.map(finding => finding.category),
    ...explicitScores.map(score => score.category),
  ]);
  return categories.map(category => {
    const label = analysisCategoryLabel(category);
    const explicit = explicitScores.find(score => score.category === category);
    const categoryFindings = findings.filter(finding => finding.category === category);
    const baseScore = result.scores.find(score => score.category === category);
    const deduction = categoryFindings.reduce((total, finding) => total + scoredFindingDeduction(result, finding), 0);
    const score = explicit
      ? explicit.score
      : categoryFindings.length > 0
        ? clampScore(result.scoreBasis.baseline - deduction)
        : baseScore?.score ?? result.scoreBasis.baseline;
    return {
      category,
      label,
      score,
      findingCount: categoryFindings.length || baseScore?.findingCount || 0,
      deduction: explicit ? Math.max(0, result.scoreBasis.baseline - explicit.score) : deduction || baseScore?.deduction || 0,
      basis: explicit?.basis ||
        (categoryFindings.length > 0
          ? `${label} uses ${categoryFindings.length} AI-scored ${categoryFindings.length === 1 ? 'finding' : 'findings'} and subtracts ${deduction} points.`
          : `${label} has no AI-scored finding for this snapshot; deterministic metric basis is retained.`),
    };
  });
}

function scoredFindingDeduction(result: AnalysisResult, finding: AnalysisAiScoredFinding) {
  if (finding.deduction != null && finding.deduction >= 0) return finding.deduction;
  return result.scoreBasis.severityWeights[finding.severity] ?? 0;
}

function countScoredFindings(findings: AnalysisAiScoredFinding[]): Record<AnalysisSeverity, number> {
  return SEVERITIES.reduce((counts, severity) => {
    counts[severity] = findings.filter(finding => finding.severity === severity).length;
    return counts;
  }, {
    critical: 0,
    high: 0,
    medium: 0,
    low: 0,
    opportunity: 0,
  } satisfies Record<AnalysisSeverity, number>);
}

function compareScoredFindings(left: AnalysisAiScoredFinding, right: AnalysisAiScoredFinding) {
  const severityCompare = SEVERITIES.indexOf(left.severity) - SEVERITIES.indexOf(right.severity);
  if (severityCompare !== 0) return severityCompare;
  return left.title.localeCompare(right.title);
}

function uniqueCategories(categories: AnalysisCategory[]) {
  const seen = new Set<AnalysisCategory>();
  return categories.filter(category => {
    if (seen.has(category)) return false;
    seen.add(category);
    return true;
  });
}

function uniqueStrings(values: string[]) {
  const seen = new Set<string>();
  return values.filter(value => {
    const normalized = value.trim();
    if (!normalized || seen.has(normalized)) return false;
    seen.add(normalized);
    return true;
  });
}

function clampScore(value: number) {
  return Math.max(0, Math.min(100, Math.round(value)));
}

function evaluationSnapshotRevision(evaluation: AnalysisAiEvaluation) {
  const cached = evaluation as AnalysisAiEvaluation & { snapshotRevision?: unknown };
  return typeof cached.snapshotRevision === 'string' && cached.snapshotRevision.trim()
    ? cached.snapshotRevision.trim()
    : undefined;
}
