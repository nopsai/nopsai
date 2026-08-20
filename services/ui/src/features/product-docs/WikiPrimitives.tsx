import { Check, Copy } from 'lucide-react';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { findWikiArticle, wikiArticlePath, type WikiExample } from './content/index.js';

export function WikiBlock({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section id={id} className="scroll-mt-20">
      <h2 className="docs-h2">{title}</h2>
      <div>{children}</div>
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
    <div className={`docs-note${tone === 'warning' ? ' docs-note--warning' : ''}`}>
      <span className="docs-note-title">{title}</span>
      <div>{children}</div>
    </div>
  );
}

export function WikiBulletList({ items, id }: { items: string[]; id: string }) {
  return (
    <ul className="docs-prose">
      {items.map((item, index) => (
        <li key={`${id}-${index}`}>
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
 * Renders `**bold**`, `` `code` `` and `[label](article-id)` spans.
 *
 * Wiki copy is plain data, not markdown documents, so a full parser would be
 * more machinery than the three marks the content actually uses. A link names an
 * article ID rather than a path: pages move between sections, and a stored path
 * would rot the next time they do.
 */
export function InlineMarkup({ value }: { value: string }) {
  const parts = value.split(/(\*\*[^*]+\*\*|`[^`]+`|\[[^\]]+\]\([a-z0-9-]+\))/g).filter(Boolean);
  return (
    <>
      {parts.map((part, index) => {
        const link = /^\[([^\]]+)\]\(([a-z0-9-]+)\)$/.exec(part);
        if (link) {
          const location = findWikiArticle(link[2]);
          // An unresolvable link renders as plain text rather than a dead link;
          // content.test.ts fails the build so it never ships that way.
          if (!location) return <span key={index}>{link[1]}</span>;
          return (
            <Link
              key={index}
              to={wikiArticlePath(location.section.id, location.article.id)}
              className="docs-inline-link"
            >
              {link[1]}
            </Link>
          );
        }
        if (part.startsWith('**') && part.endsWith('**')) {
          return (
            <strong key={index} className="font-semibold text-[var(--text-primary)]">
              {part.slice(2, -2)}
            </strong>
          );
        }
        if (part.startsWith('`') && part.endsWith('`')) {
          return (
            <code key={index} className="docs-inline-code">
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
    <figure className="docs-figure">
      <figcaption className="docs-figure-bar">
        <span className="min-w-0 truncate font-medium text-[var(--docs-text)]">{example.title}</span>
        <span className="flex shrink-0 items-center gap-2">
          <span className="text-[11px] uppercase tracking-wide text-[var(--docs-faint)]">{example.language}</span>
          <button
            type="button"
            onClick={() => onCopy(copyKey, example.code)}
            className="inline-flex h-6 items-center gap-1 rounded px-1.5 text-[12px] text-[var(--docs-muted)] hover:bg-[var(--docs-hover)] hover:text-[var(--docs-text)]"
            aria-label={copied ? `Copied ${example.title}` : `Copy ${example.title}`}
          >
            {copied ? <Check className="h-3.5 w-3.5" aria-hidden="true" /> : <Copy className="h-3.5 w-3.5" aria-hidden="true" />}
            {copied ? 'Copied' : 'Copy'}
          </button>
        </span>
      </figcaption>
      <pre className="docs-pre">
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
