import { ExternalLink } from 'lucide-react';
import type { DashboardBlock, DashboardChartSeries, DashboardSeriesPoint, DashboardSpec } from '../model';
import { isChartBlock, isOverviewChartGroup, pointValue, shouldRenderHorizontalBars } from './DashboardBlocksLayout';

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
  const groups = groupDashboardBlocks(blocks);
  return (
    <div className="space-y-3">
      {groups.map((group, index) => (
        Array.isArray(group) ? (
          <ChartGroup key={`chart-group-${index}`} blocks={group} />
        ) : (
          <DashboardBlockView key={`${group.type || 'block'}-${index}`} block={group} />
        )
      ))}
    </div>
  );
}

function groupDashboardBlocks(blocks: DashboardBlock[]): Array<DashboardBlock | DashboardBlock[]> {
  const groups: Array<DashboardBlock | DashboardBlock[]> = [];
  let index = 0;
  while (index < blocks.length) {
    const block = blocks[index];
    if (!isChartBlock(block)) {
      groups.push(block);
      index++;
      continue;
    }
    const chartGroup: DashboardBlock[] = [];
    while (index < blocks.length && isChartBlock(blocks[index])) {
      chartGroup.push(blocks[index]);
      index++;
    }
    groups.push(chartGroup.length > 1 ? chartGroup : chartGroup[0]);
  }
  return groups;
}

function ChartGroup({ blocks }: { blocks: DashboardBlock[] }) {
  if (isOverviewChartGroup(blocks)) {
    const bar = blocks.find(block => isHorizontalBarBlock(block));
    const circular = blocks.filter(block => isCircularChartBlock(block));
    const rest = blocks.filter(block => block !== bar && !circular.includes(block));
    if (bar && circular.length >= 2 && rest.length === 0) {
      return (
        <div
          data-testid="dashboard-overview-chart-grid"
          className="grid items-stretch gap-3 xl:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)]"
        >
          <DashboardBlockView block={bar} />
          <div className="grid min-w-0 items-stretch gap-3 md:grid-cols-2">
            {circular.map((block, index) => (
              <DashboardBlockView key={`${block.chart?.type || 'chart'}-${block.title || index}`} block={block} />
            ))}
          </div>
        </div>
      );
    }
  }
  return (
    <div className="grid gap-3 xl:grid-cols-2">
      {blocks.map((block, blockIndex) => (
        <DashboardBlockView key={`${block.type || 'block'}-${blockIndex}`} block={block} />
      ))}
    </div>
  );
}

function isHorizontalBarBlock(block: DashboardBlock): boolean {
  const chart = block.chart;
  return Boolean(chart && chart.type === 'bar' && shouldRenderHorizontalBars(chart.series || []));
}

function isCircularChartBlock(block: DashboardBlock): boolean {
  const chartType = block.chart?.type;
  return chartType === 'pie' || chartType === 'donut';
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
    <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2 xl:grid-cols-4">
      {(block.items || []).map(item => (
        <div key={item.label} className="rounded border border-[var(--border-subtle)] bg-[var(--bg-secondary)] p-3">
          <dt className="text-xs uppercase text-[var(--text-muted)]">{item.label}</dt>
          <dd className={`mt-1 font-semibold leading-tight text-[var(--text-primary)] ${metricValueClass(item.value)}`}>{item.value}</dd>
          {item.text ? <dd className="mt-1 text-xs text-[var(--text-secondary)]">{item.text}</dd> : null}
        </div>
      ))}
    </dl>
  );
}

function metricValueClass(value: unknown): string {
  const length = formatCell(value).length;
  if (length > 20) return 'break-words text-lg';
  if (length > 12) return 'break-words text-xl';
  return 'text-2xl';
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
                <td key={column.key} className="border-b border-[var(--border-subtle)] px-2 py-2 text-[var(--text-secondary)]">{renderCell(row[column.key], column)}</td>
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
  const circular = chartType === 'pie' || chartType === 'donut';
  return (
    <div className={circular ? 'flex h-full min-w-0 flex-col gap-3' : 'space-y-3'}>
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          {block.title ? <h4 className="text-sm font-semibold text-[var(--text-primary)]">{block.title}</h4> : null}
          {block.text ? <div className="text-sm text-[var(--text-secondary)]">{block.text}</div> : null}
          <ChartMetadata chart={chart} />
        </div>
        <ChartSummary chartType={chartType} series={series} unit={chart.unit} />
      </div>
      {circular ? (
        <CircularChart type={chartType} series={series} unit={chart.unit} />
      ) : (
        <CartesianChart type={chartType} series={series} unit={chart.unit} />
      )}
    </div>
  );
}

function ChartMetadata({ chart }: { chart: NonNullable<DashboardBlock['chart']> }) {
  const metadata = [
    chart.aggregation_interval,
    chart.time_window?.from && chart.time_window?.to ? `${chart.time_window.from} to ${chart.time_window.to}` : '',
  ].filter(Boolean);
  if (metadata.length === 0) return null;
  return <div className="text-xs text-[var(--text-muted)]">{metadata.join(' · ')}</div>;
}

function ChartSummary({ chartType, series, unit }: { chartType: string; series: DashboardChartSeries[]; unit?: string }) {
  if (chartType === 'pie' || chartType === 'donut') return null;
  const pill = chartSummaryPill(chartType, series, unit);
  if (pill) {
    return (
      <span className="rounded-full bg-[var(--accent-soft)] px-2 py-1 text-xs font-semibold text-[var(--text-primary)]">
        {pill}
      </span>
    );
  }
  if (chartType === 'bar' && series.length === 1) return null;
  return (
    <div className="flex flex-wrap gap-2">
      {series.slice(0, 6).map((item, index) => (
        <span key={item.key || index} className="inline-flex items-center gap-1 text-xs text-[var(--text-secondary)]">
          <span className="h-2 w-2 rounded-full" style={{ backgroundColor: seriesColor(item, index) }} />
          {item.label || item.key}
        </span>
      ))}
    </div>
  );
}

function chartSummaryPill(chartType: string, series: DashboardChartSeries[], unit?: string): string | null {
  if (chartType !== 'bar' || !shouldRenderHorizontalBars(series)) return null;
  const primary = series[0];
  const points = (primary.points || [])
    .map(point => pointValue(point))
    .filter((value): value is number => value !== null);
  if (points.length === 0) return null;
  const maxValue = Math.max(...points);
  const label = /build|duration|seconds?/i.test(`${primary.key} ${primary.label || ''} ${unit || ''}`) ? 'Slowest' : 'Max';
  return `${label}: ${formatChartValue(maxValue, primary.unit || unit)}`;
}

function CartesianChart({ type, series, unit }: { type: string; series: DashboardChartSeries[]; unit?: string }) {
  if (type === 'bar' && shouldRenderHorizontalBars(series)) {
    return <HorizontalBarChart series={series[0]} unit={series[0]?.unit || unit} />;
  }
  const width = 640;
  const height = 220;
  const padding = 28;
  const values = series
    .flatMap(item => (item.points || []).map(point => pointValue(point)))
    .filter((value): value is number => value !== null);
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

function HorizontalBarChart({ series, unit }: { series: DashboardChartSeries; unit?: string }) {
  const points = (series.points || [])
    .map(point => ({ point, value: pointValue(point) }))
    .filter((item): item is { point: DashboardSeriesPoint; value: number } => item.value !== null);
  if (points.length === 0) {
    return <div className="text-sm text-[var(--text-secondary)]">No chart data.</div>;
  }
  const maxValue = Math.max(1, ...points.map(item => Math.max(0, item.value)));
  return (
    <div role="img" aria-label="Dashboard bar chart" className="space-y-3 rounded border border-[var(--border-subtle)] bg-[var(--bg-secondary)] p-3">
      {points.map(({ point, value }) => {
        const width = Math.max(2, Math.min(100, (Math.max(0, value) / maxValue) * 100));
        const label = point.label || point.timestamp || 'Value';
        return (
          <div key={label} className="space-y-1.5">
            <div className="flex items-center justify-between gap-3 text-sm">
              <span className="min-w-0 truncate font-medium text-[var(--text-primary)]">{label}</span>
              <span className="shrink-0 font-semibold text-[var(--text-primary)]">{formatChartValue(value, unit)}</span>
            </div>
            <div className="h-2 overflow-hidden rounded bg-[var(--bg-tertiary)]">
              <div className="h-full rounded bg-[var(--accent)]" style={{ width: `${width}%` }} />
            </div>
          </div>
        );
      })}
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
    const points = (item.points || [])
      .map(point => ({ point, value: pointValue(point) }))
      .filter((item): item is { point: DashboardSeriesPoint; value: number } => item.value !== null);
    const path = points
      .map((item, index) => `${index === 0 ? 'M' : 'L'} ${pointX(index, points.length).toFixed(1)} ${valueY(item.value).toFixed(1)}`)
      .join(' ');
    const areaPath = type === 'area' && points.length > 0
      ? `${path} L ${pointX(points.length - 1, points.length).toFixed(1)} ${height - padding} L ${padding} ${height - padding} Z`
      : '';
    const color = seriesColor(item, seriesIndex);
    return (
      <g key={item.key || seriesIndex}>
        {areaPath ? <path d={areaPath} fill={color} opacity="0.14" /> : null}
        <path d={path} fill="none" stroke={color} strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
        {points.map(({ point, value }, index) => (
          <circle key={`${point.timestamp || point.label || index}`} cx={pointX(index, points.length)} cy={valueY(value)} r="2.8" fill={color} />
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
  const points = (primary?.points || [])
    .map(point => ({ point, value: pointValue(point) }))
    .filter((item): item is { point: DashboardSeriesPoint; value: number } => item.value !== null);
  const gap = 8;
  const barWidth = Math.max(8, ((width - padding * 2) / Math.max(1, points.length)) - gap);
  const color = seriesColor(primary, 0);
  return points.map(({ point, value }, index) => {
    const x = padding + index * (barWidth + gap);
    const y = valueY(value);
    const centerX = x + barWidth / 2;
    const label = point.label || point.timestamp || '';
    return (
      <g key={`${point.timestamp || point.label || index}`}>
        <rect x={x} y={y} width={barWidth} height={height - padding - y} rx="3" fill={color} />
        <text x={centerX} y={Math.max(14, y - 6)} textAnchor="middle" className="fill-[var(--text-muted)] text-[10px]">{formatChartValue(value)}</text>
        {label ? <text x={centerX} y={height - 8} textAnchor="middle" className="fill-[var(--text-muted)] text-[10px]">{shortChartLabel(label)}</text> : null}
      </g>
    );
  });
}

function CircularChart({ type, series, unit }: { type: string; series: DashboardChartSeries[]; unit?: string }) {
  const points = series.flatMap((item, seriesIndex) => (item.points || [])
    .map((point, pointIndex) => ({ point, value: pointValue(point), series: item, color: circularPointColor(item, seriesIndex, pointIndex) }))
    .filter((item): item is { point: DashboardSeriesPoint; value: number; series: DashboardChartSeries; color: string } => item.value !== null));
  const total = points.reduce((sum, point) => sum + Math.max(0, point.value), 0);
  if (total <= 0) {
    return <div className="text-sm text-[var(--text-secondary)]">No chart data.</div>;
  }
  const size = 168;
  const center = size / 2;
  const centerSummary = circularCenterSummary(points, total, unit);
  const radius = centerSummary || type === 'donut' ? 54 : 36;
  const strokeWidth = centerSummary || type === 'donut' ? 22 : 72;
  const circumference = Math.PI * 2 * radius;
  const slices = points.reduce(
    (acc, point, index) => {
      const value = Math.max(0, point.value);
      const length = (value / total) * circumference;
      return {
        offset: acc.offset + length,
        items: [...acc.items, { point, index, length, dashOffset: -acc.offset }],
      };
    },
    { offset: 0, items: [] as Array<{ point: (typeof points)[number]; index: number; length: number; dashOffset: number }> }
  ).items;
  return (
    <div className="dashboard-circular-chart">
      <div className="dashboard-circular-chart__body">
        <svg viewBox={`0 0 ${size} ${size}`} role="img" aria-label={`${type} dashboard chart`} className="dashboard-circular-chart__svg">
          <circle cx={center} cy={center} r={radius} fill="none" stroke="currentColor" strokeWidth={strokeWidth} className="text-[var(--bg-tertiary)]" />
          {slices.map(slice => (
            <circle
              key={`${slice.point.series.key}-${slice.point.point.label || slice.index}`}
              cx={center}
              cy={center}
              r={radius}
              fill="none"
              stroke={slice.point.color}
              strokeWidth={strokeWidth}
              strokeDasharray={`${slice.length} ${Math.max(0, circumference - slice.length)}`}
              strokeDashoffset={slice.dashOffset}
              transform={`rotate(-90 ${center} ${center})`}
            />
          ))}
          <circle cx={center} cy={center} r={centerSummary || type === 'donut' ? 39 : 0} fill="var(--bg-secondary)" />
          {centerSummary || type === 'donut' ? (
            <>
              <text x={center} y={center - 2} textAnchor="middle" className="fill-[var(--text-primary)] text-lg font-semibold">{centerSummary?.value || formatChartValue(total, unit)}</text>
              <text x={center} y={center + 16} textAnchor="middle" className="fill-[var(--text-muted)] text-[11px]">{centerSummary?.label || 'total'}</text>
            </>
          ) : null}
        </svg>
        <div className="dashboard-circular-chart__legend">
          {points.map((point, index) => {
            const value = Math.max(0, point.value);
            const percent = Math.round((value / total) * 100);
            const label = point.point.label || point.series.label || point.series.key;
            return (
              <div key={`${point.series.key}-${point.point.label || index}`} className="dashboard-circular-chart__legend-row">
                <span className="dashboard-circular-chart__legend-label">
                  <span className="dashboard-circular-chart__legend-dot" style={{ backgroundColor: point.color }} />
                  <span className="dashboard-circular-chart__legend-name">{label}</span>
                </span>
                <span className="dashboard-circular-chart__legend-value">{formatChartValue(value, unit)} · {percent}%</span>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

function circularCenterSummary(
  points: Array<{ point: DashboardSeriesPoint; value: number; series: DashboardChartSeries; color: string }>,
  total: number,
  unit?: string
): { value: string; label: string } | null {
  const positive = points.find(item => circularPositivePointKind(item.point.label || item.series.label || item.series.key));
  if (!positive) return null;
  const kind = circularPositivePointKind(positive.point.label || positive.series.label || positive.series.key);
  const value = unit ? `${formatChartValue(positive.value, unit)}/${formatChartValue(total, unit)}` : `${formatChartValue(positive.value)}/${formatChartValue(total)}`;
  return {
    value,
    label: kind === 'configuration' ? 'configured' : 'ready',
  };
}

function circularPositivePointKind(label: string | undefined): 'readiness' | 'configuration' | null {
  const normalized = (label || '').toLowerCase();
  if (/(configuration|configured|runtime).*present|present.*(configuration|configured|runtime)/.test(normalized)) {
    return 'configuration';
  }
  if (/production ready|ready/.test(normalized) && !/not ready|blocked|missing/.test(normalized)) {
    return 'readiness';
  }
  return null;
}

function seriesColor(series: DashboardChartSeries | undefined, index: number): string {
  const color = series?.color || '';
  if (/^#[0-9a-f]{6}([0-9a-f]{2})?$/i.test(color)) return color;
  return ['#2563eb', '#059669', '#d97706', '#dc2626', '#7c3aed', '#0891b2'][index % 6];
}

function circularPointColor(series: DashboardChartSeries | undefined, seriesIndex: number, pointIndex: number): string {
  const point = series?.points?.[pointIndex];
  const label = `${point?.label || ''} ${series?.label || ''} ${series?.key || ''}`.toLowerCase();
  if (/missing|vulnerab|failed|failure|blocked/.test(label)) return '#94a3b8';
  if (/ready|present|configured|success|healthy/.test(label)) return '#2563eb';
  const hasMultiplePoints = (series?.points || []).length > 1;
  if (!hasMultiplePoints) return seriesColor(series, seriesIndex);
  return seriesColor(undefined, pointIndex);
}

function shortChartLabel(label: string): string {
  const trimmed = label.trim();
  if (trimmed.length <= 16) return trimmed;
  return `${trimmed.slice(0, 13)}...`;
}

function formatCell(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value);
  return JSON.stringify(value);
}

function renderCell(value: unknown, column: { key: string; label: string }) {
  const booleanState = booleanCellState(value, column);
  if (booleanState) {
    return (
      <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-semibold ${booleanState.className}`}>
        {booleanState.label}
      </span>
    );
  }
  return formatCell(value);
}

function booleanCellState(value: unknown, column: { key: string; label: string }) {
  const raw = formatCell(value).trim();
  const normalized = raw.toLowerCase();
  const trueValues = new Set(['true', 'yes', 'pass', 'passed', 'ready', 'enabled', 'healthy', 'success']);
  const falseValues = new Set(['false', 'no', 'fail', 'failed', 'blocked', 'disabled', 'unhealthy']);
  let boolValue: boolean | null = null;
  if (typeof value === 'boolean') {
    boolValue = value;
  } else if (trueValues.has(normalized)) {
    boolValue = true;
  } else if (falseValues.has(normalized)) {
    boolValue = false;
  }
  if (boolValue === null) return null;
  const columnName = `${column.key} ${column.label}`.toLowerCase();
  const riskColumn = /vulnerab|risk|error|fail|missing|blocked|critical|down|unhealthy/.test(columnName);
  const positive = riskColumn ? !boolValue : boolValue;
  return {
    label: raw || String(boolValue),
    className: positive
      ? 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-100'
      : 'border-rose-200 bg-rose-50 text-rose-800 dark:border-rose-800 dark:bg-rose-950/30 dark:text-rose-100',
  };
}

function formatChartValue(value: number, unit?: string): string {
  const rounded = Number.isInteger(value) ? String(value) : value.toFixed(2).replace(/\.?0+$/, '');
  return `${rounded}${unit || ''}`;
}
