export function formatResourceListUpdatedAt(value?: string): string {
  const raw = (value || '').trim();
  if (!raw) return '-';
  const timestamp = Date.parse(raw);
  if (!Number.isFinite(timestamp)) return '-';
  return new Date(timestamp).toISOString().slice(0, 10);
}
