import { createElement } from 'react';
import { getObjectIconComponent, type ObjectIconType } from './objectIconRegistry';

export function ObjectIcon({
  type,
  className = 'h-4 w-4',
  strokeWidth = 1.9,
}: {
  type: ObjectIconType;
  className?: string;
  strokeWidth?: number;
}) {
  return createElement(getObjectIconComponent(type), {
    className,
    strokeWidth,
    'aria-hidden': 'true',
  });
}
