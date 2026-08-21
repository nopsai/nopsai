import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test } from 'node:test';
import { appVersionFooterText, normalizePlatformVersionInfo } from './platformVersion.js';

const empty = {
  commit: '',
  buildDate: '',
  apiVersion: '',
  cliCompatibility: '',
  runnerCompatibility: '',
  runnerProtocolVersion: '',
  releaseManifestDigest: '',
};

test('normalizes public platform version payloads', () => {
  assert.deepEqual(normalizePlatformVersionInfo({ productVersion: ' dev ' }), {
    productVersion: 'dev',
    ...empty,
  });
  assert.deepEqual(normalizePlatformVersionInfo({ version: 'dev' }), {
    productVersion: 'dev',
    ...empty,
  });
  assert.equal(normalizePlatformVersionInfo({ productVersion: '' }), null);
  assert.equal(normalizePlatformVersionInfo(null), null);
});

test('carries the rest of the build info the About dialog shows', () => {
  assert.deepEqual(
    normalizePlatformVersionInfo({
      productVersion: '1.4.0',
      commit: ' abc1234 ',
      buildDate: '2026-08-20',
      apiVersion: 'v1',
      cliCompatibility: '>=0.9',
      runnerCompatibility: '>=0.9',
      // The endpoint sends this one as a number.
      runnerProtocolVersion: 3,
      releaseManifestDigest: 'sha256:feed',
    }),
    {
      productVersion: '1.4.0',
      commit: 'abc1234',
      buildDate: '2026-08-20',
      apiVersion: 'v1',
      cliCompatibility: '>=0.9',
      runnerCompatibility: '>=0.9',
      runnerProtocolVersion: '3',
      releaseManifestDigest: 'sha256:feed',
    }
  );
});

test('formats footer as version only', () => {
  assert.equal(appVersionFooterText({ productVersion: 'dev', ...empty }), 'Version dev');
  assert.equal(appVersionFooterText(null), '');
});

test('proxies the public version endpoint before the SPA fallback', () => {
  const nginxConfig = readFileSync(resolve(process.cwd(), 'nginx.conf'), 'utf8');
  const versionLocation = nginxConfig.indexOf('location = /version');
  const spaFallback = nginxConfig.indexOf('location / {');

  assert.ok(versionLocation >= 0, 'nginx must proxy /version to the API service');
  assert.ok(spaFallback < 0 || versionLocation < spaFallback, '/version must be matched before the SPA fallback');
  assert.match(nginxConfig.slice(versionLocation), /proxy_pass http:\/\/nopsai:8080;/);
});

test('nginx serves the SPA with restrictive browser security headers', () => {
  const nginxConfig = readFileSync(resolve(process.cwd(), 'nginx.conf'), 'utf8');

  assert.match(nginxConfig, /add_header Content-Security-Policy "default-src 'self'/);
  assert.match(nginxConfig, /frame-ancestors 'none'/);
  assert.doesNotMatch(nginxConfig, /fonts\.googleapis\.com|fonts\.gstatic\.com/);
  assert.doesNotMatch(nginxConfig, /upgrade-insecure-requests/);
  assert.match(nginxConfig, /add_header X-Frame-Options "DENY" always;/);
  assert.match(nginxConfig, /add_header X-Content-Type-Options "nosniff" always;/);
});

// GitHub registers an App only from a manifest the operator's own browser POSTs
// to github.com. A form-action that omits it makes the browser cancel that
// navigation with no error, which strands the Git Apps connect flow on a
// spinner, so every policy copy has to carry the exemption.
test('nginx lets the browser post a GitHub App manifest to github.com', () => {
  const nginxConfig = readFileSync(resolve(process.cwd(), 'nginx.conf'), 'utf8');
  const policies = nginxConfig
    .split('\n')
    .filter(line => line.includes('add_header Content-Security-Policy'));

  assert.ok(policies.length > 0, 'nginx must set a Content-Security-Policy');
  for (const policy of policies) {
    assert.match(policy, /form-action 'self' https:\/\/github\.com"/);
  }
});
