import { NavLink } from 'react-router-dom';
import { ArrowRight } from 'lucide-react';
import type { StepUsageItem } from './api';
import { splitIdentifier } from './model';
import { formatStepDetailSource, formatStepUsageTeam } from './stepDetailPresentation';

type StepUsagePanelProps = {
  usage: StepUsageItem[];
  loading: boolean;
  error: string | null;
};

export function StepUsagePanel({ usage, loading, error }: StepUsagePanelProps) {
  return (
    <aside className="pipeline-activity-panels min-w-0 space-y-4">
      <section className="glass-card overflow-hidden">
        <header className="p-4 border-b border-[var(--border-primary)]">
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">Used in Pipelines</h3>
        </header>
        <div className="p-4">
          {loading ? (
            <p className="text-sm text-[var(--text-secondary)]">Loading usage…</p>
          ) : error ? (
            <p className="text-sm text-red-500">Failed to load usage: {error}</p>
          ) : usage.length ? (
            <div className="pipeline-detail-object-table pipeline-detail-object-table--step-usage" role="table" aria-label="Step usage">
              <div className="pipeline-detail-object-row pipeline-detail-object-row--head" role="row" aria-hidden="true">
                <span>Pipeline</span>
                <span>Team</span>
                <span>Source</span>
                <span>Description</span>
                <span>Open</span>
              </div>
              <ul className="pipeline-detail-object-list">
                {usage.map(item => {
                  const { name } = splitIdentifier(item.identifier);
                  const pipelineName = item.name || name || item.identifier;
                  return (
                    <li key={item.identifier}>
                      <NavLink
                        className="pipeline-detail-object-row pipeline-detail-object-row--link"
                        to={`/pipelines/${item.identifier.split('/').map(encodeURIComponent).join('/')}`}
                        aria-label={`Open pipeline ${item.identifier}`}
                      >
                        <span className="pipeline-detail-object-primary" title={item.identifier}>{pipelineName}</span>
                        <span>{formatStepUsageTeam(item)}</span>
                        <span>{formatStepDetailSource(item.source).label}</span>
                        <span title={item.description || 'No description provided'}>{item.description || 'No description provided'}</span>
                        <span className="step-usage-open-cell">
                          <ArrowRight className="h-4 w-4" aria-hidden="true" />
                        </span>
                      </NavLink>
                    </li>
                  );
                })}
              </ul>
            </div>
          ) : (
            <p className="text-sm text-[var(--text-secondary)]">No pipelines reference this step.</p>
          )}
        </div>
      </section>
    </aside>
  );
}
