import { type FormEventHandler, type ReactNode } from 'react';
import {
  WorkflowDialogCloseButton,
  WorkflowDialogFrame,
  workflowDialogOverlayClass,
} from './WorkflowPrimitives';

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
  size?: 'default' | 'wide' | 'xwide';
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
        <WorkflowDialogCloseButton onClose={onClose} disabled={closeDisabled} />
      </header>
      <div className={joinClasses('pipelines-modal-body', bodyClassName)}>
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
        size === 'xwide' && 'workflow-form-dialog--xwide',
        cardClassName
      )}
      overlayClassName={`workflow-form-dialog-overlay ${workflowDialogOverlayClass}`}
    >
      {onSubmit ? <form onSubmit={onSubmit}>{content}</form> : content}
    </WorkflowDialogFrame>
  );
}
