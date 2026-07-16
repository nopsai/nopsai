import assert from 'node:assert/strict';
import test from 'node:test';
import {
  documentationMetadata,
  documentationSections,
  findDocumentationArticle,
  normalizeField,
} from './quality.js';
import { searchDocumentation } from './search.js';

test('uses product documentation naming and internal implementation evidence', () => {
  assert.equal(documentationMetadata.title, 'NopsAI Documentation');
  assert.ok(documentationSections.length > 0);
  for (const section of documentationSections) {
    for (const article of section.articles) {
      assert.equal(article.metadata.sourceCommit, 'main');
      for (const source of article.sourceLinks) {
        assert.equal(source.sourceUrl, undefined);
        assert.ok(source.repositoryPath.length > 0);
      }
    }
  }
});

test('does not present inferred field metadata as verified', () => {
  const field = normalizeField({
    key: 'example_field',
    area: 'Example',
    description: 'Example field without explicit schema metadata.',
    example: 'true',
    type: 'boolean',
    required: 'conditional',
    scope: 'Example',
  });
  assert.equal(field.metadataStatus, 'partial');
  assert.equal(field.displayType, 'Not documented');
  assert.equal(field.displayRequired, 'Not documented');
  assert.equal(field.displayDefault, 'Not documented');
});

test('keeps explicitly constrained field metadata visible', () => {
  const field = normalizeField({
    key: 'mode',
    area: 'Output',
    description: 'Publication mode.',
    example: 'replace',
    path: 'dashboard.mode',
    type: 'string',
    required: false,
    defaultValue: 'replace',
    allowedValues: ['replace', 'append'],
  });
  assert.equal(field.metadataStatus, 'verified');
  assert.equal(field.displayType, 'string');
  assert.equal(field.displayRequired, 'No');
  assert.equal(field.displayDefault, 'replace');
  assert.equal(field.anchor, 'field-dashboard-mode');
});

test('ranks exact field matches ahead of general article matches', () => {
  const results = searchDocumentation('steps[].llm_profile');
  assert.ok(results.length > 0);
  assert.equal(results[0]?.kind, 'field');
  assert.equal(results[0]?.title, 'steps[].llm_profile');
  assert.ok(results[0]?.href.endsWith('#field-steps-llm-profile'));
});

test('marks generated placeholder runbooks as incomplete', () => {
  const article = findDocumentationArticle('troubleshooting');
  assert.ok(article);
  assert.ok(article.runbookEntries.length > 0);
  assert.ok(article.runbookEntries.every(runbook => runbook.complete === false));
});

test('provides unique field anchors within every article', () => {
  for (const section of documentationSections) {
    for (const article of section.articles) {
      const anchors = article.configRows.map(field => field.anchor);
      assert.equal(new Set(anchors).size, anchors.length, `${article.id} contains duplicate field anchors`);
    }
  }
});
