import assert from 'node:assert/strict';
import test from 'node:test';
import { formatAIResourceRatio, formatFilteredCount, matchesAIResourceSearch } from './aiResourcePresentation.js';

test('formats filtered counts only when search is active', () => {
  assert.equal(formatFilteredCount(3, 8, ''), 8);
  assert.equal(formatFilteredCount(3, 8, 'github'), '3 / 8');
});

test('formats AI resource ratios without empty denominator noise', () => {
  assert.equal(formatAIResourceRatio(3, 5), '3/5');
  assert.equal(formatAIResourceRatio(0, 0), '0');
});

test('matches AI resource search values across strings, numbers, and booleans', () => {
  assert.equal(matchesAIResourceSearch('github', 'GitHub MCP', 'streamable_http'), true);
  assert.equal(matchesAIResourceSearch('41', 'GitHub MCP', 41), true);
  assert.equal(matchesAIResourceSearch('disabled', false, 'enabled'), false);
});
