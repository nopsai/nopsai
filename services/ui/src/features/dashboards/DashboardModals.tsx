import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react';
import { AlertTriangle, CalendarClock, Pencil, PlugZap, RefreshCw, Save, Trash2, Workflow, X } from 'lucide-react';

import { ObjectIcon } from '../../components/ObjectIcon';
import { WorkflowFormDialog } from '../../components/WorkflowFormDialog';
import {
  WorkflowDialogCloseButton,
  WorkflowDialogFrame,
  WorkflowInlineAlert,
  WorkflowPropertyRow,
} from '../../components/WorkflowPrimitives';
import {
  MONTHDAY_VALUES,
  MONTH_OPTIONS,
  WEEKDAY_OPTIONS,
  WEEKDAY_VALUES,
  getTimezoneOptions,
  normalizeCronList,
  toggleCronListValue,
  type CronFormFields,
  type CronMode,
} from '../schedules/model';
import { dashboardRefFromForm } from './pipelineAssignments';
import {
  normalizeRunScope,
  runScopeLabel,
  titleFromKey,
  refreshScheduleCronExpressionFromForm,
  type DashboardFormState,
  type DashboardPublication,
  type DashboardRefreshFormState,
  type DashboardRefreshSchedule,
  type DashboardRefreshScheduleFormState,
  type DashboardSection,
  type DashboardSectionFormState,
  type DashboardSource,
  type DashboardSourceFormState,
  type DashboardSummary,
} from './model';
import {
  buildDashboardEntryOptions,
  type DashboardPipelineCatalogItem,
  type DashboardPipelineOutputOption,
} from './sourceOptions';

export type DashboardModalState =
  | { mode: 'create' }
  | { mode: 'edit'; dashboard: DashboardSummary };

export type SourceModalState =
  | { mode: 'create'; sectionKey: string }
  | { mode: 'edit'; source: DashboardSource };

export type SectionModalState =
  | { mode: 'create' }
  | { mode: 'edit'; section: DashboardSection };

export type RefreshScheduleModalState =
  | { mode: 'create' }
  | { mode: 'edit'; schedule: DashboardRefreshSchedule };

export type DashboardDeleteModalState =
  | { kind: 'dashboard'; dashboard: DashboardSummary }
  | { kind: 'section'; section: DashboardSection }
  | { kind: 'publication'; publication: DashboardPublication }
  | { kind: 'source'; source: DashboardSource }
  | { kind: 'schedule'; schedule: DashboardRefreshSchedule };

type Option = {
  value: string;
  label: string;
};

const SCHEDULE_FREQUENCY_OPTIONS: Option[] = [
  { value: 'minutes', label: 'Every N minutes' },
  { value: 'hourly', label: 'Hourly' },
  { value: 'daily', label: 'Daily' },
  { value: 'weekdays', label: 'Weekdays' },
  { value: 'weekly', label: 'Weekly' },
  { value: 'monthly', label: 'Monthly' },
  { value: 'yearly', label: 'Yearly' },
  { value: 'custom', label: 'Custom cron' },
];

const TIMEZONE_OPTIONS = getTimezoneOptions();

export function DashboardModal({
  modal,
  form,
  teams,
  sections = [],
  pipelineOptions = [],
  scopeOptions = [''],
  pipelineLoading = false,
  saving,
  error,
  onChange,
  onEditSection,
  onDeleteSection,
  onClose,
  onSubmit,
}: {
  modal: DashboardModalState;
  form: DashboardFormState;
  teams: string[];
  sections?: DashboardSection[];
  pipelineOptions?: DashboardPipelineCatalogItem[];
  scopeOptions?: string[];
  pipelineLoading?: boolean;
  saving: boolean;
  error: string | null;
  onChange: (next: DashboardFormState) => void;
  onEditSection?: (section: DashboardSection) => void;
  onDeleteSection?: (section: DashboardSection) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  const isCreate = modal.mode === 'create';
  const teamOptions = uniqueOptions(teams.filter(Boolean));
  const selectedTeamPath = teamOptions.some(option => option.value === form.teamPath) ? form.teamPath : '';
  const title = isCreate ? 'New dashboard' : 'Edit dashboard';
  const dashboardRef = dashboardRefFromForm(form);
  const matchingPipelineOptions = useMemo(
    () => dashboardPipelineOptionsForRef(pipelineOptions, dashboardRef),
    [dashboardRef, pipelineOptions]
  );
  const canSubmit =
    Boolean(selectedTeamPath && form.slug.trim() && form.title.trim()) &&
    !saving;
  const errorID = error ? 'dashboard-form-error' : undefined;

  return (
    <WorkflowFormDialog
      id={isCreate ? 'dashboard-new-modal' : 'dashboard-edit-modal'}
      titleId="dashboard-form-title"
      descriptionId={errorID}
      kicker="Dashboard"
      title={title}
      subtitle={isCreate ? 'Create the view from pipeline dashboard outputs; assign broader access from Access.' : 'Update dashboard metadata and pipeline assignments.'}
      headerLeading={<ModalIcon icon={<ObjectIcon type="dashboard" className="h-4 w-4" />} />}
      onClose={onClose}
      onSubmit={event => submitForm(event, onSubmit)}
      closeDisabled={saving}
      size="xwide"
      bodyClassName="space-y-4"
      actions={(
        <>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={saving}>
            <X className="h-4 w-4" aria-hidden="true" />
            Cancel
          </button>
          <button type="submit" className="glass-button-primary" disabled={!canSubmit}>
            <Save className="h-4 w-4" aria-hidden="true" />
            {saving ? 'Saving...' : 'Save'}
          </button>
        </>
      )}
    >
      <FormSection
        title="Dashboard identity"
        description="Choose where the dashboard lives and how people recognize it in the selector."
      >
        <div className="modal-property-grid">
          <Field label="Team" description="Existing team that owns the dashboard and its GitOps path.">
            <select
              className="pipelines-input w-full"
              value={selectedTeamPath}
              onChange={event => onChange({ ...form, teamPath: event.target.value })}
              disabled={saving || teamOptions.length === 0}
              required
              data-dialog-initial-focus
            >
              <option value="" disabled>{teamOptions.length === 0 ? 'No teams available' : 'Select team'}</option>
              {teamOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          <Field label="Slug" description="Stable URL-safe name used after the team path.">
            <input
              className="pipelines-input w-full"
              value={form.slug}
              onChange={event => onChange({ ...form, slug: event.target.value })}
              disabled={saving}
              required
            />
          </Field>
          <Field label="Title" description="Short display name shown in the dashboard dropdown and header.">
            <input
              className="pipelines-input w-full"
              value={form.title}
              onChange={event => onChange({ ...form, title: event.target.value })}
              disabled={saving}
              required
            />
          </Field>
          <Field label="Description" description="One-line purpose statement for operators choosing between dashboards.">
            <input
              className="pipelines-input w-full"
              value={form.description}
              onChange={event => onChange({ ...form, description: event.target.value })}
              disabled={saving}
              placeholder="One sentence for the dashboard catalog"
            />
          </Field>
        </div>
      </FormSection>

      <FormSection
        title="Pipeline sources"
        description="Select pipelines that publish dashboard outputs to this dashboard; sections and source bindings come from the output metadata."
      >
        <PipelineAssignmentPicker
          dashboardRef={dashboardRef}
          pipelines={matchingPipelineOptions}
          selectedIDs={form.pipelineIDs}
          scopeOptions={scopeOptions}
          scopeByPipelineID={form.pipelineScopes}
          saving={saving}
          loading={pipelineLoading}
          required={isCreate}
          onToggle={pipelineID => onChange(togglePipelineAssignment(form, pipelineID))}
          onScopeChange={(pipelineID, runScope) => onChange({
            ...form,
            pipelineScopes: {
              ...form.pipelineScopes,
              [pipelineID]: normalizeRunScope(runScope),
            },
          })}
        />
      </FormSection>
      {!isCreate ? (
        <SectionManager
          sections={sections}
          saving={saving}
          onEditSection={onEditSection}
          onDeleteSection={onDeleteSection}
        />
      ) : null}
      {error ? <p id={errorID} className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700" role="alert">{error}</p> : null}
    </WorkflowFormDialog>
  );
}

function PipelineAssignmentPicker({
  dashboardRef,
  pipelines,
  selectedIDs,
  scopeOptions,
  scopeByPipelineID,
  saving,
  loading,
  required,
  onToggle,
  onScopeChange,
}: {
  dashboardRef: string;
  pipelines: DashboardPipelineCatalogItem[];
  selectedIDs: string[];
  scopeOptions: string[];
  scopeByPipelineID: Record<string, string>;
  saving: boolean;
  loading: boolean;
  required: boolean;
  onToggle: (pipelineID: string) => void;
  onScopeChange: (pipelineID: string, runScope: string) => void;
}) {
  if (!dashboardRef) {
    return (
      <div className="rounded-lg border border-dashed border-[var(--border-subtle)] bg-[var(--bg-primary)] px-3 py-4 text-sm text-[var(--text-secondary)]">
        Choose a team and slug to see matching dashboard-output pipelines.
      </div>
    );
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-dashed border-[var(--border-subtle)] bg-[var(--bg-primary)] px-3 py-4 text-sm text-[var(--text-secondary)]">
        Loading dashboard-output pipelines...
      </div>
    );
  }

  if (pipelines.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-[var(--border-subtle)] bg-[var(--bg-primary)] px-3 py-4 text-sm text-[var(--text-secondary)]">
        {required
          ? `No pipeline dashboard outputs target ${dashboardRef}. Create requires at least one matching dashboard-output pipeline.`
          : `No pipeline dashboard outputs target ${dashboardRef}.`}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {required && !pipelines.some(pipeline => selectedIDs.includes(pipeline.id)) ? (
        <p className="text-xs font-medium text-amber-700 dark:text-amber-200">
          Select at least one matching dashboard-output pipeline before saving.
        </p>
      ) : null}
      <div className="grid gap-3 lg:grid-cols-2">
        {pipelines.map(pipeline => {
          const checked = selectedIDs.includes(pipeline.id);
          const sections = pipelineSections(pipeline.outputs);
          const runScope = normalizeRunScope(scopeByPipelineID[pipeline.id]);
          const runScopeOptions = scopeSelectOptions(scopeOptions, runScope);
          return (
            <div
              key={pipeline.id}
              className={`rounded-lg border p-3 transition-colors ${
                checked
                  ? 'border-[var(--accent)] bg-[var(--bg-active)]'
                  : 'border-[var(--border-subtle)] bg-[var(--bg-primary)] hover:bg-[var(--bg-tertiary)]'
              }`}
            >
              <label className="flex cursor-pointer items-start gap-3">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={checked}
                  onChange={() => onToggle(pipeline.id)}
                  disabled={saving}
                />
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 items-center gap-2">
                    <Workflow className="h-4 w-4 shrink-0 text-[var(--text-secondary)]" aria-hidden="true" />
                    <span className="truncate text-sm font-semibold text-[var(--text-primary)]">{pipeline.id}</span>
                  </div>
                  <div className="mt-2 flex flex-wrap gap-2 text-xs text-[var(--text-secondary)]">
                    <span className="rounded-md bg-[var(--bg-secondary)] px-2 py-1">{pipeline.outputs.length} outputs</span>
                    {sections.map(section => (
                      <span key={section} className="rounded-md bg-[var(--bg-secondary)] px-2 py-1">{section}</span>
                    ))}
                  </div>
                </div>
              </label>
              {checked ? (
                <div className="mt-3 border-t border-[var(--border-subtle)] pt-3">
                  <Field label="Run scope" description="Only pipeline runs with this exact scope can publish to these dashboard entries.">
                    <select
                      className="pipelines-input w-full"
                      value={runScope}
                      onChange={event => onScopeChange(pipeline.id, event.target.value)}
                      disabled={saving}
                      aria-label={`Run scope for ${pipeline.id}`}
                    >
                      {runScopeOptions.map(option => <option key={option.value || '__default_scope__'} value={option.value}>{option.label}</option>)}
                    </select>
                  </Field>
                </div>
              ) : null}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function SectionManager({
  sections,
  saving,
  onEditSection,
  onDeleteSection,
}: {
  sections: DashboardSection[];
  saving: boolean;
  onEditSection?: (section: DashboardSection) => void;
  onDeleteSection?: (section: DashboardSection) => void;
}) {
  return (
    <FormSection
      title="Sections"
      description="Pipeline dashboard outputs create sections automatically; adjust labels or remove stale areas here."
    >
      <div className="space-y-3">
        {sections.length === 0 ? (
          <div className="rounded-lg border border-dashed border-[var(--border-subtle)] bg-[var(--bg-primary)] px-3 py-4 text-sm text-[var(--text-secondary)]">
            No sections yet.
          </div>
        ) : (
          <div className="grid gap-2">
            {sections.map(section => (
              <div key={section.id || section.section_key} className="flex flex-col gap-3 rounded-lg bg-[var(--bg-primary)] px-3 py-3 md:flex-row md:items-center md:justify-between">
                <div className="min-w-0">
                  <div className="truncate text-sm font-semibold text-[var(--text-primary)]">{section.title || section.section_key}</div>
                  <div className="mt-1 flex flex-wrap gap-2 text-xs text-[var(--text-muted)]">
                    <span>{section.section_key}</span>
                    <span>Order {section.display_order ?? 0}</span>
                  </div>
                  {section.description ? <div className="mt-1 truncate text-sm text-[var(--text-secondary)]">{section.description}</div> : null}
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <SectionIconButton label={`Edit section ${section.title || section.section_key}`} icon={<Pencil className="h-4 w-4" />} onClick={() => onEditSection?.(section)} disabled={saving || !onEditSection} />
                  <SectionIconButton label={`Delete section ${section.title || section.section_key}`} icon={<Trash2 className="h-4 w-4" />} onClick={() => onDeleteSection?.(section)} disabled={saving || !onDeleteSection} danger />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </FormSection>
  );
}

export function SectionModal({
  modal,
  form,
  saving,
  error,
  onChange,
  onClose,
  onSubmit,
}: {
  modal: SectionModalState;
  form: DashboardSectionFormState;
  saving: boolean;
  error: string | null;
  onChange: (next: DashboardSectionFormState) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  const isCreate = modal.mode === 'create';
  const title = isCreate ? 'New section' : 'Edit section';
  const canSubmit = Boolean(form.sectionKey.trim()) && !saving;
  const errorID = error ? 'dashboard-section-form-error' : undefined;

  return (
    <WorkflowFormDialog
      id={isCreate ? 'dashboard-section-new-modal' : 'dashboard-section-edit-modal'}
      titleId="dashboard-section-form-title"
      descriptionId={errorID}
      kicker="Section"
      title={title}
      subtitle={isCreate ? 'Add a focused area to the selected dashboard.' : 'Update how this dashboard area appears.'}
      headerLeading={<ModalIcon icon={<ObjectIcon type="dashboard" className="h-4 w-4" />} />}
      onClose={onClose}
      onSubmit={event => submitForm(event, onSubmit)}
      closeDisabled={saving}
      size="xwide"
      bodyClassName="space-y-4"
      actions={(
        <>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={saving}>
            <X className="h-4 w-4" aria-hidden="true" />
            Cancel
          </button>
          <button type="submit" className="glass-button-primary" disabled={!canSubmit}>
            <Save className="h-4 w-4" aria-hidden="true" />
            {saving ? 'Saving...' : 'Save section'}
          </button>
        </>
      )}
    >
      <FormSection
        title="Section identity"
        description="Sections group related dashboard entries and give pipeline outputs a clear target."
      >
        <div className="grid gap-4 md:grid-cols-[minmax(180px,0.8fr)_minmax(220px,1fr)_minmax(120px,0.45fr)]">
          <Field label="Section" description={isCreate ? 'Stable key used by dashboard outputs.' : 'Stable key; create a new section to use a different key.'}>
            <input
              className="pipelines-input w-full"
              value={form.sectionKey}
              onChange={event => onChange({ ...form, sectionKey: event.target.value })}
              disabled={saving || !isCreate}
              required
              data-dialog-initial-focus
            />
          </Field>
          <Field label="Title" description="Human-friendly heading displayed on the dashboard.">
            <input
              className="pipelines-input w-full"
              value={form.title}
              onChange={event => onChange({ ...form, title: event.target.value })}
              disabled={saving}
              placeholder="Defaults from section key"
            />
          </Field>
          <Field label="Order" description="Lower numbers appear first.">
            <input
              className="pipelines-input w-full"
              type="number"
              value={form.displayOrder}
              onChange={event => onChange({ ...form, displayOrder: event.target.value })}
              disabled={saving}
            />
          </Field>
          <Field label="Description" description="Optional one-line note shown under the section title." wide>
            <input
              className="pipelines-input w-full"
              value={form.description}
              onChange={event => onChange({ ...form, description: event.target.value })}
              disabled={saving}
              placeholder="What should operators expect in this section?"
            />
          </Field>
        </div>
      </FormSection>
      {error ? <p id={errorID} className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700" role="alert">{error}</p> : null}
    </WorkflowFormDialog>
  );
}

export function SourceModal({
  modal,
  form,
  dashboardRef,
  sections,
  sources,
  publications,
  pipelines,
  scopeOptions = [''],
  saving,
  error,
  loadPipelineOutputs,
  onChange,
  onClose,
  onSubmit,
}: {
  modal: SourceModalState;
  form: DashboardSourceFormState;
  dashboardRef: string;
  sections: DashboardSection[];
  sources: DashboardSource[];
  publications: DashboardPublication[];
  pipelines: string[];
  scopeOptions?: string[];
  saving: boolean;
  error: string | null;
  loadPipelineOutputs: (pipelineID: string) => Promise<DashboardPipelineOutputOption[]>;
  onChange: (next: DashboardSourceFormState) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  const [outputs, setOutputs] = useState<DashboardPipelineOutputOption[]>([]);
  const [outputLoading, setOutputLoading] = useState(false);
  const [outputError, setOutputError] = useState<string | null>(null);

  useEffect(() => {
    const pipelineID = form.pipelineID.trim();
    let cancelled = false;
    void Promise.resolve().then(async () => {
      if (cancelled) return;
      if (!pipelineID) {
        setOutputs([]);
        setOutputLoading(false);
        setOutputError(null);
        return;
      }
      setOutputLoading(true);
      setOutputError(null);
      try {
        const next = await loadPipelineOutputs(pipelineID);
        if (!cancelled) setOutputs(next);
      } catch (err) {
        if (!cancelled) {
          setOutputs([]);
          setOutputError(err instanceof Error ? err.message : 'Unable to load outputs');
        }
      } finally {
        if (!cancelled) setOutputLoading(false);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [form.pipelineID, loadPipelineOutputs]);

  const sectionOptions = useMemo(() => sectionSelectOptions(sections, form.sectionKey), [sections, form.sectionKey]);
  const pipelineOptions = useMemo(() => uniqueOptions([form.pipelineID, ...pipelines].filter(Boolean)), [form.pipelineID, pipelines]);
  const runScopeOptions = useMemo(() => scopeSelectOptions(scopeOptions, form.runScope), [form.runScope, scopeOptions]);
  const outputOptions = useMemo(
    () => outputSelectOptions(outputs, form.outputName, dashboardRef, sectionOptions.map(option => option.value)),
    [dashboardRef, form.outputName, outputs, sectionOptions]
  );
  const selectedOutput = outputOptions.find(output => output.name === form.outputName);
  const entryOptions = useMemo(
    () => buildDashboardEntryOptions({
      output: selectedOutput,
      outputName: form.outputName,
      currentEntryKey: form.entryKey,
      existingEntryKeys: existingEntryKeys(form.sectionKey, sources, publications),
    }),
    [form.entryKey, form.outputName, form.sectionKey, publications, selectedOutput, sources]
  );
  const canSubmit = Boolean(form.sectionKey.trim() && form.pipelineID.trim() && form.outputName.trim()) && !saving;
  const noMatchingOutputs = Boolean(form.pipelineID && !outputLoading && !outputError && outputOptions.length === 0);
  const title = modal.mode === 'create' ? 'New source' : 'Edit source';
  const errorID = error ? 'dashboard-source-form-error' : undefined;

  const changeOutput = (outputName: string) => {
    const output = outputOptions.find(item => item.name === outputName);
    const targetSection =
      output?.sectionKey && sectionOptions.some(option => option.value === output.sectionKey)
        ? output.sectionKey
        : form.sectionKey;
    onChange({
      ...form,
      outputName,
      sectionKey: targetSection,
      entryKey: output?.entryKey || '',
    });
  };

  return (
    <WorkflowFormDialog
      id={modal.mode === 'create' ? 'dashboard-source-new-modal' : 'dashboard-source-edit-modal'}
      titleId="dashboard-source-form-title"
      descriptionId={errorID}
      kicker="Source"
      title={title}
      subtitle={dashboardRef}
      headerLeading={<ModalIcon icon={<PlugZap className="h-4 w-4" aria-hidden="true" />} />}
      onClose={onClose}
      onSubmit={event => submitForm(event, onSubmit)}
      closeDisabled={saving}
      size="xwide"
      bodyClassName="space-y-4"
      actions={(
        <>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={saving}>
            <X className="h-4 w-4" aria-hidden="true" />
            Cancel
          </button>
          <button type="submit" className="glass-button-primary" disabled={!canSubmit}>
            <Save className="h-4 w-4" aria-hidden="true" />
            {saving ? 'Saving...' : 'Save source'}
          </button>
        </>
      )}
    >
      <FormSection
        title="Source mapping"
        description="Pick the dashboard section first, then bind it to a dashboard output declared by a pipeline."
      >
        <div className="modal-property-grid">
          <Field label="Section" description="Dashboard section that receives this output.">
            <select
              className="pipelines-input w-full"
              value={form.sectionKey}
              onChange={event => onChange({ ...form, sectionKey: event.target.value })}
              disabled={saving || sectionOptions.length === 0}
              data-dialog-initial-focus
            >
              <option value="" disabled>Select section</option>
              {sectionOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          <Field label="Pipeline" description="Existing pipeline whose YAML declares dashboard outputs.">
            <select
              className="pipelines-input w-full"
              value={form.pipelineID}
              onChange={event => onChange({ ...form, pipelineID: event.target.value, outputName: '', entryKey: '' })}
              disabled={saving || pipelineOptions.length === 0}
            >
              <option value="" disabled>{pipelineOptions.length === 0 ? 'No pipelines available' : 'Select pipeline'}</option>
              {pipelineOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          <Field label="Output" description="Dashboard output item from the selected pipeline.">
            <select
              className="pipelines-input w-full"
              value={form.outputName}
              onChange={event => changeOutput(event.target.value)}
              disabled={saving || !form.pipelineID || outputLoading || outputOptions.length === 0}
            >
              <option value="" disabled>
                {outputLoading ? 'Loading outputs' : noMatchingOutputs ? 'No matching outputs' : 'Select output'}
              </option>
              {outputOptions.map(output => <option key={output.name} value={output.name}>{output.name}</option>)}
            </select>
          </Field>
          <Field label="Entry" description="Stable entry key inside the section; leaving it empty uses the output name.">
            <select
              className="pipelines-input w-full"
              value={form.entryKey}
              onChange={event => onChange({ ...form, entryKey: event.target.value })}
              disabled={saving || !form.outputName || entryOptions.length === 0}
            >
              {entryOptions.map(option => <option key={option.value || '__output_name__'} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          <Field label="Run scope" description="Only successful runs with this exact scope can publish through this source.">
            <select
              className="pipelines-input w-full"
              value={normalizeRunScope(form.runScope)}
              onChange={event => onChange({ ...form, runScope: event.target.value })}
              disabled={saving}
            >
              {runScopeOptions.map(option => <option key={option.value || '__default_scope__'} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
        </div>
      </FormSection>

      <FormSection
        title="Refresh behavior"
        description="Control whether this source participates in manual and scheduled refreshes."
      >
        <div className="modal-property-grid">
          <Field label="Refresh order" description="Lower numbers run earlier when multiple sources refresh.">
            <input
              className="pipelines-input w-full"
              type="number"
              value={form.refreshOrder}
              onChange={event => onChange({ ...form, refreshOrder: event.target.value })}
              disabled={saving}
            />
          </Field>
          <div className="flex items-end gap-4">
            <label className="flex items-center gap-2 text-sm text-[var(--text-primary)]">
              <input type="checkbox" checked={form.enabled} onChange={event => onChange({ ...form, enabled: event.target.checked })} disabled={saving} />
              Enabled
            </label>
            <label className="flex items-center gap-2 text-sm text-[var(--text-primary)]">
              <input type="checkbox" checked={form.requiredForRefresh} onChange={event => onChange({ ...form, requiredForRefresh: event.target.checked })} disabled={saving} />
              Required
            </label>
          </div>
        </div>
      </FormSection>

      {outputError ? <p className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700" role="alert">{outputError}</p> : null}
      {noMatchingOutputs ? (
        <p className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm text-amber-800">
          The selected pipeline has no dashboard outputs targeting {dashboardRef} and its sections.
        </p>
      ) : null}
      {error ? <p id={errorID} className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700" role="alert">{error}</p> : null}
      <SourceMappingPreview
        dashboardRef={dashboardRef}
        sectionKey={form.sectionKey}
        pipelineID={form.pipelineID}
        outputName={form.outputName}
        entryKey={form.entryKey}
        runScope={form.runScope}
        selectedOutput={selectedOutput}
      />
    </WorkflowFormDialog>
  );
}

export function RefreshModal({
  title,
  form,
  sections,
  saving,
  error,
  onChange,
  onClose,
  onSubmit,
}: {
  title: string;
  form: DashboardRefreshFormState;
  sections: DashboardSection[];
  saving: boolean;
  error: string | null;
  onChange: (next: DashboardRefreshFormState) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  const sectionOptions = useMemo(() => sectionSelectOptions(sections, form.sectionKey), [form.sectionKey, sections]);
  const canSubmit =
    !saving &&
    (form.scopeType === 'dashboard' ||
      (form.scopeType === 'section' && Boolean(form.sectionKey.trim())));
  const errorID = error ? 'dashboard-refresh-form-error' : undefined;

  return (
    <WorkflowFormDialog
      id="dashboard-refresh-modal"
      titleId="dashboard-refresh-form-title"
      descriptionId={errorID}
      kicker="Refresh"
      title={title}
      subtitle="Run the selected dashboard source bindings now. Required sources affect strict-mode completion; optional sources are still recorded in refresh history."
      headerLeading={<ModalIcon icon={<RefreshCw className="h-4 w-4" aria-hidden="true" />} />}
      onClose={onClose}
      onSubmit={event => submitForm(event, onSubmit)}
      closeDisabled={saving}
      size="xwide"
      bodyClassName="space-y-4"
      actions={(
        <>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={saving}>
            <X className="h-4 w-4" aria-hidden="true" />
            Cancel
          </button>
          <button type="submit" className="glass-button-primary" disabled={!canSubmit}>
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
            {saving ? 'Starting...' : 'Start'}
          </button>
        </>
      )}
    >
      <FormSection
        title="Refresh target"
        description="Choose whether to refresh the whole dashboard or one generated section."
      >
        <div className="modal-property-grid">
          <Field label="Scope" description="Dashboard runs all enabled bindings; section limits the refresh to that section's outputs.">
            <select
              className="pipelines-input w-full"
              value={form.scopeType}
              onChange={event => onChange({ ...form, scopeType: event.target.value as DashboardRefreshFormState['scopeType'] })}
              disabled={saving}
              data-dialog-initial-focus
            >
              <option value="dashboard">Dashboard</option>
              <option value="section">Section</option>
            </select>
          </Field>
          <Field label="Mode" description="Strict fails when required sources cannot complete; best effort keeps usable partial results.">
            <select
              className="pipelines-input w-full"
              value={form.mode}
              onChange={event => onChange({ ...form, mode: event.target.value as DashboardRefreshFormState['mode'] })}
              disabled={saving}
            >
              <option value="strict">Strict</option>
              <option value="best_effort">Best effort</option>
            </select>
          </Field>
          {form.scopeType === 'section' ? (
            <Field label="Section" description="Only source bindings attached to this dashboard section will run.">
              <select
                className="pipelines-input w-full"
                value={form.sectionKey}
                onChange={event => onChange({ ...form, sectionKey: event.target.value })}
                disabled={saving || sectionOptions.length === 0}
              >
                <option value="" disabled>Select section</option>
                {sectionOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
          ) : null}
        </div>
      </FormSection>
      <FormSection
        title="Execution guardrails"
        description="Keep refreshes bounded so dashboard updates stay predictable for operators."
      >
        <div className="modal-property-grid">
          <Field label="Timeout" description="Maximum wall-clock duration for this refresh request, for example 45m or 1h.">
            <input className="pipelines-input w-full" value={form.timeout} onChange={event => onChange({ ...form, timeout: event.target.value })} disabled={saving} />
          </Field>
          <Field label="Concurrency" description="Maximum number of unique pipeline and scope refresh runs that can start at the same time.">
            <input className="pipelines-input w-full" type="number" min="1" max="16" value={form.maxConcurrency} onChange={event => onChange({ ...form, maxConcurrency: event.target.value })} disabled={saving} />
          </Field>
        </div>
      </FormSection>
      {error ? <p id={errorID} className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700" role="alert">{error}</p> : null}
    </WorkflowFormDialog>
  );
}

export function RefreshScheduleModal({
  modal,
  form,
  sections,
  saving,
  error,
  onChange,
  onClose,
  onSubmit,
}: {
  modal: RefreshScheduleModalState;
  form: DashboardRefreshScheduleFormState;
  sections: DashboardSection[];
  saving: boolean;
  error: string | null;
  onChange: (next: DashboardRefreshScheduleFormState) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  const update = (patch: Partial<DashboardRefreshScheduleFormState>) => onChange({ ...form, ...patch });
  const updateCron = (patch: Partial<CronFormFields>) => {
    const next = { ...form, ...patch };
    onChange({
      ...next,
      cron_expression: next.cronMode === 'custom' ? next.cron_expression : refreshScheduleCronExpressionFromForm(next),
    });
  };
  const sectionOptions = useMemo(() => sectionSelectOptions(sections, form.sectionKey), [form.sectionKey, sections]);
  const selectedWeekdays = new Set(normalizeCronList(form.cronWeekday, WEEKDAY_VALUES, '1').split(','));
  const selectedMonthdays = new Set(normalizeCronList(form.cronMonthday, MONTHDAY_VALUES, '1').split(','));
  const canSubmit =
    !saving &&
    Boolean(form.name.trim() && form.cron_expression.trim()) &&
    (form.scopeType === 'dashboard' ||
      (form.scopeType === 'section' && Boolean(form.sectionKey.trim())));
  const isCreate = modal.mode === 'create';
  const title = isCreate ? 'Schedule refresh' : 'Edit refresh schedule';
  const errorID = error ? 'dashboard-refresh-schedule-form-error' : undefined;

  return (
    <WorkflowFormDialog
      id={isCreate ? 'dashboard-refresh-schedule-new-modal' : 'dashboard-refresh-schedule-edit-modal'}
      titleId="dashboard-refresh-schedule-form-title"
      descriptionId={errorID}
      kicker="Schedule"
      title={title}
      subtitle="Run dashboard source bindings on a recurring cadence with the same refresh guardrails used for manual refreshes."
      headerLeading={<ModalIcon icon={<CalendarClock className="h-4 w-4" aria-hidden="true" />} />}
      onClose={onClose}
      onSubmit={event => submitForm(event, onSubmit)}
      closeDisabled={saving}
      size="xwide"
      bodyClassName="space-y-4"
      actions={(
        <>
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={saving}>
            <X className="h-4 w-4" aria-hidden="true" />
            Cancel
          </button>
          <button type="submit" className="glass-button-primary" disabled={!canSubmit}>
            <Save className="h-4 w-4" aria-hidden="true" />
            {saving ? 'Saving...' : 'Save schedule'}
          </button>
        </>
      )}
    >
      <FormSection
        title="Schedule identity"
        description="Give this recurring refresh a stable name and short operator-facing purpose."
      >
        <div className="modal-property-grid">
          <Field label="Name" description="Stable unique key for this dashboard schedule. Use letters, numbers, dots, underscores, or hyphens.">
            <input
              className="pipelines-input w-full"
              value={form.name}
              onChange={event => update({ name: event.target.value })}
              disabled={saving}
              required
              placeholder="hourly-health"
              data-dialog-initial-focus
            />
          </Field>
          <div className="flex items-end">
            <label className="flex min-h-10 w-full items-center gap-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={event => update({ enabled: event.target.checked })}
                disabled={saving}
                className="h-4 w-4 rounded border-[var(--border-primary)]"
              />
              <span className="text-sm font-semibold text-[var(--text-primary)]">Enabled</span>
            </label>
          </div>
          <Field label="Description" description="One-line explanation shown with the schedule details." wide>
            <input
              className="pipelines-input w-full"
              value={form.description}
              onChange={event => update({ description: event.target.value })}
              disabled={saving}
              placeholder="Refresh service health before the morning review"
            />
          </Field>
        </div>
      </FormSection>

      <FormSection
        title="Cadence"
        description="Use the same frequency builder as pipeline schedules, then review the generated cron expression if you need a custom cadence."
      >
        <div className="modal-property-grid">
          <Field label="Frequency" description="Choose a guided cadence or switch to custom cron.">
            <select
              className="pipelines-input w-full"
              value={form.cronMode}
              onChange={event => updateCron({ cronMode: event.target.value as Exclude<CronMode, 'once'> })}
              disabled={saving}
            >
              {SCHEDULE_FREQUENCY_OPTIONS.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </Field>
          {form.cronMode === 'minutes' ? (
            <Field label="Every" description="Run every N minutes.">
              <div className="flex items-center gap-2">
                <input
                  className="pipelines-input w-full"
                  type="number"
                  min="1"
                  max="59"
                  value={form.intervalValue}
                  onChange={event => updateCron({ intervalValue: event.target.value })}
                  disabled={saving}
                />
                <span className="text-sm text-[var(--text-secondary)]">minutes</span>
              </div>
            </Field>
          ) : null}
          {form.cronMode === 'hourly' ? (
            <>
              <Field label="Every" description="Run every N hours.">
                <div className="flex items-center gap-2">
                  <input
                    className="pipelines-input w-full"
                    type="number"
                    min="1"
                    max="23"
                    value={form.intervalValue}
                    onChange={event => updateCron({ intervalValue: event.target.value })}
                    disabled={saving}
                  />
                  <span className="text-sm text-[var(--text-secondary)]">hours</span>
                </div>
              </Field>
              <Field label="Minute" description="Minute within the selected hour.">
                <input
                  className="pipelines-input w-full"
                  type="number"
                  min="0"
                  max="59"
                  value={form.cronMinute}
                  onChange={event => updateCron({ cronMinute: event.target.value })}
                  disabled={saving}
                />
              </Field>
            </>
          ) : null}
          {form.cronMode === 'weekly' ? (
            <fieldset className="space-y-2 md:col-span-2">
              <legend className="modal-property-label">Days</legend>
              <div className="modal-chip-list">
                {WEEKDAY_OPTIONS.map(option => (
                  <label key={option.value} className="modal-chip">
                    <input
                      type="checkbox"
                      checked={selectedWeekdays.has(option.value)}
                      onChange={() => updateCron({ cronWeekday: toggleCronListValue(form.cronWeekday, option.value, WEEKDAY_VALUES, '1') })}
                      disabled={saving}
                    />
                    <span>{option.short}</span>
                  </label>
                ))}
              </div>
            </fieldset>
          ) : null}
          {form.cronMode === 'monthly' ? (
            <fieldset className="space-y-2 md:col-span-2">
              <legend className="modal-property-label">Days of month</legend>
              <div className="modal-chip-list">
                {MONTHDAY_VALUES.map(day => (
                  <label key={day} className="modal-chip">
                    <input
                      type="checkbox"
                      checked={selectedMonthdays.has(day)}
                      onChange={() => updateCron({ cronMonthday: toggleCronListValue(form.cronMonthday, day, MONTHDAY_VALUES, '1') })}
                      disabled={saving}
                    />
                    <span>{day}</span>
                  </label>
                ))}
              </div>
            </fieldset>
          ) : null}
          {form.cronMode === 'yearly' ? (
            <>
              <Field label="Month" description="Month of the yearly refresh.">
                <select
                  className="pipelines-input w-full"
                  value={form.cronMonth}
                  onChange={event => updateCron({ cronMonth: event.target.value })}
                  disabled={saving}
                >
                  {MONTH_OPTIONS.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
                </select>
              </Field>
              <Field label="Day" description="Day of the selected month.">
                <select
                  className="pipelines-input w-full"
                  value={form.cronMonthday}
                  onChange={event => updateCron({ cronMonthday: event.target.value })}
                  disabled={saving}
                >
                  {Array.from({ length: 31 }, (_, index) => String(index + 1)).map(day => <option key={day} value={day}>{day}</option>)}
                </select>
              </Field>
            </>
          ) : null}
          {form.cronMode !== 'minutes' && form.cronMode !== 'hourly' && form.cronMode !== 'custom' ? (
            <Field label="Time" description="Local time in the selected timezone.">
              <input
                className="pipelines-input w-full"
                type="time"
                value={form.cronTime}
                onChange={event => updateCron({ cronTime: event.target.value })}
                disabled={saving}
              />
            </Field>
          ) : null}
          {form.cronMode === 'custom' ? (
            <Field label="Expression" description="Five-field cron expression evaluated in the selected timezone.">
              <input
                className="pipelines-input w-full font-mono"
                value={form.cron_expression}
                onChange={event => updateCron({ cron_expression: event.target.value })}
                disabled={saving}
                required
                placeholder="0 * * * *"
              />
            </Field>
          ) : null}
          <Field label="Timezone" description="IANA timezone used to calculate the next run time.">
            <input
              list="dashboard-schedule-timezone-options"
              className="pipelines-input w-full"
              value={form.timezone}
              onChange={event => update({ timezone: event.target.value })}
              disabled={saving}
              placeholder="UTC"
            />
            <datalist id="dashboard-schedule-timezone-options">
              {TIMEZONE_OPTIONS.map(zone => <option key={zone} value={zone} />)}
            </datalist>
          </Field>
          <Field label="Cron preview" description="This is the exact expression saved for the dashboard refresh worker.">
            <input className="pipelines-input w-full font-mono" value={form.cron_expression} readOnly disabled />
          </Field>
        </div>
      </FormSection>

      <FormSection
        title="Refresh target"
        description="Schedule the whole dashboard or narrow the recurring refresh to one section. Individual output cards are published by their pipeline run and are not scheduled independently."
      >
        <div className="modal-property-grid">
          <Field label="Scope" description="Dashboard runs all enabled bindings; section limits the scheduled work without pretending one output card can run independently.">
            <select
              className="pipelines-input w-full"
              value={form.scopeType}
              onChange={event => onChange({ ...form, scopeType: event.target.value as DashboardRefreshScheduleFormState['scopeType'] })}
              disabled={saving}
            >
              <option value="dashboard">Dashboard</option>
              <option value="section">Section</option>
            </select>
          </Field>
          <Field label="Mode" description="Strict fails when required sources cannot complete; best effort keeps partial usable results.">
            <select
              className="pipelines-input w-full"
              value={form.mode}
              onChange={event => onChange({ ...form, mode: event.target.value as DashboardRefreshScheduleFormState['mode'] })}
              disabled={saving}
            >
              <option value="strict">Strict</option>
              <option value="best_effort">Best effort</option>
            </select>
          </Field>
          {form.scopeType === 'section' ? (
            <Field label="Section" description="Only bindings attached to this dashboard section will refresh.">
              <select
                className="pipelines-input w-full"
                value={form.sectionKey}
                onChange={event => onChange({ ...form, sectionKey: event.target.value })}
                disabled={saving || sectionOptions.length === 0}
              >
                <option value="" disabled>Select section</option>
                {sectionOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
          ) : null}
        </div>
      </FormSection>

      <FormSection
        title="Execution guardrails"
        description="Bound the scheduled refresh so recurring dashboard updates remain predictable."
      >
        <div className="modal-property-grid">
          <Field label="Timeout" description="Maximum wall-clock duration for each scheduled refresh, for example 45m or 1h.">
            <input className="pipelines-input w-full" value={form.timeout} onChange={event => onChange({ ...form, timeout: event.target.value })} disabled={saving} />
          </Field>
          <Field label="Concurrency" description="Maximum number of unique pipeline and scope refresh runs that can start at the same time.">
            <input className="pipelines-input w-full" type="number" min="1" max="16" value={form.maxConcurrency} onChange={event => onChange({ ...form, maxConcurrency: event.target.value })} disabled={saving} />
          </Field>
        </div>
      </FormSection>

      {error ? <p id={errorID} className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700" role="alert">{error}</p> : null}
    </WorkflowFormDialog>
  );
}

export function DashboardDeleteModal({
  modal,
  saving,
  error,
  onClose,
  onConfirm,
}: {
  modal: DashboardDeleteModalState;
  saving: boolean;
  error: string | null;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const titleId = 'dashboard-delete-title';
  const descriptionId = 'dashboard-delete-description';
  const errorId = error ? 'dashboard-delete-error' : undefined;
  const target = deleteTargetCopy(modal);

  return (
    <WorkflowDialogFrame
      id="dashboard-delete-modal"
      role="alertdialog"
      titleId={titleId}
      descriptionId={`${descriptionId}${errorId ? ` ${errorId}` : ''}`}
      onClose={saving ? () => undefined : onClose}
      className="pipelines-modal-card workflow-form-dialog workflow-form-dialog--wide w-full"
    >
      <header className="pipelines-modal-header">
        <div className="flex min-w-0 items-center gap-3">
          <ModalIcon icon={<AlertTriangle className="h-4 w-4 text-rose-500" aria-hidden="true" />} />
          <div className="min-w-0">
            <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Delete {target.kicker}</p>
            <h3 id={titleId} className="text-lg font-semibold text-[var(--text-primary)]">
              Remove {target.name}?
            </h3>
          </div>
        </div>
        <WorkflowDialogCloseButton onClose={onClose} disabled={saving} />
      </header>
      <div className="pipelines-modal-body space-y-4">
        <FormSection title={target.sectionTitle} description={target.description}>
          <p id={descriptionId} className="text-sm text-[var(--text-secondary)]">
            {target.impact}
          </p>
        </FormSection>
        {error ? <WorkflowInlineAlert id={errorId} className="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700">{error}</WorkflowInlineAlert> : null}
      </div>
      <footer className="pipelines-modal-footer">
        <div className="pipelines-modal-actions">
          <button type="button" className="glass-button-ghost" onClick={onClose} disabled={saving} data-dialog-initial-focus>
            Cancel
          </button>
          <button type="button" className="glass-button-danger" onClick={onConfirm} disabled={saving}>
            {saving ? 'Deleting...' : 'Delete'}
          </button>
        </div>
      </footer>
    </WorkflowDialogFrame>
  );
}

function SourceMappingPreview({
  dashboardRef,
  sectionKey,
  pipelineID,
  outputName,
  entryKey,
  runScope,
  selectedOutput,
}: {
  dashboardRef: string;
  sectionKey: string;
  pipelineID: string;
  outputName: string;
  entryKey: string;
  runScope: string;
  selectedOutput?: DashboardPipelineOutputOption;
}) {
  const entryLabel = entryKey || (outputName ? `Use output name (${outputName})` : '-');
  const rows = [
    ['Dashboard', dashboardRef || '-'],
    ['Section', sectionKey || '-'],
    ['Entry', entryLabel],
  ];
  const outputRows = [
    ['Pipeline', pipelineID || '-'],
    ['Run scope', runScopeLabel(runScope)],
    ['Output', outputName || '-'],
    ['Mode', selectedOutput?.mode || '-'],
    ['Preset', selectedOutput?.preset || '-'],
    ['When', selectedOutput?.when || '-'],
    ['TTL', selectedOutput?.ttl || '-'],
  ];
  return (
    <div className="rounded-lg border border-[var(--border-subtle)] bg-[var(--bg-primary)] p-4">
      <div className="flex flex-col gap-1 border-b border-[var(--border-subtle)] pb-3">
        <div className="text-sm font-semibold text-[var(--text-primary)]">Mapping review</div>
        <p className="text-sm text-[var(--text-secondary)]">
          Confirm where the selected pipeline output will publish inside this dashboard.
        </p>
      </div>
      <div className="mt-4 grid gap-4 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
        <div className="rounded-lg bg-[var(--bg-secondary)] p-4">
          <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Dashboard target</div>
          <div className="mt-3 grid gap-3">
            {rows.map(([label, value]) => (
              <ReviewValue key={label} label={label} value={value} />
            ))}
          </div>
        </div>
        <div className="rounded-lg bg-[var(--bg-secondary)] p-4">
          <div className="text-xs font-semibold uppercase text-[var(--text-secondary)]">Pipeline output</div>
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            {outputRows.map(([label, value]) => (
              <ReviewValue key={label} label={label} value={value} />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function deleteTargetCopy(modal: DashboardDeleteModalState): {
  kicker: string;
  name: string;
  sectionTitle: string;
  description: string;
  impact: string;
} {
  if (modal.kind === 'dashboard') {
    const name = modal.dashboard.ref || modal.dashboard.title || modal.dashboard.id;
    return {
      kicker: 'dashboard',
      name,
      sectionTitle: 'Dashboard removal',
      description: 'Deletes the dashboard record and its generated runtime state.',
      impact: 'Sections, source bindings, current publications, history, refresh records, and schedules for this dashboard are removed. GitOps-managed dashboards can be recreated by the next config sync.',
    };
  }
  if (modal.kind === 'section') {
    const name = modal.section.title || modal.section.section_key;
    return {
      kicker: 'section',
      name,
      sectionTitle: 'Section removal',
      description: 'Deletes this dashboard section and the runtime data attached to that section.',
      impact: 'Source bindings, current publications, publication history, and section-scoped refresh schedules for this section are removed. Pipeline outputs can recreate the section when assigned again.',
    };
  }
  if (modal.kind === 'schedule') {
    return {
      kicker: 'schedule',
      name: modal.schedule.name,
      sectionTitle: 'Scheduled refresh removal',
      description: 'Deletes this recurring refresh schedule from the selected dashboard.',
      impact: 'Future automated refreshes for this schedule stop immediately. Existing refresh records, publications, and pipeline runs remain available for audit.',
    };
  }
  if (modal.kind === 'publication') {
    const name = modal.publication.content.title || modal.publication.entry_key;
    return {
      kicker: 'entry',
      name,
      sectionTitle: 'Dashboard entry removal',
      description: 'Removes this visible dashboard card from its section.',
      impact: 'The current publication is archived and a removal event is written to history. Source bindings, refresh records, and pipeline runs remain available for audit, and a future refresh can publish the entry again.',
    };
  }
  return {
    kicker: 'source',
    name: `${modal.source.pipeline_id}/${modal.source.output_name}`,
    sectionTitle: 'Source binding removal',
    description: 'Deletes this dashboard source binding from the selected section.',
    impact: 'The pipeline output will no longer refresh this dashboard entry. Existing publications remain visible until replaced, removed from the section card, or cleared by section/dashboard cleanup.',
  };
}

function ReviewValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border border-[var(--border-subtle)] bg-[var(--bg-primary)] px-3 py-2">
      <div className="text-[11px] uppercase text-[var(--text-muted)]">{label}</div>
      <div className="mt-1 truncate text-sm font-semibold text-[var(--text-primary)]" title={value}>{value}</div>
    </div>
  );
}

function FormSection({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return (
    <section className="space-y-3 rounded-lg bg-[var(--bg-secondary)] p-4">
      <div>
        <h4 className="text-sm font-semibold text-[var(--text-primary)]">{title}</h4>
        <p className="mt-1 text-sm text-[var(--text-secondary)]">{description}</p>
      </div>
      {children}
    </section>
  );
}

function Field({
  label,
  description,
  children,
  wide,
  stacked,
}: {
  label: string;
  description?: string;
  children: ReactNode;
  wide?: boolean;
  stacked?: boolean;
}) {
  return (
    <WorkflowPropertyRow
      label={label}
      hint={description}
      span={wide ? 'full' : 'half'}
      layout={stacked ? 'stacked' : 'inline'}
    >
      {children}
    </WorkflowPropertyRow>
  );
}

function SectionIconButton({
  label,
  icon,
  onClick,
  disabled,
  danger,
}: {
  label: string;
  icon: ReactNode;
  onClick: () => void;
  disabled?: boolean;
  danger?: boolean;
}) {
  const tone = danger
    ? 'bg-rose-50 text-rose-600 hover:bg-rose-100 dark:bg-rose-950/30 dark:text-rose-100'
    : 'bg-[var(--bg-secondary)] text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]';
  return (
    <button
      type="button"
      className={`inline-flex h-9 w-9 items-center justify-center rounded-md transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${tone}`}
      title={label}
      aria-label={label}
      onClick={onClick}
      disabled={disabled}
    >
      {icon}
    </button>
  );
}

function ModalIcon({ icon }: { icon: ReactNode }) {
  return (
    <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-[var(--border-subtle)] bg-[var(--bg-secondary)] text-[var(--text-secondary)]">
      {icon}
    </span>
  );
}

function submitForm(event: FormEvent<HTMLFormElement>, onSubmit: () => void) {
  event.preventDefault();
  onSubmit();
}

function dashboardPipelineOptionsForRef(
  pipelines: DashboardPipelineCatalogItem[],
  dashboardRef: string
): DashboardPipelineCatalogItem[] {
  if (!dashboardRef) return [];
  return pipelines
    .map(pipeline => ({
      id: pipeline.id,
      outputs: pipeline.outputs.filter(output => output.dashboardRef === dashboardRef && Boolean(output.sectionKey.trim())),
    }))
    .filter(pipeline => pipeline.outputs.length > 0)
    .sort((a, b) => a.id.localeCompare(b.id));
}

function pipelineSections(outputs: DashboardPipelineOutputOption[]): string[] {
  return Array.from(new Set(outputs.map(output => output.sectionKey.trim()).filter(Boolean))).sort((a, b) => a.localeCompare(b));
}

function toggleValue(values: string[], value: string): string[] {
  if (values.includes(value)) return values.filter(item => item !== value);
  return [...values, value].sort((a, b) => a.localeCompare(b));
}

function togglePipelineAssignment(form: DashboardFormState, pipelineID: string): DashboardFormState {
  const selected = form.pipelineIDs.includes(pipelineID);
  if (selected) {
    const pipelineScopes = { ...form.pipelineScopes };
    delete pipelineScopes[pipelineID];
    return {
      ...form,
      pipelineIDs: toggleValue(form.pipelineIDs, pipelineID),
      pipelineScopes,
    };
  }
  return {
    ...form,
    pipelineIDs: toggleValue(form.pipelineIDs, pipelineID),
    pipelineScopes: {
      ...form.pipelineScopes,
      [pipelineID]: normalizeRunScope(form.pipelineScopes[pipelineID]),
    },
  };
}

function uniqueOptions(values: string[]): Option[] {
  const seen = new Set<string>();
  return values
    .map(value => value.trim())
    .filter(value => {
      if (!value || seen.has(value)) return false;
      seen.add(value);
      return true;
    })
    .sort((a, b) => a.localeCompare(b))
    .map(value => ({ value, label: value }));
}

function scopeSelectOptions(scopes: string[], currentScope: string): Option[] {
  const values = Array.from(new Set(['', ...scopes, currentScope].map(normalizeRunScope))).sort((a, b) => {
    if (a === '') return -1;
    if (b === '') return 1;
    return a.localeCompare(b);
  });
  return values.map(value => ({ value, label: runScopeLabel(value) }));
}

function sectionSelectOptions(sections: DashboardSection[], currentSectionKey: string): Option[] {
  const options = sections
    .map(section => {
      const value = section.section_key.trim();
      if (!value) return null;
      const label = section.title ? `${section.title} (${value})` : `${titleFromKey(value)} (${value})`;
      return { value, label };
    })
    .filter((option): option is Option => Boolean(option));
  if (currentSectionKey && !options.some(option => option.value === currentSectionKey)) {
    options.push({ value: currentSectionKey, label: `${titleFromKey(currentSectionKey)} (${currentSectionKey})` });
  }
  return options.sort((a, b) => a.label.localeCompare(b.label));
}

function outputSelectOptions(
  outputs: DashboardPipelineOutputOption[],
  currentOutputName: string,
  dashboardRef: string,
  sectionKeys: string[]
): DashboardPipelineOutputOption[] {
  const current = currentOutputName.trim();
  const normalizedDashboardRef = dashboardRef.trim();
  const sectionSet = new Set(sectionKeys);
  const matching = outputs.filter(
    output => output.dashboardRef === normalizedDashboardRef && sectionSet.has(output.sectionKey)
  );
  if (current && !matching.some(output => output.name === current)) {
    matching.push({
      name: current,
      type: 'dashboard',
      when: '',
      dashboardRef: normalizedDashboardRef,
      sectionKey: '',
      entryKey: '',
      mode: '',
      preset: '',
      ttl: '',
    });
  }
  return matching;
}

function existingEntryKeys(
  sectionKey: string,
  sources: DashboardSource[],
  publications: DashboardPublication[]
): string[] {
  return [
    ...sources.filter(source => source.section_key === sectionKey).map(source => source.entry_key || ''),
    ...publications.filter(publication => publication.section_key === sectionKey).map(publication => publication.entry_key),
  ].filter(Boolean);
}
