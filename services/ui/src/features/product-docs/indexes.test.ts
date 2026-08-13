import assert from 'node:assert/strict';
import test from 'node:test';
import { wikiArticleLocations } from './content/index.js';
import {
  apiAreas,
  apiRouteIndex,
  directiveIndex,
  directiveScopes,
  environmentIndex,
  filterApiRoutes,
  filterIndexedFields,
  isIndexArticle,
} from './indexes.js';
import { searchWiki } from './search.js';

test('the directive index aggregates every documented YAML directive', () => {
  assert.ok(directiveIndex.length >= 150, `expected at least 150 directives, found ${directiveIndex.length}`);

  const paths = directiveIndex.map(row => row.path);
  for (const expected of [
    'name',
    'steps',
    'governance_level',
    'steps[].approval.timeout',
    'tasks[].mcp_profiles',
    'output.items[].dashboard.mode',
    'triggers[].include_paths',
    'cron_expression',
    'allowed_callers',
    'knowledge_context[].kind',
  ]) {
    assert.ok(paths.includes(expected), `directive index is missing ${expected}`);
  }

  assert.ok(directiveScopes().includes('pipeline'));
  assert.ok(directiveScopes().includes('task'));
});

test('every index row links back to the article that explains it', () => {
  const known = new Set(wikiArticleLocations.map(location => location.article.id));
  for (const row of [...directiveIndex, ...environmentIndex]) {
    assert.ok(known.has(row.articleID), `${row.path} points at unknown article ${row.articleID}`);
    assert.ok(row.href.startsWith('/docs/'), `${row.path} has a non-wiki href`);
    assert.ok(row.href.includes('#field-'), `${row.path} does not deep-link to its row`);
  }
});

test('the environment index keeps bootstrap variables and never leaks directives', () => {
  const paths = environmentIndex.map(row => row.path);

  for (const expected of [
    'DATABASE_URL',
    'NOPSAI_MASTER_KEY',
    'JWT_SIGNING_KEY',
    'SERVICE_JWT_SIGNING_KEY',
    'DISPATCHER_TLS_MODE',
    'DISPATCHER_GRPC_ADDRESS',
    'SYSTEM_LOGS_DOCKER_HOST',
    'FINAL_OUTPUT_PDF_RENDERER_URL',
    'METRICS_REQUIRE_AUTH',
    'DOCKER_NETWORK_NAME',
  ]) {
    assert.ok(paths.includes(expected), `environment index is missing ${expected}`);
  }

  assert.equal(paths.includes('steps'), false);
  assert.equal(
    directiveIndex.some(row => row.path === 'DATABASE_URL'),
    false,
    'environment variables must not appear in the directive index',
  );
});

test('the environment index does not resurrect removed aspirational settings', () => {
  const paths = environmentIndex.map(row => row.path);

  for (const removed of ['REDIS_URL', 'NOPS_STORAGE_BACKEND', 'POSTGRES_DSN', 'NOPS_ENV', 'NOPS_PUBLIC_URL']) {
    assert.equal(paths.includes(removed), false, `${removed} is not implemented and must not be documented`);
  }
});

test('the API index covers every area with a stated access class', () => {
  assert.ok(apiRouteIndex.length >= 150, `expected at least 150 routes, found ${apiRouteIndex.length}`);
  assert.ok(apiAreas().length >= 15);

  const find = (method: string, path: string) =>
    apiRouteIndex.find(route => route.method === method && route.path === path);

  assert.equal(find('GET', '/healthz')?.access, 'public');
  assert.equal(find('GET', '/v1/setup/preflight')?.access, 'public');
  assert.equal(find('POST', '/v1/git/events')?.access, 'service');
  assert.equal(find('GET', '/v1/admin/users')?.access, 'admin');
  assert.equal(find('POST', '/v1/run')?.access, 'authorized');
  assert.equal(find('GET', '/v1/auth/me')?.access, 'authenticated');

  for (const route of apiRouteIndex) {
    assert.ok(route.purpose.length > 0, `${route.method} ${route.path} needs a purpose`);
    assert.ok(route.area.length > 0, `${route.method} ${route.path} needs an area`);
  }
});

test('index filters narrow by text and by scope or area', () => {
  assert.ok(filterIndexedFields(directiveIndex, 'dashboard', '').length > 3);
  assert.ok(filterIndexedFields(directiveIndex, '', 'task').every(row => row.scope === 'task'));
  assert.equal(filterIndexedFields(directiveIndex, 'zzzz-not-a-directive', '').length, 0);

  assert.ok(filterApiRoutes(apiRouteIndex, 'dashboards', '').length > 5);
  assert.ok(filterApiRoutes(apiRouteIndex, '', 'Runs').every(route => route.area === 'Runs'));
});

test('index articles are recognised so they render a generated table', () => {
  assert.equal(isIndexArticle('directive-index'), true);
  assert.equal(isIndexArticle('environment-index'), true);
  assert.equal(isIndexArticle('api-index'), true);
  assert.equal(isIndexArticle('pipeline-schema'), false);
});

test('search finds directives, prose, endpoints, and environment variables', () => {
  const directive = searchWiki('governance_level');
  assert.equal(directive[0]?.kind, 'directive');
  assert.equal(directive[0]?.title, 'governance_level');

  const envVar = searchWiki('DISPATCHER_TLS_MODE');
  assert.equal(envVar[0]?.kind, 'environment');

  // Prose that appears in no title: the old search could not find this at all.
  const prose = searchWiki('fails open');
  assert.ok(prose.some(result => result.href.includes('git-triggers')), 'expected the fail-open path rule to be findable');

  const limit = searchWiki('ejected');
  assert.ok(limit.length > 0, 'expected runner ejection to be findable');

  const endpoint = searchWiki('/v1/analysis/evaluate');
  assert.ok(endpoint.some(result => result.kind === 'endpoint'));

  assert.deepEqual(searchWiki('   '), []);
});
