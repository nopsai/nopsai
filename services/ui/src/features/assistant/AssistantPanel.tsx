import type { FormEvent, KeyboardEvent } from 'react';
import { ObjectIcon } from '../../components/ObjectIcon.js';
import type { AssistantMessage } from './model.js';
import { useAssistantController } from './useAssistantController.js';

export function AssistantPanel({
  variant = 'page',
  onExpand,
  onClose,
}: {
  variant?: 'page' | 'dock';
  onExpand?: () => void;
  onClose?: () => void;
}) {
  const assistant = useAssistantController();
  const compact = variant === 'dock';

  const submit = (event: FormEvent) => {
    event.preventDefault();
    void assistant.submitMessage();
  };

  const handleDraftKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      void assistant.submitMessage();
    }
  };

  return (
    <section className={compact ? 'flex h-full flex-col bg-[var(--bg-primary)]' : 'flex h-full min-h-0 flex-col'}>
      <header className="flex items-center justify-between gap-3 border-b border-[var(--border-primary)] px-4 py-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="inline-flex h-8 w-8 items-center justify-center rounded-md bg-[var(--bg-tertiary)] text-[var(--text-primary)]">
            <ObjectIcon type="assistant" className="h-4 w-4" />
          </span>
          <div className="min-w-0">
            <h2 className="truncate text-base font-semibold text-[var(--text-primary)]">Nopsai AI Assistant</h2>
            <p className="truncate text-xs text-[var(--text-secondary)]">Permission-bound platform help</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {onExpand && (
            <button
              type="button"
              className="rounded-md border border-[var(--border-primary)] px-2.5 py-1.5 text-xs font-medium hover:bg-[var(--bg-tertiary)]"
              onClick={onExpand}
            >
              Full page
            </button>
          )}
          {onClose && (
            <button
              type="button"
              className="rounded-md border border-[var(--border-primary)] px-2.5 py-1.5 text-xs font-medium hover:bg-[var(--bg-tertiary)]"
              onClick={onClose}
              aria-label="Close assistant"
            >
              Close
            </button>
          )}
        </div>
      </header>

      <div className={compact ? 'grid min-h-0 flex-1 grid-rows-[auto_1fr_auto]' : 'grid min-h-0 flex-1 grid-cols-[260px_minmax(0,1fr)_280px]'}>
        <aside className={compact ? 'border-b border-[var(--border-primary)] p-3' : 'min-h-0 overflow-auto border-r border-[var(--border-primary)] p-4'}>
          <div className="space-y-3">
            <label className="block text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
              LLM profile
              <select
                className="mt-1 w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-2 py-2 text-sm normal-case text-[var(--text-primary)]"
                value={assistant.selectedProfile}
                onChange={event => assistant.setSelectedProfile(event.target.value)}
                disabled={assistant.profileOptions.length === 0}
              >
                {assistant.profileOptions.length === 0 ? (
                  <option value="">No profiles</option>
                ) : (
                  assistant.profileOptions.map(profile => <option key={profile} value={profile}>{profile}</option>)
                )}
              </select>
            </label>
            <button
              type="button"
              className="w-full rounded-md bg-[var(--border-accent)] px-3 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-60"
              onClick={() => void assistant.startConversation()}
              disabled={assistant.sending}
            >
              New conversation
            </button>
          </div>

          {!compact && (
            <div className="mt-5 space-y-2">
              <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Conversations</h2>
              {assistant.conversations.length === 0 && (
                <p className="rounded-md border border-dashed border-[var(--border-primary)] px-3 py-2 text-sm text-[var(--text-secondary)]">No conversations yet.</p>
              )}
              {assistant.conversations.map(conversation => (
                <button
                  key={conversation.id}
                  type="button"
                  className={`w-full rounded-md border px-3 py-2 text-left text-sm hover:bg-[var(--bg-tertiary)] ${assistant.activeConversation?.id === conversation.id ? 'border-[var(--border-accent)] bg-[var(--bg-tertiary)]' : 'border-[var(--border-primary)]'}`}
                  onClick={() => void assistant.selectConversation(conversation.id)}
                >
                  <span className="block truncate font-medium text-[var(--text-primary)]">{conversation.title || 'Untitled conversation'}</span>
                  <span className="block truncate text-xs text-[var(--text-secondary)]">{conversation.selected_llm_profile || 'No profile selected'}</span>
                </button>
              ))}
            </div>
          )}
        </aside>

        <main className="flex min-h-0 flex-col">
          {assistant.error && (
            <div className="border-b border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">
              {assistant.error}
            </div>
          )}
          <div className="min-h-0 flex-1 space-y-3 overflow-auto px-4 py-4">
            {assistant.loading ? (
              <p className="text-sm text-[var(--text-secondary)]">Loading assistant...</p>
            ) : assistant.activeMessages.length === 0 ? (
              <div className="rounded-md border border-dashed border-[var(--border-primary)] p-4 text-sm text-[var(--text-secondary)]">
                Ask about a failed run, pipeline YAML, schedules, scopes, costs, docs, or system status.
              </div>
            ) : (
              assistant.activeMessages.map(message => <AssistantMessageBubble key={message.id} message={message} />)
            )}
          </div>
          <form onSubmit={submit} className="border-t border-[var(--border-primary)] p-3">
            <textarea
              className="min-h-24 w-full resize-none rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--border-accent)]"
              value={assistant.draft}
              onChange={event => assistant.setDraft(event.target.value)}
              onKeyDown={handleDraftKeyDown}
              placeholder="Ask the assistant..."
            />
            <div className="mt-2 flex items-center justify-between gap-3">
              <span className="text-xs text-[var(--text-secondary)]">Ctrl/Cmd Enter sends</span>
              <button
                type="submit"
                className="rounded-md bg-[var(--border-accent)] px-4 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-60"
                disabled={!assistant.draft.trim() || assistant.sending}
              >
                {assistant.sending ? 'Sending...' : 'Send'}
              </button>
            </div>
          </form>
        </main>

        {!compact && (
          <aside className="min-h-0 overflow-auto border-l border-[var(--border-primary)] p-4">
            <AssistantContextPanel conversation={assistant.activeConversation} messages={assistant.activeMessages} />
          </aside>
        )}
      </div>
    </section>
  );
}

function AssistantMessageBubble({ message }: { message: AssistantMessage }) {
  const isUser = message.role === 'user';
  return (
    <article className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div className={`max-w-[820px] rounded-md border px-3 py-2 text-sm ${isUser ? 'border-[var(--border-accent)] bg-[var(--bg-tertiary)]' : 'border-[var(--border-primary)] bg-[var(--bg-secondary)]'}`}>
        <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{isUser ? 'You' : 'Assistant'}</div>
        <p className="whitespace-pre-wrap text-[var(--text-primary)]">{message.content}</p>
        {message.tool_calls.length > 0 && (
          <div className="mt-2 space-y-1 border-t border-[var(--border-primary)] pt-2">
            {message.tool_calls.map(tool => (
              <div key={`${message.id}-${tool.name}`} className="text-xs text-[var(--text-secondary)]">
                <span>{tool.name} {tool.status ? `(${tool.status})` : ''}</span>
                {tool.resource_uris.length > 0 && (
                  <span className="block truncate">{tool.resource_uris.join(', ')}</span>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </article>
  );
}

function AssistantContextPanel({
  conversation,
  messages,
}: {
  conversation: ReturnType<typeof useAssistantController>['activeConversation'];
  messages: AssistantMessage[];
}) {
  const memory = conversation?.memory;
  const toolCalls = messages.flatMap(message => message.tool_calls);
  const proposedChanges = proposedChangesFromMessages(messages);
  return (
    <div className="space-y-5">
      <section>
        <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Conversation memory</h2>
        <dl className="mt-2 space-y-2 text-sm">
          <ContextRow label="Docs" value={memory?.selected_docs_version || conversation?.docs_version || 'auto'} />
          <ContextRow label="Run" value={memory?.selected_run || 'None'} />
          <ContextRow label="Pipeline" value={memory?.selected_pipeline || 'None'} />
          <ContextRow label="Scope" value={memory?.selected_scope || conversation?.scope || 'None'} />
        </dl>
        {memory?.summary && <p className="mt-3 rounded-md bg-[var(--bg-tertiary)] p-3 text-sm text-[var(--text-secondary)]">{memory.summary}</p>}
      </section>

      <section>
        <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Tool activity</h2>
        {toolCalls.length === 0 ? (
          <p className="mt-2 rounded-md border border-dashed border-[var(--border-primary)] p-3 text-sm text-[var(--text-secondary)]">No tools used yet.</p>
        ) : (
          <ul className="mt-2 space-y-2">
            {toolCalls.map((tool, index) => (
              <li key={`${tool.name}-${index}`} className="rounded-md border border-[var(--border-primary)] p-2 text-sm">
                <span className="block font-medium text-[var(--text-primary)]">{tool.name}</span>
                <span className="block text-xs text-[var(--text-secondary)]">{tool.status || 'completed'}</span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Proposed changes</h2>
        {proposedChanges.length === 0 ? (
          <p className="mt-2 rounded-md border border-dashed border-[var(--border-primary)] p-3 text-sm text-[var(--text-secondary)]">
            Draft changes will appear here for review before any apply flow.
          </p>
        ) : (
          <div className="mt-2 space-y-3">
            {proposedChanges.map(change => (
              <article key={change.key} className="rounded-md border border-[var(--border-primary)] p-3">
                <h3 className="text-sm font-semibold text-[var(--text-primary)]">{change.title}</h3>
                {change.note && <p className="mt-1 text-xs text-[var(--text-secondary)]">{change.note}</p>}
                <pre className="mt-2 max-h-72 overflow-auto rounded-md bg-[var(--bg-tertiary)] p-2 text-xs text-[var(--text-primary)]">
                  {change.body}
                </pre>
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function ContextRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <dt className="text-[var(--text-secondary)]">{label}</dt>
      <dd className="truncate text-right font-medium text-[var(--text-primary)]">{value}</dd>
    </div>
  );
}

type ProposedChange = {
  key: string;
  title: string;
  body: string;
  note: string;
};

function proposedChangesFromMessages(messages: AssistantMessage[]): ProposedChange[] {
  return messages.flatMap(message => message.tool_calls.flatMap((tool, index) => {
    const proposalType = readRecordString(tool.output, 'proposal_type');
    const yaml = readRecordString(tool.output, 'yaml');
    if (!proposalType && !yaml) return [];

    const body = yaml || prettyPrintRecord(tool.output['target']) || prettyPrintRecord(tool.output);
    return [{
      key: `${message.id}-${tool.name}-${index}`,
      title: proposalTitle(proposalType || tool.name),
      body,
      note: tool.output['applies'] === false ? 'Proposal only. No changes were applied.' : '',
    }];
  }));
}

function proposalTitle(value: string): string {
  return value
    .replace(/^nopsai\./, '')
    .replace(/_/g, ' ')
    .replace(/\b\w/g, char => char.toUpperCase());
}

function readRecordString(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  return typeof value === 'string' ? value.trim() : '';
}

function prettyPrintRecord(value: unknown): string {
  if (!value) return '';
  try {
    return JSON.stringify(value, null, 2) || '';
  } catch {
    return String(value);
  }
}
