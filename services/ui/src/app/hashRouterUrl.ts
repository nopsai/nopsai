export type HashRouterLocationParts = {
  pathname: string;
  search: string;
  hash: string;
};

export function canonicalHashRouterURL(location: HashRouterLocationParts): string | null {
  const hash = location.hash || '';
  if (!hash.startsWith('#/')) return null;
  const pathname = location.pathname || '/';
  if (pathname === '/' && !location.search) return null;
  return `/${hash}`;
}

export function canonicalizeHashRouterURL() {
  if (typeof window === 'undefined') return;
  const canonical = canonicalHashRouterURL(window.location);
  if (!canonical) return;
  window.history.replaceState(window.history.state, '', canonical);
}
