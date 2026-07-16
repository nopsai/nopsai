import { useMemo, useState } from 'react';
import {
  countWikiArticles,
  filterWikiSections,
  findWikiArticle,
  findWikiSectionForArticle,
  getFirstWikiArticleID,
  wikiMetadata,
  wikiSections,
  type WikiArticle,
  type WikiSection,
} from '../features/product-docs/model';

function WikiNav({
  sections,
  activeArticleID,
  onSelectArticle,
}: {
  sections: WikiSection[];
  activeArticleID: string;
  onSelectArticle: (articleID: string) => void;
}) {
  return (
    <nav aria-label="Wiki pages" className="space-y-6">
      {sections.map(section => (
        <div key={section.id}>
          <button
            type="button"
            onClick={() => onSelectArticle(section.articles[0]?.id || activeArticleID)}
            className="w-full text-left text-xs font-semibold uppercase text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
          >
            {section.title}
          </button>
          <ul className="mt-2 space-y-1">
            {section.articles.map(article => {
              const selected = article.id === activeArticleID;
              return (
                <li key={article.id}>
                  <button
                    type="button"
                    onClick={() => onSelectArticle(article.id)}
                    aria-current={selected ? 'page' : undefined}
                    className={`w-full border-l-2 py-1.5 pl-3 pr-2 text-left text-sm leading-5 transition ${
                      selected
                        ? 'border-[var(--text-accent)] text-[var(--text-primary)]'
                        : 'border-transparent text-[var(--text-secondary)] hover:border-[var(--border-primary)] hover:text-[var(--text-primary)]'
                    }`}
                  >
                    {article.title}
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </nav>
  );
}

function ListSection({ title, items }: { title: string; items: string[] }) {
  if (items.length === 0) return null;

  return (
    <section className="border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">{title}</h2>
      <ul className="mt-4 list-disc space-y-2 pl-5 text-sm leading-7 text-[var(--text-secondary)]">
        {items.map(item => <li key={item}>{item}</li>)}
      </ul>
    </section>
  );
}

function DetailSection({ article }: { article: WikiArticle }) {
  if (article.details.length === 0) return null;

  return (
    <section className="border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">Details</h2>
      <div className="mt-4 space-y-4 text-sm leading-7 text-[var(--text-secondary)]">
        {article.details.map(detail => <p key={detail}>{detail}</p>)}
      </div>
    </section>
  );
}

function ConfigReference({ article }: { article: WikiArticle }) {
  if (article.configRows.length === 0) return null;

  return (
    <section className="border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">Configuration Reference</h2>
      <div className="mt-4 overflow-x-auto">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="border-b border-[var(--border-primary)] text-xs uppercase text-[var(--text-tertiary)]">
            <tr>
              <th className="py-2 pr-5 font-semibold">Key</th>
              <th className="py-2 pr-5 font-semibold">Area</th>
              <th className="py-2 pr-5 font-semibold">Description</th>
              <th className="py-2 font-semibold">Example</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--border-primary)]">
            {article.configRows.map(row => (
              <tr key={`${article.id}-${row.key}`}>
                <td className="whitespace-nowrap py-3 pr-5 font-mono text-xs text-[var(--text-primary)]">{row.key}</td>
                <td className="whitespace-nowrap py-3 pr-5 text-[var(--text-secondary)]">{row.area}</td>
                <td className="min-w-[18rem] py-3 pr-5 text-[var(--text-secondary)]">{row.description}</td>
                <td className="min-w-[12rem] py-3 font-mono text-xs text-[var(--text-secondary)]">{row.example}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function Examples({ article }: { article: WikiArticle }) {
  if (article.examples.length === 0) return null;

  return (
    <section className="border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">Examples</h2>
      <div className="mt-4 space-y-6">
        {article.examples.map(example => (
          <div key={`${article.id}-${example.title}`}>
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">{example.title}</h3>
            <pre className="mt-2 overflow-x-auto border-l-2 border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-xs leading-6 text-[var(--text-secondary)]">
              <code>{example.code}</code>
            </pre>
          </div>
        ))}
      </div>
    </section>
  );
}

export default function ProductDocsPage() {
  const [query, setQuery] = useState('');
  const [activeArticleID, setActiveArticleID] = useState(getFirstWikiArticleID());
  const visibleSections = useMemo(() => filterWikiSections(wikiSections, query), [query]);
  const visibleArticleCount = countWikiArticles(visibleSections);
  const activeArticle =
    findWikiArticle(visibleSections, activeArticleID) ||
    visibleSections[0]?.articles[0] ||
    findWikiArticle(wikiSections, getFirstWikiArticleID());
  const activeSection = activeArticle ? findWikiSectionForArticle(wikiSections, activeArticle.id) : undefined;

  if (!activeArticle || !activeSection) return null;

  return (
    <div className="min-h-full bg-[var(--bg-primary)]">
      <div className="mx-auto grid w-full max-w-[1280px] gap-8 px-4 py-6 sm:px-6 lg:grid-cols-[18rem_minmax(0,1fr)] lg:px-8">
        <aside className="lg:sticky lg:top-6 lg:max-h-[calc(100vh-3rem)] lg:overflow-y-auto">
          <h2 className="text-lg font-semibold text-[var(--text-primary)]">{wikiMetadata.title}</h2>
          <p className="mt-1 text-xs text-[var(--text-tertiary)]">{wikiMetadata.status}</p>

          <label className="mt-5 block text-xs font-semibold uppercase text-[var(--text-tertiary)]">
            Search
            <input
              value={query}
              onChange={event => setQuery(event.target.value)}
              placeholder="runner, Gotenberg, AAA..."
              className="mt-2 h-10 w-full border border-[var(--border-input)] bg-transparent px-3 text-sm normal-case text-[var(--text-primary)] outline-none placeholder:text-[var(--text-placeholder)] focus:border-[var(--border-input-focus)]"
            />
          </label>
          <p className="mt-2 text-xs text-[var(--text-tertiary)]">{visibleArticleCount} matching pages</p>

          <div className="mt-7">
            {visibleSections.length > 0 ? (
              <WikiNav sections={visibleSections} activeArticleID={activeArticle.id} onSelectArticle={setActiveArticleID} />
            ) : (
              <p className="text-sm text-[var(--text-secondary)]">No matching wiki pages.</p>
            )}
          </div>
        </aside>

        <main className="min-w-0 pb-16" aria-labelledby="wiki-article-title">
          <article className="mx-auto max-w-4xl space-y-7">
            <header>
              <p className="text-sm text-[var(--text-tertiary)]">{activeSection.title}</p>
              <h2 id="wiki-article-title" className="mt-2 text-3xl font-semibold text-[var(--text-primary)]">
                {activeArticle.title}
              </h2>
              <p className="mt-2 text-sm text-[var(--text-tertiary)]">
                {activeArticle.level} · {activeArticle.audience}
              </p>
              <p className="mt-5 max-w-3xl text-base leading-8 text-[var(--text-secondary)]">{activeArticle.summary}</p>
            </header>

            <ListSection title="Key Facts" items={activeArticle.keyFacts} />
            <DetailSection article={activeArticle} />
            <ConfigReference article={activeArticle} />
            <Examples article={activeArticle} />
            <ListSection title="Runbooks" items={activeArticle.runbooks} />
            <ListSection title="Boundaries" items={activeArticle.caveats} />
            <ListSection title="Source Documents" items={activeArticle.relatedDocs} />
          </article>
        </main>
      </div>
    </div>
  );
}
