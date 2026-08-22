import { useId, type ReactNode } from 'react';
import { Plus } from 'lucide-react';
import {
  WorkflowDialogCloseButton,
  WorkflowDialogFrame,
  WorkflowEmptyState,
} from '../../../components/WorkflowPrimitives';

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
    <WorkflowEmptyState
      badge={sectionLabel}
      hint={hint}
      actionLabel={actionLabel}
      actionIcon={<Plus className="h-4 w-4" strokeWidth={2} aria-hidden="true" />}
      onAction={onAction}
      className="access-editor-empty"
      metaClassName="access-editor-empty__meta"
      badgeClassName="access-editor-empty__badge"
      hintClassName="access-editor-empty__hint"
      footerClassName="access-editor-empty__footer"
      actionClassName="glass-button-primary access-editor-empty__button"
    />
  );
}

export function AccessModal({
  kicker,
  title,
  subtitle,
  icon,
  onClose,
  children,
  actions,
  variant = 'default',
}: {
  kicker: string;
  title: string;
  subtitle?: string;
  icon?: ReactNode;
  onClose: () => void;
  children: ReactNode;
  /** Dialog actions. They render on the shared action bar under the canvas. */
  actions?: ReactNode;
  variant?: 'default' | 'minimal';
}) {
  const titleID = useId();
  const descriptionID = useId();
  const minimal = variant === 'minimal';

  return (
    <WorkflowDialogFrame
      role={minimal ? 'alertdialog' : 'dialog'}
      titleId={titleID}
      descriptionId={subtitle ? descriptionID : undefined}
      onClose={onClose}
      className={`pipelines-modal-card access-modal-card w-full ${minimal ? 'workflow-dialog--compact' : ''}`}
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
          <WorkflowDialogCloseButton onClose={onClose} />
        </header>
        <div className={`pipelines-modal-body access-modal-body ${minimal ? 'access-modal-body--minimal' : ''}`}>{children}</div>
        {actions ? (
          <footer className="pipelines-modal-footer">
            <div className="pipelines-modal-actions">{actions}</div>
          </footer>
        ) : null}
    </WorkflowDialogFrame>
  );
}
