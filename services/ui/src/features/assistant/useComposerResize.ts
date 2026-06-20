import { useCallback, useState, type KeyboardEvent, type PointerEvent } from 'react';

export const assistantComposerMinHeight = 96;
export const assistantComposerDefaultHeight = 128;
export const assistantComposerMaxHeight = 320;

export function clampComposerHeight(height: number, minHeight = assistantComposerMinHeight, maxHeight = assistantComposerMaxHeight): number {
  if (!Number.isFinite(height)) return minHeight;
  return Math.min(Math.max(Math.round(height), minHeight), maxHeight);
}

export function useComposerResize({
  initialHeight = assistantComposerDefaultHeight,
  minHeight = assistantComposerMinHeight,
  maxHeight = assistantComposerMaxHeight,
}: {
  initialHeight?: number;
  minHeight?: number;
  maxHeight?: number;
} = {}) {
  const [composerHeight, setComposerHeight] = useState(() => clampComposerHeight(initialHeight, minHeight, maxHeight));

  const startComposerResize = useCallback((event: PointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    const startY = event.clientY;
    const startHeight = composerHeight;

    const handlePointerMove = (moveEvent: globalThis.PointerEvent) => {
      setComposerHeight(clampComposerHeight(startHeight + startY - moveEvent.clientY, minHeight, maxHeight));
    };
    const stopResize = () => {
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', stopResize);
      window.removeEventListener('pointercancel', stopResize);
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', stopResize);
    window.addEventListener('pointercancel', stopResize);
  }, [composerHeight, maxHeight, minHeight]);

  const resizeComposerWithKeyboard = useCallback((event: KeyboardEvent<HTMLDivElement>) => {
    const step = event.shiftKey ? 24 : 8;
    if (event.key === 'ArrowUp') {
      event.preventDefault();
      setComposerHeight(height => clampComposerHeight(height + step, minHeight, maxHeight));
    } else if (event.key === 'ArrowDown') {
      event.preventDefault();
      setComposerHeight(height => clampComposerHeight(height - step, minHeight, maxHeight));
    } else if (event.key === 'Home') {
      event.preventDefault();
      setComposerHeight(minHeight);
    } else if (event.key === 'End') {
      event.preventDefault();
      setComposerHeight(maxHeight);
    }
  }, [maxHeight, minHeight]);

  return {
    composerHeight,
    maxHeight,
    minHeight,
    resizeComposerWithKeyboard,
    startComposerResize,
  };
}
