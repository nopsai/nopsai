import type { ReactNode } from 'react';

export type YamlValidationError = {
  message: string;
  line?: number | null;
  column?: number | null;
};

type YamlValidationPanelProps = {
  id: string;
  errors: YamlValidationError[];
  maxVisible?: number;
  invalidLabel?: string;
  inline?: boolean;
  renderExample?: (message: string) => ReactNode;
};

export function YamlValidationPanel({
  id,
  errors,
  maxVisible = 3,
  invalidLabel = 'Invalid',
  inline = false,
  renderExample,
}: YamlValidationPanelProps) {
  const invalid = errors.length > 0;
  const className = [
    'validation-box',
    inline ? 'validation-box--inline' : '',
    invalid && inline ? 'validation-box--error' : '',
    invalid ? '' : 'validation-box--success',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <div id={id} className={className} role="status" aria-live="polite">
      <div className="validation-box__header">{invalid ? invalidLabel : 'Valid'}</div>
      {errors.slice(0, maxVisible).map((error, index) => {
        const example = renderExample?.(error.message);
        return (
          <div key={`${error.line ?? 'unknown'}-${error.message}-${index}`} className="validation-box__item">
            {typeof error.line === 'number' && <span className="validation-box__line">Line {error.line}</span>}
            <div className="validation-box__message">{error.message}</div>
            {example}
          </div>
        );
      })}
      {errors.length > maxVisible && (
        <div className="validation-box__item">
          <div className="validation-box__message">+ {errors.length - maxVisible} more…</div>
        </div>
      )}
    </div>
  );
}
