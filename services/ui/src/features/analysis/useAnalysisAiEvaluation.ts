import { useCallback, useEffect, useRef, useState } from 'react';
import { requestAnalysisAiEvaluation, type AnalysisAiEvaluation } from './api.js';
import type { AnalysisAiPromptContext } from './ai.js';
import {
  listCachedAnalysisEvaluations,
  loadCachedAnalysisEvaluation,
  loadLatestReusableAnalysisEvaluation,
  saveCachedAnalysisEvaluation,
} from './evaluationCache.js';
import type { AnalysisResult } from './model.js';

export type AnalysisAiEvaluationState =
  | { status: 'idle'; evaluation: null; error: '' }
  | { status: 'loading'; evaluation: null; error: '' }
  | {
      status: 'ready';
      evaluation: AnalysisAiEvaluation;
      error: '';
      source: 'fresh' | 'cache' | 'cache-previous-snapshot';
      cachedAt?: string;
      cachedSnapshotRevision?: string;
    }
  | { status: 'error'; evaluation: null; error: string };

const idleState: AnalysisAiEvaluationState = { status: 'idle', evaluation: null, error: '' };
type StoredAnalysisAiEvaluationState = AnalysisAiEvaluationState & { subjectKey: string; snapshotRevision: string };
export type AnalysisAiEvaluationOptions = {
  loadPromptContext?: () => Promise<AnalysisAiPromptContext | null | undefined>;
};

export function useAnalysisAiEvaluation(result: AnalysisResult, options: AnalysisAiEvaluationOptions = {}) {
  const { loadPromptContext } = options;
  const subjectKey = analysisEvaluationSubjectKey(result);
  const [storedState, setStoredState] = useState<StoredAnalysisAiEvaluationState>(() => initialStoredState(result, subjectKey));
  const cachedState = initialStoredState(result, subjectKey);
  const history = listCachedAnalysisEvaluations(result);
  const requestIDRef = useRef(0);
  const abortRef = useRef<AbortController | null>(null);
  const autoRequestedRevisionRef = useRef('');
  const state: AnalysisAiEvaluationState = storedState.subjectKey === subjectKey &&
    storedState.snapshotRevision === result.snapshotRevision
    ? storedState
    : cachedState;
  const stateSource = state.status === 'ready' ? state.source : '';

  const requestEvaluation = useCallback(async () => {
    const requestID = requestIDRef.current + 1;
    const snapshotRevision = result.snapshotRevision;
    requestIDRef.current = requestID;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setStoredState({ subjectKey, snapshotRevision, status: 'loading', evaluation: null, error: '' });
    try {
      const context = loadPromptContext ? await loadPromptContext() : null;
      if (requestIDRef.current !== requestID || controller.signal.aborted) return;
      const evaluation = await requestAnalysisAiEvaluation(result, context || null, controller.signal);
      if (requestIDRef.current !== requestID) return;
      const cached = saveCachedAnalysisEvaluation(result, evaluation);
      setStoredState({
        subjectKey,
        snapshotRevision,
        status: 'ready',
        evaluation,
        error: '',
        source: 'fresh',
        cachedAt: cached?.cachedAt,
      });
    } catch (error) {
      if (requestIDRef.current !== requestID) return;
      // A run the user cancelled is not a failure to report back to them.
      if (controller.signal.aborted || analysisEvaluationWasAborted(error)) {
        setStoredState({ subjectKey, snapshotRevision, ...idleState });
        return;
      }
      setStoredState({
        subjectKey,
        snapshotRevision,
        status: 'error',
        evaluation: null,
        error: error instanceof Error ? error.message : 'Unable to generate AI evaluation.',
      });
    }
  }, [loadPromptContext, result, subjectKey]);

  // Cancelling stops the request in flight. The model call already started is
  // billed either way, which is why the notice appears before it runs.
  const cancelEvaluation = useCallback(() => {
    if (!abortRef.current) return;
    abortRef.current.abort();
    abortRef.current = null;
    requestIDRef.current += 1;
    setStoredState({ subjectKey, snapshotRevision: result.snapshotRevision, ...idleState });
  }, [result.snapshotRevision, subjectKey]);

  useEffect(() => () => abortRef.current?.abort(), []);

  useEffect(() => {
    if (result.subjectType !== 'run') return;
    if (state.status === 'ready' && stateSource !== 'cache-previous-snapshot') return;
    if (autoRequestedRevisionRef.current === result.snapshotRevision) return;
    autoRequestedRevisionRef.current = result.snapshotRevision;
    const timeout = window.setTimeout(() => void requestEvaluation(), 0);
    return () => window.clearTimeout(timeout);
  }, [requestEvaluation, result.snapshotRevision, result.subjectType, state.status, stateSource]);

  return {
    state,
    requestEvaluation,
    cancelEvaluation,
    autoEvaluates: result.subjectType === 'run',
    history,
  };
}

function analysisEvaluationWasAborted(error: unknown): boolean {
  if (error instanceof DOMException && error.name === 'AbortError') return true;
  return error instanceof Error && /abort/i.test(error.message);
}

function initialStoredState(result: AnalysisResult, subjectKey: string): StoredAnalysisAiEvaluationState {
  const cached = loadCachedAnalysisEvaluation(result);
  if (cached) {
    return {
      subjectKey,
      snapshotRevision: result.snapshotRevision,
      status: 'ready',
      evaluation: cached,
      error: '',
      source: 'cache',
      cachedAt: cached.cachedAt,
      cachedSnapshotRevision: cached.snapshotRevision,
    };
  }
  const latestReusable = loadLatestReusableAnalysisEvaluation(result);
  if (latestReusable) {
    return {
      subjectKey,
      snapshotRevision: result.snapshotRevision,
      status: 'ready',
      evaluation: latestReusable,
      error: '',
      source: 'cache-previous-snapshot',
      cachedAt: latestReusable.cachedAt,
      cachedSnapshotRevision: latestReusable.snapshotRevision,
    };
  }
  return {
    ...idleState,
    subjectKey,
    snapshotRevision: result.snapshotRevision,
  };
}

function analysisEvaluationSubjectKey(result: AnalysisResult) {
  return `${result.subjectType}:${result.subjectId}`;
}
