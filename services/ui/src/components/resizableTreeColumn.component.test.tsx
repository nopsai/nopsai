import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, test } from 'vitest';
import {
  TreeColumnResizeHandle,
  clampResizableTreeColumnWidth,
  useResizableTreeColumn,
} from './resizableTreeColumn';

function ResizableTreeHarness({ storageKey = 'test' }: { storageKey?: string }) {
  const resize = useResizableTreeColumn({
    storageKey,
    defaultWidth: 280,
    minWidth: 240,
    maxWidth: 420,
  });

  return (
    <div data-testid="tree-grid" style={resize.gridStyle}>
      <TreeColumnResizeHandle {...resize} label="Resize test tree" />
    </div>
  );
}

describe('resizable tree column', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  test('clamps persisted widths to the configured bounds', () => {
    expect(clampResizableTreeColumnWidth(500, 240, 420)).toBe(420);
    expect(clampResizableTreeColumnWidth(120, 240, 420)).toBe(240);
    expect(clampResizableTreeColumnWidth(Number.NaN, 240, 420)).toBe(240);

    localStorage.setItem('treeColumnWidth:test', '999');
    render(<ResizableTreeHarness />);

    expect(screen.getByTestId('tree-grid').style.getPropertyValue('--tree-column-width')).toBe('420px');
    expect(screen.getByRole('separator', { name: 'Resize test tree' })).toHaveAttribute('aria-valuenow', '420');
  });

  test('supports keyboard resizing and persists the selected width', () => {
    render(<ResizableTreeHarness />);

    const grid = screen.getByTestId('tree-grid');
    const handle = screen.getByRole('separator', { name: 'Resize test tree' });

    expect(grid.style.getPropertyValue('--tree-column-width')).toBe('280px');

    fireEvent.keyDown(handle, { key: 'ArrowRight' });
    expect(grid.style.getPropertyValue('--tree-column-width')).toBe('296px');
    expect(localStorage.getItem('treeColumnWidth:test')).toBe('296');

    fireEvent.keyDown(handle, { key: 'Home' });
    expect(grid.style.getPropertyValue('--tree-column-width')).toBe('240px');

    fireEvent.keyDown(handle, { key: 'End' });
    expect(grid.style.getPropertyValue('--tree-column-width')).toBe('420px');
  });

  test('supports pointer dragging', () => {
    render(<ResizableTreeHarness storageKey="drag" />);

    const grid = screen.getByTestId('tree-grid');
    const handle = screen.getByRole('separator', { name: 'Resize test tree' });

    fireEvent.mouseDown(handle, { clientX: 100 });
    fireEvent.mouseMove(window, { clientX: 150 });
    fireEvent.mouseUp(window);

    expect(grid.style.getPropertyValue('--tree-column-width')).toBe('330px');
    expect(localStorage.getItem('treeColumnWidth:drag')).toBe('330');
  });
});
