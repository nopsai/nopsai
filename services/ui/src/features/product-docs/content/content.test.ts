import assert from 'node:assert/strict';
import test from 'node:test';
import {
  findBrokenRelatedLinks,
  findDuplicateArticleIDs,
  findWikiArticle,
  findWikiArticleByPath,
  getFirstWikiArticleID,
  getWikiNeighbors,
  summarizeWiki,
  wikiArticlePath,
  wikiGroupedSections,
  wikiSections,
} from './index.js';

test('every section belongs to a group and carries described articles', () => {
  const summary = summarizeWiki();

  assert.equal(summary.groups, 4);
  assert.equal(summary.sections, wikiSections.length);
  assert.ok(summary.articles >= 50, `expected at least 50 articles, found ${summary.articles}`);
  assert.ok(summary.fields >= 200, `expected at least 200 documented fields, found ${summary.fields}`);
  assert.equal(getFirstWikiArticleID(), 'what-nopsai-is');

  for (const section of wikiSections) {
    assert.ok(section.description.length > 0, `${section.id} needs a description`);
    assert.ok(section.articles.length > 0, `${section.id} has no articles`);
    for (const article of section.articles) {
      assert.ok(article.summary.length > 0, `${article.id} needs a summary`);
      assert.ok(article.keyFacts.length > 0, `${article.id} needs key facts`);
    }
  }
});

test('article ids are unique and every related link resolves', () => {
  assert.deepEqual(findDuplicateArticleIDs(), []);
  assert.deepEqual(findBrokenRelatedLinks(), []);
});

test('routes resolve to the owning section only', () => {
  const location = findWikiArticleByPath('/docs/automation/pipeline-schema');

  assert.equal(location?.section.id, 'automation');
  assert.equal(location?.article.title, 'Pipeline YAML schema');
  assert.equal(wikiArticlePath('automation', 'pipeline-schema'), '/docs/automation/pipeline-schema');
  assert.equal(findWikiArticleByPath('/docs/start/pipeline-schema'), undefined);
  assert.equal(findWikiArticleByPath('/docs'), undefined);
});

test('neighbours walk the reading order across section boundaries', () => {
  const last = wikiSections[0]?.articles.at(-1);
  const firstOfNext = wikiSections[1]?.articles[0];

  assert.ok(last && firstOfNext);
  assert.equal(getWikiNeighbors(last.id).next?.article.id, firstOfNext.id);
  assert.equal(getWikiNeighbors(getFirstWikiArticleID()).previous, undefined);
});

test('groups render in reading order with sections attached', () => {
  const groups = wikiGroupedSections();

  assert.deepEqual(groups.map(entry => entry.group), ['learn', 'build', 'run', 'look-up']);
  assert.ok(groups.every(entry => entry.sections.length > 0));
});

test('every documented field states type, requirement, default, and an example', () => {
  for (const section of wikiSections) {
    for (const article of section.articles) {
      for (const field of article.fields || []) {
        const where = `${article.id} · ${field.path}`;
        assert.ok(field.type.length > 0, `${where} is missing a type`);
        assert.ok(field.defaultValue.length > 0, `${where} is missing a default`);
        assert.ok(field.description.length > 0, `${where} is missing a description`);
        assert.ok(field.example.length > 0, `${where} is missing an example`);
        assert.ok(field.scope.length > 0, `${where} is missing a scope`);
        assert.notEqual(field.type, 'Not documented', `${where} must not be a placeholder`);
        assert.notEqual(field.defaultValue, 'Not documented', `${where} must not be a placeholder`);
      }
    }
  }
});

test('every published runbook carries diagnostics and resolution steps', () => {
  for (const section of wikiSections) {
    for (const article of section.articles) {
      for (const runbook of article.runbooks || []) {
        const where = `${article.id} · ${runbook.id}`;
        assert.ok(runbook.symptoms.length > 0, `${where} needs symptoms`);
        assert.ok(runbook.initialChecks.length > 0, `${where} needs initial checks`);
        assert.ok(runbook.diagnostics.length > 0, `${where} needs diagnostics`);
        assert.ok(runbook.resolution.length > 0, `${where} needs resolution steps`);
        assert.ok(runbook.requiredAccess.length > 0, `${where} needs required access`);
      }
    }
  }
});

test('pipeline schema documents the defaults that are easy to get wrong', () => {
  const article = findWikiArticle('pipeline-schema')?.article;
  const field = (path: string) => article?.fields?.find(candidate => candidate.path === path);

  assert.equal(article?.docType, 'reference');
  assert.equal(field('governance_level')?.defaultValue, 'strict');
  assert.equal(field('version')?.defaultValue, 'latest');
  assert.equal(field('working_directory')?.defaultValue, '/workspace');
  assert.equal(field('llm_enabled')?.defaultValue, 'true');
  assert.equal(field('llm_content_sharing')?.defaultValue, 'false');
  assert.equal(field('container_image')?.required, 'conditional');
  assert.equal(field('name')?.required, true);
  assert.equal(field('steps')?.required, true);
});

test('allowed values match the validator rather than prose', () => {
  const outputs = findWikiArticle('final-deliverables')?.article.fields || [];
  const knowledge = findWikiArticle('knowledge-context')?.article.fields || [];
  const values = (fields: typeof outputs, path: string) =>
    fields.find(field => field.path === path)?.allowedValues || [];

  assert.deepEqual(values(outputs, 'output.items[].type'), ['markdown', 'pdf', 'excel', 'json', 'html', 'dashboard']);
  assert.deepEqual(values(outputs, 'output.items[].when'), ['always', 'success', 'failure']);
  assert.deepEqual(values(outputs, 'output.items[].dashboard.mode'), ['replace', 'append', 'snapshot', 'series']);
  assert.deepEqual(values(knowledge, 'knowledge_context[].kind'), [
    'architecture',
    'guardrail',
    'policy',
    'adr',
    'guideline',
    'runbook',
    'reference',
    'example',
  ]);
});

test('the wiki does not claim unimplemented capabilities as current behavior', () => {
  const limits = findWikiArticle('known-limits')?.article.keyFacts.join(' ').toLowerCase() || '';

  assert.ok(limits.includes('terraform'));
  assert.ok(limits.includes('air-gap'));
  assert.ok(limits.includes('restore workflow'));
  assert.ok(limits.includes('object-storage'));
  assert.ok(limits.includes('networkpolicy'));
});
