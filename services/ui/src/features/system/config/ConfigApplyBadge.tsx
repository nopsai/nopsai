import type { ConfigFieldMetadata } from './model';

export function ApplyBadge({ metadata }: { metadata?: ConfigFieldMetadata }) {
  if (!metadata?.apply) return null;
  return (
    <span className="system-settings-apply-badge">
      {metadata.apply}
    </span>
  );
}
