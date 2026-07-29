import { Navigate } from 'react-router-dom';
import type { ReactNode } from 'react';

export function PermissionGuard({
  allowed,
  loading,
  children,
  fallbackPath = '/pipelineruns/main',
}: {
  allowed: boolean;
  loading: boolean;
  children: ReactNode;
  fallbackPath?: string;
}) {
  if (loading) {
    return <div className="p-4 text-sm text-[var(--text-secondary)]">Loading access...</div>;
  }
  return allowed ? <>{children}</> : <Navigate to={fallbackPath} replace />;
}
