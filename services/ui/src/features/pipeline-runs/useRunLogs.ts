import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { fetchRunLogs } from './api';
import {
  RUN_LOG_LEVELS,
  buildRunLogsHash,
  enrichRunLogLines,
  filterRunLogLines,
  getPresentRunLogLevels,
  parseRunLogsHash,
  type EnrichedRunLogLine,
  type RunLogLine,
} from './runLogs';

type RunLogsOptions = {
  runID: string;
  initialStep?: string | null;
  initialSearch?: string | null;
};

export const RUN_LOG_VISIBLE_POLL_MS = 1000;
export const RUN_LOG_HIDDEN_POLL_MS = 15000;

export function useRunLogs({ runID, initialStep, initialSearch }: RunLogsOptions) {
  const [lines, setLines] = useState<EnrichedRunLogLine[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedSteps, setSelectedSteps] = useState<Set<string>>(
    () => (initialStep && initialStep !== 'all' ? new Set([initialStep]) : new Set())
  );
  const [selectedLevels, setSelectedLevels] = useState<Set<string>>(new Set());
  const [searchText, setSearchText] = useState('');
  const [follow, setFollowState] = useState(true);
  const [wrap, setWrap] = useState(false);
  const [structured, setStructured] = useState(false);
  const [shortView, setShortView] = useState(true);
  const [agentOnly, setAgentOnly] = useState(false);
  const [hasUnseen, setHasUnseen] = useState(false);
  const lastIDRef = useRef(0);
  const followRef = useRef(true);
  const shortViewRef = useRef(true);

  const setFollow = useCallback((value: boolean | ((current: boolean) => boolean)) => {
    setFollowState(current => {
      const next = typeof value === 'function' ? value(current) : value;
      followRef.current = next;
      return next;
    });
  }, []);

  useEffect(() => {
    const parsed = parseRunLogsHash(window.location.hash, runID);
    setSelectedSteps(
      parsed?.steps.length
        ? new Set(parsed.steps)
        : initialStep && initialStep !== 'all'
          ? new Set([initialStep])
          : new Set()
    );
    setSelectedLevels(parsed?.levels ?? new Set());
    setSearchText(initialSearch ?? parsed?.search ?? '');
    setFollow(true);
    setWrap(parsed?.wrap ?? false);
    setStructured(parsed?.structured ?? false);
    setShortView(parsed?.shortView ?? true);
    setAgentOnly(parsed?.agentOnly ?? false);
    setLines([]);
    setError(null);
    lastIDRef.current = 0;
    setHasUnseen(false);
  }, [initialSearch, initialStep, runID, setFollow]);

  useEffect(() => {
    const nextHash = buildRunLogsHash({
      currentHash: window.location.hash,
      runID,
      selectedSteps,
      selectedLevels,
      wrap,
      structured,
      agentOnly,
      shortView,
      searchText,
    });
    if (!nextHash || window.location.hash === nextHash) return;
    try {
      const url = new URL(window.location.href);
      url.hash = nextHash.slice(1);
      history.replaceState(null, '', url.toString());
    } catch {
      window.location.hash = nextHash;
    }
  }, [agentOnly, runID, searchText, selectedLevels, selectedSteps, shortView, structured, wrap]);

  useEffect(() => {
    const wasShortView = shortViewRef.current;
    shortViewRef.current = shortView;
    if (shortView && !wasShortView) {
      setWrap(false);
      setStructured(false);
    }
  }, [shortView]);

  useEffect(() => {
    let cancelled = false;
    let timer: number | null = null;

    const fetchLogs = async () => {
      setLoading(true);
      setError(null);
      try {
        const payload = await fetchRunLogs<RunLogLine>(runID, lastIDRef.current);
        if (cancelled || !payload.length) return;
        lastIDRef.current = payload[payload.length - 1].id;
        setLines(current => [...current, ...enrichRunLogLines(payload)]);
        if (!followRef.current) setHasUnseen(true);
      } catch (reason) {
        if (!cancelled) setError(reason instanceof Error ? reason.message : 'Failed to load logs');
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    const tick = async () => {
      await fetchLogs();
      if (!cancelled) timer = window.setTimeout(tick, document.hidden ? RUN_LOG_HIDDEN_POLL_MS : RUN_LOG_VISIBLE_POLL_MS);
    };

    void tick();
    return () => {
      cancelled = true;
      if (timer !== null) window.clearTimeout(timer);
    };
  }, [runID]);

  const presentLevels = useMemo(() => getPresentRunLogLevels(lines), [lines]);
  const visibleLines = useMemo(
    () => filterRunLogLines(lines, { selectedSteps, selectedLevels, agentOnly, searchText }),
    [agentOnly, lines, searchText, selectedLevels, selectedSteps]
  );

  const toggleLevel = useCallback((level: string) => {
    setSelectedLevels(current => {
      const next = new Set(current);
      if (next.has(level)) next.delete(level);
      else next.add(level);
      return next.size === RUN_LOG_LEVELS.length ? new Set() : next;
    });
  }, []);

  const toggleStep = useCallback((step: string) => {
    setSelectedSteps(current => {
      const next = new Set(current);
      if (next.has(step)) next.delete(step);
      else next.add(step);
      return next;
    });
  }, []);

  const resetFilters = useCallback(() => {
    setSelectedSteps(new Set());
    setSelectedLevels(new Set());
    setAgentOnly(false);
    setSearchText('');
    setShortView(true);
    setWrap(false);
    setStructured(false);
    setHasUnseen(false);
  }, []);

  return {
    agentOnly,
    error,
    follow,
    hasUnseen,
    levelOptions: RUN_LOG_LEVELS,
    lines,
    loading,
    presentLevels,
    searchText,
    selectedLevels,
    selectedSteps,
    shortView,
    structured,
    visibleLines,
    wrap,
    resetFilters,
    setAgentOnly,
    setFollow,
    setHasUnseen,
    setSearchText,
    setSelectedSteps,
    setShortView,
    setStructured,
    setWrap,
    toggleLevel,
    toggleStep,
  };
}
