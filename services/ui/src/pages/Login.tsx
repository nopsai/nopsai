import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { loginLocal, persistSession } from '../lib/api';

export default function LoginPage({ onLogin }: { onLogin: () => void }) {
  const navigate = useNavigate();
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
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
    } catch (err: any) {
      setError(err?.message || 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-[var(--bg-primary)]">
      <div className="w-full max-w-md bg-[var(--bg-secondary)] rounded-xl shadow-lg border border-[var(--border-primary)] p-8 space-y-6">
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
          <button
            type="submit"
            disabled={loading}
            className="w-full py-2 rounded-lg bg-[var(--border-accent)] text-white font-medium hover:opacity-90 disabled:opacity-60 transition"
          >
            {loading ? 'Signing in...' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  );
}
