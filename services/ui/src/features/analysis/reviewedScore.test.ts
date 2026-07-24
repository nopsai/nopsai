import assert from 'node:assert/strict';
import test from 'node:test';
import { parseAnalysisAiEvaluation } from './ai.js';
import type { AnalysisAiEvaluation } from './api.js';
import { buildPipelineAnalysis } from './model.js';
import { buildAnalysisScoreView, formatAnalysisReportWithScoreView } from './reviewedScore.js';

const now = new Date('2026-07-24T10:00:00Z');

test('uses structured AI findings to update health and metric scores', () => {
  const result = buildPipelineAnalysis({
    now,
    scope: 'complete',
    includeRunHistory: true,
    detail: {
      id: 'platform/nopsai/release',
      name: 'nopsai-platform-release',
      rawYaml: [
        'name: nopsai-platform-release',
        'source: database',
        'steps:',
        '  - name: publish-release',
        '    script: curl https://installer.example/release.sh | bash',
        '    variables:',
        '      target_environment: production',
      ].join('\n'),
      source: 'database',
    },
    graphData: {
      error: null,
      steps: [{
        name: 'publish-release',
        status: 'pending',
        depends_on: [],
        configuration: {
          script: 'curl https://installer.example/release.sh | bash',
          variables: { target_environment: 'production' },
        },
      }],
    },
    triggers: [],
    recentRuns: [
      { run_id: 'run-1', status: 'success', duration: '2m' },
      { run_id: 'run-2', status: 'success', duration: '2m' },
      { run_id: 'run-3', status: 'success', duration: '9m' },
      { run_id: 'run-4', status: 'success', duration: '2m' },
      { run_id: 'run-5', status: 'success', duration: '10m' },
    ],
  });
  const evaluation: AnalysisAiEvaluation = {
    evaluation: parseAnalysisAiEvaluation(JSON.stringify({
      summary: 'Release governance is critically weak.',
      problem: {
        title: 'Missing release approval gate',
        detail: 'The publish-release dependency path has no explicit approval before publication.',
      },
      score: {
        reviewed_health: 36,
        detail: 'One critical, two high, one medium, and one opportunity finding reduce the score by 64.',
        drivers: ['Missing Approval Gate', 'Non-GitOps Pipeline Source', 'Unsafe Shell Scripting'],
        findings: [
          { title: 'Missing Approval Gate', severity: 'critical', category: 'security', basis: 'publish-release has no visible approval predecessor.', deduction: 25, confidence: 92 },
          { title: 'Non-GitOps Pipeline Source', severity: 'high', category: 'organization', basis: 'Pipeline source is database for a durable release workflow.', deduction: 15, confidence: 86 },
          { title: 'Unsafe Shell Scripting', severity: 'high', category: 'security', basis: 'Release script pipes remote installer output to bash.', deduction: 15, confidence: 84 },
          { title: 'Inefficient Dependency Installation', severity: 'medium', category: 'efficiency', basis: 'Dependencies appear installed repeatedly in release tasks.', deduction: 8, confidence: 72 },
          { title: 'Duration Outliers', severity: 'opportunity', category: 'efficiency', basis: 'Recent runs include duration outliers above peer median.', deduction: 1, confidence: 68 },
        ],
        category_scores: [
          { category: 'security', score: 60, basis: 'Critical approval and high script findings affect security.' },
        ],
      },
      fixes: [],
      evidence_needed: [],
      confidence: 88,
    })),
    generatedAt: '2026-07-24T10:05:00Z',
    modelLabel: 'gpt-test',
    profileName: 'analysis',
    usage: { totalTokens: 1000, durationMs: 2500 },
  };

  const scoreView = buildAnalysisScoreView(result, evaluation);

  assert.equal(scoreView.source, 'ai-reviewed');
  assert.equal(scoreView.healthScore, 36);
  assert.equal(scoreView.deterministicHealthScore, result.healthScore);
  assert.equal(scoreView.counts.critical, 1);
  assert.equal(scoreView.counts.high, 2);
  assert.equal(scoreView.counts.medium, 1);
  assert.equal(scoreView.counts.opportunity, 1);
  assert.equal(scoreView.scoreBasis.totalDeduction, 64);
  assert.ok(scoreView.scores.some(score => score.category === 'organization'));
  assert.equal(scoreView.scores.find(score => score.category === 'security')?.score, 60);
  assert.match(formatAnalysisReportWithScoreView(result, scoreView, evaluation), /AI-reviewed scored findings/);
});

test('marks reusable cached reviews from older snapshots as previous-snapshot scores', () => {
  const result = buildPipelineAnalysis({
    now,
    scope: 'complete',
    includeRunHistory: false,
    detail: {
      id: 'platform/payments/deploy',
      name: 'deploy',
      rawYaml: 'name: deploy\nsteps:\n  - name: deploy\n    script: ./deploy.sh production',
    },
    graphData: {
      error: null,
      steps: [{
        name: 'deploy',
        status: 'pending',
        depends_on: [],
        configuration: { script: './deploy.sh production' },
      }],
    },
    triggers: [],
    recentRuns: [],
  });
  const evaluation: AnalysisAiEvaluation & { snapshotRevision: string } = {
    evaluation: parseAnalysisAiEvaluation(JSON.stringify({
      summary: 'Previous review found one approval issue.',
      problem: { title: 'Missing gate', detail: 'No approval gate was visible.' },
      score: {
        reviewed_health: 75,
        detail: 'One high finding reduced the score.',
        drivers: ['Missing approval gate'],
        findings: [
          { title: 'Missing approval gate', severity: 'high', category: 'security', basis: 'No approval gate was visible.', deduction: 15, confidence: 90 },
        ],
      },
      fixes: [],
      evidence_needed: [],
      confidence: 90,
    })),
    generatedAt: '2026-07-24T10:05:00Z',
    modelLabel: 'gpt-test',
    profileName: 'analysis',
    usage: { totalTokens: 100, durationMs: 1000 },
    snapshotRevision: 'snapshot-older',
  };

  const scoreView = buildAnalysisScoreView(result, evaluation);

  assert.equal(scoreView.healthScore, 85);
  assert.equal(scoreView.snapshotMatches, false);
  assert.equal(scoreView.cachedSnapshotRevision, 'snapshot-older');
  assert.match(scoreView.scoreBasis.limitations[0], /latest cached review/);
  assert.match(formatAnalysisReportWithScoreView(result, scoreView, evaluation), /previous snapshot/);
});
