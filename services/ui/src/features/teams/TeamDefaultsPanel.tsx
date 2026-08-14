import { useEffect, useMemo, useState } from 'react';
import { Settings } from 'lucide-react';
import { buildKnowledgeID, kindOrder, type KnowledgeContextListItem } from '../knowledge-context/model';
import { aiResourceLocalName, aiResourceTeamScope } from '../system/aiResourceTeams';
import type {
  TeamAgentProfile,
  TeamAgentProfilesResponse,
  TeamDefaultsPayload,
  TeamDefaultsResponse,
  TeamLLMProfile,
  TeamLLMProfilesResponse,
} from './api';

type DefaultValues = {
  model: string;
  agent_role: string;
  knowledge_context: Record<string, string>;
};

type DefaultOption = {
  value: string;
  label: string;
  detail: string;
};

export function TeamDefaultsPanel({
  llmProfiles,
  agentProfiles,
  teamDefaults,
  knowledgeContexts,
  loading,
  error,
  teamPath,
  canManageDefaults = false,
  onSaveDefaults,
}: {
  llmProfiles: TeamLLMProfilesResponse | null;
  agentProfiles: TeamAgentProfilesResponse | null;
  teamDefaults: TeamDefaultsResponse | null;
  knowledgeContexts: KnowledgeContextListItem[];
  loading: boolean;
  error: string | null;
  teamPath: string;
  canManageDefaults?: boolean;
  onSaveDefaults?: (teamPath: string, defaults: TeamDefaultsPayload) => Promise<void>;
}) {
  const globalScope = !teamPath;
  const title = globalScope ? 'Global Defaults' : 'Team Defaults';
  const llms = useMemo(() => [...(llmProfiles?.profiles ?? [])].sort((left, right) => left.name.localeCompare(right.name)), [llmProfiles?.profiles]);
  const agents = useMemo(() => [...(agentProfiles?.profiles ?? [])].sort((left, right) => left.id.localeCompare(right.id)), [agentProfiles?.profiles]);
  const editable = Boolean(teamPath && canManageDefaults && onSaveDefaults);
  const initialValues = useMemo<DefaultValues>(() => ({
    model: teamDefaults?.model || llmProfiles?.default_profile || '',
    agent_role: teamDefaults?.agent_role || agentProfiles?.default_profile || '',
    knowledge_context: { ...(teamDefaults?.knowledge_context ?? {}) },
  }), [agentProfiles?.default_profile, llmProfiles?.default_profile, teamDefaults?.agent_role, teamDefaults?.knowledge_context, teamDefaults?.model]);
  const [values, setValues] = useState<DefaultValues>(initialValues);
  const [savingDefault, setSavingDefault] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const llmOptions = useMemo(
    () => buildLLMDefaultOptions(llms, values.model, teamPath),
    [llms, teamPath, values.model]
  );
  const agentOptions = useMemo(
    () => buildAgentDefaultOptions(agents, values.agent_role, teamPath),
    [agents, teamPath, values.agent_role]
  );

  useEffect(() => {
    setValues(initialValues);
  }, [initialValues]);

  useEffect(() => {
    setSaveError(null);
  }, [teamPath]);

  const saveValue = async (rowID: string, nextValue: string, payload: TeamDefaultsPayload, apply: (draft: DefaultValues) => DefaultValues) => {
    if (!editable || !onSaveDefaults || savingDefault) return;
    const previous = values;
    setValues(apply(values));
    setSaveError(null);
    setSavingDefault(rowID);
    try {
      await onSaveDefaults(teamPath, payload);
    } catch (error) {
      setValues(previous);
      setSaveError(error instanceof Error ? error.message : `Unable to update ${nextValue || 'default'}`);
    } finally {
      setSavingDefault(null);
    }
  };

  const saveLLMProfile = (value: string) => saveValue(
    'llm',
    value,
    { model: value },
    draft => ({ ...draft, model: value })
  );
  const saveAgentProfile = (value: string) => saveValue(
    'agent',
    value,
    { agent_role: value },
    draft => ({ ...draft, agent_role: value })
  );
  const saveKnowledgeDefault = (kind: string, value: string) => saveValue(
    `knowledge:${kind}`,
    value,
    { knowledge_context: { [kind]: value } },
    draft => ({ ...draft, knowledge_context: { ...draft.knowledge_context, [kind]: value } })
  );

  return (
    <article className="teams-card teams-focus-card teams-focus-card--wide">
      <div className="teams-focus-hero">
        <span className="teams-resource-icon teams-tone-purple" aria-hidden="true">
          <Settings className="h-5 w-5" />
        </span>
        <div>
          <h3>{title}</h3>
          <p>{defaultSummary(llms.length, agents.length, knowledgeContexts, teamPath)}</p>
        </div>
      </div>

      {loading ? (
        <div className="teams-inline-status">Loading defaults...</div>
      ) : (
        <>
          {error ? <div className="teams-inline-status teams-inline-status--error">{error}</div> : null}
          {saveError ? <div className="teams-inline-status teams-inline-status--error">{saveError}</div> : null}
          {!editable && !globalScope ? <span className="runner-pill runner-pill--muted">Read-only</span> : null}
          <div className="teams-defaults-list" aria-label="Team defaults">
            <DefaultRow
              id="team-default-llm-profile"
              label="Model"
              value={values.model}
              options={llmOptions}
              disabled={!editable || savingDefault !== null}
              saving={savingDefault === 'llm'}
              onChange={value => void saveLLMProfile(value)}
            />
            <DefaultRow
              id="team-default-agent-profile"
              label="Agent role"
              value={values.agent_role}
              options={agentOptions}
              disabled={!editable || savingDefault !== null}
              saving={savingDefault === 'agent'}
              onChange={value => void saveAgentProfile(value)}
            />
            {kindOrder.map(kind => {
              const options = buildKnowledgeDefaultOptions(knowledgeContexts, kind, values.knowledge_context[kind] || '', teamPath);
              return (
                <DefaultRow
                  key={kind}
                  id={`team-default-knowledge-${kind}`}
                  label={`${knowledgeKindLabel(kind)} knowledge`}
                  value={values.knowledge_context[kind] || ''}
                  options={options}
                  disabled={!editable || savingDefault !== null}
                  saving={savingDefault === `knowledge:${kind}`}
                  onChange={value => void saveKnowledgeDefault(kind, value)}
                />
              );
            })}
          </div>
        </>
      )}
    </article>
  );
}

function DefaultRow({
  id,
  label,
  value,
  options,
  disabled,
  saving,
  onChange,
}: {
  id: string;
  label: string;
  value: string;
  options: DefaultOption[];
  disabled: boolean;
  saving: boolean;
  onChange: (value: string) => void;
}) {
  const detail = saving ? 'Saving...' : defaultOptionDetail(options, value);
  // The label sits beside the select rather than wrapping it: a wrapping label
  // takes the option text into its accessible name, which leaves the control
  // without a usable name for assistive technology and for tests.
  return (
    <div className="teams-defaults-row">
      <label htmlFor={id}>{label}</label>
      <select
        id={id}
        className="teams-defaults-select"
        value={value}
        disabled={disabled}
        onChange={event => onChange(event.target.value)}
      >
        <option value="">No default selected</option>
        {options.map(option => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
      <small aria-live="polite">{detail}</small>
    </div>
  );
}

function buildLLMDefaultOptions(profiles: TeamLLMProfile[], currentDefault: string, teamPath: string): DefaultOption[] {
  const options = profiles
    .filter(profile => teamProfileDefaultSelectable(profile.name, profile.scope, profile.team_path, teamPath))
    .map(profile => ({
      value: profile.name,
      label: aiProfileLabel(profile.name),
      detail: [profile.provider, profile.model].filter(Boolean).join(' / ') || profile.name,
    }));
  return includeCurrentDefaultOption(options, currentDefault);
}

function buildAgentDefaultOptions(profiles: TeamAgentProfile[], currentDefault: string, teamPath: string): DefaultOption[] {
  const options = profiles
    .filter(profile => profile.enabled !== false)
    .filter(profile => teamProfileDefaultSelectable(profile.id, profile.scope, profile.team_path, teamPath))
    .map(profile => ({
      value: profile.id,
      label: profile.display_name || aiProfileLabel(profile.id),
      detail: [profile.id, profile.role].filter(Boolean).join(' / ') || profile.id,
    }));
  return includeCurrentDefaultOption(options, currentDefault);
}

function buildKnowledgeDefaultOptions(
  documents: KnowledgeContextListItem[],
  kind: string,
  currentDefault: string,
  teamPath: string
): DefaultOption[] {
  const options = documents
    .filter(document => document.kind === kind)
    .filter(document => (teamPath ? document.team === teamPath : !document.team))
    .map(document => {
      const value = buildKnowledgeID(document.kind, document.team, document.name);
      return {
        value,
        label: document.name,
        detail: document.description || [document.kind, document.source].filter(Boolean).join(' / ') || value,
      };
    });
  return includeCurrentDefaultOption(options, currentDefault);
}

function includeCurrentDefaultOption(options: DefaultOption[], currentDefault: string) {
  const normalizedDefault = currentDefault.trim();
  const byValue = new Map(options.map(option => [option.value, option]));
  if (normalizedDefault && !byValue.has(normalizedDefault)) {
    byValue.set(normalizedDefault, {
      value: normalizedDefault,
      label: aiProfileLabel(normalizedDefault),
      detail: normalizedDefault,
    });
  }
  return Array.from(byValue.values()).sort((left, right) => left.label.localeCompare(right.label, undefined, { sensitivity: 'base' }));
}

function teamProfileDefaultSelectable(profileID: string, scope: string | undefined, profileTeamPath: string | undefined, teamPath: string) {
  if (!teamPath) return false;
  const idTeamPath = aiResourceTeamScope(profileID).teamPath;
  if (profileTeamPath === teamPath || idTeamPath === teamPath) return true;
  return scope === 'team' && !idTeamPath && profileTeamPath === teamPath;
}

function aiProfileLabel(profileID: string) {
  return aiResourceLocalName(profileID) || profileID;
}

function defaultOptionDetail(options: DefaultOption[], value: string) {
  return options.find(option => option.value === value)?.detail || '';
}

function knowledgeKindLabel(kind: string) {
  if (kind === 'adr') return 'ADR';
  return kind.charAt(0).toUpperCase() + kind.slice(1);
}

function defaultSummary(llmCount: number, agentCount: number, knowledgeContexts: KnowledgeContextListItem[], teamPath: string) {
  const knowledgeCount = knowledgeContexts.filter(document => (teamPath ? document.team === teamPath : !document.team)).length;
  return `${llmCount} LLM / ${agentCount} agent / ${knowledgeCount} knowledge`;
}
