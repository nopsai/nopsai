import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

/**
 * Every team tree in the product is meant to read the same, whichever page it
 * is on: the explorer rails (schedules, MCP, AI resources) and the collection
 * rails (pipelines, pipeline runs, steps, scopes) are separate components with
 * separate stylesheets, and they drifted apart — the collection rails rendered
 * larger, darker rows with a different active state and a monospace count.
 *
 * The shared scale is the --tree-* token block in styles.css. These tests hold
 * both families to it, so restyling one rail can no longer leave the other
 * behind.
 */

const sourceRoot = resolve(process.cwd(), 'src');
const appStyles = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');
const explorerStyles = readFileSync(resolve(sourceRoot, 'features/system/aiResourcePanel.css'), 'utf8');

/** Merges every rule that names the selector, in source order. */
function declarationsFor(css: string, selector: string) {
  const declarations = new Map<string, string>();
  const withoutComments = css.replace(/\/\*[\s\S]*?\*\//g, '');
  for (const match of withoutComments.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const selectors = match[1].split(',').map(entry => entry.trim());
    if (!selectors.includes(selector)) continue;
    for (const declaration of match[2].split(';')) {
      const [property, ...rest] = declaration.split(':');
      if (!property.trim() || rest.length === 0) continue;
      declarations.set(property.trim(), rest.join(':').trim());
    }
  }
  return declarations;
}

const sharedTokens = [
  '--tree-row-font-size',
  '--tree-leaf-font-size',
  '--tree-count-font-size',
  '--tree-root-min-height',
  '--tree-row-min-height',
  '--tree-row-radius',
  '--tree-row-gap',
  '--tree-row-padding',
  '--tree-icon-color',
  '--tree-accent',
];

test('the tree scale is defined once, as tokens', () => {
  for (const token of sharedTokens) {
    const definitions = appStyles.match(new RegExp(`^\\s*${token}:`, 'gm')) ?? [];
    assert.equal(definitions.length, 1, `${token} should be defined exactly once`);
  }
});

test('collection rail rows and explorer rows share one scale', () => {
  const rail = declarationsFor(appStyles, '.pipeline-runs-scope-select');
  const explorer = declarationsFor(explorerStyles, '.ai-resource-explorer-owner');

  for (const property of ['font-size', 'min-height', 'padding', 'gap', 'border-radius', 'color']) {
    assert.equal(
      rail.get(property),
      explorer.get(property),
      `${property} differs between the collection rail and the explorer rail`,
    );
  }
});

test('collection rail roots and explorer roots share one scale', () => {
  const rail = declarationsFor(appStyles, '.pipeline-runs-scope-item');
  const explorer = declarationsFor(explorerStyles, '.ai-resource-explorer-root');

  for (const property of ['min-height', 'padding', 'gap', 'border-radius', 'color']) {
    assert.equal(rail.get(property), explorer.get(property), `${property} differs on the root row`);
  }
});

test('both rails mark the selected row the same way', () => {
  const rail = declarationsFor(appStyles, '.pipeline-runs-scope-select--active');
  const explorer = declarationsFor(explorerStyles, '.ai-resource-explorer-owner.active');

  assert.equal(rail.get('color'), 'var(--tree-accent)');
  assert.equal(explorer.get('color'), 'var(--ai-panel-accent)');
  assert.ok(rail.get('background')?.includes('accent'), 'the active row keeps a soft accent fill');
  assert.ok(rail.get('border-color')?.includes('accent'), 'the active row keeps an accent border');
});

test('tree rows are not restyled with literal values', () => {
  const rail = declarationsFor(appStyles, '.pipeline-runs-scope-select');
  for (const property of ['font-size', 'min-height', 'gap', 'border-radius']) {
    assert.match(rail.get(property) ?? '', /^var\(--tree-/, `${property} should read the shared token`);
  }
});

test('tree icons use the shared icon colour', () => {
  const railIcon = declarationsFor(appStyles, '.pipeline-runs-scope-select > svg');
  const explorerIcon = declarationsFor(explorerStyles, '.ai-resource-explorer-folder');
  assert.equal(railIcon.get('color'), 'var(--tree-icon-color)');
  assert.equal(explorerIcon.get('color'), 'var(--tree-icon-color)');
});

test('the tree count is not a monospace badge', () => {
  const count = declarationsFor(appStyles, '.resource-collection-tree-count');
  assert.equal(count.get('font-size'), 'var(--tree-count-font-size)');
  assert.equal(count.get('font-family'), undefined);
});
