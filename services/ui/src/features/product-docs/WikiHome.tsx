import { Link } from 'react-router-dom';
import {
  summarizeWiki,
  wikiArticlePath,
  wikiGroupDescriptions,
  wikiGroupLabels,
  wikiGroupedSections,
  wikiMetadata,
} from './content/index.js';
import { WikiSearchBox } from './WikiNavigation.js';
import type { WikiSearchResult } from './search.js';

/**
 * Task-based entry points.
 *
 * The landing page answers "what am I trying to do?" before it answers "what
 * sections exist?", because a reader arriving at the wiki has a task, not a
 * table of contents.
 */
const startingPoints: { label: string; description: string; sectionID: string; articleID: string }[] = [
  {
    label: 'I am new here',
    description: 'What the product is and how a run actually executes.',
    sectionID: 'start',
    articleID: 'what-nopsai-is',
  },
  {
    label: 'I want to install it',
    description: 'Run a local stack and finish first-install setup.',
    sectionID: 'get-started',
    articleID: 'install-local-docker-compose',
  },
  {
    label: 'I am writing a pipeline',
    description: 'Every pipeline, step, task, and output directive.',
    sectionID: 'automation',
    articleID: 'pipeline-schema',
  },
  {
    label: 'Something is broken',
    description: 'Symptom to likely cause, with the page that explains the fix.',
    sectionID: 'reference',
    articleID: 'troubleshooting',
  },
];

const quickReference: { label: string; sectionID: string; articleID: string }[] = [
  { label: 'All YAML directives', sectionID: 'reference', articleID: 'directive-index' },
  { label: 'All environment variables', sectionID: 'reference', articleID: 'environment-index' },
  { label: 'All REST endpoints', sectionID: 'reference', articleID: 'api-index' },
  { label: 'Glossary', sectionID: 'start', articleID: 'concepts-glossary' },
  { label: 'Production hardening', sectionID: 'reference', articleID: 'production-hardening' },
  { label: 'Confirmed limits', sectionID: 'reference', articleID: 'known-limits' },
];

export function WikiHome({
  query,
  results,
  onQueryChange,
  onSelectResult,
}: {
  query: string;
  results: WikiSearchResult[];
  onQueryChange: (value: string) => void;
  onSelectResult: (result: WikiSearchResult) => void;
}) {
  const summary = summarizeWiki();

  return (
    <div className="mx-auto max-w-4xl pb-16">
      <header>
        <h2 className="text-[22px] font-semibold tracking-tight text-[var(--text-primary)]">{wikiMetadata.title}</h2>
        <p className="mt-1.5 max-w-[62ch] text-[15px] leading-7 text-[var(--text-secondary)]">{wikiMetadata.tagline}</p>
      </header>

      <WikiSearchBox large query={query} results={results} onQueryChange={onQueryChange} onSelectResult={onSelectResult} />

      <section aria-labelledby="wiki-start" className="mt-8">
        <h2 id="wiki-start" className="text-sm font-semibold uppercase tracking-wider text-[var(--text-secondary)]">
          Start with what you need
        </h2>
        <ul className="mt-3 grid gap-2 sm:grid-cols-2">
          {startingPoints.map(item => (
            <li key={item.articleID}>
              <Link
                to={wikiArticlePath(item.sectionID, item.articleID)}
                className="block h-full rounded border border-[var(--border-primary)] px-3 py-2.5 hover:bg-[var(--bg-tertiary)]"
              >
                <span className="block text-sm font-medium text-[var(--text-primary)]">{item.label}</span>
                <span className="mt-0.5 block text-sm leading-6 text-[var(--text-secondary)]">{item.description}</span>
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <section aria-labelledby="wiki-quick" className="mt-8">
        <h2 id="wiki-quick" className="text-sm font-semibold uppercase tracking-wider text-[var(--text-secondary)]">
          Look something up
        </h2>
        <ul className="mt-3 flex flex-wrap gap-2">
          {quickReference.map(item => (
            <li key={item.articleID}>
              <Link
                to={wikiArticlePath(item.sectionID, item.articleID)}
                className="inline-flex items-center rounded border border-[var(--border-primary)] px-2.5 py-1 text-sm text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
              >
                {item.label}
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <section aria-labelledby="wiki-browse" className="mt-8">
        <h2 id="wiki-browse" className="text-sm font-semibold uppercase tracking-wider text-[var(--text-secondary)]">
          Browse everything
        </h2>
        <div className="mt-3 space-y-6">
          {wikiGroupedSections().map(({ group, sections }) => (
            <div key={group}>
              <h3 className="text-[15px] font-semibold text-[var(--text-primary)]">{wikiGroupLabels[group]}</h3>
              <p className="mt-0.5 text-sm leading-6 text-[var(--text-secondary)]">{wikiGroupDescriptions[group]}</p>
              <ul className="mt-2 grid gap-2 sm:grid-cols-2">
                {sections.map(section => (
                  <li key={section.id} className="rounded border border-[var(--border-primary)] px-3 py-2.5">
                    <p className="text-sm font-medium text-[var(--text-primary)]">{section.title}</p>
                    <p className="mt-0.5 text-sm leading-6 text-[var(--text-secondary)]">{section.description}</p>
                    <ul className="mt-2 flex flex-wrap gap-x-3 gap-y-1">
                      {section.articles.map(article => (
                        <li key={article.id}>
                          <Link
                            to={wikiArticlePath(section.id, article.id)}
                            className="text-sm text-[var(--text-secondary)] underline-offset-2 hover:text-[var(--text-primary)] hover:underline"
                          >
                            {article.title}
                          </Link>
                        </li>
                      ))}
                    </ul>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </section>

      <footer className="mt-10 border-t border-[var(--border-primary)] pt-4 text-sm text-[var(--text-secondary)]">
        {summary.sections} sections · {summary.articles} pages · {summary.fields} documented fields ·{' '}
        {summary.examples} examples · {summary.sources} implementation references
      </footer>
    </div>
  );
}
