import type { ReactNode } from 'react';
import { ArrowLeft, Copy, Eye, EyeOff, KeyRound, Pencil, Trash2 } from 'lucide-react';
import ResourceAccessCard from '../../components/ResourceAccessCard';
import {
  createInitialScopeData,
  formatScopeDisplay,
  groupScopedItems,
  isEditableScopeSource,
  isGitOpsScopeSource,
  normalizeScopeLabel,
  scopeSourceLabel,
  scopeSourcePillClass,
  type GroupedScopedItem,
  type ScopeData,
  type ScopePipelineMeta,
  type ScopeTriggerDescriptor,
} from './model';
import { ScopeUsagePanel } from './ScopeUsagePanel';

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
  if (selectedScope == null) return null;

  const scopeLabel = normalizeScopeLabel(selectedScope);
  const scopeDisplay = formatScopeDisplay(scopeLabel);
  const data = scopeDataByScope[scopeLabel] || createInitialScopeData();
  const variableGroups = groupScopedItems(data.variables);
  const secretGroups = groupScopedItems(data.secrets);
  const variableMeta = selectedVariable ? data.variableMeta[selectedVariable] : undefined;
  const secretMeta = selectedSecret ? data.secretMeta[selectedSecret] : undefined;
  const relatedVariablePipelines = selectedVariable ? Array.from(pipelineVariableIndex.get(selectedVariable) || []) : [];
  const relatedSecretPipelines = selectedSecret ? Array.from(pipelineSecretIndex.get(selectedSecret) || []) : [];
  const scopeTriggers = triggersByScope.get(scopeLabel) || [];
  const activeSelection = selectedVariable
    ? { type: 'variable' as const, name: selectedVariable, meta: variableMeta, pipelines: relatedVariablePipelines }
    : selectedSecret
      ? { type: 'secret' as const, name: selectedSecret, meta: secretMeta, pipelines: relatedSecretPipelines }
      : null;

  return (
    <div id="scopes-detail-view" className="pipelines-view">
      <div className="glass-card p-6">
        <div className="flex items-start justify-between gap-4 w-full">
          <div className="min-w-0 space-y-2">
            <p className="text-xs uppercase tracking-[0.2em] text-[var(--text-secondary)]">Scope</p>
            <h2 className="text-3xl font-bold text-[var(--text-primary)] truncate">{scopeDisplay}</h2>
            <p className="text-sm text-[var(--text-secondary)]">Manage variables and secrets for this scope, all in one view.</p>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              className="pipelines-icon-only"
              aria-label="Encrypt secret for GitOps"
              title="Encrypt secret for GitOps"
              onClick={onOpenGitOpsEncrypt}
            >
              <KeyRound className="h-4 w-4" aria-hidden="true" />
            </button>
            <ResourceAccessCard resourceType="scope" resourceID={scopeLabel || 'default'} label="scope" sensitive />
            <button type="button" className="glass-button-ghost" onClick={onBack}>
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              <span>Back</span>
            </button>
          </div>
        </div>
      </div>

      <div className="grid gap-6 mt-6 lg:grid-cols-[360px_1fr]">
        <div className="space-y-4">
          <ScopedItemPanel
            kind="variable"
            title="Variables"
            description="Plain text values."
            loading={data.variablesLoading}
            loaded={data.variablesLoaded}
            empty={!data.variables.length}
            canWrite={canWriteVariablesInSelectedScope}
            onCreate={() => onCreateVariable(scopeLabel)}
          >
            {variableGroups.global.length ? (
              <VariableSection
                title="Global"
                items={variableGroups.global}
                scopeLabel={scopeLabel}
                data={data}
                selectedVariable={selectedVariable}
                expandedVariableKey={expandedVariableKey}
                variableValueLoadingKey={variableValueLoadingKey}
                variableValues={variableValues}
                canWriteVariablesInSelectedScope={canWriteVariablesInSelectedScope}
                canDeleteScopes={canDeleteScopes}
                onSelectVariable={onSelectVariable}
                onToggleVariableValue={onToggleVariableValue}
                onUpdateVariable={onUpdateVariable}
                onCloneVariable={onCloneVariable}
                onDeleteValue={onDeleteValue}
              />
            ) : null}
            {variableGroups.repositories.map(group => (
              <VariableSection
                key={`var-section-${group.repo}`}
                title={group.repo}
                items={group.items}
                scopeLabel={scopeLabel}
                data={data}
                selectedVariable={selectedVariable}
                expandedVariableKey={expandedVariableKey}
                variableValueLoadingKey={variableValueLoadingKey}
                variableValues={variableValues}
                canWriteVariablesInSelectedScope={canWriteVariablesInSelectedScope}
                canDeleteScopes={canDeleteScopes}
                onSelectVariable={onSelectVariable}
                onToggleVariableValue={onToggleVariableValue}
                onUpdateVariable={onUpdateVariable}
                onCloneVariable={onCloneVariable}
                onDeleteValue={onDeleteValue}
              />
            ))}
          </ScopedItemPanel>

          <ScopedItemPanel
            kind="secret"
            title="Secrets"
            description="Encrypted values."
            loading={data.secretsLoading}
            loaded={data.secretsLoaded}
            empty={!data.secrets.length}
            canWrite={canWriteSecretsInSelectedScope}
            onCreate={() => onCreateSecret(scopeLabel)}
          >
            {secretGroups.global.length ? (
              <SecretSection
                title="Global"
                items={secretGroups.global}
                scopeLabel={scopeLabel}
                data={data}
                selectedSecret={selectedSecret}
                canWriteSecretsInSelectedScope={canWriteSecretsInSelectedScope}
                canDeleteScopes={canDeleteScopes}
                onSelectSecret={onSelectSecret}
                onUpdateSecret={onUpdateSecret}
                onCloneSecret={onCloneSecret}
                onDeleteValue={onDeleteValue}
              />
            ) : null}
            {secretGroups.repositories.map(group => (
              <SecretSection
                key={`secret-section-${group.repo}`}
                title={group.repo}
                items={group.items}
                scopeLabel={scopeLabel}
                data={data}
                selectedSecret={selectedSecret}
                canWriteSecretsInSelectedScope={canWriteSecretsInSelectedScope}
                canDeleteScopes={canDeleteScopes}
                onSelectSecret={onSelectSecret}
                onUpdateSecret={onUpdateSecret}
                onCloneSecret={onCloneSecret}
                onDeleteValue={onDeleteValue}
              />
            ))}
          </ScopedItemPanel>
        </div>

        <div className="space-y-4">
          <ScopeUsagePanel
            selection={activeSelection}
            pipelineMetadata={pipelineMetadata}
            triggers={scopeTriggers}
            loading={usageLoading}
            error={usageError}
          />
        </div>
      </div>
    </div>
  );
}

function ScopedItemPanel({
  title,
  description,
  kind,
  loading,
  loaded,
  empty,
  canWrite,
  onCreate,
  children,
}: {
  title: string;
  description: string;
  kind: 'variable' | 'secret';
  loading: boolean;
  loaded: boolean;
  empty: boolean;
  canWrite: boolean;
  onCreate: () => void;
  children: ReactNode;
}) {
  return (
    <div className="glass-card p-4 rounded-2xl border border-[var(--border-primary)]">
      <div className="flex items-center justify-between mb-3">
        <div>
          <p className="text-sm font-semibold text-[var(--text-primary)]">{title}</p>
          <p className="text-xs text-[var(--text-secondary)]">{description}</p>
        </div>
        {canWrite && (
          <button className="glass-button-primary" onClick={onCreate}>
            New
          </button>
        )}
      </div>
      {!loading && empty ? <div className="scope-panel-empty">No {kind}s configured yet.</div> : null}
      {loading && !loaded ? <div className="scope-panel-empty">Loading {kind}s…</div> : null}
      <div className="scope-variable-list space-y-4">{children}</div>
    </div>
  );
}

function VariableSection({
  title,
  items,
  scopeLabel,
  data,
  selectedVariable,
  expandedVariableKey,
  variableValueLoadingKey,
  variableValues,
  canWriteVariablesInSelectedScope,
  canDeleteScopes,
  onSelectVariable,
  onToggleVariableValue,
  onUpdateVariable,
  onCloneVariable,
  onDeleteValue,
}: {
  title: string;
  items: GroupedScopedItem[];
  scopeLabel: string;
  data: ScopeData;
  selectedVariable: string | null;
  expandedVariableKey: string | null;
  variableValueLoadingKey: string | null;
  variableValues: Record<string, string>;
  canWriteVariablesInSelectedScope: boolean;
  canDeleteScopes: boolean;
  onSelectVariable: (name: string | null) => void;
  onToggleVariableValue: (scopeLabel: string, fullName: string) => Promise<void>;
  onUpdateVariable: (scopeLabel: string, fullName: string) => void;
  onCloneVariable: (scopeLabel: string, fullName: string) => void;
  onDeleteValue: (kind: 'variable' | 'secret', scopeLabel: string, fullName: string) => void;
}) {
  return (
    <section key={`var-section-${title || 'global'}`} className="space-y-2">
      {title ? <p className="text-xs uppercase tracking-[0.18em] text-[var(--text-secondary)]">{title}</p> : null}
      <div className="scope-variable-buttons">
        {items.map(item => {
          const isActive = item.full === selectedVariable;
          const cacheKey = `${item.full}@@${scopeLabel}`;
          const isExpanded = expandedVariableKey === cacheKey;
          const value = variableValues[cacheKey] ?? '';
          const displayValue = value ? value : '(empty)';
          const isLoading = variableValueLoadingKey === cacheKey;
          const meta = data.variableMeta[item.full];
          const editable = isEditableScopeSource(meta?.source || 'database');
          const gitOpsManaged = isGitOpsScopeSource(meta?.source);
          return (
            <div
              key={`var-${item.full}`}
              className={`scope-variable-item rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] ${isActive ? 'scope-variable-item--active' : ''} ${isExpanded ? 'scope-variable-item--expanded' : ''}`}
            >
              <div className="scope-variable-info">
                <button
                  type="button"
                  className={`scope-variable-btn${isActive ? ' scope-variable-btn--active' : ''}`}
                  onClick={() => onSelectVariable(item.full)}
                >
                  <span className="truncate">{item.display}</span>
                </button>
                <span className={`scope-variable-source-pill ${scopeSourcePillClass(meta?.source || 'database')}`}>{scopeSourceLabel(meta?.source || 'database')}</span>
              </div>
              <div className="scope-variable-inline-actions">
                <button
                  type="button"
                  className={`scope-inline-icon${isLoading ? ' loading' : ''}${isExpanded ? ' scope-inline-icon--active' : ''}`}
                  title={isExpanded ? 'Hide value' : 'Show value'}
                  aria-label={isExpanded ? 'Hide value' : 'Show value'}
                  disabled={isLoading}
                  onClick={async event => {
                    event.preventDefault();
                    event.stopPropagation();
                    onSelectVariable(item.full);
                    await onToggleVariableValue(scopeLabel, item.full);
                  }}
                >
                  {isExpanded ? <EyeOff className="h-4 w-4" aria-hidden="true" /> : <Eye className="h-4 w-4" aria-hidden="true" />}
                </button>

                {editable ? (
                  <>
                    {canWriteVariablesInSelectedScope && (
                      <button
                        type="button"
                        className="scope-inline-icon"
                        title={gitOpsManaged ? 'Edit database override; GitOps can replace it on next sync' : 'Edit variable'}
                        onClick={event => {
                          event.preventDefault();
                          event.stopPropagation();
                          onSelectVariable(item.full);
                          onUpdateVariable(scopeLabel, item.full);
                        }}
                      >
                        <Pencil className="h-4 w-4" aria-hidden="true" />
                      </button>
                    )}
                    {canDeleteScopes && (
                      <button
                        type="button"
                        className="scope-inline-icon scope-inline-icon--danger"
                        title={gitOpsManaged ? 'Delete database row; GitOps can recreate it on next sync' : 'Delete variable'}
                        onClick={event => {
                          event.preventDefault();
                          event.stopPropagation();
                          onSelectVariable(item.full);
                          onDeleteValue('variable', scopeLabel, item.full);
                        }}
                      >
                        <Trash2 className="h-4 w-4" aria-hidden="true" />
                      </button>
                    )}
                    {gitOpsManaged && canWriteVariablesInSelectedScope ? (
                      <button
                        type="button"
                        className="scope-inline-icon"
                        title="Clone"
                        onClick={event => {
                          event.preventDefault();
                          event.stopPropagation();
                          onCloneVariable(scopeLabel, item.full);
                        }}
                      >
                        <Copy className="h-4 w-4" aria-hidden="true" />
                      </button>
                    ) : null}
                  </>
                ) : canWriteVariablesInSelectedScope ? (
                  <button
                    type="button"
                    className="scope-inline-icon"
                    title="Clone"
                    onClick={event => {
                      event.preventDefault();
                      event.stopPropagation();
                      onCloneVariable(scopeLabel, item.full);
                    }}
                  >
                    <Copy className="h-4 w-4" aria-hidden="true" />
                  </button>
                ) : null}
              </div>
              <div className="scope-variable-value">{isExpanded ? displayValue : ''}</div>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function SecretSection({
  title,
  items,
  scopeLabel,
  data,
  selectedSecret,
  canWriteSecretsInSelectedScope,
  canDeleteScopes,
  onSelectSecret,
  onUpdateSecret,
  onCloneSecret,
  onDeleteValue,
}: {
  title: string;
  items: GroupedScopedItem[];
  scopeLabel: string;
  data: ScopeData;
  selectedSecret: string | null;
  canWriteSecretsInSelectedScope: boolean;
  canDeleteScopes: boolean;
  onSelectSecret: (name: string | null) => void;
  onUpdateSecret: (scopeLabel: string, fullName: string) => void;
  onCloneSecret: (scopeLabel: string, fullName: string) => void;
  onDeleteValue: (kind: 'variable' | 'secret', scopeLabel: string, fullName: string) => void;
}) {
  return (
    <section key={`secret-section-${title || 'global'}`} className="space-y-2">
      {title ? <p className="text-xs uppercase tracking-[0.18em] text-[var(--text-secondary)]">{title}</p> : null}
      <div className="scope-variable-buttons">
        {items.map(item => {
          const isActive = item.full === selectedSecret;
          const meta = data.secretMeta[item.full];
          const editable = isEditableScopeSource(meta?.source || 'database');
          const gitOpsManaged = isGitOpsScopeSource(meta?.source);
          return (
            <div
              key={`secret-${item.full}`}
              className={`scope-variable-item rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] ${isActive ? ' scope-variable-item--active' : ''}`}
            >
              <div className="scope-variable-info">
                <button
                  type="button"
                  className={`scope-variable-btn${isActive ? ' scope-variable-btn--active' : ''}`}
                  onClick={() => onSelectSecret(item.full)}
                >
                  <span className="truncate">{item.display}</span>
                </button>
                <span className={`scope-variable-source-pill ${scopeSourcePillClass(meta?.source || 'database')}`}>{scopeSourceLabel(meta?.source || 'database')}</span>
              </div>
              <div className="scope-variable-inline-actions">
                {editable ? (
                  <>
                    {canWriteSecretsInSelectedScope && (
                      <button
                        type="button"
                        className="scope-inline-icon"
                        title={gitOpsManaged ? 'Edit database override; GitOps can replace it on next sync' : 'Edit secret'}
                        onClick={event => {
                          event.preventDefault();
                          event.stopPropagation();
                          onSelectSecret(item.full);
                          onUpdateSecret(scopeLabel, item.full);
                        }}
                      >
                        <Pencil className="h-4 w-4" aria-hidden="true" />
                      </button>
                    )}
                    {canDeleteScopes && (
                      <button
                        type="button"
                        className="scope-inline-icon scope-inline-icon--danger"
                        title={gitOpsManaged ? 'Delete database row; GitOps can recreate it on next sync' : 'Delete secret'}
                        onClick={event => {
                          event.preventDefault();
                          event.stopPropagation();
                          onSelectSecret(item.full);
                          onDeleteValue('secret', scopeLabel, item.full);
                        }}
                      >
                        <Trash2 className="h-4 w-4" aria-hidden="true" />
                      </button>
                    )}
                    {gitOpsManaged && canWriteSecretsInSelectedScope ? (
                      <button
                        type="button"
                        className="scope-inline-icon"
                        title="Clone"
                        onClick={event => {
                          event.preventDefault();
                          event.stopPropagation();
                          onCloneSecret(scopeLabel, item.full);
                        }}
                      >
                        <Copy className="h-4 w-4" aria-hidden="true" />
                      </button>
                    ) : null}
                  </>
                ) : canWriteSecretsInSelectedScope ? (
                  <button
                    type="button"
                    className="scope-inline-icon"
                    title="Clone"
                    onClick={event => {
                      event.preventDefault();
                      event.stopPropagation();
                      onCloneSecret(scopeLabel, item.full);
                    }}
                  >
                    <Copy className="h-4 w-4" aria-hidden="true" />
                  </button>
                ) : null}
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}
