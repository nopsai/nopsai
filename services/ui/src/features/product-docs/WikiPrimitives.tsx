import { Check, Copy } from 'lucide-react';
import type { ReactNode } from 'react';
import type { WikiExample } from './content/index.js';

export function WikiBlock({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section id={id} className="scroll-mt-8 pt-8">
      <h2 className="text-lg font-semibold tracking-tight text-[var(--text-primary)]">{title}</h2>
      <div className="mt-3">{children}</div>
    </section>
  );
}

export function WikiNotice({
  title,
  tone = 'neutral',
  children,
}: {
  title: string;
  tone?: 'neutral' | 'warning';
  children: ReactNode;
}) {
  return (
    <div
      className={`mt-3 rounded border px-3 py-2 ${
        tone === 'warning' ? 'border-amber-500/50' : 'border-[var(--border-primary)]'
      }`}
    >
      <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{title}</p>
      <div className="mt-1 text-sm leading-6 text-[var(--text-secondary)]">{children}</div>
    </div>
  );
}

export function WikiBulletList({ items, id }: { items: string[]; id: string }) {
  return (
    <ul className="space-y-2 pl-5 text-sm leading-6 text-[var(--text-secondary)]">
      {items.map((item, index) => (
        <li key={`${id}-${index}`} className="list-disc">
          <InlineMarkup value={item} />
        </li>
      ))}
    </ul>
  );
}

export function WikiChip({ children, tone = 'neutral' }: { children: ReactNode; tone?: 'neutral' | 'accent' }) {
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium ${
        tone === 'accent'
          ? 'border-[var(--border-input-focus)] text-[var(--text-accent)]'
          : 'border-[var(--border-primary)] text-[var(--text-secondary)]'
      }`}
    >
      {children}
    </span>
  );
}

/**
 * Renders `**bold**` and `` `code` `` spans.
 *
 * Wiki copy is plain data, not markdown documents, so a full parser would be
 * more machinery than the two marks the content actually uses.
 */
export function InlineMarkup({ value }: { value: string }) {
  const parts = value.split(/(\*\*[^*]+\*\*|`[^`]+`)/g).filter(Boolean);
  return (
    <>
      {parts.map((part, index) => {
        if (part.startsWith('**') && part.endsWith('**')) {
          return (
            <strong key={index} className="font-semibold text-[var(--text-primary)]">
              {part.slice(2, -2)}
            </strong>
          );
        }
        if (part.startsWith('`') && part.endsWith('`')) {
          return (
            <code key={index} className="rounded border border-[var(--border-primary)] bg-[var(--bg-tertiary)] px-1 py-0.5 text-[0.92em] text-[var(--text-primary)]">
              {part.slice(1, -1)}
            </code>
          );
        }
        return <span key={index}>{part}</span>;
      })}
    </>
  );
}

export function WikiCodeBlock({
  example,
  copyKey,
  copiedKey,
  onCopy,
}: {
  example: WikiExample;
  copyKey: string;
  copiedKey: string;
  onCopy: (key: string, code: string) => void;
}) {
  const copied = copiedKey === copyKey;
  return (
    <figure className="overflow-hidden rounded border border-[var(--border-primary)]">
      <figcaption className="flex items-center justify-between gap-3 border-b border-[var(--border-primary)] px-3 py-1.5">
        <span className="min-w-0 truncate text-sm font-medium text-[var(--text-primary)]">{example.title}</span>
        <span className="flex shrink-0 items-center gap-2">
          <span className="text-xs uppercase tracking-wide text-[var(--text-secondary)]">{example.language}</span>
          <button
            type="button"
            onClick={() => onCopy(copyKey, example.code)}
            className="inline-flex h-7 items-center gap-1 rounded px-2 text-sm text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
            aria-label={copied ? `Copied ${example.title}` : `Copy ${example.title}`}
          >
            {copied ? <Check className="h-3.5 w-3.5" aria-hidden="true" /> : <Copy className="h-3.5 w-3.5" aria-hidden="true" />}
            {copied ? 'Copied' : 'Copy'}
          </button>
        </span>
      </figcaption>
      <pre className="overflow-x-auto px-3 py-2.5 text-sm leading-6 text-[var(--text-secondary)]">
        <code>{example.code}</code>
      </pre>
      {example.expectedOutput ? (
        <div className="border-t border-[var(--border-primary)] px-3 py-2">
          <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Result</p>
          <p className="mt-1 text-sm leading-6 text-[var(--text-secondary)]">
            <InlineMarkup value={example.expectedOutput} />
          </p>
        </div>
      ) : null}
      {example.placeholders?.length ? (
        <div className="border-t border-[var(--border-primary)] px-3 py-2">
          <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Replace before running</p>
          <ul className="mt-1 space-y-1 pl-5 text-sm leading-6 text-[var(--text-secondary)]">
            {example.placeholders.map(item => (
              <li key={item} className="list-disc">
                <InlineMarkup value={item} />
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {example.validationCommand ? (
        <div className="border-t border-[var(--border-primary)] px-3 py-2">
          <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Verify with</p>
          <code className="mt-1 block text-sm text-[var(--text-secondary)]">{example.validationCommand}</code>
        </div>
      ) : null}
    </figure>
  );
}
