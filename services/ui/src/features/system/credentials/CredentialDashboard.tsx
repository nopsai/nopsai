import { useRef, useState } from 'react';
import { AlertTriangle, Database, Plus, RefreshCw, Search, ShieldCheck, ShieldOff, X } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import type { CredentialSummary } from './model';

type CredentialDashboardProps = {
  canCreate: boolean;
  canManage: boolean;
  loading: boolean;
  query: string;
  saving: boolean;
  summary: CredentialSummary;
  onQueryChange: (value: string) => void;
  onReload: () => void;
  onStartCreate: () => void;
};

export function CredentialDashboard({
  canCreate,
  canManage,
  loading,
  query,
  saving,
  summary,
  onQueryChange,
  onReload,
  onStartCreate,
}: CredentialDashboardProps) {
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const searchActive = searchOpen || Boolean(query.trim());
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
              className="credential-registry__button credential-registry__button--ghost"
              onClick={onReload}
              disabled={loading || saving}
              aria-label="Reload credentials"
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
          <div className={`pipelines-search-shell credential-registry__search-shell ${searchActive ? 'open' : ''}`}>
            <button
              type="button"
              className="pipelines-search-toggle"
              aria-label="Search credentials"
              title="Search credentials"
              onClick={() => {
                setSearchOpen(true);
                requestAnimationFrame(() => searchInputRef.current?.focus());
              }}
            >
              <Search className="h-4 w-4" aria-hidden="true" />
            </button>
            <input
              ref={searchInputRef}
              type="search"
              className="pipelines-search-input"
              aria-label="Search credentials query"
              placeholder="Search credentials"
              value={query}
              onFocus={() => setSearchOpen(true)}
              onChange={event => {
                onQueryChange(event.target.value);
                if (event.target.value && !searchOpen) setSearchOpen(true);
              }}
              onBlur={() => {
                if (!query.trim()) setSearchOpen(false);
              }}
            />
            {query || searchOpen ? (
              <button
                type="button"
                className="pipelines-search-clear"
                aria-label="Clear search"
                onMouseDown={event => event.preventDefault()}
                onClick={() => {
                  onQueryChange('');
                  setSearchOpen(false);
                  searchInputRef.current?.blur();
                }}
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            ) : null}
          </div>
        </div>
      </div>
    </section>
  );
}
