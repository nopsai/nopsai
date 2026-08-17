import { fetchSystemJson } from '../api.js';
import {
  gitHubAppConnectPayload,
  gitHubAppInstallationPayloadFromForm,
  gitHubAppPayloadFromForm,
  normalizeGitHubAppInstallation,
  normalizeGitHubAppInstallationRepository,
  normalizeGitHubAppPayload,
  type GitHubAppConnectFormState,
  type GitHubAppFormState,
  type GitHubAppInstallStart,
  type GitHubAppInstallation,
  type GitHubAppInstallationFormState,
  type GitHubAppInstallationRepository,
  type GitHubAppRegistrationStart,
  type GitHubAppResource,
} from './model.js';

export async function startGitHubAppRegistration(
  form: GitHubAppConnectFormState
): Promise<GitHubAppRegistrationStart> {
  const payload = await fetchSystemJson('/v1/git-apps/github/register/start', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(gitHubAppConnectPayload(form)),
  }) as Partial<GitHubAppRegistrationStart> | null;
  return {
    state: String(payload?.state || ''),
    post_url: String(payload?.post_url || ''),
    manifest: String(payload?.manifest || ''),
    app_name: String(payload?.app_name || ''),
    webhook_endpoint: String(payload?.webhook_endpoint || ''),
    expires_at: String(payload?.expires_at || ''),
  };
}

export async function startGitHubAppInstall(): Promise<GitHubAppInstallStart> {
  const payload = await fetchSystemJson('/v1/git-apps/github/install/start', {
    method: 'POST',
  }) as Partial<GitHubAppInstallStart> | null;
  return {
    state: String(payload?.state || ''),
    install_url: String(payload?.install_url || ''),
    expires_at: String(payload?.expires_at || ''),
  };
}

export async function fetchGitHubApp(): Promise<GitHubAppResource> {
  return normalizeGitHubAppPayload(await fetchSystemJson('/v1/git-apps/github'));
}

export async function saveGitHubApp(
  form: GitHubAppFormState,
  installations: readonly GitHubAppInstallation[]
): Promise<GitHubAppResource> {
  return normalizeGitHubAppPayload(await fetchSystemJson('/v1/git-apps/github', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(gitHubAppPayloadFromForm(form, installations)),
  }));
}

export async function saveGitHubAppInstallation(
  form: GitHubAppInstallationFormState
): Promise<GitHubAppInstallation> {
  return normalizeGitHubAppInstallation(await fetchSystemJson('/v1/git-apps/github/installations', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(gitHubAppInstallationPayloadFromForm(form)),
  }));
}

export async function deleteGitHubAppInstallation(installationID: string): Promise<void> {
  await fetchSystemJson(`/v1/git-apps/github/installations/${encodeURIComponent(installationID)}`, {
    method: 'DELETE',
  });
}

export async function verifyGitHubAppInstallation(installationID: string): Promise<GitHubAppInstallation> {
  return normalizeGitHubAppInstallation(await fetchSystemJson(
    `/v1/git-apps/github/installations/${encodeURIComponent(installationID)}/verify`,
    { method: 'POST' }
  ));
}

export async function refreshGitHubAppInstallation(installationID: string): Promise<GitHubAppInstallation> {
  return normalizeGitHubAppInstallation(await fetchSystemJson(
    `/v1/git-apps/github/installations/${encodeURIComponent(installationID)}/refresh`,
    { method: 'POST' }
  ));
}

export async function fetchGitHubAppInstallationRepositories(
  installationID: string
): Promise<GitHubAppInstallationRepository[]> {
  const payload = await fetchSystemJson(
    `/v1/git-apps/github/installations/${encodeURIComponent(installationID)}/repositories`
  );
  return Array.isArray(payload) ? payload.map(normalizeGitHubAppInstallationRepository) : [];
}
