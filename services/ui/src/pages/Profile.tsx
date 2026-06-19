import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { Copy, KeyRound, Plus, RefreshCw, Trash2, X } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import {
  changePassword,
  createPersonalAccessToken,
  listPersonalAccessTokens,
  revokePersonalAccessToken,
  updateEmail,
  type PersonalAccessToken,
} from '../lib/api';
import { copyTextToClipboard } from '../lib/clipboard';

type CurrentUser = {
  sub: string;
  email?: string;
  roles?: string[];
};

type Props = {
  user: CurrentUser | null;
  loading?: boolean;
  onLogout: () => void;
  onUserUpdated?: (updates: Partial<CurrentUser>) => void;
  mustChangePassword?: boolean;
  onPasswordChanged?: () => void;
  canAccessSystem?: boolean;
  systemPath?: string;
};

const TOKEN_EXPIRY_OPTIONS = [
  { value: 30, label: '30 days' },
  { value: 90, label: '90 days' },
  { value: 365, label: '1 year' },
];

type TokenExpiryMode = 'preset' | 'custom' | 'never';

function addDays(date: Date, days: number) {
  const next = new Date(date);
  next.setDate(next.getDate() + days);
  return next;
}

function formatDateInputValue(date: Date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function defaultCustomExpiryDate() {
  return formatDateInputValue(addDays(new Date(), 90));
}

function dateInputToEndOfDayISOString(value: string) {
  const [yearRaw, monthRaw, dayRaw] = value.split('-').map(part => Number(part));
  if (!yearRaw || !monthRaw || !dayRaw) return '';
  return new Date(yearRaw, monthRaw - 1, dayRaw, 23, 59, 59, 999).toISOString();
}

function formatTokenDate(value?: string) {
  if (!value) return 'Never';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'Unknown';
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(date);
}

function tokenExpired(token: PersonalAccessToken) {
  if (!token.expires_at) return false;
  const expiresAt = new Date(token.expires_at).getTime();
  return Number.isFinite(expiresAt) && expiresAt <= Date.now();
}

export default function ProfilePage({ user, loading, onLogout, onUserUpdated, mustChangePassword, onPasswordChanged, canAccessSystem, systemPath }: Props) {
  const navigate = useNavigate();
  const [email, setEmail] = useState(user?.email || '');
  const [emailDraft, setEmailDraft] = useState(user?.email || '');
  const [editingEmail, setEditingEmail] = useState(false);
  const [passwordModalOpen, setPasswordModalOpen] = useState(false);
  const [tokenModalOpen, setTokenModalOpen] = useState(false);
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [tokenName, setTokenName] = useState('');
  const [tokenExpiryDays, setTokenExpiryDays] = useState(90);
  const [tokenExpiryMode, setTokenExpiryMode] = useState<TokenExpiryMode>('preset');
  const [customTokenExpiryDate, setCustomTokenExpiryDate] = useState(defaultCustomExpiryDate);
  const [tokens, setTokens] = useState<PersonalAccessToken[]>([]);
  const [createdToken, setCreatedToken] = useState<PersonalAccessToken | null>(null);
  const [emailSaving, setEmailSaving] = useState(false);
  const [passwordSaving, setPasswordSaving] = useState(false);
  const [tokenLoading, setTokenLoading] = useState(false);
  const [tokenSaving, setTokenSaving] = useState(false);
  const [tokenActionID, setTokenActionID] = useState<string | null>(null);
  const [tokenCopyState, setTokenCopyState] = useState<'idle' | 'copied' | 'failed'>('idle');
  const initials = useMemo(() => {
    const base = (user?.email || user?.sub || 'U').trim();
    const cleaned = base.replace(/[^A-Za-z0-9]/g, '');
    return (cleaned[0] || base[0] || 'U').toUpperCase();
  }, [user?.email, user?.sub]);
  useEffect(() => {
    setEmail(user?.email || '');
    setEmailDraft(user?.email || '');
    setEditingEmail(false);
  }, [user?.email]);

  useEffect(() => {
    if (mustChangePassword) {
      setPasswordModalOpen(true);
    }
  }, [mustChangePassword]);

  const loadTokens = useCallback(async () => {
    if (!user) {
      setTokens([]);
      return;
    }
    setTokenLoading(true);
    try {
      const result = await listPersonalAccessTokens();
      setTokens(result);
    } catch (error) {
      window.alert(error instanceof Error ? error.message : 'Failed to load personal tokens.');
    } finally {
      setTokenLoading(false);
    }
  }, [user]);

  useEffect(() => {
    void loadTokens();
  }, [loadTokens]);

  const primaryLabel = user?.email || user?.sub || 'User';
  const minTokenExpiryDate = useMemo(() => formatDateInputValue(new Date()), []);
  const maxTokenExpiryDate = useMemo(() => formatDateInputValue(addDays(new Date(), 365)), []);

  const resetTokenForm = () => {
    setTokenName('');
    setTokenExpiryDays(90);
    setTokenExpiryMode('preset');
    setCustomTokenExpiryDate(defaultCustomExpiryDate());
  };

  const handleSaveEmail = async () => {
    const nextEmail = emailDraft.trim();
    if (!nextEmail) {
      window.alert('Please enter an email.');
      return;
    }
    setEmailSaving(true);
    try {
      const result = await updateEmail(nextEmail);
      const savedEmail = result?.email?.trim() || nextEmail;
      setEmail(savedEmail);
      setEmailDraft(savedEmail);
      setEditingEmail(false);
      onUserUpdated?.({ email: savedEmail });
      window.alert('Email updated.');
    } catch (error) {
      window.alert(error instanceof Error ? error.message : 'Failed to update email.');
    } finally {
      setEmailSaving(false);
    }
  };

  const handlePasswordSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (newPassword !== confirmPassword) {
      window.alert('New passwords do not match.');
      return;
    }
    setPasswordSaving(true);
    try {
      await changePassword(currentPassword, newPassword);
      window.alert('Password updated.');
      onPasswordChanged?.();
      setPasswordModalOpen(false);
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    } catch (error) {
      window.alert(error instanceof Error ? error.message : 'Failed to update password.');
    } finally {
      setPasswordSaving(false);
    }
  };

  const handleCreateToken = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const name = tokenName.trim();
    if (!name) {
      window.alert('Please enter a token name.');
      return;
    }
    setTokenSaving(true);
    try {
      const expiresAt = tokenExpiryMode === 'custom' ? dateInputToEndOfDayISOString(customTokenExpiryDate) : '';
      if (tokenExpiryMode === 'custom' && !expiresAt) {
        window.alert('Please choose a valid expiration date.');
        setTokenSaving(false);
        return;
      }
      const token = await createPersonalAccessToken(name, {
        expiresInDays: tokenExpiryMode === 'preset' ? tokenExpiryDays : undefined,
        expiresAt: tokenExpiryMode === 'custom' ? expiresAt : undefined,
        neverExpires: tokenExpiryMode === 'never',
      });
      const tokenMetadata: PersonalAccessToken = {
        id: token.id,
        name: token.name,
        token_suffix: token.token_suffix,
        created_at: token.created_at,
        expires_at: token.expires_at,
        last_used_at: token.last_used_at,
      };
      setCreatedToken(token);
      setTokens(prev => [tokenMetadata, ...prev]);
      resetTokenForm();
      setTokenModalOpen(false);
      setTokenCopyState('idle');
    } catch (error) {
      window.alert(error instanceof Error ? error.message : 'Failed to create personal token.');
    } finally {
      setTokenSaving(false);
    }
  };

  const handleCopyToken = async () => {
    if (!createdToken?.token) return;
    try {
      await copyTextToClipboard(createdToken.token);
      setTokenCopyState('copied');
      window.setTimeout(() => setTokenCopyState('idle'), 1500);
    } catch {
      setTokenCopyState('failed');
      window.setTimeout(() => setTokenCopyState('idle'), 2200);
    }
  };

  const handleRevokeToken = async (token: PersonalAccessToken) => {
    if (!window.confirm(`Revoke "${token.name}"?`)) return;
    setTokenActionID(token.id);
    try {
      await revokePersonalAccessToken(token.id);
      setTokens(prev => prev.filter(item => item.id !== token.id));
    } catch (error) {
      window.alert(error instanceof Error ? error.message : 'Failed to revoke personal token.');
    } finally {
      setTokenActionID(null);
    }
  };

  if (loading) {
    return (
      <div className="p-6 max-w-5xl mx-auto">
        <div className="glass-card p-6 rounded-xl border border-[var(--border-primary)] animate-pulse">
          <div className="h-4 w-32 bg-[var(--bg-tertiary)] rounded mb-3"></div>
          <div className="h-10 w-48 bg-[var(--bg-tertiary)] rounded"></div>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6 max-w-5xl mx-auto">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm text-[var(--text-secondary)]">Manage your account, roles, and session details.</p>
        </div>
        <div className="flex gap-2">
          <button className="glass-button-ghost" type="button" onClick={() => navigate(-1)}>
            Back
          </button>
          <button className="glass-button-danger" type="button" onClick={onLogout}>
            Logout
          </button>
        </div>
      </div>

      {!user ? (
        <div className="glass-card p-6 rounded-xl border border-[var(--border-primary)]">
          <p className="text-sm text-[var(--text-secondary)]">No active user session. Please sign in again.</p>
        </div>
      ) : (
        <div className="glass-card p-6 rounded-xl border border-[var(--border-primary)] space-y-6">
          <div className="flex flex-wrap items-center gap-4">
            <div className="h-16 w-16 rounded-full bg-[var(--border-accent)]/15 text-[var(--border-accent)] flex items-center justify-center text-xl font-semibold">
              {initials}
            </div>
            <div className="min-w-0 space-y-1">
              <p className="text-lg font-semibold text-[var(--text-primary)] truncate">{primaryLabel}</p>
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="rounded-xl border border-[var(--border-primary)] p-4 bg-[var(--bg-tertiary)] space-y-3">
              <p className="text-xs uppercase tracking-wide text-[var(--text-secondary)] mb-2">Identity</p>
              <dl className="text-sm grid grid-cols-[auto,1fr] items-center gap-x-3 gap-y-3">
                <dt className="text-[var(--text-secondary)] whitespace-nowrap">User ID</dt>
                <dd className="text-[var(--text-primary)] truncate">{user.sub}</dd>

                <dt className="text-[var(--text-secondary)] whitespace-nowrap">Email</dt>
                <dd>
                  {!editingEmail ? (
                    <div className="flex items-center gap-2 group">
                      <span className="text-[var(--text-primary)] truncate flex-1">{email || '—'}</span>
                      <button
                        type="button"
                        className="glass-button-subtle whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity"
                        onClick={() => {
                          setEditingEmail(true);
                          setEmailDraft(email);
                        }}
                      >
                        Edit
                      </button>
                    </div>
                  ) : (
                    <div className="flex gap-2 items-center">
                      <input
                        type="email"
                        className="pipelines-input flex-1"
                        value={emailDraft}
                        onChange={e => setEmailDraft(e.target.value)}
                        placeholder="you@example.com"
                        autoFocus
                      />
                      <div className="flex gap-2">
                        <button
                          type="button"
                          className="glass-button-subtle whitespace-nowrap disabled:opacity-50"
                          onClick={handleSaveEmail}
                          disabled={emailSaving || !emailDraft.trim()}
                        >
                          {emailSaving ? 'Saving...' : 'Save'}
                        </button>
                        <button
                          type="button"
                          className="glass-button-ghost whitespace-nowrap"
                          onClick={() => {
                            setEditingEmail(false);
                            setEmailDraft(email);
                          }}
                        >
                          Cancel
                        </button>
                      </div>
                    </div>
                  )}
                </dd>

                <div className="col-span-2 pt-1">
                  <button type="button" className="glass-button-subtle" onClick={() => setPasswordModalOpen(true)}>
                    Change password
                  </button>
                </div>
              </dl>
            </div>

            <div className="rounded-xl border border-[var(--border-primary)] p-4 bg-[var(--bg-tertiary)]">
              <p className="text-xs uppercase tracking-wide text-[var(--text-secondary)] mb-2">Roles</p>
              <div className="flex flex-wrap gap-2">
                {(user.roles || []).map(role => (
                  <span key={role} className="px-2 py-1 rounded-lg bg-[var(--bg-secondary)] text-[var(--text-primary)] text-xs border border-[var(--border-primary)]">
                    {role}
                  </span>
                ))}
                {(!user.roles || user.roles.length === 0) && (
                  <span className="text-xs text-[var(--text-secondary)]">No roles assigned</span>
                )}
              </div>
            </div>

            <div className="rounded-xl border border-[var(--border-primary)] p-4 bg-[var(--bg-tertiary)]">
              <p className="text-xs uppercase tracking-wide text-[var(--text-secondary)] mb-2">Session</p>
              <p className="text-sm text-[var(--text-secondary)]">Access tokens refresh automatically when they expire.</p>
              <div className="mt-3 flex gap-2">
                {canAccessSystem && (
                  <button className="glass-button-subtle" type="button" onClick={() => navigate(systemPath || '/system/config')}>
                    System settings
                  </button>
                )}
                <button className="glass-button-ghost" type="button" onClick={onLogout}>
                  Logout
                </button>
              </div>
            </div>

            <div className="rounded-xl border border-[var(--border-primary)] p-4 bg-[var(--bg-tertiary)] md:col-span-2 space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-2">
                  <KeyRound className="h-4 w-4 text-[var(--text-secondary)]" aria-hidden="true" />
                  <p className="text-xs uppercase tracking-wide text-[var(--text-secondary)]">Personal tokens</p>
                </div>
                <div className="flex gap-2">
                  <button
                    className="glass-button-ghost inline-flex items-center gap-2"
                    type="button"
                    onClick={() => void loadTokens()}
                    disabled={tokenLoading}
                    title="Refresh tokens"
                  >
                    <RefreshCw className={`h-4 w-4 ${tokenLoading ? 'animate-spin' : ''}`} aria-hidden="true" />
                    <span>Refresh</span>
                  </button>
                  <button
                    className="glass-button-subtle inline-flex items-center gap-2"
                    type="button"
                    onClick={() => setTokenModalOpen(true)}
                  >
                    <Plus className="h-4 w-4" aria-hidden="true" />
                    <span>Create token</span>
                  </button>
                </div>
              </div>

              {createdToken?.token && (
                <div className="rounded-lg border border-[var(--border-accent)]/40 bg-[var(--border-accent)]/10 p-3 space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    <p className="text-sm font-medium text-[var(--text-primary)] truncate">{createdToken.name}</p>
                    <button
                      type="button"
                      className="text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                      onClick={() => setCreatedToken(null)}
                      aria-label="Dismiss token"
                    >
                      <X className="h-4 w-4" aria-hidden="true" />
                    </button>
                  </div>
                  <div className="flex flex-col gap-2 sm:flex-row">
                    <input className="pipelines-input flex-1 font-mono text-xs" value={createdToken.token} readOnly />
                    <button type="button" className="glass-button-subtle inline-flex items-center justify-center gap-2" onClick={handleCopyToken}>
                      <Copy className="h-4 w-4" aria-hidden="true" />
                      <span>{tokenCopyState === 'copied' ? 'Copied' : tokenCopyState === 'failed' ? 'Copy failed' : 'Copy'}</span>
                    </button>
                  </div>
                  <p className="text-xs text-[var(--text-secondary)]">
                    {createdToken.expires_at ? `Shown once. Expires ${formatTokenDate(createdToken.expires_at)}.` : 'Shown once. Does not expire.'}
                  </p>
                </div>
              )}

              <div className="overflow-x-auto rounded-lg border border-[var(--border-primary)]">
                <table className="min-w-full text-sm">
                  <thead className="bg-[var(--bg-secondary)] text-[var(--text-secondary)]">
                    <tr>
                      <th className="text-left font-medium px-3 py-2">Name</th>
                      <th className="text-left font-medium px-3 py-2">Token</th>
                      <th className="text-left font-medium px-3 py-2">Last used</th>
                      <th className="text-left font-medium px-3 py-2">Expires</th>
                      <th className="text-right font-medium px-3 py-2">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {tokens.map(token => {
                      const expired = tokenExpired(token);
                      return (
                        <tr key={token.id} className="border-t border-[var(--border-primary)]">
                          <td className="px-3 py-2 text-[var(--text-primary)] max-w-[220px] truncate">{token.name}</td>
                          <td className="px-3 py-2 font-mono text-xs text-[var(--text-secondary)]">...{token.token_suffix}</td>
                          <td className="px-3 py-2 text-[var(--text-secondary)] whitespace-nowrap">{formatTokenDate(token.last_used_at)}</td>
                          <td className="px-3 py-2 whitespace-nowrap">
                            <span className={expired ? 'text-red-400' : 'text-[var(--text-secondary)]'}>{formatTokenDate(token.expires_at)}</span>
                          </td>
                          <td className="px-3 py-2 text-right">
                            <button
                              type="button"
                              className="glass-button-danger inline-flex items-center justify-center"
                              onClick={() => void handleRevokeToken(token)}
                              disabled={tokenActionID === token.id}
                              title="Revoke token"
                              aria-label={`Revoke ${token.name}`}
                            >
                              <Trash2 className="h-4 w-4" aria-hidden="true" />
                            </button>
                          </td>
                        </tr>
                      );
                    })}
                    {!tokenLoading && tokens.length === 0 && (
                      <tr>
                        <td className="px-3 py-4 text-sm text-[var(--text-secondary)]" colSpan={5}>
                          No personal tokens
                        </td>
                      </tr>
                    )}
                    {tokenLoading && (
                      <tr>
                        <td className="px-3 py-4 text-sm text-[var(--text-secondary)]" colSpan={5}>
                          Loading tokens...
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </div>
      )}
      {passwordModalOpen && (
        <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-md rounded-xl bg-[var(--bg-secondary)] border border-[var(--border-primary)] shadow-2xl p-6 space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-[var(--text-secondary)]">Security</p>
                <h2 className="text-lg font-semibold text-[var(--text-primary)]">
                  {mustChangePassword ? 'Change password required' : 'Change password'}
                </h2>
              </div>
              {!mustChangePassword && (
                <button
                  type="button"
                  className="text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                  onClick={() => {
                    setPasswordModalOpen(false);
                    setCurrentPassword('');
                    setNewPassword('');
                    setConfirmPassword('');
                  }}
                  aria-label="Close"
                >
                  <X className="h-4 w-4" aria-hidden="true" />
                </button>
              )}
            </div>
            {mustChangePassword && (
              <p className="text-sm text-[var(--text-secondary)]">
                This is your first sign-in. Choose a new password to continue.
              </p>
            )}
            <form
              className="space-y-3"
              onSubmit={handlePasswordSubmit}
            >
              <label className="flex flex-col gap-1 text-sm">
                <span className="text-[var(--text-secondary)]">Current password</span>
                <input
                  type="password"
                  className="pipelines-input"
                  value={currentPassword}
                  onChange={e => setCurrentPassword(e.target.value)}
                  placeholder="Current password"
                  required
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                <span className="text-[var(--text-secondary)]">New password</span>
                <input
                  type="password"
                  className="pipelines-input"
                  value={newPassword}
                  onChange={e => setNewPassword(e.target.value)}
                  placeholder="New password"
                  required
                />
              </label>
              <label className="flex flex-col gap-1 text-sm">
                <span className="text-[var(--text-secondary)]">Repeat new password</span>
                <input
                  type="password"
                  className="pipelines-input"
                  value={confirmPassword}
                  onChange={e => setConfirmPassword(e.target.value)}
                  placeholder="Repeat new password"
                  required
                />
              </label>
              <div className="flex justify-end gap-2 pt-2">
                {mustChangePassword ? (
                  <button type="button" className="glass-button-ghost" onClick={onLogout}>
                    Logout
                  </button>
                ) : (
                  <button
                    type="button"
                    className="glass-button-ghost"
                    onClick={() => {
                      setPasswordModalOpen(false);
                      setCurrentPassword('');
                      setNewPassword('');
                      setConfirmPassword('');
                    }}
                  >
                    Cancel
                  </button>
                )}
                <button type="submit" className="glass-button-subtle disabled:opacity-50" disabled={passwordSaving}>
                  {passwordSaving ? 'Updating...' : 'Update password'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
      {tokenModalOpen && (
        <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-md rounded-xl bg-[var(--bg-secondary)] border border-[var(--border-primary)] shadow-2xl p-6 space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-[var(--text-secondary)]">API access</p>
                <h2 className="text-lg font-semibold text-[var(--text-primary)]">Create personal token</h2>
              </div>
              <button
                type="button"
                className="text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                onClick={() => {
                  setTokenModalOpen(false);
                  resetTokenForm();
                }}
                aria-label="Close"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
            <form className="space-y-4" onSubmit={handleCreateToken}>
              <label className="flex flex-col gap-1 text-sm">
                <span className="text-[var(--text-secondary)]">Name</span>
                <input
                  type="text"
                  className="pipelines-input"
                  value={tokenName}
                  onChange={e => setTokenName(e.target.value)}
                  placeholder="Deployment script"
                  maxLength={80}
                  required
                  autoFocus
                />
              </label>
              <div className="space-y-2">
                <span className="text-sm text-[var(--text-secondary)]">Expiration</span>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
                  {TOKEN_EXPIRY_OPTIONS.map(option => (
                    <button
                      key={option.value}
                      type="button"
                      className={tokenExpiryMode === 'preset' && tokenExpiryDays === option.value ? 'glass-button-subtle' : 'glass-button-ghost'}
                      onClick={() => {
                        setTokenExpiryMode('preset');
                        setTokenExpiryDays(option.value);
                      }}
                    >
                      {option.label}
                    </button>
                  ))}
                  <button
                    type="button"
                    className={tokenExpiryMode === 'custom' ? 'glass-button-subtle' : 'glass-button-ghost'}
                    onClick={() => setTokenExpiryMode('custom')}
                  >
                    Custom
                  </button>
                  <button
                    type="button"
                    className={tokenExpiryMode === 'never' ? 'glass-button-subtle' : 'glass-button-ghost'}
                    onClick={() => setTokenExpiryMode('never')}
                  >
                    Never
                  </button>
                </div>
                {tokenExpiryMode === 'custom' && (
                  <input
                    type="date"
                    className="pipelines-input w-full"
                    value={customTokenExpiryDate}
                    min={minTokenExpiryDate}
                    max={maxTokenExpiryDate}
                    onChange={e => setCustomTokenExpiryDate(e.target.value)}
                    required
                  />
                )}
              </div>
              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  className="glass-button-ghost"
                  onClick={() => {
                    setTokenModalOpen(false);
                    resetTokenForm();
                  }}
                >
                  Cancel
                </button>
                <button type="submit" className="glass-button-subtle inline-flex items-center gap-2 disabled:opacity-50" disabled={tokenSaving || !tokenName.trim()}>
                  <KeyRound className="h-4 w-4" aria-hidden="true" />
                  <span>{tokenSaving ? 'Creating...' : 'Create'}</span>
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
