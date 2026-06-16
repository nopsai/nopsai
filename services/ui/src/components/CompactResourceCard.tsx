import type { ReactNode } from 'react';

export type CompactResourceFact = {
  label: string;
  value: ReactNode;
  mono?: boolean;
  title?: string;
};

export function CompactResourceCard({
  icon,
  title,
  subtitle,
  description,
  badges,
  facts,
  headingActions,
  actions,
  footerActions,
  selected = false,
  selectionLabel,
  onSelect,
  tone = 'neutral',
  className = '',
}: {
  icon: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  description?: ReactNode;
  badges?: ReactNode;
  facts: CompactResourceFact[];
  headingActions?: ReactNode;
  actions?: ReactNode;
  footerActions?: ReactNode;
  selected?: boolean;
  selectionLabel?: string;
  onSelect?: () => void;
  tone?: 'neutral' | 'violet' | 'cyan' | 'blue';
  className?: string;
}) {
  const content = (
    <>
      <div className="compact-resource-card__heading">
        <div className="compact-resource-card__identity-row">
          <span className="pipeline-card-icon compact-resource-card__icon" aria-hidden="true">{icon}</span>
          <div className="compact-resource-card__identity">
            <h2 className="compact-resource-card__title">{title}</h2>
            {subtitle ? <p className="compact-resource-card__subtitle">{subtitle}</p> : null}
          </div>
          {headingActions ? (
            <div className="compact-resource-card__heading-actions">{headingActions}</div>
          ) : null}
        </div>
      </div>
      <div className="compact-resource-card__description-slot">
        {description ? <p className="compact-resource-card__description">{description}</p> : null}
      </div>
      {facts.length ? (
        <dl className="compact-resource-card__facts">
          {facts.map((fact, index) => (
            <div className="compact-resource-card__fact" key={`${fact.label}-${index}`}>
              <dt>{fact.label}</dt>
              <dd className={fact.mono ? 'font-mono' : undefined} title={fact.title}>
                {fact.value}
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
    </>
  );

  return (
    <article
      className={`glass-card pipeline-card compact-resource-card compact-resource-card--${tone}${onSelect ? ' compact-resource-card--selectable' : ' compact-resource-card--static'}${selected ? ' compact-resource-card--selected' : ''}${className ? ` ${className}` : ''}`}
      data-selected={selected ? 'true' : undefined}
    >
      <div className="compact-resource-card__layout">
        <div className="compact-resource-card__content">
          {content}
          {badges || footerActions ? (
            <div className="compact-resource-card__footer">
              {badges ? <div className="compact-resource-card__badges">{badges}</div> : null}
              {footerActions ? <div className="compact-resource-card__footer-actions">{footerActions}</div> : null}
            </div>
          ) : null}
          {onSelect ? (
            <button
              type="button"
              className="compact-resource-card__select"
              aria-label={selectionLabel}
              aria-pressed={selected}
              onClick={onSelect}
            />
          ) : null}
        </div>
        {actions ? <div className="compact-resource-card__actions">{actions}</div> : null}
      </div>
    </article>
  );
}
