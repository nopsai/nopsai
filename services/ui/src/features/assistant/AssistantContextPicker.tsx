import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Loader2, Search } from 'lucide-react';
import {
  assistantContextKinds,
  fetchAssistantContextOptions,
  filterAssistantContextOptions,
  type AssistantContextKind,
  type AssistantContextOption,
} from './contextOptions.js';

/**
 * Chooses what the chat is about when the route cannot say. Options are loaded
 * per kind on demand and cached for the life of the popover, so switching tabs
 * back and forth does not re-query.
 */
export function AssistantContextPicker({
  compact,
  onSelect,
  onClose,
}: {
  compact: boolean;
  onSelect: (option: AssistantContextOption) => void;
  onClose: () => void;
}) {
  const [kind, setKind] = useState<AssistantContextKind>('pipeline');
  const [query, setQuery] = useState('');
  const [optionsByKind, setOptionsByKind] = useState<Partial<Record<AssistantContextKind, AssistantContextOption[]>>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);
  // Each kind is fetched once per popover; switching tabs back reuses the rows.
  const loadedKinds = useRef(new Set<AssistantContextKind>());
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    searchRef.current?.focus();
  }, []);

  useEffect(() => {
    const handlePointerDown = (event: MouseEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) onClose();
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    document.addEventListener('mousedown', handlePointerDown);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [onClose]);

  const loadOptions = useCallback(async (target: AssistantContextKind, cancelled: () => boolean) => {
    setLoading(true);
    setError('');
    try {
      const options = await fetchAssistantContextOptions(target);
      if (!cancelled()) setOptionsByKind(current => ({ ...current, [target]: options }));
    } catch (err) {
      // A failed load is not a loaded kind: returning to the tab should retry.
      loadedKinds.current.delete(target);
      if (!cancelled()) setError(err instanceof Error ? err.message : 'Unable to load context options');
    } finally {
      if (!cancelled()) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (loadedKinds.current.has(kind)) return;
    loadedKinds.current.add(kind);
    let active = true;
    void loadOptions(kind, () => !active);
    return () => {
      active = false;
    };
  }, [kind, loadOptions]);

  const visible = useMemo(() => filterAssistantContextOptions(optionsByKind[kind] || [], query), [optionsByKind, kind, query]);

  return (
    <div
      ref={containerRef}
      className={`absolute bottom-full left-0 z-30 mb-2 w-full overflow-hidden rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-xl ${compact ? '' : 'max-w-md'}`}
      role="dialog"
      aria-label="Select chat context"
    >
      <div className="flex flex-wrap gap-1 border-b border-[var(--border-primary)] p-2">
        {assistantContextKinds.map(entry => (
          <button
            key={entry.kind}
            type="button"
            className={`rounded-md px-2 py-1 text-xs transition-colors ${
              entry.kind === kind
                ? 'bg-[var(--border-accent)] text-white'
                : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'
            }`}
            aria-pressed={entry.kind === kind}
            onClick={() => {
              setKind(entry.kind);
              setQuery('');
            }}
          >
            {entry.label}
          </button>
        ))}
      </div>
      <label className="flex items-center gap-2 border-b border-[var(--border-primary)] px-3 py-2">
        <Search className="h-3.5 w-3.5 shrink-0 text-[var(--text-secondary)]" aria-hidden="true" />
        <span className="sr-only">Search context</span>
        <input
          ref={searchRef}
          className="w-full bg-transparent text-sm text-[var(--text-primary)] outline-none placeholder:text-[var(--text-placeholder)]"
          value={query}
          onChange={event => setQuery(event.target.value)}
          placeholder="Search"
          aria-label="Search context"
        />
      </label>
      <div className="max-h-64 overflow-y-auto py-1">
        {loading && (
          <p className="flex items-center gap-2 px-3 py-3 text-xs text-[var(--text-secondary)]">
            <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
            Loading
          </p>
        )}
        {!loading && error && <p className="px-3 py-3 text-xs text-rose-600 dark:text-rose-400">{error}</p>}
        {!loading && !error && visible.length === 0 && (
          <p className="px-3 py-3 text-xs text-[var(--text-secondary)]">Nothing to select here.</p>
        )}
        {!loading && !error && visible.map(option => (
          <button
            key={`${option.kind}:${option.id}`}
            type="button"
            className="flex w-full flex-col items-start gap-0.5 px-3 py-2 text-left transition-colors hover:bg-[var(--bg-tertiary)]"
            onClick={() => onSelect(option)}
          >
            <span className="w-full truncate text-sm text-[var(--text-primary)]">{option.label}</span>
            {option.detail && <span className="w-full truncate text-xs text-[var(--text-secondary)]">{option.detail}</span>}
          </button>
        ))}
      </div>
    </div>
  );
}
