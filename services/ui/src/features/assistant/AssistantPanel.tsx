import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react';
import { Check, Copy, Loader2, Maximize2, Plus, RotateCcw, Send, Trash2, X } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon.js';
import {
  assistantConversationUsageLabel,
  assistantMessageAuthorLabel,
  assistantMessageUsageLabel,
  assistantVisibleToolActivity,
  proposedChangesFromMessages,
  type AssistantConfig,
  type AssistantConversation,
  type AssistantMessage,
  type AssistantToolActivity,
} from './model.js';
import { assistantProgressLabel, assistantPromptStarters, assistantReadyLine } from './experience.js';
import { AssistantRichContent } from './rendering.js';
import { useComposerResize } from './useComposerResize.js';
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
  const [detailsOpen, setDetailsOpen] = useState(true);
  const transcriptRef = useRef<HTMLDivElement>(null);
  const composerResize = useComposerResize();
  const layoutClass = compact
    ? 'grid min-h-0 flex-1 grid-rows-[1fr_auto]'
    : detailsOpen
      ? 'grid min-h-0 flex-1 grid-cols-[250px_minmax(0,1fr)_310px]'
      : 'grid min-h-0 flex-1 grid-cols-[250px_minmax(0,1fr)]';

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
    <section className={compact ? 'flex h-full flex-col bg-[var(--bg-primary)]' : 'flex h-full min-h-0 flex-col bg-[var(--bg-primary)]'}>
      <header className="flex items-center justify-between gap-3 border-b border-[var(--border-primary)] px-4 py-3">
        <div className="flex min-w-0 items-center gap-3">
          <span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--bg-tertiary)] text-[var(--text-primary)]">
            <ObjectIcon type="assistant" className="h-4 w-4" />
          </span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-baseline gap-x-2">
              <h2 className="truncate text-base font-semibold text-[var(--text-primary)]">NopsAI</h2>
              <span className="text-xs text-[var(--text-secondary)]">Your operations copilot</span>
            </div>
            <p className="truncate text-xs text-[var(--text-secondary)]">{assistantReadyLine(assistant.config)}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {!compact && (
            <button
              type="button"
              className="inline-flex h-8 items-center justify-center rounded-md border border-[var(--border-primary)] px-3 text-xs font-medium text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
              onClick={() => setDetailsOpen(current => !current)}
              aria-label={detailsOpen ? 'Hide details' : 'Show details'}
              aria-pressed={detailsOpen}
              title={detailsOpen ? 'Hide details' : 'Show details'}
            >
              {detailsOpen ? 'Hide details' : 'Details'}
            </button>
          )}
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

      <div className={layoutClass}>
        {!compact && (
          <AssistantSidebar
            conversations={assistant.conversations}
            activeConversation={assistant.activeConversation}
            enabled={assistant.enabled}
            loading={assistant.loading}
            sending={assistant.sending}
            deletingConversationID={assistant.deletingConversationID}
            onStartConversation={() => void assistant.startConversation()}
            onSelectConversation={id => void assistant.selectConversation(id)}
            onDeleteConversation={id => void assistant.deleteConversation(id)}
          />
        )}

        <main className="flex min-h-0 flex-col">
          {assistant.error && (
            <div className="border-b border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">
              {assistant.error}
            </div>
          )}
          <AssistantStatusBar
            compact={compact}
            config={assistant.config}
            activeConversation={assistant.activeConversation}
            selectedProfile={assistant.selectedProfile}
            profileOptions={assistant.profileOptions}
            enabled={assistant.enabled}
            loading={assistant.loading}
            onSelectProfile={assistant.setSelectedProfile}
          />

          <div ref={transcriptRef} className="min-h-0 flex-1 space-y-4 overflow-auto px-4 py-5" aria-live="polite">
            {assistant.config && !assistant.enabled ? (
              <div className="rounded-lg border border-dashed border-[var(--border-primary)] p-4 text-sm text-[var(--text-secondary)]">
                The assistant is disabled by administrator configuration.
              </div>
            ) : assistant.loading ? (
              <p className="text-sm text-[var(--text-secondary)]">Loading assistant...</p>
            ) : assistant.activeMessages.length === 0 ? (
              <AssistantWelcome compact={compact} disabled={!assistant.enabled || assistant.loading} onStarter={assistant.setDraft} />
            ) : (
              <>
                {assistant.activeMessages.map(message => (
                  <AssistantMessageBubble
                    key={message.id}
                    message={message}
                    copied={assistant.copiedMessageID === message.id}
                    retryDisabled={!assistant.enabled || assistant.sending || assistant.loading}
                    onCopy={() => void assistant.copyMessage(message)}
                    onRetry={message.role === 'user' ? () => void assistant.retryMessage(message) : undefined}
                  />
                ))}
                {assistant.sending && (
                  <AssistantThinkingBubble label={assistantProgressLabel(assistant.activeMessages, assistant.activeConversation)} />
                )}
              </>
            )}
          </div>

          <form onSubmit={submit} className="border-t border-[var(--border-primary)] p-4">
            <div
              role="separator"
              aria-label="Resize message composer"
              aria-orientation="horizontal"
              aria-valuemin={composerResize.minHeight}
              aria-valuemax={composerResize.maxHeight}
              aria-valuenow={composerResize.composerHeight}
              tabIndex={0}
              className="-mx-4 -mt-4 mb-3 flex h-4 cursor-row-resize items-center justify-center outline-none focus-visible:ring-2 focus-visible:ring-[var(--border-accent)]"
              onKeyDown={composerResize.resizeComposerWithKeyboard}
              onPointerDown={composerResize.startComposerResize}
              title="Drag to resize the message composer"
            >
              <span className="h-px w-full bg-[var(--border-primary)] transition-colors hover:bg-[var(--border-accent)]" />
            </div>
            <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 shadow-sm focus-within:border-[var(--border-accent)]">
              <textarea
                className="w-full resize-none overflow-auto bg-transparent text-sm leading-relaxed text-[var(--text-primary)] outline-none placeholder:text-[var(--text-secondary)]"
                style={{ height: composerResize.composerHeight }}
                value={assistant.draft}
                onChange={event => assistant.setDraft(event.target.value)}
                onKeyDown={handleDraftKeyDown}
                placeholder="Describe what you are trying to achieve..."
                disabled={!assistant.enabled || assistant.loading}
              />
              <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
                <span className="text-xs text-[var(--text-secondary)]">Current-user AAA · ⌘ / Ctrl + Enter</span>
                <button
                  type="submit"
                  className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-[var(--border-accent)] px-4 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-60"
                  disabled={!assistant.enabled || assistant.loading || !assistant.draft.trim() || assistant.sending}
                >
                  {assistant.sending ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" /> : <Send className="h-4 w-4" aria-hidden="true" />}
                  Send
                </button>
              </div>
            </div>
          </form>
        </main>

        {!compact && detailsOpen && (
          <aside className="min-h-0 overflow-auto border-l border-[var(--border-primary)] p-4" aria-label="Assistant details">
            <AssistantContextPanel conversation={assistant.activeConversation} messages={assistant.activeMessages} />
          </aside>
        )}
      </div>
    </section>
  );
}

function AssistantSidebar({
  conversations,
  activeConversation,
  enabled,
  loading,
  sending,
  deletingConversationID,
  onStartConversation,
  onSelectConversation,
  onDeleteConversation,
}: {
  conversations: AssistantConversation[];
  activeConversation: AssistantConversation | null;
  enabled: boolean;
  loading: boolean;
  sending: boolean;
  deletingConversationID: string;
  onStartConversation: () => void;
  onSelectConversation: (conversationID: string) => void;
  onDeleteConversation: (conversationID: string) => void;
}) {
  return (
    <aside className="min-h-0 overflow-auto border-r border-[var(--border-primary)] p-4" aria-label="Assistant conversations">
      <div className="space-y-3">
        <button
          type="button"
          className="inline-flex w-full items-center justify-center gap-2 rounded-md bg-[var(--border-accent)] px-3 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-60"
          onClick={onStartConversation}
          disabled={!enabled || loading || sending}
        >
          <Plus className="h-4 w-4" aria-hidden="true" />
          New conversation
        </button>
      </div>

      <div className="mt-5 space-y-2">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Conversations</h2>
        {conversations.length === 0 && (
          <p className="rounded-md border border-dashed border-[var(--border-primary)] px-3 py-2 text-sm text-[var(--text-secondary)]">No conversations yet.</p>
        )}
        {conversations.map(conversation => (
          <div
            key={conversation.id}
            className={`group flex items-center gap-2 rounded-md border px-2 py-2 text-sm hover:bg-[var(--bg-tertiary)] ${activeConversation?.id === conversation.id ? 'border-[var(--border-accent)] bg-[var(--bg-tertiary)]' : 'border-[var(--border-primary)]'}`}
          >
            <button
              type="button"
              className="min-w-0 flex-1 text-left"
              onClick={() => onSelectConversation(conversation.id)}
            >
              <span className="block truncate font-medium text-[var(--text-primary)]">{conversation.title || 'Untitled conversation'}</span>
            </button>
            <button
              type="button"
              className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-[var(--text-secondary)] opacity-0 hover:bg-red-50 hover:text-red-600 focus:opacity-100 group-hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-red-950/30"
              onClick={() => onDeleteConversation(conversation.id)}
              disabled={sending || deletingConversationID === conversation.id}
              aria-label={`Delete conversation ${conversation.title || conversation.id}`}
              title="Delete"
            >
              {deletingConversationID === conversation.id ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" /> : <Trash2 className="h-4 w-4" aria-hidden="true" />}
            </button>
          </div>
        ))}
      </div>
    </aside>
  );
}

function AssistantStatusBar({
  compact,
  config,
  activeConversation,
  selectedProfile,
  profileOptions,
  enabled,
  loading,
  onSelectProfile,
}: {
  compact: boolean;
  config: AssistantConfig | null;
  activeConversation: AssistantConversation | null;
  selectedProfile: string;
  profileOptions: string[];
  enabled: boolean;
  loading: boolean;
  onSelectProfile: (profile: string) => void;
}) {
  return (
    <div className="border-b border-[var(--border-primary)] px-4 py-2 text-xs text-[var(--text-secondary)]">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="inline-flex h-2 w-2 rounded-full bg-emerald-500" aria-hidden="true" />
          <span>{assistantReadyLine(config)}</span>
          {activeConversation && <span className="hidden sm:inline">· {assistantConversationUsageLabel(activeConversation)}</span>}
        </div>
      </div>
      <details className="mt-2">
        <summary className="cursor-pointer select-none text-xs font-medium text-[var(--text-secondary)]">Session details</summary>
        <div className={compact ? 'mt-2 grid gap-2' : 'mt-2 grid grid-cols-2 gap-2'}>
          <SessionDetail label="MCP" value={config?.mcp.enabled ? 'Enabled' : 'Disabled'} />
          <SessionDetail label="Memory" value={config?.memory.enabled ? config.memory.scope || 'Enabled' : 'Disabled'} />
          <SessionDetail label="Confirmation" value={config?.actions.require_confirmation !== false ? 'Required' : 'Relaxed'} />
          <label className="text-xs text-[var(--text-secondary)]">
            LLM profile
            <select
              className="mt-1 w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-2 py-1.5 text-xs text-[var(--text-primary)]"
              value={selectedProfile}
              onChange={event => onSelectProfile(event.target.value)}
              disabled={!enabled || loading || profileOptions.length === 0}
            >
              {profileOptions.length === 0 ? (
                <option value="">No profiles</option>
              ) : (
                profileOptions.map(profile => <option key={profile} value={profile}>{profile}</option>)
              )}
            </select>
          </label>
        </div>
      </details>
    </div>
  );
}

function SessionDetail({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md bg-[var(--bg-tertiary)] px-2 py-1.5">
      <span className="block text-[10px] uppercase tracking-wide text-[var(--text-secondary)]">{label}</span>
      <span className="block truncate text-xs font-medium text-[var(--text-primary)]">{value}</span>
    </div>
  );
}

function AssistantWelcome({
  compact,
  disabled,
  onStarter,
}: {
  compact: boolean;
  disabled: boolean;
  onStarter: (starter: string) => void;
}) {
  return (
    <div className="mx-auto flex min-h-full w-full max-w-3xl flex-col justify-center py-6">
      <div className="space-y-3">
        <p className="text-2xl font-semibold tracking-normal text-[var(--text-primary)]">Hi, I&apos;m NopsAI. What are we solving today?</p>
        <p className="max-w-2xl text-sm leading-relaxed text-[var(--text-secondary)]">
          You can ask for investigation, GitOps-safe proposals, health checks, or rollout planning. You&apos;ll review changes before anything is applied.
        </p>
      </div>
      <div className={compact ? 'mt-5 grid gap-2' : 'mt-6 grid grid-cols-2 gap-3'}>
        {assistantPromptStarters.map(starter => (
          <button
            key={starter}
            type="button"
            className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-left text-sm font-medium text-[var(--text-primary)] transition hover:border-[var(--border-accent)] hover:bg-[var(--bg-tertiary)] disabled:cursor-not-allowed disabled:opacity-60"
            onClick={() => onStarter(starter)}
            disabled={disabled}
          >
            {starter}
          </button>
        ))}
      </div>
    </div>
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
  const usageLabel = assistantMessageUsageLabel(message);
  return (
    <article className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div className={`max-w-[820px] rounded-lg border px-4 py-3 text-sm shadow-sm ${isUser ? 'border-[var(--border-accent)] bg-[var(--border-accent)] text-white' : 'border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-primary)]'}`}>
        <div className="mb-2 flex items-center justify-between gap-3">
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
        <AssistantRichContent content={message.content} inverted={isUser} />
        {usageLabel && <p className={`mt-2 text-[11px] ${isUser ? 'text-white/70' : 'text-[var(--text-secondary)]'}`}>{usageLabel}</p>}
      </div>
    </article>
  );
}

function AssistantThinkingBubble({ label }: { label: string }) {
  return (
    <article className="flex justify-start">
      <div className="inline-flex items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-secondary)] shadow-sm">
        <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
        <span>{label}</span>
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
    <div className="space-y-4">
      <div>
        <h2 className="text-sm font-semibold text-[var(--text-primary)]">Details</h2>
        <p className="mt-1 text-xs leading-relaxed text-[var(--text-secondary)]">Evidence, memory, and proposed changes stay available without crowding the conversation.</p>
      </div>

      <details open className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3">
        <summary className="cursor-pointer select-none text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Usage</summary>
        <p className="mt-3 text-sm text-[var(--text-primary)]">{assistantConversationUsageLabel(conversation)}</p>
        {conversation?.usage && (
          <dl className="mt-3 space-y-2 text-sm">
            <ContextRow label="Visible text" value={`${conversation.usage.content_tokens} tokens est.`} />
            <ContextRow label="Provider input" value={`${conversation.usage.prompt_tokens} tokens`} />
            <ContextRow label="Provider output" value={`${conversation.usage.completion_tokens} tokens`} />
            <ContextRow label="LLM calls" value={`${conversation.usage.llm_calls}`} />
          </dl>
        )}
      </details>

      <details className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3">
        <summary className="cursor-pointer select-none text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">Memory</summary>
        <dl className="mt-3 space-y-2 text-sm">
          <ContextRow label="Docs" value={memory?.selected_docs_version || conversation?.docs_version || 'auto'} />
          <ContextRow label="Run" value={memory?.selected_run || 'None'} />
          <ContextRow label="Pipeline" value={memory?.selected_pipeline || 'None'} />
          <ContextRow label="Scope" value={memory?.selected_scope || conversation?.scope || 'None'} />
        </dl>
        {memory?.summary && <p className="mt-3 rounded-md bg-[var(--bg-tertiary)] p-3 text-sm text-[var(--text-secondary)]">{memory.summary}</p>}
      </details>

      <details open={toolCalls.length > 0} className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3">
        <summary className="cursor-pointer select-none text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
          NopsAI evidence {toolCalls.length > 0 ? `(${toolCalls.length})` : ''}
        </summary>
        {toolCalls.length === 0 ? (
          <p className="mt-3 rounded-md border border-dashed border-[var(--border-primary)] p-3 text-sm text-[var(--text-secondary)]">No evidence tools used yet.</p>
        ) : (
          <ul className="mt-3 space-y-2">
            {toolCalls.map((tool, index) => (
              <EvidenceToolCard key={`${tool.name}-${index}`} tool={tool} />
            ))}
          </ul>
        )}
      </details>

      <details open={proposedChanges.length > 0} className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3">
        <summary className="cursor-pointer select-none text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
          Proposed changes {proposedChanges.length > 0 ? `(${proposedChanges.length})` : ''}
        </summary>
        {proposedChanges.length === 0 ? (
          <p className="mt-3 rounded-md border border-dashed border-[var(--border-primary)] p-3 text-sm text-[var(--text-secondary)]">
            Draft changes will appear here for review before any apply flow.
          </p>
        ) : (
          <div className="mt-3 space-y-3">
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
      </details>
    </div>
  );
}

function EvidenceToolCard({ tool }: { tool: AssistantToolActivity }) {
  return (
    <li className="rounded-md border border-[var(--border-primary)] p-2 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate font-medium text-[var(--text-primary)]">{tool.name}</span>
        <span className="rounded bg-[var(--bg-tertiary)] px-2 py-0.5 text-[11px] text-[var(--text-secondary)]">{tool.status || 'completed'}</span>
      </div>
      {tool.resource_uris.length > 0 && <p className="mt-1 truncate text-xs text-[var(--text-secondary)]">{tool.resource_uris.join(', ')}</p>}
      <details className="mt-2">
        <summary className="cursor-pointer select-none text-xs font-medium text-[var(--text-secondary)]">View details</summary>
        <pre className="mt-2 max-h-72 overflow-auto rounded-md bg-[var(--bg-tertiary)] p-2 text-xs text-[var(--text-primary)]">
          {prettyPrintEvidence(tool)}
        </pre>
      </details>
    </li>
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

function prettyPrintEvidence(tool: AssistantToolActivity): string {
  try {
    return JSON.stringify({
      input: tool.input,
      output: tool.output,
      resources: tool.resource_uris,
    }, null, 2);
  } catch {
    return 'Evidence details are unavailable.';
  }
}
