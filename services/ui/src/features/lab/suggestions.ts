import type { LabSuggestionContext, LabSuggestionItem } from '../../lib/lab.js';

export function normalizeLabScopeLabel(value: unknown): string {
  if (value == null) return '';
  const normalized = String(value).trim().replace(/^\/+|\/+$/g, '');
  return normalized.toLowerCase() === 'default' ? '' : normalized;
}

export function normalizeLabSuggestionList(payload: unknown): string[] {
  if (!Array.isArray(payload)) return [];
  return payload
    .map(item => {
      if (typeof item === 'string') return item.trim();
      if (item && typeof item === 'object' && 'name' in item) {
        const name = (item as Record<string, unknown>).name;
        if (typeof name === 'string') return name.trim();
      }
      return '';
    })
    .filter(Boolean);
}

export function normalizeVariableSuggestionList(payload: unknown): string[] {
  const names = normalizeLabSuggestionList(payload);
  const values = new Set<string>();
  names.forEach(name => {
    const parts = name.split('/').filter(Boolean);
    values.add(parts.length === 3 ? parts[2] : name);
  });
  return Array.from(values).sort((a, b) => a.localeCompare(b));
}

export function normalizeLLMProfileSuggestionList(payload: unknown): string[] {
  const record = payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : null;
  const profiles = record && Array.isArray(record.profiles) ? record.profiles : [];
  return profiles
    .map(profile => {
      if (typeof profile === 'string') return profile.trim();
      if (!profile || typeof profile !== 'object') return '';
      const profileRecord = profile as Record<string, unknown>;
      if (profileRecord.allowed_in_scope === false) return '';
      return typeof profileRecord.name === 'string' ? profileRecord.name.trim() : '';
    })
    .filter(Boolean);
}

export function normalizeMCPProfileSuggestionList(payload: unknown): string[] {
  const record = payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : null;
  const profiles = record && Array.isArray(record.profiles) ? record.profiles : [];
  return profiles
    .map(profile => {
      if (typeof profile === 'string') return profile.trim();
      if (!profile || typeof profile !== 'object') return '';
      const profileRecord = profile as Record<string, unknown>;
      if (profileRecord.enabled === false) return '';
      return typeof profileRecord.name === 'string' ? profileRecord.name.trim() : '';
    })
    .filter(Boolean);
}

export function buildInlineSuggestionPreview(
  item: LabSuggestionItem,
  contextInfo: LabSuggestionContext
): string {
  const prefix = typeof contextInfo.prefix === 'string' ? contextInfo.prefix : '';
  const snippetSource = item.value || item.snippet || '';
  if (!snippetSource) return '';
  const firstLine = String(snippetSource).split('\n')[0];
  if (!firstLine) return '';

  if (!prefix) return firstLine;
  return firstLine.toLowerCase().startsWith(prefix.toLowerCase())
    ? firstLine.slice(prefix.length)
    : '';
}
