const DEV_DEFAULT_PORT = '8080';
const DEV_PORTS = new Set(['5173', '4173']);

export function getApiBaseUrl(): string {
  const envBase = (import.meta.env.VITE_API_BASE_URL || '').trim().replace(/^['"]+|['"]+$/g, '');
  if (envBase) return envBase.replace(/\/+$/, '');

  if (typeof window !== 'undefined') {
    const { protocol, hostname, port } = window.location;
    const isFileProtocol = protocol === 'file:';
    const hasHost = Boolean(hostname);

    if (port && DEV_PORTS.has(port)) {
      return `${protocol}//${hostname}:${DEV_DEFAULT_PORT}`;
    }
    if (isFileProtocol || !hasHost) {
      return `http://localhost:${DEV_DEFAULT_PORT}`;
    }
    const portSuffix = port ? `:${port}` : '';
    return `${protocol}//${hostname}${portSuffix}`;
  }

  return '';
}

export function buildApiUrl(path: string): string {
  const base = getApiBaseUrl();
  const suffix = path.startsWith('/') ? path : `/${path}`;
  return `${base}${suffix}`;
}
