import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useSidebarState } from './useSidebarState';

describe('useSidebarState', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('persists desktop collapse state separately from mobile open state', () => {
    const { result, rerender } = renderHook(({ pathname }) => useSidebarState(pathname), {
      initialProps: { pathname: '/knowledge-context' },
    });

    expect(result.current.collapsed).toBe(false);
    expect(result.current.open).toBe(false);

    act(() => result.current.toggleCollapsed());

    expect(result.current.collapsed).toBe(true);
    expect(localStorage.getItem('sidebarCollapsed')).toBe('true');

    act(() => result.current.openSidebar());
    expect(result.current.open).toBe(true);

    act(() => {
      rerender({ pathname: '/pipelines' });
    });
    act(() => {
      vi.runAllTimers();
    });
    expect(result.current.open).toBe(false);
    expect(result.current.collapsed).toBe(true);
  });

  it('hydrates collapsed state from local storage', () => {
    localStorage.setItem('sidebarCollapsed', 'true');

    const { result } = renderHook(() => useSidebarState('/pipelines'));

    expect(result.current.collapsed).toBe(true);
  });
});
