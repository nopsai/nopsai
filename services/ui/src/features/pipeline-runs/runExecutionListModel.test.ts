import assert from 'node:assert/strict';
import test from 'node:test';
import type { GraphStep, GraphTask } from './contracts.js';
import { buildRunExecutionLines, buildStepExecutionGroups, buildTaskExecutionGroups } from './runExecutionListModel.js';

test('groups steps by dependency rank while preserving authored order inside parallel groups', () => {
  const steps: GraphStep[] = [
    { id: 'checkout', name: 'checkout', status: 'success', dependsOn: [], tasks: [] },
    { id: 'unit', name: 'unit', status: 'success', dependsOn: ['checkout'], tasks: [] },
    { id: 'lint', name: 'lint', status: 'success', dependsOn: ['checkout'], tasks: [] },
    { id: 'deploy', name: 'deploy', status: 'pending', dependsOn: ['unit', 'lint'], tasks: [] },
  ];

  const groups = buildStepExecutionGroups(steps);

  assert.deepEqual(groups.map(group => group.rows.map(row => row.entity.id)), [
    ['checkout'],
    ['unit', 'lint'],
    ['deploy'],
  ]);
  assert.equal(groups[1]?.parallel, true);
  assert.equal(groups[1]?.dependencyLabel, 'After checkout');
  assert.equal(groups[2]?.rows[0]?.dependencyLabel, 'After unit, lint');
});

test('marks missing dependencies without breaking task grouping', () => {
  const tasks: GraphTask[] = [
    { id: 'publish', name: 'publish', status: 'pending', dependsOn: ['missing-build'] },
  ];

  const groups = buildTaskExecutionGroups(tasks);

  assert.equal(groups.length, 1);
  assert.deepEqual(groups[0]?.rows[0]?.missingDependsOn, ['missing-build']);
  assert.equal(groups[0]?.rows[0]?.dependencyLabel, 'After missing-build');
});

test('assigns dependency ranks to ordered task groups', () => {
  const tasks: GraphTask[] = [
    { id: 'clone', name: 'clone', status: 'success', dependsOn: [] },
    { id: 'unit', name: 'unit', status: 'success', dependsOn: ['clone'] },
    { id: 'lint', name: 'lint', status: 'success', dependsOn: ['clone'] },
  ];

  const groups = buildTaskExecutionGroups(tasks);

  assert.deepEqual(groups.map(group => group.level), [0, 1]);
  assert.equal(groups[1]?.parallel, true);
  assert.deepEqual(groups[1]?.rows.map(row => row.entity.id), ['unit', 'lint']);
});

test('flattens steps and tasks into execution log lines in dependency order', () => {
  const steps: GraphStep[] = [
    { id: 'checkout', name: 'checkout', status: 'success', dependsOn: [], duration: '4s', tasks: [] },
    {
      id: 'build',
      name: 'build',
      status: 'success',
      dependsOn: ['checkout'],
      duration: '12s',
      tasks: [
        { id: 'compile', name: 'compile', status: 'success', duration: '8s', dependsOn: [] },
        { id: 'test', name: 'test', status: 'failed', duration: '3s', dependsOn: ['compile'] },
      ],
    },
  ];

  const lines = buildRunExecutionLines(buildStepExecutionGroups(steps));

  assert.deepEqual(lines.map(line => [line.stepName, line.unitName, line.status, line.duration]), [
    ['checkout', 'checkout', 'success', '4s'],
    ['build', 'compile', 'success', '8s'],
    ['build', 'test', 'failed', '3s'],
  ]);
});
