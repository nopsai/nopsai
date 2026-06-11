import {
  BookOpen,
  Braces,
  FileText,
  Lock,
  ShieldCheck,
  type LucideIcon,
} from 'lucide-react';

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

export function kindIcon(kind: string): LucideIcon {
  switch (kind) {
    case 'guardrail':
      return ShieldCheck;
    case 'policy':
      return Lock;
    case 'runbook':
      return Braces;
    case 'reference':
      return BookOpen;
    case 'example':
      return FileText;
    default:
      return FileText;
  }
}

export function formatKnowledgeDate(value?: string) {
  if (!value) return '-';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString();
}
