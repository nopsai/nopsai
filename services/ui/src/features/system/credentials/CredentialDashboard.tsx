import { AlertTriangle, Database, Plus, ShieldCheck, ShieldOff } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import type { CredentialSummary } from './model';

type CredentialDashboardProps = {
  canCreate: boolean;
  canManage: boolean;
  saving: boolean;
  summary: CredentialSummary;
  onStartCreate: () => void;
};

export function CredentialDashboard({
  canCreate,
  canManage,
  saving,
  summary,
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
        <h2 id="credentials-heading" className="sr-only">Credential registry</h2>
        <div className="credential-registry__stats" aria-label="Credential summary">
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
                  <Icon className="h-4 w-4" />
                </div>
              </article>
            );
          })}
        </div>
        <div className="credential-registry__actions credential-registry__actions--stacked">
          <div className="credential-registry__action-row">
            {!canManage && <span className="credential-registry__pill">Read-only</span>}
            <button
              type="button"
              className="credential-registry__button credential-registry__button--primary credential-registry__button--create"
              aria-label="New credential"
              onClick={onStartCreate}
              disabled={saving || !canCreate}
              title={canCreate ? 'New credential' : canManage ? 'Team access is required to create credentials' : 'Read-only access'}
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </div>
      </div>
    </section>
  );
}
