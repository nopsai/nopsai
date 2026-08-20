import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Menu, Moon, Search, Sun, X } from 'lucide-react';
import { Link, useLocation } from 'react-router-dom';
import BrandIdentity from '../../components/BrandIdentity';
import { wikiArticlePath, wikiHomePath, wikiSections } from './content/index.js';
import { wikiSearchKindLabels, type WikiSearchResult } from './search.js';

export type DocsTocItem = { id: string; label: string };

/**
 * Standalone documentation chrome.
 *
 * The wiki is a documentation site rather than an operator workspace, so it
 * owns the whole viewport: fixed header, fixed sidebar, fixed on-this-page
 * rail. Layout metrics live in the `.docs-*` rules in styles.css so the design
 * contract asserted by docsShell.test.ts is stated once.
 */
export function DocsShell({
  activeArticleID,
  query,
  results,
  onQueryChange,
  onSelectResult,
  productVersion,
  theme,
  onToggleTheme,
  toc,
  children,
}: {
  activeArticleID: string;
  query: string;
  results: WikiSearchResult[];
  onQueryChange: (value: string) => void;
  onSelectResult: (result: WikiSearchResult) => void;
  productVersion?: string;
  theme?: 'light' | 'dark';
  onToggleTheme?: () => void;
  toc: DocsTocItem[];
  children: ReactNode;
}) {
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <div className="docs-shell">
      <DocsHeader
        query={query}
        results={results}
        onQueryChange={onQueryChange}
        onSelectResult={onSelectResult}
        productVersion={productVersion}
        theme={theme}
        onToggleTheme={onToggleTheme}
        menuOpen={menuOpen}
        onToggleMenu={() => setMenuOpen(open => !open)}
      />
      <div className="docs-layout">
        <DocsSidebar
          activeArticleID={activeArticleID}
          query={query}
          open={menuOpen}
          onNavigate={() => setMenuOpen(false)}
        />
        <main className="docs-main" id="docs-main">
          {children}
        </main>
        <DocsOnThisPage items={toc} />
      </div>
    </div>
  );
}

function DocsHeader({
  query,
  results,
  onQueryChange,
  onSelectResult,
  productVersion,
  theme,
  onToggleTheme,
  menuOpen,
  onToggleMenu,
}: {
  query: string;
  results: WikiSearchResult[];
  onQueryChange: (value: string) => void;
  onSelectResult: (result: WikiSearchResult) => void;
  productVersion?: string;
  theme?: 'light' | 'dark';
  onToggleTheme?: () => void;
  menuOpen: boolean;
  onToggleMenu: () => void;
}) {
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const typingElsewhere = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable);
      if (event.key === '/' && !typingElsewhere) {
        event.preventDefault();
        inputRef.current?.focus();
      }
      if (event.key === 'Escape' && document.activeElement === inputRef.current) {
        onQueryChange('');
        inputRef.current?.blur();
      }
    };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [onQueryChange]);

  return (
    <header className="docs-header">
      <Link to={wikiHomePath()} className="docs-brand">
        <BrandIdentity className="docs-brand-identity" variant="mark" />
        <span className="docs-brand-name">
          NopsAI <span className="docs-brand-name-suffix">docs</span>
        </span>
        {productVersion ? <span className="docs-version">{productVersion}</span> : null}
      </Link>

      <div className="docs-search">
        <label>
          <span className="sr-only">Search the wiki</span>
          <Search className="docs-search-icon h-4 w-4" aria-hidden="true" />
          <input
            ref={inputRef}
            value={query}
            onChange={event => onQueryChange(event.target.value)}
            placeholder="Search documentation"
            className="docs-search-input"
          />
        </label>
        {query.trim() ? <DocsSearchResults results={results} onSelectResult={onSelectResult} /> : null}
      </div>

      <div className="docs-header-links">
        {onToggleTheme ? (
          <button type="button" onClick={onToggleTheme} aria-label={theme === 'dark' ? 'Use light theme' : 'Use dark theme'}>
            {theme === 'dark' ? <Sun className="h-4 w-4" aria-hidden="true" /> : <Moon className="h-4 w-4" aria-hidden="true" />}
          </button>
        ) : null}
        <Link to="/pipelineruns/main">Back to NopsAI</Link>
      </div>

      <button
        type="button"
        className="docs-menu-button"
        onClick={onToggleMenu}
        aria-expanded={menuOpen}
        aria-controls="docs-sidebar"
        aria-label={menuOpen ? 'Close documentation menu' : 'Open documentation menu'}
      >
        {menuOpen ? <X className="h-5 w-5" aria-hidden="true" /> : <Menu className="h-5 w-5" aria-hidden="true" />}
      </button>
    </header>
  );
}

function DocsSearchResults({
  results,
  onSelectResult,
}: {
  results: WikiSearchResult[];
  onSelectResult: (result: WikiSearchResult) => void;
}) {
  if (!results.length) {
    return (
      <div className="docs-search-results">
        <p className="px-2 py-2 text-[13px] text-[var(--docs-muted)]">Nothing matches this search.</p>
      </div>
    );
  }
  return (
    <ul aria-label="Search results" className="docs-search-results">
      {results.map(result => (
        <li key={result.id}>
          <button type="button" onClick={() => onSelectResult(result)} className="docs-search-result">
            <span className="flex items-baseline gap-2">
              <span className="shrink-0 text-[10px] font-bold uppercase tracking-wider text-[var(--docs-faint)]">
                {wikiSearchKindLabels[result.kind]}
              </span>
              <span className="min-w-0 truncate text-[13px] font-medium text-[var(--docs-text)]">{result.title}</span>
            </span>
            <span className="mt-0.5 block truncate text-[12px] text-[var(--docs-muted)]">{result.context}</span>
          </button>
        </li>
      ))}
    </ul>
  );
}

function DocsSidebar({
  activeArticleID,
  query,
  open,
  onNavigate,
}: {
  activeArticleID: string;
  query: string;
  open: boolean;
  /** Selecting a page closes the mobile menu; on wider viewports the sidebar is always visible. */
  onNavigate: () => void;
}) {
  const term = query.trim().toLowerCase();
  const groups = wikiSections
    .map(section => ({
      section,
      articles: term
        ? section.articles.filter(
            article => article.title.toLowerCase().includes(term) || section.title.toLowerCase().includes(term),
          )
        : section.articles,
    }))
    .filter(entry => entry.articles.length > 0);

  return (
    <aside id="docs-sidebar" className={`docs-sidebar${open ? ' is-open' : ''}`}>
      <nav aria-label="Wiki pages">
        {groups.map(({ section, articles }) => (
          <div key={section.id} className="docs-nav-group">
            <p className="docs-nav-title">{section.title}</p>
            {articles.map(article => (
              <Link
                key={article.id}
                to={wikiArticlePath(section.id, article.id)}
                onClick={onNavigate}
                aria-current={article.id === activeArticleID ? 'page' : undefined}
                className={`docs-nav-link${article.id === activeArticleID ? ' is-active' : ''}`}
              >
                {article.title}
              </Link>
            ))}
          </div>
        ))}
      </nav>
    </aside>
  );
}

function DocsOnThisPage({ items }: { items: DocsTocItem[] }) {
  const location = useLocation();
  const activeID = useActiveSection(items);

  if (items.length < 2) return <div className="docs-toc" aria-hidden="true" />;
  return (
    <aside className="docs-toc" aria-label="On this page">
      <p className="docs-toc-title">On this page</p>
      <nav>
        {items.map(item => (
          /*
           * The app runs under a HashRouter, so the URL hash is the route. A raw
           * `href="#section"` replaces the route with a non-existent one and the
           * reader lands on the fallback page instead of the section. Routing the
           * link through the router keeps the current path and moves only the
           * fragment, which is what the scroll effect already listens for.
           */
          <Link
            key={item.id}
            to={{ pathname: location.pathname, search: location.search, hash: `#${item.id}` }}
            className={item.id === activeID ? 'is-active' : undefined}
            aria-current={item.id === activeID ? 'true' : undefined}
          >
            {item.label}
          </Link>
        ))}
      </nav>
    </aside>
  );
}

/**
 * The section the reader is currently in.
 *
 * Computed from scroll position rather than intersection: a documentation block
 * can be several screens tall, so "is it on screen?" stays true long after the
 * reader has moved past its heading, which marked the wrong entry. The section
 * being read is the last one whose heading has passed under the header — and at
 * the bottom of the page it is the final entry, whose heading may never reach
 * the threshold at all.
 */
function useActiveSection(items: DocsTocItem[]) {
  const [activeID, setActiveID] = useState('');
  const signature = items.map(item => item.id).join('|');

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const ids = signature ? signature.split('|') : [];
    if (ids.length === 0) return;

    const scroller = document.getElementById('page-content-wrapper');
    const threshold = 96;
    let frame = 0;

    const measure = () => {
      frame = 0;
      const elements = ids
        .map(id => document.getElementById(id))
        .filter((node): node is HTMLElement => Boolean(node));
      if (elements.length === 0) return;

      if (scroller && scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 8) {
        setActiveID(elements[elements.length - 1].id);
        return;
      }

      const passed = elements.filter(element => element.getBoundingClientRect().top <= threshold);
      setActiveID(passed.length > 0 ? passed[passed.length - 1].id : elements[0].id);
    };

    const schedule = () => {
      if (frame) return;
      frame = window.requestAnimationFrame(measure);
    };

    measure();
    const target: HTMLElement | Window = scroller || window;
    target.addEventListener('scroll', schedule, { passive: true });
    window.addEventListener('resize', schedule);
    return () => {
      if (frame) window.cancelAnimationFrame(frame);
      target.removeEventListener('scroll', schedule);
      window.removeEventListener('resize', schedule);
    };
  }, [signature]);

  return activeID;
}
