export type UiBoundaryFile = {
  path: string;
  contents: string;
};

export type UiBoundaryViolationKind =
  | 'raw-fetch'
  | 'typescript-suppression'
  | 'hook-rule-suppression'
  | 'route-transport-increase';

export type UiBoundaryViolation = {
  kind: UiBoundaryViolationKind;
  filePath: string;
  line: number;
  message: string;
};

export type RouteTransportSummary = {
  filePath: string;
  count: number;
  baseline: number;
  lines: number[];
};

export type LargeFileSummary = {
  filePath: string;
  lines: number;
  threshold: number;
  category: 'route' | 'feature-shell';
};

export type BrowserApiUsageSummary = {
  filePath: string;
  api: string;
  count: number;
};

export type UiBoundaryReport = {
  violations: UiBoundaryViolation[];
  routeTransport: RouteTransportSummary[];
  largeFiles: LargeFileSummary[];
  browserApiUsage: BrowserApiUsageSummary[];
};

export type UiBoundaryCheckOptions = {
  allowedRawFetchFiles?: readonly string[];
  largeFileThresholds?: {
    route: number;
    featureShell: number;
  };
  routeTransportBaseline?: Record<string, number>;
};

export const DEFAULT_ROUTE_TRANSPORT_BASELINE: Record<string, number> = {
  'src/pages/ExternalTriggers.tsx': 1,
  'src/pages/Lab.tsx': 8,
  'src/pages/Login.tsx': 1,
  'src/pages/Monitoring.tsx': 1,
};

export const DEFAULT_LARGE_FILE_THRESHOLDS = {
  route: 900,
  featureShell: 900,
} as const;

const RAW_FETCH_PATTERN = /(?<![A-Za-z0-9_$.])fetch\s*\(|(?<![A-Za-z0-9_$])(?:window|globalThis)\s*\.\s*fetch\s*\(/g;
const API_CLIENT_FETCH_PATTERN = /(?<![A-Za-z0-9_$])apiClient\s*\.\s*fetch\s*\(/g;
const TYPESCRIPT_SUPPRESSION_PATTERN = /@ts-(?:ignore|nocheck|expect-error)\b/g;
const HOOK_RULE_SUPPRESSION_PATTERN =
  /eslint-disable(?:-next-line|-line)?[^\n]*(?:react-hooks|react-refresh)(?:\/[A-Za-z-]+)?/g;
const WINDOW_API_PATTERN = /(?<![A-Za-z0-9_$])window\s*\.\s*([A-Za-z_$][A-Za-z0-9_$]*)/g;

export function analyzeUiBoundaries(
  files: readonly UiBoundaryFile[],
  options: UiBoundaryCheckOptions = {}
): UiBoundaryReport {
  const allowedRawFetchFiles = new Set((options.allowedRawFetchFiles || ['src/lib/api.ts']).map(normalizeBoundaryPath));
  const largeFileThresholds = options.largeFileThresholds || DEFAULT_LARGE_FILE_THRESHOLDS;
  const routeTransportBaseline = options.routeTransportBaseline || DEFAULT_ROUTE_TRANSPORT_BASELINE;

  const violations: UiBoundaryViolation[] = [];
  const largeFiles: LargeFileSummary[] = [];
  const routeTransportByPath = new Map<string, RouteTransportSummary>();
  const browserApiUsageByKey = new Map<string, BrowserApiUsageSummary>();

  for (const file of files) {
    const filePath = normalizeBoundaryPath(file.path);
    const { code, comments } = partitionSource(file.contents);

    for (const match of collectMatches(comments, TYPESCRIPT_SUPPRESSION_PATTERN)) {
      violations.push({
        kind: 'typescript-suppression',
        filePath,
        line: match.line,
        message: 'TypeScript suppression comments are not allowed; model the type boundary explicitly.',
      });
    }

    for (const match of collectMatches(comments, HOOK_RULE_SUPPRESSION_PATTERN)) {
      violations.push({
        kind: 'hook-rule-suppression',
        filePath,
        line: match.line,
        message: 'React Hooks and Fast Refresh rule suppressions are not allowed in UI code.',
      });
    }

    if (!isRuntimeSource(filePath)) continue;

    if (!allowedRawFetchFiles.has(filePath)) {
      for (const match of collectMatches(code, RAW_FETCH_PATTERN)) {
        violations.push({
          kind: 'raw-fetch',
          filePath,
          line: match.line,
          message: 'Use apiClient or a feature-owned api.ts helper instead of raw fetch.',
        });
      }
    }

    if (isRouteFile(filePath)) {
      const matches = collectMatches(code, API_CLIENT_FETCH_PATTERN);
      if (matches.length > 0 || routeTransportBaseline[filePath]) {
        routeTransportByPath.set(filePath, {
          filePath,
          count: matches.length,
          baseline: routeTransportBaseline[filePath] || 0,
          lines: matches.map(match => match.line),
        });
      }
      if (matches.length > (routeTransportBaseline[filePath] || 0)) {
        const excess = matches[(routeTransportBaseline[filePath] || 0)] || matches[matches.length - 1];
        violations.push({
          kind: 'route-transport-increase',
          filePath,
          line: excess.line,
          message: 'Route-local apiClient.fetch calls must move to a feature api.ts helper before merge.',
        });
      }
    }

    const largeFile = summarizeLargeFile(filePath, file.contents, largeFileThresholds);
    if (largeFile) largeFiles.push(largeFile);

    if (isPageOrFeatureFile(filePath)) {
      for (const match of collectMatches(code, WINDOW_API_PATTERN)) {
        const api = match.groups[0] || 'unknown';
        const key = `${filePath}:${api}`;
        const current = browserApiUsageByKey.get(key);
        if (current) current.count += 1;
        else browserApiUsageByKey.set(key, { filePath, api, count: 1 });
      }
    }
  }

  return {
    violations: violations.sort(compareViolation),
    routeTransport: [...routeTransportByPath.values()].sort(compareFilePath),
    largeFiles: largeFiles.sort((a, b) => b.lines - a.lines || a.filePath.localeCompare(b.filePath)),
    browserApiUsage: [...browserApiUsageByKey.values()].sort(
      (a, b) => b.count - a.count || a.filePath.localeCompare(b.filePath) || a.api.localeCompare(b.api)
    ),
  };
}

export function formatBoundaryReport(report: UiBoundaryReport): string {
  const routeTransportTotal = report.routeTransport.reduce((total, item) => total + item.count, 0);
  const routeTransportBaseline = report.routeTransport.reduce((total, item) => total + item.baseline, 0);
  const browserApiTotal = report.browserApiUsage.reduce((total, item) => total + item.count, 0);
  const browserApiFileCount = new Set(report.browserApiUsage.map(item => item.filePath)).size;
  const largeRouteFiles = report.largeFiles.filter(item => item.category === 'route');
  const largeFeatureShells = report.largeFiles.filter(item => item.category === 'feature-shell');

  const lines = [
    'UI boundary report',
    `- Boundary violations: ${report.violations.length}`,
    `- Route-local apiClient.fetch calls: ${routeTransportTotal} current / ${routeTransportBaseline} baseline`,
    `- Large route files over threshold: ${largeRouteFiles.length}`,
    `- Large feature shell files over threshold: ${largeFeatureShells.length}`,
    `- Browser window API usage: ${browserApiTotal} occurrence(s) across ${browserApiFileCount} runtime file(s), report-only`,
  ];

  if (report.violations.length > 0) {
    lines.push('', 'Violations:');
    for (const violation of report.violations) {
      lines.push(`- ${violation.filePath}:${violation.line} [${violation.kind}] ${violation.message}`);
    }
  }

  if (report.routeTransport.length > 0) {
    lines.push('', 'Route transport baseline:');
    for (const item of report.routeTransport) {
      const suffix = item.count > item.baseline ? ' increased' : '';
      lines.push(`- ${item.filePath}: ${item.count}/${item.baseline}${suffix}`);
    }
  }

  if (report.largeFiles.length > 0) {
    lines.push('', 'Large file report:');
    for (const item of report.largeFiles) {
      lines.push(`- ${item.filePath}: ${item.lines} lines (${item.category}, threshold ${item.threshold})`);
    }
  }

  if (report.browserApiUsage.length > 0) {
    lines.push('', 'Top browser API usage:');
    for (const item of report.browserApiUsage.slice(0, 12)) {
      lines.push(`- ${item.filePath}: window.${item.api} x${item.count}`);
    }
  }

  return lines.join('\n');
}

export function normalizeBoundaryPath(filePath: string): string {
  return filePath.replace(/\\/g, '/').replace(/^\.\//, '');
}

function summarizeLargeFile(
  filePath: string,
  contents: string,
  thresholds: NonNullable<UiBoundaryCheckOptions['largeFileThresholds']>
): LargeFileSummary | null {
  const lines = countLines(contents);
  if (isRouteFile(filePath) && lines > thresholds.route) {
    return { filePath, lines, threshold: thresholds.route, category: 'route' };
  }
  if (isFeatureShellFile(filePath) && lines > thresholds.featureShell) {
    return { filePath, lines, threshold: thresholds.featureShell, category: 'feature-shell' };
  }
  return null;
}

function isRuntimeSource(filePath: string): boolean {
  return (
    /\.(?:ts|tsx)$/.test(filePath) &&
    !/\.d\.ts$/.test(filePath) &&
    !/(?:\.test|\.component\.test)\.(?:ts|tsx)$/.test(filePath) &&
    !filePath.startsWith('src/test/')
  );
}

function isRouteFile(filePath: string): boolean {
  return /^src\/pages\/[^/]+\.tsx$/.test(filePath) && !filePath.endsWith('.component.test.tsx');
}

function isFeatureShellFile(filePath: string): boolean {
  return /^src\/features\/.+\.tsx$/.test(filePath) && !filePath.endsWith('.component.test.tsx');
}

function isPageOrFeatureFile(filePath: string): boolean {
  return filePath.startsWith('src/pages/') || filePath.startsWith('src/features/');
}

function countLines(contents: string): number {
  if (contents.length === 0) return 0;
  const normalized = contents.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
  const lines = normalized.split('\n').length;
  return normalized.endsWith('\n') ? lines - 1 : lines;
}

function collectMatches(contents: string, pattern: RegExp): Array<{ line: number; groups: string[] }> {
  const matches: Array<{ line: number; groups: string[] }> = [];
  const lines = contents.split(/\r\n|\r|\n/);
  for (let lineIndex = 0; lineIndex < lines.length; lineIndex += 1) {
    const line = lines[lineIndex];
    pattern.lastIndex = 0;
    let match = pattern.exec(line);
    while (match) {
      matches.push({ line: lineIndex + 1, groups: match.slice(1).filter(Boolean) });
      if (match[0].length === 0) pattern.lastIndex += 1;
      match = pattern.exec(line);
    }
  }
  return matches;
}

function partitionSource(contents: string): { code: string; comments: string } {
  let code = '';
  let comments = '';
  let index = 0;
  let state: 'code' | 'line-comment' | 'block-comment' | 'single-quote' | 'double-quote' | 'template' = 'code';

  while (index < contents.length) {
    const current = contents[index];
    const next = contents[index + 1];

    if (state === 'code') {
      if (current === '/' && next === '/') {
        code += '  ';
        comments += '//';
        index += 2;
        state = 'line-comment';
        continue;
      }
      if (current === '/' && next === '*') {
        code += '  ';
        comments += '/*';
        index += 2;
        state = 'block-comment';
        continue;
      }
      if (current === "'") {
        code += ' ';
        comments += maskForComment(current);
        index += 1;
        state = 'single-quote';
        continue;
      }
      if (current === '"') {
        code += ' ';
        comments += maskForComment(current);
        index += 1;
        state = 'double-quote';
        continue;
      }
      if (current === '`') {
        code += ' ';
        comments += maskForComment(current);
        index += 1;
        state = 'template';
        continue;
      }
      code += current;
      comments += maskForComment(current);
      index += 1;
      continue;
    }

    if (state === 'line-comment') {
      code += maskForCode(current);
      comments += current;
      index += 1;
      if (current === '\n' || current === '\r') state = 'code';
      continue;
    }

    if (state === 'block-comment') {
      if (current === '*' && next === '/') {
        code += '  ';
        comments += '*/';
        index += 2;
        state = 'code';
        continue;
      }
      code += maskForCode(current);
      comments += current;
      index += 1;
      continue;
    }

    const quote = state === 'single-quote' ? "'" : state === 'double-quote' ? '"' : '`';
    code += maskForCode(current);
    comments += maskForComment(current);
    index += 1;
    if (current === '\\' && index < contents.length) {
      code += maskForCode(contents[index]);
      comments += maskForComment(contents[index]);
      index += 1;
      continue;
    }
    if (current === quote) state = 'code';
  }

  return { code, comments };
}

function maskForCode(value: string): string {
  return value === '\n' || value === '\r' ? value : ' ';
}

function maskForComment(value: string): string {
  return value === '\n' || value === '\r' ? value : ' ';
}

function compareViolation(a: UiBoundaryViolation, b: UiBoundaryViolation): number {
  return a.filePath.localeCompare(b.filePath) || a.line - b.line || a.kind.localeCompare(b.kind);
}

function compareFilePath(a: { filePath: string }, b: { filePath: string }): number {
  return a.filePath.localeCompare(b.filePath);
}
