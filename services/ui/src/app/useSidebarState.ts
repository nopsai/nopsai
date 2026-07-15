import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type TouchEvent as ReactTouchEvent,
} from 'react';
import { SIDEBAR_DEFAULT_WIDTH, SIDEBAR_MAX_WIDTH, SIDEBAR_MIN_WIDTH } from './constants';

export function useSidebarState(pathname: string) {
  const [open, setOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === 'undefined') return false;
    return localStorage.getItem('sidebarCollapsed') === 'true';
  });
  const [width, setWidth] = useState<number>(() => {
    if (typeof window === 'undefined') return SIDEBAR_DEFAULT_WIDTH;
    const stored = Number(localStorage.getItem('sidebarWidth'));
    if (Number.isFinite(stored)) return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, stored));
    return SIDEBAR_DEFAULT_WIDTH;
  });
  const [isResizing, setIsResizing] = useState(false);
  const resizeStartXRef = useRef(0);
  const resizeStartWidthRef = useRef(SIDEBAR_DEFAULT_WIDTH);

  useEffect(() => {
    if (!open) return;
    const handle = window.setTimeout(() => setOpen(false), 0);
    return () => window.clearTimeout(handle);
  }, [open, pathname]);

  const close = useCallback(() => setOpen(false), []);
  const openSidebar = useCallback(() => setOpen(true), []);
  const toggleCollapsed = useCallback(() => setCollapsed(value => !value), []);

  const clampSidebarWidth = useCallback(
    (value: number) => Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, value)),
    []
  );

  const startResize = useCallback(
    (event: ReactMouseEvent | ReactTouchEvent) => {
      if (typeof window !== 'undefined' && window.innerWidth < 640) return;
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
      if (event.key === 'ArrowLeft') nextWidth = width - 16;
      if (event.key === 'ArrowRight') nextWidth = width + 16;
      if (event.key === 'Home') nextWidth = SIDEBAR_MIN_WIDTH;
      if (event.key === 'End') nextWidth = SIDEBAR_MAX_WIDTH;
      if (nextWidth === null) return;
      event.preventDefault();
      setWidth(clampSidebarWidth(nextWidth));
    },
    [clampSidebarWidth, width]
  );

  useEffect(() => {
    if (typeof window === 'undefined') return;
    localStorage.setItem('sidebarWidth', String(width));
  }, [width]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    localStorage.setItem('sidebarCollapsed', String(collapsed));
  }, [collapsed]);

  useEffect(() => {
    if (!isResizing) return undefined;
    const handleMove = (event: MouseEvent | TouchEvent) => {
      const clientX = 'touches' in event ? event.touches[0]?.clientX : event.clientX;
      if (typeof clientX !== 'number') return;
      const delta = clientX - resizeStartXRef.current;
      setWidth(clampSidebarWidth(resizeStartWidthRef.current + delta));
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
  }, [clampSidebarWidth, isResizing]);

  useEffect(() => {
    if (typeof document === 'undefined') return undefined;
    if (!isResizing) return undefined;
    const prevCursor = document.body.style.cursor;
    const prevUserSelect = document.body.style.userSelect;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    return () => {
      document.body.style.cursor = prevCursor;
      document.body.style.userSelect = prevUserSelect;
    };
  }, [isResizing]);

  return {
    close,
    collapsed,
    isResizing,
    open,
    openSidebar,
    resizeWithKeyboard,
    startResize,
    toggleCollapsed,
    width,
  };
}
