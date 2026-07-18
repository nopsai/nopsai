import {
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type TouchEvent as ReactTouchEvent,
} from 'react';

export function TreeColumnResizeHandle({
  isResizing,
  maxWidth,
  minWidth,
  resizeWithKeyboard,
  startResize,
  width,
  label = 'Resize tree panel',
}: {
  isResizing: boolean;
  maxWidth: number;
  minWidth: number;
  resizeWithKeyboard: (event: ReactKeyboardEvent) => void;
  startResize: (event: ReactMouseEvent | ReactTouchEvent) => void;
  width: number;
  label?: string;
}) {
  return (
    <div
      className={`tree-column-resizer ${isResizing ? 'tree-column-resizer--active' : ''}`}
      onMouseDown={startResize}
      onTouchStart={startResize}
      onKeyDown={resizeWithKeyboard}
      role="separator"
      tabIndex={0}
      aria-label={label}
      aria-orientation="vertical"
      aria-valuemin={minWidth}
      aria-valuemax={maxWidth}
      aria-valuenow={width}
      title="Drag to resize tree panel"
    />
  );
}
