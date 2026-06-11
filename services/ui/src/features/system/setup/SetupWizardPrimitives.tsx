import type { ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, Info } from 'lucide-react';
import { WIZARD_STEPS } from './model';

export function SetupStatusIcon({ status }: { status: string }) {
  if (status === 'success') return <CheckCircle2 className="h-4 w-4" />;
  if (status === 'error') return <AlertTriangle className="h-4 w-4" />;
  if (status === 'warning') return <AlertTriangle className="h-4 w-4" />;
  return <Info className="h-4 w-4" />;
}

export function StepIntro({ title, children, icon }: { title: string; children: ReactNode; icon: ReactNode }) {
  return (
    <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4">
      <div className="flex items-center gap-2 text-sm font-semibold">
        {icon}
        {title}
      </div>
      <div className="mt-2 text-sm leading-6 text-[var(--text-secondary)]">{children}</div>
    </div>
  );
}

export function WarningCallout({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-sm leading-6 text-amber-700 dark:text-amber-300">
      <div className="flex items-start gap-2">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
        <div>{children}</div>
      </div>
    </div>
  );
}

export function SetupStepNavigation({ wizardStepIndex, onSelectStep }: { wizardStepIndex: number; onSelectStep: (index: number) => void }) {
  return (
    <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-8">
      {WIZARD_STEPS.map((step, index) => (
        <button
          key={step.id}
          type="button"
          onClick={() => onSelectStep(index)}
          aria-label={`${step.label} ${step.required ? 'Required' : 'Optional'}`}
          aria-current={index === wizardStepIndex ? 'step' : undefined}
          className={`rounded-md border px-2 py-2 text-left text-xs ${index === wizardStepIndex ? 'border-[var(--border-accent)] bg-[var(--bg-tertiary)]' : 'border-[var(--border-primary)]'}`}
        >
          <span className="block font-semibold">{step.label}</span>
          <span className="text-[var(--text-secondary)]">{step.required ? 'Required' : 'Optional'}</span>
        </button>
      ))}
    </div>
  );
}
