import { describe, expect, test } from 'vitest';
import { isRunnerRecentlyDisconnected } from './presentation.js';

describe('isRunnerRecentlyDisconnected', () => {
  test('returns true for a recent dispatcher disconnect', () => {
    const nowMs = Date.parse('2026-08-04T06:30:00Z');
    expect(isRunnerRecentlyDisconnected(nowMs, '2026-08-04T06:25:42Z')).toBe(true);
  });

  test('returns false for old, missing, invalid, or future timestamps', () => {
    const nowMs = Date.parse('2026-08-04T06:30:00Z');
    expect(isRunnerRecentlyDisconnected(nowMs, '2026-08-04T05:30:00Z')).toBe(false);
    expect(isRunnerRecentlyDisconnected(nowMs, '')).toBe(false);
    expect(isRunnerRecentlyDisconnected(nowMs, 'not-a-date')).toBe(false);
    expect(isRunnerRecentlyDisconnected(nowMs, '2026-08-04T06:31:00Z')).toBe(false);
  });
});
