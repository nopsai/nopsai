export type PipelineDraft = {
  id: string;
  yaml: string;
  createdAt: string;
  updatedAt: string;
};

const PIPELINE_DRAFTS_STORAGE_PREFIX = 'nopsai:pipelines:drafts:v2';
export const PIPELINE_DRAFTS_CHANGED_EVENT = 'nopsai:pipelines:drafts:changed';

function notifyDraftsChanged() {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new Event(PIPELINE_DRAFTS_CHANGED_EVENT));
}

function normalizeDraft(value: unknown): PipelineDraft | null {
  if (!value || typeof value !== 'object') return null;
  const record = value as Record<string, unknown>;
  const id = typeof record.id === 'string' ? record.id.trim() : '';
  const yaml = typeof record.yaml === 'string' ? record.yaml : '';
  const createdAt = typeof record.createdAt === 'string' ? record.createdAt : '';
  const updatedAt = typeof record.updatedAt === 'string' ? record.updatedAt : '';
  if (!id || !yaml) return null;
  const now = new Date().toISOString();
  return {
    id,
    yaml,
    createdAt: createdAt || now,
    updatedAt: updatedAt || now,
  };
}

function safeParse(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

function normalizeScope(scope: string): string {
  return encodeURIComponent(scope.trim());
}

export function getPipelineDraftStorageKey(scope: string): string {
  const normalizedScope = normalizeScope(scope);
  return normalizedScope ? `${PIPELINE_DRAFTS_STORAGE_PREFIX}:${normalizedScope}` : PIPELINE_DRAFTS_STORAGE_PREFIX;
}

export function loadPipelineDrafts(scope: string): PipelineDraft[] {
  if (typeof window === 'undefined') return [];
  const normalizedScope = scope.trim();
  if (!normalizedScope) return [];
  const raw = localStorage.getItem(getPipelineDraftStorageKey(normalizedScope));
  if (!raw) return [];
  const parsed = safeParse(raw);
  if (Array.isArray(parsed)) {
    return parsed.map(normalizeDraft).filter(Boolean) as PipelineDraft[];
  }
  if (parsed && typeof parsed === 'object') {
    return Object.values(parsed)
      .map(normalizeDraft)
      .filter(Boolean) as PipelineDraft[];
  }
  return [];
}

export function savePipelineDrafts(drafts: PipelineDraft[], scope: string) {
  if (typeof window === 'undefined') return;
  const normalizedScope = scope.trim();
  if (!normalizedScope) return;
  const unique = new Map<string, PipelineDraft>();
  drafts.forEach(draft => {
    const normalized = normalizeDraft(draft);
    if (normalized) unique.set(normalized.id, normalized);
  });
  const sorted = Array.from(unique.values()).sort((a, b) => a.id.localeCompare(b.id));
  localStorage.setItem(getPipelineDraftStorageKey(normalizedScope), JSON.stringify(sorted));
  notifyDraftsChanged();
}

export function getPipelineDraft(id: string, scope: string): PipelineDraft | null {
  const normalizedId = id.trim();
  if (!normalizedId) return null;
  const drafts = loadPipelineDrafts(scope);
  return drafts.find(draft => draft.id === normalizedId) || null;
}

export function upsertPipelineDraft(next: { id: string; yaml: string }, scope: string): PipelineDraft[] {
  const normalizedId = next.id.trim();
  if (!normalizedId) return loadPipelineDrafts(scope);
  const now = new Date().toISOString();
  const drafts = loadPipelineDrafts(scope);
  const existing = drafts.find(draft => draft.id === normalizedId);
  const updated: PipelineDraft = {
    id: normalizedId,
    yaml: next.yaml,
    createdAt: existing?.createdAt || now,
    updatedAt: now,
  };
  const merged = drafts.filter(draft => draft.id !== normalizedId);
  merged.push(updated);
  savePipelineDrafts(merged, scope);
  return merged;
}

export function deletePipelineDraft(id: string, scope: string): PipelineDraft[] {
  const normalizedId = id.trim();
  if (!normalizedId) return loadPipelineDrafts(scope);
  const drafts = loadPipelineDrafts(scope).filter(draft => draft.id !== normalizedId);
  savePipelineDrafts(drafts, scope);
  return drafts;
}
