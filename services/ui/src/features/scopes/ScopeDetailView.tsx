import { useMemo, useState, type ReactNode } from 'react';
import { ArrowLeft, ChevronUp, Clipboard, Copy, Eye, EyeOff, KeyRound, Pencil, Plus, Route, Search, Server, Trash2 } from 'lucide-react';
import { Link } from 'react-router-dom';
import ResourceAccessCard from '../../components/ResourceAccessCard';
import { copyTextToClipboard } from '../../lib/clipboard';
import {
  buildRunnerAssignmentsForScope,
  formatDispatcherRouteScope,
  getRunnerMeta,
  type DispatcherStatusState,
  type Runner,
  type RunnerRouteAssignment,
} from '../system/dispatcher/model';
import {
  buildScopeDetailItems,
  createInitialScopeData,
  filterScopeDetailItems,
  formatScopeDisplay,
  formatScopeTimestamp,
  groupScopeDetailItems,
  isEditableScopeSource,
  isGitOpsScopeSource,
  normalizeScopeLabel,
  parseScopedIdentity,
  scopeSourcePillClass,
  summarizeScopeSourceMix,
  type ScopeData,
  type ScopeDetailItem,
  type ScopeDetailItemFilter,
  type ScopeDetailItemSection,
  type ScopePipelineMeta,
  type ScopeTriggerDescriptor,
} from './model';

type ScopeDetailViewProps = {
  selectedScope: string | null;
  scopeDataByScope: Record<string, ScopeData>;
  selectedVariable: string | null;
  selectedSecret: string | null;
  expandedVariableKey: string | null;
  variableValueLoadingKey: string | null;
  variableValues: Record<string, string>;
  pipelineVariableIndex: Map<string, Set<string>>;
  pipelineSecretIndex: Map<string, Set<string>>;
  pipelineMetadata: Map<string, ScopePipelineMeta>;
  triggersByScope: Map<string, ScopeTriggerDescriptor[]>;
  usageLoading: boolean;
  usageError: string | null;
  runnerStatus?: DispatcherStatusState | null;
  runnerStatusLoading?: boolean;
  runnerStatusError?: string | null;
  canWriteVariablesInSelectedScope: boolean;
  canWriteSecretsInSelectedScope: boolean;
  canDeleteScopes: boolean;
  onSelectVariable: (name: string | null) => void;
  onSelectSecret: (name: string | null) => void;
  onToggleVariableValue: (scopeLabel: string, fullName: string) => Promise<void>;
  onCreateVariable: (scopeLabel: string) => void;
  onUpdateVariable: (scopeLabel: string, fullName: string) => void;
  onCloneVariable: (scopeLabel: string, fullName: string) => void;
  onCreateSecret: (scopeLabel: string) => void;
  onUpdateSecret: (scopeLabel: string, fullName: string) => void;
  onCloneSecret: (scopeLabel: string, fullName: string) => void;
  onDeleteValue: (kind: 'variable' | 'secret', scopeLabel: string, fullName: string) => void;
  onOpenGitOpsEncrypt: () => void;
  onBack: () => void;
};

type RunnerAvailability = 'online' | 'paused' | 'offline';
type ValueCopyState = 'idle' | 'copied' | 'error';

export function ScopeDetailView({
  selectedScope,
  scopeDataByScope,
  selectedVariable,
  selectedSecret,
  expandedVariableKey,
  variableValueLoadingKey,
  variableValues,
  pipelineVariableIndex,
  pipelineSecretIndex,
  pipelineMetadata,
  triggersByScope,
  usageLoading,
  usageError,
  runnerStatus = null,
  runnerStatusLoading = false,
  runnerStatusError = null,
  canWriteVariablesInSelectedScope,
  canWriteSecretsInSelectedScope,
  canDeleteScopes,
  onSelectVariable,
  onSelectSecret,
  onToggleVariableValue,
  onCreateVariable,
  onUpdateVariable,
  onCloneVariable,
  onCreateSecret,
  onUpdateSecret,
  onCloneSecret,
  onDeleteValue,
  onOpenGitOpsEncrypt,
  onBack,
}: ScopeDetailViewProps) {
  const [searchTerm, setSearchTerm] = useState('');
  const [activeFilter, setActiveFilter] = useState<ScopeDetailItemFilter>('all');
  const [createMenuOpen, setCreateMenuOpen] = useState(false);
  const [runnerPanelOpen, setRunnerPanelOpen] = useState(false);
  const [selectedRunnerId, setSelectedRunnerId] = useState<string | null>(null);
  const [copyState, setCopyState] = useState<ValueCopyState>('idle');

  const scopeLabel = normalizeScopeLabel(selectedScope ?? '');
  const scopeDisplay = formatScopeDisplay(scopeLabel);
  const data = scopeDataByScope[scopeLabel] || createInitialScopeData();
  const scopeTriggers = triggersByScope.get(scopeLabel) || [];
  const items = useMemo(
    () => buildScopeDetailItems(data, pipelineVariableIndex, pipelineSecretIndex),
    [data, pipelineSecretIndex, pipelineVariableIndex]
  );
  const filteredItems = useMemo(
    () => filterScopeDetailItems(items, searchTerm, activeFilter),
    [activeFilter, items, searchTerm]
  );
  const itemSections = useMemo(() => groupScopeDetailItems(filteredItems), [filteredItems]);
  const selectedItem = findSelectedItem(items, selectedVariable, selectedSecret) || items[0] || null;
  const selectedPipelineIds = selectedItem
    ? Array.from((selectedItem.kind === 'variable' ? pipelineVariableIndex : pipelineSecretIndex).get(selectedItem.fullName) || [])
        .sort((left, right) => left.localeCompare(right, undefined, { sensitivity: 'base' }))
    : [];
  const variableItems = items.filter(item => item.kind === 'variable');
  const secretItems = items.filter(item => item.kind === 'secret');
  const totalRelatedResources = new Set([
    ...items.flatMap(item => Array.from((item.kind === 'variable' ? pipelineVariableIndex : pipelineSecretIndex).get(item.fullName) || [])),
    ...scopeTriggers.map(trigger => `trigger:${trigger.slug}:${trigger.event}`),
  ]).size;
  const runnerAssignments = buildRunnerAssignmentsForScope(runnerStatus, scopeLabel);
  const runnerSummary = summarizeRunnerAssignments(runnerAssignments, runnerStatusLoading, runnerStatusError);
  const canCreateAny = canWriteVariablesInSelectedScope || canWriteSecretsInSelectedScope;

  if (selectedScope == null) return null;

  const handleSelectItem = (item: ScopeDetailItem) => {
    setCopyState('idle');
    if (item.kind === 'variable') {
      onSelectVariable(item.fullName);
      return;
    }
    onSelectSecret(item.fullName);
  };

  const handleCreateVariable = () => {
    setCreateMenuOpen(false);
    onCreateVariable(scopeLabel);
  };

  const handleCreateSecret = () => {
    setCreateMenuOpen(false);
    onCreateSecret(scopeLabel);
  };

  return (
    <div id="scopes-detail-view" className="scope-detail-redesign">
      <header className="scope-detail-redesign__head">
        <div className="scope-detail-redesign__title">
          <span className="scope-detail-redesign__eyebrow">Scope</span>
          <h2>{scopeDisplay}</h2>
          <p>Manage configuration values, encrypted credentials, access, and runtime usage across this scope.</p>
        </div>
        <div className="scope-detail-redesign__actions">
          <ResourceAccessCard
            resourceType="scope"
            resourceID={scopeLabel || 'default'}
            label="scope"
            sensitive
            buttonClassName="scope-detail-action scope-detail-action--ghost"
          />
          <button
            type="button"
            className="scope-detail-icon-action"
            aria-label="Encrypt secret for GitOps"
            title="Encrypt secret for GitOps"
            onClick={onOpenGitOpsEncrypt}
          >
            <KeyRound className="h-4 w-4" aria-hidden="true" />
          </button>
          {canCreateAny ? (
            <div className="scope-detail-create-action">
              <button
                type="button"
                className="scope-detail-action scope-detail-action--primary"
                aria-haspopup="menu"
                aria-expanded={createMenuOpen}
                onClick={() => setCreateMenuOpen(open => !open)}
              >
                <Plus className="h-4 w-4" aria-hidden="true" />
                <span>New item</span>
              </button>
              {createMenuOpen ? (
                <div className="scope-detail-create-menu" role="menu" aria-label="Create scoped item">
                  <button type="button" role="menuitem" disabled={!canWriteVariablesInSelectedScope} onClick={handleCreateVariable}>
                    Variable
                  </button>
                  <button type="button" role="menuitem" disabled={!canWriteSecretsInSelectedScope} onClick={handleCreateSecret}>
                    Secret
                  </button>
                </div>
              ) : null}
            </div>
          ) : null}
          <button type="button" className="scope-detail-action scope-detail-action--ghost" onClick={onBack}>
            <ArrowLeft className="h-4 w-4" aria-hidden="true" />
            <span>Back</span>
          </button>
        </div>
      </header>

      <section className="scope-detail-summary-grid" aria-label="Scope summary">
        <MetricCard label="Variables" value={data.variables.length} note={summarizeScopeSourceMix(variableItems, 'No variables configured')} />
        <MetricCard label="Secrets" value={data.secrets.length} note={summarizeScopeSourceMix(secretItems, 'No secrets configured')} />
        <MetricCard label="Used by" value={totalRelatedResources} note="Pipelines and triggers" />
        <button
          type="button"
          className="scope-detail-metric scope-detail-metric--button"
          aria-expanded={runnerPanelOpen}
          aria-controls="scope-runner-assignments"
          onClick={() => setRunnerPanelOpen(open => !open)}
        >
          <span className="scope-detail-metric__label">Runner assignments</span>
          <span className="scope-detail-metric__value">{runnerSummary.countLabel}</span>
          <span className="scope-detail-metric__note">{runnerSummary.note}</span>
        </button>
      </section>

      {runnerPanelOpen ? (
        <RunnerScopePanel
          assignments={runnerAssignments}
          loading={runnerStatusLoading}
          error={runnerStatusError}
          selectedRunnerId={selectedRunnerId}
          onSelectRunner={setSelectedRunnerId}
          onClose={() => setRunnerPanelOpen(false)}
        />
      ) : null}

      <div className="scope-detail-content-grid">
        <section className="scope-detail-panel scope-detail-list-panel" aria-labelledby="scope-detail-items-title">
          <header className="scope-detail-panel-titlebar">
            <div>
              <h3 id="scope-detail-items-title">Variables and secrets</h3>
              <p>{data.variablesLoading || data.secretsLoading ? 'Loading scoped values' : `${filteredItems.length} visible`}</p>
            </div>
            <span className="scope-detail-count-badge">{filteredItems.length}</span>
          </header>

          <div className="scope-detail-toolbar">
            <label className="scope-detail-search" htmlFor="scope-detail-search">
              <Search className="h-4 w-4" aria-hidden="true" />
              <span className="sr-only">Search variables and secrets</span>
              <input
                id="scope-detail-search"
                type="search"
                placeholder="Search name or source"
                value={searchTerm}
                onChange={event => setSearchTerm(event.target.value)}
              />
            </label>
            <div className="scope-detail-segmented" aria-label="Filter item type">
              {(['all', 'variable', 'secret'] as const).map(filter => (
                <button
                  key={filter}
                  type="button"
                  className={activeFilter === filter ? 'scope-detail-segmented__item scope-detail-segmented__item--active' : 'scope-detail-segmented__item'}
                  aria-pressed={activeFilter === filter}
                  onClick={() => setActiveFilter(filter)}
                >
                  {filter === 'all' ? 'All' : filter === 'variable' ? 'Vars' : 'Secrets'}
                </button>
              ))}
            </div>
          </div>

          <div className="scope-detail-items">
            {itemSections.map(section => (
              <ScopeItemSection
                key={section.id}
                section={section}
                activeItem={selectedItem}
                onSelectItem={handleSelectItem}
              />
            ))}
            {filteredItems.length === 0 ? (
              <div className="scope-detail-empty-state">
                {data.variablesLoading || data.secretsLoading ? 'Loading scoped values...' : 'No matching items.'}
              </div>
            ) : null}
          </div>
        </section>

        <ScopeItemInspector
          scopeDisplay={scopeDisplay}
          scopeLabel={scopeLabel}
          item={selectedItem}
          selectedPipelineIds={selectedPipelineIds}
          pipelineMetadata={pipelineMetadata}
          triggers={scopeTriggers}
          loading={usageLoading}
          error={usageError}
          expandedVariableKey={expandedVariableKey}
          variableValueLoadingKey={variableValueLoadingKey}
          variableValues={variableValues}
          copyState={copyState}
          canWriteVariablesInSelectedScope={canWriteVariablesInSelectedScope}
          canWriteSecretsInSelectedScope={canWriteSecretsInSelectedScope}
          canDeleteScopes={canDeleteScopes}
          onCopyStateChange={setCopyState}
          onToggleVariableValue={onToggleVariableValue}
          onUpdateVariable={onUpdateVariable}
          onCloneVariable={onCloneVariable}
          onUpdateSecret={onUpdateSecret}
          onCloneSecret={onCloneSecret}
          onDeleteValue={onDeleteValue}
        />
      </div>
    </div>
  );
}

function MetricCard({ label, value, note }: { label: string; value: ReactNode; note: string }) {
  return (
    <div className="scope-detail-metric">
      <span className="scope-detail-metric__label">{label}</span>
      <strong className="scope-detail-metric__value">{value}</strong>
      <span className="scope-detail-metric__note">{note}</span>
    </div>
  );
}

function ScopeItemSection({
  section,
  activeItem,
  onSelectItem,
}: {
  section: ScopeDetailItemSection;
  activeItem: ScopeDetailItem | null;
  onSelectItem: (item: ScopeDetailItem) => void;
}) {
  return (
    <section className="scope-detail-item-section">
      <h4>{section.title}</h4>
      <div className="scope-detail-item-stack">
        {section.items.map(item => {
          const active = activeItem ? scopeDetailItemKey(activeItem) === scopeDetailItemKey(item) : false;
          const usageLabel = `${item.pipelineCount} usage${item.pipelineCount === 1 ? '' : 's'}`;
          return (
            <button
              key={scopeDetailItemKey(item)}
              type="button"
              className={active ? 'scope-detail-item scope-detail-item--active' : 'scope-detail-item'}
              onClick={() => onSelectItem(item)}
            >
              <span className="scope-detail-item__top">
                <span className="scope-detail-item__name" title={item.fullName}>{item.displayName}</span>
                <span className={`scope-variable-source-pill ${scopeSourcePillClass(item.source)}`}>{item.sourceLabel}</span>
              </span>
              <span className="scope-detail-item__meta">
                <span>{item.kind === 'variable' ? 'Variable' : 'Secret'}</span>
                <span>{usageLabel}</span>
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
}

function ScopeItemInspector({
  scopeDisplay,
  scopeLabel,
  item,
  selectedPipelineIds,
  pipelineMetadata,
  triggers,
  loading,
  error,
  expandedVariableKey,
  variableValueLoadingKey,
  variableValues,
  copyState,
  canWriteVariablesInSelectedScope,
  canWriteSecretsInSelectedScope,
  canDeleteScopes,
  onCopyStateChange,
  onToggleVariableValue,
  onUpdateVariable,
  onCloneVariable,
  onUpdateSecret,
  onCloneSecret,
  onDeleteValue,
}: {
  scopeDisplay: string;
  scopeLabel: string;
  item: ScopeDetailItem | null;
  selectedPipelineIds: string[];
  pipelineMetadata: Map<string, ScopePipelineMeta>;
  triggers: ScopeTriggerDescriptor[];
  loading: boolean;
  error: string | null;
  expandedVariableKey: string | null;
  variableValueLoadingKey: string | null;
  variableValues: Record<string, string>;
  copyState: ValueCopyState;
  canWriteVariablesInSelectedScope: boolean;
  canWriteSecretsInSelectedScope: boolean;
  canDeleteScopes: boolean;
  onCopyStateChange: (state: ValueCopyState) => void;
  onToggleVariableValue: (scopeLabel: string, fullName: string) => Promise<void>;
  onUpdateVariable: (scopeLabel: string, fullName: string) => void;
  onCloneVariable: (scopeLabel: string, fullName: string) => void;
  onUpdateSecret: (scopeLabel: string, fullName: string) => void;
  onCloneSecret: (scopeLabel: string, fullName: string) => void;
  onDeleteValue: (kind: 'variable' | 'secret', scopeLabel: string, fullName: string) => void;
}) {
  if (!item) {
    return (
      <section className="scope-detail-panel scope-detail-inspector" aria-label="Scoped item details">
        <div className="scope-detail-empty-state scope-detail-empty-state--large">Create a variable or secret to inspect scope usage.</div>
      </section>
    );
  }

  const identity = parseScopedIdentity(item.fullName);
  const repository = identity.repoSlug || 'Global';
  const variableCacheKey = `${item.fullName}@@${scopeLabel}`;
  const variableExpanded = item.kind === 'variable' && expandedVariableKey === variableCacheKey;
  const variableLoading = item.kind === 'variable' && variableValueLoadingKey === variableCacheKey;
  const variableValue = variableValues[variableCacheKey] ?? '';
  const canWriteItem = item.kind === 'variable' ? canWriteVariablesInSelectedScope : canWriteSecretsInSelectedScope;
  const editable = isEditableScopeSource(item.source);
  const canEdit = canWriteItem && editable;
  const canClone = canWriteItem;
  const canDelete = canDeleteScopes && editable;
  const gitOpsManaged = isGitOpsScopeSource(item.source);
  const valueText = item.kind === 'secret'
    ? 'Secret value is encrypted and never displayed'
    : variableExpanded
      ? variableValue || '(empty)'
      : 'Hidden until revealed';
  const canCopyValue = item.kind === 'variable' && variableExpanded && !variableLoading;

  const revealLabel = variableExpanded ? 'Hide value' : variableLoading ? 'Loading value' : 'Reveal value';
  const copyLabel = copyState === 'copied' ? 'Copied' : copyState === 'error' ? 'Copy failed' : 'Copy value';
  const pipelineEmpty = loading ? 'Loading impact analysis...' : error || `No pipelines declare this ${item.kind}.`;
  const triggersEmpty = loading ? 'Loading impact analysis...' : error || 'No triggers reference this scope.';

  const handleReveal = async () => {
    if (item.kind !== 'variable' || variableLoading) return;
    await onToggleVariableValue(scopeLabel, item.fullName);
  };

  const handleCopy = async () => {
    if (!canCopyValue) return;
    try {
      await copyTextToClipboard(variableValue);
      onCopyStateChange('copied');
      window.setTimeout(() => onCopyStateChange('idle'), 1800);
    } catch (copyError) {
      console.error('Failed to copy variable value', copyError);
      onCopyStateChange('error');
    }
  };

  const handleEdit = () => {
    if (!canEdit) return;
    if (item.kind === 'variable') {
      onUpdateVariable(scopeLabel, item.fullName);
      return;
    }
    onUpdateSecret(scopeLabel, item.fullName);
  };

  const handleClone = () => {
    if (!canClone) return;
    if (item.kind === 'variable') {
      onCloneVariable(scopeLabel, item.fullName);
      return;
    }
    onCloneSecret(scopeLabel, item.fullName);
  };

  const handleDelete = () => {
    if (!canDelete) return;
    onDeleteValue(item.kind, scopeLabel, item.fullName);
  };

  return (
    <section className="scope-detail-panel scope-detail-inspector" aria-label="Scoped item details">
      <header className="scope-detail-inspector__head">
        <div className="scope-detail-inspector__title">
          <h3>{item.displayName}</h3>
          <p>{item.kind === 'variable' ? 'Variable' : 'Secret'} / {item.sourceLabel}</p>
        </div>
        <div className="scope-detail-inspector__actions">
          <button
            type="button"
            className="scope-detail-icon-action"
            title={copyLabel}
            aria-label={copyLabel}
            disabled={!canCopyValue}
            onClick={() => void handleCopy()}
          >
            <Clipboard className="h-4 w-4" aria-hidden="true" />
          </button>
          <button
            type="button"
            className="scope-detail-icon-action"
            title={gitOpsManaged ? 'Edit database override' : `Edit ${item.kind}`}
            aria-label={gitOpsManaged ? 'Edit database override' : `Edit ${item.kind}`}
            disabled={!canEdit}
            onClick={handleEdit}
          >
            <Pencil className="h-4 w-4" aria-hidden="true" />
          </button>
          <button
            type="button"
            className="scope-detail-icon-action"
            title={`Clone ${item.kind}`}
            aria-label={`Clone ${item.kind}`}
            disabled={!canClone}
            onClick={handleClone}
          >
            <Copy className="h-4 w-4" aria-hidden="true" />
          </button>
          <button
            type="button"
            className="scope-detail-icon-action scope-detail-icon-action--danger"
            title={`Delete ${item.kind}`}
            aria-label={`Delete ${item.kind}`}
            disabled={!canDelete}
            onClick={handleDelete}
          >
            <Trash2 className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </header>

      <div className="scope-detail-value-card">
        <span className="scope-detail-value-card__label">Current value</span>
        <div className="scope-detail-value-card__row">
          <code title={item.kind === 'variable' && variableExpanded ? variableValue : undefined}>{valueText}</code>
          {item.kind === 'variable' ? (
            <button
              type="button"
              className="scope-detail-action scope-detail-action--ghost"
              disabled={variableLoading}
              onClick={() => void handleReveal()}
            >
              {variableExpanded ? <EyeOff className="h-4 w-4" aria-hidden="true" /> : <Eye className="h-4 w-4" aria-hidden="true" />}
              <span>{revealLabel}</span>
            </button>
          ) : null}
        </div>
      </div>

      <div className="scope-detail-inspector__scroll">
        <dl className="scope-detail-info-grid">
          <InfoBox label="Scope" value={scopeDisplay} />
          <InfoBox label="Repository" value={repository} />
          <InfoBox label="Updated" value={formatScopeTimestamp(item.updatedAt)} />
          <InfoBox label="Created" value={formatScopeTimestamp(item.createdAt)} />
          <InfoBox label="Source" value={gitOpsManaged ? 'GitOps managed' : item.sourceLabel} />
          <InfoBox label="Usage" value={`${selectedPipelineIds.length} pipeline${selectedPipelineIds.length === 1 ? '' : 's'}`} />
        </dl>

        <div className="scope-detail-relationships-grid">
          <RelationshipColumn title="Used by pipelines" count={selectedPipelineIds.length} empty={pipelineEmpty}>
            {!loading && !error
              ? selectedPipelineIds.map(identifier => (
                  <PipelineUsageCard key={`pipe-${identifier}`} identifier={identifier} metadata={pipelineMetadata.get(identifier)} />
                ))
              : null}
          </RelationshipColumn>
          <RelationshipColumn title="Related triggers" count={triggers.length} empty={triggersEmpty}>
            {!loading && !error
              ? triggers.map(trigger => (
                  <TriggerUsageCard key={`trigger-${trigger.slug}-${trigger.scope}-${trigger.event}`} trigger={trigger} />
                ))
              : null}
          </RelationshipColumn>
        </div>
      </div>
    </section>
  );
}

function InfoBox({ label, value }: { label: string; value: string }) {
  return (
    <div className="scope-detail-info-box">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function RelationshipColumn({
  title,
  count,
  empty,
  children,
}: {
  title: string;
  count: number;
  empty: string;
  children: ReactNode;
}) {
  return (
    <section className="scope-detail-relationship-column">
      <header>
        <h4>{title}</h4>
        <span className="scope-detail-count-badge">{count}</span>
      </header>
      <div className="scope-detail-relationship-list" data-empty={empty}>
        {children}
      </div>
    </section>
  );
}

function PipelineUsageCard({ identifier, metadata }: { identifier: string; metadata?: ScopePipelineMeta }) {
  const title = metadata?.name || identifier;
  const pathDisplay = metadata?.path ? `/${metadata.path}` : '/';
  const href = `#/pipelines/${identifier.split('/').map(encodeURIComponent).join('/')}`;
  return (
    <a className="scope-detail-usage-card" href={href}>
      <span className="scope-detail-usage-card__title">
        <span>{title}</span>
        <span aria-hidden="true">Open</span>
      </span>
      {metadata?.description ? <p>{metadata.description}</p> : null}
      <span className="scope-detail-usage-card__meta">
        <span>{metadata?.version || 'latest'}</span>
        <span>{metadata?.source || 'Config Repository'}</span>
        <span>{pathDisplay}</span>
      </span>
    </a>
  );
}

function TriggerUsageCard({ trigger }: { trigger: ScopeTriggerDescriptor }) {
  const href = `#/triggers/${trigger.slug.split('/').map(encodeURIComponent).join('/')}`;
  const pipelineCount = trigger.pipelines.length;
  const branchSummary = trigger.branches.length ? trigger.branches.join(', ') : 'All branches';
  return (
    <a className="scope-detail-usage-card" href={href}>
      <span className="scope-detail-usage-card__title">
        <span>{trigger.slug}</span>
        <span>{trigger.event}</span>
      </span>
      <p>{pipelineCount ? `${pipelineCount} linked pipeline${pipelineCount === 1 ? '' : 's'}` : 'No linked pipelines'}</p>
      <span className="scope-detail-usage-card__meta">
        <span>{branchSummary}</span>
        <span>{trigger.tags.length ? trigger.tags.join(', ') : 'No tags'}</span>
      </span>
    </a>
  );
}

function RunnerScopePanel({
  assignments,
  loading,
  error,
  selectedRunnerId,
  onSelectRunner,
  onClose,
}: {
  assignments: RunnerRouteAssignment[];
  loading: boolean;
  error: string | null;
  selectedRunnerId: string | null;
  onSelectRunner: (runnerId: string) => void;
  onClose: () => void;
}) {
  const summary = summarizeRunnerAssignments(assignments, loading, error);
  const activeRunnerId = assignments.some(assignment => assignment.runner.runnerId === selectedRunnerId)
    ? selectedRunnerId
    : assignments[0]?.runner.runnerId || null;

  return (
    <section id="scope-runner-assignments" className="scope-detail-panel scope-runner-panel" aria-label="Runner assignments">
      <header className="scope-runner-panel__head">
        <div className="scope-runner-panel__title">
          <Server className="h-4 w-4" aria-hidden="true" />
          <div>
            <h3>Runner assignments</h3>
            <p>{summary.note}</p>
          </div>
          <span className="scope-detail-count-badge">{summary.countLabel}</span>
        </div>
        <div className="scope-runner-panel__actions">
          <Link to="/system/dispatcher" className="scope-detail-action scope-detail-action--ghost">
            <Route className="h-4 w-4" aria-hidden="true" />
            <span>Dispatcher</span>
          </Link>
          <button type="button" className="scope-detail-icon-action" aria-label="Hide runner assignments" onClick={onClose}>
            <ChevronUp className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </header>

      <div className="scope-runner-grid">
        {loading && !assignments.length ? <div className="scope-detail-empty-state">Loading runner assignments...</div> : null}
        {error ? <div className="scope-detail-empty-state">Runner assignments unavailable.</div> : null}
        {!loading && !error && !assignments.length ? <div className="scope-detail-empty-state">No runner assignments found.</div> : null}
        {!error
          ? assignments.map(assignment => (
              <RunnerCard
                key={assignment.runner.runnerId}
                assignment={assignment}
                active={assignment.runner.runnerId === activeRunnerId}
                onSelectRunner={onSelectRunner}
              />
            ))
          : null}
      </div>
    </section>
  );
}

function RunnerCard({
  assignment,
  active,
  onSelectRunner,
}: {
  assignment: RunnerRouteAssignment;
  active: boolean;
  onSelectRunner: (runnerId: string) => void;
}) {
  const runner = assignment.runner;
  const meta = getRunnerMeta(runner);
  const status = runnerAvailability(runner);
  const load = Math.max(runner.activeJobs || 0, runner.inflightJobs || 0);
  const runtime = meta.runtime || 'docker';
  const tags = [
    ...assignment.scopes.map(formatDispatcherRouteScope),
    meta.namespace,
    meta.node,
  ].filter(Boolean);

  return (
    <button
      type="button"
      className={active ? 'scope-runner-card scope-runner-card--active' : 'scope-runner-card'}
      onClick={() => onSelectRunner(runner.runnerId)}
    >
      <span className="scope-runner-card__top">
        <span className="scope-runner-card__name" title={runner.runnerId}>{runner.runnerId}</span>
        <span className={`scope-runner-state scope-runner-state--${status}`}>
          <span aria-hidden="true" />
          {status === 'online' ? 'Online' : status === 'paused' ? 'Paused' : 'Offline'}
        </span>
      </span>
      <span className="scope-runner-card__stats">
        <span>
          <small>Capacity</small>
          <strong>{load} / {runner.capacity || 1} active</strong>
        </span>
        <span>
          <small>Executor</small>
          <strong>{runtime}</strong>
        </span>
      </span>
      <span className="scope-runner-card__tags">
        {tags.slice(0, 4).map(tag => (
          <span key={`${runner.runnerId}-${tag}`}>{tag}</span>
        ))}
        <span>{formatRunnerLastSeen(runner)}</span>
      </span>
    </button>
  );
}

function findSelectedItem(items: ScopeDetailItem[], selectedVariable: string | null, selectedSecret: string | null) {
  if (selectedVariable) {
    const variable = items.find(item => item.kind === 'variable' && item.fullName === selectedVariable);
    if (variable) return variable;
  }
  if (selectedSecret) {
    return items.find(item => item.kind === 'secret' && item.fullName === selectedSecret) || null;
  }
  return null;
}

function scopeDetailItemKey(item: ScopeDetailItem) {
  return `${item.kind}:${item.fullName}`;
}

function runnerAvailability(runner: Runner): RunnerAvailability {
  const meta = getRunnerMeta(runner);
  if (!meta.reachable) return 'offline';
  return runner.allowDispatch ? 'online' : 'paused';
}

function summarizeRunnerAssignments(assignments: RunnerRouteAssignment[], loading: boolean, error: string | null) {
  if (loading && !assignments.length) return { countLabel: '...', note: 'Loading dispatcher status' };
  if (error) return { countLabel: '0', note: 'Runner assignments unavailable' };
  const online = assignments.filter(assignment => runnerAvailability(assignment.runner) === 'online').length;
  const paused = assignments.filter(assignment => runnerAvailability(assignment.runner) === 'paused').length;
  const offline = assignments.filter(assignment => runnerAvailability(assignment.runner) === 'offline').length;
  const note = [
    `${online} online`,
    paused ? `${paused} paused` : '',
    `${offline} offline`,
    assignments.length ? 'click to view' : 'no matching runners',
  ].filter(Boolean).join(' / ');
  return { countLabel: String(assignments.length), note };
}

function formatRunnerLastSeen(runner: Runner): string {
  const heartbeat = Number(runner.lastHeartbeatUnix || 0);
  if (!heartbeat) return 'No heartbeat';
  const elapsedSeconds = Math.max(0, Math.floor(Date.now() / 1000 - heartbeat));
  if (elapsedSeconds < 60) return 'Just now';
  if (elapsedSeconds < 3600) return `${Math.floor(elapsedSeconds / 60)}m ago`;
  if (elapsedSeconds < 86400) return `${Math.floor(elapsedSeconds / 3600)}h ago`;
  return `${Math.floor(elapsedSeconds / 86400)}d ago`;
}
