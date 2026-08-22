import { type ButtonHTMLAttributes, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { useDialogFocus } from './useDialogFocus';

/*
 * Overlay class for every dialog in the product. `workflow-dialog-shell` is what
 * components/modalShell.css hangs the shared skin on, so a dialog that opts out
 * of it silently loses the skin — pass this constant instead of retyping the
 * class list when a feature needs its own spacing.
 */
export const workflowDialogOverlayClass =
  'workflow-dialog-shell fixed inset-0 z-50 flex items-center justify-center bg-[var(--bg-overlay)] show';

export function WorkflowDialogFrame({
  id,
  role = 'dialog',
  titleId,
  descriptionId,
  onClose,
  className,
  overlayClassName = workflowDialogOverlayClass,
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

/*
 * Close control for the dialog title pill. It reads as an icon so the pill stays
 * a title bar, and keeps the accessible name "Close" that every dialog test and
 * screen reader already expects.
 */
export function WorkflowDialogCloseButton({
  onClose,
  disabled = false,
  label = 'Close',
  initialFocus = false,
}: {
  onClose: () => void;
  disabled?: boolean;
  label?: string;
  /** Take the dialog's opening focus, for dialogs with nothing to type into. */
  initialFocus?: boolean;
}) {
  return (
    <button
      type="button"
      className="workflow-dialog-close"
      onClick={onClose}
      disabled={disabled}
      aria-label={label}
      title={label}
      data-dialog-initial-focus={initialFocus || undefined}
    >
      <X aria-hidden="true" />
    </button>
  );
}

/*
 * One row of the property inspector: the label and its hint on the left, the
 * control on the right, aligned with every other row in the grid.
 */
export function WorkflowPropertyRow({
  label,
  hint,
  htmlFor,
  span = 'half',
  layout = 'inline',
  children,
}: {
  label: ReactNode;
  hint?: ReactNode;
  htmlFor?: string;
  span?: 'half' | 'full';
  layout?: 'inline' | 'stacked';
  children: ReactNode;
}) {
  const classes = [
    'modal-property-row',
    span === 'full' ? 'modal-property-row--full' : '',
    layout === 'stacked' ? 'modal-property-row--stacked' : '',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <div className={classes}>
      <div className="min-w-0">
        <label className="modal-property-label" htmlFor={htmlFor}>
          {label}
        </label>
        {hint ? <span className="modal-property-hint">{hint}</span> : null}
      </div>
      <div className="modal-property-control">{children}</div>
    </div>
  );
}

/*
 * Segmented control for a short closed set of choices. Real radios stay in the
 * markup, so the group keeps arrow-key navigation and its accessible names.
 */
export function WorkflowSegmentedControl<Value extends string>({
  name,
  value,
  options,
  onChange,
  legend,
  size = 'compact',
  stretch = false,
  disabled = false,
}: {
  name: string;
  value: Value;
  options: Array<{ value: Value; label: string }>;
  onChange: (value: Value) => void;
  legend?: string;
  /** 'compact' sits in a property row; 'pill' is a control in its own right. */
  size?: 'compact' | 'pill';
  stretch?: boolean;
  disabled?: boolean;
}) {
  const classes = [
    'modal-segmented',
    size === 'pill' ? 'modal-segmented--pill' : '',
    stretch ? 'modal-segmented--stretch' : '',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <div className={classes} role="group" aria-label={legend}>
      {options.map(option => (
        <label key={option.value}>
          <input
            type="radio"
            name={name}
            value={option.value}
            checked={value === option.value}
            disabled={disabled}
            onChange={() => onChange(option.value)}
          />
          <span>{option.label}</span>
        </label>
      ))}
    </div>
  );
}
