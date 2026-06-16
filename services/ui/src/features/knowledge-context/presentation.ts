import type { LucideIcon } from 'lucide-react';
import { getObjectIconComponent, type ObjectIconType } from '../../components/objectIconRegistry';

export function kindTitle(kind: string) {
  if (kind === 'adr') return 'ADR';
  return `${kind.charAt(0).toUpperCase()}${kind.slice(1)}`;
}

export function kindPlural(kind: string) {
  if (kind === 'architecture') return 'Architecture';
  if (kind === 'adr') return 'ADRs';
  if (kind === 'policy') return 'Policies';
  if (kind === 'reference') return 'References';
  return `${kindTitle(kind)}s`;
}

export function kindIconType(kind: string): ObjectIconType {
  switch (kind) {
    case 'guardrail':
      return 'knowledge-guardrail';
    case 'policy':
      return 'knowledge-policy';
    case 'runbook':
      return 'knowledge-runbook';
    case 'reference':
      return 'knowledge-reference';
    case 'example':
      return 'knowledge-example';
    default:
      return 'knowledge-default';
  }
}

export function kindIcon(kind: string): LucideIcon {
  return getObjectIconComponent(kindIconType(kind));
}

export function formatKnowledgeDate(value?: string) {
  if (!value) return '-';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString();
}
