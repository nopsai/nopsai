import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildPipelineGraphData,
  buildPipelineDependencyReferences,
  filterVisiblePipelineList,
  formatPipelineGitRef,
  formatPipelineTriggerBranchField,
  formatPipelineTriggerEvent,
  formatPipelineTriggerScope,
  normalizePipelineSource,
  parsePipelineDependencyReference,
  parsePipelineYaml,
  pipelineRunStatusClass,
  pipelineRunStatusLabel,
  validatePipelineYaml,
} from './model.js';

test('validates pipeline container image requirements for executable steps', () => {
  const result = validatePipelineYaml(`
name: deploy
steps:
  - name: build
    script: npm run build
`);

  assert.equal(result.errors[0]?.message, "'container_image' is required when steps do not specify their own image.");
});

test('validates pipeline final outputs without unknown-field errors', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
output:
  model: report-writer
  items:
    - name: Executive summary
      type: markdown
      when: success
      prompt: |
        Summarize the run for leadership.
steps:
  - name: build
    script: echo "ok"
`);

  assert.deepEqual(result.errors, []);
});

test('validates governance level directives without unknown-field errors', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
governance_level: strict
steps:
  - name: build
    governance_level: advisory
    tasks:
      - name: summarize
        governance_level: advisory
        goal: Summarize deployment readiness.
`);

  assert.deepEqual(result.errors, []);
});

test('validates blocking knowledge direct scripts when LLM is enabled', () => {
  const result = validatePipelineYaml(`
name: ad-hoc-pipeline
version: latest
description: Lab pipeline (ad-hoc)
container_image: alpine:latest
llm_enabled: true
llm_content_preload: true
model: fast
knowledge_context:
  - kind: guardrail
    ref: data-team/runtime-output-safety
steps:
  - name: hello
    script: echo "show env variables"
  - name: hello2
    script: env
    depends_on:
      - hello
`);

  assert.deepEqual(result.errors, []);
});

test('rejects blocking knowledge direct scripts when LLM is disabled', () => {
  const result = validatePipelineYaml(`
name: governed-script
container_image: alpine:3.20
llm_enabled: false
knowledge_context:
  - kind: guardrail
    ref: data-team/runtime-output-safety
steps:
  - name: hello
    script: env
`);

  assert.match(result.errors[0]?.message || '', /LLM disabled.*script step 'hello'.*blocking knowledge context/);
});

test('validates runtime variable declarations, task outputs, and qualified dependencies', () => {
  const result = validatePipelineYaml(`
name: variable-feature-exercise
container_image: alpine:3.20
llm_enabled: false
variables:
  - API_VERSION
  - default:GLOBAL_REGION
  - team-1/prod:REGISTRY
steps:
  - name: collect-runtime-values
    tasks:
      - name: produce
        outputs:
          - release_manifest
          - image_tag
          - IMAGE_TAG
          - name: access_token
            sensitive: true
        script: |
          printf manifest > /nopsai/outputs/release_manifest
      - name: consume
        depends_on:
          - produce
        variables:
          RELEASE_MANIFEST: $steps.collect-runtime-values.produce.outputs.release_manifest
          IMAGE_TAG_LOWER: $steps.collect-runtime-values.produce.outputs.image_tag
          IMAGE_TAG_UPPER: $steps.collect-runtime-values.produce.outputs.IMAGE_TAG
          ACCESS_TOKEN: $steps.collect-runtime-values.produce.outputs.access_token
        script: echo consume
  - name: child-pipeline-variable-overrides
    include: pipeline:variable-feature-child
    depends_on:
      - collect-runtime-values.produce
    variables:
      CHILD_RELEASE_MANIFEST: $steps.collect-runtime-values.produce.outputs.release_manifest
`);

  assert.deepEqual(result.errors, []);
});

test('validates sync pipeline include outputs with step-level references in UI validation', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
steps:
  - name: child-build
    include: pipeline:child-build
    sync: true
    outputs:
      - image_tag
  - name: deploy
    depends_on:
      - child-build
    variables:
      IMAGE_TAG: $steps.child-build.outputs.image_tag
    script: echo deploy
`);

  assert.deepEqual(result.errors, []);
});

test('rejects async pipeline include outputs in UI validation', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
steps:
  - name: child-build
    include: pipeline:child-build
    outputs:
      - image_tag
`);

  assert.match(result.errors[0]?.message || '', /sync: true/);
});

test('rejects invalid pipeline variable declarations in UI validation', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
variables:
  - default:API_VERSION
  - team-1/prod:API_VERSION
steps:
  - name: build
    script: echo ok
`);

  assert.match(result.errors[0]?.message || '', /declares runtime variable 'API_VERSION' more than once/);
});

test('rejects runtime output references without valid dependencies in UI validation', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
steps:
  - name: prepare
    tasks:
      - name: produce
        outputs:
          - image_tag
        script: echo ok
  - name: build
    variables:
      IMAGE_TAG: $steps.prepare.produce.outputs.image_tag
    script: echo build
`);

  assert.match(result.errors[0]?.message || '', /without a valid dependency/);
});

test('allows runtime output references through transitive dependencies in UI validation', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
steps:
  - name: prepare
    tasks:
      - name: produce
        outputs:
          - image_tag
        script: echo ok
  - name: gate
    depends_on:
      - prepare
    script: echo gate
  - name: build
    depends_on:
      - gate
    variables:
      IMAGE_TAG: $steps.prepare.produce.outputs.image_tag
    script: echo build
`);

  assert.deepEqual(result.errors, []);
});

test('allows runtime output references through approval dependency gates in UI validation', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
steps:
  - name: prepare
    tasks:
      - name: produce
        outputs:
          - image_tag
        script: echo ok
  - name: approval
    depends_on:
      - prepare
    approval:
      type: deploy
      teams:
        - platform/prod
  - name: deploy
    depends_on:
      - approval
    variables:
      IMAGE_TAG: $steps.prepare.produce.outputs.image_tag
    script: echo deploy
`);

  assert.deepEqual(result.errors, []);
});

test('validates approval timeout in UI validation', () => {
  const valid = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
steps:
  - name: approval
    approval:
      type: deploy
      teams:
        - platform/prod
      timeout: 1h30m
`);
  assert.deepEqual(valid.errors, []);

  const invalid = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
steps:
  - name: approval
    approval:
      type: deploy
      teams:
        - platform/prod
      timeout: soon
`);
  assert.match(invalid.errors[0]?.message || '', /approval\.timeout must be a positive duration/);
});

test('validates pipeline dashboard final outputs without dashboard field errors', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
output:
  items:
    - name: Dashboard summary
      type: dashboard
      when: success
      dashboard:
        ref: platform/ops
        section: overview
        entry_key: daily
        mode: replace
        preset: status
        ttl: 24h
      prompt: |
        Summarize the deployment state for the dashboard.
steps:
  - name: build
    script: echo "ok"
`);

  assert.deepEqual(result.errors, []);
});

test('validates pipeline dashboard final output target requirements', () => {
  const missingTarget = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
output:
  items:
    - name: Dashboard summary
      type: dashboard
      prompt: Summarize the deployment state.
steps:
  - name: build
    script: echo "ok"
`);

  assert.match(missingTarget.errors[0]?.message || '', /dashboard target must be an object/);

  const invalidMode = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
output:
  items:
    - name: Dashboard summary
      type: dashboard
      dashboard:
        ref: platform/ops
        section: overview
        mode: overwrite
      prompt: Summarize the deployment state.
steps:
  - name: build
    script: echo "ok"
`);

  assert.match(invalidMode.errors[0]?.message || '', /dashboard\.mode 'overwrite' is not supported/);
});

test('rejects dashboard target configuration on non-dashboard outputs', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
output:
  items:
    - name: Executive summary
      type: markdown
      dashboard:
        ref: platform/ops
        section: overview
      prompt: Summarize the deployment state.
steps:
  - name: build
    script: echo "ok"
`);

  assert.match(result.errors[0]?.message || '', /dashboard configuration requires type 'dashboard'/);
});

test('validates pipeline final output item shape', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
output:
  items:
    - name: Executive summary
      type: archive
      prompt: |
        Summarize the run for leadership.
steps:
  - name: build
    script: echo "ok"
`);

  assert.match(result.errors[0]?.message || '', /unsupported type 'archive'/);
});

test('validates pipeline final output when values', () => {
  const result = validatePipelineYaml(`
name: deploy
container_image: alpine:3.20
output:
  items:
    - name: Executive summary
      type: markdown
      when: manual
      prompt: |
        Summarize the run for leadership.
steps:
  - name: build
    script: echo "ok"
`);

  assert.match(result.errors[0]?.message || '', /unsupported when 'manual'/);
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
  assert.deepEqual(buildPipelineDependencyReferences(detail), [
    {
      raw: 'step:shared/build',
      identifier: 'shared/build',
      typeLabel: 'Step',
      actionLabel: 'Open',
      navigable: true,
      kind: 'step',
    },
  ]);
});

test('builds pipeline graph data from editor YAML', () => {
  const graph = buildPipelineGraphData(`
name: deploy
description: Deploy app
version: v2
steps:
  - name: build
    image: node:22
    runtime_pool: ci
    script: npm run build
    secrets:
      - NPM_TOKEN
  - name: approve
    depends_on:
      - build
    approval:
      type: production-deploy
      teams:
        - sre
      allow_self_approval: false
      timeout: 24h
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
  assert.equal(graph.steps[0]?.configuration?.runtime_pool, 'ci');
  assert.deepEqual(graph.steps[1]?.configuration?.approval?.teams, ['sre']);
  assert.equal(graph.steps[1]?.configuration?.approval?.timeout, '24h');
  assert.deepEqual(graph.steps[2]?.configuration?.tasks?.[0]?.variables, { environment: 'prod' });
});

test('normalizes pipeline source labels', () => {
  assert.equal(normalizePipelineSource('GitOps'), 'git');
  assert.equal(normalizePipelineSource('draft'), 'draft');
  assert.equal(normalizePipelineSource('db'), 'database');
});

test('filters visible pipelines by all teams, selected team subtree, and search', () => {
  const items = [
    { id: 'release' },
    { id: 'platform/api/build' },
    { id: 'platform/deploy' },
    { id: 'sandbox/release' },
  ];

  assert.deepEqual(
    filterVisiblePipelineList(items, '', '').map(item => item.id),
    ['platform/api/build', 'platform/deploy', 'release', 'sandbox/release']
  );
  assert.deepEqual(
    filterVisiblePipelineList(items, '', 'global').map(item => item.id),
    ['release']
  );
  assert.deepEqual(
    filterVisiblePipelineList(items, '', 'platform').map(item => item.id),
    ['platform/api/build', 'platform/deploy']
  );
  assert.deepEqual(
    filterVisiblePipelineList(items, 'build', 'sandbox').map(item => item.id),
    ['platform/api/build']
  );
});

test('formats pipeline activity presentation consistently', () => {
  assert.equal(formatPipelineGitRef('refs/heads/main'), 'main');
  assert.equal(formatPipelineTriggerEvent('pull_request'), 'Pull request');
  assert.deepEqual(formatPipelineTriggerBranchField({ skip_branches: ['release/*'] }), {
    label: 'skip_branches:',
    value: 'release/*',
  });
  assert.equal(formatPipelineTriggerScope({ scope: 'production' }), 'production');
  assert.equal(pipelineRunStatusClass('failed'), 'runner-pill--error');
  assert.equal(pipelineRunStatusLabel('in_progress'), 'In progress');
  assert.deepEqual(parsePipelineDependencyReference('pipeline:platform/deploy'), {
    raw: 'pipeline:platform/deploy',
    identifier: 'platform/deploy',
    typeLabel: 'Pipeline',
    actionLabel: 'Open',
    navigable: true,
    kind: 'pipeline',
  });
  assert.deepEqual(parsePipelineDependencyReference('step:shared/build'), {
    raw: 'step:shared/build',
    identifier: 'shared/build',
    typeLabel: 'Step',
    actionLabel: 'Open',
    navigable: true,
    kind: 'step',
  });
});
