import assert from 'node:assert/strict';
import test from 'node:test';
import { analysisResultFromServer } from './serverResult.js';

const fallback = {
  subjectType: 'team' as const,
  subjectId: '42',
  subjectLabel: 'Payments',
  scopePath: 'platform/payments',
  title: 'Payments resource analysis',
};

function serverPayload(overrides: Record<string, unknown> = {}) {
  return {
    analysis: 'team',
    subject: { type: 'team', id: '42', label: 'Payments', path: 'platform/payments' },
    window: { from: '2026-07-22T00:00:00Z', to: '2026-08-21T00:00:00Z', days: 30 },
    health_score: 72,
    score_basis: { baseline: 100, formula: 'Starts at 100.', severity_weights: { critical: 25, high: 15, medium: 8, low: 3, opportunity: 1 }, total_deduction: 28 },
    scores: [{ category: 'security', score: 85, finding_count: 1, deduction: 15, basis: 'Security starts at 100.' }],
    findings: [
      {
        id: 'team-42-security-high-1',
        category: 'security',
        severity: 'high',
        title: '2 credential resources share one name',
        summary: 'Rotating the wrong one is an outage.',
        evidence: [{ label: 'registry-token', value: 'credential in platform', kind: 'fact' }],
        recommendations: [{ title: 'Keep one canonical resource', detail: 'Consolidate the variants.' }],
        confidence: 0.82,
      },
    ],
    limitations: ['Only evidence the current user may read contributes.'],
    data_sources: ['/v1/monitoring/summary'],
    summary: 'Payments scores 72/100.',
    ...overrides,
  };
}

test('maps the server contract onto the shape the modal renders', () => {
  const result = analysisResultFromServer(serverPayload(), fallback);

  assert.equal(result.healthScore, 72);
  assert.equal(result.subjectLabel, 'Payments');
  assert.equal(result.summary, 'Payments scores 72/100.');
  assert.equal(result.findings.length, 1);
  assert.equal(result.findings[0].category, 'security');
  assert.equal(result.findings[0].severity, 'high');
  // The server states confidence as a fraction; the modal renders a percentage.
  assert.equal(result.findings[0].confidence, 82);
  assert.deepEqual(result.counts, { critical: 0, high: 1, medium: 0, low: 0, opportunity: 0 });
  assert.equal(result.scores[0].score, 85);
  assert.ok(result.scoreBasis.inputs.includes('/v1/monitoring/summary'));
});

// A missing score must not render as zero out of a hundred, which reads as the
// worst possible result rather than as "we could not look".
test('marks an unscored analysis instead of showing it as a zero score', () => {
  const result = analysisResultFromServer(serverPayload({ health_score: null, findings: [] }), fallback);

  assert.match(result.scoreBasis.limitations[0], /No health score was produced/);
  assert.ok(result.safeguards.some(item => /absent score as unknown/.test(item)));
});

test('changes the snapshot revision when the analysis changes', () => {
  const first = analysisResultFromServer(serverPayload(), fallback);
  const same = analysisResultFromServer(serverPayload(), fallback);
  const rescored = analysisResultFromServer(serverPayload({ health_score: 40 }), fallback);

  assert.equal(first.snapshotRevision, same.snapshotRevision);
  assert.notEqual(first.snapshotRevision, rescored.snapshotRevision);
});

test('carries a run diagnosis through to the modal', () => {
  const result = analysisResultFromServer(
    serverPayload({ analysis: 'run', primary_diagnosis: { domain: 'Application tests', confidence: 0.82 } }),
    { ...fallback, subjectType: 'run', subjectId: 'run-9', subjectLabel: 'platform/deploy-api', title: 'Run analysis' }
  );

  assert.equal(result.primaryDiagnosis?.domain, 'Application tests');
  assert.equal(result.primaryDiagnosis?.confidence, 82);
});

test('falls back safely on an unrecognised payload', () => {
  const result = analysisResultFromServer({ findings: [{ category: 'nonsense', severity: 'weird' }] }, fallback);

  assert.equal(result.findings[0].category, 'organization');
  assert.equal(result.findings[0].severity, 'low');
  assert.equal(result.subjectLabel, 'Payments');
  assert.equal(result.healthScore, 0);
});

test('the evaluation notice scales with how much there is to review', async () => {
  const { analysisEvaluationCostNotice } = await import('./evaluationCost.js');

  assert.match(analysisEvaluationCostNotice(3), /one model call and records its spend/);
  assert.match(analysisEvaluationCostNotice(20), /several seconds/);
  assert.match(analysisEvaluationCostNotice(60), /tens of seconds/);
  // Every wording says cancelling is possible or that spend is recorded, so the
  // cost is never a surprise after the fact.
  for (const count of [1, 20, 60]) {
    assert.match(analysisEvaluationCostNotice(count), /cancel|spend/i);
  }
});
