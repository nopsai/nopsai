import type { DocumentBlock, DocumentSpec } from './finalOutputSpecs';

export function DocumentPreview({ document }: { document: DocumentSpec }) {
  return (
    <article className="mx-auto max-w-4xl bg-white p-5 text-sm leading-6 text-slate-800 dark:bg-slate-950 dark:text-slate-100">
      <header className="mb-6 border-b-2 border-blue-600 pb-4">
        <h4 className="text-2xl font-bold text-slate-950 dark:text-white">{document.title}</h4>
        {document.subtitle && <p className="mt-1 text-base text-slate-600 dark:text-slate-300">{document.subtitle}</p>}
        {Boolean(document.metadata?.length) && (
          <dl className="mt-4 grid gap-2 sm:grid-cols-2">
            {document.metadata?.map((item, index) => (
              <div key={`${item.label}-${index}`} className="border-l-2 border-blue-200 pl-2">
                <dt className="text-[11px] font-bold uppercase text-slate-500">{item.label}</dt>
                <dd className="break-words">{item.value}</dd>
              </div>
            ))}
          </dl>
        )}
      </header>
      <div className="space-y-6">
        {document.sections.map((section, sectionIndex) => (
          <section key={`${section.title}-${sectionIndex}`}>
            <h5 className="mb-3 text-lg font-bold text-slate-900 dark:text-white">{section.title}</h5>
            <div className="space-y-3">
              {section.blocks.map((block, blockIndex) => (
                <DocumentBlockPreview key={`${block.type}-${blockIndex}`} block={block} />
              ))}
            </div>
          </section>
        ))}
      </div>
    </article>
  );
}

function DocumentBlockPreview({ block }: { block: DocumentBlock }) {
  if (block.type === 'paragraph') return <p className="whitespace-pre-line">{block.text}</p>;
  if (block.type === 'bullet_list') {
    return <ul className="list-disc space-y-1 pl-6">{block.items.map((item, index) => <li key={index}>{item}</li>)}</ul>;
  }
  if (block.type === 'numbered_list') {
    return <ol className="list-decimal space-y-1 pl-6">{block.items.map((item, index) => <li key={index}>{item}</li>)}</ol>;
  }
  if (block.type === 'table') {
    return (
      <div className="overflow-x-auto border border-slate-300 dark:border-slate-700">
        <table className="min-w-full border-collapse text-left text-xs">
          <thead className="bg-blue-50 text-slate-900 dark:bg-slate-800 dark:text-white">
            <tr>{block.table.columns.map((column, index) => <th key={index} className="border-b border-slate-300 px-3 py-2 font-bold dark:border-slate-700">{column}</th>)}</tr>
          </thead>
          <tbody>{block.table.rows.map((row, rowIndex) => <tr key={rowIndex} className="even:bg-slate-50 dark:even:bg-slate-900">{row.map((cell, cellIndex) => <td key={cellIndex} className="border-b border-slate-200 px-3 py-2 align-top dark:border-slate-800">{cell}</td>)}</tr>)}</tbody>
        </table>
      </div>
    );
  }
  if (block.type !== 'callout') return null;
  const toneClass = {
    info: 'border-blue-500 bg-blue-50 dark:bg-blue-950/30',
    success: 'border-emerald-500 bg-emerald-50 dark:bg-emerald-950/30',
    warning: 'border-amber-500 bg-amber-50 dark:bg-amber-950/30',
    critical: 'border-red-500 bg-red-50 dark:bg-red-950/30',
  }[block.tone || 'info'];
  return <aside className={`border-l-4 p-3 ${toneClass}`}>{block.title && <div className="font-bold">{block.title}</div>}<div>{block.text}</div></aside>;
}
