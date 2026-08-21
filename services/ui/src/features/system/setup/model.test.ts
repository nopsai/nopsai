import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  LLM_SKIP_WARNING,
  WIZARD_STEPS,
  defaultCredentialRef,
  deriveGitBotBaseURL,
  isLikelyPublicURL,
  normalizeTeamName,
  parseRepositories,
  runtimeDefaults,
  secretPlaceholder,
  statusClasses,
} from './model.js';

test('defines setup wizard steps and skip warning copy', () => {
  assert.deepEqual(
    WIZARD_STEPS.map(step => step.id),
    ['readiness', 'runtime', 'github', 'gitops', 'repositories', 'ai', 'users', 'review']
  );
  assert.match(LLM_SKIP_WARNING, /LLM profile setup was skipped/);
});

test('normalizes setup repository input', () => {
  assert.deepEqual(parseRepositories('acme/api\nacme/web, acme/api'), ['acme/api', 'acme/web']);
  assert.equal(normalizeTeamName('/Platform Services/'), 'Platform-Services');
});

test('derives runtime and GitHub integration defaults', () => {
  assert.deepEqual(runtimeDefaults('docker'), {
    nopsaiAPIURL: 'http://nopsai:8080',
    gitBotServiceURL: 'http://git-bot:8081',
  });
  assert.deepEqual(runtimeDefaults('kubernetes'), {
    nopsaiAPIURL: 'http://nopsai.nopsai.svc.cluster.local:8080',
    gitBotServiceURL: 'http://nopsai-git-bot.nopsai.svc.cluster.local:8081',
  });
  assert.equal(deriveGitBotBaseURL('https://hooks.example.test/webhook'), 'https://hooks.example.test');
  assert.equal(deriveGitBotBaseURL(''), 'https://nopsai.example.com/git-bot');
});

test('classifies public URLs and provider credential references', () => {
  assert.equal(isLikelyPublicURL('https://hooks.example.test/webhook'), true);
  assert.equal(isLikelyPublicURL('http://localhost:8081/webhook'), false);
  assert.equal(isLikelyPublicURL('http://git-bot:8081/webhook'), false);
  assert.equal(defaultCredentialRef('gemini'), 'credential://system/llm/standard');
  assert.equal(defaultCredentialRef('lmstudio'), 'credential://system/llm/standard');
  assert.equal(defaultCredentialRef('openai'), 'credential://system/llm/standard');
  assert.equal(defaultCredentialRef('anthropic'), 'credential://system/llm/standard');
  assert.equal(defaultCredentialRef('ollama'), 'credential://system/llm/standard');
});

test('formats setup status classes and secret placeholders', () => {
  assert.match(statusClasses('success'), /emerald/);
  assert.match(statusClasses('error'), /red/);
  assert.match(statusClasses('warning'), /amber/);
  assert.match(statusClasses('info'), /sky/);
  assert.equal(secretPlaceholder(true, '<fallback>'), '<provided in wizard; store as a secret value>');
  assert.equal(secretPlaceholder(false, '<fallback>'), '<fallback>');
});


