import { useEffect, useRef, type FormEvent, type KeyboardEvent } from 'react';
import { Check, Copy, Loader2, Maximize2, Plus, RefreshCw, RotateCcw, Send, Trash2, X } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon.js';
import {
  assistantMessageAuthorLabel,
  assistantVisibleToolActivity,
  type AssistantConversation,
  type AssistantMessage,
} from './model.js';
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
  const transcriptRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const transcript = transcriptRef.current;
    if (!transcript) return;
    transcript.scrollTop = transcript.scrollHeight;
  }, [assistant.activeConversation?.id, assistant.activeMessages.length, assistant.sending, assistant.loading]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!assistant.enabled) return;
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
          <button
            type="button"
            className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-primary)] hover:bg-[var(--bg-tertiary)] disabled:cursor-not-allowed disabled:opacity-50"
            onClick={() => void assistant.load()}
            disabled={assistant.loading || assistant.sending}
            aria-label="Refresh assistant"
            title="Refresh"
          >
            <RefreshCw className={`h-4 w-4 ${assistant.loading ? 'animate-spin' : ''}`} aria-hidden="true" />
          </button>
          <button
            type="button"
            className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-primary)] hover:bg-[var(--bg-tertiary)] disabled:cursor-not-allowed disabled:opacity-50"
            onClick={() => void assistant.retryLastUserMessage()}
            disabled={!assistant.enabled || !assistant.canRetry || assistant.sending || assistant.loading}
            aria-label="Retry last prompt"
            title="Retry last prompt"
          >
            <RotateCcw className={`h-4 w-4 ${assistant.retrying ? 'animate-spin' : ''}`} aria-hidden="true" />
          </button>
          <button
            type="button"
            className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-primary)] text-red-500 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-red-950/30"
            onClick={() => void assistant.deleteConversation()}
            disabled={!assistant.activeConversation || assistant.sending || assistant.loading || Boolean(assistant.deletingConversationID)}
            aria-label="Delete conversation"
            title="Delete conversation"
          >
            {assistant.deletingConversationID === assistant.activeConversation?.id ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" /> : <Trash2 className="h-4 w-4" aria-hidden="true" />}
          </button>
          {onExpand && (
            <button
              type="button"
              className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-primary)] hover:bg-[var(--bg-tertiary)]"
              onClick={onExpand}
              aria-label="Open full assistant page"
              title="Full page"
            >
              <Maximize2 className="h-4 w-4" aria-hidden="true" />
            </button>
          )}
          {onClose && (
            <button
              type="button"
              className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-primary)] hover:bg-[var(--bg-tertiary)]"
              onClick={onClose}
              aria-label="Close assistant"
              title="Close"
            >
              <X className="h-4 w-4" aria-hidden="true" />
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
                disabled={!assistant.enabled || assistant.loading || assistant.profileOptions.length === 0}
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
              className="inline-flex w-full items-center justify-center gap-2 rounded-md bg-[var(--border-accent)] px-3 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-60"
              onClick={() => void assistant.startConversation()}
              disabled={!assistant.enabled || assistant.loading || assistant.sending}
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
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
                <div
                  key={conversation.id}
                  className={`group flex items-center gap-2 rounded-md border px-2 py-2 text-sm hover:bg-[var(--bg-tertiary)] ${assistant.activeConversation?.id === conversation.id ? 'border-[var(--border-accent)] bg-[var(--bg-tertiary)]' : 'border-[var(--border-primary)]'}`}
                >
                  <button
                    type="button"
                    className="min-w-0 flex-1 text-left"
                    onClick={() => void assistant.selectConversation(conversation.id)}
                  >
                    <span className="block truncate font-medium text-[var(--text-primary)]">{conversation.title || 'Untitled conversation'}</span>
                    <span className="block truncate text-xs text-[var(--text-secondary)]">{conversation.selected_llm_profile || 'No profile selected'}</span>
                  </button>
                  <button
                    type="button"
                    className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-[var(--text-secondary)] opacity-0 hover:bg-red-50 hover:text-red-600 focus:opacity-100 group-hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-red-950/30"
                    onClick={() => void assistant.deleteConversation(conversation.id)}
                    disabled={assistant.sending || assistant.deletingConversationID === conversation.id}
                    aria-label={`Delete conversation ${conversation.title || conversation.id}`}
                    title="Delete"
                  >
                    {assistant.deletingConversationID === conversation.id ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" /> : <Trash2 className="h-4 w-4" aria-hidden="true" />}
                  </button>
                </div>
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
          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border-primary)] px-4 py-2 text-xs text-[var(--text-secondary)]">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <span className="rounded-md bg-[var(--bg-tertiary)] px-2 py-1">{assistant.config?.mcp.enabled ? 'MCP on' : 'MCP off'}</span>
              <span className="rounded-md bg-[var(--bg-tertiary)] px-2 py-1">{assistant.config?.memory.enabled ? 'Memory on' : 'Memory off'}</span>
              <span className="rounded-md bg-[var(--bg-tertiary)] px-2 py-1">{assistant.config?.actions.require_confirmation !== false ? 'Confirm required' : 'Confirm policy relaxed'}</span>
            </div>
            <button
              type="button"
              className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-primary)] hover:bg-[var(--bg-tertiary)] disabled:cursor-not-allowed disabled:opacity-50"
              onClick={() => void assistant.copyConversation()}
              disabled={!assistant.activeConversation || assistant.activeMessages.length === 0}
              aria-label="Copy conversation"
              title="Copy conversation"
            >
              {assistant.conversationCopied ? <Check className="h-4 w-4 text-emerald-500" aria-hidden="true" /> : <Copy className="h-4 w-4" aria-hidden="true" />}
            </button>
          </div>
          <div ref={transcriptRef} className="min-h-0 flex-1 space-y-3 overflow-auto px-4 py-4" aria-live="polite">
            {assistant.config && !assistant.enabled ? (
              <div className="rounded-md border border-dashed border-[var(--border-primary)] p-4 text-sm text-[var(--text-secondary)]">
                The assistant is disabled by administrator configuration.
              </div>
            ) : assistant.loading ? (
              <p className="text-sm text-[var(--text-secondary)]">Loading assistant...</p>
            ) : assistant.activeMessages.length === 0 ? (
              <div className="rounded-md border border-dashed border-[var(--border-primary)] p-4 text-sm text-[var(--text-secondary)]">
                Ask about a failed run, pipeline YAML, schedules, scopes, costs, docs, or system status.
              </div>
            ) : (
              <>
                {assistant.activeMessages.map(message => (
                  <AssistantMessageBubble
                    key={message.id}
                    message={message}
                    copied={assistant.copiedMessageID === message.id}
                    retryDisabled={!assistant.enabled || assistant.sending || assistant.loading}
                    onCopy={() => void assistant.copyMessage(message)}
                    onRetry={message.role === 'user' ? () => void assistant.retryLastUserMessage() : undefined}
                  />
                ))}
                {assistant.sending && <AssistantThinkingBubble />}
              </>
            )}
          </div>
          <form onSubmit={submit} className="border-t border-[var(--border-primary)] p-3">
            <textarea
              className="min-h-24 w-full resize-none rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)] outline-none focus:border-[var(--border-accent)]"
              value={assistant.draft}
              onChange={event => assistant.setDraft(event.target.value)}
              onKeyDown={handleDraftKeyDown}
              placeholder="Ask the assistant..."
              disabled={!assistant.enabled || assistant.loading}
            />
            <div className="mt-2 flex items-center justify-between gap-3">
              <span className="text-xs text-[var(--text-secondary)]">Current-user AAA</span>
              <button
                type="submit"
                className="inline-flex h-10 w-10 items-center justify-center rounded-md bg-[var(--border-accent)] text-white disabled:cursor-not-allowed disabled:opacity-60"
                disabled={!assistant.enabled || assistant.loading || !assistant.draft.trim() || assistant.sending}
                aria-label="Send message"
                title="Send"
              >
                {assistant.sending ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" /> : <Send className="h-4 w-4" aria-hidden="true" />}
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

function AssistantMessageBubble({
  message,
  copied,
  retryDisabled,
  onCopy,
  onRetry,
}: {
  message: AssistantMessage;
  copied: boolean;
  retryDisabled: boolean;
  onCopy: () => void;
  onRetry?: () => void;
}) {
  const isUser = message.role === 'user';
  return (
    <article className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div className={`max-w-[820px] rounded-md border px-3 py-2 text-sm shadow-sm ${isUser ? 'border-[var(--border-accent)] bg-[var(--border-accent)] text-white' : 'border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-primary)]'}`}>
        <div className="mb-1 flex items-center justify-between gap-3">
          <div className={`text-xs font-semibold uppercase tracking-wide ${isUser ? 'text-white/75' : 'text-[var(--text-secondary)]'}`}>{assistantMessageAuthorLabel(message)}</div>
          <div className="flex items-center gap-1">
            {onRetry && (
              <button
                type="button"
                className={`inline-flex h-7 w-7 items-center justify-center rounded-md ${isUser ? 'text-white/80 hover:bg-white/15' : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)]'}`}
                onClick={onRetry}
                disabled={retryDisabled}
                aria-label="Retry this prompt"
                title="Retry"
              >
                <RotateCcw className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            )}
            <button
              type="button"
              className={`inline-flex h-7 w-7 items-center justify-center rounded-md ${isUser ? 'text-white/80 hover:bg-white/15' : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)]'}`}
              onClick={onCopy}
              aria-label="Copy message"
              title="Copy"
            >
              {copied ? <Check className="h-3.5 w-3.5" aria-hidden="true" /> : <Copy className="h-3.5 w-3.5" aria-hidden="true" />}
            </button>
          </div>
        </div>
        <p className="whitespace-pre-wrap">{message.content}</p>
      </div>
    </article>
  );
}

function AssistantThinkingBubble() {
  return (
    <article className="flex justify-start">
      <div className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-secondary)] shadow-sm">
        <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
        <span>Thinking</span>
      </div>
    </article>
  );
}

function AssistantContextPanel({
  conversation,
  messages,
}: {
  conversation: AssistantConversation | null;
  messages: AssistantMessage[];
}) {
  const memory = conversation?.memory;
  const toolCalls = assistantVisibleToolActivity(messages);
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
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">NopsAI evidence</h2>
          {toolCalls.length > 0 && <span className="rounded-md bg-[var(--bg-tertiary)] px-2 py-1 text-xs text-[var(--text-secondary)]">{toolCalls.length}</span>}
        </div>
        {toolCalls.length === 0 ? (
          <p className="mt-2 rounded-md border border-dashed border-[var(--border-primary)] p-3 text-sm text-[var(--text-secondary)]">No evidence tools used yet.</p>
        ) : (
          <ul className="mt-2 space-y-2">
            {toolCalls.map((tool, index) => (
              <li key={`${tool.name}-${index}`} className="rounded-md border border-[var(--border-primary)] p-2 text-sm">
                <span className="block font-medium text-[var(--text-primary)]">{tool.name}</span>
                <span className="block text-xs text-[var(--text-secondary)]">{tool.status || 'completed'}</span>
                {tool.resource_uris.length > 0 && <span className="block truncate text-xs text-[var(--text-secondary)]">{tool.resource_uris.join(', ')}</span>}
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
