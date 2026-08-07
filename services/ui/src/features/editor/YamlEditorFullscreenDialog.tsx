import { useEffect, useId, type ReactNode } from 'react';
import { FileCode2, X } from 'lucide-react';

type YamlEditorFullscreenDialogProps = {
  title: string;
  subtitle?: string;
  validationIssueCount: number;
  actions?: ReactNode;
  children: ReactNode;
  onClose: () => void;
};

export function YamlEditorFullscreenDialog({
  title,
  subtitle,
  validationIssueCount,
  actions,
  children,
  onClose,
}: YamlEditorFullscreenDialogProps) {
  const titleID = useId();
  const issueLabel = validationIssueCount === 0
    ? 'YAML valid'
    : `${validationIssueCount} issue${validationIssueCount === 1 ? '' : 's'}`;

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
          <div className="yaml-editor-fullscreen-modal__title">
            <FileCode2 className="h-4 w-4" aria-hidden="true" />
            <div>
              <h2 id={titleID}>{title}</h2>
              {subtitle ? <p>{subtitle}</p> : null}
            </div>
            <span>{issueLabel}</span>
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
