import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test } from 'node:test';
import { appVersionFooterText, normalizePlatformVersionInfo } from './platformVersion.js';

test('normalizes public platform version payloads', () => {
  assert.deepEqual(normalizePlatformVersionInfo({ productVersion: ' dev ' }), {
    productVersion: 'dev',
  });
  assert.deepEqual(normalizePlatformVersionInfo({ version: 'dev' }), {
    productVersion: 'dev',
  });
  assert.equal(normalizePlatformVersionInfo({ productVersion: '' }), null);
  assert.equal(normalizePlatformVersionInfo(null), null);
});

test('formats footer as version only', () => {
  assert.equal(appVersionFooterText({ productVersion: 'dev' }), 'Version dev');
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
  assert.doesNotMatch(nginxConfig, /upgrade-insecure-requests/);
  assert.match(nginxConfig, /add_header X-Frame-Options "DENY" always;/);
  assert.match(nginxConfig, /add_header X-Content-Type-Options "nosniff" always;/);
});
