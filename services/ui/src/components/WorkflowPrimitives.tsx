import { type ButtonHTMLAttributes, type ReactNode } from 'react';
import { useDialogFocus } from './useDialogFocus';

export function WorkflowDialogFrame({
  id,
  role = 'dialog',
  titleId,
  descriptionId,
  onClose,
  className,
  overlayClassName = 'fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show',
  children,
}: {
  id?: string;
  role?: 'dialog' | 'alertdialog';
  titleId: string;
  descriptionId?: string;
  onClose: () => void;
  className: string;
  overlayClassName?: string;
  children: ReactNode;
}) {
  const dialogRef = useDialogFocus(onClose);

  return (
    <div
      id={id}
      className={overlayClassName}
      onPointerDown={event => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        ref={dialogRef}
        className={className}
        role={role}
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        tabIndex={-1}
      >
        {children}
      </div>
    </div>
  );
}

export function WorkflowInlineAlert({
  id,
  children,
  className = 'text-sm text-red-500',
}: {
  id?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <p id={id} className={className} role="alert">
      {children}
    </p>
  );
}

export function WorkflowEmptyState({
  badge,
  hint,
  actionLabel,
  actionIcon,
  onAction,
  className = 'pipelines-empty',
  metaClassName,
  badgeClassName,
  hintClassName,
  footerClassName,
  actionClassName = 'glass-button-primary',
}: {
  badge: ReactNode;
  hint: ReactNode;
  actionLabel?: string;
  actionIcon?: ReactNode;
  onAction?: () => void;
  className?: string;
  metaClassName?: string;
  badgeClassName?: string;
  hintClassName?: string;
  footerClassName?: string;
  actionClassName?: string;
}) {
  return (
    <div className={className}>
      <div className={metaClassName}>
        <span className={badgeClassName}>{badge}</span>
        <p className={hintClassName}>{hint}</p>
      </div>
      {actionLabel && onAction ? (
        <div className={footerClassName}>
          <button type="button" className={actionClassName} onClick={onAction}>
            {actionIcon}
            <span>{actionLabel}</span>
          </button>
        </div>
      ) : null}
    </div>
  );
}

export function WorkflowIconButton({
  label,
  icon,
  showLabel = false,
  title = label,
  className = 'pipelines-icon-only',
  type = 'button',
  ...buttonProps
}: Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'aria-label' | 'children'> & {
  label: string;
  icon: ReactNode;
  showLabel?: boolean;
}) {
  return (
    <button
      {...buttonProps}
      type={type}
      className={className}
      aria-label={label}
      title={title}
    >
      {icon}
      {showLabel ? <span>{label}</span> : null}
    </button>
  );
}
