import type { AnalysisAiEvaluation } from './api.js';
import type { AnalysisResult } from './model.js';

export type CachedAnalysisAiEvaluation = AnalysisAiEvaluation & {
  cacheVersion: 1;
  subjectType: AnalysisResult['subjectType'];
  subjectId: string;
  subjectLabel: string;
  snapshotRevision: string;
  cachedAt: string;
};

const STORAGE_KEY = 'nopsai.analysis.ai-evaluations.v1';
const MAX_RECORDS = 80;

type AnalysisEvaluationCachePayload = {
  version: 1;
  records: CachedAnalysisAiEvaluation[];
};

export function loadCachedAnalysisEvaluation(result: AnalysisResult): CachedAnalysisAiEvaluation | null {
  return listCachedAnalysisEvaluations(result, MAX_RECORDS)
    .find(record => record.snapshotRevision === result.snapshotRevision) || null;
}

export function loadLatestReusableAnalysisEvaluation(result: AnalysisResult): CachedAnalysisAiEvaluation | null {
  return listCachedAnalysisEvaluations(result, MAX_RECORDS)
    .find(record => record.snapshotRevision !== result.snapshotRevision && cachedReviewHasStructuredScore(record)) || null;
}

export function listCachedAnalysisEvaluations(result: AnalysisResult, limit = 8): CachedAnalysisAiEvaluation[] {
  return readCache().records
    .filter(record => record.subjectType === result.subjectType && record.subjectId === result.subjectId)
    .sort((left, right) => Date.parse(right.cachedAt || right.generatedAt) - Date.parse(left.cachedAt || left.generatedAt))
    .slice(0, Math.max(0, limit));
}

export function saveCachedAnalysisEvaluation(
  result: AnalysisResult,
  evaluation: AnalysisAiEvaluation,
  now = new Date()
): CachedAnalysisAiEvaluation | null {
  const storage = browserStorage();
  if (!storage) return null;

  const cache = readCache();
  const record: CachedAnalysisAiEvaluation = {
    ...evaluation,
    cacheVersion: 1,
    subjectType: result.subjectType,
    subjectId: result.subjectId,
    subjectLabel: result.subjectLabel,
    snapshotRevision: result.snapshotRevision,
    cachedAt: now.toISOString(),
  };
  const records = [
    record,
    ...cache.records.filter(item =>
      !(item.subjectType === record.subjectType &&
        item.subjectId === record.subjectId &&
        item.snapshotRevision === record.snapshotRevision)
    ),
  ].slice(0, MAX_RECORDS);

  try {
    storage.setItem(STORAGE_KEY, JSON.stringify({ version: 1, records } satisfies AnalysisEvaluationCachePayload));
    return record;
  } catch {
    return null;
  }
}

export function clearCachedAnalysisEvaluation(result: AnalysisResult) {
  const storage = browserStorage();
  if (!storage) return;
  const cache = readCache();
  const records = cache.records.filter(record =>
    !(record.subjectType === result.subjectType &&
      record.subjectId === result.subjectId &&
      record.snapshotRevision === result.snapshotRevision)
  );
  try {
    storage.setItem(STORAGE_KEY, JSON.stringify({ version: 1, records } satisfies AnalysisEvaluationCachePayload));
  } catch {
    // Cache cleanup should never block analysis.
  }
}

function readCache(): AnalysisEvaluationCachePayload {
  const storage = browserStorage();
  if (!storage) return { version: 1, records: [] };
  try {
    const parsed = JSON.parse(storage.getItem(STORAGE_KEY) || '');
    if (!parsed || parsed.version !== 1 || !Array.isArray(parsed.records)) {
      return { version: 1, records: [] };
    }
    return {
      version: 1,
      records: parsed.records
        .map(normalizeCachedRecord)
        .filter(Boolean)
        .slice(0, MAX_RECORDS),
    };
  } catch {
    return { version: 1, records: [] };
  }
}

function normalizeCachedRecord(value: unknown): CachedAnalysisAiEvaluation | null {
  if (!value || typeof value !== 'object') return null;
  const record = value as Partial<CachedAnalysisAiEvaluation>;
  if (record.cacheVersion !== 1) return null;
  if (!record.subjectType || !record.subjectId || !record.snapshotRevision || !record.evaluation) return null;
  return record as CachedAnalysisAiEvaluation;
}

function cachedReviewHasStructuredScore(record: CachedAnalysisAiEvaluation) {
  const score = record.evaluation?.score;
  return record.evaluation?.structured === true && Boolean(
    score?.health != null ||
    score?.findings?.length ||
    score?.categoryScores?.length
  );
}

function browserStorage(): Storage | null {
  try {
    if (typeof window !== 'undefined' && window.localStorage) return window.localStorage;
    if (typeof globalThis.localStorage !== 'undefined') return globalThis.localStorage;
  } catch {
    return null;
  }
  return null;
}
