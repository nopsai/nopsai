import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test } from 'node:test';

/*
 * The modal shell is the one skin every create/edit dialog wears, so these tests
 * guard the two ways it drifts: a feature stylesheet repainting the three blocks
 * with its own surface, and a dialog sizing itself with a width utility instead
 * of the shell's size classes. Both are how the product ended up with dialogs
 * that looked different on every page.
 */

const sourceRoot = resolve(process.cwd(), 'src');
const shell = readFileSync(resolve(sourceRoot, 'components/modalShell.css'), 'utf8');
const styles = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');

function cssBlock(source: string, selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = source.match(new RegExp(`(^|\\n)${escaped}\\s*\\{([\\s\\S]*?)\\}`));
  assert.ok(match, `expected ${selector} CSS block`);
  return match[2] || '';
}

test('the shell paints the three dialog blocks and the card paints nothing', () => {
  const card = cssBlock(shell, '.workflow-dialog-shell .pipelines-modal-card');
  const header = cssBlock(shell, '.workflow-dialog-shell .pipelines-modal-header');
  const body = cssBlock(shell, '.workflow-dialog-shell .pipelines-modal-body');
  const footer = cssBlock(shell, '.workflow-dialog-shell .pipelines-modal-footer');

  assert.match(card, /background:\s*none;/);
  assert.match(card, /box-shadow:\s*none;/);
  assert.match(card, /border:\s*0;/);

  for (const block of [header, body]) {
    assert.match(block, /background:\s*var\(--modal-surface\);/);
    assert.match(block, /border:\s*1px solid var\(--modal-border\);/);
  }

  assert.match(header, /border-radius:\s*var\(--modal-pill-radius\);/);
  assert.match(body, /border-radius:\s*var\(--modal-radius\);/);
  assert.match(footer, /background:\s*none;/);
  assert.match(footer, /border:\s*0;/);
});

test('the shell reads its colours from theme tokens so both themes hold up', () => {
  const light = cssBlock(shell, '.workflow-dialog-shell');
  const dark = cssBlock(shell, 'html.dark .workflow-dialog-shell');

  for (const token of ['--modal-surface', '--modal-shadow', '--modal-blob-opacity']) {
    assert.ok(light.includes(`${token}:`), `expected ${token} in the light palette`);
    assert.ok(dark.includes(`${token}:`), `expected ${token} to be restated for dark`);
  }
  assert.match(light, /--modal-surface:\s*color-mix\(in srgb, var\(--bg-panel\)/);
});

test('dialog width comes from the shell size classes, not a feature repaint', () => {
  const card = cssBlock(shell, '.workflow-dialog-shell .pipelines-modal-card');
  assert.match(card, /max-width:\s*var\(--modal-max-width, 640px\);/);

  for (const size of ['.workflow-dialog--compact', '.kc-document-modal']) {
    const source = size === '.kc-document-modal' ? styles : shell;
    assert.match(cssBlock(source, size), /--modal-max-width:/);
  }
});

test('no feature stylesheet repaints the shared dialog blocks', () => {
  const repaints = [
    '.pipelines-modal-card {',
    '.pipelines-modal-header {',
    '.pipelines-modal-body {',
    '.pipelines-modal-footer {',
  ];

  for (const repaint of repaints) {
    assert.ok(!styles.includes(repaint), `styles.css must leave ${repaint.trim()} to the modal shell`);
  }
});
