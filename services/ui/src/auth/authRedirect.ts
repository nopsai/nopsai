export type LoginRedirectState = {
  returnTo?: string;
};

export function buildLoginRedirectState(pathname: string, search: string): LoginRedirectState {
  return { returnTo: normalizeInternalReturnPath(`${pathname}${search}`) };
}

export function resolvePostLoginPath(state: unknown, fallback = '/pipelineruns/main'): string {
  if (!state || typeof state !== 'object') return fallback;
  const returnTo = (state as LoginRedirectState).returnTo;
  return normalizeInternalReturnPath(returnTo) || fallback;
}

function normalizeInternalReturnPath(value: unknown): string {
  if (typeof value !== 'string') return '';
  const trimmed = value.trim();
  if (!trimmed.startsWith('/') || trimmed.startsWith('//') || trimmed.startsWith('/login')) {
    return '';
  }
  return trimmed;
}
