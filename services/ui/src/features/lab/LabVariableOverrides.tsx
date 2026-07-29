import { Plus, X } from 'lucide-react';
import type { LabOverride } from './useLabSession';

type LabVariableOverridesProps = {
  overrides: LabOverride[];
  onAdd: () => void;
  onUpdate: (id: number, field: 'key' | 'value', value: string) => void;
  onRemove: (id: number) => void;
};

export function LabVariableOverrides({
  overrides,
  onAdd,
  onUpdate,
  onRemove,
}: LabVariableOverridesProps) {
  return (
    <section className="glass-card p-4 space-y-3 rounded-lg ring-1 ring-[var(--border-primary)]/70">
      <div className="flex items-start justify-between gap-3">
        <h3 className="text-lg font-semibold text-[var(--text-primary)] leading-tight" style={{ paddingTop: 10 }}>
          Variable overrides
        </h3>
        <button
          id="lab-add-override"
          type="button"
          className="glass-button-primary"
          onClick={onAdd}
          aria-label="Add variable override"
        >
          <Plus className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>

      <div className="lab-overrides-panel">
        {!overrides.length ? (
          <div id="lab-overrides-empty" className="lab-overrides-empty">
            No overrides yet. Leave it blank to inherit scope defaults.
          </div>
        ) : null}
        <div id="lab-overrides-list" className="lab-overrides-list">
          {overrides.map(row => (
            <div key={row.id} className="lab-override-row">
              <div className="lab-override-field">
                <label className="sr-only" htmlFor={`lab-override-${row.id}-key`}>Override key</label>
                <input
                  id={`lab-override-${row.id}-key`}
                  className="pipelines-input lab-override-input"
                  placeholder="key"
                  value={row.key}
                  onChange={event => onUpdate(row.id, 'key', event.target.value)}
                />
              </div>
              <div className="lab-override-field">
                <label className="sr-only" htmlFor={`lab-override-${row.id}-value`}>Override value</label>
                <input
                  id={`lab-override-${row.id}-value`}
                  className="pipelines-input lab-override-input"
                  placeholder="value"
                  value={row.value}
                  onChange={event => onUpdate(row.id, 'value', event.target.value)}
                />
              </div>
              <button
                type="button"
                className="lab-override-remove"
                onClick={() => onRemove(row.id)}
                aria-label="Remove override"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
