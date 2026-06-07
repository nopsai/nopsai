import type { ReactNode } from 'react';
import { AuthContext } from './AuthContext';
import { useCurrentUser } from './useCurrentUser';

export function AuthProvider({ children }: { children: ReactNode }) {
  const auth = useCurrentUser();
  return <AuthContext.Provider value={auth}>{children}</AuthContext.Provider>;
}
