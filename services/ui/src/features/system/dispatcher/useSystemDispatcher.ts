import { useCallback, useEffect, useRef, useState } from 'react';
import {
  getRunnerMeta,
  runnerActionKey,
  type DispatcherStatusState,
  type Runner,
} from './model';
import { fetchSystemJson } from '../api';
import { fetchDispatcherStatus } from './api';

type ToastTone = 'success' | 'error' | 'info';

type UseSystemDispatcherOptions = {
  enabled: boolean;
  locationSearch: string;
  addToast: (message: string, tone?: ToastTone) => void;
};

type SystemDispatcherPanelState = {
  loading: boolean;
  error: string | null;
  status: DispatcherStatusState | null;
  pendingActions: Set<string>;
  pendingEjections: Set<string>;
  onRefresh: () => void;
  onToggleRunnerDispatch: (runner: Runner) => Promise<void>;
  onEjectRunner: (runner: Runner) => Promise<void>;
};

const POLL_INTERVAL_MS = 5000;
const RUNNER_DEPLOYMENT_GUIDE_QUERY = 'guide';
const RUNNER_DEPLOYMENT_GUIDE_VALUE = 'runner';
const RUNNER_DEPLOYMENT_GUIDE_ID = 'dispatcher-runner-deployment-guide';

function scrollRunnerDeploymentGuide() {
  window.setTimeout(() => {
    document.getElementById(RUNNER_DEPLOYMENT_GUIDE_ID)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }, 0);
}

export function useSystemDispatcher({
  enabled,
  locationSearch,
  addToast,
}: UseSystemDispatcherOptions): SystemDispatcherPanelState {
  const isMountedRef = useRef(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<DispatcherStatusState | null>(null);
  const [pendingActions, setPendingActions] = useState<Set<string>>(new Set());
  const [pendingEjections, setPendingEjections] = useState<Set<string>>(new Set());

  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  const loadStatus = useCallback(async (opts?: { quiet?: boolean }) => {
    if (!opts?.quiet) {
      setError(null);
      setLoading(true);
    }
    try {
      const normalized = await fetchDispatcherStatus();
      if (isMountedRef.current) {
        setStatus(normalized);
        setError(null);
      }
    } catch (loadError) {
      console.error('Failed to load dispatcher status', loadError);
      if (!isMountedRef.current) return;
      setError(loadError instanceof Error ? loadError.message : 'Unable to load dispatcher status');
    } finally {
      if (isMountedRef.current && !opts?.quiet) {
        setLoading(false);
      }
    }
  }, []);

  const setRunnerPending = useCallback((runnerId: string, connectionId: string, pending: boolean) => {
    const key = runnerActionKey(runnerId, connectionId);
    if (!key) return;
    setPendingActions(prev => {
      const next = new Set(prev);
      if (pending) next.add(key);
      else next.delete(key);
      return next;
    });
  }, []);

  const setRunnerEjectionPending = useCallback((runnerId: string, connectionId: string, pending: boolean) => {
    const key = runnerActionKey(runnerId, connectionId);
    if (!key) return;
    setPendingEjections(prev => {
      const next = new Set(prev);
      if (pending) next.add(key);
      else next.delete(key);
      return next;
    });
  }, []);

  const toggleRunnerDispatch = useCallback(
    async (runner: Runner) => {
      const meta = getRunnerMeta(runner);
      const key = runnerActionKey(runner.runnerId, meta.connectionId);
      if (!key || pendingActions.has(key) || pendingEjections.has(key)) return;

      const nextAllow = !runner.allowDispatch;
      setRunnerPending(runner.runnerId, meta.connectionId, true);
      try {
        await fetchSystemJson(`/v1/system/dispatcher/runners/${encodeURIComponent(runner.runnerId)}/dispatch`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            allow_dispatch: nextAllow,
            ...(meta.connectionId ? { connection_id: meta.connectionId } : {}),
          }),
        });
        await loadStatus({ quiet: true });
      } catch (updateError) {
        console.error('Failed to toggle runner dispatch', updateError);
        addToast('Failed to update runner dispatch.', 'error');
      } finally {
        setRunnerPending(runner.runnerId, meta.connectionId, false);
      }
    },
    [addToast, loadStatus, pendingActions, pendingEjections, setRunnerPending]
  );

  const ejectRunner = useCallback(
    async (runner: Runner) => {
      const meta = getRunnerMeta(runner);
      const key = runnerActionKey(runner.runnerId, meta.connectionId);
      if (!key || pendingActions.has(key) || pendingEjections.has(key)) return;

      const confirmed = window.confirm(
        `Remove and revoke runner "${runner.runnerId}"? This disconnects any live runner stream, requeues in-flight work, and blocks this runner ID from reconnecting until a replacement install is generated or the revoked ID is cleared.`
      );
      if (!confirmed) return;

      setRunnerEjectionPending(runner.runnerId, meta.connectionId, true);
      try {
        await fetchSystemJson(`/v1/system/dispatcher/runners/${encodeURIComponent(runner.runnerId)}`, {
          method: 'DELETE',
        });
        await loadStatus({ quiet: true });
        addToast('Runner registration removed and ID revoked.', 'success');
      } catch (deleteError) {
        console.error('Failed to remove runner registration', deleteError);
        addToast('Failed to remove runner registration.', 'error');
      } finally {
        setRunnerEjectionPending(runner.runnerId, meta.connectionId, false);
      }
    },
    [addToast, loadStatus, pendingActions, pendingEjections, setRunnerEjectionPending]
  );

  const refresh = useCallback(() => {
    void loadStatus();
  }, [loadStatus]);

  useEffect(() => {
    if (!enabled) return;
    void loadStatus();
  }, [enabled, loadStatus]);

  useEffect(() => {
    if (!enabled) return;
    const search = new URLSearchParams(locationSearch);
    if (search.get(RUNNER_DEPLOYMENT_GUIDE_QUERY) !== RUNNER_DEPLOYMENT_GUIDE_VALUE) return;
    scrollRunnerDeploymentGuide();
  }, [enabled, locationSearch]);

  useEffect(() => {
    if (!enabled) return undefined;
    const handle = window.setInterval(() => {
      void loadStatus({ quiet: true });
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(handle);
  }, [enabled, loadStatus]);

  return {
    loading,
    error,
    status,
    pendingActions,
    pendingEjections,
    onRefresh: refresh,
    onToggleRunnerDispatch: toggleRunnerDispatch,
    onEjectRunner: ejectRunner,
  };
}
