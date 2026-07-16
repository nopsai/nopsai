import type { DocumentationArticle, DocumentationField } from './quality';
import { CompactList, MetadataItem, Notice, SectionFrame } from './DocumentationPrimitives';

export function FieldReference({ article }: { article: DocumentationArticle }) {
  if (!article.configRows.length) return null;
  return (
    <SectionFrame id="configuration" title="Field reference">
      <div className="space-y-2">
        {article.configRows.map(field => <FieldCard key={`${article.id}-${field.key}`} field={field} />)}
      </div>
    </SectionFrame>
  );
}

function FieldCard({ field }: { field: DocumentationField }) {
  const constraints = [
    ...(field.allowedValues?.length ? [`Allowed values: ${field.allowedValues.join(', ')}`] : []),
    ...(field.constraints || []),
    ...(field.inheritedFrom?.length ? [`Inherited from: ${field.inheritedFrom.join(', ')}`] : []),
    ...(field.permission ? [`Permission: ${field.permission}`] : []),
    ...(field.security ? [`Security: ${field.security}`] : []),
    ...(field.deprecatedIn ? [`Deprecated in: ${field.deprecatedIn}`] : []),
  ];

  return (
    <details id={field.anchor} className="scroll-mt-8 rounded border border-[var(--border-primary)] px-4 py-3">
      <summary className="cursor-pointer list-none marker:hidden">
        <span className="flex flex-wrap items-center justify-between gap-3">
          <code className="text-sm font-semibold text-[var(--text-primary)]">{field.path || field.key}</code>
          <span className="text-xs text-[var(--text-tertiary)]">
            {field.metadataStatus === 'verified' ? `${field.displayType} · ${field.displayRequired}` : 'Metadata incomplete'}
          </span>
        </span>
        <span className="mt-1 block text-sm leading-6 text-[var(--text-secondary)]">{field.description}</span>
      </summary>
      <div className="mt-4 border-t border-[var(--border-primary)] pt-4">
        {field.metadataStatus === 'partial' ? (
          <Notice title="Documentation status">Type, requirement, or default information has not been explicitly verified in the documentation source.</Notice>
        ) : null}
        <dl className="grid gap-x-6 gap-y-3 text-xs sm:grid-cols-2 lg:grid-cols-4">
          <MetadataItem label="Type" value={field.displayType} />
          <MetadataItem label="Required" value={field.displayRequired} />
          <MetadataItem label="Default" value={field.displayDefault} />
          <MetadataItem label="Scope" value={field.scope || field.area} />
        </dl>
        {constraints.length ? <CompactList title="Constraints" items={constraints} /> : null}
        <div className="mt-4">
          <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">Example</p>
          <code className="mt-1 block overflow-x-auto rounded bg-[var(--bg-secondary)] px-3 py-2 text-xs text-[var(--text-secondary)]">{field.example}</code>
        </div>
        <a className="mt-4 inline-block text-xs font-medium text-[var(--text-accent)]" href={`#${field.anchor}`}>Link to this field</a>
      </div>
    </details>
  );
}

export function OperationalGuidance({ article }: { article: DocumentationArticle }) {
  if (!article.runbookEntries.length) return null;
  const complete = article.runbookEntries.filter(runbook => runbook.complete);
  const incomplete = article.runbookEntries.filter(runbook => !runbook.complete);

  return (
    <SectionFrame id="operations" title="Operational guidance">
      {complete.length ? (
        <div className="space-y-3">
          {complete.map(runbook => (
            <details key={`${article.id}-${runbook.id}`} className="rounded border border-[var(--border-primary)] px-4 py-3">
              <summary className="cursor-pointer font-medium text-[var(--text-primary)]">{runbook.title}</summary>
              <div className="mt-4 border-t border-[var(--border-primary)] pt-4">
                <p className="text-sm leading-7 text-[var(--text-secondary)]">{runbook.impact}</p>
                <CompactList title="Symptoms" items={runbook.symptoms} />
                <CompactList title="Initial checks" items={runbook.initialChecks} />
                {runbook.diagnosticCommands.length ? <CompactList title="Diagnostic commands" items={runbook.diagnosticCommands} code /> : null}
                <CompactList title="Resolution" items={runbook.resolution} />
                {runbook.rollback ? <Notice title="Rollback">{runbook.rollback}</Notice> : null}
                {runbook.metrics?.length ? <CompactList title="Metrics and logs" items={runbook.metrics} /> : null}
                <dl className="mt-4 grid gap-3 text-xs sm:grid-cols-2">
                  <MetadataItem label="Required access" value={runbook.requiredAccess} />
                  <MetadataItem label="Escalation" value={runbook.escalation || 'Escalate to the article owner.'} />
                </dl>
              </div>
            </details>
          ))}
        </div>
      ) : null}
      {incomplete.length ? (
        <div className={complete.length ? 'mt-5' : ''}>
          <p className="text-sm font-medium text-[var(--text-primary)]">Related operational tasks</p>
          <p className="mt-1 text-sm leading-6 text-[var(--text-secondary)]">These tasks are indexed, but detailed runbook steps have not yet been documented.</p>
          <ul className="mt-3 flex flex-wrap gap-2">
            {incomplete.map(runbook => <li key={`${article.id}-${runbook.id}`} className="rounded-full border border-[var(--border-primary)] px-3 py-1.5 text-xs text-[var(--text-secondary)]">{runbook.title}</li>)}
          </ul>
        </div>
      ) : null}
    </SectionFrame>
  );
}
