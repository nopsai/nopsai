import type { DashboardBlock, DashboardChartSeries, DashboardSeriesPoint, DashboardSpec } from '../model';

const numericValuePattern = /[-+]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][-+]?\d+)?/;

export function dashboardSpecNeedsWideLayout(spec: DashboardSpec): boolean {
  const blocks = spec.blocks || [];
  const chartBlocks = blocks.filter(isChartBlock);
  if (chartBlocks.length >= 3 && isOverviewChartGroup(chartBlocks)) return true;
  if (blocks.some(block => block.type === 'table' && (block.columns || []).length >= 6)) return true;
  if (blocks.some(block => block.type === 'properties' && (block.items || []).length >= 4)) return true;
  return /operations?\s+(overview|digest)|release\s+readiness|image\s+comparison/i.test(spec.title || '');
}

export function isChartBlock(block: DashboardBlock): boolean {
  return block.type === 'chart' || block.type === 'series';
}

export function isOverviewChartGroup(blocks: DashboardBlock[]): boolean {
  return blocks.some(isHorizontalBarBlock) && blocks.filter(isCircularChartBlock).length >= 2;
}

export function shouldRenderHorizontalBars(series: DashboardChartSeries[]): boolean {
  if (series.length !== 1) return false;
  const points = series[0]?.points || [];
  return points.length > 0 && points.every(point => Boolean(point.label) && pointValue(point) !== null);
}

export function pointValue(point: DashboardSeriesPoint): number | null {
  return numericValue(point.value);
}

function isHorizontalBarBlock(block: DashboardBlock): boolean {
  const chart = block.chart;
  return Boolean(chart && chart.type === 'bar' && shouldRenderHorizontalBars(chart.series || []));
}

function isCircularChartBlock(block: DashboardBlock): boolean {
  const chartType = block.chart?.type;
  return chartType === 'pie' || chartType === 'donut';
}

function numericValue(value: unknown): number | null {
  if (typeof value === 'number') {
    return Number.isFinite(value) ? value : null;
  }
  if (typeof value !== 'string') return null;
  const match = value.trim().match(numericValuePattern);
  if (!match) return null;
  const parsed = Number(match[0]);
  return Number.isFinite(parsed) ? parsed : null;
}
