export function formatFilteredCount(visible: number, total: number, searchTerm: string) {
  return searchTerm.trim() ? `${visible} / ${total}` : total;
}

export function matchesAIResourceSearch(query: string, ...values: Array<string | number | boolean | undefined | null>) {
  const normalizedQuery = query.trim().toLowerCase();
  if (!normalizedQuery) return true;
  return values.some(value => String(value ?? '').toLowerCase().includes(normalizedQuery));
}
