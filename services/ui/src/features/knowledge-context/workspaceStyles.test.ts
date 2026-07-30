import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test } from 'node:test';

const sourceRoot = resolve(process.cwd(), 'src');

function cssBlock(source: string, selector: string): string {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = source.match(new RegExp(`${escapedSelector}\\s*\\{([\\s\\S]*?)\\}`));
  assert.ok(match, `expected ${selector} CSS block`);
  return match[1] || '';
}

function declarationValue(block: string, property: string): string {
  const escapedProperty = property.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = block.match(new RegExp(`${escapedProperty}\\s*:\\s*([^;]+);`));
  assert.ok(match, `expected ${property} declaration`);
  return (match[1] || '').trim();
}

test('aligns the Knowledge Context list table with the tree top edge', () => {
  const styles = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');
  const browserMain = cssBlock(styles, '[data-page="knowledge-context"] .kc-demo-browser-main');
  const collection = cssBlock(styles, '[data-page="knowledge-context"] .kc-demo-browser-main .kc-demo-collection');

  assert.equal(declarationValue(browserMain, 'padding-top'), '0');
  assert.equal(declarationValue(browserMain, 'background'), 'var(--bg-primary)');
  assert.equal(declarationValue(collection, 'background'), 'transparent');
});
