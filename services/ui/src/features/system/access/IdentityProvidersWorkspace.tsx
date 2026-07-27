import type { Dispatch, FormEvent, SetStateAction } from 'react';
import { Edit3, Trash2 } from 'lucide-react';
import type {
  IdentityProviderFormState,
  IdentityProviderRecord,
  IdentityProviderSettings,
} from './model';
import { CredentialReferenceLink } from '../credentials/CredentialReferenceLink';

type IdentityProvidersWorkspaceProps = {
  providers: IdentityProviderRecord[];
  filteredProviders: IdentityProviderRecord[];
  settings: IdentityProviderSettings;
  domainMappingDraft: string;
  form: IdentityProviderFormState;
  selectedProvider: IdentityProviderRecord | null;
  loading: boolean;
  error: string | null;
  savingSettings: boolean;
  savingProvider: boolean;
  onSettingsChange: Dispatch<SetStateAction<IdentityProviderSettings>>;
  onDomainMappingChange: (value: string) => void;
  onFormChange: Dispatch<SetStateAction<IdentityProviderFormState>>;
  onEdit: (provider: IdentityProviderRecord) => void;
  onCreate: () => void;
  onDelete: (providerID: string) => void;
  onSubmitSettings: (event: FormEvent<HTMLFormElement>) => void;
  onSubmitProvider: (event: FormEvent<HTMLFormElement>) => void;
};

export function IdentityProvidersWorkspace({
  providers,
  filteredProviders,
  settings,
  domainMappingDraft,
  form,
  selectedProvider,
  loading,
  error,
  savingSettings,
  savingProvider,
  onSettingsChange,
  onDomainMappingChange,
  onFormChange,
  onEdit,
  onCreate,
  onDelete,
  onSubmitSettings,
  onSubmitProvider,
}: IdentityProvidersWorkspaceProps) {
  return (
    <div className="access-workspace">
      <div className="space-y-4 access-workspace__list">
        <form className="access-editor-surface access-editor-surface--minimal" onSubmit={onSubmitSettings}>
          <div className="access-editor-header">
            <div>
              <p className="access-editor-kicker">Authentication</p>
              <h5 className="access-editor-title">Login policy</h5>
            </div>
            <button type="submit" className="glass-button-primary" disabled={savingSettings}>
              {savingSettings ? 'Saving...' : 'Save policy'}
            </button>
          </div>
          <div className="access-editor-grid">
            <label className="access-minimal-label">
              <span>Local login</span>
              <input
                className="pipelines-input"
                value="Enabled"
                readOnly
              />
            </label>
            <label className="access-minimal-label">
              <span>External login</span>
              <select
                className="pipelines-input"
                value={settings.oidc_enabled ? 'true' : 'false'}
                onChange={event => onSettingsChange(prev => ({ ...prev, oidc_enabled: event.target.value === 'true' }))}
              >
                <option value="true">Enabled</option>
                <option value="false">Disabled</option>
              </select>
            </label>
            <label className="access-minimal-label">
              <span>Auto-create users</span>
              <select
                className="pipelines-input"
                value={settings.auto_create_users ? 'true' : 'false'}
                onChange={event => onSettingsChange(prev => ({ ...prev, auto_create_users: event.target.value === 'true' }))}
              >
                <option value="false">Disabled</option>
                <option value="true">Enabled</option>
              </select>
            </label>
            <label className="access-minimal-label">
              <span>Email linking</span>
              <select
                className="pipelines-input"
                value={settings.allow_email_linking ? 'true' : 'false'}
                onChange={event => onSettingsChange(prev => ({ ...prev, allow_email_linking: event.target.value === 'true' }))}
              >
                <option value="false">Require explicit link</option>
                <option value="true">Allow verified email link</option>
              </select>
            </label>
            <label className="access-minimal-label">
              <span>Default role</span>
              <input
                className="pipelines-input"
                value={settings.default_role}
                onChange={event => onSettingsChange(prev => ({ ...prev, default_role: event.target.value }))}
                placeholder="viewer"
              />
            </label>
          </div>
          <label className="access-minimal-label">
            <span>Domain mappings</span>
            <textarea
              className="pipelines-input min-h-24"
              value={domainMappingDraft}
              onChange={event => onDomainMappingChange(event.target.value)}
              placeholder={'company.com: corporate\nexample.com: microsoft'}
            />
          </label>
        </form>

        <div className="access-entity-grid access-entity-grid--roles">
          {error && <div className="access-error-banner">{error}</div>}
          {loading ? (
            <div className="access-empty-card">Loading identity providers...</div>
          ) : filteredProviders.length === 0 ? (
            <div className="access-empty-card">{providers.length === 0 ? 'No identity providers configured.' : 'No providers match your search.'}</div>
          ) : (
            filteredProviders.map(provider => (
              <article key={provider.id} className={`access-card ${selectedProvider?.id === provider.id ? 'access-card--selected' : ''}`}>
                <div className="access-card__header">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <p className="access-card__title">{provider.display_name}</p>
                      <span className={`access-status access-status--${provider.enabled ? 'ok' : 'muted'}`}>{provider.enabled ? 'enabled' : 'disabled'}</span>
                    </div>
                    <p className="access-card__subtitle">{provider.type} • {provider.issuer || 'issuer missing'}</p>
                    <p className="access-card__meta-line">
                      {provider.config_source || 'database'}
                      {provider.allowed_email_domains.length ? ` • ${provider.allowed_email_domains.join(', ')}` : ''}
                    </p>
                  </div>
                  <div className="access-card__actions">
                    <button type="button" className="access-card-action" aria-label={`Edit ${provider.display_name}`} onClick={() => onEdit(provider)}>
                      <Edit3 className="h-4 w-4" aria-hidden="true" />
                    </button>
                    <button type="button" className="access-card-action access-card-action--danger" aria-label={`Delete ${provider.display_name}`} onClick={() => onDelete(provider.id)}>
                      <Trash2 className="h-4 w-4" aria-hidden="true" />
                    </button>
                  </div>
                </div>
                <button type="button" className="text-left text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)]" onClick={() => onEdit(provider)}>
                  Client ID: {provider.client_id || 'not configured'}
                </button>
              </article>
            ))
          )}
        </div>
      </div>

      <aside className="access-editor-pane">
        <div className="access-editor-surface access-editor-surface--minimal">
          <div className="access-editor-header">
            <div>
              <p className="access-editor-kicker">{selectedProvider ? 'Edit provider' : 'New provider'}</p>
              <h5 className="access-editor-title">{selectedProvider?.display_name || 'Identity provider'}</h5>
            </div>
            <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={onCreate}>
              New
            </button>
          </div>
          <form className="access-editor-form access-editor-form--compact" onSubmit={onSubmitProvider}>
            <div className="access-editor-grid">
              <ProviderInput label="Provider ID" value={form.id} onChange={value => onFormChange(prev => ({ ...prev, id: value }))} placeholder="corporate" />
              <label className="access-minimal-label">
                <span>Type</span>
                <select className="pipelines-input" value={form.type} onChange={event => onFormChange(prev => ({ ...prev, type: event.target.value }))}>
                  <option value="oidc">Generic OIDC</option>
                  <option value="okta">Okta</option>
                  <option value="keycloak">Keycloak</option>
                  <option value="google">Google</option>
                  <option value="microsoft">Microsoft / Entra ID</option>
                  <option value="github">GitHub OAuth2</option>
                </select>
              </label>
              <ProviderInput label="Display name" value={form.display_name} onChange={value => onFormChange(prev => ({ ...prev, display_name: value }))} placeholder="Company SSO" />
              <ProviderInput label="Issuer" value={form.issuer} onChange={value => onFormChange(prev => ({ ...prev, issuer: value }))} placeholder="https://idp.company.com" />
              <ProviderInput label="Client ID" value={form.client_id} onChange={value => onFormChange(prev => ({ ...prev, client_id: value }))} />
              <ProviderInput label="Client credential ref" value={form.client_credential_ref} onChange={value => onFormChange(prev => ({ ...prev, client_credential_ref: value }))} placeholder="credential://system/oidc/corporate/client-secret" credentialReference={form.client_credential_ref} hint="Expected type: client_secret" />
              <ProviderInput label="Scopes" value={form.scopes} onChange={value => onFormChange(prev => ({ ...prev, scopes: value }))} placeholder="openid, email, profile" />
              <ProviderInput label="Allowed domains" value={form.allowed_email_domains} onChange={value => onFormChange(prev => ({ ...prev, allowed_email_domains: value }))} placeholder="company.com" />
              <ProviderInput label="Authorization endpoint" value={form.authorization_endpoint} onChange={value => onFormChange(prev => ({ ...prev, authorization_endpoint: value }))} />
              <ProviderInput label="Token endpoint" value={form.token_endpoint} onChange={value => onFormChange(prev => ({ ...prev, token_endpoint: value }))} />
              <ProviderInput label="JWKS URI" value={form.jwks_uri} onChange={value => onFormChange(prev => ({ ...prev, jwks_uri: value }))} />
              <ProviderInput label="UserInfo endpoint" value={form.userinfo_endpoint} onChange={value => onFormChange(prev => ({ ...prev, userinfo_endpoint: value }))} />
              <ProviderInput label="Team claim" value={form.team_claim} onChange={value => onFormChange(prev => ({ ...prev, team_claim: value }))} placeholder="teams" />
              <ProviderInput label="Default role" value={form.default_role} onChange={value => onFormChange(prev => ({ ...prev, default_role: value }))} placeholder="viewer" />
              <label className="access-minimal-label">
                <span>Auto-create users</span>
                <select className="pipelines-input" value={form.auto_create_users} onChange={event => onFormChange(prev => ({ ...prev, auto_create_users: event.target.value }))}>
                  <option value="inherit">Inherit policy</option>
                  <option value="false">Disabled</option>
                  <option value="true">Enabled</option>
                </select>
              </label>
              <label className="access-minimal-label">
                <span>Email linking</span>
                <select className="pipelines-input" value={form.allow_email_linking} onChange={event => onFormChange(prev => ({ ...prev, allow_email_linking: event.target.value }))}>
                  <option value="inherit">Inherit policy</option>
                  <option value="false">Require explicit link</option>
                  <option value="true">Allow verified email link</option>
                </select>
              </label>
              <label className="access-minimal-label">
                <span>Status</span>
                <select className="pipelines-input" value={form.enabled ? 'true' : 'false'} onChange={event => onFormChange(prev => ({ ...prev, enabled: event.target.value === 'true' }))}>
                  <option value="true">Enabled</option>
                  <option value="false">Disabled</option>
                </select>
              </label>
            </div>
            <label className="access-minimal-label">
              <span>Role mappings</span>
              <textarea
                className="pipelines-input min-h-24"
                value={form.role_mapping}
                onChange={event => onFormChange(prev => ({ ...prev, role_mapping: event.target.value }))}
                placeholder={'nopsai-admins: admin\nnopsai-viewers: viewer'}
              />
            </label>
            <label className="access-minimal-label">
              <span>Auth team mappings</span>
              <textarea
                className="pipelines-input min-h-24"
                value={form.team_mapping}
                onChange={event => onFormChange(prev => ({ ...prev, team_mapping: event.target.value }))}
                placeholder={'engineering: Engineering\nrelease-managers: Release Managers'}
              />
              <p className="text-xs text-[var(--text-secondary)]">
                Maps identity-provider teams to NopsAI auth teams for basic/scoped grants.
              </p>
            </label>
            <label className="access-minimal-label">
              <span>Basic role mappings</span>
              <textarea
                className="pipelines-input min-h-24"
                value={form.basic_role_mapping}
                onChange={event => onFormChange(prev => ({ ...prev, basic_role_mapping: event.target.value }))}
                placeholder={'team-1-owner: owner team:team-1\nteam-1-dev-viewer: viewer team:team-1/dev'}
              />
              <p className="text-xs text-[var(--text-secondary)]">
                Syncs identity-provider teams directly into scoped NopsAI basic roles.
              </p>
            </label>
            <div className="access-editor-footer">
              <button type="submit" className="glass-button-primary" disabled={savingProvider}>
                {savingProvider ? 'Saving...' : 'Save provider'}
              </button>
            </div>
          </form>
        </div>
      </aside>
    </div>
  );
}

function ProviderInput({
  label,
  value,
  onChange,
  placeholder,
  type = 'text',
  credentialReference,
  hint,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: string;
  credentialReference?: string;
  hint?: string;
}) {
  return (
    <label className="access-minimal-label">
      <span className="flex flex-wrap items-center gap-2">
        <span>{label}</span>
        {credentialReference ? (
          <CredentialReferenceLink reference={credentialReference} className="text-xs underline decoration-dotted underline-offset-4 hover:text-[var(--accent-primary)]">
            Open credential
          </CredentialReferenceLink>
        ) : null}
      </span>
      <input
        className="pipelines-input"
        type={type}
        value={value}
        onChange={event => onChange(event.target.value)}
        placeholder={placeholder}
      />
      {hint ? <span className="text-xs text-[var(--text-secondary)]">{hint}</span> : null}
    </label>
  );
}
