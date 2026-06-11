import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import {
  analyzeUiBoundaries,
  formatBoundaryReport,
  type UiBoundaryFile,
} from '../src/tools/uiBoundaryCheck.js';

const SOURCE_EXTENSIONS = new Set(['.ts', '.tsx']);
const IGNORED_DIRECTORIES = new Set([
  'coverage',
  'dist',
  'dist-test',
  'node_modules',
  'playwright-report',
  'test-results',
]);

async function main(): Promise<void> {
  const root = process.cwd();
  const files = await collectSourceFiles(path.join(root, 'src'), root);
  const report = analyzeUiBoundaries(files);
  const json = process.argv.includes('--json');

  if (json) {
    console.log(JSON.stringify(report, null, 2));
  } else {
    console.log(formatBoundaryReport(report));
  }

  if (report.violations.length > 0) {
    process.exitCode = 1;
  }
}

async function collectSourceFiles(directory: string, root: string): Promise<UiBoundaryFile[]> {
  const entries = await readdir(directory, { withFileTypes: true });
  const files: UiBoundaryFile[] = [];

  for (const entry of entries) {
    if (entry.name.startsWith('.')) continue;
    const fullPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      if (IGNORED_DIRECTORIES.has(entry.name)) continue;
      files.push(...(await collectSourceFiles(fullPath, root)));
      continue;
    }
    if (!entry.isFile() || !SOURCE_EXTENSIONS.has(path.extname(entry.name))) continue;
    files.push({
      path: path.relative(root, fullPath),
      contents: await readFile(fullPath, 'utf8'),
    });
  }

  return files.sort((a, b) => a.path.localeCompare(b.path));
}

await main();
