import { ChevronLeft, ChevronRight, Search } from 'lucide-react';
import { Link } from 'react-router-dom';
import {
  getWikiNeighbors,
  wikiArticlePath,
  wikiGroupLabels,
  wikiGroupedSections,
  wikiHomePath,
} from './content/index.js';
import { wikiSearchKindLabels, type WikiSearchResult } from './search.js';

export function WikiSidebar({
  activeArticleID,
  query,
  results,
  onQueryChange,
  onSelectResult,
  showSearch = true,
}: {
  activeArticleID: string;
  query: string;
  results: WikiSearchResult[];
  onQueryChange: (value: string) => void;
  onSelectResult: (result: WikiSearchResult) => void;
  /** The landing page owns its own search field, so the sidebar hides its duplicate there. */
  showSearch?: boolean;
}) {
  const groups = wikiGroupedSections();
  const activeSectionID = groups
    .flatMap(entry => entry.sections)
    .find(section => section.articles.some(article => article.id === activeArticleID))?.id;

  return (
    <aside className="lg:sticky lg:top-4 lg:max-h-[calc(100vh-2rem)] lg:overflow-y-auto lg:pr-1">
      <Link to={wikiHomePath()} className="block text-sm font-semibold text-[var(--text-primary)] hover:underline">
        NopsAI wiki
      </Link>
      {showSearch ? (
        <WikiSearchBox query={query} results={results} onQueryChange={onQueryChange} onSelectResult={onSelectResult} />
      ) : null}
      <nav aria-label="Wiki pages" className="mt-4 space-y-4">
        {groups.map(({ group, sections }) => (
          <div key={group}>
            <p className="px-1 text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]">
              {wikiGroupLabels[group]}
            </p>
            <div className="mt-1 space-y-1">
              {sections.map(section => (
                <details key={section.id} open={section.id === activeSectionID}>
                  <summary className="cursor-pointer list-none rounded px-1 py-1 text-sm font-medium text-[var(--text-secondary)] marker:hidden hover:text-[var(--text-primary)]">
                    {section.title}
                  </summary>
                  <ul className="mt-0.5 space-y-0.5 border-l border-[var(--border-primary)] pl-2">
                    {section.articles.map(article => {
                      const selected = article.id === activeArticleID;
                      return (
                        <li key={article.id}>
                          <Link
                            to={wikiArticlePath(section.id, article.id)}
                            aria-current={selected ? 'page' : undefined}
                            className={`block rounded px-2 py-1 text-sm leading-5 transition ${
                              selected
                                ? 'bg-[var(--bg-tertiary)] font-medium text-[var(--text-primary)]'
                                : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'
                            }`}
                          >
                            {article.title}
                          </Link>
                        </li>
                      );
                    })}
                  </ul>
                </details>
              ))}
            </div>
          </div>
        ))}
      </nav>
    </aside>
  );
}

export function WikiSearchBox({
  query,
  results,
  onQueryChange,
  onSelectResult,
  large = false,
}: {
  query: string;
  results: WikiSearchResult[];
  onQueryChange: (value: string) => void;
  onSelectResult: (result: WikiSearchResult) => void;
  large?: boolean;
}) {
  return (
    <div className={large ? 'mt-6' : 'mt-3'}>
      <label className="block">
        <span className="sr-only">Search the wiki</span>
        <span className="relative block">
          <Search
            className={`pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)] ${
              large ? 'h-4 w-4' : 'h-3.5 w-3.5'
            }`}
            aria-hidden="true"
          />
          <input
            value={query}
            onChange={event => onQueryChange(event.target.value)}
            placeholder={large ? 'Search directives, endpoints, settings, guides…' : 'Search'}
            className={`w-full rounded border border-[var(--border-input)] bg-transparent pr-3 text-[var(--text-primary)] outline-none placeholder:text-[var(--text-placeholder)] focus:border-[var(--border-input-focus)] ${
              large ? 'h-11 pl-10 text-[15px]' : 'h-8 pl-8 text-sm'
            }`}
          />
        </span>
      </label>
      {query.trim() ? <WikiSearchResults results={results} onSelectResult={onSelectResult} /> : null}
    </div>
  );
}

function WikiSearchResults({
  results,
  onSelectResult,
}: {
  results: WikiSearchResult[];
  onSelectResult: (result: WikiSearchResult) => void;
}) {
  if (!results.length) {
    return (
      <p className="mt-2 rounded border border-[var(--border-primary)] px-3 py-3 text-sm text-[var(--text-secondary)]">
        Nothing matches this search.
      </p>
    );
  }
  return (
    <ul aria-label="Search results" className="mt-2 space-y-0.5 rounded border border-[var(--border-primary)] p-1">
      {results.map(result => (
        <li key={result.id}>
          <button
            type="button"
            onClick={() => onSelectResult(result)}
            className="w-full rounded px-2 py-1.5 text-left hover:bg-[var(--bg-tertiary)]"
          >
            <span className="flex items-baseline gap-2">
              <span className="shrink-0 text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
                {wikiSearchKindLabels[result.kind]}
              </span>
              <span className="min-w-0 truncate text-sm font-medium text-[var(--text-primary)]">{result.title}</span>
            </span>
            <span className="mt-0.5 block truncate text-sm text-[var(--text-secondary)]">{result.context}</span>
          </button>
        </li>
      ))}
    </ul>
  );
}

export function WikiOnThisPage({ items }: { items: { id: string; label: string }[] }) {
  if (items.length < 2) return null;
  return (
    <aside className="hidden xl:sticky xl:top-4 xl:block xl:max-h-[calc(100vh-2rem)] xl:overflow-y-auto" aria-label="On this page">
      <p className="text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]">On this page</p>
      <nav className="mt-2 space-y-1.5">
        {items.map(item => (
          <a
            key={item.id}
            href={`#${item.id}`}
            className="block text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
          >
            {item.label}
          </a>
        ))}
      </nav>
    </aside>
  );
}

export function WikiPager({ articleID }: { articleID: string }) {
  const { previous, next } = getWikiNeighbors(articleID);
  if (!previous && !next) return null;
  return (
    <nav aria-label="Wiki pagination" className="mt-10 grid gap-2 border-t border-[var(--border-primary)] pt-5 sm:grid-cols-2">
      {previous ? (
        <Link
          to={wikiArticlePath(previous.section.id, previous.article.id)}
          className="rounded border border-[var(--border-primary)] px-3 py-2 hover:bg-[var(--bg-tertiary)]"
        >
          <span className="flex items-center gap-1 text-xs uppercase tracking-wide text-[var(--text-secondary)]">
            <ChevronLeft className="h-3 w-3" aria-hidden="true" />
            Previous
          </span>
          <span className="mt-0.5 block text-sm font-medium text-[var(--text-primary)]">{previous.article.title}</span>
        </Link>
      ) : (
        <span />
      )}
      {next ? (
        <Link
          to={wikiArticlePath(next.section.id, next.article.id)}
          className="rounded border border-[var(--border-primary)] px-3 py-2 text-right hover:bg-[var(--bg-tertiary)]"
        >
          <span className="flex items-center justify-end gap-1 text-xs uppercase tracking-wide text-[var(--text-secondary)]">
            Next
            <ChevronRight className="h-3 w-3" aria-hidden="true" />
          </span>
          <span className="mt-0.5 block text-sm font-medium text-[var(--text-primary)]">{next.article.title}</span>
        </Link>
      ) : null}
    </nav>
  );
}
