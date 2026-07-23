export function isInitialSetupAllowedRoute(pathname: string, mustChangePassword: boolean): boolean {
  const path = pathname.trim() || '/';
  return path === '/system/setup' || (mustChangePassword && path === '/profile');
}
