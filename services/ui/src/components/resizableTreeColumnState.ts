import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type TouchEvent as ReactTouchEvent,
} from 'react';

export type ResizableTreeColumnOptions = {
  storageKey: string;
  defaultWidth: number;
  minWidth: number;
  maxWidth: number;
  keyboardStep?: number;
};

export function clampResizableTreeColumnWidth(value: number, minWidth: number, maxWidth: number): number {
  if (!Number.isFinite(value)) return minWidth;
  return Math.min(maxWidth, Math.max(minWidth, Math.round(value)));
}

export function useResizableTreeColumn({
  storageKey,
  defaultWidth,
  minWidth,
  maxWidth,
  keyboardStep = 16,
}: ResizableTreeColumnOptions) {
  const storageName = `treeColumnWidth:${storageKey}`;
  const [width, setWidth] = useState(() => {
    if (typeof window === 'undefined') return defaultWidth;
    const storedValue = window.localStorage.getItem(storageName);
    const stored = storedValue == null ? Number.NaN : Number(storedValue);
    return Number.isFinite(stored)
      ? clampResizableTreeColumnWidth(stored, minWidth, maxWidth)
      : clampResizableTreeColumnWidth(defaultWidth, minWidth, maxWidth);
  });
  const [isResizing, setIsResizing] = useState(false);
  const resizeStartXRef = useRef(0);
  const resizeStartWidthRef = useRef(width);

  const clampWidth = useCallback(
    (value: number) => clampResizableTreeColumnWidth(value, minWidth, maxWidth),
    [maxWidth, minWidth]
  );

  const startResize = useCallback(
    (event: ReactMouseEvent | ReactTouchEvent) => {
      if (typeof window !== 'undefined' && window.innerWidth < 900) return;
      const clientX = 'touches' in event ? event.touches[0]?.clientX : event.clientX;
      if (typeof clientX !== 'number') return;
      resizeStartXRef.current = clientX;
      resizeStartWidthRef.current = width;
      setIsResizing(true);
      event.stopPropagation();
      event.preventDefault();
    },
    [width]
  );

  const resizeWithKeyboard = useCallback(
    (event: ReactKeyboardEvent) => {
      let nextWidth: number | null = null;
      if (event.key === 'ArrowLeft') nextWidth = width - keyboardStep;
      if (event.key === 'ArrowRight') nextWidth = width + keyboardStep;
      if (event.key === 'Home') nextWidth = minWidth;
      if (event.key === 'End') nextWidth = maxWidth;
      if (nextWidth == null) return;
      event.preventDefault();
      setWidth(clampWidth(nextWidth));
    },
    [clampWidth, keyboardStep, maxWidth, minWidth, width]
  );

  useEffect(() => {
    if (typeof window === 'undefined') return;
    window.localStorage.setItem(storageName, String(width));
  }, [storageName, width]);

  useEffect(() => {
    if (!isResizing) return undefined;
    const handleMove = (event: MouseEvent | TouchEvent) => {
      const clientX = 'touches' in event ? event.touches[0]?.clientX : event.clientX;
      if (typeof clientX !== 'number') return;
      const delta = clientX - resizeStartXRef.current;
      setWidth(clampWidth(resizeStartWidthRef.current + delta));
    };
    const handleUp = () => setIsResizing(false);
    window.addEventListener('mousemove', handleMove);
    window.addEventListener('touchmove', handleMove);
    window.addEventListener('mouseup', handleUp);
    window.addEventListener('touchend', handleUp);
    return () => {
      window.removeEventListener('mousemove', handleMove);
      window.removeEventListener('touchmove', handleMove);
      window.removeEventListener('mouseup', handleUp);
      window.removeEventListener('touchend', handleUp);
    };
  }, [clampWidth, isResizing]);

  useEffect(() => {
    if (typeof document === 'undefined' || !isResizing) return undefined;
    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    return () => {
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
    };
  }, [isResizing]);

  const gridStyle = useMemo(
    () => ({ '--tree-column-width': `${width}px` }) as CSSProperties,
    [width]
  );

  return {
    gridStyle,
    isResizing,
    maxWidth,
    minWidth,
    resizeWithKeyboard,
    startResize,
    width,
  };
}
