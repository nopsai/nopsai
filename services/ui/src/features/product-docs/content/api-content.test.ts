import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';
import { apiRoutes } from './fields/api/index.js';
import { wikiSections } from './index.js';

const repositoryRoot = resolve(process.cwd(), '..', '..');
const documented = apiRoutes.filter(route => Boolean(route.depth));

test('every route carries the index row a reader needs to find it', () => {
  const seen = new Set<string>();
  for (const route of apiRoutes) {
    const key = `${route.method} ${route.path}`;
    assert.ok(!seen.has(key), `${key} is listed twice`);
    seen.add(key);
    assert.ok(route.area.length > 0, `${key} needs an area`);
    assert.ok(route.purpose.length > 0, `${key} needs a purpose`);
  }
});

test('a route documented in full answers every question the plan requires', () => {
  // Full depth owes: parameters, a call, a response with a
  // sample, a failure mode, and the test that proves the behaviour.
  for (const route of documented.filter(route => route.depth === 'full')) {
    const where = `${route.method} ${route.path}`;
    assert.ok(route.parameters !== undefined, `${where} must state its parameters, even when there are none`);
    assert.ok(route.requestSample, `${where} needs a runnable call`);
    assert.ok((route.responses?.length || 0) > 0, `${where} needs at least one documented response`);
    // A download route answers with bytes and a 204 or a redirect has no body, so
    // in both cases a pasted sample would be fiction rather than documentation.
    const carriesNoBody = (response: { status: number; contentType?: string }) =>
      response.status === 204 ||
      (response.status >= 300 && response.status < 400) ||
      /zip|octet-stream|pdf|excel|spreadsheet/.test(response.contentType || '');
    if (!route.responses?.every(carriesNoBody)) {
      assert.ok(
        route.responses?.some(response => Boolean(response.sample)),
        `${where} needs at least one response sample`,
      );
    }
    // An empty list is a real answer — some routes have no failure mode beyond the
    // shared middleware — but it has to be stated, and explained in the notes.
    assert.ok(route.errors !== undefined, `${where} must state its failure modes, even when there are none`);
    if (route.errors.length === 0) {
      assert.ok(route.notes, `${where} says it cannot fail, so it needs a note explaining why`);
    }
    assert.ok((route.sideEffects?.length || 0) > 0, `${where} must state its side effects, even if none`);
    assert.ok((route.coveringTests?.length || 0) > 0, `${where} needs a covering test`);
  }
});

test('a probe route documents what an operator should assert on', () => {
  for (const route of documented.filter(route => route.depth === 'probe')) {
    const where = `${route.method} ${route.path}`;
    assert.ok((route.responses?.length || 0) > 0, `${where} needs a documented response`);
    assert.ok((route.sideEffects?.length || 0) > 0, `${where} must state its side effects`);
  }
});

test('a contract route states its caller and its boundary', () => {
  // Contract depth owes less: an internal route gets a purpose, a
  // caller, and an explicit statement that it is not a public API — and no call
  // samples, because calling it by hand corrupts state rather than integrating.
  for (const route of documented.filter(route => route.depth === 'contract')) {
    const where = `${route.method} ${route.path}`;
    assert.ok(route.notes, `${where} needs a note naming its caller and boundary`);
    assert.ok((route.sideEffects?.length || 0) > 0, `${where} must state its side effects`);
    assert.ok((route.evidence?.length || 0) > 0, `${where} needs implementation evidence`);
    assert.equal(route.requestSample, undefined, `${where} is internal and must not carry a call sample`);
  }
});

test('path parameters match the path they belong to', () => {
  // Contract routes carry no parameter tables by design: they document the
  // caller and the boundary rather than how to call them.
  for (const route of documented.filter(route => route.depth !== 'contract')) {
    const placeholders = Array.from(route.path.matchAll(/\{([^}.]+)(\.\.\.)?\}/g)).map(match => match[1]);
    const declared = (route.parameters || []).filter(parameter => parameter.in === 'path').map(parameter => parameter.name);
    for (const placeholder of placeholders) {
      assert.ok(
        declared.includes(placeholder),
        `${route.method} ${route.path} does not document its {${placeholder}} parameter`,
      );
    }
    for (const name of declared) {
      assert.ok(
        placeholders.includes(name),
        `${route.method} ${route.path} documents a path parameter {${name}} the path does not contain`,
      );
    }
  }
});

test('samples match the route they document', () => {
  for (const route of documented) {
    const sample = route.requestSample;
    if (!sample) continue;
    const where = `${route.method} ${route.path}`;
    // A prefix route documents itself as `/v1/resources/...`; a sample calls a
    // concrete path under it, so compare on the stable prefix in both cases.
    const literalPrefix = route.path.split('{')[0].replace(/\.\.\.$/, '');
    assert.ok(sample.code.includes(literalPrefix), `${where} sample does not call the route it documents`);
    // `ANY` means the route accepts several methods, so a sample picks one.
    if (route.method !== 'GET' && route.method !== 'ANY') {
      assert.ok(
        sample.code.includes(`-X ${route.method}`) || sample.code.includes(`-sX ${route.method}`),
        `${where} sample does not use ${route.method}`,
      );
    }
    for (const response of route.responses || []) {
      if (!response.sample || !(response.contentType || '').includes('json')) continue;
      assert.doesNotThrow(
        () => JSON.parse(response.sample as string),
        `${where} has a ${response.status} sample that is not valid JSON`,
      );
    }
  }
});

test('cited tests and handlers exist', () => {
  const missing: string[] = [];
  for (const route of documented) {
    for (const path of [...(route.coveringTests || []), ...(route.evidence || [])]) {
      if (!existsSync(resolve(repositoryRoot, path))) missing.push(`${route.method} ${route.path} -> ${path}`);
    }
  }
  assert.deepEqual(missing, []);
});

test('an area page publishes every documented route in its area', () => {
  const apiSection = wikiSections.find(section => section.id === 'api');
  assert.ok(apiSection, 'the wiki must carry an API section');

  const published = new Map<string, string>();
  for (const article of apiSection.articles) {
    for (const route of article.apiRoutes || []) {
      published.set(`${route.method} ${route.path}`, article.id);
    }
  }

  for (const route of documented) {
    const key = `${route.method} ${route.path}`;
    assert.ok(published.has(key), `${key} is documented in full but no area page renders it`);
  }
});

test('every route is documented in depth or explicitly an alias', () => {
  // The surface is complete when no route is merely listed. A route without a
  // depth has to be a compatibility alias that names the form to prefer, so a
  // reader never lands on a bare row with nowhere to go.
  const undocumented: string[] = [];
  for (const route of apiRoutes) {
    if (route.depth) continue;
    const note = route.notes || '';
    if (!/same handler|compatibility|prefer/i.test(note)) {
      undocumented.push(`${route.method} ${route.path}`);
    }
  }
  assert.deepEqual(undocumented, []);

  // Aliases are a small, deliberate set rather than a growing backlog.
  const aliases = apiRoutes.filter(route => !route.depth);
  assert.ok(aliases.length <= 15, `expected at most 15 compatibility aliases, found ${aliases.length}`);
});
