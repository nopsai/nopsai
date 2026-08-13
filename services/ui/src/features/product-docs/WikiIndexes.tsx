import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { formatWikiRequired, wikiRouteAccessLabels } from './content/index.js';
import {
  apiAreas,
  apiRouteIndex,
  directiveIndex,
  directiveScopes,
  environmentIndex,
  environmentScopes,
  filterApiRoutes,
  filterIndexedFields,
  type IndexArticleID,
  type IndexedField,
} from './indexes.js';
import { InlineMarkup } from './WikiPrimitives.js';

export function WikiIndex({ indexID }: { indexID: IndexArticleID }) {
  if (indexID === 'api-index') return <ApiIndexTable />;
  if (indexID === 'environment-index') {
    return <FieldIndexTable rows={environmentIndex} scopes={environmentScopes()} scopeLabel="service" columnLabel="Variable" />;
  }
  return <FieldIndexTable rows={directiveIndex} scopes={directiveScopes()} scopeLabel="scope" columnLabel="Directive" />;
}

function IndexToolbar({
  query,
  onQueryChange,
  filter,
  onFilterChange,
  options,
  optionLabel,
  count,
  total,
  noun,
}: {
  query: string;
  onQueryChange: (value: string) => void;
  filter: string;
  onFilterChange: (value: string) => void;
  options: string[];
  optionLabel: string;
  count: number;
  total: number;
  noun: string;
}) {
  return (
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <label className="min-w-[14rem] flex-1">
        <span className="sr-only">Filter {noun}</span>
        <input
          value={query}
          onChange={event => onQueryChange(event.target.value)}
          placeholder={`Search ${total} ${noun}`}
          className="h-8 w-full rounded border border-[var(--border-input)] bg-transparent px-2.5 text-sm text-[var(--text-primary)] outline-none placeholder:text-[var(--text-placeholder)] focus:border-[var(--border-input-focus)]"
        />
      </label>
      <label>
        <span className="sr-only">Filter by {optionLabel}</span>
        <select
          value={filter}
          onChange={event => onFilterChange(event.target.value)}
          className="h-8 rounded border border-[var(--border-input)] bg-transparent px-2 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--border-input-focus)]"
        >
          <option value="">All {optionLabel}s</option>
          {options.map(option => (
            <option key={option} value={option}>
              {option}
            </option>
          ))}
        </select>
      </label>
      <span className="text-sm text-[var(--text-secondary)]" aria-live="polite">
        {count} of {total}
      </span>
    </div>
  );
}

function FieldIndexTable({
  rows,
  scopes,
  scopeLabel,
  columnLabel,
}: {
  rows: IndexedField[];
  scopes: string[];
  scopeLabel: string;
  columnLabel: string;
}) {
  const [query, setQuery] = useState('');
  const [scope, setScope] = useState('');
  const visible = useMemo(() => filterIndexedFields(rows, query, scope), [rows, query, scope]);

  return (
    <div>
      <IndexToolbar
        query={query}
        onQueryChange={setQuery}
        filter={scope}
        onFilterChange={setScope}
        options={scopes}
        optionLabel={scopeLabel}
        count={visible.length}
        total={rows.length}
        noun={columnLabel.toLowerCase() + 's'}
      />
      <div className="overflow-x-auto rounded border border-[var(--border-primary)]">
        <table className="w-full min-w-[52rem] border-collapse text-left">
          <thead>
            <tr className="border-b border-[var(--border-primary)] text-xs uppercase tracking-wide text-[var(--text-secondary)]">
              <th scope="col" className="w-[26%] px-3 py-2 font-semibold">{columnLabel}</th>
              <th scope="col" className="w-[13%] px-3 py-2 font-semibold">{scopeLabel}</th>
              <th scope="col" className="w-[10%] px-3 py-2 font-semibold">Required</th>
              <th scope="col" className="w-[16%] px-3 py-2 font-semibold">Default</th>
              <th scope="col" className="px-3 py-2 font-semibold">Description</th>
            </tr>
          </thead>
          <tbody>
            {visible.map(row => (
              <tr key={`${row.scope}-${row.path}`} className="border-b border-[var(--border-primary)] align-top last:border-b-0">
                <td className="px-3 py-2">
                  <Link to={row.href} className="break-all text-sm font-medium text-[var(--text-accent)] hover:underline">
                    <code>{row.path}</code>
                  </Link>
                  {row.deprecatedIn ? <span className="ml-1.5 text-xs text-amber-500">deprecated</span> : null}
                </td>
                <td className="px-3 py-2 text-sm text-[var(--text-secondary)]">{row.scope}</td>
                <td className="px-3 py-2 text-sm text-[var(--text-secondary)]">{formatWikiRequired(row.required)}</td>
                <td className="px-3 py-2 text-sm text-[var(--text-secondary)]">{row.defaultValue}</td>
                <td className="px-3 py-2 text-sm leading-6 text-[var(--text-secondary)]">
                  <InlineMarkup value={row.description} />
                  <span className="mt-0.5 block text-xs text-[var(--text-secondary)]">{row.articleTitle}</span>
                </td>
              </tr>
            ))}
            {visible.length === 0 ? (
              <tr>
                <td colSpan={5} className="px-3 py-6 text-center text-sm text-[var(--text-secondary)]">
                  Nothing matches this filter.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ApiIndexTable() {
  const [query, setQuery] = useState('');
  const [area, setArea] = useState('');
  const areas = useMemo(() => apiAreas(), []);
  const visible = useMemo(() => filterApiRoutes(apiRouteIndex, query, area), [query, area]);

  return (
    <div>
      <IndexToolbar
        query={query}
        onQueryChange={setQuery}
        filter={area}
        onFilterChange={setArea}
        options={areas}
        optionLabel="area"
        count={visible.length}
        total={apiRouteIndex.length}
        noun="endpoints"
      />
      <div className="overflow-x-auto rounded border border-[var(--border-primary)]">
        <table className="w-full min-w-[54rem] border-collapse text-left">
          <thead>
            <tr className="border-b border-[var(--border-primary)] text-xs uppercase tracking-wide text-[var(--text-secondary)]">
              <th scope="col" className="w-[7%] px-3 py-2 font-semibold">Method</th>
              <th scope="col" className="w-[32%] px-3 py-2 font-semibold">Path</th>
              <th scope="col" className="w-[13%] px-3 py-2 font-semibold">Access</th>
              <th scope="col" className="px-3 py-2 font-semibold">Purpose</th>
            </tr>
          </thead>
          <tbody>
            {visible.map(route => (
              <tr
                key={`${route.method}-${route.path}`}
                className="border-b border-[var(--border-primary)] align-top last:border-b-0"
              >
                <td className="px-3 py-2 text-sm font-semibold text-[var(--text-primary)]">{route.method}</td>
                <td className="px-3 py-2">
                  <code className="break-all text-sm text-[var(--text-primary)]">{route.path}</code>
                  <span className="mt-0.5 block text-xs text-[var(--text-secondary)]">{route.area}</span>
                </td>
                <td className="px-3 py-2 text-sm text-[var(--text-secondary)]">{wikiRouteAccessLabels[route.access]}</td>
                <td className="px-3 py-2 text-sm leading-6 text-[var(--text-secondary)]">
                  <InlineMarkup value={route.purpose} />
                  {route.notes ? (
                    <span className="mt-0.5 block text-xs text-[var(--text-secondary)]">
                      <InlineMarkup value={route.notes} />
                    </span>
                  ) : null}
                </td>
              </tr>
            ))}
            {visible.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-3 py-6 text-center text-sm text-[var(--text-secondary)]">
                  Nothing matches this filter.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}
