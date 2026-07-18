import { ChevronLeft, ChevronRight, Search } from 'lucide-react';
import {
  documentationMetadata,
  documentationSections,
  findDocumentationSectionForArticle,
  getDocumentationNeighbors,
} from './quality';
import type { DocumentationSearchResult } from './search';

export type TocItem = { id: string; label: string };

export function DocumentationSidebar({
  activeArticleID,
  query,
  searchResults,
  onQueryChange,
  onSelectArticle,
  onSelectSearchResult,
}: {
  activeArticleID: string;
  query: string;
  searchResults: DocumentationSearchResult[];
  onQueryChange: (value: string) => void;
  onSelectArticle: (articleID: string) => void;
  onSelectSearchResult: (result: DocumentationSearchResult) => void;
}) {
  const activeSection = findDocumentationSectionForArticle(activeArticleID);

  return (
    <aside className="lg:sticky lg:top-6 lg:max-h-[calc(100vh-3rem)] lg:overflow-y-auto">
      <h2 className="text-lg font-semibold text-[var(--text-primary)]">{documentationMetadata.title}</h2>
      <p className="mt-1 text-xs leading-5 text-[var(--text-tertiary)]">{documentationMetadata.status}</p>
      <label className="mt-5 block">
        <span className="sr-only">Search documentation</span>
        <span className="relative block">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-tertiary)]" aria-hidden="true" />
          <input
            value={query}
            onChange={event => onQueryChange(event.target.value)}
            placeholder="Search fields, guides, runbooks..."
            className="h-10 w-full rounded border border-[var(--border-input)] bg-transparent pl-9 pr-3 text-sm text-[var(--text-primary)] outline-none placeholder:text-[var(--text-placeholder)] focus:border-[var(--border-input-focus)]"
          />
        </span>
      </label>
      <SearchResults query={query} results={searchResults} onSelect={onSelectSearchResult} />
      <nav aria-label="Documentation pages" className="mt-6 space-y-3">
        {documentationSections.map(section => (
          <details key={section.id} open={section.id === activeSection?.id}>
            <summary className="cursor-pointer list-none py-1 text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)] marker:hidden hover:text-[var(--text-primary)]">
              {section.title}
            </summary>
            <ul className="mt-1 space-y-0.5 border-l border-[var(--border-primary)] pl-3">
              {section.articles.map(article => {
                const selected = article.id === activeArticleID;
                return (
                  <li key={article.id}>
                    <button
                      type="button"
                      onClick={() => onSelectArticle(article.id)}
                      aria-current={selected ? 'page' : undefined}
                      className={`w-full rounded px-2 py-1.5 text-left text-sm leading-5 transition ${
                        selected
                          ? 'bg-[var(--bg-secondary)] font-medium text-[var(--text-primary)]'
                          : 'text-[var(--text-secondary)] hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]'
                      }`}
                    >
                      {article.title}
                    </button>
                  </li>
                );
              })}
            </ul>
          </details>
        ))}
      </nav>
    </aside>
  );
}

function SearchResults({
  query,
  results,
  onSelect,
}: {
  query: string;
  results: DocumentationSearchResult[];
  onSelect: (result: DocumentationSearchResult) => void;
}) {
  if (!query.trim()) return null;
  return (
    <section aria-label="Documentation search results" className="mt-3 rounded border border-[var(--border-primary)] bg-[var(--bg-primary)] p-2">
      <div className="flex items-center justify-between gap-3 px-2 py-1">
        <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">Search results</p>
        <span className="text-xs text-[var(--text-tertiary)]">{results.length}</span>
      </div>
      {results.length ? (
        <ul className="mt-1 space-y-1">
          {results.map(result => (
            <li key={result.id}>
              <button type="button" onClick={() => onSelect(result)} className="w-full rounded px-2 py-2 text-left hover:bg-[var(--bg-secondary)]">
                <span className="flex items-center gap-2">
                  <span className="text-xs font-semibold uppercase text-[var(--text-tertiary)]">{result.kind}</span>
                  <span className="min-w-0 truncate text-sm font-medium text-[var(--text-primary)]">{result.title}</span>
                </span>
                <span className="mt-1 block line-clamp-2 text-xs leading-5 text-[var(--text-secondary)]">{result.context}</span>
              </button>
            </li>
          ))}
        </ul>
      ) : <p className="px-2 py-3 text-sm text-[var(--text-secondary)]">No documentation matches this search.</p>}
    </section>
  );
}

export function OnThisPage({ items }: { items: TocItem[] }) {
  if (!items.length) return null;
  return (
    <aside className="hidden xl:block xl:sticky xl:top-6 xl:max-h-[calc(100vh-3rem)] xl:overflow-y-auto" aria-label="On this page">
      <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">On this page</h2>
      <nav className="mt-3 space-y-2">
        {items.map(item => <a key={item.id} className="block text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)]" href={`#${item.id}`}>{item.label}</a>)}
      </nav>
    </aside>
  );
}

export function ArticleNavigation({ articleID, onSelect }: { articleID: string; onSelect: (articleID: string) => void }) {
  const neighbors = getDocumentationNeighbors(articleID);
  return (
    <nav aria-label="Documentation pagination" className="mt-12 grid gap-3 border-t border-[var(--border-primary)] pt-6 sm:grid-cols-2">
      {neighbors.previous ? (
        <button type="button" onClick={() => onSelect(neighbors.previous?.article.id || '')} className="rounded border border-[var(--border-primary)] px-4 py-3 text-left hover:bg-[var(--bg-secondary)]">
          <span className="flex items-center gap-2 text-xs uppercase tracking-wide text-[var(--text-tertiary)]"><ChevronLeft className="h-3.5 w-3.5" />Previous</span>
          <span className="mt-1 block text-sm font-medium text-[var(--text-primary)]">{neighbors.previous.article.title}</span>
        </button>
      ) : <span />}
      {neighbors.next ? (
        <button type="button" onClick={() => onSelect(neighbors.next?.article.id || '')} className="rounded border border-[var(--border-primary)] px-4 py-3 text-right hover:bg-[var(--bg-secondary)]">
          <span className="flex items-center justify-end gap-2 text-xs uppercase tracking-wide text-[var(--text-tertiary)]">Next<ChevronRight className="h-3.5 w-3.5" /></span>
          <span className="mt-1 block text-sm font-medium text-[var(--text-primary)]">{neighbors.next.article.title}</span>
        </button>
      ) : null}
    </nav>
  );
}
