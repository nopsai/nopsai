import { useMemo, useState, type FormEvent, type ReactNode } from "react";
import { Check, Trash2, X } from "lucide-react";

export type AccessEditorSection<ID extends string = string> = {
  id: ID;
  label: string;
  description: string;
  children: ReactNode;
};

export type AccessSectionedEditorProps<ID extends string = string> = {
  modeLabel: string;
  entityLabel: string;
  title: string;
  subtitle: string;
  icon: ReactNode;
  sections: AccessEditorSection<ID>[];
  resetKey: string;
  saveLabel: string;
  savingLabel?: string;
  saving?: boolean;
  deleteLabel?: string;
  deleteDisabled?: boolean;
  secondaryFooterAction?: ReactNode;
  onClose: () => void;
  onDelete?: () => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
};

export function AccessSectionedEditor<ID extends string = string>({
  modeLabel,
  entityLabel,
  title,
  subtitle,
  icon,
  sections,
  resetKey,
  saveLabel,
  savingLabel = "Saving...",
  saving = false,
  deleteLabel,
  deleteDisabled = false,
  secondaryFooterAction,
  onClose,
  onDelete,
  onSubmit,
}: AccessSectionedEditorProps<ID>) {
  const firstSectionID = sections[0]?.id;
  const [selectedSection, setSelectedSection] = useState<{
    resetKey: string;
    sectionID: ID | undefined;
  }>(() => ({ resetKey, sectionID: firstSectionID }));
  const activeSection =
    selectedSection.resetKey === resetKey ? selectedSection.sectionID : firstSectionID;

  const activeIndex = useMemo(
    () => Math.max(0, sections.findIndex(section => section.id === activeSection)),
    [activeSection, sections],
  );
  const active = sections[activeIndex] ?? sections[0];

  if (!active) return null;

  return (
    <form className="access-sectioned-editor" onSubmit={onSubmit}>
      <header className="access-sectioned-editor__head">
        <div className="access-sectioned-editor__heading">
          <span className="access-sectioned-editor__icon" aria-hidden="true">
            {icon}
          </span>
          <div className="access-sectioned-editor__heading-copy">
            <p className="access-sectioned-editor__eyebrow">
              <span className="access-sectioned-editor__mode">{modeLabel}</span>
              <span>{entityLabel}</span>
            </p>
            <h5 className="access-sectioned-editor__title">{title}</h5>
            <p className="access-sectioned-editor__subtitle">{subtitle}</p>
          </div>
        </div>
        <button
          type="button"
          className="access-card-action access-sectioned-editor__close"
          aria-label="Close"
          title="Close"
          data-dialog-initial-focus
          onClick={onClose}
        >
          <X className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />
        </button>
      </header>

      <div className="access-sectioned-editor__layout">
        <nav className="access-sectioned-editor__nav" aria-label={`${entityLabel} editor sections`}>
          <div className="access-sectioned-editor__nav-label">Configuration</div>
          {sections.map((section, index) => {
            const activeNavItem = section.id === active.id;
            return (
              <button
                key={section.id}
                type="button"
                className={`access-sectioned-editor__nav-button ${activeNavItem ? "access-sectioned-editor__nav-button--active" : ""}`}
                aria-current={activeNavItem ? "step" : undefined}
                onClick={() => setSelectedSection({ resetKey, sectionID: section.id })}
              >
                <span className="access-sectioned-editor__nav-number">{index + 1}</span>
                <span className="access-sectioned-editor__nav-copy">
                  <strong>{section.label}</strong>
                  <span>{section.description}</span>
                </span>
                {index < activeIndex ? (
                  <Check className="access-sectioned-editor__nav-check h-3.5 w-3.5" strokeWidth={2.2} aria-hidden="true" />
                ) : null}
              </button>
            );
          })}
        </nav>

        <main className="access-sectioned-editor__content">
          <section className="access-sectioned-editor__panel" aria-label={active.label}>
            <div className="access-form-stack">{active.children}</div>
          </section>
        </main>
      </div>

      <footer className="access-sectioned-editor__footer">
        <div className="access-sectioned-editor__footer-left">
          {onDelete ? (
            <button
              type="button"
              className="access-inline-btn access-inline-btn--danger access-sectioned-editor__delete"
              disabled={deleteDisabled || saving}
              onClick={onDelete}
            >
              <Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />
              <span>{deleteLabel ?? "Delete"}</span>
            </button>
          ) : null}
        </div>
        <div className="access-sectioned-editor__footer-right">
          {secondaryFooterAction}
          <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={onClose} disabled={saving}>
            Cancel
          </button>
          <button type="submit" className="glass-button-primary" disabled={saving}>
            {saving ? savingLabel : saveLabel}
          </button>
        </div>
      </footer>
    </form>
  );
}

export function AccessFormCard({
  title,
  description,
  badge,
  children,
}: {
  title: string;
  description: string;
  badge?: string;
  children: ReactNode;
}) {
  return (
    <section className="access-form-card">
      <div className="access-form-card__header">
        <div>
          <h6 className="access-form-card__title">{title}</h6>
          <p className="access-form-card__description">{description}</p>
        </div>
        {badge ? <span className="access-form-card__badge">{badge}</span> : null}
      </div>
      <div className="access-form-card__body">{children}</div>
    </section>
  );
}

export function AccessReviewStat({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="access-review-stat">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export function AccessReviewRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="access-review-row">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}
