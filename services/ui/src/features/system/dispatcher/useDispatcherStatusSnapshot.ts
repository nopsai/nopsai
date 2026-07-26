import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchDispatcherStatus } from './api';
import type { DispatcherStatusState } from './model';

type UseDispatcherStatusSnapshotOptions = {
  enabled?: boolean;
  pollMs?: number;
};

export function useDispatcherStatusSnapshot({
  enabled = true,
  pollMs = 15000,
}: UseDispatcherStatusSnapshotOptions = {}) {
  const mountedRef = useRef(true);
  const [status, setStatus] = useState<DispatcherStatusState | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const refresh = useCallback(async (quiet = false) => {
    if (!quiet) {
      setLoading(true);
      setError(null);
    }
    try {
      const next = await fetchDispatcherStatus();
      if (!mountedRef.current) return;
      setStatus(next);
      setError(null);
    } catch (loadError) {
      if (!mountedRef.current) return;
      setError(loadError instanceof Error ? loadError.message : 'Unable to load runner assignments');
    } finally {
      if (mountedRef.current && !quiet) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;
    void refresh();
  }, [enabled, refresh]);

  useEffect(() => {
    if (!enabled || pollMs <= 0) return undefined;
    const handle = window.setInterval(() => {
      void refresh(true);
    }, pollMs);
    return () => window.clearInterval(handle);
  }, [enabled, pollMs, refresh]);

  return { status, loading, error, refresh };
}
