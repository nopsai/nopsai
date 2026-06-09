// Update this default when the product documentation route or external docs site exists.
const DOCUMENTATION_BASE_URL = '';

export function buildDocumentationHref(docsPath: string, baseURL = DOCUMENTATION_BASE_URL) {
  const base = baseURL.trim().replace(/\/+$/, '');
  if (!base) return '';
  return `${base}/${docsPath.replace(/^\/+/, '')}`;
}

export function resolveHelpTopicKey(pathname: string) {
  const segments = pathname.split('/').filter(Boolean);
  const primary = segments[0] || '';
  const secondary = segments[1] || '';

  if (primary === 'pipelineruns') return `pipelineruns/${secondary || 'main'}`;
  if (primary === 'system') return `system/${secondary || 'config'}`;
  if (primary === 'knowledge-context') return 'knowledge-context';
  return primary || 'default';
}
