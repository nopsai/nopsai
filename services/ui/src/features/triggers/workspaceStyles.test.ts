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

test('aligns the Triggers table top edge with the tree rail', () => {
  const styles = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');
  const listContainer = cssBlock(styles, '[data-page="triggers"] .triggers-browser-main .triggers-list-container');

  assert.equal(declarationValue(listContainer, 'padding-top'), '0');
});
