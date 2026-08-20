import { Link } from 'react-router-dom';
import { summarizeWiki, wikiArticlePath, wikiMetadata, wikiSections } from './content/index.js';

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
    sectionID: 'getting-started',
    articleID: 'what-nopsai-is',
  },
  {
    label: 'I want to install it',
    description: 'Run a local stack and finish first-install setup.',
    sectionID: 'getting-started',
    articleID: 'install-local-docker-compose',
  },
  {
    label: 'I am writing a pipeline',
    description: 'One capability per page, on a manifest that grows as you read.',
    sectionID: 'pipelines',
    articleID: 'pipeline-anatomy',
  },
  {
    label: 'Something is broken',
    description: 'Symptom to likely cause, with the page that explains the fix.',
    sectionID: 'operations',
    articleID: 'troubleshooting',
  },
];

const quickReference: { label: string; sectionID: string; articleID: string }[] = [
  { label: 'All YAML directives', sectionID: 'reference', articleID: 'directive-index' },
  { label: 'All environment variables', sectionID: 'reference', articleID: 'environment-index' },
  { label: 'All REST endpoints', sectionID: 'api', articleID: 'api-index' },
  { label: 'Glossary', sectionID: 'reference', articleID: 'concepts-glossary' },
  { label: 'Production hardening', sectionID: 'operations', articleID: 'production-hardening' },
  { label: 'Confirmed limits', sectionID: 'reference', articleID: 'known-limits' },
];

export function WikiHome() {
  const summary = summarizeWiki();

  return (
    <div>
      <p className="docs-breadcrumb">NopsAI / Documentation</p>
      <h1 className="docs-h1">{wikiMetadata.title}</h1>
      <p className="docs-lead">{wikiMetadata.tagline}</p>

      <section aria-labelledby="wiki-start">
        <h2 id="wiki-start" className="docs-h2">
          Start with what you need
        </h2>
        <div className="docs-grid">
          {startingPoints.map(item => (
            <Link key={item.articleID} to={wikiArticlePath(item.sectionID, item.articleID)} className="docs-card">
              <strong>{item.label}</strong>
              <span>{item.description}</span>
            </Link>
          ))}
        </div>
      </section>

      <section aria-labelledby="wiki-quick">
        <h2 id="wiki-quick" className="docs-h2">
          Look something up
        </h2>
        <ul className="mt-3 flex flex-wrap gap-2">
          {quickReference.map(item => (
            <li key={item.articleID}>
              <Link
                to={wikiArticlePath(item.sectionID, item.articleID)}
                className="inline-flex items-center rounded-md border border-[var(--docs-border)] px-3 py-1.5 text-[13px] text-[var(--docs-muted)] hover:border-[var(--docs-accent-border)] hover:text-[var(--docs-accent)]"
              >
                {item.label}
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <section aria-labelledby="wiki-browse">
        <h2 id="wiki-browse" className="docs-h2">
          Browse everything
        </h2>
        <div className="docs-grid">
          {wikiSections.map(section => (
            <div key={section.id} className="docs-card">
              <strong>{section.title}</strong>
              <span>{section.description}</span>
              <ul className="mt-2.5 flex flex-wrap gap-x-3 gap-y-1">
                {section.articles.map(article => (
                  <li key={article.id}>
                    <Link
                      to={wikiArticlePath(section.id, article.id)}
                      className="text-[13px] text-[var(--docs-muted)] underline-offset-2 hover:text-[var(--docs-accent)] hover:underline"
                    >
                      {article.title}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </section>

      <p className="mt-12 border-t border-[var(--docs-border)] pt-4 text-[12px] text-[var(--docs-faint)]">
        {summary.sections} sections · {summary.articles} pages · {summary.fields} documented fields ·{' '}
        {summary.examples} examples · {summary.sources} implementation references
      </p>
    </div>
  );
}
