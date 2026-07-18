import { useMemo, useState, type DragEvent as ReactDragEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { GripVertical, RotateCcw, Trash2 } from 'lucide-react';

import { ObjectIcon } from '../../components/ObjectIcon';
import { DashboardBlocks } from './blocks/DashboardBlocks';
import { dashboardSpecNeedsWideLayout } from './blocks/DashboardBlocksLayout';
import { dashboardCardLayoutItemKey, type DashboardCardSize } from './dashboardCardLayout';
import { formatDateTime, runScopeLabel, staleLabel, type DashboardPublication } from './model';
import { useDashboardCardLayout } from './useDashboardCardLayout';

type DashboardPublicationGridProps = {
  dashboardID: string;
  sectionKey: string;
  publications: DashboardPublication[];
  canWriteDashboards: boolean;
  onDeletePublication: (publication: DashboardPublication) => void;
};

type DashboardCardDropPosition = 'before' | 'after';
type DashboardCardDragState = {
  cardKey: string;
  targetCardKey?: string;
  dropPosition?: DashboardCardDropPosition;
};

const DASHBOARD_CARD_DRAG_MIME = 'application/x-nopsai-dashboard-card';
const DASHBOARD_CARD_RESIZE_STEP_PX = 120;

export function DashboardPublicationGrid({
  dashboardID,
  sectionKey,
  publications,
  canWriteDashboards,
  onDeletePublication,
}: DashboardPublicationGridProps) {
  const {
    layout,
    orderedPublications,
    resizeCard,
    placeCard,
    resetLayout,
    hasSavedLayout,
  } = useDashboardCardLayout(dashboardID, sectionKey, publications);
  const [dragState, setDragState] = useState<DashboardCardDragState | null>(null);
  const orderedCardKeys = useMemo(
    () => orderedPublications.map(publication => dashboardCardLayoutItemKey(publication)),
    [orderedPublications]
  );

  const clearDragState = () => setDragState(null);
  const handleCardDragStart = (cardKey: string, event: ReactDragEvent<HTMLElement>) => {
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData(DASHBOARD_CARD_DRAG_MIME, cardKey);
    event.dataTransfer.setData('text/plain', cardKey);
    setDragState({ cardKey });
  };
  const handleCardDragOver = (targetCardKey: string, event: ReactDragEvent<HTMLElement>) => {
    const draggedCardKey = dragState?.cardKey || event.dataTransfer.getData(DASHBOARD_CARD_DRAG_MIME);
    if (!draggedCardKey || draggedCardKey === targetCardKey) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
    const dropPosition =
      cardDropPositionFromEvent(event) || fallbackCardDropPosition(orderedCardKeys, draggedCardKey, targetCardKey);
    setDragState(current => {
      if (current?.cardKey === draggedCardKey && current.targetCardKey === targetCardKey && current.dropPosition === dropPosition) {
        return current;
      }
      return { cardKey: draggedCardKey, targetCardKey, dropPosition };
    });
  };
  const handleCardDrop = (targetCardKey: string, event: ReactDragEvent<HTMLElement>) => {
    const draggedCardKey = event.dataTransfer.getData(DASHBOARD_CARD_DRAG_MIME) || dragState?.cardKey;
    if (!draggedCardKey || draggedCardKey === targetCardKey) {
      clearDragState();
      return;
    }
    event.preventDefault();
    const dropPosition = dragState?.targetCardKey === targetCardKey && dragState.dropPosition
      ? dragState.dropPosition
      : fallbackCardDropPosition(orderedCardKeys, draggedCardKey, targetCardKey);
    placeCard(draggedCardKey, targetCardKey, dropPosition);
    clearDragState();
  };

  return (
    <>
      {hasSavedLayout ? (
        <div className="flex justify-end">
          <button
            type="button"
            className="inline-flex h-8 items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 text-xs font-semibold text-[var(--text-secondary)] shadow-sm transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]"
            onClick={resetLayout}
          >
            <RotateCcw className="h-3.5 w-3.5" aria-hidden="true" />
            Reset card layout
          </button>
        </div>
      ) : null}
      <div className="grid gap-3 xl:grid-cols-4">
        {orderedPublications.map(publication => {
          const cardKey = dashboardCardLayoutItemKey(publication);
          const currentSize = layout[cardKey]?.size || defaultPublicationCardSize(publication, publications.length);
          return (
            <PublicationCard
              key={cardKey}
              publication={publication}
              cardKey={cardKey}
              cardSize={currentSize}
              dragging={dragState?.cardKey === cardKey}
              dropPosition={dragState?.targetCardKey === cardKey ? dragState.dropPosition : undefined}
              canWriteDashboards={canWriteDashboards}
              onResizeCard={size => resizeCard(cardKey, size)}
              onDragStart={event => handleCardDragStart(cardKey, event)}
              onDragOver={event => handleCardDragOver(cardKey, event)}
              onDrop={event => handleCardDrop(cardKey, event)}
              onDragEnd={clearDragState}
              onDeletePublication={onDeletePublication}
            />
          );
        })}
      </div>
    </>
  );
}

function PublicationCard({
  publication,
  cardKey,
  cardSize,
  dragging,
  dropPosition,
  canWriteDashboards,
  onResizeCard,
  onDragStart,
  onDragOver,
  onDrop,
  onDragEnd,
  onDeletePublication,
}: {
  publication: DashboardPublication;
  cardKey: string;
  cardSize: DashboardCardSize;
  dragging: boolean;
  dropPosition?: DashboardCardDropPosition;
  canWriteDashboards: boolean;
  onResizeCard: (size: DashboardCardSize) => void;
  onDragStart: (event: ReactDragEvent<HTMLElement>) => void;
  onDragOver: (event: ReactDragEvent<HTMLElement>) => void;
  onDrop: (event: ReactDragEvent<HTMLElement>) => void;
  onDragEnd: () => void;
  onDeletePublication: (publication: DashboardPublication) => void;
}) {
  const cardTitle = publication.content.title || publication.entry_key;
  const handleResizePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.stopPropagation();

    const startX = event.clientX;
    const startSize = cardSize;
    let lastSize = startSize;
    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = 'ew-resize';
    document.body.style.userSelect = 'none';

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const nextSize = cardSizeFromResizeDelta(startSize, moveEvent.clientX - startX);
      if (nextSize === lastSize) return;
      lastSize = nextSize;
      onResizeCard(nextSize);
    };
    const handlePointerUp = () => {
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', handlePointerUp);
      window.removeEventListener('pointercancel', handlePointerUp);
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', handlePointerUp);
    window.addEventListener('pointercancel', handlePointerUp);
  };

  return (
    <article
      aria-label={`Dashboard card ${cardTitle}`}
      data-dashboard-card-key={cardKey}
      onDragOver={onDragOver}
      onDrop={onDrop}
      className={`relative min-w-0 overflow-hidden rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-sm transition-[opacity,box-shadow,border-color] ${publicationCardSizeClass(cardSize)} ${
        dragging ? 'opacity-60 ring-2 ring-[var(--accent)]' : ''
      } ${dropPosition ? 'border-[var(--accent)] shadow-md' : ''}`}
    >
      {dropPosition ? (
        <div
          className={`pointer-events-none absolute bottom-0 top-0 z-10 w-1 bg-[var(--accent)] ${dropPosition === 'before' ? 'left-0' : 'right-0'}`}
          aria-hidden="true"
        />
      ) : null}
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label={`Resize card ${cardTitle}`}
        aria-valuemin={1}
        aria-valuemax={3}
        aria-valuenow={dashboardCardSizeStep(cardSize) + 1}
        aria-valuetext={`${dashboardCardSizeLabel(cardSize)} width`}
        tabIndex={0}
        title={`Drag left or right to resize. Current size: ${dashboardCardSizeLabel(cardSize)}.`}
        className="absolute bottom-0 right-0 top-0 z-20 w-3 cursor-ew-resize touch-none bg-transparent transition-colors hover:bg-[var(--accent-soft)] focus:bg-[var(--accent-soft)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
        onPointerDown={handleResizePointerDown}
      />
      <header className="flex flex-wrap items-start justify-between gap-3 border-b border-[var(--border-primary)] px-4 py-3">
        <div className="flex min-w-0 items-start gap-3">
          <div
            role="button"
            tabIndex={0}
            draggable
            aria-label={`Drag card ${cardTitle}`}
            title="Drag to move this card"
            className="inline-flex h-8 w-5 shrink-0 cursor-grab items-center justify-center rounded-md text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] active:cursor-grabbing"
            onDragStart={onDragStart}
            onDragEnd={onDragEnd}
          >
            <GripVertical className="h-4 w-4" aria-hidden="true" />
          </div>
          <div className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-[var(--accent-soft)] text-[var(--accent)]">
            <ObjectIcon type="dashboard" className="h-4 w-4" />
          </div>
          <div className="min-w-0">
            <div className="truncate text-sm font-bold text-[var(--text-primary)]">{cardTitle}</div>
            <div className="mt-1 truncate text-xs text-[var(--text-muted)]">
              {publication.pipeline_id} / {publication.output_name} / {runScopeLabel(publication.run_scope)}
            </div>
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 pr-2">
          <span className="text-xs font-semibold text-[var(--text-muted)]">
            {dashboardCardSizeLabel(cardSize)}
          </span>
          <span className="runner-pill runner-pill--muted">
            {publication.mode}
          </span>
          <span className={`runner-pill ${publication.stale ? 'runner-pill--warning' : 'runner-pill--ok'}`}>
            {staleLabel(publication)}
          </span>
          {canWriteDashboards ? (
            <CardIconButton
              label={`Remove entry ${publication.entry_key}`}
              icon={<Trash2 className="h-4 w-4" />}
              onClick={() => onDeletePublication(publication)}
              danger
            />
          ) : null}
        </div>
      </header>
      <div className="p-4">
        <DashboardBlocks spec={publication.content} />
      </div>
      <footer className="flex flex-wrap items-center gap-3 border-t border-[var(--border-primary)] bg-[var(--bg-tertiary)] px-4 py-3 pr-6 text-xs text-[var(--text-secondary)]">
        <span>Revision {publication.revision}</span>
        <span>{formatDateTime(publication.published_at)}</span>
        {publication.run_id ? (
          <Link
            className="font-medium text-[var(--accent)] hover:underline"
            to={runDetailHref(publication.run_id)}
          >
            Run {publication.run_id.slice(0, 8)}
          </Link>
        ) : null}
      </footer>
    </article>
  );
}

function CardIconButton({
  label,
  icon,
  onClick,
  danger,
}: {
  label: string;
  icon: ReactNode;
  onClick: () => void;
  danger?: boolean;
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      className={cardIconButtonClass({ danger })}
      onClick={onClick}
    >
      {icon}
    </button>
  );
}

function cardIconButtonClass(options: { danger?: boolean } = {}) {
  const base = 'inline-flex h-9 w-9 items-center justify-center rounded-md border border-[var(--border-primary)] text-sm shadow-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50';
  if (options.danger) return `${base} bg-rose-50 text-rose-600 hover:bg-rose-100 dark:bg-rose-950/30 dark:text-rose-100`;
  return `${base} bg-[var(--bg-secondary)] text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]`;
}

function defaultPublicationCardSize(publication: DashboardPublication, publicationCount: number): DashboardCardSize {
  if (publicationCount === 1 || dashboardSpecNeedsWideLayout(publication.content)) return 'wide';
  return 'standard';
}

function cardDropPositionFromEvent(event: ReactDragEvent<HTMLElement>): DashboardCardDropPosition | undefined {
  const rect = event.currentTarget.getBoundingClientRect();
  if (rect.width <= 0) return undefined;
  return event.clientX >= rect.left + rect.width / 2 ? 'after' : 'before';
}

function fallbackCardDropPosition(cardKeys: string[], draggedCardKey: string, targetCardKey: string): DashboardCardDropPosition {
  const draggedIndex = cardKeys.indexOf(draggedCardKey);
  const targetIndex = cardKeys.indexOf(targetCardKey);
  if (draggedIndex === -1 || targetIndex === -1) return 'before';
  return draggedIndex < targetIndex ? 'after' : 'before';
}

function cardSizeFromResizeDelta(startSize: DashboardCardSize, deltaX: number): DashboardCardSize {
  const startStep = dashboardCardSizeStep(startSize);
  const stepDelta = Math.round(deltaX / DASHBOARD_CARD_RESIZE_STEP_PX);
  const nextStep = Math.min(2, Math.max(0, startStep + stepDelta));
  return dashboardCardSizeFromStep(nextStep);
}

function dashboardCardSizeStep(size: DashboardCardSize): number {
  if (size === 'compact') return 0;
  if (size === 'wide') return 2;
  return 1;
}

function dashboardCardSizeFromStep(step: number): DashboardCardSize {
  if (step <= 0) return 'compact';
  if (step >= 2) return 'wide';
  return 'standard';
}

function dashboardCardSizeLabel(size: DashboardCardSize): string {
  if (size === 'compact') return 'Compact';
  if (size === 'wide') return 'Wide';
  return 'Standard';
}

function publicationCardSizeClass(size: DashboardCardSize): string {
  if (size === 'compact') return 'xl:col-span-1';
  if (size === 'wide') return 'xl:col-span-4';
  return 'xl:col-span-2';
}

function runDetailHref(runID: string): string {
  return `/pipelineruns/recent/${encodeURIComponent(runID)}`;
}
