import assert from 'node:assert/strict';
import test from 'node:test';

import {
  monitoringTabFromPath,
  monitoringTabFromSearch,
  monitoringTabRoute,
} from './routes.js';

test('builds monitoring tab routes while preserving filters', () => {
  assert.equal(monitoringTabFromPath('/monitoring/ai-usage'), 'ai-usage');
  assert.equal(monitoringTabFromSearch('?tab=reliability'), 'reliability');
  assert.equal(
    monitoringTabRoute('ai-usage', new URLSearchParams('tab=runs&runId=run-1')),
    '/monitoring/ai-usage?runId=run-1'
  );
});
