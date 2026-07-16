import { wikiAudienceLabel, wikiDocTypeLabel, type WikiPrerequisite, type WikiStep } from './model';
import type { DocumentationArticle, DocumentationSection } from './quality';
import { CodeExample } from './DocumentationExamples';
import { MetadataItem, Notice, SectionFrame } from './DocumentationPrimitives';

export function ArticleHeader({ section, article }: { section: DocumentationSection; article: DocumentationArticle }) {
  return (
    <header>
      <nav aria-label="Breadcrumb" className="flex flex-wrap items-center gap-2 text-sm text-[var(--text-tertiary)]">
        <span>Documentation</span><span aria-hidden="true">/</span><span>{section.title}</span><span aria-hidden="true">/</span><span className="text-[var(--text-secondary)]">{article.title}</span>
      </nav>
      <div className="mt-5 flex flex-wrap items-center gap-2 text-xs text-[var(--text-tertiary)]">
        <span className="rounded-full border border-[var(--border-primary)] px-2.5 py-1 font-semibold uppercase tracking-wide">{wikiDocTypeLabel(article.docType)}</span>
        <span>{article.audiences.map(wikiAudienceLabel).join(', ')}</span>
      </div>
      <h2 id="documentation-article-title" className="mt-3 text-3xl font-semibold tracking-tight text-[var(--text-primary)] sm:text-4xl">{article.title}</h2>
      <p className="mt-4 max-w-3xl text-base leading-8 text-[var(--text-secondary)]">{article.summary}</p>
      <details className="mt-5 max-w-3xl rounded border border-[var(--border-primary)] px-4 py-3 text-sm">
        <summary className="cursor-pointer font-medium text-[var(--text-secondary)]">Page details</summary>
        <dl className="mt-4 grid gap-x-6 gap-y-3 text-xs sm:grid-cols-2 lg:grid-cols-3">
          <MetadataItem label="Owner" value={article.metadata.owner || section.owner} />
          <MetadataItem label="Applies to" value={article.metadata.appliesTo} />
          <MetadataItem label="Introduced" value={article.metadata.introducedIn} />
          <MetadataItem label="Last verified" value={article.metadata.lastVerified} />
          <MetadataItem label="Source revision" value={article.metadata.sourceCommit} />
          <MetadataItem label="Status" value={article.metadata.status} />
        </dl>
      </details>
    </header>
  );
}

export function KeyFacts({ article }: { article: DocumentationArticle }) {
  if (!article.keyFacts.length) return null;
  return (
    <SectionFrame id="key-facts" title={article.docType === 'tutorial' ? 'What you will accomplish' : 'Key points'}>
      <ul className="space-y-2 pl-5 text-sm leading-7 text-[var(--text-secondary)]">
        {article.keyFacts.map((item, index) => <li key={`${article.id}-fact-${index}`} className="list-disc">{item}</li>)}
      </ul>
    </SectionFrame>
  );
}

export function Prerequisites({ items }: { items: WikiPrerequisite[] }) {
  if (!items.length) return null;
  return (
    <SectionFrame id="prerequisites" title="Prerequisites">
      <div className="divide-y divide-[var(--border-primary)] rounded border border-[var(--border-primary)]">
        {items.map(item => (
          <div key={item.label} className="grid gap-2 px-4 py-3 sm:grid-cols-[10rem_minmax(0,1fr)]">
            <div className="text-sm font-medium text-[var(--text-primary)]">{item.label}</div>
            <div>
              <p className="text-sm leading-6 text-[var(--text-secondary)]">{item.value}</p>
              {item.verification ? <code className="mt-1 block text-xs leading-5 text-[var(--text-tertiary)]">{item.verification}</code> : null}
            </div>
          </div>
        ))}
      </div>
    </SectionFrame>
  );
}

export function Procedure({
  article,
  copiedKey,
  onCopy,
}: {
  article: DocumentationArticle;
  copiedKey: string;
  onCopy: (key: string, code: string) => void;
}) {
  if (!article.steps.length) return null;
  return (
    <SectionFrame id="procedure" title="Procedure">
      <ol className="space-y-7">
        {article.steps.map((step, index) => <ProcedureStep key={`${article.id}-${step.title}`} articleID={article.id} step={step} index={index} copiedKey={copiedKey} onCopy={onCopy} />)}
      </ol>
    </SectionFrame>
  );
}

function ProcedureStep({ articleID, step, index, copiedKey, onCopy }: { articleID: string; step: WikiStep; index: number; copiedKey: string; onCopy: (key: string, code: string) => void }) {
  return (
    <li className="grid gap-4 sm:grid-cols-[2.25rem_minmax(0,1fr)]">
      <div className="flex h-8 w-8 items-center justify-center rounded-full border border-[var(--border-primary)] text-sm font-semibold text-[var(--text-primary)]">{index + 1}</div>
      <div>
        <h3 className="text-base font-semibold text-[var(--text-primary)]">{step.title}</h3>
        <p className="mt-2 text-sm leading-7 text-[var(--text-secondary)]">{step.description}</p>
        {step.warning ? <Notice tone="warning" title="Important">{step.warning}</Notice> : null}
        {step.commands?.length ? <div className="mt-4 space-y-4">{step.commands.map((example, commandIndex) => <CodeExample key={`${articleID}-${step.title}-${commandIndex}`} example={example} copyKey={`${articleID}-${step.title}-${commandIndex}`} copiedKey={copiedKey} onCopy={onCopy} compact />)}</div> : null}
        {step.expectedOutput ? <Notice title="Expected result">{step.expectedOutput}</Notice> : null}
        {step.verification ? <Notice title="Verification">{step.verification}</Notice> : null}
      </div>
    </li>
  );
}

export function Details({ article }: { article: DocumentationArticle }) {
  if (!article.details.length) return null;
  const title = article.docType === 'concept' ? 'How it works' : article.docType === 'troubleshooting' ? 'Diagnosis notes' : article.docType === 'reference' ? 'Behavior' : 'Notes';
  return (
    <SectionFrame id="details" title={title}>
      <div className="space-y-4 text-sm leading-7 text-[var(--text-secondary)]">{article.details.map((detail, index) => <p key={`${article.id}-detail-${index}`}>{detail}</p>)}</div>
    </SectionFrame>
  );
}

export function Boundaries({ article }: { article: DocumentationArticle }) {
  if (!article.caveats.length) return null;
  return (
    <SectionFrame id="boundaries" title="Boundaries">
      <Notice tone="warning" title="Current limits"><ul className="space-y-1 pl-5">{article.caveats.map((item, index) => <li key={`${article.id}-boundary-${index}`} className="list-disc">{item}</li>)}</ul></Notice>
    </SectionFrame>
  );
}

export function Sources({ article }: { article: DocumentationArticle }) {
  if (!article.sourceLinks.length) return null;
  return (
    <section id="sources" className="scroll-mt-8 border-t border-[var(--border-primary)] pt-8">
      <details>
        <summary className="cursor-pointer text-lg font-semibold text-[var(--text-primary)]">Implementation evidence</summary>
        <ul className="mt-4 space-y-3">
          {article.sourceLinks.map(source => (
            <li key={`${article.id}-${source.repositoryPath}`} className="rounded border border-[var(--border-primary)] px-4 py-3">
              <p className="text-sm font-medium text-[var(--text-primary)]">{source.title}</p>
              <code className="mt-1 block text-xs text-[var(--text-tertiary)]">{source.repositoryPath}</code>
              <p className="mt-2 text-sm leading-6 text-[var(--text-secondary)]">{source.purpose}</p>
            </li>
          ))}
        </ul>
      </details>
    </section>
  );
}
