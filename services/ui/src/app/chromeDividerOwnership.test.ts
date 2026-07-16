import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test } from 'node:test';

const sourceRoot = resolve(process.cwd(), 'src');

function lineContaining(source: string, marker: string): string {
  const line = source.split('\n').find(item => item.includes(marker));
  assert.ok(line, `expected source to contain ${marker}`);
  return line;
}

test('keeps app chrome free of persistent structural divider utilities', () => {
  const shellSource = readFileSync(resolve(sourceRoot, 'app/AppShell.tsx'), 'utf8');
  const stylesSource = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');

  assert.doesNotMatch(lineContaining(shellSource, 'app-sidebar-shell'), /\bborder-r\b/, 'sidebar shell should not render a permanent right divider');
  assert.doesNotMatch(lineContaining(shellSource, 'app-sidebar-brand-row'), /\bborder-b\b/, 'sidebar brand row should not render a permanent bottom divider');
  assert.doesNotMatch(lineContaining(shellSource, 'app-header-shell'), /\bborder-b\b/, 'top header should not render a permanent bottom divider');
  assert.match(shellSource, /app-sidebar-resizer/, 'sidebar resize hit area should use the hover-only resizer styling');

  assert.match(stylesSource, /\.app-header-shell[\s\S]*?box-shadow:\s*none/, 'header shadow must stay disabled');
  assert.match(stylesSource, /\.app-sidebar-resizer::before[\s\S]*?background:\s*transparent !important/, 'sidebar resizer handle should be invisible until interaction');
  assert.match(stylesSource, /\.tree-column-resizer::before[\s\S]*?opacity:\s*0/, 'tree resizers should not draw permanent rail lines');
});
