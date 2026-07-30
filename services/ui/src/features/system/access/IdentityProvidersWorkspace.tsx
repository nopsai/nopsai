import type {
  Dispatch,
  FormEvent,
  KeyboardEvent,
  MouseEvent,
  ReactNode,
  SetStateAction,
} from 'react';
import { useMemo, useState } from 'react';
import { Edit3, Trash2 } from 'lucide-react';
import type {
  IdentityProviderFormState,
  IdentityProviderRecord,
  IdentityProviderSettings,
} from './model';
import { AccessEditorDrawer } from './AccessEditorDrawer';
import { CredentialReferenceLink } from '../credentials/CredentialReferenceLink';

type ProviderEditorSection = 'details' | 'connection' | 'login' | 'mappings' | 'review';

const PROVIDER_EDITOR_SECTIONS: Array<{
  id: ProviderEditorSection;
  label: string;
  description: string;
}> = [
  { id: 'details', label: 'Provider', description: 'Name and status' },
  { id: 'connection', label: 'Connection', description: 'OIDC endpoints' },
  { id: 'login', label: 'Login policy', description: 'Provisioning defaults' },
  { id: 'mappings', label: 'Mappings', description: 'Groups and roles' },
  { id: 'review', label: 'Review', description: 'Save impact' },
];

function countConfiguredLines(value: string) {
  return value
    .split('\n')
    .map(line => line.trim())
    .filter(Boolean).length;
}

function countCSV(value: string) {
  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean).length;
}

function inheritLabel(value: string, enabledLabel: string, disabledLabel: string) {
  if (value === 'true') return enabledLabel;
  if (value === 'false') return disabledLabel;
  return 'Inherit policy';
}

type IdentityProvidersWorkspaceProps = {
  providers: IdentityProviderRecord[];
  filteredProviders: IdentityProviderRecord[];
  settings: IdentityProviderSettings;
  form: IdentityProviderFormState;
  selectedProvider: IdentityProviderRecord | null;
  editorOpen: boolean;
  loading: boolean;
  error: string | null;
  savingProvider: boolean;
  onFormChange: Dispatch<SetStateAction<IdentityProviderFormState>>;
  onEdit: (provider: IdentityProviderRecord) => void;
  onCreate: () => void;
  onCloseEditor: () => void;
  onDelete: (providerID: string) => void;
  onSubmitProvider: (event: FormEvent<HTMLFormElement>) => void;
};

export function IdentityProvidersWorkspace({
  providers,
  filteredProviders,
  settings,
  form,
  selectedProvider,
  editorOpen,
  loading,
  error,
  savingProvider,
  onFormChange,
  onEdit,
  onCreate,
  onCloseEditor,
  onDelete,
  onSubmitProvider,
}: IdentityProvidersWorkspaceProps) {
  const [activeEditorSection, setActiveEditorSection] =
    useState<ProviderEditorSection>('details');
  const providerModeLabel = selectedProvider ? 'Edit provider' : 'New provider';
  const providerSource = selectedProvider?.config_source || 'database';
  const providerCredentialLabel = form.client_credential_ref.trim()
    ? 'Credential reference'
    : selectedProvider?.has_client_credential
      ? 'Stored credential'
      : 'Not configured';
  const providerSummary = useMemo(
    () => ({
      scopes: countCSV(form.scopes),
      domains: countCSV(form.allowed_email_domains),
      endpoints: [
        form.authorization_endpoint,
        form.token_endpoint,
        form.jwks_uri,
        form.userinfo_endpoint,
      ].filter(value => value.trim()).length,
      roleMappings: countConfiguredLines(form.role_mapping),
      teamMappings: countConfiguredLines(form.team_mapping),
      basicMappings: countConfiguredLines(form.basic_role_mapping),
    }),
    [form],
  );
  const openProviderEditor = (provider: IdentityProviderRecord) => {
    setActiveEditorSection('details');
    onEdit(provider);
  };
  const openProviderCreator = () => {
    setActiveEditorSection('details');
    onCreate();
  };
  const activateProvider = (
    event: KeyboardEvent<HTMLTableRowElement>,
    provider: IdentityProviderRecord,
  ) => {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    openProviderEditor(provider);
  };
  const runRowAction = (
    event: MouseEvent<HTMLButtonElement>,
    action: () => void,
  ) => {
    event.stopPropagation();
    action();
  };

  return (
    <div className="access-workspace">
      <div className="space-y-4 access-workspace__list">
        <div className="access-table-shell" aria-label="Identity providers">
          {error && <div className="access-error-banner">{error}</div>}
          {loading ? (
            <div className="access-empty-card">Loading identity providers...</div>
          ) : filteredProviders.length === 0 ? (
            <div className="access-empty-card">{providers.length === 0 ? 'No identity providers configured.' : 'No providers match your search.'}</div>
          ) : (
            <div className="access-table-wrap">
              <table className="access-table access-table--providers">
                <thead>
                  <tr>
                    <th>Provider</th>
                    <th>Type</th>
                    <th>Allowed domains</th>
                    <th>Source</th>
                    <th className="access-table__right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredProviders.map(provider => (
                    <tr
                      key={provider.id}
                      tabIndex={0}
                      className={`access-table-row ${selectedProvider?.id === provider.id ? 'access-table-row--selected' : ''}`}
                      onClick={() => openProviderEditor(provider)}
                      onKeyDown={event => activateProvider(event, provider)}
                    >
                      <td>
                        <div className="access-table-entity">
                          <div className="access-avatar">ID</div>
                          <div className="access-table-entity__copy">
                            <div className="access-table-entity__title">
                              <span className="access-table-title-line">
                                {provider.display_name}
                                <span
                                  className={`access-status-dot access-status-dot--${provider.enabled ? 'ok' : 'muted'}`}
                                  title={provider.enabled ? 'Enabled' : 'Disabled'}
                                  aria-label={`Status: ${provider.enabled ? 'Enabled' : 'Disabled'}`}
                                />
                              </span>
                            </div>
                            <div className="access-table-entity__meta">{provider.issuer || 'issuer missing'}</div>
                            <div className="access-table-entity__detail">Client ID: {provider.client_id || 'not configured'}</div>
                          </div>
                        </div>
                      </td>
                      <td>
                        <span className="access-chip access-chip--brand">{provider.type}</span>
                      </td>
                      <td>
                        {provider.allowed_email_domains.length ? (
                          <div className="access-chip-list access-chip-list--compact">
                            {provider.allowed_email_domains.slice(0, 2).map(domain => (
                              <span key={`${provider.id}-${domain}`} className="access-chip access-chip--team">
                                {domain}
                              </span>
                            ))}
                            {provider.allowed_email_domains.length > 2 ? (
                              <span className="access-chip access-chip--more">
                                +{provider.allowed_email_domains.length - 2}
                              </span>
                            ) : null}
                          </div>
                        ) : (
                          <span className="access-cell-muted">Any verified domain</span>
                        )}
                      </td>
                      <td className="access-cell-muted">
                        {provider.config_source || 'database'}
                      </td>
                      <td className="access-table__right">
                        <span className="access-row-actions">
                          <button
                            type="button"
                            className="access-card-action"
                            aria-label={`Edit ${provider.display_name}`}
                            onClick={event => runRowAction(event, () => openProviderEditor(provider))}
                          >
                            <Edit3 className="h-4 w-4" aria-hidden="true" />
                          </button>
                          <button
                            type="button"
                            className="access-card-action access-card-action--danger"
                            aria-label={`Delete ${provider.display_name}`}
                            onClick={event => runRowAction(event, () => onDelete(provider.id))}
                          >
                            <Trash2 className="h-4 w-4" aria-hidden="true" />
                          </button>
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      <AccessEditorDrawer
        open={editorOpen}
        label="Identity provider editor"
        onClose={onCloseEditor}
      >
        <div className="access-editor-surface access-editor-surface--minimal access-provider-editor">
          <div className="access-editor-header">
            <div>
              <p className="access-editor-kicker">{providerModeLabel}</p>
              <h5 className="access-editor-title">{selectedProvider?.display_name || 'Identity provider'}</h5>
            </div>
            <div className="access-editor-header__actions">
              <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={openProviderCreator}>
                New
              </button>
              <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={onCloseEditor}>
                Close
              </button>
            </div>
          </div>
          <form className="access-editor-form access-editor-form--compact access-provider-editor__form" onSubmit={onSubmitProvider}>
            <div className="access-provider-editor__layout">
              <ProviderEditorNav
                activeSection={activeEditorSection}
                onChange={setActiveEditorSection}
              />

              <div className="access-provider-editor__content">

            {activeEditorSection === 'details' && (
              <ProviderFormCard
                title="Provider details"
                description="Stable identity and operator-facing metadata."
                badge={form.enabled ? 'Enabled' : 'Disabled'}
              >
                <div className="access-provider-field-stack">
                  <ProviderInput
                    label="Provider ID"
                    value={form.id}
                    onChange={value => onFormChange(prev => ({ ...prev, id: value }))}
                    placeholder="corporate"
                    disabled={Boolean(selectedProvider)}
                  />
                  <ProviderInput
                    label="Display name"
                    value={form.display_name}
                    onChange={value => onFormChange(prev => ({ ...prev, display_name: value }))}
                    placeholder="Company SSO"
                  />
                  <div className="access-provider-select-grid">
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
                    <label className="access-minimal-label">
                      <span>Status</span>
                      <select className="pipelines-input" value={form.enabled ? 'true' : 'false'} onChange={event => onFormChange(prev => ({ ...prev, enabled: event.target.value === 'true' }))}>
                        <option value="true">Enabled</option>
                        <option value="false">Disabled</option>
                      </select>
                    </label>
                  </div>
                  <label className="access-minimal-label">
                    <span>Config source</span>
                    <input className="pipelines-input" value={providerSource} readOnly />
                  </label>
                  <ProviderInput
                    label="Default role"
                    value={form.default_role}
                    onChange={value => onFormChange(prev => ({ ...prev, default_role: value }))}
                    placeholder={settings.default_role || 'viewer'}
                  />
                </div>
              </ProviderFormCard>
            )}

            {activeEditorSection === 'connection' && (
              <div className="access-form-stack">
                <ProviderFormCard
                  title="OIDC connection"
                  description="Issuer, client identity, scopes, and discovery endpoints."
                  badge={providerCredentialLabel}
                >
                  <div className="access-provider-field-stack">
                    <ProviderInput
                      label="Issuer"
                      value={form.issuer}
                      onChange={value => onFormChange(prev => ({ ...prev, issuer: value }))}
                      placeholder="https://idp.company.com"
                      type="url"
                    />
                    <ProviderInput label="Client ID" value={form.client_id} onChange={value => onFormChange(prev => ({ ...prev, client_id: value }))} />
                    <ProviderInput label="Client credential ref" value={form.client_credential_ref} onChange={value => onFormChange(prev => ({ ...prev, client_credential_ref: value }))} placeholder="credential://system/oidc/corporate/client-secret" credentialReference={form.client_credential_ref} hint="Expected type: client_secret" />
                    <ProviderInput label="Scopes" value={form.scopes} onChange={value => onFormChange(prev => ({ ...prev, scopes: value }))} placeholder="openid, email, profile" />
                    <ProviderInput label="Allowed domains" value={form.allowed_email_domains} onChange={value => onFormChange(prev => ({ ...prev, allowed_email_domains: value }))} placeholder="company.com" />
                    <ProviderInput label="Team claim" value={form.team_claim} onChange={value => onFormChange(prev => ({ ...prev, team_claim: value }))} placeholder="teams" />
                  </div>
                </ProviderFormCard>
                <ProviderFormCard
                  title="Explicit endpoints"
                  description="Optional overrides when discovery metadata is incomplete."
                  badge={`${providerSummary.endpoints} configured`}
                >
                  <div className="access-provider-field-stack">
                    <ProviderInput label="Authorization endpoint" value={form.authorization_endpoint} onChange={value => onFormChange(prev => ({ ...prev, authorization_endpoint: value }))} type="url" />
                    <ProviderInput label="Token endpoint" value={form.token_endpoint} onChange={value => onFormChange(prev => ({ ...prev, token_endpoint: value }))} type="url" />
                    <ProviderInput label="JWKS URI" value={form.jwks_uri} onChange={value => onFormChange(prev => ({ ...prev, jwks_uri: value }))} type="url" />
                    <ProviderInput label="UserInfo endpoint" value={form.userinfo_endpoint} onChange={value => onFormChange(prev => ({ ...prev, userinfo_endpoint: value }))} type="url" />
                  </div>
                </ProviderFormCard>
              </div>
            )}

            {activeEditorSection === 'login' && (
              <ProviderFormCard
                title="Provider login policy"
                description="Per-provider provisioning overrides inherit the global policy unless set here."
                badge={form.enabled ? 'Accepting login' : 'Paused'}
              >
                <div className="access-provider-field-stack">
                  <div className="access-provider-select-grid">
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
                  </div>
                  <ProviderInput label="Default role" value={form.default_role} onChange={value => onFormChange(prev => ({ ...prev, default_role: value }))} placeholder={settings.default_role || 'viewer'} />
                  <label className="access-minimal-label">
                    <span>Global external login</span>
                    <input className="pipelines-input" value={settings.oidc_enabled ? 'Enabled' : 'Disabled'} readOnly />
                  </label>
                  <label className="access-minimal-label">
                    <span>Global user creation</span>
                    <input className="pipelines-input" value={settings.auto_create_users ? 'Enabled' : 'Disabled'} readOnly />
                  </label>
                  <label className="access-minimal-label">
                    <span>Global email linking</span>
                    <input className="pipelines-input" value={settings.allow_email_linking ? 'Allowed' : 'Explicit link required'} readOnly />
                  </label>
                </div>
              </ProviderFormCard>
            )}

            {activeEditorSection === 'mappings' && (
              <div className="access-form-stack">
                <ProviderFormCard
                  title="Role mappings"
                  description="Map external groups to reusable access roles."
                  badge={`${providerSummary.roleMappings} mapped`}
                >
                  <ProviderTextarea
                    label="Role mappings"
                    value={form.role_mapping}
                    onChange={value => onFormChange(prev => ({ ...prev, role_mapping: value }))}
                    placeholder={'nopsai-admins: admin\nnopsai-viewers: viewer'}
                  />
                </ProviderFormCard>
                <ProviderFormCard
                  title="Auth team mappings"
                  description="Map external groups to NopsAI auth teams."
                  badge={`${providerSummary.teamMappings} mapped`}
                >
                  <ProviderTextarea
                    label="Auth team mappings"
                    value={form.team_mapping}
                    onChange={value => onFormChange(prev => ({ ...prev, team_mapping: value }))}
                    placeholder={'engineering: Engineering\nrelease-managers: Release Managers'}
                  />
                </ProviderFormCard>
                <ProviderFormCard
                  title="Basic role mappings"
                  description="Sync external groups into scoped NopsAI basic roles."
                  badge={`${providerSummary.basicMappings} mapped`}
                >
                  <ProviderTextarea
                    label="Basic role mappings"
                    value={form.basic_role_mapping}
                    onChange={value => onFormChange(prev => ({ ...prev, basic_role_mapping: value }))}
                    placeholder={'team-1-owner: owner team:team-1\nteam-1-dev-viewer: viewer team:team-1/dev'}
                  />
                </ProviderFormCard>
              </div>
            )}

            {activeEditorSection === 'review' && (
              <div className="access-form-stack">
                <div className="access-review-grid">
                  <ProviderReviewStat label="Status" value={form.enabled ? 'Enabled' : 'Disabled'} />
                  <ProviderReviewStat label="Domains" value={String(providerSummary.domains)} />
                  <ProviderReviewStat label="Scopes" value={String(providerSummary.scopes)} />
                  <ProviderReviewStat label="Mappings" value={String(providerSummary.roleMappings + providerSummary.teamMappings + providerSummary.basicMappings)} />
                </div>
                <ProviderFormCard
                  title="Provider summary"
                  description="Configuration that will be submitted for this provider."
                  badge={selectedProvider ? 'Update' : 'Create'}
                >
                  <dl className="access-review-list">
                    <ProviderReviewRow label="Provider ID" value={form.id || 'Not entered'} />
                    <ProviderReviewRow label="Display name" value={form.display_name || 'Not entered'} />
                    <ProviderReviewRow label="Type" value={form.type || 'oidc'} />
                    <ProviderReviewRow label="Issuer" value={form.issuer || 'Not entered'} />
                    <ProviderReviewRow label="Client ID" value={form.client_id || 'Not entered'} />
                    <ProviderReviewRow label="Credential" value={providerCredentialLabel} />
                    <ProviderReviewRow label="Auto-create users" value={inheritLabel(form.auto_create_users, 'Enabled', 'Disabled')} />
                    <ProviderReviewRow label="Email linking" value={inheritLabel(form.allow_email_linking, 'Allowed', 'Explicit link required')} />
                    <ProviderReviewRow label="Default role" value={form.default_role || settings.default_role || 'Not set'} />
                  </dl>
                </ProviderFormCard>
              </div>
            )}

              </div>
            </div>

            <div className="access-editor-footer access-editor-footer--provider access-sectioned-editor__footer">
              <div className="access-sectioned-editor__footer-left">
                {selectedProvider ? (
                  <button
                    type="button"
                    className="access-inline-btn access-inline-btn--danger access-sectioned-editor__delete"
                    disabled={savingProvider}
                    onClick={() => {
                      onCloseEditor();
                      onDelete(selectedProvider.id);
                    }}
                  >
                    <Trash2 className="h-4 w-4" aria-hidden="true" />
                    <span>Delete provider</span>
                  </button>
                ) : null}
              </div>
              <div className="access-sectioned-editor__footer-right">
                <button
                  type="button"
                  className="access-inline-btn access-inline-btn--pill"
                  onClick={onCloseEditor}
                  disabled={savingProvider}
                >
                  Cancel
                </button>
                <button type="submit" className="glass-button-primary" disabled={savingProvider}>
                  {savingProvider ? 'Saving...' : 'Save provider'}
                </button>
              </div>
            </div>
          </form>
        </div>
      </AccessEditorDrawer>
    </div>
  );
}

function ProviderEditorNav({
  activeSection,
  onChange,
}: {
  activeSection: ProviderEditorSection;
  onChange: (section: ProviderEditorSection) => void;
}) {
  return (
    <nav className="access-provider-nav" aria-label="Identity provider sections">
      {PROVIDER_EDITOR_SECTIONS.map((section, index) => (
        <button
          key={section.id}
          type="button"
          className={`access-provider-nav__button ${activeSection === section.id ? 'access-provider-nav__button--active' : ''}`}
          onClick={() => onChange(section.id)}
        >
          <span className="access-provider-nav__number">{index + 1}</span>
          <span className="access-provider-nav__copy">
            <strong>{section.label}</strong>
            <span>{section.description}</span>
          </span>
        </button>
      ))}
    </nav>
  );
}

function ProviderFormCard({
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
    <section
      className="access-provider-form-card"
      aria-label={description ? `${title}: ${description}` : title}
    >
      {badge ? (
        <div className="access-provider-form-card__header">
          <span className="access-provider-form-card__badge">{badge}</span>
        </div>
      ) : null}
      <div className="access-provider-form-card__body">{children}</div>
    </section>
  );
}

function ProviderTextarea({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}) {
  return (
    <label className="access-minimal-label">
      <span>{label}</span>
      <textarea
        className="pipelines-input min-h-24"
        value={value}
        onChange={event => onChange(event.target.value)}
        placeholder={placeholder}
      />
    </label>
  );
}

function ProviderReviewStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="access-review-stat">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function ProviderReviewRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="access-review-row">
      <dt>{label}</dt>
      <dd>{value}</dd>
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
  disabled = false,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: string;
  credentialReference?: string;
  hint?: string;
  disabled?: boolean;
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
        disabled={disabled}
      />
      {hint ? <span className="text-xs text-[var(--text-secondary)]">{hint}</span> : null}
    </label>
  );
}
