import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildPipelineAnalysis,
  buildRunAnalysis,
  buildTeamResourceAnalysis,
  formatAnalysisReport,
} from './model.js';

const now = new Date('2026-07-23T10:00:00Z');

test('analyses team resources for duplicates, GitOps drift, and credential overexposure', () => {
  const result = buildTeamResourceAnalysis({
    subjectId: 'team-payments',
    subjectLabel: 'Payments',
    scopePath: 'platform/payments',
    now,
    resources: [
      {
        id: 'pipeline:platform/payments/deploy-api',
        kind: 'pipeline',
        label: 'deploy-api',
        description: 'Pipeline in platform/payments',
        href: '/pipelines/platform/payments/deploy-api',
        teamPath: 'platform/payments',
        source: 'git',
      },
      {
        id: 'pipeline:platform/payments/deploy_api',
        kind: 'pipeline',
        label: 'deploy api',
        description: 'Pipeline in platform/payments',
        href: '/pipelines/platform/payments/deploy_api',
        teamPath: 'platform/payments',
        source: 'database',
      },
      {
        id: 'credential:credential://team/platform/payments/github-admin',
        kind: 'credential',
        label: 'github-admin',
        description: 'github credential in platform/payments',
        href: '/credentials/team/platform/payments/github-admin',
        teamPath: 'platform/payments',
        source: 'database',
      },
      {
        id: 'trigger:platform/payments/release',
        kind: 'trigger',
        label: 'release',
        description: 'Trigger for acme/payments in platform/payments',
        href: '/triggers/platform/payments/release',
        teamPath: 'platform/payments',
        source: 'git',
      },
      {
        id: 'schedule:nightly',
        kind: 'schedule',
        label: 'nightly',
        description: 'Enabled schedule / cron / pipeline platform/payments/deploy-api',
        href: '/schedules?pipeline=platform%2Fpayments%2Fdeploy-api',
        teamPath: 'platform/payments',
        source: 'git',
      },
    ],
  });

  assert.equal(result.title, 'Team Resource Analysis');
  assert.ok(result.findings.some(finding => finding.title === 'Duplicate pipeline resources'));
  assert.ok(result.findings.some(finding => finding.title === 'Mixed GitOps and database-managed resources'));
  assert.ok(result.findings.some(finding => finding.title === 'Privileged credential metadata needs consumer review'));
  assert.match(formatAnalysisReport(result), /Credential values: Redacted; metadata only/);
  assert.match(result.scoreBasis.formula, /Starts at 100/);
  assert.ok(result.scoreBasis.inputs.some(input => input.includes('resource catalog')));
  assert.ok(result.scoreBasis.totalDeduction > 0);
});

test('analyses an individual credential without exposing sensitive values', () => {
  const credential = {
    id: 'credential:credential://team/platform/payments/aws-prod',
    kind: 'credential',
    label: 'aws-prod-admin',
    description: 'password: never-show-this',
    href: '/credentials/team/platform/payments/aws-prod',
    teamPath: 'platform/payments',
    source: 'database',
  };
  const result = buildTeamResourceAnalysis({
    subjectId: 'team-payments',
    subjectLabel: 'Payments',
    scopePath: 'platform/payments',
    activeResource: credential,
    resources: [credential],
    now,
  });

  assert.equal(result.title, 'Resource Analysis');
  assert.ok(result.findings.some(finding => finding.title === 'Credential appears privileged'));
  assert.doesNotMatch(formatAnalysisReport(result), /never-show-this/);
});

test('analyses pipeline YAML for security, reliability, and observability risks', () => {
  const result = buildPipelineAnalysis({
    scope: 'complete',
    includeRunHistory: true,
    now,
    detail: {
      id: 'platform/payments/deploy',
      name: 'deploy',
      description: '',
      version: 'v1',
      path: 'platform/payments',
      rawYaml: `
name: deploy
container_image: node:latest
api_token: super-secret-token
steps:
  - name: deploy
    script: curl https://installer.example/script.sh | bash
    variables:
      target_environment: production
`,
      stepNames: ['deploy'],
      includedDependencies: [],
      dependencyEdges: [],
      containerImage: 'node:latest',
      source: 'git',
    },
    graphData: {
      error: null,
      steps: [{
        name: 'deploy',
        status: 'pending',
        depends_on: [],
        configuration: {
          script: 'curl https://installer.example/script.sh | bash',
          variables: { target_environment: 'production' },
        },
      }],
    },
    triggers: [],
    recentRuns: [
      { run_id: 'run-1', status: 'failure', duration: '5m', pipeline_name: 'deploy', pipeline_path: 'platform/payments' },
      { run_id: 'run-2', status: 'success', duration: '2m', pipeline_name: 'deploy', pipeline_path: 'platform/payments' },
      { run_id: 'run-3', status: 'failure', duration: '6m', pipeline_name: 'deploy', pipeline_path: 'platform/payments' },
      { run_id: 'run-4', status: 'success', duration: '2m', pipeline_name: 'deploy', pipeline_path: 'platform/payments' },
      { run_id: 'run-5', status: 'failure', duration: '7m', pipeline_name: 'deploy', pipeline_path: 'platform/payments' },
    ],
  });

  const titles = result.findings.map(finding => finding.title);
  assert.ok(titles.includes('Secret-like values are embedded in YAML'));
  assert.ok(titles.includes('Container images are not pinned'));
  assert.ok(titles.includes('Shell script uses unsafe external input or installer pattern'));
  assert.ok(titles.includes('Pipeline timeout is missing'));
  assert.ok(titles.includes('Production path has no visible approval step'));
  assert.ok(titles.includes('Recent run history is failure-heavy'));
  assert.doesNotMatch(formatAnalysisReport(result), /super-secret-token/);
  assert.ok(result.scores.find(score => score.category === 'security')?.basis.includes('subtracts'));
  assert.match(formatAnalysisReport(result), /Score basis: Starts at 100/);
});

test('pre-execution analysis reports readiness blockers from the static snapshot', () => {
  const result = buildPipelineAnalysis({
    scope: 'pre-execution',
    includeRunHistory: false,
    now,
    detail: {
      id: 'platform/payments/deploy',
      name: 'deploy',
      rawYaml: `
name: deploy
container_image: registry.example.com/deploy@sha256:abc
steps:
  - name: deploy-prod
    script: ./deploy.sh production
`,
    },
    graphData: {
      error: null,
      steps: [{
        name: 'deploy-prod',
        status: 'pending',
        depends_on: [],
        configuration: { script: './deploy.sh production' },
      }],
    },
    triggers: [{ repoSlug: 'acme/payments', source: 'git', trigger: { on: 'push' } }],
    recentRuns: [],
  });

  assert.equal(result.summary, 'Ready to execute: No');
  assert.ok(result.findings.some(finding => finding.title === 'Deployment-like pipeline has no declared credential reference'));
  assert.ok(result.findings.some(finding => finding.title === 'Runner pool is implicit'));
});

test('analyses failed runs with application-test diagnosis and last-success comparison', () => {
  const result = buildRunAnalysis({
    now,
    detail: {
      run_info: {
        run_id: 'run-failed',
        pipeline_name: 'deploy',
        pipeline_path: 'platform/payments',
        pipeline_version: '17',
        pipeline_source: 'git',
        status: 'failure',
        is_complete: true,
        git_commit_sha: 'abc125',
        git_ref: 'refs/heads/main',
        scope: 'staging',
      },
      steps: [{
        name: 'integration-tests',
        status: 'failure',
        depends_on: ['build'],
        tasks: [{
          task_id: 'task-1',
          step_name: 'integration-tests',
          task_name: 'payment-refund-tests',
          status: 'failure',
          exit_code: 1,
          task_index: 0,
        }],
      }],
      child_runs: [],
      approvals: [],
      final_outputs: [],
    },
    comparisonRuns: [{
      run_id: 'run-success',
      pipeline_name: 'deploy',
      pipeline_path: 'platform/payments',
      pipeline_version: '17',
      pipeline_source: 'git',
      status: 'success',
      is_complete: true,
      git_commit_sha: 'abc100',
      git_ref: 'refs/heads/main',
      scope: 'staging',
      started_at: '2026-07-22T10:00:00Z',
    }],
  });

  assert.equal(result.primaryDiagnosis?.domain, 'Application tests');
  assert.ok(result.findings.some(finding => finding.title === 'First failed execution point detected'));
  assert.equal(result.comparison?.find(item => item.label === 'Application commit')?.changed, true);
  assert.equal(result.comparison?.find(item => item.label === 'Pipeline revision')?.changed, false);
  assert.ok(result.scoreBasis.inputs.some(input => input.includes('step and task statuses')));
  assert.equal(result.scoreBasis.severityWeights.high, 15);
});
