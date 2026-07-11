import type { ButtonHTMLAttributes, ReactNode } from 'react';
import { Search } from 'lucide-react';
import './aiResourcePanel.css';

export type AIResourceStat = {
  label: string;
  value: ReactNode;
  tone?: 'default' | 'ok' | 'warning' | 'info';
};

export function AIResourceHero({
  control,
  stats,
  actions,
}: {
  control: ReactNode;
  stats: AIResourceStat[];
  actions: ReactNode;
}) {
  return (
    <section className="ai-resource-hero">
      <div className="ai-resource-hero__card">
        <div className="ai-resource-hero__control">{control}</div>
        <div className="ai-resource-hero__divider" aria-hidden="true" />
        <div className="ai-resource-hero__stats">
          {stats.map(stat => (
            <div key={stat.label} className={`ai-resource-stat ai-resource-stat--${stat.tone || 'default'}`}>
              <span>{stat.label}</span>
              <strong>{stat.value}</strong>
            </div>
          ))}
        </div>
      </div>
      <div className="ai-resource-hero__actions">{actions}</div>
    </section>
  );
}

export function AIResourceTableHeader({
  title,
  count,
  loading,
  searchLabel,
  searchPlaceholder,
  searchValue,
  onSearchChange,
}: {
  title: string;
  count: ReactNode;
  loading?: boolean;
  searchLabel: string;
  searchPlaceholder: string;
  searchValue: string;
  onSearchChange: (value: string) => void;
}) {
  return (
    <div className="ai-resource-table-head">
      <div className="ai-resource-table-title">
        <h3>{title}</h3>
        <span>{count}</span>
        {loading && <em>Loading...</em>}
      </div>
      <label className="ai-resource-search">
        <span className="sr-only">{searchLabel}</span>
        <Search className="ai-resource-search__icon" aria-hidden="true" />
        <input
          aria-label={searchLabel}
          value={searchValue}
          onChange={event => onSearchChange(event.target.value)}
          placeholder={searchPlaceholder}
        />
      </label>
    </div>
  );
}

export function AIResourceIconAction({
  label,
  tone = 'default',
  className,
  children,
  type = 'button',
  ...buttonProps
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  tone?: 'default' | 'primary' | 'accent' | 'warning' | 'danger';
  children: ReactNode;
}) {
  const classes = [
    'ai-resource-icon-action',
    tone !== 'default' ? `ai-resource-icon-action--${tone}` : '',
    className || '',
  ].filter(Boolean).join(' ');

  return (
    <button {...buttonProps} type={type} className={classes} aria-label={label} title={buttonProps.title || label}>
      {children}
      <span className="sr-only">{label}</span>
    </button>
  );
}

export function AIResourceField({
  label,
  children,
  wide,
  mono,
}: {
  label: string;
  children: ReactNode;
  wide?: boolean;
  mono?: boolean;
}) {
  const className = [
    'ai-resource-field',
    wide ? 'ai-resource-field--wide' : '',
    mono ? 'ai-resource-field--mono' : '',
  ].filter(Boolean).join(' ');

  return (
    <div className={className}>
      <span className="ai-resource-field__label">{label}</span>
      <span className="ai-resource-field__value">{children}</span>
    </div>
  );
}

export function AIResourceEmptyState({ children }: { children: ReactNode }) {
  return <div className="ai-resource-list-empty">{children}</div>;
}
