import assert from 'node:assert/strict';
import test from 'node:test';
import { apiClient } from '../../lib/api.js';
import { requestAnalysisAiEvaluation } from './api.js';
import { analysisResultFromServer } from './serverResult.js';

const now = new Date('2026-07-24T10:00:00Z');

test('selects the unscoped default LLM profile for scoped team analysis', async () => {
  const originalFetch = apiClient.fetch.bind(apiClient);
  const calls: Array<{ input: string; body?: string }> = [];

  (apiClient as { fetch: typeof apiClient.fetch }).fetch = async (input, init) => {
    const path = String(input);
    calls.push({ input: path, body: typeof init?.body === 'string' ? init.body : undefined });
    if (path === '/v1/assistant/config') {
      return jsonResponse({
        enabled: true,
        features: {
          pipeline_debugging: true,
          maintenance_recommendations: true,
        },
      });
    }
    if (path === '/v1/assistant/models') {
      return jsonResponse({
        default_profile: 'hosted-default',
        profiles: [
          {
            name: 'payments-reviewer',
            provider: 'openai',
            model: 'gpt-team',
            status: 'valid',
            allowed_in_scope: true,
          },
          {
            name: 'hosted-default',
            provider: 'openai',
            model: 'gpt-default',
            status: 'valid',
            allowed_in_scope: true,
          },
        ],
      });
    }
    if (path.startsWith('/v1/assistant/models?')) {
      throw new Error(`profile lookup must not be scoped for analysis: ${path}`);
    }
    if (path === '/v1/analysis/evaluate') {
      return jsonResponse({
        content: JSON.stringify({
          summary: 'Team analysis reviewed.',
          problem: { title: 'No blocking issue', detail: 'The supplied snapshot has no critical blocker.' },
          score: { reviewed_health: 100, detail: 'No scored deductions.', drivers: [], findings: [], category_scores: [] },
          fixes: [],
          evidence_needed: [],
          confidence: 90,
        }),
        profile_name: 'hosted-default',
        provider: 'openai',
        model: 'gpt-default',
        generated_at: now.toISOString(),
        usage: { total_tokens: 42, duration_ms: 1200 },
      });
    }
    return new Response('not found', { status: 404 });
  };

  try {
    const result = teamAnalysisFixture();
    const evaluation = await requestAnalysisAiEvaluation(result);
    const profileLookup = calls.find(call => call.input.startsWith('/v1/assistant/models'));
    const evaluationCall = calls.find(call => call.input === '/v1/analysis/evaluate');
    const requestBody = JSON.parse(evaluationCall?.body || '{}') as Record<string, unknown>;

    assert.equal(profileLookup?.input, '/v1/assistant/models');
    assert.equal(requestBody.scope, 'platform/payments');
    assert.equal(requestBody.selected_llm_profile, 'hosted-default');
    assert.equal(evaluation.profileName, 'hosted-default');
  } finally {
    (apiClient as { fetch: typeof apiClient.fetch }).fetch = originalFetch;
  }
});

function jsonResponse(value: unknown) {
  return new Response(JSON.stringify(value), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}

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
