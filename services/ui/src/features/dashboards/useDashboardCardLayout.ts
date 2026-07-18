import { useCallback, useMemo, useState } from 'react';

import {
  dashboardCardLayoutItemKey,
  dashboardCardLayoutStorageKey,
  moveDashboardCard,
  normalizeDashboardCardLayout,
  orderDashboardCards,
  placeDashboardCard,
  setDashboardCardSize,
  type DashboardCardLayout,
  type DashboardCardSize,
} from './dashboardCardLayout';
import type { DashboardPublication } from './model';

type DashboardCardLayoutState = {
  storageKey: string;
  cardKeySignature: string;
  layout: DashboardCardLayout;
};

export function useDashboardCardLayout(
  dashboardID: string,
  sectionKey: string,
  publications: DashboardPublication[]
) {
  const storageKey = useMemo(
    () => dashboardCardLayoutStorageKey(dashboardID, sectionKey),
    [dashboardID, sectionKey]
  );
  const cardKeys = useMemo(
    () => publications.map(publication => dashboardCardLayoutItemKey(publication)),
    [publications]
  );
  const cardKeySignature = cardKeys.join('\u001f');
  const [layoutState, setLayoutState] = useState<DashboardCardLayoutState>(() => ({
    storageKey,
    cardKeySignature,
    layout: readDashboardCardLayout(storageKey, cardKeys),
  }));
  const layout = useMemo(() => {
    if (layoutState.storageKey === storageKey && layoutState.cardKeySignature === cardKeySignature) {
      return layoutState.layout;
    }
    return readDashboardCardLayout(storageKey, cardKeys);
  }, [cardKeySignature, cardKeys, layoutState, storageKey]);

  const updateLayout = useCallback(
    (updater: (current: DashboardCardLayout) => DashboardCardLayout) => {
      setLayoutState(currentState => {
        const current = currentState.storageKey === storageKey && currentState.cardKeySignature === cardKeySignature
          ? currentState.layout
          : readDashboardCardLayout(storageKey, cardKeys);
        const normalizedCurrent = normalizeDashboardCardLayout(current, cardKeys);
        const next = normalizeDashboardCardLayout(updater(normalizedCurrent), cardKeys);
        writeDashboardCardLayout(storageKey, next);
        return { storageKey, cardKeySignature, layout: next };
      });
    },
    [cardKeySignature, cardKeys, storageKey]
  );

  const orderedPublications = useMemo(
    () => orderDashboardCards(publications, dashboardCardLayoutItemKey, layout),
    [layout, publications]
  );

  const resizeCard = useCallback(
    (cardKey: string, size: DashboardCardSize) => {
      updateLayout(current => setDashboardCardSize(current, cardKeys, cardKey, size));
    },
    [cardKeys, updateLayout]
  );

  const moveCard = useCallback(
    (cardKey: string, direction: 'earlier' | 'later') => {
      updateLayout(current => moveDashboardCard(current, cardKeys, cardKey, direction));
    },
    [cardKeys, updateLayout]
  );

  const placeCard = useCallback(
    (cardKey: string, targetCardKey: string, position: 'before' | 'after') => {
      updateLayout(current => placeDashboardCard(current, cardKeys, cardKey, targetCardKey, position));
    },
    [cardKeys, updateLayout]
  );

  const resetLayout = useCallback(() => {
    removeDashboardCardLayout(storageKey);
    setLayoutState({ storageKey, cardKeySignature, layout: {} });
  }, [cardKeySignature, storageKey]);

  return {
    layout,
    orderedPublications,
    resizeCard,
    moveCard,
    placeCard,
    resetLayout,
    hasSavedLayout: Object.keys(layout).length > 0,
  };
}

function readDashboardCardLayout(storageKey: string, cardKeys: string[]): DashboardCardLayout {
  const storage = browserStorage();
  if (!storage) return {};
  try {
    const raw = storage.getItem(storageKey);
    return normalizeDashboardCardLayout(raw ? JSON.parse(raw) : {}, cardKeys);
  } catch {
    return {};
  }
}

function writeDashboardCardLayout(storageKey: string, layout: DashboardCardLayout) {
  const storage = browserStorage();
  if (!storage) return;
  try {
    storage.setItem(storageKey, JSON.stringify(layout));
  } catch {
    // Layout preferences are best-effort and must not block dashboard rendering.
  }
}

function removeDashboardCardLayout(storageKey: string) {
  const storage = browserStorage();
  if (!storage) return;
  try {
    storage.removeItem(storageKey);
  } catch {
    // Ignore storage cleanup failures for the same reason as writes.
  }
}

function browserStorage(): Storage | null {
  if (typeof window === 'undefined') return null;
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}
