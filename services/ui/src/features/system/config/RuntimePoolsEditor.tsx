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
  const [selectedPoolName, setSelectedPoolName] = useState('');
  const activePoolName = poolNames.includes(selectedPoolName) ? selectedPoolName : poolNames[0] || '';
  const activePool = activePoolName ? pools[activePoolName] || emptyPool() : null;

  const updatePool = (name: string, pool: RuntimePoolConfig) => {
    onChange({ ...pools, [name]: pool });
  };

  const addPool = () => {
    const nextName = nextPoolName(pools);
    onChange({ ...pools, [nextName]: emptyPool() });
    setSelectedPoolName(nextName);
  };

  const renamePool = (currentName: string, nextName: string) => {
    if (currentName === nextName) return true;
    if (!RUNTIME_POOL_NAME_PATTERN.test(nextName) || Object.prototype.hasOwnProperty.call(pools, nextName)) return false;
    const next = { ...pools };
    const pool = next[currentName] || emptyPool();
    delete next[currentName];
    next[nextName] = pool;
    onChange(next);
    setSelectedPoolName(nextName);
    return true;
  };

  const removePool = (name: string) => {
    const next = { ...pools };
    delete next[name];
    onChange(next);
    const remainingPoolName = sortPoolNames(Object.keys(next))[0] || '';
    setSelectedPoolName(remainingPoolName);
  };

  return (
    <div className="system-settings-card system-settings-card--tool system-settings-runtime-pools">
      <div className="system-settings-card__toolbar system-settings-card__toolbar--compact">
        <div>
          <p className="system-settings-card__eyebrow">Kubernetes scheduling</p>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">
            <span className="inline-flex flex-wrap items-center gap-2">
              <span>Runtime pools</span>
              <ApplyBadge metadata={metadata} />
            </span>
          </h3>
        </div>
        <button
          type="button"
          className="glass-button-subtle"
          onClick={addPool}
          disabled={disabled}
        >
          Add runtime pool
        </button>
      </div>

      {!poolNames.length ? (
        <div className="system-settings-inline-alert system-settings-inline-alert--empty">
          No runtime pools configured. Kubernetes runners use their default scheduling.
        </div>
      ) : (
        <div className="system-settings-runtime-pool-workbench">
          <div className="system-settings-runtime-pool-list" aria-label="Runtime pools">
            {poolNames.map(name => {
              const pool = pools[name] || emptyPool();
              const ruleCount = runtimePoolRuleCount(pool);
              return (
                <button
                  key={name}
                  type="button"
                  className={`system-settings-runtime-pool-item ${activePoolName === name ? 'is-active' : ''}`}
                  onClick={() => setSelectedPoolName(name)}
                  aria-pressed={activePoolName === name}
                  aria-label={`Edit runtime pool ${name}`}
                >
                  <span>
                    <strong>{name}</strong>
                    <small>{runtimePoolSummary(pool)}</small>
                  </span>
                  <em>{ruleCount === 1 ? '1 rule' : `${ruleCount} rules`}</em>
                </button>
              );
            })}
          </div>
          {activePool && (
            <RuntimePoolSection
              key={activePoolName}
              name={activePoolName}
              pool={activePool}
              disabled={disabled}
              onRename={nextName => renamePool(activePoolName, nextName)}
              onRemove={() => removePool(activePoolName)}
              onChange={pool => updatePool(activePoolName, pool)}
            />
          )}
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
    <section className="system-settings-runtime-pool-detail" aria-label={`Runtime pool ${name}`}>
      <div className="system-settings-runtime-pool__header">
        <label className="system-settings-field">
          <span className="system-settings-field__label">Pool name</span>
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
          className="glass-button-danger"
          onClick={onRemove}
          disabled={disabled}
        >
          Remove pool
        </button>
      </div>

      <div className="system-settings-runtime-map-grid">
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
      </div>
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
    <div className="system-settings-runtime-map">
      <div className="system-settings-runtime-map__header">
        <p>{title}</p>
        <button
          type="button"
          className="glass-button-ghost"
          onClick={addRow}
          disabled={disabled}
          aria-label={`Add ${title.toLowerCase()} to ${poolName}`}
        >
          Add row
        </button>
      </div>

      {!entries.length ? (
        <p className="system-settings-muted">No {title.toLowerCase()} configured.</p>
      ) : (
        <div className="system-settings-runtime-map__rows">
          {entries.map(([key, mapValue], index) => (
            <div key={`${poolName}-${title}-${index}`} className="system-settings-runtime-map__row">
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
                className="glass-button-ghost"
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

function runtimePoolRuleCount(pool: RuntimePoolConfig) {
  return (
    Object.keys(pool.node_selector || {}).length +
    Object.keys(pool.resources?.requests || {}).length +
    Object.keys(pool.resources?.limits || {}).length
  );
}

function runtimePoolSummary(pool: RuntimePoolConfig) {
  const selectors = Object.keys(pool.node_selector || {}).length;
  const requests = Object.keys(pool.resources?.requests || {}).length;
  const limits = Object.keys(pool.resources?.limits || {}).length;
  return `${selectors} selectors / ${requests} requests / ${limits} limits`;
}
