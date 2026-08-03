const DEV_DEFAULT_PORT = '8080';
const DEV_PORTS = new Set(['5173', '4173']);

const AUTH_TOKEN_KEY = 'nopsai.auth.token';
const REFRESH_TOKEN_KEY = 'nopsai.auth.refresh';
const ROLES_KEY = 'nopsai.auth.roles';
const SUB_KEY = 'nopsai.auth.sub';
const PASSWORD_CHANGE_REQUIRED_KEY = 'nopsai.auth.mustChangePassword';
const FORCE_SSO_PROMPT_KEY = 'nopsai.auth.forceSsoPrompt';

export type StoredSession = {
  accessToken?: string;
  refreshToken?: string;
  roles?: string[];
  sub?: string;
  mustChangePassword?: boolean;
};

type AuthRefreshResponse = {
  access_token?: string;
  refresh_token?: string;
  roles?: unknown;
  sub?: unknown;
  must_change_password?: unknown;
};

type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export type ApiRequestInit = RequestInit & {
  auth?: boolean;
  retryOnUnauthorized?: boolean;
};

export function getApiBaseUrl(): string {
  const metaEnv = (import.meta as ImportMeta & { env?: Record<string, string | undefined> }).env || {};
  const configuredBase = (metaEnv.VITE_API_BASE_URL || '').trim().replace(/^['"]+|['"]+$/g, '');
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
  localStorage.removeItem(FORCE_SSO_PROMPT_KEY);
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

export function requirePromptForNextSSOLogin() {
  if (typeof localStorage === 'undefined') return;
  localStorage.setItem(FORCE_SSO_PROMPT_KEY, 'true');
}

export function consumeNextSSOLoginPrompt(): boolean {
  if (typeof localStorage === 'undefined') return false;
  const required = localStorage.getItem(FORCE_SSO_PROMPT_KEY) === 'true';
  localStorage.removeItem(FORCE_SSO_PROMPT_KEY);
  return required;
}

export class ApiClient {
  private readonly fetchImpl: FetchLike;
  private refreshPromise: Promise<StoredSession | null> | null = null;

  constructor(fetchImpl: FetchLike = (input, init) => fetch(input, init)) {
    this.fetchImpl = fetchImpl;
  }

  async fetch(input: RequestInfo | URL | string, init: ApiRequestInit = {}): Promise<Response> {
    const { auth = true, retryOnUnauthorized = true, ...requestInit } = init;
    const url = this.resolveUrl(input);
    const headers = this.buildHeaders(input, requestInit.headers);
    const session = getStoredSession();
    const authEligible = auth && this.shouldAttachAuth(input, url);

    if (authEligible && session.accessToken && !headers.has('Authorization')) {
      headers.set('Authorization', `Bearer ${session.accessToken}`);
    }

    const sendRequest = (requestHeaders: Headers) =>
      this.fetchImpl(url, {
        ...this.copyRequestDefaults(input),
        ...requestInit,
        headers: requestHeaders,
      });

    let response = await sendRequest(headers);
    if (!authEligible || !retryOnUnauthorized || response.status !== 401 || this.shouldBypassRefresh(url)) {
      return response;
    }

    try {
      const refreshed = await this.refreshSession(session.refreshToken || getStoredSession().refreshToken);
      if (!refreshed?.accessToken) {
        clearSession();
        return response;
      }
      const retryHeaders = new Headers(headers);
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
  }

  async json<T>(input: RequestInfo | URL | string, init: ApiRequestInit = {}): Promise<T> {
    const response = await this.fetch(input, init);
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || `Request failed (${response.status})`);
    }
    if (response.status === 204) return undefined as T;
    return response.json() as Promise<T>;
  }

  private resolveUrl(input: RequestInfo | URL | string): string {
    if (this.isRequest(input)) return input.url;
    const raw = input instanceof URL ? input.toString() : input;
    if (/^https?:\/\//i.test(raw)) return raw;
    return buildApiUrl(raw);
  }

  private buildHeaders(input: RequestInfo | URL | string, initHeaders?: HeadersInit): Headers {
    const headers = this.isRequest(input) ? new Headers(input.headers) : new Headers();
    new Headers(initHeaders || {}).forEach((value, key) => headers.set(key, value));
    return headers;
  }

  private copyRequestDefaults(input: RequestInfo | URL | string): RequestInit {
    if (!this.isRequest(input)) return {};
    return {
      method: input.method,
      mode: input.mode,
      credentials: input.credentials,
      cache: input.cache,
      redirect: input.redirect,
      referrer: input.referrer,
      referrerPolicy: input.referrerPolicy,
      integrity: input.integrity,
      keepalive: input.keepalive,
      signal: input.signal,
    };
  }

  private isRequest(input: RequestInfo | URL | string): input is Request {
    return typeof Request !== 'undefined' && input instanceof Request;
  }

  private async refreshSession(refreshToken?: string): Promise<StoredSession | null> {
    if (!refreshToken) return null;
    if (!this.refreshPromise) {
      this.refreshPromise = (async () => {
        const response = await this.fetchImpl(buildApiUrl('/v1/auth/refresh'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: refreshToken }),
        });
        if (!response.ok) {
          throw new Error(`refresh failed (${response.status})`);
        }
        const payload = (await response.json()) as AuthRefreshResponse;
        const current = getStoredSession();

        persistSession({
          accessToken: payload?.access_token || '',
          refreshToken: payload?.refresh_token || refreshToken,
          roles: Array.isArray(payload?.roles) ? payload.roles.filter((role): role is string => typeof role === 'string') : current.roles,
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
          this.refreshPromise = null;
        });
    }
    return this.refreshPromise;
  }

  private shouldBypassRefresh(url: string) {
    return (
      url.includes('/v1/auth/login') ||
      url.includes('/v1/auth/refresh') ||
      url.includes('/v1/auth/password')
    );
  }

  private shouldAttachAuth(input: RequestInfo | URL | string, url: string): boolean {
    if (typeof input === 'string' && !/^https?:\/\//i.test(input)) return true;
    if (url.startsWith('/')) return true;
    const apiBase = getApiBaseUrl();
    return Boolean(apiBase) && url.startsWith(`${apiBase}/`);
  }
}

export const apiClient = new ApiClient();

export function apiFetch(input: RequestInfo | URL | string, init?: ApiRequestInit): Promise<Response> {
  return apiClient.fetch(input, init);
}

export async function loginLocal(identifier: string, password: string) {
  const response = await apiClient.fetch('/v1/auth/login', {
    auth: false,
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

export type AuthProviderOption = {
  id: string;
  type: string;
  display_name: string;
  scopes?: string[];
  allowed_email_domains?: string[];
  auth_url_kind?: string;
};

export type AuthProvidersResponse = {
  local_enabled: boolean;
  oidc_enabled: boolean;
  providers: AuthProviderOption[];
};

export async function fetchAuthProviders(): Promise<AuthProvidersResponse> {
  const response = await apiClient.fetch('/v1/auth/providers', {
    auth: false,
    cache: 'no-store',
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Failed to load authentication providers');
  }
  const payload = await response.json();
  return {
    local_enabled: Boolean(payload?.local_enabled),
    oidc_enabled: Boolean(payload?.oidc_enabled),
    providers: Array.isArray(payload?.providers)
      ? payload.providers.filter((provider: unknown): provider is AuthProviderOption => {
          const record = provider as Partial<AuthProviderOption>;
          return typeof record?.id === 'string' && typeof record?.display_name === 'string';
        })
      : [],
  };
}

export async function discoverAuthProvider(email: string): Promise<AuthProviderOption | null> {
  const response = await apiClient.fetch('/v1/auth/discover', {
    auth: false,
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Failed to discover authentication provider');
  }
  const payload = await response.json();
  return payload?.found && payload?.provider ? payload.provider as AuthProviderOption : null;
}

export async function exchangeSessionCode(code: string) {
  const response = await apiClient.fetch('/v1/auth/session/exchange', {
    auth: false,
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Session exchange failed');
  }
  return response.json();
}

export function buildOIDCStartUrl(providerID: string, returnTo: string, options: { prompt?: string } = {}): string {
  return buildAuthProviderStartUrl(providerID, returnTo, { ...options, kind: 'oidc' });
}

export function buildAuthProviderStartUrl(providerID: string, returnTo: string, options: { prompt?: string; kind?: string } = {}): string {
  const values = new URLSearchParams();
  if (returnTo) values.set('return_to', returnTo);
  if (options.prompt) values.set('prompt', options.prompt);
  const kind = options.kind === 'oauth2' ? 'oauth2' : 'oidc';
  return buildApiUrl(`/v1/auth/${kind}/${encodeURIComponent(providerID)}/start?${values.toString()}`);
}

export async function logoutCurrentSession(): Promise<{ logoutURL?: string }> {
  const session = getStoredSession();
  try {
    if (!session.refreshToken) return {};
    const response = await apiClient.fetch('/v1/auth/logout', {
      auth: false,
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: session.refreshToken }),
      retryOnUnauthorized: false,
    });
    if (!response.ok) return {};
    if (response.status === 204) return {};
    const payload = await response.json().catch(() => ({}));
    const logoutURL = typeof payload?.logout_url === 'string' ? payload.logout_url : '';
    return logoutURL ? { logoutURL } : {};
  } finally {
    clearSession();
    requirePromptForNextSSOLogin();
  }
}

export async function updateEmail(email: string): Promise<{ email: string }> {
  const response = await apiClient.fetch('/v1/auth/email', {
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
  const response = await apiClient.fetch('/v1/auth/password', {
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
  const response = await apiClient.fetch('/v1/auth/personal-tokens');
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Failed to load personal tokens');
  }
  const payload = await response.json();
  return Array.isArray(payload) ? payload : [];
}

export async function createPersonalAccessToken(name: string, options: CreatePersonalAccessTokenOptions): Promise<PersonalAccessToken> {
  const response = await apiClient.fetch('/v1/auth/personal-tokens', {
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
  const response = await apiClient.fetch(`/v1/auth/personal-tokens/${encodeURIComponent(tokenID)}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Failed to revoke personal token');
  }
}
