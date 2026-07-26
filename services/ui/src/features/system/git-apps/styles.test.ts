import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';

test('keeps GitHub App action icons visible outside shared ghost button styles', () => {
  const styles = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8');
  assert.match(styles, /\.github-app-action\s*\{/);
  assert.match(styles, /\.github-app-action svg\s*\{[\s\S]*stroke: currentColor;[\s\S]*opacity: 1;/);
  assert.match(styles, /\.github-app-action--verify\s*\{[\s\S]*color: #047857;/);
  assert.match(styles, /html\.dark \.github-app-action--sync\s*\{[\s\S]*color: #7dd3fc;/);
});
