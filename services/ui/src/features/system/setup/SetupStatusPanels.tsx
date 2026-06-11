import { AlertTriangle, CheckCircle2, Info } from 'lucide-react';
import { statusClasses, type BootstrapResponse, type SetupStatus, type SetupTemplates } from './model';
import { SetupStatusIcon } from './SetupWizardPrimitives';

type SetupStatusOverviewProps = {
  status: SetupStatus | null;
};

type SetupBootstrapResultProps = {
  bootstrapResult: BootstrapResponse | null;
};

type SetupStarterFilesPreviewProps = {
  templates: SetupTemplates | null;
  templatePaths: string[];
  selectedTemplatePath: string;
  selectedTemplate: string;
  onSelectedTemplatePathChange: (path: string) => void;
};

export function SetupStatusOverview({ status }: SetupStatusOverviewProps) {
  return (
    <section className="grid gap-4 xl:grid-cols-[1.2fr_0.8fr]">
      <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
        <div className="mb-3 flex items-center gap-2 text-sm font-semibold">
          <CheckCircle2 className="h-4 w-4" />
          Health checks
        </div>
        <div className="grid gap-3 md:grid-cols-2">
          {(status?.checks || []).map(check => (
            <div key={check.id} className={`rounded-lg border p-3 ${statusClasses(check.status)}`}>
              <div className="flex items-center gap-2 text-sm font-semibold">
                <SetupStatusIcon status={check.status} />
                {check.label}
              </div>
              {check.message && <div className="mt-2 text-xs leading-5">{check.message}</div>}
            </div>
          ))}
        </div>
      </div>

      <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
        <div className="mb-3 flex items-center gap-2 text-sm font-semibold">
          <Info className="h-4 w-4" />
          Resource counts
        </div>
        <div className="grid grid-cols-2 gap-2 text-sm">
          {status && Object.entries(status.counts).map(([key, value]) => (
            <div key={key} className="rounded-md border border-[var(--border-primary)] p-2">
              <div className="text-xs capitalize text-[var(--text-secondary)]">{key.replaceAll('_', ' ')}</div>
              <div className="text-lg font-semibold">{value}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

export function SetupBootstrapResult({ bootstrapResult }: SetupBootstrapResultProps) {
  if (!bootstrapResult) return null;

  return (
    <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
      <div className="mb-3 text-sm font-semibold">Bootstrap result</div>
      <div className="grid gap-3 lg:grid-cols-3">
        {(bootstrapResult.warnings || []).map(warning => (
          <div key={warning} className="rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300">
            <div className="flex items-start gap-2">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{warning}</span>
            </div>
          </div>
        ))}
        {(bootstrapResult.messages || []).map(message => (
          <div key={message} className="rounded-md border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-700 dark:text-emerald-300">{message}</div>
        ))}
        {(bootstrapResult.generated_secrets || []).map(secret => (
          <div key={secret} className="rounded-md border border-[var(--border-primary)] p-3 font-mono text-xs">{secret}</div>
        ))}
      </div>
      {(bootstrapResult.temporary_credentials || []).length > 0 && (
        <pre className="mt-3 overflow-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3 text-xs">{JSON.stringify(bootstrapResult.temporary_credentials, null, 2)}</pre>
      )}
    </section>
  );
}

export function SetupStarterFilesPreview({
  templates,
  templatePaths,
  selectedTemplatePath,
  selectedTemplate,
  onSelectedTemplatePathChange,
}: SetupStarterFilesPreviewProps) {
  if (!templates) return null;

  return (
    <section className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <div className="text-sm font-semibold">Starter files</div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <select className="max-w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm" value={selectedTemplatePath} onChange={event => onSelectedTemplatePathChange(event.target.value)}>
            {templatePaths.map(path => <option key={path} value={path}>{path}</option>)}
          </select>
        </div>
      </div>
      <pre className="max-h-[520px] overflow-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4 text-xs leading-5">{selectedTemplate}</pre>
    </section>
  );
}
