import { useEffect, useId, type ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, FileCode2, X } from 'lucide-react';
import type { YamlValidationError } from './YamlValidationPanel';

type YamlEditorFullscreenDialogProps = {
  title: string;
  subtitle?: string;
  validationErrors: YamlValidationError[];
  validationMaxVisible?: number;
  renderValidationExample?: (message: string) => ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  onClose: () => void;
};

export function YamlEditorFullscreenDialog({
  title,
  subtitle,
  validationErrors,
  validationMaxVisible = 4,
  renderValidationExample,
  actions,
  children,
  onClose,
}: YamlEditorFullscreenDialogProps) {
  const titleID = useId();

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  return (
    <div
      className="yaml-editor-fullscreen-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleID}
      onMouseDown={event => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <section className="yaml-editor-fullscreen-modal__surface">
        <header className="yaml-editor-fullscreen-modal__head">
          <div className="yaml-editor-fullscreen-modal__head-main">
            <div className="yaml-editor-fullscreen-modal__title">
              <FileCode2 className="h-4 w-4" aria-hidden="true" />
              <div>
                <h2 id={titleID}>{title}</h2>
                {subtitle ? <p>{subtitle}</p> : null}
              </div>
            </div>
            <TopBarValidation
              errors={validationErrors}
              maxVisible={validationMaxVisible}
              renderExample={renderValidationExample}
            />
          </div>
          <div className="yaml-editor-fullscreen-modal__actions">
            {actions}
            <button
              type="button"
              className="yaml-editor-fullscreen-modal__close"
              aria-label="Close expanded YAML editor"
              onClick={onClose}
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </header>
        <div className="yaml-editor-fullscreen-modal__body">
          {children}
        </div>
      </section>
    </div>
  );
}

function TopBarValidation({
  errors,
  maxVisible,
  renderExample,
}: {
  errors: YamlValidationError[];
  maxVisible: number;
  renderExample?: (message: string) => ReactNode;
}) {
  if (!errors.length) {
    return (
      <div className="yaml-editor-fullscreen-modal__validation yaml-editor-fullscreen-modal__validation--valid" role="status" aria-live="polite">
        <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
        <span>YAML valid</span>
      </div>
    );
  }

  const issueLabel = `${errors.length} validation issue${errors.length === 1 ? '' : 's'}`;
  const visibleErrors = errors.slice(0, maxVisible);
  const hiddenCount = Math.max(0, errors.length - visibleErrors.length);

  return (
    <details className="yaml-editor-fullscreen-modal__validation yaml-editor-fullscreen-modal__validation--invalid">
      <summary>
        <AlertTriangle className="h-4 w-4" aria-hidden="true" />
        <span>{issueLabel}</span>
      </summary>
      <div className="yaml-editor-fullscreen-modal__validation-list" role="status" aria-live="polite">
        {visibleErrors.map((error, index) => {
          const example = renderExample?.(error.message);
          return (
            <div key={`${error.line ?? 'unknown'}-${error.message}-${index}`} className="yaml-editor-fullscreen-modal__validation-item">
              {typeof error.line === 'number' ? <span className="yaml-editor-fullscreen-modal__validation-line">Line {error.line}</span> : null}
              <p className="yaml-editor-fullscreen-modal__validation-message">{error.message}</p>
              {example ? <div className="yaml-editor-fullscreen-modal__validation-example">{example}</div> : null}
            </div>
          );
        })}
        {hiddenCount ? (
          <div className="yaml-editor-fullscreen-modal__validation-item">
            <p className="yaml-editor-fullscreen-modal__validation-message">{`+ ${hiddenCount} more`}</p>
          </div>
        ) : null}
      </div>
    </details>
  );
}
