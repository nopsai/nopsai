import { apiClient } from '../lib/api';
import { normalizePlatformVersionInfo, type PlatformVersionInfo } from './platformVersion';

export async function fetchPlatformVersionInfo(): Promise<PlatformVersionInfo | null> {
  const response = await apiClient.fetch('/version', { auth: false, cache: 'no-store' });
  if (!response.ok) return null;
  return normalizePlatformVersionInfo(await response.json().catch(() => null));
}
