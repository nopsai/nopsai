import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test } from 'node:test';

const sourceRoot = resolve(process.cwd(), 'src');

function cssBlock(source: string, selector: string): string {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = source.match(new RegExp(`(?:^|\\n)${escapedSelector}\\s*\\{([\\s\\S]*?)\\}`));
  assert.ok(match, `expected ${selector} CSS block`);
  return match[1] || '';
}

function declarationValue(block: string, property: string): string {
  const escapedProperty = property.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = block.match(new RegExp(`${escapedProperty}\\s*:\\s*([^;]+);`));
  assert.ok(match, `expected ${property} declaration`);
  return (match[1] || '').trim();
}

function cssBlockWithDeclaration(source: string, selector: string, property: string): string {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const escapedProperty = property.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const matches = source.matchAll(new RegExp(`${escapedSelector}\\s*\\{([\\s\\S]*?)\\}`, 'g'));
  for (const match of matches) {
    const block = match[1] || '';
    if (new RegExp(`${escapedProperty}\\s*:`).test(block)) return block;
  }
  assert.fail(`expected ${selector} CSS block with ${property} declaration`);
}

test('keeps trigger table typography aligned with pipeline tables', () => {
  const styles = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');
  const pipelineHeader = cssBlock(styles, '.pipeline-runs-table th');
  const pipelineCell = cssBlock(styles, '.pipeline-runs-table td');
  const triggerHeader = cssBlock(styles, '.triggers-resource-table th');
  const triggerCell = cssBlock(styles, '.triggers-resource-table td');
  const triggerMono = cssBlock(styles, '.triggers-mono');
  const triggerName = cssBlock(styles, '.triggers-resource-name strong,\n.triggers-team-row-body strong');

  assert.equal(declarationValue(triggerHeader, 'font-size'), declarationValue(pipelineHeader, 'font-size'));
  assert.equal(declarationValue(triggerHeader, 'letter-spacing'), declarationValue(pipelineHeader, 'letter-spacing'));
  assert.equal(declarationValue(triggerCell, 'font-size'), declarationValue(pipelineCell, 'font-size'));
  assert.equal(declarationValue(triggerMono, 'font-family'), declarationValue(cssBlock(styles, '.pipeline-runs-mono'), 'font-family'));
  assert.equal(declarationValue(triggerMono, 'font-size'), declarationValue(pipelineCell, 'font-size'));
  assert.equal(declarationValue(triggerName, 'font-size'), declarationValue(pipelineCell, 'font-size'));
});

test('keeps trigger tree background and create action aligned with schedule workspaces', () => {
  const styles = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');
  const explorer = cssBlock(styles, '.triggers-explorer');
  const toolbarActions = cssBlockWithDeclaration(styles, '.triggers-page-toolbar-actions', 'margin-right');

  assert.equal(declarationValue(explorer, 'background'), 'var(--bg-secondary)');
  assert.equal(declarationValue(toolbarActions, 'margin-right'), '12px');
});
