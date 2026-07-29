import type { ChangeEvent, ReactNode } from 'react';
import {
  Bell,
  CheckCircle2,
  CloudCog,
  GitBranch,
  Mail,
  ServerCog,
  Settings2,
  ShieldCheck,
  SlidersHorizontal,
} from 'lucide-react';
import {
  type SystemSettingsSection,
  type SystemSettingsSectionId,
  type SystemSettingsSummaryCard,
} from './settingsPresentation';
import { systemSettingsSectionDomID } from './settingsWorkspaceHelpers';

export function SummaryCard({ card }: { card: SystemSettingsSummaryCard }) {
  return (
    <article className={`system-settings-summary-card system-settings-summary-card--${card.tone}`}>
      <div className="system-settings-summary-card__top">
        <span>{card.label}</span>
        {summaryIcon(card.id)}
      </div>
      <strong>{card.value}</strong>
      <p>{card.detail}</p>
    </article>
  );
}

export function SettingsSection({ section, children }: { section: SystemSettingsSection; children: ReactNode }) {
  return (
    <section
      id={systemSettingsSectionDomID(section.id)}
      className="system-settings-section"
      data-settings-section={section.id}
      role="tabpanel"
      aria-labelledby={`system-settings-tab-${section.id}`}
    >
      <div className="system-settings-section__header">
        <p>{section.eyebrow}</p>
        <h3>{section.title}</h3>
        <span>{section.description}</span>
      </div>
      {children}
    </section>
  );
}

export function SettingsPanel({
  title,
  description,
  action,
  children,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="system-settings-card system-settings-card--quiet">
      <div className="system-settings-card__toolbar system-settings-card__toolbar--compact">
        <div>
          <h3>{title}</h3>
          {description && <p>{description}</p>}
        </div>
        {action}
      </div>
      {children}
    </div>
  );
}

export function SettingField({ label, children, helper, wide = false }: { label: ReactNode; children: ReactNode; helper?: ReactNode; wide?: boolean }) {
  return (
    <label className={`system-settings-field ${wide ? 'system-settings-field--wide' : ''}`}>
      <span className="system-settings-field__label">{label}</span>
      {children}
      {helper && <span className="system-settings-field__helper">{helper}</span>}
    </label>
  );
}

export function SettingsToggle({
  label,
  checked,
  onChange,
  disabled,
  wide = false,
}: {
  label: ReactNode;
  checked: boolean;
  onChange: (event: ChangeEvent<HTMLInputElement>) => void;
  disabled: boolean;
  wide?: boolean;
}) {
  return (
    <label className={`system-settings-toggle ${wide ? 'system-settings-toggle--wide' : ''}`}>
      <span className="system-settings-toggle__copy">{label}</span>
      <input type="checkbox" checked={checked} onChange={onChange} disabled={disabled} />
      <span className="system-settings-toggle__track" aria-hidden="true">
        <span />
      </span>
    </label>
  );
}

export function ErrorBox({ children }: { children: ReactNode }) {
  return <div className="system-settings-error">{children}</div>;
}

export function RepoFact({ label, value, truncate = false }: { label: string; value: string; truncate?: boolean }) {
  return (
    <div className="system-settings-repo-fact">
      <span>{label}</span>
      <strong className={truncate ? 'truncate' : undefined} title={value}>{value}</strong>
    </div>
  );
}

export function SectionIcon({ sectionID }: { sectionID: SystemSettingsSectionId }) {
  switch (sectionID) {
    case 'platform':
      return <Settings2 className="h-4 w-4" />;
    case 'execution':
      return <ServerCog className="h-4 w-4" />;
    case 'networking':
      return <CloudCog className="h-4 w-4" />;
    case 'notifications':
      return <Bell className="h-4 w-4" />;
    case 'source':
      return <GitBranch className="h-4 w-4" />;
  }
}

function summaryIcon(cardID: string) {
  switch (cardID) {
    case 'environment':
      return <ShieldCheck className="h-4 w-4" />;
    case 'runtime-pools':
      return <SlidersHorizontal className="h-4 w-4" />;
    case 'mail':
      return <Mail className="h-4 w-4" />;
    case 'gitops':
      return <CheckCircle2 className="h-4 w-4" />;
    default:
      return <Settings2 className="h-4 w-4" />;
  }
}
