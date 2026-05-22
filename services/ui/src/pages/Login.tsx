import { useEffect, useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { buildApiUrl, loginLocal, persistSession } from '../lib/api';

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
  const navigate = useNavigate();
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [preflight, setPreflight] = useState<SetupPreflight | null>(null);
  const [preflightUnavailable, setPreflightUnavailable] = useState(false);

  useEffect(() => {
    let cancelled = false;
    fetch(buildApiUrl('/v1/setup/preflight'), { cache: 'no-store' })
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

  const requiredFailures = (preflight?.checks || []).filter(check => check.required && check.status === 'error');
  const loginBlocked = preflightUnavailable || requiredFailures.length > 0 || preflight?.can_login === false;
  const showReadiness = loginBlocked;

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (loginBlocked) return;
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
      navigate(resp.must_change_password ? '/profile' : '/pipelineruns/main', { replace: true });
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
          <p className="text-sm text-[var(--text-secondary)]">Use your local account to continue</p>
        </div>
        <form className="space-y-4" onSubmit={handleSubmit}>
          <div className="space-y-2">
            <label className="block text-sm font-medium text-[var(--text-secondary)]">Email or username</label>
            <input
              value={identifier}
              onChange={e => setIdentifier(e.target.value)}
              className="w-full rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)]"
              placeholder="you@example.com"
              required
            />
          </div>
          <div className="space-y-2">
            <label className="block text-sm font-medium text-[var(--text-secondary)]">Password</label>
            <input
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              className="w-full rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)]"
              placeholder="••••••••"
              required
            />
          </div>
          {error && <div className="text-sm text-red-500">{error}</div>}
          {loginBlocked && <div className="text-sm text-amber-600 dark:text-amber-300">Complete required installation settings before signing in.</div>}
          <button
            type="submit"
            disabled={loading || loginBlocked}
            className="w-full py-2 rounded-lg bg-[var(--border-accent)] text-white font-medium hover:opacity-90 disabled:opacity-60 transition"
          >
            {loading ? 'Signing in...' : 'Sign in'}
          </button>
        </form>
        </div>
      </div>
    </div>
  );
}
