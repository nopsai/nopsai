import { useEffect, useState, type FormEvent } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { resolvePostLoginPath } from '../auth/authRedirect';
import { isEmailLikeIdentifier, shouldUseLocalPasswordForIdentifier } from '../auth/loginIdentifier';
import {
  apiClient,
  buildOIDCStartUrl,
  consumeNextSSOLoginPrompt,
  discoverAuthProvider,
  exchangeSessionCode,
  fetchAuthProviders,
  loginLocal,
  persistSession,
  type AuthProvidersResponse,
} from '../lib/api';

type SetupPreflightCheck = {
  id: string;
  label: string;
  status: 'success' | 'warning' | 'error' | 'info' | string;
  message: string;
  required: boolean;
  suggested_env?: Record<string, string>;
};

type SetupPreflight = {
  ready: boolean;
  can_login: boolean;
  mode: string;
  config_path?: string;
  env_file_path?: string;
  checks: SetupPreflightCheck[];
};

export default function LoginPage({ onLogin }: { onLogin: () => void }) {
  const location = useLocation();
  const navigate = useNavigate();
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [discoveryLoading, setDiscoveryLoading] = useState(false);
  const [error, setError] = useState('');
  const [preflight, setPreflight] = useState<SetupPreflight | null>(null);
  const [preflightUnavailable, setPreflightUnavailable] = useState(false);
  const [authProviders, setAuthProviders] = useState<AuthProvidersResponse>({ local_enabled: true, oidc_enabled: false, providers: [] });
  const [providersLoading, setProvidersLoading] = useState(true);
  const [showLocalPassword, setShowLocalPassword] = useState(false);

  const postLoginPath = resolvePostLoginPath(location.state);

  useEffect(() => {
    let cancelled = false;
    apiClient.fetch('/v1/setup/preflight', { cache: 'no-store' })
      .then(async response => {
        if (response.status === 404) return null;
        const payload = (await response.json()) as SetupPreflight;
        return payload;
      })
      .then(payload => {
        if (cancelled) return;
        setPreflight(payload);
        setPreflightUnavailable(false);
      })
      .catch(() => {
        if (cancelled) return;
        setPreflight(null);
        setPreflightUnavailable(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    fetchAuthProviders()
      .then(payload => {
        if (cancelled) return;
        setAuthProviders(payload);
        setShowLocalPassword(!payload.oidc_enabled || payload.providers.length === 0);
      })
      .catch(() => {
        if (cancelled) return;
        setAuthProviders({ local_enabled: true, oidc_enabled: false, providers: [] });
        setShowLocalPassword(true);
      })
      .finally(() => {
        if (!cancelled) setProvidersLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const code = params.get('session_code');
    if (!code) return;
    let cancelled = false;
    setError('');
    setLoading(true);
    exchangeSessionCode(code)
      .then(resp => {
        if (cancelled) return;
        persistSession({
          accessToken: resp.access_token,
          refreshToken: resp.refresh_token,
          roles: resp.roles,
          sub: resp.sub,
          mustChangePassword: Boolean(resp.must_change_password),
        });
        onLogin();
        navigate(
          resp.must_change_password ? '/profile' : (params.get('return_to') || postLoginPath),
          { replace: true }
        );
      })
      .catch(err => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Session exchange failed');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [location.search, navigate, onLogin, postLoginPath]);

  const requiredFailures = (preflight?.checks || []).filter(check => check.required && check.status === 'error');
  const loginBlocked = preflightUnavailable || requiredFailures.length > 0 || preflight?.can_login === false;
  const showReadiness = loginBlocked;
  const ssoEnabled = authProviders.oidc_enabled && authProviders.providers.length > 0;
  const localEnabled = authProviders.local_enabled;

  const startProviderLogin = (providerID: string) => {
    if (loginBlocked) return;
    const prompt = consumeNextSSOLoginPrompt() ? 'login' : undefined;
    window.location.assign(buildOIDCStartUrl(providerID, postLoginPath, { prompt }));
  };

  const handleDiscover = async () => {
    if (loginBlocked) return;
    const loginIdentifier = identifier.trim();
    if (!loginIdentifier) {
      setError(localEnabled ? 'Email or username is required' : 'Email is required');
      return;
    }
    if (shouldUseLocalPasswordForIdentifier({ identifier: loginIdentifier, localEnabled, ssoEnabled })) {
      setShowLocalPassword(true);
      setError('');
      return;
    }
    if (!isEmailLikeIdentifier(loginIdentifier)) {
      setError('Company email is required for SSO.');
      return;
    }
    setError('');
    setDiscoveryLoading(true);
    try {
      const provider = await discoverAuthProvider(loginIdentifier);
      if (provider) {
        startProviderLogin(provider.id);
        return;
      }
      if (localEnabled) {
        setShowLocalPassword(true);
        setError('');
      } else {
        setError('No SSO provider is mapped to this email domain.');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Provider discovery failed');
    } finally {
      setDiscoveryLoading(false);
    }
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (loginBlocked || !localEnabled) return;
    setError('');
    setLoading(true);
    try {
      const resp = await loginLocal(identifier.trim(), password);
      persistSession({
        accessToken: resp.access_token,
        refreshToken: resp.refresh_token,
        roles: resp.roles,
        sub: resp.sub,
        mustChangePassword: Boolean(resp.must_change_password),
      });
      onLogin();
      navigate(
        resp.must_change_password ? '/profile' : postLoginPath,
        { replace: true }
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  const suggestedEnv = requiredFailures.flatMap(check => Object.entries(check.suggested_env || {}));

  return (
    <div className="min-h-screen flex items-center justify-center bg-[var(--bg-primary)] p-6">
      <div className={`w-full grid gap-5 ${showReadiness ? 'max-w-4xl lg:grid-cols-[1fr_0.85fr]' : 'max-w-md'}`}>
        {showReadiness && (
          <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-6 shadow-lg">
            <div className="login-brand mb-5" aria-label="NopsAI">
              <span className="sr-only">NopsAI</span>
              <img className="brand-logo brand-logo--light" src="/brand/nopsai-logo-light.png" alt="" aria-hidden="true" />
              <img className="brand-logo brand-logo--dark" src="/brand/nopsai-logo-dark.png" alt="" aria-hidden="true" />
            </div>
            <h1 className="text-2xl font-semibold text-[var(--text-primary)]">Installation readiness</h1>
            <p className="mt-2 text-sm leading-6 text-[var(--text-secondary)]">
              NopsAI needs the database, master encryption key, and JWT signing key before the authenticated workspace can open.
            </p>

            <div className="mt-5 space-y-3">
              {preflightUnavailable ? (
                <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-700 dark:text-red-300">
                  The API is not reachable. Check that the `nopsai` service is running and verify the required environment below.
                </div>
              ) : preflight ? (
                <>
                  {(preflight.checks || []).filter(check => check.required || check.status === 'error').map(check => (
                    <div key={check.id} className={`rounded-lg border p-3 text-sm ${check.status === 'success' ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : check.status === 'error' ? 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300' : 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'}`}>
                      <div className="flex items-center justify-between gap-3">
                        <span className="font-semibold">{check.label}</span>
                        <span className="text-xs uppercase">{check.required ? 'Required' : 'Optional'}</span>
                      </div>
                      <div className="mt-1 text-xs leading-5">{check.message}</div>
                    </div>
                  ))}
                </>
              ) : (
                <div className="rounded-lg border border-[var(--border-primary)] p-4 text-sm text-[var(--text-secondary)]">Checking installation prerequisites...</div>
              )}
            </div>

            {(preflightUnavailable || suggestedEnv.length > 0) && (
              <div className="mt-5">
                <div className="mb-2 text-sm font-semibold">Suggested runtime values</div>
                <pre className="overflow-auto rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3 text-xs leading-5">{suggestedEnv.length > 0 ? suggestedEnv.map(([key, value]) => `${key}=${value}`).join('\n') : 'DATABASE_URL=postgres://nopsai_user:yoursecurepassword@nopsai-db:5432/nopsai_db\nNOPSAI_MASTER_KEY=$(openssl rand -base64 32)\nJWT_SIGNING_KEY=$(openssl rand -base64 48)'}</pre>
              </div>
            )}
          </div>
        )}

        <div className="w-full bg-[var(--bg-secondary)] rounded-xl shadow-lg border border-[var(--border-primary)] p-8 space-y-6 self-start">
        <div className="text-center space-y-2">
          <div className="login-brand" aria-label="NopsAI">
            <span className="sr-only">NopsAI</span>
            <img className="brand-logo brand-logo--light" src="/brand/nopsai-logo-light.png" alt="" aria-hidden="true" />
            <img className="brand-logo brand-logo--dark" src="/brand/nopsai-logo-dark.png" alt="" aria-hidden="true" />
          </div>
          <h1 className="text-2xl font-semibold text-[var(--text-primary)]">Sign in</h1>
          <p className="text-sm text-[var(--text-secondary)]">
            {ssoEnabled ? 'Use SSO or your local account to continue' : 'Use your local account to continue'}
          </p>
        </div>
        {ssoEnabled && (
          <div className="space-y-3">
            <div className="grid gap-2">
              {authProviders.providers.map(provider => (
                <button
                  key={provider.id}
                  type="button"
                  className="w-full rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm font-medium text-[var(--text-primary)] transition hover:border-[var(--border-accent)] disabled:opacity-60"
                  disabled={loginBlocked || providersLoading}
                  onClick={() => startProviderLogin(provider.id)}
                >
                  Continue with {provider.display_name}
                </button>
              ))}
            </div>
            <div className="flex items-center gap-3 text-xs uppercase tracking-wide text-[var(--text-secondary)]">
              <span className="h-px flex-1 bg-[var(--border-primary)]" />
              <span>or</span>
              <span className="h-px flex-1 bg-[var(--border-primary)]" />
            </div>
          </div>
        )}
        <form className="space-y-4" onSubmit={showLocalPassword ? handleSubmit : event => { event.preventDefault(); void handleDiscover(); }}>
          <div className="space-y-2">
            <label htmlFor="login-identifier" className="block text-sm font-medium text-[var(--text-secondary)]">
              {ssoEnabled && !showLocalPassword ? 'Company email' : 'Email or username'}
            </label>
            <input
              id="login-identifier"
              value={identifier}
              onChange={e => setIdentifier(e.target.value)}
              className="w-full rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)]"
              placeholder={ssoEnabled && !showLocalPassword ? 'you@example.com' : 'admin or you@example.com'}
              required
            />
          </div>
          {showLocalPassword && localEnabled && (
            <div className="space-y-2">
              <label htmlFor="login-password" className="block text-sm font-medium text-[var(--text-secondary)]">
                Password
              </label>
              <input
                id="login-password"
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                className="w-full rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)]"
                placeholder="••••••••"
                required={showLocalPassword}
              />
            </div>
          )}
          {error && <div className="text-sm text-red-500">{error}</div>}
          {loginBlocked && <div className="text-sm text-amber-600 dark:text-amber-300">Complete required installation settings before signing in.</div>}
          <button
            type="submit"
            disabled={loading || discoveryLoading || providersLoading || loginBlocked || (!showLocalPassword && !ssoEnabled) || (showLocalPassword && !localEnabled)}
            className="w-full rounded-lg bg-indigo-700 py-2 font-medium text-white transition hover:bg-indigo-800 disabled:opacity-60 dark:bg-indigo-600 dark:hover:bg-indigo-500"
          >
            {showLocalPassword ? (loading ? 'Signing in...' : 'Sign in') : (discoveryLoading ? 'Checking...' : 'Continue')}
          </button>
          {ssoEnabled && localEnabled && showLocalPassword && (
            <button
              type="button"
              className="w-full text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
              onClick={() => {
                setShowLocalPassword(false);
                setPassword('');
                setError('');
              }}
            >
              Use company SSO instead
            </button>
          )}
        </form>
        </div>
      </div>
    </div>
  );
}
