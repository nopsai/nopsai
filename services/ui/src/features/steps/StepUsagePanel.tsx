import { NavLink } from 'react-router-dom';
import type { StepUsageItem } from './api';
import { normalizeSource } from './model';

type StepUsagePanelProps = {
  usage: StepUsageItem[];
  loading: boolean;
  error: string | null;
};

export function StepUsagePanel({ usage, loading, error }: StepUsagePanelProps) {
  return (
    <aside className="min-w-0 space-y-6">
      <section className="glass-card overflow-hidden">
        <header className="p-4 border-b border-[var(--border-primary)]" style={{ paddingTop: 4 }}>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">Used in Pipelines</h3>
          <p className="text-xs text-[var(--text-secondary)] mt-1">Pipelines currently importing this step.</p>
        </header>
        <div className="p-4">
          {loading ? (
            <p className="text-sm text-[var(--text-secondary)]">Loading usage…</p>
          ) : error ? (
            <p className="text-sm text-red-500">Failed to load usage: {error}</p>
          ) : usage.length ? (
            <ul className="space-y-2">
              {usage.map(item => (
                <li key={item.identifier}>
                  <NavLink
                    className="glass-card p-3 block hover:border-[var(--border-accent)] transition-colors"
                    to={`/pipelines/${item.identifier.split('/').map(encodeURIComponent).join('/')}`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-sm font-medium text-[var(--text-primary)] truncate">{item.identifier}</span>
                      <span className="text-xs text-[var(--text-secondary)] uppercase">{normalizeSource(item.source)}</span>
                    </div>
                    {item.description ? (
                      <p className="text-xs text-[var(--text-secondary)] mt-1 line-clamp-2">{item.description}</p>
                    ) : null}
                  </NavLink>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-[var(--text-secondary)]">No pipelines reference this step.</p>
          )}
        </div>
      </section>
    </aside>
  );
}
