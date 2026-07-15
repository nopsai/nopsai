import type { ReactNode } from 'react';

export function MCPDetailSection({
  title,
  rows,
}: {
  title: string;
  rows: Array<{ label: string; value: ReactNode; mono?: boolean; full?: boolean }>;
}) {
  return (
    <section className="ai-resource-detail-section">
      <h4>{title}</h4>
      <dl>
        {rows.map(row => (
          <div key={row.label} className={`ai-resource-detail-row ${row.full ? 'ai-resource-detail-row--full' : ''}`}>
            {!row.full && <dt>{row.label}</dt>}
            <dd className={row.mono ? 'ai-resource-detail-row__mono' : undefined}>{row.value}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
