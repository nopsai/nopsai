const DEV_DEFAULT_PORT = '8080';
const DEV_PORTS = new Set(['5173', '4173']);

const AUTH_TOKEN_KEY = 'nopsai.auth.token';
const REFRESH_TOKEN_KEY = 'nopsai.auth.refresh';
const TENANT_KEY = 'nopsai.auth.tenant';
const DEFAULT_TENANT_KEY = 'nopsai.auth.default_tenant';
const ROLES_KEY = 'nopsai.auth.roles';

export type StoredSession = {
  accessToken?: string;
  refreshToken?: string;
  tenantId?: string;
  defaultTenant?: string;
  roles?: string[];
};

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

export function getStoredSession(): StoredSession {
  if (typeof localStorage === 'undefined') return {};
  const rawRoles = localStorage.getItem(ROLES_KEY);
  let roles: string[] | undefined;
  try {
    roles = rawRoles ? (JSON.parse(rawRoles) as string[]) : undefined;
  } catch {
    roles = undefined;
  }
  return {
    accessToken: localStorage.getItem(AUTH_TOKEN_KEY) || undefined,
    refreshToken: localStorage.getItem(REFRESH_TOKEN_KEY) || undefined,
    tenantId: localStorage.getItem(TENANT_KEY) || undefined,
    defaultTenant: localStorage.getItem(DEFAULT_TENANT_KEY) || undefined,
    roles,
  };
}

function dispatchAuthChanged() {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new Event('nopsai-auth-changed'));
}

export function persistSession(session: { accessToken: string; refreshToken?: string; tenantId?: string; defaultTenant?: string; roles?: string[] }) {
  if (typeof localStorage === 'undefined') return;
  localStorage.setItem(AUTH_TOKEN_KEY, session.accessToken);
  if (session.refreshToken) localStorage.setItem(REFRESH_TOKEN_KEY, session.refreshToken);
  else localStorage.removeItem(REFRESH_TOKEN_KEY);
  const tenant = session.tenantId || session.defaultTenant;
  if (tenant) localStorage.setItem(TENANT_KEY, tenant);
  if (session.defaultTenant) localStorage.setItem(DEFAULT_TENANT_KEY, session.defaultTenant);
  if (session.roles) localStorage.setItem(ROLES_KEY, JSON.stringify(session.roles));
  dispatchAuthChanged();
}

export function clearSession() {
  if (typeof localStorage === 'undefined') return;
  localStorage.removeItem(AUTH_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
  localStorage.removeItem(TENANT_KEY);
  localStorage.removeItem(DEFAULT_TENANT_KEY);
  localStorage.removeItem(ROLES_KEY);
  dispatchAuthChanged();
}

export function setSelectedTenant(tenantId: string) {
  if (typeof localStorage === 'undefined') return;
  localStorage.setItem(TENANT_KEY, tenantId);
  dispatchAuthChanged();
}

export function installAuthInterceptor() {
  if (typeof window === 'undefined') return;
  if ((window as any).__nopsaiAuthFetchInstalled) return;
  (window as any).__nopsaiAuthFetchInstalled = true;
  const originalFetch = window.fetch.bind(window);
  let refreshPromise: Promise<StoredSession | null> | null = null;

  const refreshSession = async (refreshToken?: string): Promise<StoredSession | null> => {
    if (!refreshToken) return null;
    if (!refreshPromise) {
      refreshPromise = (async () => {
        const response = await originalFetch(buildApiUrl('/v1/auth/refresh'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: refreshToken }),
        });
        if (!response.ok) {
          throw new Error(`refresh failed (${response.status})`);
        }
        const payload: any = await response.json();
        const current = getStoredSession();
        const tenantChoice =
          current.tenantId ||
          current.defaultTenant ||
          payload?.default_tenant ||
          (Array.isArray(payload?.tenant_ids) && payload.tenant_ids.length > 0 ? payload.tenant_ids[0] : undefined);

        persistSession({
          accessToken: payload?.access_token || '',
          refreshToken: payload?.refresh_token || refreshToken,
          tenantId: tenantChoice,
          defaultTenant: payload?.default_tenant,
          roles: Array.isArray(payload?.roles) ? payload.roles : current.roles,
        });
        return getStoredSession();
      })()
        .catch(err => {
          clearSession();
          throw err;
        })
        .finally(() => {
          refreshPromise = null;
        });
    }
    return refreshPromise;
  };

  const shouldBypassRefresh = (url: string) =>
    url.includes('/v1/auth/login') || url.includes('/v1/auth/refresh');

  window.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const baseRequest = input instanceof Request ? input : new Request(input, init);
    const url = baseRequest.url;

    const baseHeaders = init?.headers instanceof Headers ? new Headers(init.headers) : new Headers(init?.headers || {});
    baseRequest.headers.forEach((value, key) => {
      if (!baseHeaders.has(key)) baseHeaders.set(key, value);
    });

    const session = getStoredSession();
    if (session.accessToken && !baseHeaders.has('Authorization')) {
      baseHeaders.set('Authorization', `Bearer ${session.accessToken}`);
    }
    const tenantHeader = baseHeaders.get('X-Tenant-ID') || session.tenantId || session.defaultTenant;
    if (tenantHeader) {
      baseHeaders.set('X-Tenant-ID', tenantHeader);
    }

    const finalInit: RequestInit = { ...init, headers: baseHeaders };
    const sendRequest = (headers: Headers = baseHeaders) =>
      originalFetch(new Request(baseRequest, { ...finalInit, headers }));

    let response = await sendRequest();
    if (response.status !== 401 || shouldBypassRefresh(url)) {
      return response;
    }

    try {
      const refreshed = await refreshSession(session.refreshToken || getStoredSession().refreshToken);
      if (!refreshed?.accessToken) {
        clearSession();
        return response;
      }
      const retryHeaders = new Headers(baseHeaders);
      retryHeaders.set('Authorization', `Bearer ${refreshed.accessToken}`);
      response = await sendRequest(retryHeaders);
      if (response.status === 401) {
        clearSession();
      }
      return response;
    } catch {
      clearSession();
      return response;
    }
  };
}

export async function loginLocal(identifier: string, password: string) {
  const response = await fetch(buildApiUrl('/v1/auth/login'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identifier, password }),
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Login failed');
  }
  return response.json();
}
