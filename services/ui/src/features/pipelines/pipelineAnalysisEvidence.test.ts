import assert from 'node:assert/strict';
import test from 'node:test';
import { buildPipelineAnalysisPromptContext } from './pipelineAnalysisEvidence.js';

test('builds pipeline AI context from YAML, graph, triggers, validation, and run history', () => {
  const context = buildPipelineAnalysisPromptContext({
    detail: {
      id: 'platform/payments/deploy',
      name: 'deploy',
      description: 'Deploy payments api password=never-show-this',
      version: 'v3',
      path: 'platform/payments',
      rawYaml: [
        'name: deploy',
        'container_image: registry.example.com/payments@sha256:abc',
        'api_token: never-show-this',
        'steps:',
        '  - name: build',
        '    script: npm run build',
        '  - name: deploy',
        '    depends_on: [build]',
        '    script: ./deploy.sh',
      ].join('\n'),
      stepNames: ['build', 'deploy'],
      variables: ['environment'],
      includedDependencies: ['step:shared/docker-login'],
      dependencyEdges: [{ from: 'build', to: 'deploy' }],
      containerImage: 'registry.example.com/payments@sha256:abc',
      source: 'git',
    },
    graphData: {
      error: null,
      steps: [
        {
          name: 'build',
          status: 'pending',
          depends_on: [],
          tasks: [],
          configuration: {
            script: 'npm run build --token=never-show-this',
            runtime_pool: 'ci',
          },
        },
        {
          name: 'deploy',
          status: 'pending',
          depends_on: ['build'],
          tasks: [],
          configuration: {
            runtime_pool: 'prod',
            tasks: [{
              name: 'rollout',
              script: './deploy.sh --password=never-show-this',
              depends_on: ['smoke'],
            }],
          },
        },
      ],
    },
    triggers: [{
      repoSlug: 'acme/payments',
      source: 'git',
      trigger: { event: 'push', branches: ['main'], token: 'never-show-this' },
    }],
    recentRuns: [{
      run_id: 'run-123',
      pipeline_name: 'deploy',
      pipeline_path: 'platform/payments',
      status: 'failure',
      duration: '4m',
      started_at: '2026-07-24T10:00:00Z',
      git_repo_owner: 'acme',
      git_repo_name: 'payments',
      git_ref: 'refs/heads/main',
      final_output_status: {
        configured: 2,
        status: 'failed',
        pending: 0,
        generating: 0,
        generated: 1,
        failed: 1,
        cancelled: 0,
        total: 2,
      },
    }],
    includeRunHistory: true,
    validationErrors: [{ line: 7, column: 5, message: 'step deploy must declare secrets explicitly' }],
  });

  const serialized = JSON.stringify(context);
  assert.match(serialized, /Pipeline YAML snapshot/);
  assert.match(serialized, /Parsed pipeline graph/);
  assert.match(serialized, /Pipeline trigger bindings/);
  assert.match(serialized, /Recent run history/);
  assert.match(serialized, /step deploy must declare secrets explicitly/);
  assert.match(serialized, /step:shared\/docker-login/);
  assert.match(serialized, /rollout/);
  assert.match(serialized, /run-123/);
  assert.doesNotMatch(serialized, /never-show-this/);
  assert.match(serialized, /\[redacted\]/);
});
