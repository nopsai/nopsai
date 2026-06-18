import type { ConfigFieldMetadata } from './model';

export function ApplyBadge({ metadata }: { metadata?: ConfigFieldMetadata }) {
  if (!metadata?.apply) return null;
  return (
    <span className="runner-pill runner-pill--muted text-[10px] leading-4">
      {metadata.apply}
    </span>
  );
}
