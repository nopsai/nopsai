import assert from 'node:assert/strict';
import test from 'node:test';
import { parseRunSidebarTimestamp, runSidebarActivityTimestamp, timeAgoShort } from './runSidebarUtils.js';

test('sidebar run timestamps ignore Go zero time values', t => {
  const originalNow = Date.now;
  Date.now = () => Date.parse('2026-07-12T12:05:00Z');
  t.after(() => {
    Date.now = originalNow;
  });

  assert.equal(parseRunSidebarTimestamp('0001-01-01T00:00:00Z'), null);
  assert.equal(timeAgoShort('0001-01-01T00:00:00Z'), '—');
  assert.equal(timeAgoShort('2026-07-12T12:00:00Z'), '5m ago');
  assert.equal(
    runSidebarActivityTimestamp({
      run_id: 'failed-before-start',
      pipeline_name: 'release',
      status: 'failure',
      is_complete: true,
      started_at: '0001-01-01T00:00:00Z',
      finished_at: '2026-07-12T12:00:00Z',
    }),
    '2026-07-12T12:00:00Z'
  );
});
