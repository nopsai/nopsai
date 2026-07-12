import { AlertTriangle, Database, KeyRound, Plus, RefreshCw, ShieldCheck, ShieldOff } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import type { CredentialSummary } from './model';

type CredentialDashboardProps = {
  canCreate: boolean;
  canManage: boolean;
  loading: boolean;
  saving: boolean;
  scopeDescription: string;
  summary: CredentialSummary;
  onReload: () => void;
  onStartCreate: () => void;
};

export function CredentialDashboard({
  canCreate,
  canManage,
  loading,
  saving,
  scopeDescription,
  summary,
  onReload,
  onStartCreate,
}: CredentialDashboardProps) {
  const metrics: Array<{
    label: string;
    value: number;
    note: string;
    icon: LucideIcon;
    tone?: 'good' | 'warn';
  }> = [
    {
      label: 'Total credentials',
      value: summary.total,
      note: 'Across all scopes',
      icon: Database,
    },
    {
      label: 'Active',
      value: summary.active,
      note: 'Ready for use',
      icon: ShieldCheck,
      tone: 'good',
    },
    {
      label: 'Disabled',
      value: summary.disabled,
      note: 'Blocked from use',
      icon: ShieldOff,
    },
    {
      label: 'Pending review',
      value: summary.pending,
      note: 'Requires attention',
      icon: AlertTriangle,
      tone: 'warn',
    },
  ];

  return (
    <section className="credential-registry__dashboard" aria-labelledby="credentials-heading">
      <div className="credential-registry__topbar">
        <div className="credential-registry__title-wrap">
          <div className="credential-registry__title-icon" aria-hidden="true">
            <KeyRound className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <h2 id="credentials-heading" className="credential-registry__title">
              Credential registry
            </h2>
            <p className="credential-registry__subtitle">
              {scopeDescription}
            </p>
          </div>
        </div>
        <div className="credential-registry__actions">
          {!canManage && <span className="credential-registry__pill">Read-only</span>}
          <button
            type="button"
            className="credential-registry__button credential-registry__button--ghost"
            onClick={onReload}
            disabled={loading || saving}
          >
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} aria-hidden="true" />
            Reload
          </button>
          <button
            type="button"
            className="credential-registry__button credential-registry__button--primary"
            onClick={onStartCreate}
            disabled={saving || !canCreate}
            title={canCreate ? undefined : canManage ? 'Team access is required to create credentials' : 'Read-only access'}
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            New credential
          </button>
        </div>
      </div>

      <div className="credential-registry__stats">
        {metrics.map(metric => {
          const Icon = metric.icon;
          return (
            <article key={metric.label} className="credential-registry__stat">
              <p className="credential-registry__stat-label">{metric.label}</p>
              <p className="credential-registry__stat-value">{metric.value}</p>
              <p className={`credential-registry__stat-note ${metric.tone ? `credential-registry__stat-note--${metric.tone}` : ''}`}>
                {metric.note}
              </p>
              <div className={`credential-registry__stat-icon ${metric.tone ? `credential-registry__stat-icon--${metric.tone === 'warn' ? 'orange' : 'green'}` : ''}`} aria-hidden="true">
                <Icon className="h-5 w-5" />
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
}
