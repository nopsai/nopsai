import type { CurrentUser } from './types.js';

function firstNonEmpty(...values: Array<string | undefined>): string {
  for (const value of values) {
    const trimmed = (value || '').trim();
    if (trimmed) return trimmed;
  }
  return '';
}

export function currentUserDisplayName(user?: CurrentUser | null): string {
  const subject = (user?.sub || '').trim();
  const readableSubject = subject.toLowerCase().startsWith('oidc:') ? '' : subject;
  return firstNonEmpty(user?.displayName, user?.email, readableSubject, 'User');
}

export function currentUserInitials(user?: CurrentUser | null): string {
  const display = currentUserDisplayName(user);
  const localName = display.includes('@') ? display.slice(0, display.indexOf('@')) : display;
  const parts = localName.match(/[A-Za-z0-9]+/g) || [];
  const initials = parts.slice(0, 2).map(part => part[0]).join('');
  return (initials || display[0] || 'U').toUpperCase();
}
