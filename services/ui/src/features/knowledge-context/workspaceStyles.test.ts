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

function cssBlockWithDeclaration(source: string, selector: string, property: string): string {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const matches = Array.from(source.matchAll(new RegExp(`${escapedSelector}\\s*\\{([\\s\\S]*?)\\}`, 'g')));
  assert.ok(matches.length, `expected ${selector} CSS block`);
  const block = matches.map(match => match[1] || '').reverse().find(candidate => candidate.includes(`${property}:`));
  assert.ok(block, `expected ${selector} CSS block with ${property}`);
  return block;
}

function declarationValue(block: string, property: string): string {
  const escapedProperty = property.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = block.match(new RegExp(`${escapedProperty}\\s*:\\s*([^;]+);`));
  assert.ok(match, `expected ${property} declaration`);
  return (match[1] || '').trim();
}

test('keeps the Knowledge Context browser unframed and aligned', () => {
  const styles = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');
  const workspace = cssBlockWithDeclaration(styles, '[data-page="knowledge-context"] .kc-demo-workspace', 'min-height');
  const tree = cssBlock(styles, '[data-page="knowledge-context"] .kc-demo-workspace > .triggers-explorer');
  const browserMain = cssBlock(styles, '[data-page="knowledge-context"] .kc-demo-browser-main');
  const collection = cssBlock(styles, '[data-page="knowledge-context"] .kc-demo-browser-main .kc-demo-collection');

  assert.equal(declarationValue(workspace, 'min-height'), '560px');
  assert.equal(declarationValue(workspace, 'border'), '0');
  assert.equal(declarationValue(workspace, 'background'), 'transparent');
  assert.equal(declarationValue(workspace, 'overflow'), 'visible');
  assert.equal(declarationValue(workspace, 'box-shadow'), 'none');
  assert.equal(declarationValue(tree, 'max-height'), 'calc(100vh - 180px)');
  assert.equal(declarationValue(tree, 'border-color'), 'transparent');
  assert.equal(declarationValue(browserMain, 'padding-top'), '0');
  assert.equal(declarationValue(browserMain, 'background'), 'var(--bg-primary)');
  assert.equal(declarationValue(collection, 'border'), '0');
  assert.equal(declarationValue(collection, 'background'), 'transparent');
});
