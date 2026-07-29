import { useRef, useState, type ButtonHTMLAttributes, type ReactNode } from 'react';
import { Search, X } from 'lucide-react';
import {
  AI_RESOURCE_TEAM_FILTER_ALL,
  AI_RESOURCE_TEAM_FILTER_GLOBAL,
  buildAIResourceScopedID,
  formatAIResourceTeamLabel,
  normalizeAIResourceTeamPath,
} from './aiResourceTeams';
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
  filters,
  className,
}: {
  title?: string;
  count?: ReactNode;
  loading?: boolean;
  searchLabel?: string;
  searchPlaceholder?: string;
  searchValue?: string;
  onSearchChange?: (value: string) => void;
  filters?: ReactNode;
  className?: string;
}) {
  const hasCount = count !== undefined && count !== null && count !== false;
  const hasTitle = Boolean(title || hasCount || loading);
  const searchControl = searchLabel && typeof searchValue === 'string' && onSearchChange ? (
    <AIResourceExpandableSearch
      label={searchLabel}
      placeholder={searchPlaceholder || searchLabel}
      value={searchValue}
      onChange={onSearchChange}
    />
  ) : null;
  const headerClassName = [
    'ai-resource-table-head',
    !hasTitle ? 'ai-resource-table-head--controls-only' : '',
    className || '',
  ].filter(Boolean).join(' ');

  return (
    <div className={headerClassName}>
      {hasTitle ? (
        <div className="ai-resource-table-title">
          {title ? <h3>{title}</h3> : null}
          {hasCount ? <span>{count}</span> : null}
          {loading ? <em>Loading...</em> : null}
        </div>
      ) : null}
      <div className="ai-resource-table-controls">
        {filters}
        {searchControl}
      </div>
    </div>
  );
}

export function AIResourceExpandableSearch({
  label,
  placeholder,
  value,
  onChange,
  className,
}: {
  label: string;
  placeholder: string;
  value: string;
  onChange: (value: string) => void;
  className?: string;
}) {
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const searchActive = searchOpen || Boolean(value.trim());
  const classes = [
    'ai-resource-search',
    searchActive ? 'ai-resource-search--open' : '',
    className || '',
  ].filter(Boolean).join(' ');

  return (
    <div className={classes}>
      <button
        type="button"
        className="ai-resource-search__toggle"
        aria-label={label}
        title={label}
        onClick={() => {
          setSearchOpen(true);
          requestAnimationFrame(() => searchInputRef.current?.focus());
        }}
      >
        <Search className="ai-resource-search__icon" aria-hidden="true" />
      </button>
      <input
        ref={searchInputRef}
        type="search"
        aria-label={`${label} query`}
        value={value}
        onFocus={() => setSearchOpen(true)}
        onChange={event => {
          onChange(event.target.value);
          if (event.target.value && !searchOpen) setSearchOpen(true);
        }}
        onBlur={() => {
          if (!value.trim()) setSearchOpen(false);
        }}
        placeholder={placeholder}
      />
      {value || searchOpen ? (
        <button
          type="button"
          className="ai-resource-search__clear"
          aria-label="Clear search"
          onMouseDown={event => event.preventDefault()}
          onClick={() => {
            onChange('');
            setSearchOpen(false);
            searchInputRef.current?.blur();
          }}
        >
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      ) : null}
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

export function AIResourceTeamBadge({ resourceID }: { resourceID: string }) {
  const label = formatAIResourceTeamLabel(resourceID);
  const isGlobal = label === 'Global';
  return (
    <span className={`ai-resource-team-badge ${isGlobal ? 'ai-resource-team-badge--global' : ''}`}>
      {label}
    </span>
  );
}

export function AIResourceTeamFilter({
  value,
  onChange,
  teamPaths,
  disabled,
}: {
  value: string;
  onChange: (value: string) => void;
  teamPaths: string[];
  disabled?: boolean;
}) {
  const normalizedTeamPaths = teamPaths.map(normalizeAIResourceTeamPath).filter(Boolean);
  const selectableTeamPaths = [...new Set(normalizedTeamPaths)];
  const normalizedValue = normalizeAIResourceTeamPath(value);
  const safeValue = value === AI_RESOURCE_TEAM_FILTER_GLOBAL
    ? AI_RESOURCE_TEAM_FILTER_GLOBAL
    : normalizedValue && selectableTeamPaths.includes(normalizedValue)
      ? normalizedValue
      : AI_RESOURCE_TEAM_FILTER_ALL;

  return (
    <label className="ai-resource-team-filter">
      <span className="sr-only">Filter by team</span>
      <select
        aria-label="Filter by team"
        value={safeValue}
        onChange={event => onChange(event.target.value)}
        disabled={disabled}
      >
        <option value={AI_RESOURCE_TEAM_FILTER_ALL}>All teams</option>
        <option value={AI_RESOURCE_TEAM_FILTER_GLOBAL}>Global</option>
        {selectableTeamPaths.map(path => (
          <option key={path} value={path}>
            /{path}
          </option>
        ))}
      </select>
    </label>
  );
}

export function AIResourceTeamPlacementField({
  teamPath,
  onTeamPathChange,
  teamPaths,
  teamPathsLoading,
  localName,
  resourceLabel,
  disabled,
}: {
  teamPath: string;
  onTeamPathChange: (value: string) => void;
  teamPaths: string[];
  teamPathsLoading?: boolean;
  localName: string;
  resourceLabel: string;
  disabled?: boolean;
}) {
  const normalizedTeamPath = normalizeAIResourceTeamPath(teamPath);
  const normalizedTeamPaths = teamPaths.map(normalizeAIResourceTeamPath).filter(Boolean);
  const selectableTeamPaths = [...new Set(normalizedTeamPath ? [...normalizedTeamPaths, normalizedTeamPath] : normalizedTeamPaths)];
  const finalID = buildAIResourceScopedID(normalizedTeamPath, localName);

  return (
    <div className="ai-resource-team-placement">
      <label className="ai-resource-team-placement__field">
        <span>Team placement</span>
        <select
          value={normalizedTeamPath}
          onChange={event => onTeamPathChange(event.target.value)}
          disabled={disabled || teamPathsLoading}
        >
          <option value="">Global workspace</option>
          {selectableTeamPaths.map(path => (
            <option key={path} value={path}>
              /{path}
            </option>
          ))}
        </select>
      </label>
      <div className="ai-resource-team-placement__preview" aria-live="polite">
        <span>{resourceLabel} ID</span>
        <strong>{finalID || (normalizedTeamPath ? `${normalizedTeamPath}/...` : 'Enter a name')}</strong>
      </div>
    </div>
  );
}
