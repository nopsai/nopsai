import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildRunLogsHash,
  enrichRunLogLines,
  filterRunLogLines,
  formatRunLogDownload,
  getPresentRunLogLevels,
  parseRunLogLine,
  parseRunLogsHash,
} from './runLogs.js';

test('parses structured and plain run log metadata', () => {
  assert.deepEqual(parseRunLogLine('prefix {"level":"warning","step_name":"deploy","message":"done"}'), {
    level: 'warning',
    step: 'deploy',
  });
  assert.deepEqual(parseRunLogLine('ERROR request failed'), { level: 'error' });
  assert.deepEqual(parseRunLogLine('ordinary output'), { level: undefined });
});

test('round-trips run log filters through the legacy hash contract', () => {
  const hash = buildRunLogsHash({
    currentHash: '#/pipelineruns/events/run-1',
    runID: 'run-1',
    selectedSteps: new Set(['build', 'deploy']),
    selectedLevels: new Set(['error', 'info']),
    wrap: true,
    structured: false,
    agentOnly: true,
    shortView: false,
    searchText: 'failed request',
  });

  assert.equal(
    hash,
    '#/pipelineruns/events/run-1/logs/build%2Cdeploy/info%2Cerror/wrap/unstructured/agent/full?search=failed%20request'
  );
  const parsed = parseRunLogsHash(hash || '', 'run-1');
  assert.deepEqual(parsed?.steps, ['build', 'deploy']);
  assert.deepEqual(Array.from(parsed?.levels || []), ['info', 'error']);
  assert.equal(parsed?.agentOnly, true);
  assert.equal(parsed?.search, 'failed request');
  assert.equal(parseRunLogsHash(hash || '', 'another-run'), null);
});

test('enriches, filters, and formats log lines consistently', () => {
  const lines = enrichRunLogLines([
    { id: 1, timestamp: '2026-06-08T12:00:00Z', line: '{"level":"info","step":"build","message":"compiled"}' },
    { id: 2, timestamp: '2026-06-08T12:00:01Z', line: '{"level":"error","step":"agent-review","message":"failed"}' },
  ]);

  assert.deepEqual(Array.from(getPresentRunLogLevels(lines)).sort(), ['agent', 'error', 'info']);
  assert.deepEqual(
    filterRunLogLines(lines, {
      selectedSteps: new Set(['agent-review']),
      selectedLevels: new Set(['error']),
      agentOnly: true,
      searchText: 'failed',
    }).map(line => line.id),
    [2]
  );
  assert.match(formatRunLogDownload([lines[1]]), /\[agent-review\] ERROR -/);
});
