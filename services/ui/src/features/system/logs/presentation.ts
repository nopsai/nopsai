import type { SystemLogSource } from './types.js';

function normalizedSourceStatus(value?: string): string {
  const normalized = (value || '').trim();
  return normalized.toLowerCase() === 'none' ? '' : normalized;
}

export function systemLogSourceStatusLabel(source: SystemLogSource): string {
  if (!source.available) return 'unavailable';
  return normalizedSourceStatus(source.health) || normalizedSourceStatus(source.state) || 'available';
}
