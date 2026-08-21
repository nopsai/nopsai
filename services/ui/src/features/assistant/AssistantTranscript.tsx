import { useState } from 'react';
import { AlertTriangle, Check, Copy, Loader2, RotateCcw } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon.js';
import type { CurrentUser } from '../../app/types.js';
import { currentUserInitials } from '../../app/userIdentity.js';
import { copyTextToClipboard } from '../../lib/clipboard.js';
import {
  assistantExecutionPlanFromMessage,
  assistantMessageUsageLabel,
  type AssistantExecutionPlan,
  type AssistantExecutionPlanStep,
  type AssistantMessage,
} from './model.js';
import {
  assistantFailureDetailBody,
  assistantMessageFailure,
  assistantMessageProse,
  type AssistantFailure,
} from './failures.js';
import { assistantProgressElapsedLabel, assistantPromptStarters, type AssistantProgressStep } from './experience.js';
import { AssistantRichContent } from './rendering.js';

export function AssistantWelcome({ compact, disabled, onStarter }: {
  compact: boolean;
  disabled: boolean;
  onStarter: (starter: string) => void;
}) {
  return (
    <div className="flex flex-1 flex-col justify-center py-6">
      <div className="space-y-3">
        <p className={`font-semibold text-[var(--text-primary)] ${compact ? 'text-xl' : 'text-2xl'}`}>Hi, I&apos;m NopsAI. What are we solving today?</p>
        <p className="max-w-2xl text-[15px] leading-relaxed text-[var(--text-secondary)]">
          You can ask for investigation, GitOps-safe proposals, health checks, or rollout planning. You&apos;ll review changes before anything is applied.
        </p>
      </div>
      <div className={compact ? 'mt-5 grid gap-2' : 'mt-6 grid grid-cols-2 gap-3'}>
        {assistantPromptStarters.map(starter => (
          <button
            key={starter}
            type="button"
            className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 text-left text-sm font-medium text-[var(--text-primary)] shadow-sm transition hover:border-[var(--border-secondary)] hover:bg-[var(--bg-tertiary)] disabled:cursor-not-allowed disabled:opacity-60"
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

export function AssistantMessageRow({
  message,
  currentUser,
  copied,
  actionsDisabled,
  onCopy,
  onRetry,
}: {
  message: AssistantMessage;
  currentUser?: CurrentUser | null;
  copied: boolean;
  actionsDisabled: boolean;
  onCopy: () => void;
  onRetry?: () => void;
}) {
  const isUser = message.role === 'user';
  const failure = assistantMessageFailure(message);
  const executionPlan = isUser ? null : assistantExecutionPlanFromMessage(message);
  const prose = assistantMessageProse(message.content, failure);
  const meta = [assistantMessageUsageLabel(message), assistantMessageTimeLabel(message)].filter(Boolean).join(' · ');

  return (
    <article className={`group flex w-full gap-3 md:gap-4 ${isUser ? 'justify-end' : 'justify-start'}`}>
      {!isUser && <AssistantAvatar />}
      <div className={`flex min-w-0 max-w-[85%] flex-col md:max-w-[75%] ${isUser ? 'items-end' : 'items-start'}`}>
        <div className={`mb-1 flex items-center gap-2 ${isUser ? 'flex-row-reverse' : ''}`}>
          <span className="text-sm font-medium text-[var(--text-primary)]">{isUser ? 'You' : 'NopsAI'}</span>
          {meta && (
            <span className="text-[11px] text-[var(--text-secondary)] opacity-0 transition-opacity group-hover:opacity-100">
              {meta}
            </span>
          )}
          <span className="flex items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
            {isUser && onRetry && (
              <button
                type="button"
                className="inline-flex h-6 w-6 items-center justify-center rounded text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] disabled:cursor-not-allowed disabled:opacity-40"
                onClick={onRetry}
                disabled={actionsDisabled}
                aria-label="Retry this prompt"
                title="Retry"
              >
                <RotateCcw className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            )}
            <button
              type="button"
              className="inline-flex h-6 w-6 items-center justify-center rounded text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
              onClick={onCopy}
              aria-label="Copy message"
              title="Copy"
            >
              {copied ? <Check className="h-3.5 w-3.5" aria-hidden="true" /> : <Copy className="h-3.5 w-3.5" aria-hidden="true" />}
            </button>
          </span>
        </div>

        {isUser ? (
          <div className="whitespace-pre-wrap break-words rounded-2xl rounded-tr-sm bg-[var(--bg-tertiary)] px-4 py-3 text-[15px] leading-relaxed text-[var(--text-primary)] shadow-sm">
            {message.content}
          </div>
        ) : (
          <div className="w-full min-w-0">
            {executionPlan && <AssistantExecutionPlanBlock plan={executionPlan} />}
            {prose && (
              <div className="px-1 text-[15px] leading-relaxed text-[var(--text-primary)]">
                <AssistantRichContent content={prose} />
              </div>
            )}
            {failure && <AssistantFailureCard failure={failure} onRetry={onRetry} retryDisabled={actionsDisabled} />}
          </div>
        )}
      </div>
      {isUser && <AssistantUserAvatar currentUser={currentUser} />}
    </article>
  );
}

export function AssistantFailureCard({
  failure,
  onRetry,
  retryDisabled,
}: {
  failure: AssistantFailure;
  onRetry?: () => void;
  retryDisabled: boolean;
}) {
  const [copied, setCopied] = useState(false);
  const body = assistantFailureDetailBody(failure.detail);

  const copyDetail = () => {
    void copyTextToClipboard(body)
      .then(() => setCopied(true))
      .catch(() => setCopied(false));
  };

  return (
    <div className="mt-3 w-full rounded-xl border border-rose-200 bg-rose-50/70 p-4 shadow-sm dark:border-rose-900/60 dark:bg-rose-950/30">
      <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-rose-700 dark:text-rose-300">
        <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
        {failure.title}
      </div>
      <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-rose-200 bg-[var(--bg-primary)] p-3 font-mono text-[13px] leading-relaxed text-[var(--text-primary)] dark:border-rose-900/60">
        {body}
      </pre>
      <div className="mt-3 flex flex-wrap gap-2">
        {onRetry && (
          <button
            type="button"
            className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-1.5 text-xs font-medium text-[var(--text-primary)] shadow-sm transition hover:bg-[var(--bg-tertiary)] disabled:cursor-not-allowed disabled:opacity-60"
            onClick={onRetry}
            disabled={retryDisabled}
          >
            Retry request
          </button>
        )}
        <button
          type="button"
          className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-1.5 text-xs font-medium text-[var(--text-primary)] shadow-sm transition hover:bg-[var(--bg-tertiary)]"
          onClick={copyDetail}
        >
          {copied ? 'Error copied' : 'Copy error'}
        </button>
      </div>
    </div>
  );
}

export function AssistantThinkingRow({ steps, elapsedMs }: { steps: AssistantProgressStep[]; elapsedMs: number }) {
  const visibleSteps = steps.length > 0 ? steps : [{ label: 'Preparing a bounded answer', state: 'active' as const }];
  return (
    <article className="flex w-full gap-3 md:gap-4">
      <AssistantAvatar />
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex items-center gap-2">
          <span className="text-sm font-medium text-[var(--text-primary)]">NopsAI</span>
          <span className="text-[11px] text-[var(--text-secondary)]">{assistantProgressElapsedLabel(elapsedMs)}</span>
        </div>
        <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3 shadow-sm">
          <div className="flex items-center gap-2 text-sm text-[var(--text-primary)]">
            <Loader2 className="h-4 w-4 shrink-0 animate-spin" aria-hidden="true" />
            <span>Working through the request</span>
          </div>
          <ol className="mt-2 space-y-1 text-xs leading-relaxed">
            {visibleSteps.map((step, index) => (
              <li key={`${step.label}-${index}`} className="flex items-start gap-2">
                <span className="mt-0.5 inline-flex h-4 w-4 shrink-0 items-center justify-center">
                  {step.state === 'done' ? (
                    <Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400" aria-hidden="true" />
                  ) : step.state === 'active' ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--text-accent)]" aria-hidden="true" />
                  ) : (
                    <span className="h-1.5 w-1.5 rounded-full bg-[var(--border-secondary)]" aria-hidden="true" />
                  )}
                </span>
                <span className={step.state === 'pending' ? 'text-[var(--text-secondary)] opacity-80' : 'text-[var(--text-secondary)]'}>
                  {step.label}
                </span>
              </li>
            ))}
          </ol>
          {elapsedMs > 30000 && (
            <p className="mt-2 max-w-md text-xs leading-relaxed text-[var(--text-secondary)]">
              Still waiting on the selected model. Saved results will appear as soon as the turn finishes.
            </p>
          )}
        </div>
      </div>
    </article>
  );
}

export function AssistantTranscriptDivider({ label }: { label: string }) {
  return (
    <div className="flex justify-center">
      <span className="rounded-full border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-1 text-[11px] font-medium text-[var(--text-secondary)]">
        {label}
      </span>
    </div>
  );
}

function AssistantAvatar() {
  return (
    <span className="mt-6 inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--text-primary)] text-[var(--bg-primary)] shadow-sm">
      <ObjectIcon type="assistant" className="h-4 w-4" strokeWidth={2.4} />
    </span>
  );
}

function AssistantUserAvatar({ currentUser }: { currentUser?: CurrentUser | null }) {
  return (
    <span className="mt-6 inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-[var(--border-primary)] bg-[var(--bg-tertiary)] text-xs font-medium text-[var(--text-secondary)]">
      {currentUserInitials(currentUser)}
    </span>
  );
}

function AssistantExecutionPlanBlock({ plan }: { plan: AssistantExecutionPlan }) {
  const visibleSteps = plan.steps.slice(0, 6);
  return (
    <div className="mb-3 border-l-2 border-[var(--border-accent)] pl-3 text-xs text-[var(--text-secondary)]">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-semibold uppercase tracking-wide">Execution plan</span>
        {plan.requires_confirmation && (
          <span className="rounded bg-[var(--bg-tertiary)] px-2 py-0.5 text-[11px] font-medium text-[var(--text-primary)]">Needs confirmation</span>
        )}
      </div>
      {plan.summary && <p className="mt-1 leading-relaxed">{plan.summary}</p>}
      {visibleSteps.length > 0 && (
        <ol className="mt-2 space-y-1.5">
          {visibleSteps.map((step, index) => (
            <AssistantExecutionPlanRow key={`${step.index || index}-${step.title}-${step.source}`} step={step} fallbackIndex={index + 1} />
          ))}
        </ol>
      )}
    </div>
  );
}

function AssistantExecutionPlanRow({ step, fallbackIndex }: { step: AssistantExecutionPlanStep; fallbackIndex: number }) {
  const index = step.index || fallbackIndex;
  const title = step.title || step.reason || step.tool || 'Assistant step';
  return (
    <li className="grid grid-cols-[1.5rem_minmax(0,1fr)] gap-2 leading-relaxed">
      <span className="mt-0.5 inline-flex h-5 w-5 items-center justify-center rounded bg-[var(--bg-tertiary)] text-[11px] font-semibold text-[var(--text-secondary)]">{index}</span>
      <span className="min-w-0">
        <span className="block text-[var(--text-primary)]">{title}</span>
        <span className="mt-0.5 flex flex-wrap gap-1.5">
          <AssistantExecutionPlanBadge value={assistantExecutionPlanSourceLabel(step.source)} />
          <AssistantExecutionPlanBadge value={step.phase} />
          <AssistantExecutionPlanBadge value={step.confidence ? `${step.confidence} confidence` : ''} />
        </span>
      </span>
    </li>
  );
}

function AssistantExecutionPlanBadge({ value }: { value: string }) {
  if (!value) return null;
  return <span className="rounded bg-[var(--bg-tertiary)] px-1.5 py-0.5 text-[11px] text-[var(--text-secondary)]">{value}</span>;
}

function assistantExecutionPlanSourceLabel(value: string): string {
  const source = value.trim().toLowerCase();
  if (source === 'mcp') return 'MCP';
  if (source === 'llm') return 'LLM';
  if (source === 'docs') return 'Docs';
  if (source === 'knowledge_context') return 'Knowledge context';
  if (source === 'gitops_proposal') return 'GitOps proposal';
  return source.replace(/_/g, ' ');
}

function assistantMessageTimeLabel(message: AssistantMessage): string {
  const timestamp = Date.parse(message.created_at);
  if (!Number.isFinite(timestamp)) return '';
  return new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}
