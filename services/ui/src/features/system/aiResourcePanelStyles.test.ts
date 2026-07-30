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

test('keeps AI resource table typography aligned with pipeline tables', () => {
  const appStyles = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');
  const aiStyles = readFileSync(resolve(sourceRoot, 'features/system/aiResourcePanel.css'), 'utf8');

  const pipelineHeader = cssBlock(appStyles, '.pipeline-runs-table th');
  const pipelineCell = cssBlock(appStyles, '.pipeline-runs-table td');
  const aiHeader = cssBlock(aiStyles, '.ai-resource-registry-table th');
  const aiCell = cssBlock(aiStyles, '.ai-resource-registry-table td');
  const aiMono = cssBlock(aiStyles, '.ai-resource-table-mono');
  const aiName = cssBlock(aiStyles, '.ai-resource-table-resource-name strong');

  assert.equal(declarationValue(aiHeader, 'font-size'), declarationValue(pipelineHeader, 'font-size'));
  assert.equal(declarationValue(aiHeader, 'letter-spacing'), declarationValue(pipelineHeader, 'letter-spacing'));
  assert.equal(declarationValue(aiCell, 'font-size'), declarationValue(pipelineCell, 'font-size'));
  assert.equal(declarationValue(aiMono, 'font-family'), declarationValue(cssBlock(appStyles, '.pipeline-runs-mono'), 'font-family'));
  assert.equal(declarationValue(aiMono, 'font-size'), declarationValue(pipelineCell, 'font-size'));
  assert.equal(declarationValue(aiName, 'font-size'), declarationValue(pipelineCell, 'font-size'));

  const pipelineBadge = cssBlock(appStyles, '.pipeline-runs-status');
  const aiHealth = cssBlock(aiStyles, '.ai-resource-health');
  assert.equal(declarationValue(aiHealth, 'font-size'), declarationValue(pipelineBadge, 'font-size'));
  assert.equal(declarationValue(aiHealth, 'font-weight'), declarationValue(pipelineBadge, 'font-weight'));
});

test('keeps AI resource workspaces free of the extra outer border line', () => {
  const aiStyles = readFileSync(resolve(sourceRoot, 'features/system/aiResourcePanel.css'), 'utf8');
  const workspaceCard = cssBlock(aiStyles, '.ai-resource-workspace-card');
  const darkWorkspaceCard = cssBlock(aiStyles, 'html.dark .ai-resource-workspace-card');

  assert.equal(declarationValue(workspaceCard, 'border'), '0');
  assert.equal(declarationValue(workspaceCard, 'overflow'), 'visible');
  assert.equal(declarationValue(workspaceCard, 'box-shadow'), 'none');
  assert.equal(declarationValue(darkWorkspaceCard, 'box-shadow'), 'none');
});

test('keeps AI resource list actions above tables and aligned to the table edge', () => {
  const aiStyles = readFileSync(resolve(sourceRoot, 'features/system/aiResourcePanel.css'), 'utf8');
  const listActions = cssBlock(aiStyles, '.ai-resource-table-head--list-actions');
  const list = cssBlock(aiStyles, '.ai-resource-browser-list');
  const listTableShell = cssBlock(
    aiStyles,
    '.ai-resource-table-head--list-actions + .ai-resource-browser-list .ai-resource-table-shell'
  );

  assert.equal(declarationValue(listActions, 'position'), 'absolute');
  assert.equal(declarationValue(listActions, 'top'), '-44px');
  assert.equal(declarationValue(listActions, 'right'), '12px');
  assert.equal(declarationValue(listActions, 'justify-content'), 'flex-end');
  assert.equal(declarationValue(listActions, 'padding'), '0');
  assert.equal(declarationValue(list, 'padding'), '0 12px 12px');
  assert.equal(declarationValue(listTableShell, 'padding-right'), '0');
});
