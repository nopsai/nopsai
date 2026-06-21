import { useState } from 'react';
import type { SpreadsheetSpec } from './finalOutputSpecs';
import { formatCell } from './finalOutputSpecs';

const maxPreviewRows = 200;

export function SpreadsheetPreview({ spreadsheet }: { spreadsheet: SpreadsheetSpec }) {
  const [activeSheet, setActiveSheet] = useState(0);
  const sheet = spreadsheet.sheets[activeSheet] || spreadsheet.sheets[0];
  if (!sheet) return <PreviewError message="The spreadsheet does not contain a sheet." />;
  return (
    <div className="space-y-3">
      {spreadsheet.title && <h4 className="font-semibold text-[var(--text-primary)]">{spreadsheet.title}</h4>}
      {spreadsheet.sheets.length > 1 && (
        <div className="flex gap-1 overflow-x-auto border-b border-[var(--border-primary)]" role="tablist" aria-label="Workbook sheets">
          {spreadsheet.sheets.map((item, index) => (
            <button key={`${item.name}-${index}`} type="button" role="tab" aria-selected={index === activeSheet} className={`px-3 py-2 text-xs font-semibold ${index === activeSheet ? 'border-b-2 border-blue-600 text-blue-600' : 'text-[var(--text-secondary)]'}`} onClick={() => setActiveSheet(index)}>{item.name}</button>
          ))}
        </div>
      )}
      <div className="max-h-[28rem] overflow-auto border border-[var(--border-primary)]">
        <table className="min-w-full border-collapse text-left text-xs">
          <thead className="sticky top-0 z-10 bg-blue-50 text-slate-900 dark:bg-slate-800 dark:text-white">
            <tr>{sheet.columns.map(column => <th key={column.key} className="whitespace-nowrap border-b border-r border-[var(--border-primary)] px-3 py-2 font-bold">{column.header}</th>)}</tr>
          </thead>
          <tbody>{sheet.rows.slice(0, maxPreviewRows).map((row, rowIndex) => <tr key={rowIndex} className="even:bg-slate-50 dark:even:bg-slate-900">{sheet.columns.map(column => <td key={column.key} className="max-w-80 border-b border-r border-[var(--border-primary)] px-3 py-2 align-top whitespace-pre-wrap break-words">{formatCell(row[column.key])}</td>)}</tr>)}</tbody>
        </table>
      </div>
      {sheet.rows.length > maxPreviewRows && <p className="text-xs text-[var(--text-secondary)]">Showing the first {maxPreviewRows} of {sheet.rows.length} rows. Download the workbook for the complete data.</p>}
    </div>
  );
}

export function PreviewError({ message }: { message: string }) {
  return <div role="alert" className="border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200">{message}</div>;
}
