import { Plus, RefreshCw } from 'lucide-react';
import type { CredentialSummary } from './model';

type CredentialDashboardProps = {
  canManage: boolean;
  loading: boolean;
  saving: boolean;
  summary: CredentialSummary;
  onReload: () => void;
  onStartCreate: () => void;
};

export function CredentialDashboard({
  canManage,
  loading,
  saving,
  summary,
  onReload,
  onStartCreate,
}: CredentialDashboardProps) {
  const metrics = [
    {
      label: 'Total credentials',
      value: summary.total,
      note: 'Across all scopes',
    },
    {
      label: 'Active',
      value: summary.active,
      note: 'Ready for use',
      tone: 'text-emerald-500',
    },
    {
      label: 'Disabled',
      value: summary.disabled,
      note: 'Blocked from use',
    },
    {
      label: 'Pending value',
      value: summary.pending,
      note: 'Requires attention',
      tone: 'text-amber-500',
    },
  ];

  return (
    <section className="space-y-4 rounded-xl bg-[var(--bg-primary)]" aria-labelledby="credentials-heading">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="max-w-3xl">
          <h2 id="credentials-heading" className="text-2xl font-semibold text-[var(--text-primary)]">
            Credential registry
          </h2>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            Manage encrypted, versioned credentials across teams and global resources.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {!canManage && <span className="runner-pill runner-pill--muted">Read-only</span>}
          <button
            type="button"
            className="glass-button-ghost !rounded-lg"
            onClick={onReload}
            disabled={loading || saving}
          >
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} aria-hidden="true" />
            Reload
          </button>
          {canManage && (
            <button type="button" className="glass-button-primary !rounded-lg" onClick={onStartCreate} disabled={saving}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              New credential
            </button>
          )}
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {metrics.map(metric => (
          <article
            key={metric.label}
            className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 shadow-sm"
          >
            <p className="text-xs text-[var(--text-secondary)]">{metric.label}</p>
            <p className="mt-1 text-2xl font-semibold text-[var(--text-primary)]">{metric.value}</p>
            <p className={`mt-2 text-xs ${metric.tone || 'text-[var(--text-secondary)]'}`}>{metric.note}</p>
          </article>
        ))}
      </div>
    </section>
  );
}
