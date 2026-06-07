import assert from 'node:assert/strict';
import { test } from 'node:test';
import { buildPipelineGraphData, normalizePipelineSource, parsePipelineYaml, validatePipelineYaml } from './model.js';

test('validates pipeline container image requirements for executable steps', () => {
  const result = validatePipelineYaml(`
name: deploy
steps:
  - name: build
    script: npm run build
`);

  assert.equal(result.errors[0]?.message, "'container_image' is required when steps do not specify their own image.");
});

test('parses pipeline details, includes, variables, and dependency edges', () => {
  const detail = parsePipelineYaml(
    `
name: deploy
description: Deploy app
version: v2
container_image: alpine:3.20
variables:
  - environment
steps:
  - name: build
    include: step:shared/build
  - name: deploy
    depends_on:
      - build
    script: ./deploy.sh
`,
    'platform/payments/deploy',
    'git'
  );

  assert.equal(detail.name, 'deploy');
  assert.equal(detail.path, 'platform/payments');
  assert.deepEqual(detail.variables, ['environment']);
  assert.deepEqual(detail.includedDependencies, ['step:shared/build']);
  assert.deepEqual(detail.dependencyEdges, [{ from: 'build', to: 'deploy' }]);
});

test('builds pipeline graph data from editor YAML', () => {
  const graph = buildPipelineGraphData(`
name: deploy
description: Deploy app
version: v2
steps:
  - name: build
    image: node:22
    script: npm run build
    secrets:
      - NPM_TOKEN
  - name: approve
    depends_on:
      - build
    approval:
      type: group
      groups:
        - sre
      allow_self_approval: false
  - name: deploy
    depends_on:
      - approve
    tasks:
      - name: rollout
        script: ./deploy.sh
        variables:
          environment: prod
`);

  assert.equal(graph.error, null);
  assert.equal(graph.definition?.name, 'deploy');
  assert.deepEqual(
    graph.definition?.steps?.map(step => ({ name: step.name, depends_on: step.depends_on })),
    [
      { name: 'build', depends_on: [] },
      { name: 'approve', depends_on: ['build'] },
      { name: 'deploy', depends_on: ['approve'] },
    ]
  );
  assert.deepEqual(
    graph.steps.map(step => ({
      name: step.name,
      depends_on: step.depends_on,
      taskNames: step.tasks.map(task => task.task_name),
    })),
    [
      { name: 'build', depends_on: [], taskNames: [] },
      { name: 'approve', depends_on: ['build'], taskNames: [] },
      { name: 'deploy', depends_on: ['approve'], taskNames: ['rollout'] },
    ]
  );
  assert.deepEqual(graph.steps[0]?.configuration?.secrets, ['NPM_TOKEN']);
  assert.deepEqual(graph.steps[1]?.configuration?.approval?.groups, ['sre']);
  assert.deepEqual(graph.steps[2]?.configuration?.tasks?.[0]?.variables, { environment: 'prod' });
});

test('normalizes pipeline source labels', () => {
  assert.equal(normalizePipelineSource('GitOps'), 'git');
  assert.equal(normalizePipelineSource('draft'), 'draft');
  assert.equal(normalizePipelineSource('db'), 'database');
});
