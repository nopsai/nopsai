import { useCallback, useMemo, useSyncExternalStore, useEffect } from 'react';

type DraftRecord = {
  id: string;
  yaml: string;
};

type DraftCollectionOptions<TDraft extends DraftRecord> = {
  enabled: boolean;
  scope: string;
  changedEvent: string;
  getStorageKey: (scope: string) => string;
  load: (scope: string) => TDraft[];
  upsert: (draft: { id: string; yaml: string }, scope: string) => TDraft[];
  remove: (id: string, scope: string) => TDraft[];
  autosave?: {
    active: boolean;
    id: string;
    yaml: string;
    delay?: number;
  };
};

export function useDraftCollection<TDraft extends DraftRecord>({
  enabled,
  scope,
  changedEvent,
  getStorageKey,
  load,
  upsert,
  remove,
  autosave,
}: DraftCollectionOptions<TDraft>) {
  const normalizedScope = scope.trim();
  const storageKey = normalizedScope ? getStorageKey(normalizedScope) : '';

  const subscribe = useCallback(
    (notify: () => void) => {
      if (typeof window === 'undefined' || !enabled || !storageKey) return () => undefined;
      const handleStorage = (event: StorageEvent) => {
        if (event.key === storageKey) notify();
      };
      window.addEventListener(changedEvent, notify);
      window.addEventListener('storage', handleStorage);
      return () => {
        window.removeEventListener(changedEvent, notify);
        window.removeEventListener('storage', handleStorage);
      };
    },
    [changedEvent, enabled, storageKey]
  );

  const getSnapshot = useCallback(() => {
    if (typeof window === 'undefined' || !enabled || !storageKey) return '';
    return localStorage.getItem(storageKey) || '';
  }, [enabled, storageKey]);

  const snapshot = useSyncExternalStore(subscribe, getSnapshot, () => '');
  const drafts = useMemo(() => {
    void snapshot;
    return enabled && normalizedScope ? load(normalizedScope) : [];
  }, [enabled, load, normalizedScope, snapshot]);
  const draftsByID = useMemo(
    () => new Map(drafts.map(draft => [draft.id, draft])),
    [drafts]
  );

  const upsertDraft = useCallback(
    (draft: { id: string; yaml: string }) => {
      if (!enabled || !normalizedScope) return drafts;
      return upsert(draft, normalizedScope);
    },
    [drafts, enabled, normalizedScope, upsert]
  );

  const removeDraft = useCallback(
    (id: string) => {
      if (!enabled || !normalizedScope) return drafts;
      return remove(id, normalizedScope);
    },
    [drafts, enabled, normalizedScope, remove]
  );

  useEffect(() => {
    if (!enabled || !normalizedScope || !autosave?.active || !autosave.id) return;
    const handle = window.setTimeout(
      () => upsert({ id: autosave.id, yaml: autosave.yaml }, normalizedScope),
      autosave.delay ?? 800
    );
    return () => window.clearTimeout(handle);
  }, [
    autosave?.active,
    autosave?.delay,
    autosave?.id,
    autosave?.yaml,
    enabled,
    normalizedScope,
    upsert,
  ]);

  return {
    drafts,
    draftsByID,
    removeDraft,
    upsertDraft,
  };
}
