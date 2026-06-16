import assert from 'node:assert/strict';
import { test } from 'node:test';
import { normalizeSystemConfigPayload, systemConfigPayloadFromForm } from './model.js';

function hasOwn(value: object, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(value, key);
}

test('normalizes system runtime config without GitHub App UI ownership', () => {
  const { config, envFilePath } = normalizeSystemConfigPayload({
    agent_image: 'nopsai-agent:test',
    docker_network_name: 'nopsai-net',
    default_pipeline_timeout: '30m',
    llm_agent_timeout: '2m',
    auto_removal_agent_container: true,
    agent_nopsai_api_url: 'http://nopsai:8080',
    git_bot_nopsai_api_url: 'http://nopsai:8080',
    nopsai_git_bot_api_url: 'http://git-bot:8081',
    dispatcher_address: 'dispatcher:9090',
    dispatcher_routing: { prod: ['runner-prod'], '*': ['runner-general'] },
    runner_id: 'runner-general',
    runner_scopes: 'dev,prod',
    runner_capacity: 2,
    env_file_path: '.env',
    github_app_id: '123456',
    github_installation_id: '987654',
    github_private_key_credential_ref: 'credential://system/github/app-private-key',
    github_webhook_credential_ref: 'credential://system/github/webhook-secret',
  });

  assert.equal(envFilePath, '.env');
  assert.equal(config.runner_capacity, '2');
  assert.deepEqual(config.dispatcher_routing, { '*': ['runner-general'], prod: ['runner-prod'] });
  assert.equal(hasOwn(config, 'github_app_id'), false);

  const payload = systemConfigPayloadFromForm({
    ...config,
    agent_image: ' nopsai-agent:prod ',
    runner_capacity: '3',
  });

  assert.equal(payload.agent_image, 'nopsai-agent:prod');
  assert.equal(payload.runner_capacity, 3);
  assert.equal(hasOwn(payload, 'github_app_id'), false);
  assert.equal(hasOwn(payload, 'github_installation_id'), false);
  assert.equal(hasOwn(payload, 'github_private_key_credential_ref'), false);
  assert.equal(hasOwn(payload, 'github_webhook_credential_ref'), false);
});
