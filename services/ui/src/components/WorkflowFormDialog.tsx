import { type FormEventHandler, type ReactNode } from 'react';
import { WorkflowDialogFrame } from './WorkflowPrimitives';

type WorkflowFormDialogProps = {
  id: string;
  titleId: string;
  descriptionId?: string;
  kicker: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  headerLeading?: ReactNode;
  children: ReactNode;
  actions: ReactNode;
  onClose: () => void;
  onSubmit?: FormEventHandler<HTMLFormElement>;
  closeDisabled?: boolean;
  size?: 'default' | 'wide';
  cardClassName?: string;
  bodyClassName?: string;
};

function joinClasses(...classes: Array<string | undefined | false>) {
  return classes.filter(Boolean).join(' ');
}

export function WorkflowFormDialog({
  id,
  titleId,
  descriptionId,
  kicker,
  title,
  subtitle,
  headerLeading,
  children,
  actions,
  onClose,
  onSubmit,
  closeDisabled = false,
  size = 'default',
  cardClassName,
  bodyClassName = 'space-y-4',
}: WorkflowFormDialogProps) {
  const content = (
    <>
      <header className="pipelines-modal-header">
        <div className="flex min-w-0 items-center gap-3">
          {headerLeading}
          <div className="min-w-0">
            <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">{kicker}</p>
            <h3 id={titleId} className="text-lg font-semibold text-[var(--text-primary)]">
              {title}
            </h3>
            {subtitle ? <p className="mt-1 text-sm text-[var(--text-secondary)]">{subtitle}</p> : null}
          </div>
        </div>
        <button
          type="button"
          className="glass-button-ghost"
          onClick={onClose}
          disabled={closeDisabled}
        >
          Close
        </button>
      </header>
      <div
        className={joinClasses(
          'pipelines-modal-body max-h-[calc(100vh-12rem)] overflow-y-auto',
          bodyClassName
        )}
      >
        {children}
      </div>
      <footer className="pipelines-modal-footer">
        <div className="pipelines-modal-actions">{actions}</div>
      </footer>
    </>
  );

  return (
    <WorkflowDialogFrame
      id={id}
      titleId={titleId}
      descriptionId={descriptionId}
      onClose={onClose}
      className={joinClasses(
        'pipelines-modal-card workflow-form-dialog w-full',
        size === 'wide' && 'workflow-form-dialog--wide',
        cardClassName
      )}
      overlayClassName="workflow-form-dialog-overlay fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)] p-4 show"
    >
      {onSubmit ? <form onSubmit={onSubmit}>{content}</form> : content}
    </WorkflowDialogFrame>
  );
}
