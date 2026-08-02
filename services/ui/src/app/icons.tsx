import { Menu, X } from 'lucide-react';
import { ObjectIcon } from '../components/ObjectIcon.js';

export function IconX() {
  return <X className="h-6 w-6" strokeWidth={2} aria-hidden="true" />;
}

export function IconMenu() {
  return <Menu className="h-6 w-6" strokeWidth={2} aria-hidden="true" />;
}

export function IconPlay() {
  return <ObjectIcon type="pipeline-run" />;
}

export function IconFlow() {
  return <ObjectIcon type="pipeline" />;
}

export function IconMonitoring() {
  return <ObjectIcon type="monitoring" />;
}

export function RunIdIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M4 7h4v10H4z" />
      <path d="M12 7h8" />
      <path d="M12 12h8" />
      <path d="M12 17h8" />
    </svg>
  );
}

export function BranchIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <line x1="6" y1="3" x2="6" y2="15" />
      <circle cx="18" cy="6" r="3" />
      <circle cx="6" cy="18" r="3" />
      <path d="M18 9a9 9 0 01-9 9" />
    </svg>
  );
}

export function IconBell() {
  return <ObjectIcon type="trigger" />;
}

export function IconZap() {
  return <ObjectIcon type="external-trigger" />;
}

export function IconCalendarSchedule() {
  return <ObjectIcon type="schedule" />;
}

export function IconScope() {
  return <ObjectIcon type="scope" />;
}

export function IconFlask() {
  return <ObjectIcon type="lab" />;
}

export function IconCog() {
  return <ObjectIcon type="system-config" />;
}

export function IconDatabase() {
  return <ObjectIcon type="data-management" />;
}

export function IconDispatch() {
  return <ObjectIcon type="dispatcher" />;
}

export function IconShield() {
  return <ObjectIcon type="access" />;
}

export function IconSteps() {
  return <ObjectIcon type="step" />;
}

export function IconKnowledge() {
  return <ObjectIcon type="knowledge-context" />;
}

export function IconDocs() {
  return <ObjectIcon type="knowledge-context" />;
}
