import { useEffect, useRef, useState } from 'react';
import { CircleDollarSign, Maximize2, X } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon.js';
import type { CurrentUser } from '../../app/types.js';
import {
  assistantConversationSpendLabel,
  assistantConversationUsageLabel,
  assistantLastUserMessage,
  type AssistantConversation,
} from './model.js';
import { assistantProgressSteps, assistantReadyLine } from './experience.js';
import { assistantSendFailure } from './failures.js';
import { AssistantComposer } from './AssistantComposer.js';
import { AssistantConversationRail } from './AssistantConversationRail.js';
import {
  AssistantFailureCard,
  AssistantMessageRow,
  AssistantThinkingRow,
  AssistantTranscriptDivider,
  AssistantWelcome,
} from './AssistantTranscript.js';
import { useAssistantController } from './useAssistantController.js';
import { assistantPageContextKey, assistantPageContextLabel, type AssistantPageContext } from './pageContext.js';
import { assistantPageContextFromOption } from './contextOptions.js';

export function AssistantPanel({
  variant = 'page',
  startFresh = false,
  pageContext = null,
  currentUser = null,
  initialDraft = '',
  onExpand,
  onClose,
}: {
  variant?: 'page' | 'dock';
  startFresh?: boolean;
  pageContext?: Partial<AssistantPageContext> | null;
  currentUser?: CurrentUser | null;
  /** Prefills the composer once, for hand-offs such as "Ask NopsAI" from an analysis. */
  initialDraft?: string;
  onExpand?: () => void;
  onClose?: () => void;
}) {
  const [removedPageContextKey, setRemovedPageContextKey] = useState('');
  // A context picked in the composer outranks the route: the assistant page has
  // no resource of its own, and on a resource page an explicit pick is the user
  // saying they mean something else.
  const [pickedPageContext, setPickedPageContext] = useState<AssistantPageContext | null>(null);
  const routeContextKey = assistantPageContextKey(pageContext);
  const routeContextRemoved = routeContextKey !== '' && removedPageContextKey === routeContextKey;
  const activePageContext = pickedPageContext || (routeContextRemoved ? null : pageContext);
  const assistant = useAssistantController({ startFresh, pageContext: activePageContext });
  const compact = variant === 'dock';
  const pageContextLabel = assistantPageContextLabel(activePageContext);
  const [progressNow, setProgressNow] = useState(() => Date.now());
  const transcriptRef = useRef<HTMLDivElement>(null);

  const draftSeeded = useRef(false);
  useEffect(() => {
    if (draftSeeded.current || !initialDraft) return;
    draftSeeded.current = true;
    assistant.setDraft(initialDraft);
  }, [assistant, initialDraft]);

  useEffect(() => {
    const transcript = transcriptRef.current;
    if (!transcript) return;
    transcript.scrollTop = transcript.scrollHeight;
  }, [assistant.activeConversation?.id, assistant.activeMessages.length, assistant.sending, assistant.loading]);

  useEffect(() => {
    if (!assistant.activeConversationSending || assistant.activeConversationSendingStartedAt <= 0) return;
    const interval = window.setInterval(() => setProgressNow(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, [assistant.activeConversationSending, assistant.activeConversationSendingStartedAt]);

  const progressElapsedMs = assistant.activeConversationSendingStartedAt > 0
    ? Math.max(0, progressNow - assistant.activeConversationSendingStartedAt)
    : 0;
  const actionsDisabled = !assistant.enabled || assistant.sending || assistant.loading;

  return (
    <section className="flex h-full min-h-0 bg-[var(--bg-primary)]">
      {!compact && (
        <AssistantConversationRail
          conversations={assistant.conversations}
          activeConversation={assistant.activeConversation}
          enabled={assistant.enabled}
          loading={assistant.loading}
          sending={assistant.sending}
          sendingConversationID={assistant.sendingConversationID}
          deletingConversationID={assistant.deletingConversationID}
          onStartConversation={() => void assistant.startConversation()}
          onSelectConversation={id => void assistant.selectConversation(id)}
          onDeleteConversation={id => void assistant.deleteConversation(id)}
        />
      )}

      <main className="relative flex min-w-0 flex-1 flex-col">
        <AssistantTopBar
          compact={compact}
          activeConversation={assistant.activeConversation}
          selectedProfile={assistant.selectedProfile}
          profileOptions={assistant.profileOptions}
          enabled={assistant.enabled}
          loading={assistant.loading}
          onSelectProfile={assistant.setSelectedProfile}
          onExpand={onExpand}
          onClose={onClose}
        />

        <div ref={transcriptRef} className="min-h-0 flex-1 overflow-y-auto" aria-live="polite">
          <div className={`mx-auto flex min-h-full w-full max-w-3xl flex-col gap-8 pb-44 ${compact ? 'px-3 pt-6' : 'px-4 pt-8 md:px-8'}`}>
            {assistant.config && !assistant.enabled ? (
              <div className="rounded-xl border border-dashed border-[var(--border-primary)] p-4 text-sm text-[var(--text-secondary)]">
                The assistant is disabled by administrator configuration.
              </div>
            ) : assistant.loading ? (
              <p className="text-sm text-[var(--text-secondary)]">Loading assistant...</p>
            ) : (
              <>
                {assistant.activeMessages.length === 0 ? (
                  <AssistantWelcome
                    compact={compact}
                    disabled={!assistant.enabled || assistant.loading}
                    onStarter={assistant.setDraft}
                  />
                ) : (
                  assistant.activeMessages.map((message, index) => {
                    // A reply retries the prompt that produced it, which is what the
                    // failure card offers when a turn ends on a provider error.
                    const prompt = message.role === 'user'
                      ? message
                      : assistantLastUserMessage(assistant.activeMessages.slice(0, index));
                    return (
                      <AssistantMessageRow
                        key={message.id}
                        message={message}
                        currentUser={currentUser}
                        copied={assistant.copiedMessageID === message.id}
                        actionsDisabled={actionsDisabled}
                        onCopy={() => void assistant.copyMessage(message)}
                        onRetry={prompt ? () => void assistant.retryMessage(prompt) : undefined}
                        onSuggestion={suggestion => assistant.setDraft(suggestion.label)}
                      />
                    );
                  })
                )}
                {assistant.retrying && <AssistantTranscriptDivider label="Retrying request..." />}
                {assistant.activeConversationSending && (
                  <AssistantThinkingRow
                    elapsedMs={progressElapsedMs}
                    steps={assistantProgressSteps(assistant.activeMessages, assistant.activeConversation, progressElapsedMs)}
                  />
                )}
                {assistant.error && (
                  <AssistantFailureCard
                    failure={assistantSendFailure(assistant.error)}
                    onRetry={assistant.canRetry ? () => void assistant.retryLastUserMessage() : undefined}
                    retryDisabled={actionsDisabled}
                  />
                )}
              </>
            )}
          </div>
        </div>

        <AssistantComposer
          compact={compact}
          draft={assistant.draft}
          disabled={!assistant.enabled || assistant.loading}
          sending={assistant.activeConversationSending}
          pageContextLabel={pageContextLabel}
          footnote={`NopsAI can be inaccurate. ${assistantReadyLine(assistant.config)}`}
          onDraftChange={assistant.setDraft}
          onRemovePageContext={() => {
            setPickedPageContext(null);
            setRemovedPageContextKey(routeContextKey);
          }}
          onSelectPageContext={option => setPickedPageContext(assistantPageContextFromOption(option))}
          onSubmit={() => {
            if (!assistant.enabled) return;
            void assistant.submitMessage();
          }}
        />
      </main>
    </section>
  );
}

function AssistantTopBar({
  compact,
  activeConversation,
  selectedProfile,
  profileOptions,
  enabled,
  loading,
  onSelectProfile,
  onExpand,
  onClose,
}: {
  compact: boolean;
  activeConversation: AssistantConversation | null;
  selectedProfile: string;
  profileOptions: string[];
  enabled: boolean;
  loading: boolean;
  onSelectProfile: (profile: string) => void;
  onExpand?: () => void;
  onClose?: () => void;
}) {
  const spend = assistantConversationSpendLabel(activeConversation);
  return (
    <header className="sticky top-0 z-20 border-b border-[var(--border-primary)] bg-[var(--bg-primary)] px-4 py-2.5">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          {compact && (
            <span className="flex items-center gap-2 pr-1 text-sm font-semibold text-[var(--text-primary)]">
              <ObjectIcon type="assistant" className="h-4 w-4" />
              NopsAI
            </span>
          )}
          <label className="flex min-w-0 items-center gap-2 text-xs font-medium text-[var(--text-secondary)]">
            <span className={compact ? 'sr-only' : ''}>Model</span>
            <select
              className="min-w-0 max-w-[220px] truncate rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-2 py-1.5 text-sm font-medium text-[var(--text-primary)] outline-none transition hover:border-[var(--border-secondary)] focus:border-[var(--border-accent)] disabled:cursor-not-allowed disabled:opacity-60"
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

        <div className="flex items-center gap-2">
          <span
            className="inline-flex items-center gap-1.5 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-2.5 py-1.5 text-xs text-[var(--text-secondary)]"
            title={assistantConversationUsageLabel(activeConversation)}
          >
            <CircleDollarSign className="h-3.5 w-3.5" aria-hidden="true" />
            <span>
              Spend <span className="font-medium text-[var(--text-primary)]">{spend}</span>
            </span>
          </span>
          {onExpand && (
            <button
              type="button"
              className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-primary)] text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
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
              className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--border-primary)] text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
              onClick={onClose}
              aria-label="Close assistant"
              title="Close"
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          )}
        </div>
      </div>
    </header>
  );
}
