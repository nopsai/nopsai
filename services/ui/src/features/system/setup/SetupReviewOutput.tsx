import { Download, FileText } from 'lucide-react';
import { LLM_SKIP_WARNING, type RepositoryTeamSummary, type RuntimeEnvSection } from './model';
import { StepIntro, WarningCallout } from './SetupWizardPrimitives';

type SetupReviewOutputProps = {
  aiEnabled: boolean;
  normalizedRepositoryTeams: RepositoryTeamSummary[];
  repositories: string[];
  userCount: number;
  runtimeEnvSections: RuntimeEnvSection[];
  environmentSnippet: string;
  gitOpsStructureSnippet: string;
  gitOpsFiles: string[];
  templateLoading: boolean;
  templatesLoaded: boolean;
  downloadingGitOpsZip: boolean;
  onLoadTemplates: () => void;
  onDownloadGitOpsZip: () => void;
};

function downloadTextFile(fileName: string, content: string, mimeType = 'text/plain;charset=utf-8') {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = fileName;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

export function SetupReviewOutput({
  aiEnabled,
  normalizedRepositoryTeams,
  repositories,
  userCount,
  runtimeEnvSections,
  environmentSnippet,
  gitOpsStructureSnippet,
  gitOpsFiles,
  templateLoading,
  templatesLoaded,
  downloadingGitOpsZip,
  onLoadTemplates,
  onDownloadGitOpsZip,
}: SetupReviewOutputProps) {
  return (
    <div className="space-y-4">
      <StepIntro title="Generated setup output" icon={<FileText className="h-4 w-4" />}>
        Review what will be created now and what should be applied outside the database. Runtime variables are organized by target container; GitOps starter files can be previewed in the page and downloaded as one zip for the global config repository.
      </StepIntro>
      {!aiEnabled && <WarningCallout>{LLM_SKIP_WARNING}</WarningCallout>}
      <div className="grid gap-3 md:grid-cols-3">
        <div className="rounded-md border border-[var(--border-primary)] p-3 text-sm">
          <div className="text-xs text-[var(--text-secondary)]">Repository teams</div>
          <div className="mt-1">{normalizedRepositoryTeams.length || 'Skipped'}</div>
        </div>
        <div className="rounded-md border border-[var(--border-primary)] p-3 text-sm">
          <div className="text-xs text-[var(--text-secondary)]">Repositories</div>
          <div className="mt-1">{repositories.length || 'Skipped'}</div>
        </div>
        <div className="rounded-md border border-[var(--border-primary)] p-3 text-sm">
          <div className="text-xs text-[var(--text-secondary)]">Users</div>
          <div className="mt-1">{userCount || 'Skipped'}</div>
        </div>
      </div>
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="text-sm font-semibold">Runtime variables by container</div>
          <button type="button" className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] px-3 py-2 text-sm" onClick={() => downloadTextFile('nopsai-runtime-env.txt', environmentSnippet)}>
            <Download className="h-4 w-4" />
            Download all env
          </button>
        </div>
        <div className="grid gap-3 xl:grid-cols-3">
          {runtimeEnvSections.map(section => (
            <div key={section.fileName} className="rounded-md border border-[var(--border-primary)] p-3">
              <div className="mb-2 flex items-center justify-between gap-2">
                <div className="text-sm font-semibold">{section.title}</div>
                <button type="button" className="rounded-md border border-[var(--border-primary)] p-2" onClick={() => downloadTextFile(section.fileName, section.lines.join('\n'))} title={`Download ${section.fileName}`}>
                  <Download className="h-4 w-4" />
                </button>
              </div>
              <pre className="max-h-72 overflow-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3 text-xs leading-5">{section.lines.join('\n')}</pre>
            </div>
          ))}
        </div>
      </div>
      <div>
        <div className="mb-2 text-sm font-semibold">GitOps team files</div>
        <pre className="max-h-80 overflow-auto rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3 text-xs leading-5">{gitOpsStructureSnippet}</pre>
      </div>
      <div className="rounded-md border border-[var(--border-primary)] p-3 text-sm">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <div className="font-semibold">Files to commit when using GitOps</div>
            <div className="mt-1 text-xs text-[var(--text-secondary)]">The zip preserves repository paths such as `pipelines/`, `steps/`, `triggers/`, and `setting/system/`.</div>
          </div>
          <button type="button" className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] px-3 py-2 text-sm" onClick={onDownloadGitOpsZip} disabled={downloadingGitOpsZip}>
            <Download className="h-4 w-4" />
            {downloadingGitOpsZip ? 'Downloading...' : 'Download GitOps zip'}
          </button>
        </div>
        <div className="mt-3 grid gap-2 md:grid-cols-2">
          {gitOpsFiles.map(file => <div key={file} className="rounded border border-[var(--border-primary)] px-2 py-1 font-mono text-xs">{file}</div>)}
        </div>
      </div>
      <div className="rounded-md border border-sky-500/30 bg-sky-500/10 p-3 text-sm leading-6 text-sky-700 dark:text-sky-300">
        Apply each env block to the matching container or secret manager, configure provider webhook settings on the git-bot deployment, commit the GitOps zip if you are using GitOps, restart services that received new runtime values, and run `setup/first-run` to prove runner, agent, logs, and UI are working{aiEnabled ? ', including AI.' : '.'}
      </div>
      <div className="flex flex-wrap gap-2">
        <button className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] px-4 py-2 text-sm" onClick={onLoadTemplates} disabled={templateLoading}>
          <FileText className="h-4 w-4" />
          {templateLoading ? 'Loading...' : templatesLoaded ? 'Refresh file preview' : 'Preview GitOps files'}
        </button>
      </div>
    </div>
  );
}
