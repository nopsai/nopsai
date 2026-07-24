export function redactAnalysisPromptText(value: string, maxLength = 900) {
  const normalized = String(value || '')
    .replace(/("(?:token|secret|password|api[_-]?key|private[_-]?key)"\s*:\s*)("[^"]+"|'[^']+'|[^\s,;}]+)/gi, '$1"[redacted]"')
    .replace(/(token|secret|password|api[_-]?key|private[_-]?key)(\s*[=:]\s*)("[^"]+"|'[^']+'|[^\s,;]+)/gi, '$1$2[redacted]')
    .replace(/\bBearer\s+[A-Za-z0-9._~+/=-]+/gi, 'Bearer [redacted]')
    .replace(/(credential:\/\/)[^\s'",\]]+/gi, '$1[redacted]')
    .replace(/\s+/g, ' ')
    .trim();
  if (normalized.length <= maxLength) return normalized;
  return `${normalized.slice(0, Math.max(0, maxLength - 3))}...`;
}

export function formatPromptTimestamp(value?: string) {
  if (!value) return '';
  const timestamp = new Date(value);
  return Number.isNaN(timestamp.getTime()) ? '' : timestamp.toISOString();
}

export function compactPromptList(values: Array<string | undefined | null>, fallback = '-') {
  const normalized = values.map(value => String(value || '').trim()).filter(Boolean);
  return normalized.length > 0 ? normalized.join(', ') : fallback;
}
