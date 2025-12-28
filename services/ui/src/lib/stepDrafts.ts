export type StepDraft = {
  id: string;
  yaml: string;
  createdAt: string;
  updatedAt: string;
};

export const STEP_DRAFTS_STORAGE_KEY = 'nopsai:steps:drafts:v1';
export const STEP_DRAFTS_CHANGED_EVENT = 'nopsai:steps:drafts:changed';

function notifyDraftsChanged() {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new Event(STEP_DRAFTS_CHANGED_EVENT));
}

function normalizeDraft(value: unknown): StepDraft | null {
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

export function loadStepDrafts(): StepDraft[] {
  if (typeof window === 'undefined') return [];
  const raw = localStorage.getItem(STEP_DRAFTS_STORAGE_KEY);
  if (!raw) return [];
  const parsed = safeParse(raw);
  if (Array.isArray(parsed)) {
    return parsed.map(normalizeDraft).filter(Boolean) as StepDraft[];
  }
  if (parsed && typeof parsed === 'object') {
    return Object.values(parsed)
      .map(normalizeDraft)
      .filter(Boolean) as StepDraft[];
  }
  return [];
}

export function saveStepDrafts(drafts: StepDraft[]) {
  if (typeof window === 'undefined') return;
  const unique = new Map<string, StepDraft>();
  drafts.forEach(draft => {
    const normalized = normalizeDraft(draft);
    if (normalized) unique.set(normalized.id, normalized);
  });
  const sorted = Array.from(unique.values()).sort((a, b) => a.id.localeCompare(b.id));
  localStorage.setItem(STEP_DRAFTS_STORAGE_KEY, JSON.stringify(sorted));
  notifyDraftsChanged();
}

export function getStepDraft(id: string): StepDraft | null {
  const normalizedId = id.trim();
  if (!normalizedId) return null;
  const drafts = loadStepDrafts();
  return drafts.find(draft => draft.id === normalizedId) || null;
}

export function upsertStepDraft(next: { id: string; yaml: string }): StepDraft[] {
  const normalizedId = next.id.trim();
  if (!normalizedId) return loadStepDrafts();
  const now = new Date().toISOString();
  const drafts = loadStepDrafts();
  const existing = drafts.find(draft => draft.id === normalizedId);
  const updated: StepDraft = {
    id: normalizedId,
    yaml: next.yaml,
    createdAt: existing?.createdAt || now,
    updatedAt: now,
  };
  const merged = drafts.filter(draft => draft.id !== normalizedId);
  merged.push(updated);
  saveStepDrafts(merged);
  return merged;
}

export function deleteStepDraft(id: string): StepDraft[] {
  const normalizedId = id.trim();
  if (!normalizedId) return loadStepDrafts();
  const drafts = loadStepDrafts().filter(draft => draft.id !== normalizedId);
  saveStepDrafts(drafts);
  return drafts;
}

