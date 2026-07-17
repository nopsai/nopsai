import { useState } from 'react';
import { AlertCircle, CheckCircle2, Clipboard, Download, Eye, FileText, Loader2, Square } from 'lucide-react';
import { apiClient } from '../../lib/api';
import { copyTextToClipboard } from '../../lib/clipboard';
import type { PipelineRunFinalOutput } from './contracts';
import { FinalOutputPreview } from './final-output-preview/FinalOutputPreview';
import { documentSpecToText, parseDocumentSpec, parseSpreadsheetSpec, spreadsheetSpecToText } from './final-output-preview/finalOutputSpecs';

type RunFinalOutputsProps = {
  runID: string;
  outputs?: PipelineRunFinalOutput[];
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

export function RunFinalOutputs({ runID, outputs = [], onCancelOutput }: RunFinalOutputsProps) {
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
    <section className="border border-[var(--border-primary)] rounded-2xl bg-white dark:bg-slate-950 p-4 space-y-3 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="font-semibold text-[var(--text-primary)]">Final Outputs</h3>
        <span className="text-xs text-[var(--text-secondary)]">{outputs.length} deliverable{outputs.length === 1 ? '' : 's'}</span>
      </div>
      <div className="space-y-3">
        {outputs.map(output => {
          const ready = output.status === 'success' && Boolean(output.content);
          const cancellable = output.status === 'pending' || output.status === 'generating';
          const expanded = previewID === output.id;
          return (
            <div key={output.id} className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3 text-sm">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 space-y-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <FinalOutputStatus status={output.status} />
                    <span className="font-semibold text-[var(--text-primary)] break-words">{output.name || 'Untitled output'}</span>
                    <span className="runner-pill runner-pill--muted">{formatOutputType(output.type)}</span>
                    {output.llm_profile && <span className="runner-pill runner-pill--muted">LLM {output.llm_profile}</span>}
                  </div>
                  {output.error && <div className="text-xs text-red-600 dark:text-red-300 break-words">{output.error}</div>}
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <button className={actionClass} type="button" disabled={!ready} onClick={() => setPreviewID(expanded ? null : output.id)}>
                    <Eye className="h-4 w-4" aria-hidden="true" />
                    {expanded ? 'Hide' : 'Preview'}
                  </button>
                  <button className={actionClass} type="button" disabled={!ready || pendingAction === `copy:${output.id}`} onClick={() => void handleCopy(output)}>
                    <Clipboard className="h-4 w-4" aria-hidden="true" />
                    {pendingAction === `copy:${output.id}` ? 'Copying' : 'Copy'}
                  </button>
                  <button className={actionClass} type="button" disabled={output.status !== 'success' || pendingAction === `download:${output.id}`} onClick={() => void handleDownload(output)}>
                    <Download className="h-4 w-4" aria-hidden="true" />
                    {pendingAction === `download:${output.id}` ? 'Downloading' : 'Download'}
                  </button>
                  {onCancelOutput ? (
                    <button className={actionClass} type="button" disabled={!cancellable || pendingAction === `cancel:${output.id}`} onClick={() => void handleCancel(output)}>
                      <Square className="h-4 w-4" aria-hidden="true" />
                      {pendingAction === `cancel:${output.id}` ? 'Cancelling' : 'Cancel'}
                    </button>
                  ) : null}
                </div>
              </div>
              {expanded && ready && (
                <div className="mt-3 rounded-lg border border-[var(--border-primary)] bg-white dark:bg-black/20 p-3">
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
