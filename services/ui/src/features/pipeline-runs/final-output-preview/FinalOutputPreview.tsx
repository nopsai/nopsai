import { DashboardBlocks } from '../../dashboards/blocks/DashboardBlocks';
import { normalizeDashboardSpec, type DashboardSpec } from '../../dashboards/model';
import type { PipelineRunFinalOutput } from '../contracts';
import { DocumentPreview } from './DocumentPreview';
import { MarkdownPreview } from './MarkdownPreview';
import { PDFPreview } from './PDFPreview';
import { PreviewError } from './SpreadsheetPreview';
import { SpreadsheetPreview } from './SpreadsheetPreview';
import { parseDocumentSpec, parseSpreadsheetSpec } from './finalOutputSpecs';

export function FinalOutputPreview({ runID, output }: { runID: string; output: PipelineRunFinalOutput }) {
  if (output.type === 'pdf') return <PDFPreview runID={runID} output={output} />;
  if (output.type === 'markdown') return <MarkdownPreview content={output.content || ''} />;
  if (output.type === 'dashboard') {
    const spec = parseDashboardSpec(output.content || '');
    return spec ? <DashboardOutputPreview spec={spec} /> : <PreviewError message="This dashboard output cannot be previewed. Download it to inspect the generated specification." />;
  }
  if (output.type === 'excel') {
    const spec = parseSpreadsheetSpec(output.content || '');
    return spec ? <SpreadsheetPreview spreadsheet={spec} /> : <PreviewError message="This legacy spreadsheet cannot be previewed. Download it to view the workbook." />;
  }
  if (output.type === 'html') {
    const spec = parseDocumentSpec(output.content || '');
    return spec ? <DocumentPreview document={spec} /> : <iframe title={`${output.name} preview`} sandbox="" srcDoc={output.content} className="h-[32rem] w-full bg-white" />;
  }
  if (output.type === 'json') {
    let formatted = output.content || '';
    try { formatted = JSON.stringify(JSON.parse(formatted), null, 2); } catch { /* Keep the server response visible if it predates strict validation. */ }
    return <pre className="max-h-[32rem] overflow-auto whitespace-pre-wrap break-words text-xs leading-5 text-[var(--text-primary)]">{formatted}</pre>;
  }
  return <pre className="max-h-[32rem] overflow-auto whitespace-pre-wrap break-words text-xs leading-5 text-[var(--text-primary)]">{output.content}</pre>;
}

function DashboardOutputPreview({ spec }: { spec: DashboardSpec }) {
  return (
    <div className="space-y-3">
      {spec.title ? <h4 className="text-base font-semibold text-[var(--text-primary)]">{spec.title}</h4> : null}
      <DashboardBlocks spec={spec} />
    </div>
  );
}

function parseDashboardSpec(content: string): DashboardSpec | null {
  try {
    return normalizeDashboardSpec(JSON.parse(content));
  } catch {
    return null;
  }
}
