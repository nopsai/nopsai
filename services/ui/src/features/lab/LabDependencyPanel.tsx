import type { LabIncludedDependencies } from './model';

type LabDependencyPanelProps = {
  dependencies: LabIncludedDependencies;
};

function splitDependencyLabel(value: string) {
  const separator = value.indexOf(':');
  if (separator <= 0) return { kind: 'dependency', target: value };
  return {
    kind: value.slice(0, separator),
    target: value.slice(separator + 1),
  };
}

export function LabDependencyPanel({ dependencies }: LabDependencyPanelProps) {
  let content = <p>No included dependencies found.</p>;
  if (dependencies.status === 'no-steps') {
    content = <p>No steps defined yet.</p>;
  } else if (dependencies.status === 'parse-error' || dependencies.status === 'invalid') {
    content = <p>Unable to parse pipeline YAML.</p>;
  } else if (dependencies.items.length > 0) {
    content = (
      <ul className="lab-dependency-list">
        {dependencies.items.map(item => {
          const dependency = splitDependencyLabel(item);
          return (
            <li key={item}>
              <span>{dependency.kind}</span>
              <strong>{dependency.target}</strong>
            </li>
          );
        })}
      </ul>
    );
  }

  return (
    <section className="glass-card lab-dependency-panel" aria-label="Included dependencies">
      <div className="lab-dependency-panel__head">
        <h3>Included dependencies</h3>
        {dependencies.items.length ? <span>{dependencies.items.length}</span> : null}
      </div>
      <div id="lab-includes" className="lab-dependency-panel__body" data-empty="No steps defined yet.">
        {content}
      </div>
    </section>
  );
}
