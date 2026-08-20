import { ChevronDown, ChevronRight } from 'lucide-react';
import { useMemo, useState } from 'react';
import { formatWikiRequired, wikiFieldAnchor, type WikiField } from './content/index.js';
import { InlineMarkup, WikiChip } from './WikiPrimitives.js';

/**
 * Field references render as a scannable table rather than a stack of collapsed
 * cards: the common question is "which directive do I need and is it required?",
 * which a table answers at a glance and an accordion list does not.
 */
export function WikiFieldTable({
  fields,
  targetAnchor,
  searchable = true,
}: {
  fields: WikiField[];
  targetAnchor?: string;
  searchable?: boolean;
}) {
  /**
   * Rows are identified by position as well as path.
   *
   * An anchor is derived from the path, and a page can legitimately document the
   * same path twice — a directive and the limit that bounds it. Keying rows by
   * the anchor alone made those two rows share an identity, so expanding one
   * expanded both and React reused the row across pages.
   */
  const rowKey = (field: WikiField, index: number) => `${index}:${field.scope}:${field.path}`;
  const [query, setQuery] = useState('');
  const [scope, setScope] = useState('');
  const [expanded, setExpanded] = useState<string[]>([]);

  const scopes = useMemo(() => Array.from(new Set(fields.map(field => field.scope))), [fields]);
  const visible = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return fields.map((field, index) => ({ field, key: rowKey(field, index) })).filter(({ field }) => {
      if (scope && field.scope !== scope) return false;
      if (!normalized) return true;
      return (
        field.path.toLowerCase().includes(normalized) ||
        field.description.toLowerCase().includes(normalized) ||
        (field.allowedValues || []).some(value => value.toLowerCase().includes(normalized))
      );
    });
  }, [fields, query, scope]);

  const toggle = (key: string) =>
    setExpanded(current => (current.includes(key) ? current.filter(item => item !== key) : [...current, key]));

  return (
    <div>
      {searchable && fields.length > 8 ? (
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <label className="min-w-[12rem] flex-1">
            <span className="sr-only">Filter fields</span>
            <input
              value={query}
              onChange={event => setQuery(event.target.value)}
              placeholder={`Filter ${fields.length} fields`}
              className="h-8 w-full rounded border border-[var(--border-input)] bg-transparent px-2.5 text-sm text-[var(--text-primary)] outline-none placeholder:text-[var(--text-placeholder)] focus:border-[var(--border-input-focus)]"
            />
          </label>
          {scopes.length > 1 ? (
            <label>
              <span className="sr-only">Filter by scope</span>
              <select
                value={scope}
                onChange={event => setScope(event.target.value)}
                className="h-8 rounded border border-[var(--border-input)] bg-transparent px-2 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--border-input-focus)]"
              >
                <option value="">All scopes</option>
                {scopes.map(item => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </label>
          ) : null}
        </div>
      ) : null}

      <div className="overflow-x-auto rounded border border-[var(--border-primary)]">
        <table className="w-full min-w-[44rem] border-collapse text-left">
          <thead>
            <tr className="border-b border-[var(--border-primary)] text-xs uppercase tracking-wide text-[var(--text-secondary)]">
              <th scope="col" className="w-[32%] px-3 py-2 font-semibold">Directive</th>
              <th scope="col" className="w-[14%] px-3 py-2 font-semibold">Type</th>
              <th scope="col" className="w-[12%] px-3 py-2 font-semibold">Required</th>
              <th scope="col" className="px-3 py-2 font-semibold">Default</th>
            </tr>
          </thead>
          <tbody>
            {visible.map(({ field, key }) => {
              const anchor = wikiFieldAnchor(field.path, field.scope);
              // A deep link opens the first row carrying that anchor.
              const isOpen = expanded.includes(key) || (Boolean(targetAnchor) && targetAnchor === anchor);
              const hasDetail = Boolean(
                field.allowedValues?.length ||
                  field.constraints?.length ||
                  field.overriddenBy?.length ||
                  field.permission ||
                  field.security ||
                  field.deprecatedIn ||
                  field.example,
              );
              return (
                <FieldRows
                  key={key}
                  field={field}
                  anchor={anchor}
                  showScope={scopes.length > 1}
                  isOpen={isOpen}
                  hasDetail={hasDetail}
                  onToggle={() => toggle(key)}
                />
              );
            })}
            {visible.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-3 py-6 text-center text-sm text-[var(--text-secondary)]">
                  No field matches this filter.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function FieldRows({
  field,
  anchor,
  showScope,
  isOpen,
  hasDetail,
  onToggle,
}: {
  field: WikiField;
  anchor: string;
  /** Tables that mix scopes name the owning document on every row. */
  showScope: boolean;
  isOpen: boolean;
  hasDetail: boolean;
  onToggle: () => void;
}) {
  const required = formatWikiRequired(field.required);
  return (
    <>
      <tr id={anchor} className="scroll-mt-8 border-b border-[var(--border-primary)] align-top last:border-b-0">
        <td className="px-3 py-2">
          <button
            type="button"
            onClick={onToggle}
            aria-expanded={hasDetail ? isOpen : undefined}
            disabled={!hasDetail}
            className="flex w-full items-start gap-1.5 text-left disabled:cursor-default"
          >
            {hasDetail ? (
              isOpen ? (
                <ChevronDown className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--text-tertiary)]" aria-hidden="true" />
              ) : (
                <ChevronRight className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--text-tertiary)]" aria-hidden="true" />
              )
            ) : (
              <span className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            )}
            <span className="min-w-0">
              <code className="break-all text-sm font-semibold text-[var(--text-primary)]">{field.path}</code>
              {showScope ? (
                <span className="ml-2 text-xs text-[var(--text-tertiary)]">{field.scope}</span>
              ) : null}
              {field.deprecatedIn ? <span className="ml-2 text-xs text-amber-500">deprecated</span> : null}
              <span className="mt-0.5 block text-sm leading-6 text-[var(--text-secondary)]">
                <InlineMarkup value={field.description} />
              </span>
            </span>
          </button>
        </td>
        <td className="px-3 py-2 text-sm text-[var(--text-secondary)]">{field.type}</td>
        <td className="px-3 py-2 text-sm">
          <span className={required === 'Required' ? 'font-medium text-[var(--text-primary)]' : 'text-[var(--text-secondary)]'}>
            {required}
          </span>
        </td>
        <td className="px-3 py-2 text-sm text-[var(--text-secondary)]">{field.defaultValue}</td>
      </tr>
      {isOpen && hasDetail ? (
        <tr className="border-b border-[var(--border-primary)] last:border-b-0">
          <td colSpan={4} className="bg-[var(--bg-tertiary)] px-3 py-3">
            <FieldDetail field={field} />
          </td>
        </tr>
      ) : null}
    </>
  );
}

function FieldDetail({ field }: { field: WikiField }) {
  return (
    <div className="space-y-3 text-sm leading-6 text-[var(--text-secondary)]">
      <p className="flex flex-wrap items-center gap-2">
        <WikiChip>scope: {field.scope}</WikiChip>
        {field.deprecatedIn ? <WikiChip>{field.deprecatedIn}</WikiChip> : null}
      </p>
      {field.allowedValues?.length ? (
        <p className="flex flex-wrap items-center gap-1.5">
          <span className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Allowed</span>
          {field.allowedValues.map(value => (
            <code key={value} className="rounded border border-[var(--border-primary)] bg-[var(--bg-primary)] px-1.5 py-0.5 text-sm text-[var(--text-primary)]">
              {value}
            </code>
          ))}
        </p>
      ) : null}
      {field.constraints?.length ? (
        <div>
          <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Rules</p>
          <ul className="mt-1 space-y-1 pl-5">
            {field.constraints.map(item => (
              <li key={item} className="list-disc">
                <InlineMarkup value={item} />
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {field.overriddenBy?.length ? (
        <p>
          <span className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Overridden by </span>
          {field.overriddenBy.map(item => (
            <code key={item} className="mr-1.5 rounded border border-[var(--border-primary)] bg-[var(--bg-primary)] px-1.5 py-0.5 text-sm">
              {item}
            </code>
          ))}
        </p>
      ) : null}
      {field.permission ? (
        <p>
          <span className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Permission </span>
          <InlineMarkup value={field.permission} />
        </p>
      ) : null}
      {field.security ? (
        <p className="rounded border border-amber-500/40 px-2.5 py-1.5">
          <span className="text-xs font-semibold uppercase tracking-wide text-amber-600">Security </span>
          <InlineMarkup value={field.security} />
        </p>
      ) : null}
      <div>
        <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Example</p>
        <pre className="mt-1 overflow-x-auto rounded border border-[var(--border-primary)] bg-[var(--bg-primary)] px-2.5 py-2 text-sm leading-6 text-[var(--text-primary)]">
          <code>{field.example}</code>
        </pre>
      </div>
      {field.evidence ? (
        <p className="text-sm text-[var(--text-secondary)]">
          Verified against <code className="text-[var(--text-primary)]">{field.evidence}</code>
        </p>
      ) : null}
    </div>
  );
}
