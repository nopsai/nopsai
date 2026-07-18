import type { DashboardPublication } from './model.js';

export type DashboardCardSize = 'compact' | 'standard' | 'wide';

export type DashboardCardLayoutItem = {
  order?: number;
  size?: DashboardCardSize;
};

export type DashboardCardLayout = Record<string, DashboardCardLayoutItem>;

export const DASHBOARD_CARD_SIZE_OPTIONS: Array<{ id: DashboardCardSize; label: string }> = [
  { id: 'compact', label: 'Compact' },
  { id: 'standard', label: 'Standard' },
  { id: 'wide', label: 'Wide' },
];

export function dashboardCardLayoutStorageKey(dashboardID: string, sectionKey: string): string {
  return `nopsai.dashboard-card-layout.v1:${dashboardID.trim()}:${sectionKey.trim()}`;
}

export function dashboardCardLayoutItemKey(publication: Pick<DashboardPublication, 'id' | 'section_key' | 'entry_key' | 'pipeline_id' | 'output_name'>): string {
  return publication.id || [
    publication.section_key,
    publication.entry_key,
    publication.pipeline_id,
    publication.output_name,
  ].map(value => value || '-').join(':');
}

export function normalizeDashboardCardLayout(raw: unknown, cardKeys: string[]): DashboardCardLayout {
  if (!isRecord(raw)) return {};

  const allowedKeys = new Set(cardKeys);
  const layout: DashboardCardLayout = {};
  for (const [key, value] of Object.entries(raw)) {
    if (!allowedKeys.has(key) || !isRecord(value)) continue;
    const order = normalizeOrder(value.order);
    const size = normalizeCardSize(value.size);
    if (order === undefined && !size) continue;
    layout[key] = {
      ...(order === undefined ? {} : { order }),
      ...(size ? { size } : {}),
    };
  }
  if (Object.keys(layout).length === 0) return {};
  return compactDashboardCardLayout(layout, cardKeys);
}

export function orderDashboardCards<T>(
  cards: T[],
  cardKey: (card: T) => string,
  layout: DashboardCardLayout
): T[] {
  return cards
    .map((card, index) => ({
      card,
      index,
      order: layout[cardKey(card)]?.order ?? index,
    }))
    .sort((left, right) => left.order - right.order || left.index - right.index)
    .map(item => item.card);
}

export function moveDashboardCard(
  layout: DashboardCardLayout,
  cardKeys: string[],
  cardKey: string,
  direction: 'earlier' | 'later'
): DashboardCardLayout {
  const orderedKeys = dashboardCardOrderKeys(cardKeys, layout);
  const currentIndex = orderedKeys.indexOf(cardKey);
  if (currentIndex === -1) return compactDashboardCardLayout(layout, cardKeys);

  const nextIndex = direction === 'earlier' ? currentIndex - 1 : currentIndex + 1;
  if (nextIndex < 0 || nextIndex >= orderedKeys.length) return compactDashboardCardLayout(layout, cardKeys);

  const nextKeys = [...orderedKeys];
  [nextKeys[currentIndex], nextKeys[nextIndex]] = [nextKeys[nextIndex], nextKeys[currentIndex]];
  return layoutWithCardOrder(layout, nextKeys);
}

export function setDashboardCardSize(
  layout: DashboardCardLayout,
  cardKeys: string[],
  cardKey: string,
  size: DashboardCardSize
): DashboardCardLayout {
  if (!cardKeys.includes(cardKey)) return compactDashboardCardLayout(layout, cardKeys);
  const compacted = compactDashboardCardLayout(layout, cardKeys);
  return {
    ...compacted,
    [cardKey]: {
      ...compacted[cardKey],
      size,
    },
  };
}

function dashboardCardOrderKeys(cardKeys: string[], layout: DashboardCardLayout): string[] {
  return cardKeys
    .map((key, index) => ({ key, index, order: layout[key]?.order ?? index }))
    .sort((left, right) => left.order - right.order || left.index - right.index)
    .map(item => item.key);
}

function compactDashboardCardLayout(layout: DashboardCardLayout, cardKeys: string[]): DashboardCardLayout {
  const orderedKeys = dashboardCardOrderKeys(cardKeys, layout);
  return orderedKeys.reduce<DashboardCardLayout>((next, key, index) => {
    const size = layout[key]?.size;
    next[key] = {
      order: index,
      ...(size ? { size } : {}),
    };
    return next;
  }, {});
}

function layoutWithCardOrder(layout: DashboardCardLayout, orderedKeys: string[]): DashboardCardLayout {
  return orderedKeys.reduce<DashboardCardLayout>((next, key, index) => {
    const size = layout[key]?.size;
    next[key] = {
      order: index,
      ...(size ? { size } : {}),
    };
    return next;
  }, {});
}

function normalizeOrder(value: unknown): number | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value)) return undefined;
  return Math.max(0, Math.round(value));
}

function normalizeCardSize(value: unknown): DashboardCardSize | undefined {
  if (value === 'compact' || value === 'standard' || value === 'wide') return value;
  return undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
