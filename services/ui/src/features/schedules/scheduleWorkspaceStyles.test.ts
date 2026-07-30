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

test('aligns the schedule list table with the tree top edge', () => {
  const styles = readFileSync(resolve(sourceRoot, 'features/schedules/scheduleWorkspace.css'), 'utf8');
  const list = cssBlock(styles, '.schedule-workspace .ai-resource-browser-list');
  const headerActions = cssBlock(styles, '.schedule-workspace .schedule-workspace__header-actions');

  assert.equal(declarationValue(list, 'padding-top'), '0');
  assert.equal(declarationValue(headerActions, 'margin-right'), '12px');
});
