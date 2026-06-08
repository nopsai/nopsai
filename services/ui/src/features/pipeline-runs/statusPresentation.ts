export const STATUS_META: Record<
  string,
  { text: string; pillClass: string; icon: string; strokeClass: string; border: string; bg: string }
> = {
  success: {
    text: 'Success',
    pillClass: 'bg-green-100 text-green-700 border-green-200 dark:bg-green-900/30 dark:text-green-200 dark:border-green-700',
    icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z',
    strokeClass: 'text-green-700',
    border: 'border-green-500/50',
    bg: 'fill-green-100 dark:fill-green-900/50 stroke-green-500',
  },
  failure: {
    text: 'Failure',
    pillClass: 'bg-red-100 text-red-700 border-red-200 dark:bg-red-900/30 dark:text-red-200 dark:border-red-700',
    icon: 'M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0',
    strokeClass: 'text-red-500',
    border: 'border-red-500/60',
    bg: 'fill-red-100 dark:fill-red-900/50 stroke-red-500',
  },
  'failure (ignored)': {
    text: 'Failure (ignored)',
    pillClass: 'bg-amber-100 text-amber-800 border-amber-200 dark:bg-amber-900/30 dark:text-amber-100 dark:border-amber-600',
    icon: 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z',
    strokeClass: 'text-amber-600',
    border: 'border-amber-500/60',
    bg: 'fill-amber-100 dark:fill-amber-900/50 stroke-amber-500',
  },
  running: {
    text: 'Running',
    pillClass: 'bg-blue-100 text-blue-700 border-blue-200 dark:bg-blue-900/30 dark:text-blue-200 dark:border-blue-700',
    icon: 'M21 12a9 9 0 11-6.219-8.56',
    strokeClass: 'text-blue-500 animate-pulse',
    border: 'border-blue-500/60',
    bg: 'fill-blue-100 dark:fill-blue-900/50 stroke-blue-500',
  },
  waiting_approval: {
    text: 'Waiting approval',
    pillClass: 'bg-cyan-100 text-cyan-800 border-cyan-200 dark:bg-cyan-900/30 dark:text-cyan-100 dark:border-cyan-700',
    icon: 'M9 11l3 3L22 4M21 12v7a2 2 0 01-2 2H5a2 2 0 01-2-2V5a2 2 0 012-2h11',
    strokeClass: 'text-cyan-600',
    border: 'border-cyan-500/60',
    bg: 'fill-cyan-100 dark:fill-cyan-900/50 stroke-cyan-500',
  },
  rejected: {
    text: 'Rejected',
    pillClass: 'bg-rose-100 text-rose-700 border-rose-200 dark:bg-rose-900/30 dark:text-rose-100 dark:border-rose-700',
    icon: 'M18 6L6 18M6 6l12 12',
    strokeClass: 'text-rose-500',
    border: 'border-rose-500/60',
    bg: 'fill-rose-100 dark:fill-rose-900/50 stroke-rose-500',
  },
  pending: {
    text: 'Pending',
    pillClass: 'bg-gray-100 text-gray-700 border-gray-200 dark:bg-gray-800/40 dark:text-gray-200 dark:border-gray-700',
    icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z',
    strokeClass: 'text-gray-500',
    border: 'border-gray-500/60',
    bg: 'fill-gray-100 dark:fill-gray-800 stroke-gray-500',
  },
  skipped: {
    text: 'Skipped',
    pillClass: 'bg-slate-100 text-slate-700 border-slate-200 dark:bg-slate-800/60 dark:text-slate-200 dark:border-slate-700',
    icon: 'M6 12h12M12 3a9 9 0 110 18 9 9 0 010-18z',
    strokeClass: 'text-slate-500',
    border: 'border-slate-500/60',
    bg: 'fill-slate-100 dark:fill-slate-800 stroke-slate-500',
  },
  cancelled: {
    text: 'Cancelled',
    pillClass: 'bg-orange-100 text-orange-700 border-orange-200 dark:bg-orange-900/30 dark:text-orange-200 dark:border-orange-700',
    icon: 'M6 18L18 6M6 6l12 12',
    strokeClass: 'text-orange-500',
    border: 'border-orange-500/60',
    bg: 'fill-orange-100 dark:fill-orange-900/50 stroke-orange-500',
  },
};

export function normalizeStatus(status: string | undefined, complete?: boolean): string {
  const raw = (status || '').toLowerCase();
  if (STATUS_META[raw]) return raw;
  if (!complete) return raw || 'pending';
  return 'pending';
}

export function getStatusMeta(status: string | undefined, complete?: boolean) {
  const normalized = normalizeStatus(status, complete);
  return STATUS_META[normalized] || STATUS_META.pending;
}

