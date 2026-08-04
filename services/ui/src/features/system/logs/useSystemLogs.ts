import { useCallback, useEffect, useRef, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { fetchSystemLogSources, streamSystemLogs } from './api.js';
import type { SystemLogConnectionState, SystemLogEntry, SystemLogSource } from './types.js';

const MAX_CLIENT_ENTRIES = 5_000;
const DEFAULT_TAIL_LINES = 500;

const appendBounded = (current: SystemLogEntry[], incoming: SystemLogEntry[]) =>
  [...current, ...incoming].slice(-MAX_CLIENT_ENTRIES);

function requestedSourceID(locationSearch: string) {
  return new URLSearchParams(locationSearch).get('source')?.trim() || '';
}

export function useSystemLogs() {
  const location = useLocation();
  const [sources, setSources] = useState<SystemLogSource[]>([]);
  const [selectedSourceID, setSelectedSourceIDState] = useState('');
  const [entries, setEntries] = useState<SystemLogEntry[]>([]);
  const [connectionState, setConnectionState] = useState<SystemLogConnectionState>('idle');
  const [error, setError] = useState<string | null>(null);
  const [redactionWarning, setRedactionWarning] = useState('');
  const [live, setLive] = useState(true);
  const [paused, setPaused] = useState(false);
  const [unseenCount, setUnseenCount] = useState(0);
  const [cursorExpired, setCursorExpired] = useState(false);
  const [reconnectAttempt, setReconnectAttempt] = useState(0);
  const cursorRef = useRef('');
  const pausedRef = useRef(false);
  const pendingRef = useRef<SystemLogEntry[]>([]);

  const loadSources = useCallback(async () => {
    try {
      const payload = await fetchSystemLogSources();
      setSources(payload.sources);
      setRedactionWarning(payload.redaction_warning);
      setError(null);
      setSelectedSourceIDState(current => {
        const requested = requestedSourceID(location.search);
        if (requested && payload.sources.some(source => source.id === requested)) return requested;
        if (payload.sources.some(source => source.id === current)) return current;
        return payload.sources.find(source => source.available)?.id || payload.sources[0]?.id || '';
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load system log sources');
    }
  }, [location.search]);

  useEffect(() => {
    const timeout = window.setTimeout(() => { void loadSources(); }, 0);
    return () => window.clearTimeout(timeout);
  }, [loadSources]);

  const acceptEntry = useCallback((entry: SystemLogEntry) => {
    cursorRef.current = entry.id;
    if (pausedRef.current) {
      pendingRef.current = appendBounded(pendingRef.current, [entry]);
      setUnseenCount(pendingRef.current.length);
      return;
    }
    setEntries(current => appendBounded(current, [entry]));
  }, []);

  useEffect(() => {
    if (!live || !selectedSourceID) {
      return;
    }
    const controller = new AbortController();
    let stopped = false;
    const connect = async () => {
      let attempt = 0;
      while (!stopped && !controller.signal.aborted) {
        setConnectionState(attempt === 0 ? 'connecting' : 'reconnecting');
        setReconnectAttempt(attempt);
        try {
          await streamSystemLogs({
            sourceID: selectedSourceID,
            cursor: cursorRef.current || undefined,
            tail: DEFAULT_TAIL_LINES,
            signal: controller.signal,
            onEvent: event => {
              if (event.event === 'status') {
                setConnectionState('connected');
                setError(null);
              } else if (event.event === 'reset') {
                cursorRef.current = '';
                setCursorExpired(true);
              } else {
                acceptEntry(event.data);
              }
            },
          });
          if (controller.signal.aborted) return;
        } catch (cause) {
          if (controller.signal.aborted) return;
          setError(cause instanceof Error ? cause.message : 'System log stream disconnected');
          setConnectionState('error');
        }
        attempt += 1;
        const delay = Math.min(10_000, 500 * 2 ** Math.min(attempt, 5));
        await new Promise<void>(resolve => {
          const onAbort = () => {
            window.clearTimeout(timeout);
            resolve();
          };
          const timeout = window.setTimeout(() => {
            controller.signal.removeEventListener('abort', onAbort);
            resolve();
          }, delay);
          controller.signal.addEventListener('abort', onAbort, { once: true });
        });
      }
    };
    void connect();
    return () => {
      stopped = true;
      controller.abort();
    };
  }, [acceptEntry, live, selectedSourceID]);

  const selectSource = useCallback((sourceID: string) => {
    cursorRef.current = '';
    pendingRef.current = [];
    pausedRef.current = false;
    setPaused(false);
    setUnseenCount(0);
    setCursorExpired(false);
    setEntries([]);
    setSelectedSourceIDState(sourceID);
  }, []);

  const togglePaused = useCallback(() => {
    if (pausedRef.current) {
      pausedRef.current = false;
      setPaused(false);
      setEntries(current => appendBounded(current, pendingRef.current));
      pendingRef.current = [];
      setUnseenCount(0);
      return;
    }
    pausedRef.current = true;
    setPaused(true);
  }, []);

  const clear = useCallback(() => {
    pendingRef.current = [];
    setEntries([]);
    setUnseenCount(0);
    setCursorExpired(false);
  }, []);

  const updateLive = useCallback((enabled: boolean) => {
    setLive(enabled);
    if (!enabled) setConnectionState('idle');
  }, []);

  return {
    sources,
    selectedSourceID,
    entries,
    connectionState,
    error,
    redactionWarning,
    live,
    paused,
    unseenCount,
    cursorExpired,
    reconnectAttempt,
    selectSource,
    setLive: updateLive,
    togglePaused,
    clear,
    refreshSources: loadSources,
  };
}
