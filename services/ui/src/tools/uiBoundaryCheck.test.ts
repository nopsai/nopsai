import assert from 'node:assert/strict';
import test from 'node:test';
import {
  analyzeUiBoundaries,
  formatBoundaryReport,
  normalizeBoundaryPath,
  type UiBoundaryFile,
} from './uiBoundaryCheck.js';

test('flags raw fetch while allowing apiClient transport helpers', () => {
  const files: UiBoundaryFile[] = [
    {
      path: 'src/pages/Unsafe.tsx',
      contents: [
        "import { apiClient } from '../lib/api';",
        'export async function loadViaHelper() {',
        "  return apiClient.fetch('/v1/safe');",
        '}',
        'export async function loadDirectly() {',
        "  return fetch('/v1/unsafe');",
        '}',
      ].join('\n'),
    },
    {
      path: 'src/lib/api.ts',
      contents: 'export const rawAdapter = (path: string) => fetch(path);',
    },
  ];

  const report = analyzeUiBoundaries(files, {
    routeTransportBaseline: { 'src/pages/Unsafe.tsx': 1 },
  });

  assert.deepEqual(
    report.violations.map(violation => violation.kind),
    ['raw-fetch']
  );
  assert.equal(report.violations[0]?.filePath, 'src/pages/Unsafe.tsx');
  assert.equal(report.violations[0]?.line, 6);
  assert.deepEqual(report.routeTransport, [
    {
      filePath: 'src/pages/Unsafe.tsx',
      count: 1,
      baseline: 1,
      lines: [3],
    },
  ]);
});

test('blocks route-local transport growth beyond the baseline', () => {
  const report = analyzeUiBoundaries(
    [
      {
        path: 'src/pages/Lab.tsx',
        contents: [
          "import { apiClient } from '../lib/api';",
          'export async function load() {',
          "  await apiClient.fetch('/v1/a');",
          "  await apiClient.fetch('/v1/b');",
          '}',
        ].join('\n'),
      },
    ],
    {
      routeTransportBaseline: { 'src/pages/Lab.tsx': 1 },
    }
  );

  assert.deepEqual(
    report.violations.map(violation => violation.kind),
    ['route-transport-increase']
  );
  assert.equal(report.violations[0]?.line, 4);
  assert.equal(formatBoundaryReport(report).includes('src/pages/Lab.tsx: 2/1 increased'), true);
});

test('flags TypeScript and hook suppressions in comments but ignores strings', () => {
  const tsIgnore = '@ts-' + 'ignore';
  const eslintSuppression = 'eslint-disable-next-line react-hooks/exhaustive-deps';
  const report = analyzeUiBoundaries([
    {
      path: 'src/features/example/model.ts',
      contents: [
        `const fixture = "// ${tsIgnore}";`,
        `// ${tsIgnore}`,
        `/* ${eslintSuppression} */`,
        'export const value = 1;',
      ].join('\n'),
    },
  ]);

  assert.deepEqual(
    report.violations.map(violation => violation.kind),
    ['typescript-suppression', 'hook-rule-suppression']
  );
  assert.deepEqual(
    report.violations.map(violation => violation.line),
    [2, 3]
  );
});

test('reports large files and browser API usage without making them violations', () => {
  const report = analyzeUiBoundaries(
    [
      {
        path: 'src/features/example/BigPanel.tsx',
        contents: ['export function BigPanel() {', '  window.confirm("Run?");', '}', '', ''].join('\n'),
      },
    ],
    {
      largeFileThresholds: { route: 3, featureShell: 3 },
      routeTransportBaseline: {},
    }
  );

  assert.equal(report.violations.length, 0);
  assert.deepEqual(report.largeFiles, [
    {
      filePath: 'src/features/example/BigPanel.tsx',
      lines: 4,
      threshold: 3,
      category: 'feature-shell',
    },
  ]);
  assert.deepEqual(report.browserApiUsage, [
    {
      filePath: 'src/features/example/BigPanel.tsx',
      api: 'confirm',
      count: 1,
    },
  ]);
});

test('normalizes Windows paths for deterministic CI reports', () => {
  assert.equal(normalizeBoundaryPath('.\\src\\pages\\Lab.tsx'), 'src/pages/Lab.tsx');
});
