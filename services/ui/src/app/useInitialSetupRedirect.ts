import { useCallback, useEffect, useRef, useState } from 'react';
import type { NavigateFunction } from 'react-router-dom';
import { fetchSetupStatus } from '../features/system/setup/api';
import type { SetupStatus } from '../features/system/setup/model';
import { isInitialSetupAllowedRoute } from './initialSetupGate.js';
export { isInitialSetupAllowedRoute } from './initialSetupGate.js';

type InitialSetupRedirectOptions = {
  accessToken: string;
  authSubject?: string;
  canViewSystemSetup: boolean;
  currentSubject?: string;
  currentUserLoading: boolean;
  isInitialAdminUser: boolean;
  isAuthenticated: boolean;
  mustChangePassword: boolean;
  pathname: string;
  navigate: NavigateFunction;
};

export type InitialSetupGate = {
  checking: boolean;
  required: boolean;
  complete: boolean;
  error: string | null;
  status: SetupStatus | null;
  recordStatus: (status: SetupStatus) => void;
};

type SetupGatePhase = 'idle' | 'required' | 'complete' | 'error';

type SetupGateState = {
  phase: SetupGatePhase;
  checkKey: string;
  error: string | null;
  status: SetupStatus | null;
};

function gateStateForSetupStatus(status: SetupStatus, checkKey: string): SetupGateState {
  return {
    phase: status.completed ? 'complete' : 'required',
    checkKey,
    error: null,
    status,
  };
}

export function useInitialSetupRedirect({
  accessToken,
  authSubject,
  canViewSystemSetup,
  currentSubject,
  currentUserLoading,
  isInitialAdminUser,
  isAuthenticated,
  mustChangePassword,
  pathname,
  navigate,
}: InitialSetupRedirectOptions): InitialSetupGate {
  const checkedRef = useRef('');
  const [gate, setGate] = useState<SetupGateState>({
    phase: 'idle',
    checkKey: '',
    error: null,
    status: null,
  });

  const shouldCheck =
    isAuthenticated &&
    !mustChangePassword &&
    !currentUserLoading &&
    canViewSystemSetup &&
    isInitialAdminUser &&
    Boolean(accessToken);
  const subject = (currentSubject || authSubject || 'current').trim() || 'current';
  const checkKey = `${subject}:${accessToken}`;

  const recordStatus = useCallback((status: SetupStatus) => {
    setGate(gateStateForSetupStatus(status, checkKey));
  }, [checkKey]);

  useEffect(() => {
    if (!shouldCheck) {
      checkedRef.current = '';
      return;
    }

    if (checkedRef.current === checkKey && gate.checkKey === checkKey && gate.phase !== 'idle') return;
    checkedRef.current = checkKey;

    let cancelled = false;
    void fetchSetupStatus()
      .then(status => {
        if (cancelled) return;
        setGate(gateStateForSetupStatus(status, checkKey));
      })
      .catch(error => {
        if (cancelled) return;
        console.warn('Failed to check setup status', error);
        setGate({
          phase: 'error',
          checkKey,
          error: error instanceof Error ? error.message : 'Failed to check setup status',
          status: null,
        });
      });

    return () => {
      cancelled = true;
    };
  }, [
    accessToken,
    authSubject,
    canViewSystemSetup,
    checkKey,
    currentSubject,
    currentUserLoading,
    gate.checkKey,
    gate.phase,
    isInitialAdminUser,
    isAuthenticated,
    mustChangePassword,
    shouldCheck,
  ]);

  const waitingForIdentity = isAuthenticated && !mustChangePassword && currentUserLoading;
  const currentGate = gate.checkKey === checkKey;
  const required = shouldCheck && currentGate && (gate.phase === 'required' || gate.phase === 'error');
  const checking = waitingForIdentity || (shouldCheck && (!currentGate || gate.phase === 'idle'));

  useEffect(() => {
    if (!required || isInitialSetupAllowedRoute(pathname, mustChangePassword)) return;
    navigate('/system/setup', { replace: true });
  }, [mustChangePassword, navigate, pathname, required]);

  return {
    checking,
    required,
    complete: shouldCheck && currentGate && gate.phase === 'complete',
    error: gate.error,
    status: gate.status,
    recordStatus,
  };
}
