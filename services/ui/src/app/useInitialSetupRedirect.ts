import { useEffect, useRef } from 'react';
import type { NavigateFunction } from 'react-router-dom';
import { fetchSetupStatus } from '../features/system/setup/api';
import { SETUP_REDIRECTED_KEY } from './constants';

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
}: InitialSetupRedirectOptions) {
  const checkedRef = useRef('');

  useEffect(() => {
    if (!isAuthenticated || mustChangePassword || currentUserLoading || !canViewSystemSetup) return;
    if (!isInitialAdminUser || pathname === '/system/setup') return;

    const subject = (currentSubject || authSubject || 'current').trim() || 'current';
    const checkKey = `${subject}:${accessToken}`;
    if (checkedRef.current === checkKey) return;
    checkedRef.current = checkKey;

    let cancelled = false;
    void fetchSetupStatus()
      .then(status => {
        if (cancelled || status.completed) return;
        const redirectKey = `${SETUP_REDIRECTED_KEY}.${subject}`;
        if (sessionStorage.getItem(redirectKey) === 'true') return;
        sessionStorage.setItem(redirectKey, 'true');
        navigate('/system/setup', { replace: true });
      })
      .catch(error => {
        console.warn('Failed to check setup status', error);
      });

    return () => {
      cancelled = true;
    };
  }, [
    accessToken,
    authSubject,
    canViewSystemSetup,
    currentSubject,
    currentUserLoading,
    isInitialAdminUser,
    isAuthenticated,
    mustChangePassword,
    navigate,
    pathname,
  ]);
}
