import { createContext, useContext } from 'react';
import type { CurrentUserSession } from './useCurrentUser';

export const AuthContext = createContext<CurrentUserSession | null>(null);

export function useAuth(): CurrentUserSession {
  const auth = useContext(AuthContext);
  if (!auth) throw new Error('useAuth must be used within AuthProvider');
  return auth;
}
