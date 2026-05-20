const DEV_DEFAULT_PORT = '8080';
const DEV_PORTS = new Set(['5173', '4173']);

const AUTH_TOKEN_KEY = 'nopsai.auth.token';
const REFRESH_TOKEN_KEY = 'nopsai.auth.refresh';
const ROLES_KEY = 'nopsai.auth.roles';
const SUB_KEY = 'nopsai.auth.sub';
const PASSWORD_CHANGE_REQUIRED_KEY = 'nopsai.auth.mustChangePassword';

export type StoredSession = {
  accessToken?: string;
  refreshToken?: string;
  roles?: string[];
  sub?: string;
  mustChangePassword?: boolean;
};

export function getApiBaseUrl(): string {
  const configuredBase = (import.meta.env.VITE_API_BASE_URL || '').trim().replace(/^['"]+|['"]+$/g, '');
  if (configuredBase) return configuredBase.replace(/\/+$/, '');

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
    roles,
    sub: localStorage.getItem(SUB_KEY) || undefined,
    mustChangePassword: localStorage.getItem(PASSWORD_CHANGE_REQUIRED_KEY) === 'true',
  };
}

function dispatchAuthChanged() {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new Event('nopsai-auth-changed'));
}

export function persistSession(session: {
  accessToken: string;
  refreshToken?: string;
  roles?: string[];
  sub?: string;
  mustChangePassword?: boolean;
}) {
  if (typeof localStorage === 'undefined') return;
  localStorage.setItem(AUTH_TOKEN_KEY, session.accessToken);
  if (session.refreshToken) localStorage.setItem(REFRESH_TOKEN_KEY, session.refreshToken);
  else localStorage.removeItem(REFRESH_TOKEN_KEY);
  if (session.roles) localStorage.setItem(ROLES_KEY, JSON.stringify(session.roles));
  else localStorage.removeItem(ROLES_KEY);
  if (session.sub) localStorage.setItem(SUB_KEY, session.sub);
  else localStorage.removeItem(SUB_KEY);
  if (session.mustChangePassword) localStorage.setItem(PASSWORD_CHANGE_REQUIRED_KEY, 'true');
  else localStorage.removeItem(PASSWORD_CHANGE_REQUIRED_KEY);
  dispatchAuthChanged();
}

export function setPasswordChangeRequired(required: boolean) {
  if (typeof localStorage === 'undefined') return;
  const current = localStorage.getItem(PASSWORD_CHANGE_REQUIRED_KEY) === 'true';
  if (current === required) return;
  if (required) localStorage.setItem(PASSWORD_CHANGE_REQUIRED_KEY, 'true');
  else localStorage.removeItem(PASSWORD_CHANGE_REQUIRED_KEY);
  dispatchAuthChanged();
}

export function clearSession() {
  if (typeof localStorage === 'undefined') return;
  localStorage.removeItem(AUTH_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
  localStorage.removeItem(ROLES_KEY);
  localStorage.removeItem(SUB_KEY);
  localStorage.removeItem(PASSWORD_CHANGE_REQUIRED_KEY);
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

        persistSession({
          accessToken: payload?.access_token || '',
          refreshToken: payload?.refresh_token || refreshToken,
          roles: Array.isArray(payload?.roles) ? payload.roles : current.roles,
          sub: typeof payload?.sub === 'string' ? payload.sub : current.sub,
          mustChangePassword:
            typeof payload?.must_change_password === 'boolean'
              ? payload.must_change_password
              : current.mustChangePassword,
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
    url.includes('/v1/auth/login') ||
    url.includes('/v1/auth/refresh') ||
    url.includes('/v1/auth/password');

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

export async function updateEmail(email: string): Promise<{ email: string }> {
  const response = await fetch(buildApiUrl('/v1/auth/email'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Failed to update email');
  }
  const payload = await response.json();
  const nextEmail = typeof payload?.email === 'string' ? payload.email : email;
  return { email: nextEmail };
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  const response = await fetch(buildApiUrl('/v1/auth/password'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Failed to change password');
  }
}

export type PersonalAccessToken = {
  id: string;
  name: string;
  token?: string;
  token_suffix: string;
  created_at: string;
  expires_at?: string;
  last_used_at?: string;
};

export type CreatePersonalAccessTokenOptions = {
  expiresInDays?: number;
  expiresAt?: string;
  neverExpires?: boolean;
};

export async function listPersonalAccessTokens(): Promise<PersonalAccessToken[]> {
  const response = await fetch(buildApiUrl('/v1/auth/personal-tokens'));
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Failed to load personal tokens');
  }
  const payload = await response.json();
  return Array.isArray(payload) ? payload : [];
}

export async function createPersonalAccessToken(name: string, options: CreatePersonalAccessTokenOptions): Promise<PersonalAccessToken> {
  const response = await fetch(buildApiUrl('/v1/auth/personal-tokens'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name,
      expires_in_days: options.expiresInDays,
      expires_at: options.expiresAt,
      never_expires: options.neverExpires,
    }),
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Failed to create personal token');
  }
  return response.json();
}

export async function revokePersonalAccessToken(tokenID: string): Promise<void> {
  const response = await fetch(buildApiUrl(`/v1/auth/personal-tokens/${encodeURIComponent(tokenID)}`), {
    method: 'DELETE',
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Failed to revoke personal token');
  }
}
