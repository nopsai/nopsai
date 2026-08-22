import assert from 'node:assert/strict';
import test from 'node:test';
import { parseAnalysisAiEvaluation } from './ai.js';
import type { AnalysisAiEvaluation } from './api.js';
import { buildPipelineAnalysis } from './model.js';
import {
  listCachedAnalysisEvaluations,
  loadCachedAnalysisEvaluation,
  loadLatestReusableAnalysisEvaluation,
  saveCachedAnalysisEvaluation,
} from './evaluationCache.js';

class MemoryStorage implements Storage {
  private readonly values = new Map<string, string>();
  get length() { return this.values.size; }
  clear() { this.values.clear(); }
  getItem(key: string) { return this.values.get(key) ?? null; }
  key(index: number) { return Array.from(this.values.keys())[index] ?? null; }
  removeItem(key: string) { this.values.delete(key); }
  setItem(key: string, value: string) { this.values.set(key, value); }
}

function installMemoryStorage() {
  Object.defineProperty(globalThis, 'localStorage', {
    value: new MemoryStorage(),
    configurable: true,
  });
}

const now = new Date('2026-07-24T10:00:00Z');

test('stores AI evaluation history by subject and exact snapshot revision', () => {
  installMemoryStorage();
  const result = buildPipelineAnalysis({
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
  const changedResult = buildPipelineAnalysis({
    now,
    scope: 'complete',
    includeRunHistory: false,
    detail: {
      id: 'platform/payments/deploy',
      name: 'deploy',
      rawYaml: 'name: deploy\nsteps:\n  - name: test',
    },
    graphData: { error: null, steps: [] },
    triggers: [],
    recentRuns: [],
  });
  const evaluation: AnalysisAiEvaluation = {
    evaluation: parseAnalysisAiEvaluation(JSON.stringify({
      summary: 'Pipeline needs approval.',
      problem: { title: 'Missing gate', detail: 'No approval gate was visible.' },
      score: {
        reviewed_health: 75,
        detail: 'One high finding was scored.',
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
    serverGrounded: true,
    dataSources: ['/v1/monitoring/summary'],
    usage: { totalTokens: 120, durationMs: 800 },
  };

  const cached = saveCachedAnalysisEvaluation(result, evaluation, new Date('2026-07-24T10:06:00Z'));

  assert.equal(cached?.snapshotRevision, result.snapshotRevision);
  assert.equal(loadCachedAnalysisEvaluation(result)?.evaluation.score.health, 75);
  assert.equal(loadCachedAnalysisEvaluation(changedResult), null);
  assert.equal(loadLatestReusableAnalysisEvaluation(changedResult)?.evaluation.score.health, 75);
  assert.equal(listCachedAnalysisEvaluations(result).length, 1);
});
