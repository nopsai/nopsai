import { useEffect, useRef } from 'react';

type DismissTarget = { readonly current: Node | null };

export function useOutsideDismiss(
  targets: DismissTarget | DismissTarget[],
  active: boolean,
  onDismiss: () => void,
  { escape = true, ignore = [] }: { escape?: boolean; ignore?: string[] } = {}
) {
  const dismissRef = useRef(onDismiss);

  useEffect(() => {
    dismissRef.current = onDismiss;
  }, [onDismiss]);

  useEffect(() => {
    if (!active) return undefined;
    const targetRefs = Array.isArray(targets) ? targets : [targets];

    const isInside = (target: EventTarget | null) => {
      if (!(target instanceof Node)) return false;
      if (target instanceof Element && ignore.some(selector => target.closest(selector))) return true;
      return targetRefs.some(ref => {
        const node = ref.current;
        return node ? node.contains(target) : false;
      });
    };

    const handlePointerDown = (event: PointerEvent) => {
      if (isInside(event.target)) return;
      dismissRef.current();
    };

    const handleKeyDown = (event: KeyboardEvent) => {
      if (!escape || event.key !== 'Escape') return;
      dismissRef.current();
    };

    document.addEventListener('pointerdown', handlePointerDown, true);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown, true);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [active, escape, ignore, targets]);
}
