export function eventAutomationSourceKey(source?: string): string {
  const key = (source || '').trim().toLowerCase();
  if (!key) return 'database';
  if (key.includes('git')) return 'git';
  if (key.includes('draft')) return 'draft';
  if (key.includes('db') || key.includes('database')) return 'database';
  if (key.includes('local')) return 'local';
  return key;
}

export function eventAutomationSourceLabel(source?: string): string {
  switch (eventAutomationSourceKey(source)) {
    case 'git':
      return 'Git';
    case 'draft':
      return 'Draft';
    case 'local':
      return 'Local';
    default:
      return 'Database';
  }
}
