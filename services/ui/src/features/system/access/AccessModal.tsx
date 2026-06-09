import { useId, type ReactNode } from 'react';
import { Plus, X } from 'lucide-react';
import { useDialogFocus } from '../../../components/useDialogFocus';

export function AccessEditorEmptyState({
  sectionLabel,
  hint,
  actionLabel,
  onAction,
}: {
  sectionLabel: string;
  hint: string;
  actionLabel?: string;
  onAction?: () => void;
}) {
  return (
    <div className="access-editor-empty">
      <div className="access-editor-empty__meta">
        <span className="access-editor-empty__badge">{sectionLabel}</span>
        <p className="access-editor-empty__hint">{hint}</p>
      </div>
      {actionLabel && onAction ? (
        <div className="access-editor-empty__footer">
          <button type="button" className="glass-button-primary access-editor-empty__button" onClick={onAction}>
            <Plus className="h-4 w-4" strokeWidth={2} aria-hidden="true" />
            <span>{actionLabel}</span>
          </button>
        </div>
      ) : null}
    </div>
  );
}

export function AccessModal({
  kicker,
  title,
  subtitle,
  icon,
  onClose,
  children,
  variant = 'default',
}: {
  kicker: string;
  title: string;
  subtitle?: string;
  icon?: ReactNode;
  onClose: () => void;
  children: ReactNode;
  variant?: 'default' | 'minimal';
}) {
  const titleID = useId();
  const descriptionID = useId();
  const dialogRef = useDialogFocus(onClose);
  const minimal = variant === 'minimal';

  return (
    <div className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show">
      <div
        ref={dialogRef}
        className={`pipelines-modal-card access-modal-card max-w-xl w-full ${minimal ? 'access-modal-card--minimal' : ''}`}
        role={minimal ? 'alertdialog' : 'dialog'}
        aria-modal="true"
        aria-labelledby={titleID}
        aria-describedby={subtitle ? descriptionID : undefined}
        tabIndex={-1}
      >
        <header className={`pipelines-modal-header access-modal-header ${minimal ? 'access-modal-header--minimal' : ''}`}>
          <div className="access-modal-heading">
            {!minimal ? (
              <span className="access-modal-icon" aria-hidden="true">
                {icon ?? <Plus className="h-4 w-4" strokeWidth={2} />}
              </span>
            ) : null}
            <div className="min-w-0">
              {kicker ? (
                <p
                  className={`pipelines-modal-kicker ${minimal ? 'text-[11px] tracking-[0.12em] uppercase text-[var(--text-tertiary)]' : 'text-xs text-[var(--text-secondary)]'}`}
                >
                  {kicker}
                </p>
              ) : null}
              <h3 id={titleID} className="text-lg font-semibold text-[var(--text-primary)]">{title}</h3>
              {subtitle ? (
                <p id={descriptionID} className="text-xs mt-1 text-[var(--text-secondary)]">
                  {subtitle}
                </p>
              ) : null}
            </div>
          </div>
          <button type="button" className={minimal ? 'access-inline-btn' : 'glass-button-ghost'} onClick={onClose} aria-label="Close">
            <X className="h-4 w-4" aria-hidden="true" />
            {!minimal ? <span>Close</span> : null}
          </button>
        </header>
        <div className={`pipelines-modal-body access-modal-body ${minimal ? 'access-modal-body--minimal' : ''}`}>{children}</div>
      </div>
    </div>
  );
}
