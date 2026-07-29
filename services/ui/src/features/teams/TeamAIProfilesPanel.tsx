import { Bot, BrainCircuit, Route } from 'lucide-react';
import { Link } from 'react-router-dom';
import type {
  TeamAgentProfile,
  TeamAgentProfilesResponse,
  TeamLLMProfile,
  TeamLLMProfilesResponse,
  TeamMCPProfile,
  TeamMCPProfilesResponse,
} from './api';

export function TeamAIProfilesPanel({
  llmProfiles,
  agentProfiles,
  mcpProfiles,
  loading,
  error,
  teamPath,
  gitOpsTarget,
}: {
  llmProfiles: TeamLLMProfilesResponse | null;
  agentProfiles: TeamAgentProfilesResponse | null;
  mcpProfiles: TeamMCPProfilesResponse | null;
  loading: boolean;
  error: string | null;
  teamPath: string;
  gitOpsTarget: string;
}) {
  const globalScope = !teamPath;
  const encodedTeam = encodeURIComponent(teamPath);
  const teamQuery = teamPath ? `?team=${encodedTeam}` : '?team=global';
  const title = globalScope ? 'Global AI Profiles' : 'Team AI Profiles';
  const description = gitOpsTarget
    ? `GitOps target: ${gitOpsTarget}`
    : globalScope
      ? 'Global profile defaults and tool access.'
      : 'No config repository connected';
  const llms = [...(llmProfiles?.profiles ?? [])].sort((left, right) => left.name.localeCompare(right.name));
  const agents = [...(agentProfiles?.profiles ?? [])].sort((left, right) => left.id.localeCompare(right.id));
  const mcps = [...(mcpProfiles?.profiles ?? [])].sort((left, right) => left.name.localeCompare(right.name));

  return (
    <article className="teams-card teams-focus-card teams-focus-card--wide">
      <div className="teams-focus-hero">
        <span className="teams-resource-icon teams-tone-purple" aria-hidden="true">
          <Bot className="h-5 w-5" />
        </span>
        <div>
          <h3>{title}</h3>
          <p>{description}</p>
        </div>
        <div className="teams-focus-actions">
          <Link className="teams-secondary-btn" to={`/llm-profiles${teamQuery}`}>
            <BrainCircuit className="h-4 w-4" aria-hidden="true" />
            LLM Profiles
          </Link>
          <Link className="teams-secondary-btn" to={`/agent-profiles${teamQuery}`}>
            <Bot className="h-4 w-4" aria-hidden="true" />
            Agent Profiles
          </Link>
          <Link className="teams-secondary-btn" to={`/mcp/profiles${teamQuery}`}>
            <Route className="h-4 w-4" aria-hidden="true" />
            MCP
          </Link>
        </div>
      </div>

      {loading ? (
        <div className="teams-inline-status">Loading AI profiles...</div>
      ) : (
        <>
          {error ? <div className="teams-inline-status teams-inline-status--error">{error}</div> : null}
          <div className="teams-focus-grid teams-focus-grid--three">
            <div className="teams-focus-metric">
              <span>LLM profiles</span>
              <strong>{llms.length}</strong>
            </div>
            <div className="teams-focus-metric">
              <span>Agent profiles</span>
              <strong>{agents.length}</strong>
            </div>
            <div className="teams-focus-metric">
              <span>MCP profiles</span>
              <strong>{mcps.length}</strong>
            </div>
          </div>
          <div className="teams-profile-summary-grid">
            <ProfileSummarySection
              title="LLM"
              emptyLabel={globalScope ? 'No global LLM profiles' : 'No team LLM profiles'}
              rows={llms.map(profile => ({
                key: profile.name,
                title: profile.name,
                subtitle: llmProfileSubtitle(profile),
                meta: profile.name === llmProfiles?.default_profile ? ['Default', ...(profile.allowed_scopes ?? [])] : profile.allowed_scopes ?? [],
              }))}
            />
            <ProfileSummarySection
              title="Agents"
              emptyLabel={globalScope ? 'No global agent profiles' : 'No team agent profiles'}
              rows={agents.map(profile => ({
                key: profile.id,
                title: profile.display_name || profile.id,
                subtitle: agentProfileSubtitle(profile),
                meta: [
                  profile.id === agentProfiles?.default_profile ? 'Default' : '',
                  profile.enabled === false ? 'Disabled' : 'Enabled',
                ].filter(Boolean),
              }))}
            />
            <ProfileSummarySection
              title="MCP"
              emptyLabel={globalScope ? 'No global MCP profiles' : 'No team MCP profiles'}
              rows={mcps.map(profile => ({
                key: profile.name,
                title: profile.name,
                subtitle: mcpProfileSubtitle(profile),
                meta: [
                  profile.enabled === false ? 'Disabled' : 'Enabled',
                  ...(profile.allowed_scopes ?? []),
                ],
              }))}
            />
          </div>
        </>
      )}
    </article>
  );
}

function ProfileSummarySection({
  title,
  rows,
  emptyLabel,
}: {
  title: string;
  emptyLabel: string;
  rows: Array<{ key: string; title: string; subtitle: string; meta: string[] }>;
}) {
  return (
    <section className="teams-profile-summary" aria-label={`${title} profiles`}>
      <div className="teams-table-heading">
        <h3>{title}</h3>
        <span>{rows.length} items</span>
      </div>
      {rows.length === 0 ? (
        <div className="teams-profile-summary__empty">{emptyLabel}</div>
      ) : (
        <div className="teams-profile-summary__list">
          {rows.map(row => (
            <div key={row.key} className="teams-profile-summary__row">
              <div className="min-w-0">
                <strong title={row.title}>{row.title}</strong>
                <p title={row.subtitle}>{row.subtitle || '-'}</p>
              </div>
              {row.meta.length > 0 ? (
                <div className="teams-profile-summary__tags">
                  {row.meta.slice(0, 3).map((tag, index) => (
                    <span key={`${tag}-${index}`} className="runner-pill runner-pill--muted">{tag}</span>
                  ))}
                </div>
              ) : null}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function llmProfileSubtitle(profile: TeamLLMProfile) {
  return [profile.provider, profile.model, profile.credential_ref].filter(Boolean).join(' / ');
}

function agentProfileSubtitle(profile: TeamAgentProfile) {
  return [profile.id, profile.role, profile.description].filter(Boolean).join(' / ');
}

function mcpProfileSubtitle(profile: TeamMCPProfile) {
  const servers = (profile.servers ?? []).map(ref => `${ref.server}${ref.tools?.length ? `:${ref.tools.join(',')}` : ''}`);
  return servers.join(', ') || profile.description || 'No servers';
}
