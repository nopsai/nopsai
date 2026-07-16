import { ExternalLink } from 'lucide-react';
import type { DashboardBlock, DashboardChartSeries, DashboardSeriesPoint, DashboardSpec } from '../model';

const toneClass: Record<string, string> = {
  success: 'border-emerald-200 bg-emerald-50 text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-100',
  warning: 'border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-100',
  critical: 'border-rose-200 bg-rose-50 text-rose-900 dark:border-rose-800 dark:bg-rose-950/30 dark:text-rose-100',
  info: 'border-sky-200 bg-sky-50 text-sky-900 dark:border-sky-800 dark:bg-sky-950/30 dark:text-sky-100',
  neutral: 'border-[var(--border-subtle)] bg-[var(--bg-secondary)] text-[var(--text-primary)]',
};

export function DashboardBlocks({ spec }: { spec: DashboardSpec }) {
  const blocks = spec.blocks || [];
  if (blocks.length === 0) {
    return <div className="text-sm text-[var(--text-secondary)]">No blocks published.</div>;
  }
  return (
    <div className="space-y-3">
      {blocks.map((block, index) => (
        <DashboardBlockView key={`${block.type || 'block'}-${index}`} block={block} />
      ))}
    </div>
  );
}

function DashboardBlockView({ block }: { block: DashboardBlock }) {
  switch (block.type) {
    case 'status':
      return <StatusBlock block={block} />;
    case 'text':
      return <TextBlock block={block} />;
    case 'callout':
      return <CalloutBlock block={block} />;
    case 'list':
      return <ListBlock block={block} />;
    case 'properties':
      return <PropertiesBlock block={block} />;
    case 'table':
      return <TableBlock block={block} />;
    case 'progress':
      return <ProgressBlock block={block} />;
    case 'link':
      return <LinkBlock block={block} />;
    case 'chart':
    case 'series':
      return <ChartBlock block={block} />;
    default:
      return <pre className="overflow-auto rounded border border-[var(--border-subtle)] bg-[var(--bg-secondary)] p-3 text-xs">{JSON.stringify(block, null, 2)}</pre>;
  }
}

function StatusBlock({ block }: { block: DashboardBlock }) {
  const tone = block.status || block.tone || 'neutral';
  return (
    <div className={`rounded border p-3 ${toneClass[tone] || toneClass.neutral}`}>
      <div className="text-xs font-semibold uppercase tracking-wide">{block.label || block.title || 'Status'}</div>
      <div className="mt-1 text-2xl font-semibold">{block.value || block.status || block.text}</div>
      {block.text && block.value ? <div className="mt-1 text-sm">{block.text}</div> : null}
    </div>
  );
}

function TextBlock({ block }: { block: DashboardBlock }) {
  return (
    <div>
      {block.title ? <h4 className="text-sm font-semibold text-[var(--text-primary)]">{block.title}</h4> : null}
      <p className="mt-1 whitespace-pre-wrap text-sm leading-6 text-[var(--text-secondary)]">{block.text}</p>
    </div>
  );
}

function CalloutBlock({ block }: { block: DashboardBlock }) {
  const tone = block.tone || 'info';
  return (
    <div className={`rounded border p-3 ${toneClass[tone] || toneClass.info}`}>
      {block.title ? <div className="text-sm font-semibold">{block.title}</div> : null}
      <div className="whitespace-pre-wrap text-sm leading-6">{block.text}</div>
    </div>
  );
}

function ListBlock({ block }: { block: DashboardBlock }) {
  return (
    <div>
      {block.title ? <h4 className="mb-2 text-sm font-semibold">{block.title}</h4> : null}
      <ul className="space-y-2 text-sm text-[var(--text-secondary)]">
        {(block.items || []).map((item, index) => (
          <li key={`${item.label || item.text || index}`} className="flex gap-2">
            <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--accent)]" />
            <span>
              {item.label ? <span className="font-medium text-[var(--text-primary)]">{item.label}: </span> : null}
              {item.href ? <a className="text-[var(--accent)] hover:underline" href={item.href}>{item.text || item.value || item.href}</a> : (item.text || item.value)}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function PropertiesBlock({ block }: { block: DashboardBlock }) {
  return (
    <dl className="grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
      {(block.items || []).map(item => (
        <div key={item.label} className="border-b border-[var(--border-subtle)] pb-2">
          <dt className="text-xs uppercase text-[var(--text-muted)]">{item.label}</dt>
          <dd className="mt-1 font-medium text-[var(--text-primary)]">{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}

function TableBlock({ block }: { block: DashboardBlock }) {
  const columns = block.columns || [];
  return (
    <div className="overflow-auto">
      {block.title ? <h4 className="mb-2 text-sm font-semibold">{block.title}</h4> : null}
      <table className="min-w-full border-collapse text-left text-sm">
        <thead>
          <tr>
            {columns.map(column => (
              <th key={column.key} className="border-b border-[var(--border-subtle)] px-2 py-2 text-xs uppercase text-[var(--text-muted)]">{column.label}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {(block.rows || []).map((row, index) => (
            <tr key={index}>
              {columns.map(column => (
                <td key={column.key} className="border-b border-[var(--border-subtle)] px-2 py-2 text-[var(--text-secondary)]">{formatCell(row[column.key])}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ProgressBlock({ block }: { block: DashboardBlock }) {
  const value = Number(block.progress?.value || 0);
  const max = Number(block.progress?.max || 100);
  const percent = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  return (
    <div>
      <div className="flex items-center justify-between text-sm">
        <span className="font-medium">{block.title || block.label || 'Progress'}</span>
        <span className="text-[var(--text-secondary)]">{value}{block.progress?.unit || ''} / {max}{block.progress?.unit || ''}</span>
      </div>
      <div className="mt-2 h-2 overflow-hidden rounded bg-[var(--bg-tertiary)]">
        <div className="h-full bg-[var(--accent)]" style={{ width: `${percent}%` }} />
      </div>
    </div>
  );
}

function LinkBlock({ block }: { block: DashboardBlock }) {
  return (
    <a className="inline-flex items-center gap-2 text-sm font-medium text-[var(--accent)] hover:underline" href={block.href}>
      {block.text || block.title || block.label || block.href}
      <ExternalLink className="h-4 w-4" aria-hidden="true" />
    </a>
  );
}

function ChartBlock({ block }: { block: DashboardBlock }) {
  const chart = block.chart;
  const series = chart?.series || [];
  if (!chart || series.length === 0) {
    return <div className="text-sm text-[var(--text-secondary)]">No chart data.</div>;
  }
  const chartType = chart.type || 'line';
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          {block.title ? <h4 className="text-sm font-semibold text-[var(--text-primary)]">{block.title}</h4> : null}
          <div className="text-xs text-[var(--text-muted)]">
            {[chartType, chart.aggregation_interval, chart.time_window?.from && chart.time_window?.to ? `${chart.time_window.from} to ${chart.time_window.to}` : '']
              .filter(Boolean)
              .join(' · ')}
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {series.slice(0, 6).map((item, index) => (
            <span key={item.key || index} className="inline-flex items-center gap-1 text-xs text-[var(--text-secondary)]">
              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: seriesColor(item, index) }} />
              {item.label || item.key}
            </span>
          ))}
        </div>
      </div>
      {chartType === 'pie' || chartType === 'donut' ? (
        <PieSummary series={series} />
      ) : (
        <CartesianChart type={chartType} series={series} unit={chart.unit} />
      )}
    </div>
  );
}

function CartesianChart({ type, series, unit }: { type: string; series: DashboardChartSeries[]; unit?: string }) {
  const width = 640;
  const height = 220;
  const padding = 28;
  const points = series.flatMap(item => (item.points || []).filter(point => typeof point.value === 'number')) as Required<Pick<DashboardSeriesPoint, 'value'>>[];
  const values = points.map(point => Number(point.value));
  const minValue = Math.min(0, ...values);
  const maxValue = Math.max(1, ...values);
  const valueY = (value: number) => {
    const span = maxValue - minValue || 1;
    return height - padding - ((value - minValue) / span) * (height - padding * 2);
  };
  const pointX = (index: number, total: number) => {
    if (total <= 1) return padding;
    return padding + (index / (total - 1)) * (width - padding * 2);
  };
  return (
    <div className="overflow-hidden rounded border border-[var(--border-subtle)] bg-[var(--bg-secondary)] p-2">
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label="Dashboard chart" className="h-56 w-full">
        <line x1={padding} y1={height - padding} x2={width - padding} y2={height - padding} stroke="currentColor" className="text-[var(--border-subtle)]" />
        <line x1={padding} y1={padding} x2={padding} y2={height - padding} stroke="currentColor" className="text-[var(--border-subtle)]" />
        {type === 'bar' ? renderBars(series, width, height, padding, valueY) : renderLines(series, type, height, padding, pointX, valueY)}
        <text x={padding} y={18} className="fill-[var(--text-muted)] text-[11px]">{maxValue}{unit || ''}</text>
        <text x={padding} y={height - 6} className="fill-[var(--text-muted)] text-[11px]">{minValue}{unit || ''}</text>
      </svg>
    </div>
  );
}

function renderLines(
  series: DashboardChartSeries[],
  type: string,
  height: number,
  padding: number,
  pointX: (index: number, total: number) => number,
  valueY: (value: number) => number
) {
  return series.map((item, seriesIndex) => {
    const points = (item.points || []).filter(point => typeof point.value === 'number');
    const path = points
      .map((point, index) => `${index === 0 ? 'M' : 'L'} ${pointX(index, points.length).toFixed(1)} ${valueY(Number(point.value)).toFixed(1)}`)
      .join(' ');
    const areaPath = type === 'area' && points.length > 0
      ? `${path} L ${pointX(points.length - 1, points.length).toFixed(1)} ${height - padding} L ${padding} ${height - padding} Z`
      : '';
    const color = seriesColor(item, seriesIndex);
    return (
      <g key={item.key || seriesIndex}>
        {areaPath ? <path d={areaPath} fill={color} opacity="0.14" /> : null}
        <path d={path} fill="none" stroke={color} strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
        {points.map((point, index) => (
          <circle key={`${point.timestamp || point.label || index}`} cx={pointX(index, points.length)} cy={valueY(Number(point.value))} r="2.8" fill={color} />
        ))}
      </g>
    );
  });
}

function renderBars(
  series: DashboardChartSeries[],
  width: number,
  height: number,
  padding: number,
  valueY: (value: number) => number
) {
  const primary = series[0];
  const points = (primary?.points || []).filter(point => typeof point.value === 'number');
  const gap = 8;
  const barWidth = Math.max(8, ((width - padding * 2) / Math.max(1, points.length)) - gap);
  const color = seriesColor(primary, 0);
  return points.map((point, index) => {
    const value = Number(point.value);
    const x = padding + index * (barWidth + gap);
    const y = valueY(value);
    return <rect key={`${point.timestamp || point.label || index}`} x={x} y={y} width={barWidth} height={height - padding - y} rx="3" fill={color} />;
  });
}

function PieSummary({ series }: { series: DashboardChartSeries[] }) {
  const points = series.flatMap(item => (item.points || []).map(point => ({ ...point, series: item }))).filter(point => typeof point.value === 'number');
  const total = points.reduce((sum, point) => sum + Math.max(0, Number(point.value)), 0) || 1;
  return (
    <div className="space-y-2">
      {points.map((point, index) => {
        const value = Math.max(0, Number(point.value));
        const percent = Math.round((value / total) * 100);
        return (
          <div key={`${point.series.key}-${point.label || index}`}>
            <div className="flex items-center justify-between gap-3 text-sm">
              <span className="inline-flex min-w-0 items-center gap-2">
                <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: seriesColor(point.series, index) }} />
                <span className="truncate">{point.label || point.series.label || point.series.key}</span>
              </span>
              <span className="shrink-0 text-[var(--text-secondary)]">{formatCell(point.value)} · {percent}%</span>
            </div>
            <div className="mt-1 h-2 overflow-hidden rounded bg-[var(--bg-tertiary)]">
              <div className="h-full" style={{ width: `${percent}%`, backgroundColor: seriesColor(point.series, index) }} />
            </div>
          </div>
        );
      })}
    </div>
  );
}

function seriesColor(series: DashboardChartSeries | undefined, index: number): string {
  const color = series?.color || '';
  if (/^#[0-9a-f]{6}([0-9a-f]{2})?$/i.test(color)) return color;
  return ['#2563eb', '#059669', '#d97706', '#dc2626', '#7c3aed', '#0891b2'][index % 6];
}

function formatCell(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value);
  return JSON.stringify(value);
}
