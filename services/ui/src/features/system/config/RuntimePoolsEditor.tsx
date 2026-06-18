import { useEffect, useState, type KeyboardEvent } from 'react';
import type { ConfigFieldMetadata, RuntimePoolConfig, RuntimePoolsConfig } from './model';
import { ApplyBadge } from './ConfigApplyBadge';

type RuntimePoolsEditorProps = {
  value: RuntimePoolsConfig;
  metadata?: ConfigFieldMetadata;
  disabled: boolean;
  onChange: (next: RuntimePoolsConfig) => void;
};

type RuntimePoolMapKind = 'node_selector' | 'requests' | 'limits';

const RUNTIME_POOL_NAME_PATTERN = /^[A-Za-z0-9_.-]+$/;

const emptyPool = (): RuntimePoolConfig => ({
  node_selector: {},
  resources: {
    requests: {},
    limits: {},
  },
});

export function RuntimePoolsEditor({ value, metadata, disabled, onChange }: RuntimePoolsEditorProps) {
  const pools = value || {};
  const poolNames = sortPoolNames(Object.keys(pools));

  const updatePool = (name: string, pool: RuntimePoolConfig) => {
    onChange({ ...pools, [name]: pool });
  };

  const addPool = () => {
    const nextName = nextPoolName(pools);
    onChange({ ...pools, [nextName]: emptyPool() });
  };

  const renamePool = (currentName: string, nextName: string) => {
    if (currentName === nextName) return true;
    if (!RUNTIME_POOL_NAME_PATTERN.test(nextName) || Object.prototype.hasOwnProperty.call(pools, nextName)) return false;
    const next = { ...pools };
    const pool = next[currentName] || emptyPool();
    delete next[currentName];
    next[nextName] = pool;
    onChange(next);
    return true;
  };

  const removePool = (name: string) => {
    const next = { ...pools };
    delete next[name];
    onChange(next);
  };

  return (
    <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <p className="text-xs text-[var(--text-secondary)]">Kubernetes scheduling</p>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">
            <span className="inline-flex flex-wrap items-center gap-2">
              <span>Runtime pools</span>
              <ApplyBadge metadata={metadata} />
            </span>
          </h3>
        </div>
        <button
          type="button"
          className="glass-button-subtle self-start"
          onClick={addPool}
          disabled={disabled}
        >
          Add runtime pool
        </button>
      </div>

      {!poolNames.length ? (
        <div className="rounded-lg border border-dashed border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-sm text-[var(--text-secondary)]">
          No runtime pools configured. Kubernetes runners use their default scheduling.
        </div>
      ) : (
        <div className="space-y-3">
          {poolNames.map(name => (
            <RuntimePoolSection
              key={name}
              name={name}
              pool={pools[name] || emptyPool()}
              disabled={disabled}
              onRename={nextName => renamePool(name, nextName)}
              onRemove={() => removePool(name)}
              onChange={pool => updatePool(name, pool)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function RuntimePoolSection({
  name,
  pool,
  disabled,
  onRename,
  onRemove,
  onChange,
}: {
  name: string;
  pool: RuntimePoolConfig;
  disabled: boolean;
  onRename: (nextName: string) => boolean;
  onRemove: () => void;
  onChange: (pool: RuntimePoolConfig) => void;
}) {
  const [draftName, setDraftName] = useState(name);

  useEffect(() => {
    setDraftName(name);
  }, [name]);

  const commitName = () => {
    const nextName = draftName.trim();
    if (nextName === name) {
      setDraftName(name);
      return;
    }
    if (!RUNTIME_POOL_NAME_PATTERN.test(nextName) || !onRename(nextName)) {
      setDraftName(name);
    }
  };

  const handleNameKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== 'Enter') return;
    event.preventDefault();
    commitName();
  };

  const updateMap = (kind: RuntimePoolMapKind, nextMap: Record<string, string>) => {
    if (kind === 'node_selector') {
      onChange({ ...pool, node_selector: nextMap });
      return;
    }
    onChange({
      ...pool,
      resources: {
        ...pool.resources,
        [kind]: nextMap,
      },
    });
  };

  return (
    <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 space-y-4">
      <div className="grid grid-cols-1 md:grid-cols-[minmax(0,1fr)_auto] gap-3 items-end">
        <label className="flex flex-col gap-1 text-sm">
          <span>Pool name</span>
          <input
            type="text"
            className="pipelines-input"
            value={draftName}
            onChange={event => setDraftName(event.target.value)}
            onBlur={commitName}
            onKeyDown={handleNameKeyDown}
            aria-label={`Runtime pool name ${name}`}
            placeholder="high-memory"
            pattern="[A-Za-z0-9_.-]+"
            title="Use letters, numbers, dots, underscores, and hyphens."
            disabled={disabled}
          />
        </label>
        <button
          type="button"
          className="glass-button-danger justify-center"
          onClick={onRemove}
          disabled={disabled}
        >
          Remove pool
        </button>
      </div>

      <RuntimePoolMapEditor
        poolName={name}
        title="Node selector"
        keyPlaceholder="node-class"
        valuePlaceholder="memory"
        value={pool.node_selector || {}}
        disabled={disabled}
        onChange={next => updateMap('node_selector', next)}
      />
      <RuntimePoolMapEditor
        poolName={name}
        title="Resource requests"
        keyPlaceholder="memory"
        valuePlaceholder="4Gi"
        value={pool.resources?.requests || {}}
        disabled={disabled}
        onChange={next => updateMap('requests', next)}
      />
      <RuntimePoolMapEditor
        poolName={name}
        title="Resource limits"
        keyPlaceholder="memory"
        valuePlaceholder="16Gi"
        value={pool.resources?.limits || {}}
        disabled={disabled}
        onChange={next => updateMap('limits', next)}
      />
    </section>
  );
}

function RuntimePoolMapEditor({
  poolName,
  title,
  keyPlaceholder,
  valuePlaceholder,
  value,
  disabled,
  onChange,
}: {
  poolName: string;
  title: string;
  keyPlaceholder: string;
  valuePlaceholder: string;
  value: Record<string, string>;
  disabled: boolean;
  onChange: (next: Record<string, string>) => void;
}) {
  const entries = Object.entries(value || {});

  const addRow = () => {
    onChange({ ...value, [nextMapKey(value, keyPlaceholder)]: '' });
  };

  const updateKey = (currentKey: string, nextKey: string) => {
    if (currentKey === nextKey) return;
    if (!nextKey.trim() || Object.prototype.hasOwnProperty.call(value, nextKey)) return;
    const next = { ...value };
    const currentValue = next[currentKey] || '';
    delete next[currentKey];
    next[nextKey] = currentValue;
    onChange(next);
  };

  const updateValue = (key: string, nextValue: string) => {
    onChange({ ...value, [key]: nextValue });
  };

  const removeRow = (key: string) => {
    const next = { ...value };
    delete next[key];
    onChange(next);
  };

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{title}</p>
        <button
          type="button"
          className="glass-button-ghost px-3 py-1 text-xs"
          onClick={addRow}
          disabled={disabled}
          aria-label={`Add ${title.toLowerCase()} to ${poolName}`}
        >
          Add row
        </button>
      </div>

      {!entries.length ? (
        <p className="text-xs text-[var(--text-secondary)]">No {title.toLowerCase()} configured.</p>
      ) : (
        <div className="space-y-2">
          {entries.map(([key, mapValue], index) => (
            <div key={`${poolName}-${title}-${index}`} className="grid grid-cols-1 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-2">
              <input
                type="text"
                className="pipelines-input"
                value={key}
                onChange={event => updateKey(key, event.target.value)}
                aria-label={`${poolName} ${title} key ${index + 1}`}
                placeholder={keyPlaceholder}
                disabled={disabled}
              />
              <input
                type="text"
                className="pipelines-input"
                value={mapValue}
                onChange={event => updateValue(key, event.target.value)}
                aria-label={`${poolName} ${title} value ${index + 1}`}
                placeholder={valuePlaceholder}
                disabled={disabled}
              />
              <button
                type="button"
                className="glass-button-ghost justify-center"
                onClick={() => removeRow(key)}
                disabled={disabled}
                aria-label={`Remove ${poolName} ${title} row ${index + 1}`}
              >
                Remove
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function nextPoolName(pools: RuntimePoolsConfig) {
  if (!Object.prototype.hasOwnProperty.call(pools, 'default')) return 'default';
  let index = 1;
  while (Object.prototype.hasOwnProperty.call(pools, `pool-${index}`)) {
    index += 1;
  }
  return `pool-${index}`;
}

function nextMapKey(value: Record<string, string>, fallback: string) {
  if (!Object.prototype.hasOwnProperty.call(value, fallback)) return fallback;
  let index = 2;
  while (Object.prototype.hasOwnProperty.call(value, `${fallback}_${index}`)) {
    index += 1;
  }
  return `${fallback}_${index}`;
}

function sortPoolNames(names: string[]) {
  return [...names].sort((a, b) => {
    if (a === 'default' && b !== 'default') return -1;
    if (b === 'default' && a !== 'default') return 1;
    return a.localeCompare(b);
  });
}
