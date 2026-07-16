import { render } from '@testing-library/react';
import { BrainCircuit } from 'lucide-react';
import { describe, expect, it } from 'vitest';
import { ObjectIcon } from './ObjectIcon';
import {
  getObjectIconComponent,
  objectIconRegistry,
  objectIconTypes,
  type ObjectIconType,
} from './objectIconRegistry';

describe('ObjectIcon', () => {
  it('keeps the typed object icon list and registry in sync', () => {
    expect(Object.keys(objectIconRegistry).sort()).toEqual([...objectIconTypes].sort());
  });

  it('renders every registered object icon with the shared vector contract', () => {
    objectIconTypes.forEach(type => {
      const { container, unmount } = render(<ObjectIcon type={type} />);
      const svg = container.querySelector('svg');

      expect(svg).not.toBeNull();
      expect(svg).toHaveClass('h-4', 'w-4');
      expect(svg).toHaveAttribute('aria-hidden', 'true');

      unmount();
    });
  });

  it('allows card surfaces to override size while resolving a stable component', () => {
    const type: ObjectIconType = 'pipeline';
    const Icon = getObjectIconComponent(type);
    const { container } = render(<ObjectIcon type={type} className="h-5 w-5" strokeWidth={2.2} />);
    const svg = container.querySelector('svg');

    expect(Icon).toBe(objectIconRegistry[type]);
    expect(svg).toHaveClass('h-5', 'w-5');
    expect(svg).toHaveAttribute('stroke-width', '2.2');
  });

  it('uses the dedicated LLM profile glyph instead of the generic bot icon', () => {
    expect(getObjectIconComponent('llm-profile')).toBe(BrainCircuit);
  });
});
