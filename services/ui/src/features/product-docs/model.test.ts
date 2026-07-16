import assert from 'node:assert/strict';
import test from 'node:test';
import {
  collectWikiConfigKeys,
  countWikiArticles,
  filterWikiSections,
  findWikiArticle,
  findWikiArticleByPath,
  getFirstWikiArticleID,
  summarizeWiki,
  unsupportedClaims,
  wikiArticlePath,
  wikiSections,
} from './model.js';

test('summarizes the product wiki and keeps a stable first article', () => {
  const summary = summarizeWiki();

  assert.equal(summary.sections, 9);
  assert.equal(summary.articles, 41);
  assert.ok(summary.configKeys > 20);
  assert.ok(summary.runbooks > 20);
  assert.equal(summary.tutorials, 8);
  assert.ok(summary.proceduralPages >= 8);
  assert.ok(summary.sourceLinks > 30);
  assert.equal(getFirstWikiArticleID(), 'what-nopsai-is');
});

test('searches article body, examples, config rows, and known limitations', () => {
  const gotenbergMatches = filterWikiSections(wikiSections, 'Gotenberg');
  const gotenbergArticleIDs = gotenbergMatches.flatMap(section => section.articles.map(article => article.id));

  assert.equal(findWikiArticle(wikiSections, 'docker-compose')?.title, 'Docker Compose');
  assert.ok(countWikiArticles(gotenbergMatches) >= 4);
  assert.ok(gotenbergArticleIDs.includes('final-deliverables'));
  assert.deepEqual(filterWikiSections(wikiSections, 'air-gap').map(section => section.id), [
    'platform-admin',
    'security-reference',
  ]);
  assert.ok(unsupportedClaims.some(claim => claim.includes('air-gap')));
});

test('does not expose removed aspirational deployment config keys', () => {
  const configKeys = collectWikiConfigKeys();

  assert.equal(configKeys.includes('REDIS_URL'), false);
  assert.equal(configKeys.includes('NOPS_STORAGE_BACKEND'), false);
  assert.equal(configKeys.includes('POSTGRES_DSN'), false);
  assert.equal(configKeys.includes('NOPS_ENV'), false);
  assert.equal(configKeys.includes('NOPS_PUBLIC_URL'), false);
  assert.ok(configKeys.includes('DATABASE_URL'));
  assert.ok(configKeys.includes('DISPATCHER_GRPC_ADDRESS'));
});

test('documents step-level LLM profile directives', () => {
  const article = findWikiArticle(wikiSections, 'step-task-directives');
  assert.ok(article);
  assert.equal(article.docType, 'reference');
  assert.ok(article.audiences.includes('automation-author'));
  assert.ok(article.keyFacts.some(fact => fact.includes('steps[].llm_profile')));
  assert.ok(article.configRows.some(row => row.key === 'steps[].llm_profile' && row.description.includes('provider/model') && row.path === 'steps[].llm_profile'));
  assert.ok(article.configRows.some(row => row.key === 'tasks[].llm_profile' && row.required === 'conditional'));
});

test('models getting-started tutorials as route-backed procedural documentation', () => {
  const path = wikiArticlePath('getting-started', 'first-script-pipeline');
  const selection = findWikiArticleByPath(wikiSections, path);

  assert.equal(path, '/docs/getting-started/first-script-pipeline');
  assert.equal(selection?.section.id, 'getting-started');
  assert.equal(selection?.article.docType, 'tutorial');
  assert.ok(selection?.article.audiences.includes('automation-author'));
  assert.ok(selection?.article.prerequisites.some(item => item.label === 'Permission'));
  assert.ok(selection?.article.steps.some(step => step.title === 'Inspect logs'));
  assert.ok(selection?.article.sourceLinks.some(source => source.repositoryPath === 'doc/runtime-flows.md'));
});
