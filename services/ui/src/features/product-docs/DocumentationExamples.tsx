import { Check, Copy } from 'lucide-react';
import type { WikiExample } from './model';
import type { DocumentationArticle } from './quality';
import { CompactList, Notice, SectionFrame } from './DocumentationPrimitives';

export function Examples({
  article,
  copiedKey,
  onCopy,
}: {
  article: DocumentationArticle;
  copiedKey: string;
  onCopy: (key: string, code: string) => void;
}) {
  if (!article.examples.length) return null;
  return (
    <SectionFrame id="examples" title="Examples">
      <div className="space-y-6">
        {article.examples.map((example, index) => (
          <CodeExample
            key={`${article.id}-${example.title}`}
            example={example}
            copyKey={`${article.id}-example-${index}`}
            copiedKey={copiedKey}
            onCopy={onCopy}
          />
        ))}
      </div>
    </SectionFrame>
  );
}

export function CodeExample({
  example,
  copyKey,
  copiedKey,
  onCopy,
  compact = false,
}: {
  example: WikiExample;
  copyKey: string;
  copiedKey: string;
  onCopy: (key: string, code: string) => void;
  compact?: boolean;
}) {
  const copied = copiedKey === copyKey;
  return (
    <div>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text-primary)]">{example.title}</h3>
          <p className="mt-1 text-xs text-[var(--text-tertiary)]">
            {example.language} · {example.complete === false ? 'Partial snippet' : 'Complete example'}
            {example.testedIn ? ` · Tested ${example.testedIn}` : ''}
          </p>
        </div>
        <button
          type="button"
          onClick={() => onCopy(copyKey, example.code)}
          className="inline-flex h-8 items-center gap-2 rounded border border-[var(--border-primary)] px-3 text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
        >
          {copied ? <Check className="h-3.5 w-3.5" aria-hidden="true" /> : <Copy className="h-3.5 w-3.5" aria-hidden="true" />}
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <pre className={`mt-2 overflow-x-auto rounded bg-[var(--bg-secondary)] px-4 py-3 font-mono text-xs leading-6 text-[var(--text-secondary)] ${compact ? '' : 'border border-[var(--border-primary)]'}`}>
        <code className={`language-${example.language}`}>{example.code}</code>
      </pre>
      {example.placeholderNotes?.length ? <CompactList title="Replace" items={example.placeholderNotes} /> : null}
      {example.expectedOutput ? <Notice title="Expected result">{example.expectedOutput}</Notice> : null}
      {example.validationCommand ? <Notice title="Validation"><code>{example.validationCommand}</code></Notice> : null}
      {example.rollback ? <Notice title="Cleanup or rollback"><code>{example.rollback}</code></Notice> : null}
    </div>
  );
}
