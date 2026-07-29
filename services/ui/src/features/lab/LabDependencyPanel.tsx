import type { LabIncludedDependencies } from './model';

type LabDependencyPanelProps = {
  dependencies: LabIncludedDependencies;
};

export function LabDependencyPanel({ dependencies }: LabDependencyPanelProps) {
  let content = <p>No included dependencies found.</p>;
  if (dependencies.status === 'no-steps') {
    content = <p>No steps defined yet.</p>;
  } else if (dependencies.status === 'parse-error' || dependencies.status === 'invalid') {
    content = <p>Unable to parse pipeline YAML.</p>;
  } else if (dependencies.items.length > 0) {
    content = (
      <ul className="triggers-pipeline-list">
        {dependencies.items.map(item => (
          <li key={item} className="triggers-pipeline-item">
            <span className="triggers-pipeline-name">{item}</span>
          </li>
        ))}
      </ul>
    );
  }

  return (
    <section className="glass-card p-4 space-y-2 rounded-lg shadow-sm ring-1 ring-[var(--border-primary)]/70">
      <h3 className="text-sm font-semibold text-[var(--text-primary)]">Included dependencies</h3>
      <div id="lab-includes" className="text-sm text-[var(--text-secondary)] space-y-2" data-empty="No steps defined yet.">
        {content}
      </div>
    </section>
  );
}
