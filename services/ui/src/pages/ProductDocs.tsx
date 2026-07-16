import { useMemo } from 'react';
import { Copy, ExternalLink } from 'lucide-react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  countWikiArticles,
  filterWikiSections,
  findWikiArticle,
  findWikiArticleByPath,
  findWikiSectionForArticle,
  getFirstWikiArticleID,
  wikiArticlePath,
  wikiAudienceLabel,
  wikiDocTypeLabel,
  wikiMetadata,
  wikiSections,
  type WikiArticle,
  type WikiConfigRow,
  type WikiExample,
  type WikiSection,
} from '../features/product-docs/model';

type TocItem = {
  id: string;
  label: string;
  visible: boolean;
};

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
          <p className="mt-1 text-xs leading-5 text-[var(--text-tertiary)]">{section.description}</p>
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
                    <span className="block">{article.title}</span>
                    <span className="mt-0.5 block text-xs text-[var(--text-tertiary)]">{wikiDocTypeLabel(article.docType)}</span>
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

function ArticleMetadata({ article, section }: { article: WikiArticle; section: WikiSection }) {
  const metadata = [
    ['Content type', wikiDocTypeLabel(article.docType)],
    ['Applies to', article.metadata.appliesTo],
    ['Audience', article.audiences.map(wikiAudienceLabel).join(', ')],
    ['Owner', article.metadata.owner || section.owner],
    ['Introduced', article.metadata.introducedIn],
    ['Last verified', article.metadata.lastVerified],
    ['Source commit', article.metadata.sourceCommit],
    ['Status', article.metadata.status],
  ];

  return (
    <dl className="grid gap-x-6 gap-y-3 border-y border-[var(--border-primary)] py-4 text-xs sm:grid-cols-2 lg:grid-cols-4">
      {metadata.map(([label, value]) => (
        <div key={label}>
          <dt className="font-semibold uppercase text-[var(--text-tertiary)]">{label}</dt>
          <dd className="mt-1 text-[var(--text-secondary)]">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function ListSection({ id, title, items }: { id: string; title: string; items: string[] }) {
  if (items.length === 0) return null;

  return (
    <section id={id} className="scroll-mt-8 border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">{title}</h2>
      <ul className="mt-4 list-disc space-y-2 pl-5 text-sm leading-7 text-[var(--text-secondary)]">
        {items.map((item, index) => <li key={`${item}-${index}`}>{item}</li>)}
      </ul>
    </section>
  );
}

function Prerequisites({ article }: { article: WikiArticle }) {
  if (article.prerequisites.length === 0) return null;

  return (
    <section id="prerequisites" className="scroll-mt-8 border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">Prerequisites</h2>
      <div className="mt-4 overflow-x-auto">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="border-b border-[var(--border-primary)] text-xs uppercase text-[var(--text-tertiary)]">
            <tr>
              <th className="py-2 pr-5 font-semibold">Requirement</th>
              <th className="py-2 pr-5 font-semibold">Value</th>
              <th className="py-2 font-semibold">Verification</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--border-primary)]">
            {article.prerequisites.map(item => (
              <tr key={`${article.id}-${item.label}`}>
                <td className="whitespace-nowrap py-3 pr-5 font-semibold text-[var(--text-primary)]">{item.label}</td>
                <td className="min-w-[18rem] py-3 pr-5 text-[var(--text-secondary)]">{item.value}</td>
                <td className="min-w-[12rem] py-3 font-mono text-xs text-[var(--text-secondary)]">{item.verification || 'Review in UI or deployment config'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function Procedure({ article }: { article: WikiArticle }) {
  if (article.steps.length === 0) return null;

  return (
    <section id="procedure" className="scroll-mt-8 border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">Procedure</h2>
      <ol className="mt-5 space-y-6">
        {article.steps.map((step, index) => (
          <li key={`${article.id}-${step.title}`} className="grid gap-4 sm:grid-cols-[2.5rem_minmax(0,1fr)]">
            <div className="flex h-9 w-9 items-center justify-center rounded-full border border-[var(--border-primary)] text-sm font-semibold text-[var(--text-primary)]">
              {index + 1}
            </div>
            <div>
              <h3 className="text-base font-semibold text-[var(--text-primary)]">{step.title}</h3>
              <p className="mt-2 text-sm leading-7 text-[var(--text-secondary)]">{step.description}</p>
              {step.warning ? (
                <p className="mt-3 border-l-2 border-amber-500 pl-3 text-sm leading-6 text-[var(--text-secondary)]">{step.warning}</p>
              ) : null}
              {step.commands?.length ? <ExampleList articleID={article.id} examples={step.commands} compact /> : null}
              {step.expectedOutput ? <Callout label="Expected result" value={step.expectedOutput} /> : null}
              {step.verification ? <Callout label="Verification" value={step.verification} /> : null}
            </div>
          </li>
        ))}
      </ol>
    </section>
  );
}

function DetailSection({ article }: { article: WikiArticle }) {
  if (article.details.length === 0) return null;

  return (
    <section id="details" className="scroll-mt-8 border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">Details</h2>
      <div className="mt-4 space-y-4 text-sm leading-7 text-[var(--text-secondary)]">
        {article.details.map((detail, index) => <p key={`${article.id}-detail-${index}`}>{detail}</p>)}
      </div>
    </section>
  );
}

function ConfigReference({ article }: { article: WikiArticle }) {
  if (article.configRows.length === 0) return null;

  return (
    <section id="configuration-reference" className="scroll-mt-8 border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">Configuration Reference</h2>
      <div className="mt-4 overflow-x-auto">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="border-b border-[var(--border-primary)] text-xs uppercase text-[var(--text-tertiary)]">
            <tr>
              <th className="py-2 pr-5 font-semibold">Field path</th>
              <th className="py-2 pr-5 font-semibold">Type</th>
              <th className="py-2 pr-5 font-semibold">Required</th>
              <th className="py-2 pr-5 font-semibold">Default</th>
              <th className="py-2 pr-5 font-semibold">Scope</th>
              <th className="py-2 pr-5 font-semibold">Description</th>
              <th className="py-2 pr-5 font-semibold">Constraints</th>
              <th className="py-2 font-semibold">Example</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--border-primary)]">
            {article.configRows.map(row => <ConfigRow key={`${article.id}-${row.key}`} row={row} />)}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ConfigRow({ row }: { row: WikiConfigRow }) {
  const constraints = [
    ...(row.allowedValues?.length ? [`Allowed: ${row.allowedValues.join(', ')}`] : []),
    ...(row.constraints || []),
    ...(row.inheritedFrom?.length ? [`Inherited from: ${row.inheritedFrom.join(', ')}`] : []),
    ...(row.permission ? [`Permission: ${row.permission}`] : []),
    ...(row.security ? [`Security: ${row.security}`] : []),
    ...(row.deprecatedIn ? [`Deprecated: ${row.deprecatedIn}`] : []),
  ];

  return (
    <tr>
      <td className="whitespace-nowrap py-3 pr-5 font-mono text-xs text-[var(--text-primary)]">{row.path || row.key}</td>
      <td className="whitespace-nowrap py-3 pr-5 text-[var(--text-secondary)]">{row.type || 'string'}</td>
      <td className="whitespace-nowrap py-3 pr-5 text-[var(--text-secondary)]">{formatRequired(row.required)}</td>
      <td className="whitespace-nowrap py-3 pr-5 font-mono text-xs text-[var(--text-secondary)]">{row.defaultValue || 'none'}</td>
      <td className="whitespace-nowrap py-3 pr-5 text-[var(--text-secondary)]">{row.scope || row.area}</td>
      <td className="min-w-[18rem] py-3 pr-5 text-[var(--text-secondary)]">{row.description}</td>
      <td className="min-w-[16rem] py-3 pr-5 text-xs leading-6 text-[var(--text-secondary)]">{constraints.length ? constraints.join(' ') : 'Current validation rules apply.'}</td>
      <td className="min-w-[12rem] py-3 font-mono text-xs text-[var(--text-secondary)]">{row.example}</td>
    </tr>
  );
}

function Examples({ article }: { article: WikiArticle }) {
  if (article.examples.length === 0) return null;

  return (
    <section id="examples" className="scroll-mt-8 border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">Examples</h2>
      <ExampleList articleID={article.id} examples={article.examples} />
    </section>
  );
}

function ExampleList({ articleID, examples, compact = false }: { articleID: string; examples: WikiExample[]; compact?: boolean }) {
  return (
    <div className={`mt-4 ${compact ? 'space-y-4' : 'space-y-6'}`}>
      {examples.map(example => (
        <div key={`${articleID}-${example.title}`}>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold text-[var(--text-primary)]">{example.title}</h3>
              <p className="mt-1 text-xs text-[var(--text-tertiary)]">
                {example.language} · {example.complete === false ? 'Partial snippet' : 'Complete example'}
                {example.testedIn ? ` · Tested ${example.testedIn}` : ''}
                {example.permission ? ` · ${example.permission}` : ''}
              </p>
            </div>
            <button
              type="button"
              onClick={() => copyExample(example.code)}
              className="inline-flex h-8 items-center gap-2 border border-[var(--border-primary)] px-3 text-xs font-semibold text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
            >
              <Copy className="h-3.5 w-3.5" aria-hidden="true" />
              Copy
            </button>
          </div>
          <pre className="mt-2 overflow-x-auto border-l-2 border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-xs leading-6 text-[var(--text-secondary)]">
            <code>{example.code}</code>
          </pre>
          {example.placeholderNotes?.length ? <ListNote label="Replace" items={example.placeholderNotes} /> : null}
          {example.expectedOutput ? <Callout label="Expected result" value={example.expectedOutput} /> : null}
          {example.validationCommand ? <Callout label="Validation command" value={example.validationCommand} code /> : null}
          {example.rollback ? <Callout label="Rollback or cleanup" value={example.rollback} code /> : null}
        </div>
      ))}
    </div>
  );
}

function Runbooks({ article }: { article: WikiArticle }) {
  if (article.runbookEntries.length === 0) return null;

  return (
    <section id="runbooks" className="scroll-mt-8 border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">Runbooks</h2>
      <div className="mt-4 space-y-5">
        {article.runbookEntries.map(runbook => (
          <article key={`${article.id}-${runbook.id}`} className="border-l-2 border-[var(--border-primary)] pl-4">
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">{runbook.title}</h3>
            <p className="mt-2 text-sm leading-7 text-[var(--text-secondary)]">{runbook.impact}</p>
            <dl className="mt-3 grid gap-3 text-xs sm:grid-cols-2">
              <RunbookField label="Required access" value={runbook.requiredAccess} />
              <RunbookField label="Escalation" value={runbook.escalation || 'Escalate to the article owner when normal checks do not explain the failure.'} />
            </dl>
            <ListNote label="Symptoms" items={runbook.symptoms} />
            <ListNote label="Initial checks" items={runbook.initialChecks} />
            {runbook.diagnosticCommands.length ? <ListNote label="Diagnostic commands" items={runbook.diagnosticCommands} code /> : null}
            <ListNote label="Resolution" items={runbook.resolution} />
            {runbook.rollback ? <Callout label="Rollback" value={runbook.rollback} /> : null}
            {runbook.metrics?.length ? <ListNote label="Metrics and logs" items={runbook.metrics} /> : null}
          </article>
        ))}
      </div>
    </section>
  );
}

function Sources({ article }: { article: WikiArticle }) {
  if (article.sourceLinks.length === 0) return null;

  return (
    <section id="source-documents" className="scroll-mt-8 border-t border-[var(--border-primary)] pt-7">
      <h2 className="text-xl font-semibold text-[var(--text-primary)]">Source Documents</h2>
      <ul className="mt-4 space-y-3 text-sm text-[var(--text-secondary)]">
        {article.sourceLinks.map(source => (
          <li key={`${article.id}-${source.repositoryPath}`} className="border-l-2 border-[var(--border-primary)] pl-4">
            <div className="flex flex-wrap items-center gap-2">
              {source.sourceUrl ? (
                <a className="inline-flex items-center gap-1 font-semibold text-[var(--text-accent)]" href={source.sourceUrl} target="_blank" rel="noreferrer">
                  {source.title}
                  <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
                </a>
              ) : (
                <span className="font-semibold text-[var(--text-primary)]">{source.title}</span>
              )}
              <code className="text-xs text-[var(--text-tertiary)]">{source.repositoryPath}</code>
              {source.sourceLines ? <span className="text-xs text-[var(--text-tertiary)]">{source.sourceLines}</span> : null}
            </div>
            <p className="mt-1 text-sm leading-6">{source.purpose}</p>
          </li>
        ))}
      </ul>
    </section>
  );
}

function OnThisPage({ items }: { items: TocItem[] }) {
  const visibleItems = items.filter(item => item.visible);
  if (visibleItems.length === 0) return null;

  return (
    <aside className="hidden lg:block lg:sticky lg:top-6 lg:max-h-[calc(100vh-3rem)] lg:overflow-y-auto" aria-label="On this page">
      <h2 className="text-xs font-semibold uppercase text-[var(--text-tertiary)]">On this page</h2>
      <nav className="mt-3 space-y-2">
        {visibleItems.map(item => (
          <a key={item.id} className="block border-l-2 border-transparent pl-3 text-sm text-[var(--text-secondary)] hover:border-[var(--border-primary)] hover:text-[var(--text-primary)]" href={`#${item.id}`}>
            {item.label}
          </a>
        ))}
      </nav>
    </aside>
  );
}

function RunbookField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="font-semibold uppercase text-[var(--text-tertiary)]">{label}</dt>
      <dd className="mt-1 leading-5 text-[var(--text-secondary)]">{value}</dd>
    </div>
  );
}

function ListNote({ label, items, code = false }: { label: string; items: string[]; code?: boolean }) {
  if (items.length === 0) return null;
  return (
    <div className="mt-3">
      <p className="text-xs font-semibold uppercase text-[var(--text-tertiary)]">{label}</p>
      <ul className="mt-1 list-disc space-y-1 pl-5 text-sm leading-6 text-[var(--text-secondary)]">
        {items.map((item, index) => (
          <li key={`${label}-${item}-${index}`}>{code ? <code className="font-mono text-xs">{item}</code> : item}</li>
        ))}
      </ul>
    </div>
  );
}

function Callout({ label, value, code = false }: { label: string; value: string; code?: boolean }) {
  return (
    <div className="mt-3 border-l-2 border-[var(--border-primary)] pl-3">
      <p className="text-xs font-semibold uppercase text-[var(--text-tertiary)]">{label}</p>
      {code ? (
        <code className="mt-1 block font-mono text-xs leading-6 text-[var(--text-secondary)]">{value}</code>
      ) : (
        <p className="mt-1 text-sm leading-6 text-[var(--text-secondary)]">{value}</p>
      )}
    </div>
  );
}

function formatRequired(value: WikiConfigRow['required']) {
  if (value === true) return 'Yes';
  if (value === false) return 'No';
  return 'Conditional';
}

function copyExample(code: string) {
  if (typeof navigator === 'undefined' || !navigator.clipboard) return;
  void navigator.clipboard.writeText(code);
}

function buildToc(article: WikiArticle): TocItem[] {
  return [
    { id: 'key-facts', label: 'Key facts', visible: article.keyFacts.length > 0 },
    { id: 'prerequisites', label: 'Prerequisites', visible: article.prerequisites.length > 0 },
    { id: 'procedure', label: 'Procedure', visible: article.steps.length > 0 },
    { id: 'details', label: 'Details', visible: article.details.length > 0 },
    { id: 'configuration-reference', label: 'Configuration reference', visible: article.configRows.length > 0 },
    { id: 'examples', label: 'Examples', visible: article.examples.length > 0 },
    { id: 'runbooks', label: 'Runbooks', visible: article.runbookEntries.length > 0 },
    { id: 'boundaries', label: 'Boundaries', visible: article.caveats.length > 0 },
    { id: 'source-documents', label: 'Source documents', visible: article.sourceLinks.length > 0 },
  ];
}

export default function ProductDocsPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const query = new URLSearchParams(location.search).get('q') || '';
  const routeSelection = useMemo(() => findWikiArticleByPath(wikiSections, location.pathname), [location.pathname]);
  const visibleSections = useMemo(() => filterWikiSections(wikiSections, query), [query]);
  const visibleArticleCount = countWikiArticles(visibleSections);
  const activeArticle =
    findWikiArticle(visibleSections, routeSelection?.article.id || '') ||
    visibleSections[0]?.articles[0] ||
    routeSelection?.article ||
    findWikiArticle(wikiSections, getFirstWikiArticleID());
  const activeSection = activeArticle ? findWikiSectionForArticle(wikiSections, activeArticle.id) : undefined;
  const tocItems = useMemo(() => activeArticle ? buildToc(activeArticle) : [], [activeArticle]);

  if (!activeArticle || !activeSection) return null;

  const handleQueryChange = (nextQuery: string) => {
    const params = new URLSearchParams(location.search);
    if (nextQuery.trim()) {
      params.set('q', nextQuery);
    } else {
      params.delete('q');
    }
    navigate({ pathname: location.pathname, search: params.toString() ? `?${params.toString()}` : '' }, { replace: true });
  };

  const handleSelectArticle = (articleID: string) => {
    const section = findWikiSectionForArticle(wikiSections, articleID);
    if (!section) return;
    navigate({ pathname: wikiArticlePath(section.id, articleID), search: location.search });
  };

  return (
    <div className="min-h-full bg-[var(--bg-primary)]">
      <div className="mx-auto grid w-full max-w-[1480px] gap-8 px-4 py-6 sm:px-6 lg:grid-cols-[19rem_minmax(0,1fr)_13rem] lg:px-8">
        <aside className="lg:sticky lg:top-6 lg:max-h-[calc(100vh-3rem)] lg:overflow-y-auto">
          <h2 className="text-lg font-semibold text-[var(--text-primary)]">{wikiMetadata.title}</h2>
          <p className="mt-1 text-xs text-[var(--text-tertiary)]">{wikiMetadata.status}</p>

          <label className="mt-5 block text-xs font-semibold uppercase text-[var(--text-tertiary)]">
            Search wiki pages
            <input
              value={query}
              onChange={event => handleQueryChange(event.target.value)}
              placeholder="steps[].llm_profile, runbook, Docker..."
              className="mt-2 h-10 w-full border border-[var(--border-input)] bg-transparent px-3 text-sm normal-case text-[var(--text-primary)] outline-none placeholder:text-[var(--text-placeholder)] focus:border-[var(--border-input-focus)]"
            />
          </label>
          <p className="mt-2 text-xs text-[var(--text-tertiary)]">{visibleArticleCount} matching pages</p>

          <div className="mt-7">
            {visibleSections.length > 0 ? (
              <WikiNav sections={visibleSections} activeArticleID={activeArticle.id} onSelectArticle={handleSelectArticle} />
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
                {wikiDocTypeLabel(activeArticle.docType)} · {activeArticle.audiences.map(wikiAudienceLabel).join(', ')}
              </p>
              <p className="mt-5 max-w-3xl text-base leading-8 text-[var(--text-secondary)]">{activeArticle.summary}</p>
            </header>

            <ArticleMetadata article={activeArticle} section={activeSection} />
            <ListSection id="key-facts" title="Key Facts" items={activeArticle.keyFacts} />
            <Prerequisites article={activeArticle} />
            <Procedure article={activeArticle} />
            <DetailSection article={activeArticle} />
            <ConfigReference article={activeArticle} />
            <Examples article={activeArticle} />
            <Runbooks article={activeArticle} />
            <ListSection id="boundaries" title="Boundaries" items={activeArticle.caveats} />
            <Sources article={activeArticle} />
          </article>
        </main>

        <OnThisPage items={tocItems} />
      </div>
    </div>
  );
}
