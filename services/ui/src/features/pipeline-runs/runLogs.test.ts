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
  assert.deepEqual(parseRunLogLine('prefix {"level":"warning","step_name":"deploy","task":"publish","message":"done"}'), {
    level: 'warn',
    step: 'deploy',
    task: 'publish',
  });
  assert.deepEqual(parseRunLogLine('ERROR request failed'), { level: 'error' });
  assert.deepEqual(parseRunLogLine('ordinary output'), { level: undefined });
  assert.deepEqual(parseRunLogLine('{"output_level":"trace","step":"test","task_name":"unit","message":"verbose"}'), {
    level: 'debug',
    step: 'test',
    task: 'unit',
  });
});

test('round-trips run log filters through the legacy hash contract', () => {
  const hash = buildRunLogsHash({
    currentHash: '#/pipelineruns/events/run-1',
    runID: 'run-1',
    selectedSteps: new Set(['build', 'deploy']),
    selectedTasks: new Set(['compile']),
    selectedLevels: new Set(['error', 'info']),
    wrap: true,
    structured: false,
    agentOnly: true,
    shortView: false,
    searchText: 'failed request',
  });

  assert.equal(
    hash,
    '#/pipelineruns/events/run-1/logs/build%2Cdeploy/info%2Cerror/wrap/unstructured/agent/full?search=failed%20request&task=compile'
  );
  const parsed = parseRunLogsHash(hash || '', 'run-1');
  assert.deepEqual(parsed?.steps, ['build', 'deploy']);
  assert.deepEqual(parsed?.tasks, ['compile']);
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

test('preserves API log metadata when line parsing cannot provide it', () => {
  const lines = enrichRunLogLines([
    {
      id: 3,
      timestamp: '2026-06-08T12:00:02Z',
      line: 'plain stderr output',
      level: 'error',
      step_name: 'release',
      task_name: 'publish',
    },
  ]);

  assert.equal(lines[0].level, 'error');
  assert.equal(lines[0].step, 'release');
  assert.equal(lines[0].task, 'publish');
  assert.deepEqual(
    filterRunLogLines(lines, {
      selectedSteps: new Set(['release']),
      selectedTasks: new Set(['publish']),
      selectedLevels: new Set(['error']),
      agentOnly: false,
      searchText: 'stderr',
    }).map(line => line.id),
    [3]
  );
});

test('matches child pipeline logs through their parent include step', () => {
  const lines = enrichRunLogLines([
    {
      id: 4,
      timestamp: '2026-06-08T12:00:03Z',
      run_id: 'child-run',
      pipeline_name: 'child-pipeline',
      parent_run_id: 'parent-run',
      parent_step_name: 'included',
      line: '{"level":"info","step":"child-build","message":"child compiled"}',
    },
  ]);

  assert.equal(lines[0].parentStep, 'included');
  assert.equal(lines[0].pipelineName, 'child-pipeline');
  assert.deepEqual(
    filterRunLogLines(lines, {
      selectedSteps: new Set(['included']),
      selectedLevels: new Set(),
      agentOnly: false,
      searchText: 'compiled',
    }).map(line => line.id),
    [4]
  );
  assert.match(formatRunLogDownload(lines), /\[child-pipeline\] \[parent:included\] \[child-build\]/);
});
