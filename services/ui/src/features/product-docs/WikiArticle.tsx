import { Link } from 'react-router-dom';
import {
  findWikiArticle,
  wikiArticlePath,
  wikiAudienceLabel,
  wikiDocTypeLabel,
  type WikiArticle as WikiArticleModel,
  type WikiRunbook,
  type WikiSection,
} from './content/index.js';
import { wikiBlockTitle, type WikiBlockID } from './blocks.js';
import { WikiFieldTable } from './WikiFieldTable.js';
import { InlineMarkup, WikiBlock, WikiBulletList, WikiChip, WikiCodeBlock, WikiNotice } from './WikiPrimitives.js';

export function WikiArticleHeader({ section, article }: { section: WikiSection; article: WikiArticleModel }) {
  return (
    <header>
      <nav aria-label="Breadcrumb" className="flex flex-wrap items-center gap-1.5 text-sm text-[var(--text-secondary)]">
        <Link to="/docs" className="hover:text-[var(--text-primary)]">
          Wiki
        </Link>
        <span aria-hidden="true">/</span>
        <span>{section.title}</span>
      </nav>
      <h2 id="wiki-article-title" className="mt-2 text-[22px] font-semibold tracking-tight text-[var(--text-primary)]">
        {article.title}
      </h2>
      <p className="mt-2 max-w-[62ch] text-[15px] leading-7 text-[var(--text-secondary)]">
        <InlineMarkup value={article.summary} />
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        <WikiChip tone="accent">{wikiDocTypeLabel(article.docType)}</WikiChip>
        {article.audiences.map(audience => (
          <WikiChip key={audience}>{wikiAudienceLabel(audience)}</WikiChip>
        ))}
        {article.status && article.status !== 'current' ? <WikiChip>{article.status}</WikiChip> : null}
      </div>
    </header>
  );
}

export function WikiArticleBody({
  article,
  blocks,
  targetAnchor,
  copiedKey,
  onCopy,
}: {
  article: WikiArticleModel;
  blocks: WikiBlockID[];
  targetAnchor?: string;
  copiedKey: string;
  onCopy: (key: string, code: string) => void;
}) {
  return (
    <div className="divide-y divide-[var(--border-primary)]">
      {blocks.map(block => {
        switch (block) {
          case 'key-facts':
            return (
              <WikiBlock key={block} id="key-facts" title={wikiBlockTitle('key-facts', article)}>
                <WikiBulletList id={`${article.id}-fact`} items={article.keyFacts} />
              </WikiBlock>
            );
          case 'prerequisites':
            return (
              <WikiBlock key={block} id="prerequisites" title={wikiBlockTitle('prerequisites')}>
                <dl className="divide-y divide-[var(--border-primary)] rounded border border-[var(--border-primary)]">
                  {(article.prerequisites || []).map(item => (
                    <div key={item.label} className="grid gap-1 px-3 py-2 sm:grid-cols-[10rem_minmax(0,1fr)]">
                      <dt className="text-sm font-medium text-[var(--text-primary)]">{item.label}</dt>
                      <dd>
                        <span className="text-sm leading-6 text-[var(--text-secondary)]">
                          <InlineMarkup value={item.value} />
                        </span>
                        {item.verification ? (
                          <code className="mt-0.5 block text-sm text-[var(--text-secondary)]">{item.verification}</code>
                        ) : null}
                      </dd>
                    </div>
                  ))}
                </dl>
              </WikiBlock>
            );
          case 'procedure':
            return (
              <WikiBlock key={block} id="procedure" title={wikiBlockTitle('procedure')}>
                <ol className="space-y-6">
                  {(article.steps || []).map((step, index) => (
                    <li key={step.title} className="grid gap-3 sm:grid-cols-[1.75rem_minmax(0,1fr)]">
                      <span className="flex h-7 w-7 items-center justify-center rounded-full border border-[var(--border-primary)] text-sm font-semibold text-[var(--text-primary)]">
                        {index + 1}
                      </span>
                      <div className="min-w-0">
                        <h4 className="text-[15px] font-semibold text-[var(--text-primary)]">{step.title}</h4>
                        <p className="mt-1 text-sm leading-6 text-[var(--text-secondary)]">
                          <InlineMarkup value={step.description} />
                        </p>
                        {step.warning ? (
                          <WikiNotice tone="warning" title="Important">
                            <InlineMarkup value={step.warning} />
                          </WikiNotice>
                        ) : null}
                        {step.commands?.length ? (
                          <div className="mt-3 space-y-3">
                            {step.commands.map((example, commandIndex) => (
                              <WikiCodeBlock
                                key={`${step.title}-${commandIndex}`}
                                example={example}
                                copyKey={`${article.id}-${step.title}-${commandIndex}`}
                                copiedKey={copiedKey}
                                onCopy={onCopy}
                              />
                            ))}
                          </div>
                        ) : null}
                        {step.expectedOutput ? (
                          <WikiNotice title="Expected result">
                            <InlineMarkup value={step.expectedOutput} />
                          </WikiNotice>
                        ) : null}
                        {step.verification ? (
                          <WikiNotice title="Verify">
                            <InlineMarkup value={step.verification} />
                          </WikiNotice>
                        ) : null}
                      </div>
                    </li>
                  ))}
                </ol>
              </WikiBlock>
            );
          case 'details':
            return (
              <WikiBlock key={block} id="details" title={wikiBlockTitle('details')}>
                <div className="space-y-3 text-sm leading-7 text-[var(--text-secondary)]">
                  {article.details.map((detail, index) => (
                    <p key={`${article.id}-detail-${index}`}>
                      <InlineMarkup value={detail} />
                    </p>
                  ))}
                </div>
              </WikiBlock>
            );
          case 'fields':
            return (
              <WikiBlock key={block} id="fields" title={wikiBlockTitle('fields')}>
                <WikiFieldTable fields={article.fields || []} targetAnchor={targetAnchor} />
              </WikiBlock>
            );
          case 'examples':
            return (
              <WikiBlock key={block} id="examples" title={wikiBlockTitle('examples')}>
                <div className="space-y-4">
                  {(article.examples || []).map((example, index) => (
                    <WikiCodeBlock
                      key={`${article.id}-example-${index}`}
                      example={example}
                      copyKey={`${article.id}-example-${index}`}
                      copiedKey={copiedKey}
                      onCopy={onCopy}
                    />
                  ))}
                </div>
              </WikiBlock>
            );
          case 'runbooks':
            return (
              <WikiBlock key={block} id="runbooks" title={wikiBlockTitle('runbooks')}>
                <div className="space-y-3">
                  {(article.runbooks || []).map(runbook => (
                    <RunbookCard key={runbook.id} runbook={runbook} />
                  ))}
                </div>
              </WikiBlock>
            );
          case 'limits':
            return (
              <WikiBlock key={block} id="limits" title={wikiBlockTitle('limits')}>
                <WikiNotice tone="warning" title="Current behavior">
                  <ul className="space-y-1 pl-5">
                    {(article.limits || []).map(item => (
                      <li key={item} className="list-disc">
                        <InlineMarkup value={item} />
                      </li>
                    ))}
                  </ul>
                </WikiNotice>
              </WikiBlock>
            );
          case 'related':
            return (
              <WikiBlock key={block} id="related" title={wikiBlockTitle('related')}>
                <ul className="flex flex-wrap gap-2">
                  {(article.related || []).map(relatedID => {
                    const location = findWikiArticle(relatedID);
                    if (!location) return null;
                    return (
                      <li key={relatedID}>
                        <Link
                          to={wikiArticlePath(location.section.id, location.article.id)}
                          className="inline-flex items-center rounded border border-[var(--border-primary)] px-2.5 py-1 text-sm text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
                        >
                          {location.article.title}
                        </Link>
                      </li>
                    );
                  })}
                </ul>
              </WikiBlock>
            );
          case 'sources':
            return (
              <WikiBlock key={block} id="sources" title={wikiBlockTitle('sources')}>
                <ul className="space-y-2">
                  {(article.sources || []).map(source => (
                    <li key={source.repositoryPath} className="rounded border border-[var(--border-primary)] px-3 py-2">
                      <code className="block break-all text-sm text-[var(--text-primary)]">{source.repositoryPath}</code>
                      <p className="mt-0.5 text-sm leading-6 text-[var(--text-secondary)]">{source.purpose}</p>
                    </li>
                  ))}
                </ul>
              </WikiBlock>
            );
          default:
            return null;
        }
      })}
    </div>
  );
}

function RunbookCard({ runbook }: { runbook: WikiRunbook }) {
  return (
    <details className="rounded border border-[var(--border-primary)] px-3 py-2">
      <summary className="cursor-pointer text-sm font-medium text-[var(--text-primary)]">{runbook.title}</summary>
      <div className="mt-3 space-y-3 border-t border-[var(--border-primary)] pt-3 text-sm leading-6 text-[var(--text-secondary)]">
        <p>{runbook.impact}</p>
        <RunbookList title="Symptoms" items={runbook.symptoms} />
        <RunbookList title="Initial checks" items={runbook.initialChecks} />
        <RunbookList title="Diagnostics" items={runbook.diagnostics} />
        <RunbookList title="Resolution" items={runbook.resolution} />
        {runbook.rollback ? <WikiNotice title="Rollback">{runbook.rollback}</WikiNotice> : null}
        <dl className="grid gap-2 text-sm sm:grid-cols-2">
          <div>
            <dt className="font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Required access</dt>
            <dd className="mt-0.5">{runbook.requiredAccess}</dd>
          </div>
          {runbook.escalation ? (
            <div>
              <dt className="font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Escalation</dt>
              <dd className="mt-0.5">{runbook.escalation}</dd>
            </div>
          ) : null}
        </dl>
      </div>
    </details>
  );
}

function RunbookList({ title, items }: { title: string; items: string[] }) {
  if (!items.length) return null;
  return (
    <div>
      <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{title}</p>
      <ul className="mt-1 space-y-1 pl-5">
        {items.map(item => (
          <li key={item} className="list-disc">
            <InlineMarkup value={item} />
          </li>
        ))}
      </ul>
    </div>
  );
}
