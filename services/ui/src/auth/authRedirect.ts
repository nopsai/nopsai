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
  // Browsers normalise backslashes to forward slashes while parsing the
  // authority, so '/\evil.example' navigates cross-origin exactly like
  // '//evil.example'. An internal path never needs a backslash.
  if (trimmed.includes('\\')) return '';
  return trimmed;
}
