import { useState } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { wikiRouteAccessLabels, type WikiApiRoute } from './content/index.js';
import { wikiOperationAnchor } from './apiAnchors.js';
import { InlineMarkup, WikiCodeBlock } from './WikiPrimitives.js';

const methodTone: Record<string, string> = {
  GET: 'text-[var(--docs-accent)]',
  POST: 'text-emerald-600 dark:text-emerald-400',
  PUT: 'text-amber-600 dark:text-amber-400',
  PATCH: 'text-amber-600 dark:text-amber-400',
  DELETE: 'text-red-600 dark:text-red-400',
  ANY: 'text-[var(--docs-muted)]',
};

export function WikiApiOperations({
  routes,
  articleID,
  targetAnchor,
  copiedKey,
  onCopy,
}: {
  routes: WikiApiRoute[];
  articleID: string;
  targetAnchor?: string;
  copiedKey: string;
  onCopy: (key: string, code: string) => void;
}) {
  return (
    <div className="space-y-4">
      {routes.map(route => (
        <WikiApiOperation
          key={`${route.method} ${route.path}`}
          route={route}
          articleID={articleID}
          expanded={targetAnchor === wikiOperationAnchor(route)}
          copiedKey={copiedKey}
          onCopy={onCopy}
        />
      ))}
    </div>
  );
}

function WikiApiOperation({
  route,
  articleID,
  expanded,
  copiedKey,
  onCopy,
}: {
  route: WikiApiRoute;
  articleID: string;
  expanded: boolean;
  copiedKey: string;
  onCopy: (key: string, code: string) => void;
}) {
  const [open, setOpen] = useState(expanded);
  const anchor = wikiOperationAnchor(route);

  return (
    <section id={anchor} className="scroll-mt-24 overflow-hidden rounded-md border border-[var(--docs-border)]">
      <button
        type="button"
        onClick={() => setOpen(value => !value)}
        aria-expanded={open}
        className="flex w-full items-baseline gap-3 px-4 py-3 text-left hover:bg-[var(--docs-hover)]"
      >
        {open ? (
          <ChevronDown className="h-3.5 w-3.5 shrink-0 self-center text-[var(--docs-faint)]" aria-hidden="true" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5 shrink-0 self-center text-[var(--docs-faint)]" aria-hidden="true" />
        )}
        <span className={`shrink-0 font-mono text-[12px] font-bold ${methodTone[route.method] || ''}`}>{route.method}</span>
        <span className="min-w-0 flex-1 truncate font-mono text-[13px] text-[var(--docs-text)]">{route.path}</span>
        <span className="shrink-0 text-[11px] uppercase tracking-wide text-[var(--docs-faint)]">
          {wikiRouteAccessLabels[route.access]}
        </span>
      </button>

      <div className="border-t border-[var(--docs-border)] px-4 py-3">
        <p className="text-[13px] leading-6 text-[var(--docs-muted)]">
          <InlineMarkup value={route.purpose} />
        </p>
        {route.permission ? (
          <p className="mt-1 text-[12px] text-[var(--docs-faint)]">
            Requires <code className="docs-inline-code">{route.permission}</code>
          </p>
        ) : null}

        {open ? (
          <div className="mt-4 space-y-5">
            {route.parameters?.length ? (
              <OperationBlock title="Parameters">
                <div className="docs-table-scroll">
                  <table className="docs-table">
                    <thead>
                      <tr>
                        <th>Name</th>
                        <th>In</th>
                        <th>Type</th>
                        <th>Required</th>
                        <th>Description</th>
                      </tr>
                    </thead>
                    <tbody>
                      {route.parameters.map(parameter => (
                        <tr key={`${parameter.in}-${parameter.name}`}>
                          <td>
                            <code className="docs-inline-code">{parameter.name}</code>
                          </td>
                          <td>{parameter.in}</td>
                          <td>{parameter.type}</td>
                          <td>{parameter.required ? 'Required' : 'Optional'}</td>
                          <td>
                            <InlineMarkup value={parameter.description} />
                            {parameter.allowedValues?.length ? (
                              <span className="mt-1 block text-[12px] text-[var(--docs-faint)]">
                                Allowed: {parameter.allowedValues.join(', ')}
                              </span>
                            ) : null}
                            {parameter.defaultValue ? (
                              <span className="mt-1 block text-[12px] text-[var(--docs-faint)]">
                                Default: {parameter.defaultValue}
                              </span>
                            ) : null}
                            {parameter.repeatable ? (
                              <span className="mt-1 block text-[12px] text-[var(--docs-faint)]">Repeatable</span>
                            ) : null}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </OperationBlock>
            ) : null}

            {route.requestSample ? (
              <OperationBlock title="Call it">
                <WikiCodeBlock
                  example={route.requestSample}
                  copyKey={`${articleID}-${anchor}-request`}
                  copiedKey={copiedKey}
                  onCopy={onCopy}
                />
              </OperationBlock>
            ) : null}

            {route.responses?.length ? (
              <OperationBlock title="Responses">
                <div className="space-y-3">
                  {route.responses.map(response => (
                    <div key={response.status} className="rounded border border-[var(--docs-border)] px-3 py-2">
                      <p className="text-[13px] font-medium text-[var(--docs-text)]">
                        {response.status}
                        {response.contentType ? (
                          <span className="ml-2 font-normal text-[12px] text-[var(--docs-faint)]">{response.contentType}</span>
                        ) : null}
                      </p>
                      <p className="mt-0.5 text-[13px] leading-6 text-[var(--docs-muted)]">
                        <InlineMarkup value={response.description} />
                      </p>
                      {response.sample ? (
                        <pre className="docs-pre mt-2 rounded">
                          <code>{response.sample}</code>
                        </pre>
                      ) : null}
                    </div>
                  ))}
                </div>
              </OperationBlock>
            ) : null}

            {route.errors?.length ? (
              <OperationBlock title="When it fails">
                <div className="docs-table-scroll">
                  <table className="docs-table">
                    <thead>
                      <tr>
                        <th>Status</th>
                        <th>Cause</th>
                        <th>What to do</th>
                      </tr>
                    </thead>
                    <tbody>
                      {route.errors.map(error => (
                        <tr key={`${error.status}-${error.cause}`}>
                          <td>
                            {error.status}
                            {error.code ? <span className="block text-[12px] text-[var(--docs-faint)]">{error.code}</span> : null}
                          </td>
                          <td>
                            <InlineMarkup value={error.cause} />
                          </td>
                          <td>
                            <InlineMarkup value={error.action} />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </OperationBlock>
            ) : null}

            {route.streaming ? (
              <OperationBlock title="Streaming">
                <p className="text-[13px] leading-6 text-[var(--docs-muted)]">
                  {route.streaming.contentType} · {route.streaming.framing}
                </p>
              </OperationBlock>
            ) : null}

            {route.sideEffects?.length ? (
              <OperationBlock title="Side effects">
                <ul className="docs-prose">
                  {route.sideEffects.map(effect => (
                    <li key={effect}>
                      <InlineMarkup value={effect} />
                    </li>
                  ))}
                </ul>
              </OperationBlock>
            ) : null}

            {route.notes ? (
              <OperationBlock title="Notes">
                <p className="text-[13px] leading-6 text-[var(--docs-muted)]">
                  <InlineMarkup value={route.notes} />
                </p>
              </OperationBlock>
            ) : null}

            {route.coveringTests?.length || route.evidence?.length ? (
              <OperationBlock title="Proven by">
                <ul className="space-y-1">
                  {[...(route.coveringTests || []), ...(route.evidence || [])].map(path => (
                    <li key={path}>
                      <code className="block break-all text-[12px] text-[var(--docs-muted)]">{path}</code>
                    </li>
                  ))}
                </ul>
              </OperationBlock>
            ) : null}
          </div>
        ) : null}
      </div>
    </section>
  );
}

function OperationBlock({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <p className="text-[11px] font-bold uppercase tracking-wider text-[var(--docs-faint)]">{title}</p>
      <div className="mt-1.5">{children}</div>
    </div>
  );
}
