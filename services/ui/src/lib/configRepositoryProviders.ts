export type ConfigRepositoryProvider = 'github' | 'gitlab' | 'bitbucket' | 'gitea';

export const CONFIG_REPOSITORY_PROVIDER_OPTIONS: Array<{ value: ConfigRepositoryProvider; label: string }> = [
  { value: 'github', label: 'GitHub' },
  { value: 'gitlab', label: 'GitLab' },
  { value: 'bitbucket', label: 'Bitbucket' },
  { value: 'gitea', label: 'Gitea' },
];

export function normalizeConfigRepositoryProvider(value: unknown): ConfigRepositoryProvider {
  const provider = typeof value === 'string' ? value.trim().toLowerCase() : '';
  if (provider === 'gitlab' || provider === 'bitbucket' || provider === 'gitea') return provider;
  return 'github';
}
