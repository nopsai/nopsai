import type { ChangeEvent } from 'react';
import { ApplyBadge } from './ConfigApplyBadge';
import type { ConfigFieldMetadata, ConfigFormState } from './model';
import { CredentialReferenceLink } from '../credentials/CredentialReferenceLink';

type GitHubAppConfigKey =
  | 'github_app_id'
  | 'github_installation_id'
  | 'github_private_key_credential_ref'
  | 'github_webhook_credential_ref';

type GitHubAppSettingsCardProps = {
  config: ConfigFormState;
  fieldMetadata: Record<string, ConfigFieldMetadata>;
  disabled: boolean;
  onChange: (next: ConfigFormState) => void;
};

function GitHubAppSettingsCard({ config, fieldMetadata, disabled, onChange }: GitHubAppSettingsCardProps) {
  const handleChange = (key: GitHubAppConfigKey) => (event: ChangeEvent<HTMLInputElement>) => {
    const value = event.target.value;
    onChange({ ...config, [key]: value } as ConfigFormState);
  };

  const labelWithApply = (label: string, key: GitHubAppConfigKey) => (
    <span className="flex flex-wrap items-center gap-2">
      <span>{label}</span>
      <ApplyBadge metadata={fieldMetadata[key]} />
    </span>
  );

  return (
    <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
      <div>
        <p className="text-xs text-[var(--text-secondary)]">GitHub App</p>
        <h3 className="text-lg font-semibold text-[var(--text-primary)]">git-bot application</h3>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <label className="flex flex-col gap-1 text-sm">
          {labelWithApply('GitHub App ID', 'github_app_id')}
          <input
            id="system-github-app-id"
            type="text"
            inputMode="numeric"
            className="pipelines-input"
            value={config.github_app_id}
            onChange={handleChange('github_app_id')}
            placeholder="123456"
            disabled={disabled}
          />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          {labelWithApply('GitHub installation ID', 'github_installation_id')}
          <input
            id="system-github-installation-id"
            type="text"
            inputMode="numeric"
            className="pipelines-input"
            value={config.github_installation_id}
            onChange={handleChange('github_installation_id')}
            placeholder="987654"
            disabled={disabled}
          />
        </label>
        <label className="flex flex-col gap-1 text-sm md:col-span-2">
          {labelWithApply('Private key credential ref', 'github_private_key_credential_ref')}
          <input
            id="system-github-private-key-ref"
            type="text"
            className="pipelines-input"
            value={config.github_private_key_credential_ref}
            onChange={handleChange('github_private_key_credential_ref')}
            placeholder="credential://system/github/app-private-key"
            disabled={disabled}
          />
          <CredentialReferenceLink reference={config.github_private_key_credential_ref} className="text-xs underline decoration-dotted underline-offset-4 hover:text-[var(--accent-primary)]">
            Open credential
          </CredentialReferenceLink>
        </label>
        <label className="flex flex-col gap-1 text-sm md:col-span-2">
          {labelWithApply('Webhook secret credential ref', 'github_webhook_credential_ref')}
          <input
            id="system-github-webhook-ref"
            type="text"
            className="pipelines-input"
            value={config.github_webhook_credential_ref}
            onChange={handleChange('github_webhook_credential_ref')}
            placeholder="credential://system/github/webhook-secret"
            disabled={disabled}
          />
          <CredentialReferenceLink reference={config.github_webhook_credential_ref} className="text-xs underline decoration-dotted underline-offset-4 hover:text-[var(--accent-primary)]">
            Open credential
          </CredentialReferenceLink>
        </label>
      </div>
    </div>
  );
}

export default GitHubAppSettingsCard;
