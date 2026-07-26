import assert from 'node:assert/strict';
import { test } from 'node:test';
import { configRepositoryFormFromRecord, configRepositoryPayloadFromForm, normalizeConfigRepository, normalizeRuntimePools, normalizeSystemConfigPayload, systemConfigPayloadFromForm } from './model.js';

test('normalizes system runtime config with GitHub App UI ownership', () => {
  const { config, envFilePath, fieldMetadata } = normalizeSystemConfigPayload({
    log_level: 'info',
    log_format: 'json',
    environment: 'production',
    public_url: 'https://nopsai.example.com',
    notification_mail_logo_url: 'https://cdn.example.com/logo.png',
    notification_mail_website_url: 'https://example.com',
    notification_mail_support_url: 'https://support.example.com',
    notification_mail_footer_address: 'Example Corp',
    require_production_gates: true,
    agent_image: 'nopsai-agent:test',
    docker_network_name: 'nopsai-net',
    default_pipeline_timeout: '30m',
    llm_agent_timeout: '2m',
    auto_removal_agent_container: true,
    nopsai_api_url: 'http://nopsai:8080',
    git_bot_api_url: 'http://git-bot:8081',
    dispatcher_grpc_address: 'dispatcher:9090',
    dispatcher_routing: { prod: ['runner-prod'], '*': ['runner-general'] },
    runner_id: 'runner-general',
    runner_scopes: 'dev,prod',
    runner_capacity: 2,
    runtime_pools: {
      default: {
        node_selector: { workload: 'nopsai' },
      },
      'high-memory': {
        node_selector: { workload: 'nopsai', 'node-class': ' memory ' },
        resources: {
          requests: { memory: ' 4Gi ' },
          limits: { memory: '16Gi' },
        },
      },
    },
    env_file_path: '.env',
    github_app_id: '123456',
    github_installation_id: '987654',
    github_private_key_credential_ref: 'credential://system/github/app-private-key',
    github_webhook_credential_ref: 'credential://system/github/webhook-secret',
    field_metadata: {
      log_level: { scope: 'runtime_live', label: 'Log level', section: 'General', apply: 'Applied immediately' },
      agent_image: { scope: 'next_run_only', label: 'Agent image', section: 'Runtime', apply: 'Applies to new runs only' },
    },
  });

  assert.equal(envFilePath, '.env');
  assert.equal(config.log_level, 'info');
  assert.equal(config.public_url, 'https://nopsai.example.com');
  assert.equal(config.require_production_gates, true);
  assert.equal(config.github_app_id, '123456');
  assert.equal(config.github_installation_id, '987654');
  assert.equal(config.github_private_key_credential_ref, 'credential://system/github/app-private-key');
  assert.equal(config.github_webhook_credential_ref, 'credential://system/github/webhook-secret');
  assert.equal(config.runner_capacity, '2');
  assert.deepEqual(config.runtime_pools, {
    default: {
      node_selector: { workload: 'nopsai' },
      resources: { requests: {}, limits: {} },
    },
    'high-memory': {
      node_selector: { workload: 'nopsai', 'node-class': 'memory' },
      resources: { requests: { memory: '4Gi' }, limits: { memory: '16Gi' } },
    },
  });
  assert.deepEqual(config.dispatcher_routing, { '*': ['runner-general'], prod: ['runner-prod'] });
  assert.equal(fieldMetadata.log_level.apply, 'Applied immediately');

  const payload = systemConfigPayloadFromForm({
    ...config,
    public_url: ' https://nopsai.prod.example.com ',
    agent_image: ' nopsai-agent:prod ',
    github_app_id: ' 654321 ',
    github_installation_id: ' 456789 ',
    github_private_key_credential_ref: ' credential://system/github/prod-private-key ',
    github_webhook_credential_ref: ' credential://system/github/prod-webhook-secret ',
    runner_capacity: '3',
  });

  assert.equal(payload.public_url, 'https://nopsai.prod.example.com');
  assert.equal(payload.require_production_gates, true);
  assert.equal(payload.agent_image, 'nopsai-agent:prod');
  assert.equal(Object.hasOwn(payload, 'github_app_id'), false);
  assert.equal(Object.hasOwn(payload, 'github_installation_id'), false);
  assert.equal(Object.hasOwn(payload, 'github_private_key_credential_ref'), false);
  assert.equal(Object.hasOwn(payload, 'github_webhook_credential_ref'), false);
  assert.equal(payload.runner_capacity, 3);
  assert.deepEqual(payload.runtime_pools, config.runtime_pools);
});

test('normalizes runtime pool map names, selectors, requests, and limits', () => {
  assert.deepEqual(
    normalizeRuntimePools({
      ' high-memory ': {
        node_selector: {
          ' node-class ': ' memory ',
          empty: '',
        },
        resources: {
          requests: {
            cpu: ' 500m ',
            memory: '4Gi',
          },
          limits: {
            memory: ' 16Gi ',
          },
        },
      },
      ' ': {
        node_selector: {
          workload: 'ignored',
        },
      },
    }),
    {
      'high-memory': {
        node_selector: { 'node-class': 'memory' },
        resources: {
          requests: { cpu: '500m', memory: '4Gi' },
          limits: { memory: '16Gi' },
        },
      },
    }
  );
});

test('normalizes config repository provider credentials and payload', () => {
  const repo = normalizeConfigRepository({
    id: 8,
    scope_type: 'system',
    scope_id: 'global',
    provider: 'gitlab',
    repo_url: 'https://gitlab.com/acme/platform/configs',
    branch: '',
    base_path: 'nopsai',
    credential_ref: 'credential://system/gitops/gitlab',
    enabled: true,
    write_enabled: true,
    write_branch: '',
    last_sync_status: 'success',
  });

  assert.ok(repo);
  assert.equal(repo.provider, 'gitlab');
  assert.equal(repo.credential_ref, 'credential://system/gitops/gitlab');
  assert.equal(repo.branch, 'main');
  assert.equal(repo.write_branch, 'nopsai/ui-changes');

  const form = configRepositoryFormFromRecord(repo);
  const payload = configRepositoryPayloadFromForm({
    ...form,
    credential_ref: ' credential://system/gitops/gitlab-prod ',
  });
  assert.equal(payload.provider, 'gitlab');
  assert.equal(payload.credential_ref, 'credential://system/gitops/gitlab-prod');
});
