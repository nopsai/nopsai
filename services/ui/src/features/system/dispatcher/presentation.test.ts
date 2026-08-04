import { strictEqual } from 'node:assert';
import { describe, test } from 'node:test';
import { isRunnerRecentlyDisconnected } from './presentation.js';

describe('isRunnerRecentlyDisconnected', () => {
  test('returns true for a recent dispatcher disconnect', () => {
    const nowMs = Date.parse('2026-08-04T06:30:00Z');
    strictEqual(isRunnerRecentlyDisconnected(nowMs, '2026-08-04T06:25:42Z'), true);
  });

  test('returns false for old, missing, invalid, or future timestamps', () => {
    const nowMs = Date.parse('2026-08-04T06:30:00Z');
    strictEqual(isRunnerRecentlyDisconnected(nowMs, '2026-08-04T05:30:00Z'), false);
    strictEqual(isRunnerRecentlyDisconnected(nowMs, ''), false);
    strictEqual(isRunnerRecentlyDisconnected(nowMs, 'not-a-date'), false);
    strictEqual(isRunnerRecentlyDisconnected(nowMs, '2026-08-04T06:31:00Z'), false);
  });
});
