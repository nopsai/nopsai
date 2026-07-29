import { useRef, useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { AlertCircle, CheckCircle2, Clipboard, Download, Eye, FileText, LayoutDashboard, Loader2, MoreHorizontal, Square } from 'lucide-react';
import { useOutsideDismiss } from '../../components/useOutsideDismiss';
import { apiClient } from '../../lib/api';
import { copyTextToClipboard } from '../../lib/clipboard';
import type { PipelineDefinition, PipelineRunFinalOutput } from './contracts';
import { finalOutputDashboardLink } from './finalOutputs';
import { FinalOutputPreview } from './final-output-preview/FinalOutputPreview';
import { documentSpecToText, parseDocumentSpec, parseSpreadsheetSpec, spreadsheetSpecToText } from './final-output-preview/finalOutputSpecs';

type RunFinalOutputsProps = {
  runID: string;
  outputs?: PipelineRunFinalOutput[];
  pipelineDefinition?: PipelineDefinition | null;
  onCancelOutput?: (outputId: string) => void | Promise<void>;
};

type FileSaveHandle = {
  createWritable: () => Promise<{
    write: (data: Blob) => Promise<void>;
    close: () => Promise<void>;
  }>;
};

type FileSavePickerWindow = Window & {
  showSaveFilePicker?: (options: {
    suggestedName?: string;
    types?: Array<{
      description: string;
      accept: Record<string, string[]>;
    }>;
  }) => Promise<FileSaveHandle>;
};

const actionClass =
  'inline-flex items-center gap-2 rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-1.5 text-xs font-semibold text-[var(--text-primary)] transition hover:border-indigo-300/60 hover:text-indigo-600 disabled:cursor-not-allowed disabled:opacity-60 dark:border-white/10 dark:bg-white/5 dark:text-[var(--text-primary)] dark:hover:bg-white/10';

const menuItemClass =
  'flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-xs font-semibold text-[var(--text-primary)] transition hover:bg-[var(--bg-tertiary)] disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-white/10';

export function RunFinalOutputs({ runID, outputs = [], pipelineDefinition = null, onCancelOutput }: RunFinalOutputsProps) {
  const [previewID, setPreviewID] = useState<string | null>(null);
  const [pendingAction, setPendingAction] = useState<string | null>(null);
  const [message, setMessage] = useState<string>('');

  if (outputs.length === 0) return null;

  const handleCopy = async (output: PipelineRunFinalOutput) => {
    const content = copyableFinalOutputContent(output);
    if (!content) return;
    const key = `copy:${output.id}`;
    setPendingAction(key);
    setMessage('');
    try {
      await copyTextToClipboard(content);
      setMessage(`${output.name} copied`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Copy failed');
    } finally {
      setPendingAction(null);
    }
  };

  const handleDownload = async (output: PipelineRunFinalOutput) => {
    const key = `download:${output.id}`;
    setPendingAction(key);
    setMessage('');
    const fallbackName = fallbackOutputFilename(output);
    let saveHandle: FileSaveHandle | null = null;
    try {
      saveHandle = await requestFileSaveHandle(output, fallbackName);
      const response = await apiClient.fetch(`/v1/runs/${encodeURIComponent(runID)}/outputs/${encodeURIComponent(output.id)}/download`, {
        cache: 'no-store',
      });
      if (!response.ok) {
        throw new Error((await response.text()) || `Download failed (${response.status})`);
      }
      const blob = await response.blob();
      if (saveHandle) {
        await writeBlobToSaveHandle(saveHandle, blob);
        setMessage(`${output.name} saved`);
      } else {
        downloadBlob(blob, filenameFromContentDisposition(response.headers.get('content-disposition')) || fallbackName);
      }
    } catch (error) {
      if (isAbortError(error)) {
        setMessage('Download cancelled');
        return;
      }
      setMessage(error instanceof Error ? error.message : 'Download failed');
    } finally {
      setPendingAction(null);
    }
  };

  const handleCancel = async (output: PipelineRunFinalOutput) => {
    if (!onCancelOutput) return;
    const key = `cancel:${output.id}`;
    setPendingAction(key);
    setMessage('');
    try {
      await onCancelOutput(output.id);
      setMessage(`${output.name || 'Output'} cancellation requested`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Cancel failed');
    } finally {
      setPendingAction(null);
    }
  };

  return (
    <section className="border border-[var(--border-primary)] rounded-lg bg-white dark:bg-slate-950 p-3 space-y-3 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="font-semibold text-[var(--text-primary)]">Final Outputs</h3>
        <span className="text-xs text-[var(--text-secondary)]">{outputs.length} deliverable{outputs.length === 1 ? '' : 's'}</span>
      </div>
      <div className="space-y-3">
        {outputs.map(output => {
          const ready = output.status === 'success' && Boolean(output.content);
          const cancellable = output.status === 'pending' || output.status === 'generating';
          const expanded = previewID === output.id;
          const timingText = finalOutputTimingText(output);
          const dashboardLink = finalOutputDashboardLink(output, pipelineDefinition);
          const previewLabel = `${expanded ? 'Hide preview' : 'Preview'} ${output.name || 'output'}`;
          return (
            <div key={output.id} className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 text-sm">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <button
                  type="button"
                  className="min-w-0 flex-1 space-y-2 rounded-lg text-left outline-none transition hover:bg-[var(--bg-tertiary)] focus-visible:ring-2 focus-visible:ring-[var(--border-accent)] disabled:cursor-default disabled:hover:bg-transparent"
                  onClick={() => setPreviewID(expanded ? null : output.id)}
                  disabled={!ready}
                  aria-label={ready ? previewLabel : `${output.name || 'Output'} is not ready for preview`}
                  aria-expanded={ready ? expanded : undefined}
                  aria-controls={ready ? `final-output-preview-${output.id}` : undefined}
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <FinalOutputStatus status={output.status} />
                    <span className="font-semibold text-[var(--text-primary)] break-words">{output.name || 'Untitled output'}</span>
                    <span className="runner-pill runner-pill--muted">{formatOutputType(output.type)}</span>
                    {output.llm_profile && <span className="runner-pill runner-pill--muted">LLM {output.llm_profile}</span>}
                  </div>
                  {timingText ? <div className="text-xs text-[var(--text-secondary)]">{timingText}</div> : null}
                  {output.error && <div className="text-xs text-red-600 dark:text-red-300 break-words">{output.error}</div>}
                </button>
                <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
                  {dashboardLink ? <FinalOutputDashboardLink link={dashboardLink} /> : null}
                  <FinalOutputActionsMenu
                    output={output}
                    ready={ready}
                    expanded={expanded}
                    pendingAction={pendingAction}
                    cancellable={cancellable}
                    canCancel={Boolean(onCancelOutput)}
                    onTogglePreview={() => setPreviewID(expanded ? null : output.id)}
                    onCopy={() => void handleCopy(output)}
                    onDownload={() => void handleDownload(output)}
                    onCancel={() => void handleCancel(output)}
                  />
                </div>
              </div>
              {expanded && ready && (
                <div id={`final-output-preview-${output.id}`} className="mt-3 rounded-lg border border-[var(--border-primary)] bg-white dark:bg-black/20 p-3">
                  <FinalOutputPreview runID={runID} output={output} />
                </div>
              )}
            </div>
          );
        })}
      </div>
      {message && <div className="text-xs text-[var(--text-secondary)]">{message}</div>}
    </section>
  );
}

function FinalOutputDashboardLink({ link }: { link: NonNullable<ReturnType<typeof finalOutputDashboardLink>> }) {
  return (
    <Link
      to={link.href}
      className="inline-flex h-9 max-w-full items-center gap-2 rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 text-xs font-semibold text-[var(--text-primary)] transition hover:border-indigo-300/60 hover:text-indigo-600"
      aria-label={`Open dashboard ${link.ref}${link.section ? ` section ${link.section}` : ''}`}
      title={link.label}
    >
      <LayoutDashboard className="h-4 w-4 shrink-0" aria-hidden="true" />
      <span className="min-w-0 truncate">{link.label}</span>
    </Link>
  );
}

function FinalOutputActionsMenu({
  output,
  ready,
  expanded,
  pendingAction,
  cancellable,
  canCancel,
  onTogglePreview,
  onCopy,
  onDownload,
  onCancel,
}: {
  output: PipelineRunFinalOutput;
  ready: boolean;
  expanded: boolean;
  pendingAction: string | null;
  cancellable: boolean;
  canCancel: boolean;
  onTogglePreview: () => void;
  onCopy: () => void;
  onDownload: () => void;
  onCancel: () => void;
}) {
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const close = () => setOpen(false);
  const name = output.name || 'output';
  useOutsideDismiss(menuRef, open, close);

  return (
    <div className="relative shrink-0" ref={menuRef}>
      <button
        className={actionClass}
        type="button"
        aria-label={`Actions for ${name}`}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen(current => !current)}
      >
        <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
        Actions
      </button>
      {open ? (
        <div
          role="menu"
          aria-label={`Actions for ${name}`}
          className="absolute right-0 z-40 mt-2 grid min-w-48 gap-1 rounded-xl border border-[var(--border-primary)] bg-white p-1.5 shadow-xl dark:border-white/10 dark:bg-slate-950"
        >
          <FinalOutputActionItem
            label={expanded ? 'Hide preview' : 'Preview'}
            icon={<Eye className="h-4 w-4" aria-hidden="true" />}
            disabled={!ready}
            close={close}
            onClick={onTogglePreview}
          />
          <FinalOutputActionItem
            label={pendingAction === `copy:${output.id}` ? 'Copying' : 'Copy'}
            icon={<Clipboard className="h-4 w-4" aria-hidden="true" />}
            disabled={!ready || pendingAction === `copy:${output.id}`}
            close={close}
            onClick={onCopy}
          />
          <FinalOutputActionItem
            label={pendingAction === `download:${output.id}` ? 'Downloading' : 'Download'}
            icon={<Download className="h-4 w-4" aria-hidden="true" />}
            disabled={output.status !== 'success' || pendingAction === `download:${output.id}`}
            close={close}
            onClick={onDownload}
          />
          {canCancel ? (
            <FinalOutputActionItem
              label={pendingAction === `cancel:${output.id}` ? 'Cancelling' : 'Cancel'}
              icon={<Square className="h-4 w-4" aria-hidden="true" />}
              disabled={!cancellable || pendingAction === `cancel:${output.id}`}
              close={close}
              onClick={onCancel}
            />
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function FinalOutputActionItem({
  label,
  icon,
  disabled,
  close,
  onClick,
}: {
  label: string;
  icon: ReactNode;
  disabled?: boolean;
  close: () => void;
  onClick: () => void;
}) {
  const handleClick = () => {
    if (disabled) return;
    close();
    onClick();
  };
  return (
    <button type="button" role="menuitem" className={menuItemClass} disabled={disabled} onClick={handleClick}>
      {icon}
      <span className="truncate">{label}</span>
    </button>
  );
}

function copyableFinalOutputContent(output: PipelineRunFinalOutput) {
  const content = output.content || '';
  if (output.type === 'pdf' || output.type === 'html') {
    const document = parseDocumentSpec(content);
    if (document) return documentSpecToText(document);
  }
  if (output.type === 'excel') {
    const spreadsheet = parseSpreadsheetSpec(content);
    if (spreadsheet) return spreadsheetSpecToText(spreadsheet);
  }
  return content;
}

function FinalOutputStatus({ status }: { status: string }) {
  if (status === 'success') {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-1 text-xs font-semibold text-emerald-700 dark:text-emerald-200">
        <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />
        Ready
      </span>
    );
  }
  if (status === 'failure') {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full border border-red-500/30 bg-red-500/10 px-2 py-1 text-xs font-semibold text-red-700 dark:text-red-200">
        <AlertCircle className="h-3.5 w-3.5" aria-hidden="true" />
        Failed
      </span>
    );
  }
  if (status === 'cancelled') {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full border border-slate-500/30 bg-slate-500/10 px-2 py-1 text-xs font-semibold text-slate-700 dark:text-slate-200">
        <Square className="h-3.5 w-3.5" aria-hidden="true" />
        Cancelled
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-blue-500/30 bg-blue-500/10 px-2 py-1 text-xs font-semibold text-blue-700 dark:text-blue-200">
      {status === 'generating' ? <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" /> : <FileText className="h-3.5 w-3.5" aria-hidden="true" />}
      {status === 'generating' ? 'Generating' : 'Pending'}
    </span>
  );
}

function formatOutputType(type: string) {
  if (!type) return 'Output';
  return type.charAt(0).toUpperCase() + type.slice(1);
}

function finalOutputTimingText(output: PipelineRunFinalOutput) {
  const timestamp = formatOutputTimestamp(output.updated_at || output.created_at);
  const duration = formatOutputGenerationDuration(output);
  const verb = output.status === 'success'
    ? 'Generated'
    : output.status === 'failure'
      ? 'Failed'
      : output.status === 'cancelled'
        ? 'Cancelled'
        : output.status === 'generating'
          ? 'Generating'
          : 'Queued';
  const parts = [timestamp ? `${verb} ${timestamp}` : '', duration ? `${duration} duration` : ''].filter(Boolean);
  return parts.join(' / ');
}

function formatOutputTimestamp(value?: string | null) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}

function formatOutputGenerationDuration(output: PipelineRunFinalOutput) {
  if (output.generation_duration) return output.generation_duration;
  const seconds = Number(output.generation_duration_seconds || 0);
  if (Number.isFinite(seconds) && seconds > 0) return formatDurationSeconds(seconds);
  const started = output.generation_started_at ? Date.parse(output.generation_started_at) : Number.NaN;
  const updated = output.updated_at ? Date.parse(output.updated_at) : Number.NaN;
  if (Number.isFinite(started) && Number.isFinite(updated) && updated >= started) {
    return formatDurationSeconds((updated - started) / 1000);
  }
  if (output.status === 'generating' && Number.isFinite(started)) {
    return formatDurationSeconds(Math.max(0, (Date.now() - started) / 1000));
  }
  return '';
}

function formatDurationSeconds(rawSeconds: number) {
  const seconds = Math.max(0, Math.round(rawSeconds));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) return remainingSeconds ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
}

function filenameFromContentDisposition(value: string | null) {
  const match = /filename="([^"]+)"/i.exec(value || '');
  return match?.[1] || '';
}

function fallbackOutputFilename(output: PipelineRunFinalOutput) {
  const base = (output.name || 'final-output').toLowerCase().replace(/[^a-z0-9_.-]+/g, '-').replace(/^-+|-+$/g, '') || 'final-output';
  const extension = output.type === 'markdown' ? 'md' : output.type === 'excel' ? 'xlsx' : output.type || 'txt';
  return `${base}.${extension}`;
}

async function requestFileSaveHandle(output: PipelineRunFinalOutput, suggestedName: string): Promise<FileSaveHandle | null> {
  const pickerWindow = window as unknown as FileSavePickerWindow;
  if (!window.isSecureContext || typeof pickerWindow.showSaveFilePicker !== 'function') {
    return null;
  }
  return pickerWindow.showSaveFilePicker({
    suggestedName,
    types: [filePickerType(output)],
  });
}

async function writeBlobToSaveHandle(handle: FileSaveHandle, blob: Blob) {
  const writable = await handle.createWritable();
  await writable.write(blob);
  await writable.close();
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  window.setTimeout(() => {
    if (typeof URL.revokeObjectURL === 'function') {
      URL.revokeObjectURL(url);
    }
  }, 1000);
}

function filePickerType(output: PipelineRunFinalOutput) {
  const extension = output.type === 'markdown' ? '.md' : output.type === 'excel' ? '.xlsx' : `.${output.type || 'txt'}`;
  const mime =
    output.type === 'pdf'
      ? 'application/pdf'
      : output.type === 'excel'
        ? 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
        : output.type === 'json'
          ? 'application/json'
          : output.type === 'html'
            ? 'text/html'
            : output.type === 'markdown'
              ? 'text/markdown'
              : 'text/plain';
  return {
    description: `${formatOutputType(output.type)} output`,
    accept: { [mime]: [extension] },
  };
}

function isAbortError(error: unknown) {
  if (!error || typeof error !== 'object') return false;
  const name = 'name' in error ? String(error.name) : '';
  return name === 'AbortError';
}
