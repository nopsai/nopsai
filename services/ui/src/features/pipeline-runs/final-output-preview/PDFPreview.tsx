import { useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { apiClient } from '../../../lib/api';
import type { PipelineRunFinalOutput } from '../contracts';
import { PreviewError } from './SpreadsheetPreview';

export function PDFPreview({ runID, output }: { runID: string; output: PipelineRunFinalOutput }) {
  const [pdfURL, setPDFURL] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    let objectURL = '';
    void (async () => {
      try {
        const response = await apiClient.fetch(`/v1/runs/${encodeURIComponent(runID)}/outputs/${encodeURIComponent(output.id)}/download`, { cache: 'no-store', signal: controller.signal });
        if (!response.ok) throw new Error((await response.text()) || `Preview failed (${response.status})`);
        objectURL = URL.createObjectURL(await response.blob());
        setPDFURL(objectURL);
      } catch (reason) {
        if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : 'PDF preview failed');
      }
    })();
    return () => {
      controller.abort();
      if (objectURL && typeof URL.revokeObjectURL === 'function') URL.revokeObjectURL(objectURL);
    };
  }, [output.id, runID]);
  if (error) return <PreviewError message={error} />;
  if (!pdfURL) return <div className="flex min-h-48 items-center justify-center gap-2 text-sm text-[var(--text-secondary)]"><Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />Rendering PDF preview</div>;
  return <iframe title={`${output.name} PDF preview`} src={pdfURL} className="h-[36rem] w-full bg-white" />;
}
