import { useEffect, useRef, useState, type ChangeEvent, type FormEvent, type KeyboardEvent } from 'react';
import { Loader2, Paperclip, Send, X } from 'lucide-react';
import { appendAssistantAttachment, readAssistantAttachmentText } from './attachments.js';

const assistantComposerMaxHeight = 160;

export function AssistantComposer({
  compact,
  draft,
  disabled,
  sending,
  pageContextLabel,
  footnote,
  onDraftChange,
  onRemovePageContext,
  onSubmit,
}: {
  compact: boolean;
  draft: string;
  disabled: boolean;
  sending: boolean;
  pageContextLabel: string;
  footnote: string;
  onDraftChange: (draft: string) => void;
  onRemovePageContext: () => void;
  onSubmit: () => void;
}) {
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [attachError, setAttachError] = useState('');

  // The box grows with the draft up to a cap, then scrolls, so a long prompt
  // never pushes the transcript off screen.
  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.style.height = 'auto';
    textarea.style.height = `${Math.min(textarea.scrollHeight, assistantComposerMaxHeight)}px`;
  }, [draft]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSubmit();
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      onSubmit();
    }
  };

  const handleAttach = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (fileInputRef.current) fileInputRef.current.value = '';
    if (!file) return;
    setAttachError('');
    try {
      const text = await readAssistantAttachmentText(file);
      onDraftChange(appendAssistantAttachment(draft, file.name, text));
      textareaRef.current?.focus();
    } catch (error) {
      setAttachError(error instanceof Error ? error.message : 'Unable to read that file');
    }
  };

  return (
    <div
      className={`pointer-events-none absolute bottom-0 left-0 w-full pt-10 ${compact ? 'px-3 pb-4' : 'px-4 pb-5 md:px-8'}`}
      style={{ background: 'linear-gradient(to top, var(--bg-primary) 55%, transparent)' }}
    >
      <form onSubmit={submit} className="pointer-events-auto mx-auto w-full max-w-3xl">
        <div className="rounded-2xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-2 shadow-lg transition focus-within:border-[var(--border-accent)] focus-within:ring-2 focus-within:ring-[var(--border-accent-focus-ring)]">
          {pageContextLabel && (
            <div className="mb-2 flex min-h-7 items-center gap-2 rounded-lg bg-[var(--bg-tertiary)] px-2 text-xs text-[var(--text-secondary)]">
              <span className="font-medium text-[var(--text-primary)]">Context</span>
              <span className="min-w-0 truncate">{pageContextLabel}</span>
              <button
                type="button"
                className="ml-auto inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-[var(--text-secondary)] hover:bg-[var(--bg-primary)] hover:text-[var(--text-primary)]"
                onClick={onRemovePageContext}
                aria-label="Remove page context"
                title="Remove context"
              >
                <X className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            </div>
          )}
          <div className="flex items-end gap-2">
            <label
              className={`mb-0.5 inline-flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-xl text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] ${disabled ? 'pointer-events-none opacity-50' : ''}`}
              title="Attach a text file"
            >
              <Paperclip className="h-4 w-4" aria-hidden="true" />
              <input
                ref={fileInputRef}
                type="file"
                className="sr-only"
                aria-label="Attach a text file"
                accept=".txt,.log,.md,.json,.yaml,.yml,.csv,.tf,.sh,.env,.ini,.conf,text/*"
                disabled={disabled}
                onChange={event => void handleAttach(event)}
              />
            </label>
            <textarea
              ref={textareaRef}
              rows={1}
              className="max-h-40 w-full resize-none overflow-y-auto bg-transparent py-2.5 text-[15px] leading-normal text-[var(--text-primary)] outline-none placeholder:text-[var(--text-placeholder)]"
              value={draft}
              onChange={event => onDraftChange(event.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Message NopsAI..."
              disabled={disabled}
            />
            <button
              type="submit"
              className="mb-0.5 inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-[var(--border-accent)] text-white shadow-sm transition hover:brightness-95 disabled:cursor-not-allowed disabled:opacity-50"
              disabled={disabled || sending || !draft.trim()}
              aria-label="Send message"
              title="Send message"
            >
              {sending
                ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                : <Send className="h-4 w-4" aria-hidden="true" />}
            </button>
          </div>
        </div>
        {attachError && <p className="mt-2 text-center text-[11px] text-rose-600 dark:text-rose-400">{attachError}</p>}
        <p className="mt-2 text-center text-[11px] text-[var(--text-secondary)]">
          {footnote}
        </p>
      </form>
    </div>
  );
}
