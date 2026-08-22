import assert from 'node:assert/strict';
import test from 'node:test';
import {
  analysisAssistantChatPrompt,
  analysisAssistantPageContext,
  buildAnalysisAiPrompt,
  buildAnalysisAiPromptSnapshot,
  parseAnalysisAiEvaluation,
} from './ai.js';
import { buildPipelineAnalysis, buildRunAnalysis } from './model.js';
import { analysisResultFromServer } from './serverResult.js';

const now = new Date('2026-07-24T10:00:00Z');

test('builds a redacted run AI prompt with score basis and repair focus', () => {
  const result = buildRunAnalysis({
    now,
    detail: {
      run_info: {
        run_id: 'run-failed',
        pipeline_name: 'deploy',
        pipeline_path: 'platform/payments',
        pipeline_version: '17',
        pipeline_source: 'git',
        status: 'failure',
        is_complete: true,
        git_commit_sha: 'abc125',
        git_ref: 'refs/heads/main',
        scope: 'staging',
        failure_reason: 'integration test failed with token=never-show-this',
      },
      steps: [{
        name: 'integration-tests',
        status: 'failure',
        tasks: [{
          task_id: 'task-1',
          step_name: 'integration-tests',
          task_name: 'payment-refund-tests',
          status: 'failure',
          exit_code: 1,
          task_index: 0,
        }],
      }],
      child_runs: [],
      approvals: [],
      final_outputs: [],
    },
    comparisonRuns: [],
  });

  const prompt = buildAnalysisAiPrompt(result);
  assert.match(prompt, /Return ONLY valid JSON/);
  assert.match(prompt, /"evidence_needed"/);
  assert.match(prompt, /reviewed_health/);
  assert.match(prompt, /score\.findings/);
  assert.match(prompt, /Starts at 100/);
  assert.match(prompt, /Subject instructions for Analyse Run/);
  assert.match(prompt, /distinguish root cause from downstream symptoms/);
  assert.match(prompt, /Credential or authorization/);
  assert.doesNotMatch(prompt, /quality-gates/);
  assert.doesNotMatch(prompt, /never-show-this/);

  const snapshot = buildAnalysisAiPromptSnapshot(result);
  assert.equal(snapshot.score.health, result.healthScore);
  assert.equal(snapshot.score.deductions, result.scoreBasis.totalDeduction);
  assert.equal(snapshot.primaryDiagnosis?.domain, 'Credential or authorization');
});

test('builds assistant page context for run analysis', () => {
  const result = buildRunAnalysis({
    now,
    detail: {
      run_info: {
        run_id: 'run-123',
        pipeline_name: 'deploy',
        status: 'success',
        is_complete: true,
      },
      steps: [],
      child_runs: [],
      approvals: [],
      final_outputs: [],
    },
    comparisonRuns: [],
  });

  const context = analysisAssistantPageContext(result);
  assert.equal(context.area, 'pipelineruns');
  assert.equal(context.resource_type, 'pipeline_run');
  assert.equal(context.run_id, 'run-123');
});

test('keeps latest additional evidence lines in the AI prompt snapshot', () => {
  const result = buildRunAnalysis({
    now,
    detail: {
      run_info: {
        run_id: 'run-tail',
        pipeline_name: 'nopsai-platform-release',
        status: 'failure',
        is_complete: true,
      },
      steps: [{
        name: 'quality-gates',
        status: 'failure',
        tasks: [{
          task_id: 'task-tail',
          step_name: 'quality-gates',
          task_name: 'quality-gates',
          status: 'failure',
          exit_code: 1,
          task_index: 0,
        }],
      }],
      child_runs: [],
      approvals: [],
      final_outputs: [],
    },
    comparisonRuns: [],
  });

  const lines = [
    ...Array.from({ length: 68 }, (_, index) => `go test ./package-${index} ok`),
    'running scripts/release-tooling-test.sh',
    'chart-values.yaml is missing the nopsai-runner repository',
  ];
  const snapshot = buildAnalysisAiPromptSnapshot(result, {
    sections: [{
      title: 'Failed task log excerpt',
      lines,
      lineRetention: 'tail',
    }],
  });

  const keptLines = snapshot.additionalEvidence?.sections[0]?.lines || [];
  assert.equal(keptLines.length, 60);
  assert.doesNotMatch(JSON.stringify(keptLines), /package-0/);
  assert.match(JSON.stringify(keptLines), /scripts\/release-tooling-test\.sh/);
  assert.match(JSON.stringify(keptLines), /chart-values\.yaml is missing the nopsai-runner repository/);
});

test('uses subject-specific prompt instructions for pipeline and team reviewers', () => {
  const pipeline = buildPipelineAnalysis({
    now,
    scope: 'complete',
    includeRunHistory: false,
    detail: {
      id: 'platform/payments/deploy',
      name: 'deploy',
      rawYaml: 'name: deploy\nsteps: []',
    },
    graphData: { error: null, steps: [] },
    triggers: [],
    recentRuns: [],
  });
  const team = teamAnalysisFixture();

  assert.match(buildAnalysisAiPrompt(pipeline), /Subject instructions for Analyse Pipeline/);
  assert.match(buildAnalysisAiPrompt(pipeline), /YAML correctness/);
  assert.match(buildAnalysisAiPrompt(team), /Subject instructions for Analyse Team Resources/);
  assert.match(buildAnalysisAiPrompt(team), /duplicate resources/);
});

test('preserves team scope path rather than subject id for AI evaluation context', () => {
  const result = teamAnalysisFixture();

  const context = analysisAssistantPageContext(result);
  const snapshot = buildAnalysisAiPromptSnapshot(result);
  assert.equal(result.subjectId, '42');
  assert.equal(result.scopePath, 'platform/payments');
  assert.equal(context.scope, 'platform/payments');
  assert.notEqual(context.scope, result.subjectId);
  assert.equal(snapshot.subject.scopePath, 'platform/payments');
});

test('parses structured AI evaluation without exposing raw long output', () => {
  const evaluation = parseAnalysisAiEvaluation(`
    \`\`\`json
    {
      "summary": "The run likely failed in integration tests.",
      "problem": {"title": "Integration test failure", "detail": "The first failed task is payment-refund-tests."},
      "score": {
        "reviewed_health": 36,
        "detail": "High bug findings reduced the score.",
        "drivers": ["First failed task", "Failure reason"],
        "findings": [
          {"title":"First failed task","severity":"high","category":"bug","basis":"payment-refund-tests failed with exit code 1","deduction":15,"confidence":88}
        ],
        "category_scores": [
          {"category":"bug","score":85,"basis":"One high bug finding was scored."}
        ]
      },
      "fixes": [
        {"title": "Inspect test logs", "detail": "Open the first failed task logs before changing YAML.", "priority": "now", "safe_action": "Open logs"}
      ],
      "evidence_needed": ["Full task log excerpt"],
      "confidence": 0.81
    }
    \`\`\`
  `);

  assert.equal(evaluation.structured, true);
  assert.equal(evaluation.problem.title, 'Integration test failure');
  assert.equal(evaluation.score.health, 36);
  assert.equal(evaluation.score.findings[0]?.severity, 'high');
  assert.equal(evaluation.score.categoryScores[0]?.category, 'bug');
  assert.equal(evaluation.fixes[0].safeAction, 'Open logs');
  assert.equal(evaluation.confidence, 81);
});

test('normalizes unstructured AI evaluation without showing raw assistant text', () => {
  const evaluation = parseAnalysisAiEvaluation('I could not safely execute that assistant plan: hosted MCP evidence missing.');

  assert.equal(evaluation.structured, false);
  assert.equal(evaluation.score.health, null);
  assert.equal(evaluation.summary, 'AI evaluation returned an unstructured response.');
  assert.doesNotMatch(evaluation.problem.detail, /hosted MCP evidence/);
  assert.deepEqual(evaluation.evidenceNeeded, ['Regenerate AI Evaluation after confirming the selected LLM profile is reachable.']);
});

test('builds a chat opener that names the score and the blocking findings', () => {
  const result = buildRunAnalysis({
    now,
    detail: {
      run_info: { run_id: 'run-9', pipeline_name: 'deploy', status: 'failure', is_complete: true },
      steps: [],
      child_runs: [],
      approvals: [],
      final_outputs: [],
    },
    comparisonRuns: [],
  });

  const prompt = analysisAssistantChatPrompt(result);
  const blocking = result.counts.critical + result.counts.high;
  const expected = blocking > 0
    ? `Walk me through the analysis of run ${result.subjectLabel} (score ${result.healthScore}/100, ${blocking} critical or high findings) and tell me what to fix first.`
    : `Walk me through the analysis of run ${result.subjectLabel} (score ${result.healthScore}/100) and tell me which finding is worth acting on.`;
  assert.equal(prompt, expected);
  assert.match(prompt, /Walk me through the analysis of run /);
});

function teamAnalysisFixture(overrides: Record<string, unknown> = {}) {
  return analysisResultFromServer({
    analysis: 'team',
    subject: { type: 'team', id: '42', label: 'Payments', path: 'platform/payments' },
    window: { from: '2026-06-24T10:00:00Z', to: '2026-07-24T10:00:00Z', days: 30 },
    health_score: 82,
    score_basis: { baseline: 100, formula: 'test formula', severity_weights: { critical: 25 }, total_deduction: 18 },
    scores: [{ category: 'security', score: 85, finding_count: 1, deduction: 15, basis: 'Security starts at 100.' }],
    findings: [],
    limitations: [],
    data_sources: ['/v1/monitoring/summary'],
    summary: 'Payments scores 82/100.',
    ...overrides,
  }, {
    subjectType: 'team',
    subjectId: '42',
    subjectLabel: 'Payments',
    scopePath: 'platform/payments',
    title: 'Payments resource analysis',
  });
}
