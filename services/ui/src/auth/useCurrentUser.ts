import { useCallback, useEffect, useMemo, useState } from 'react';
import type { AuthSession, CurrentUser } from '../app/types';
import { apiClient, clearSession, getStoredSession, setPasswordChangeRequired } from '../lib/api';
import { normalizeCurrentUser } from './capabilities';

export type CurrentUserSession = {
  authSession: AuthSession;
  currentUser: CurrentUser | null;
  currentUserLoading: boolean;
  isAuthenticated: boolean;
  refreshAuthSession: () => void;
  clearAuthSession: () => void;
  updateCurrentUser: (updates: Partial<CurrentUser>) => void;
  markPasswordChanged: () => void;
};

export function useCurrentUser(): CurrentUserSession {
  const [authSession, setAuthSession] = useState<AuthSession>(() => getStoredSession());
  const [currentUser, setCurrentUser] = useState<CurrentUser | null>(null);
  const [currentUserLoading, setCurrentUserLoading] = useState(() => Boolean(getStoredSession().accessToken));
  const isAuthenticated = useMemo(() => Boolean(authSession.accessToken), [authSession.accessToken]);

  const refreshAuthSession = useCallback(() => {
    setAuthSession(getStoredSession());
  }, []);

  const clearAuthSession = useCallback(() => {
    clearSession();
    setAuthSession({});
    setCurrentUser(null);
    setCurrentUserLoading(false);
  }, []);

  const updateCurrentUser = useCallback((updates: Partial<CurrentUser>) => {
    setCurrentUser(prev => (prev ? { ...prev, ...updates } : prev));
  }, []);

  const markPasswordChanged = useCallback(() => {
    setPasswordChangeRequired(false);
    setAuthSession(getStoredSession());
  }, []);

  useEffect(() => {
    const handleAuthChange = () => {
      setAuthSession(getStoredSession());
    };
    window.addEventListener('storage', handleAuthChange);
    window.addEventListener('nopsai-auth-changed', handleAuthChange as EventListener);
    return () => {
      window.removeEventListener('storage', handleAuthChange);
      window.removeEventListener('nopsai-auth-changed', handleAuthChange as EventListener);
    };
  }, []);

  useEffect(() => {
    if (!authSession.accessToken) {
      const handle = window.setTimeout(() => {
        setCurrentUser(null);
        setCurrentUserLoading(false);
      }, 0);
      return () => window.clearTimeout(handle);
    }

    let cancelled = false;
    const loadingHandle = window.setTimeout(() => {
      if (!cancelled) setCurrentUserLoading(true);
    }, 0);

    apiClient.fetch('/v1/auth/me')
      .then(resp => {
        if (resp.status === 401 || resp.status === 403 || resp.status === 404) throw new Error('session-invalid');
        if (!resp.ok) throw new Error(`Failed to load user (${resp.status})`);
        return resp.json();
      })
      .then(data => {
        if (cancelled) return;
        const record = data && typeof data === 'object' ? (data as Record<string, unknown>) : {};
        setCurrentUser(normalizeCurrentUser(data));
        setPasswordChangeRequired(Boolean(record.must_change_password));
      })
      .catch(err => {
        if (cancelled) return;
        setCurrentUser(null);
        if (err instanceof Error && err.message === 'session-invalid') {
          clearSession();
          setAuthSession(getStoredSession());
        }
      })
      .finally(() => {
        if (!cancelled) setCurrentUserLoading(false);
      });

    return () => {
      cancelled = true;
      window.clearTimeout(loadingHandle);
    };
  }, [authSession.accessToken]);

  return {
    authSession,
    currentUser,
    currentUserLoading,
    isAuthenticated,
    refreshAuthSession,
    clearAuthSession,
    updateCurrentUser,
    markPasswordChanged,
  };
}
