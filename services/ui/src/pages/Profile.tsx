import { useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { changePassword, updateEmail } from '../lib/api';

type CurrentUser = {
  sub: string;
  email?: string;
  tenantIds?: string[];
  defaultTenant?: string;
  roles?: string[];
};

type Tenant = {
  id: string;
  name: string;
};

type Props = {
  user: CurrentUser | null;
  loading?: boolean;
  onLogout: () => void;
  tenants?: Tenant[];
  selectedTenant?: string;
  onUserUpdated?: (updates: Partial<CurrentUser>) => void;
};

export default function ProfilePage({ user, loading, onLogout, tenants = [], selectedTenant, onUserUpdated }: Props) {
  const navigate = useNavigate();
  const [email, setEmail] = useState(user?.email || '');
  const [emailDraft, setEmailDraft] = useState(user?.email || '');
  const [editingEmail, setEditingEmail] = useState(false);
  const [passwordModalOpen, setPasswordModalOpen] = useState(false);
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [emailSaving, setEmailSaving] = useState(false);
  const [passwordSaving, setPasswordSaving] = useState(false);
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

  const tenantName = useMemo(() => {
    if (!user?.tenantIds || user.tenantIds.length === 0) return 'None';
    const resolveName = (id: string) => tenants.find(t => t.id === id)?.name || id;
    const preferredId = selectedTenant && user.tenantIds.includes(selectedTenant) ? selectedTenant : user.tenantIds[0];
    return resolveName(preferredId);
  }, [selectedTenant, tenants, user?.tenantIds]);

  const primaryLabel = user?.email || user?.sub || 'User';

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
              <div className="flex items-center gap-2 flex-wrap">
                {user.defaultTenant && (
                  <span className="px-2 py-1 rounded-full text-[11px] bg-[var(--bg-tertiary)] text-[var(--text-secondary)]">
                    Tenant: {user.defaultTenant}
                  </span>
                )}
              </div>
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
              <p className="text-xs uppercase tracking-wide text-[var(--text-secondary)] mb-2">Tenants</p>
              <p className="text-sm text-[var(--text-primary)]">Active: {tenantName || 'None'}</p>
              <div className="flex flex-wrap gap-2 mt-3">
                {(user.tenantIds || []).map(id => (
                  <span key={id} className="px-2 py-1 rounded-lg bg-[var(--border-accent)]/10 text-[var(--text-primary)] text-xs">
                    {tenants.find(t => t.id === id)?.name || id}
                  </span>
                ))}
                {(!user.tenantIds || user.tenantIds.length === 0) && (
                  <span className="text-xs text-[var(--text-secondary)]">No tenant memberships</span>
                )}
              </div>
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
                <button className="glass-button-subtle" type="button" onClick={() => navigate('/system/config')}>
                  System settings
                </button>
                <button className="glass-button-ghost" type="button" onClick={onLogout}>
                  Logout
                </button>
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
                <h2 className="text-lg font-semibold text-[var(--text-primary)]">Change password</h2>
              </div>
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
                ×
              </button>
            </div>
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
                <button type="submit" className="glass-button-subtle disabled:opacity-50" disabled={passwordSaving}>
                  {passwordSaving ? 'Updating...' : 'Update password'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
