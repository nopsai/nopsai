import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';
import {
  findBrokenRelatedLinks,
  findDuplicateArticleIDs,
  findWikiArticle,
  findWikiArticleByPath,
  getFirstWikiArticleID,
  getWikiNeighbors,
  summarizeWiki,
  findBrokenSupersededArticleIDs,
  findWikiArticleRedirect,
  wikiArticlePath,
  wikiSections,
} from './index.js';

test('every section carries a description and described articles', () => {
  const summary = summarizeWiki();

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
  const location = findWikiArticleByPath('/docs/pipelines/pipeline-anatomy');

  assert.equal(location?.section.id, 'pipelines');
  assert.equal(location?.article.title, 'Pipeline anatomy');
  assert.equal(wikiArticlePath('pipelines', 'pipeline-anatomy'), '/docs/pipelines/pipeline-anatomy');
  assert.equal(findWikiArticleByPath('/docs/getting-started/pipeline-anatomy'), undefined);
  assert.equal(findWikiArticleByPath('/docs'), undefined);
});

test('neighbours walk the reading order across section boundaries', () => {
  const last = wikiSections[0]?.articles.at(-1);
  const firstOfNext = wikiSections[1]?.articles[0];

  assert.ok(last && firstOfNext);
  assert.equal(getWikiNeighbors(last.id).next?.article.id, firstOfNext.id);
  assert.equal(getWikiNeighbors(getFirstWikiArticleID()).previous, undefined);
});

test('sections read in onboarding order and every one carries pages', () => {
  assert.deepEqual(
    wikiSections.map(section => section.id),
    ['getting-started', 'pipelines', 'automation', 'platform', 'operations', 'api', 'reference'],
  );
  assert.ok(wikiSections.every(section => section.articles.length > 0));
});

test('a bookmark from the previous section layout lands on the page it named', () => {
  // A page that moved between sections.
  assert.equal(findWikiArticleRedirect('/docs/start/concepts-glossary'), '/docs/reference/concepts-glossary');
  // A page replaced by a successor, from its old section and from its new one.
  assert.equal(findWikiArticleRedirect('/docs/automation/pipeline-schema'), '/docs/pipelines/pipeline-anatomy');
  assert.equal(findWikiArticleRedirect('/docs/pipelines/pipeline-schema'), '/docs/pipelines/pipeline-anatomy');
  assert.equal(findWikiArticleRedirect('/docs/pipelines/pipeline-anatomy'), undefined);
  assert.equal(findWikiArticleRedirect('/docs/automation/does-not-exist'), undefined);
  assert.deepEqual(findBrokenSupersededArticleIDs(), []);
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

test('the chapter documents the defaults that are easy to get wrong', () => {
  // Each directive now lives on the page that introduces it, so the defaults are
  // checked wherever they landed rather than on one combined schema page.
  const field = (articleID: string, path: string) =>
    findWikiArticle(articleID)?.article.fields?.find(candidate => candidate.path === path);

  assert.equal(findWikiArticle('pipeline-anatomy')?.article.docType, 'reference');
  assert.equal(field('pipeline-anatomy', 'version')?.defaultValue, 'latest');
  assert.equal(field('pipeline-anatomy', 'working_directory')?.defaultValue, '/workspace');
  assert.equal(field('pipeline-anatomy', 'container_image')?.required, 'conditional');
  assert.equal(field('pipeline-anatomy', 'name')?.required, true);
  assert.equal(field('pipeline-anatomy', 'steps')?.required, true);
  assert.equal(field('ai-context-and-tools', 'governance_level')?.defaultValue, 'strict');
  assert.equal(field('ai-context-and-tools', 'llm_content_preload')?.defaultValue, 'false');
  assert.equal(field('ai-steps', 'llm_enabled')?.defaultValue, 'true');
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

// The GitHub App page carries the two facts that are expensive to rediscover:
// why one App can serve many accounts at all, and what stops that from letting
// a stranger attach their organization to the installation.
test('the GitHub App page documents the installation model and the approval guard', () => {
  const article = findWikiArticle('github-app')?.article;
  assert.ok(article, 'the wiki needs a GitHub App page');

  const prose = [...article.keyFacts, ...article.details, ...(article.limits || [])].join(' ').toLowerCase();
  assert.ok(prose.includes('one github app per nopsai installation'), 'the one-App model must be stated');
  assert.ok(prose.includes('public'), 'why the App is public must be stated');
  assert.ok(prose.includes('pending approval') || prose.includes('pending_approval'));
  assert.ok(prose.includes('inert'), 'a held installation must be described as inert, not merely flagged');

  const field = (path: string) => article.fields?.find(candidate => candidate.path === path);
  assert.equal(field('github_app_owner')?.required, false);
  assert.equal(field('github_installations[].pending_approval')?.defaultValue, 'false');
  assert.equal(field('github_installations[].enabled')?.defaultValue, 'true');
  assert.ok(field('github_app_owner')?.security, 'the owner field decides trust and must say so');
  assert.ok(field('github_installations[].pending_approval')?.security);
});

test('the wiki does not claim unimplemented capabilities as current behavior', () => {
  const limits = findWikiArticle('known-limits')?.article.keyFacts.join(' ').toLowerCase() || '';

  assert.ok(limits.includes('terraform'));
  assert.ok(limits.includes('air-gap'));
  assert.ok(limits.includes('restore workflow'));
  assert.ok(limits.includes('object-storage'));
  assert.ok(limits.includes('networkpolicy'));
});

test('the onboarding path stays in order and every procedural page can be checked', () => {
  const gettingStarted = wikiSections.find(section => section.id === 'getting-started');
  assert.ok(gettingStarted, 'the wiki must open with a getting-started section');

  assert.deepEqual(
    gettingStarted.articles.map(article => article.id),
    [
      'what-nopsai-is',
      'requirements',
      'install-local-docker-compose',
      'complete-first-install-wizard',
      'architecture-and-networking',
      'add-docker-runner',
      'first-script-pipeline',
      'first-variables-and-secrets',
      'connect-git-repository',
      'trigger-pipeline-from-git',
      'first-external-trigger',
      'first-run-logs-history',
    ],
    'each onboarding page hands off to the next one, so the order is part of the contract',
  );

  // A reader following the path must be able to tell whether a step worked
  // before moving on, so a procedural onboarding page states what to expect.
  for (const article of gettingStarted.articles) {
    const steps = article.steps || [];
    if (steps.length === 0) continue;
    assert.ok((article.prerequisites?.length || 0) > 0, `${article.id} needs prerequisites`);
    assert.ok(
      steps.some(step => Boolean(step.verification) || Boolean(step.expectedOutput)),
      `${article.id} needs at least one step with a verification or an expected result`,
    );
  }
});

test('the pipeline chapter grows one manifest and introduces each directive once', () => {
  const chapter = wikiSections.find(section => section.id === 'pipelines');
  assert.ok(chapter, 'the wiki must carry a pipelines chapter');

  const manifests = chapter.articles.map(article => ({
    id: article.id,
    code: article.examples?.find(example => example.title === 'Pipeline so far')?.code,
  }));
  assert.ok(
    manifests.every(entry => Boolean(entry.code)),
    `every chapter page shows the running manifest: ${manifests.filter(entry => !entry.code).map(entry => entry.id).join(', ')}`,
  );

  // The reader follows one artefact, so a page may add lines to the manifest but
  // never quietly change or drop what an earlier page showed.
  const countLines = (code: string) => {
    const counts = new Map<string, number>();
    for (const line of code.split('\n')) {
      if (!line.trim()) continue;
      counts.set(line, (counts.get(line) || 0) + 1);
    }
    return counts;
  };
  for (let index = 1; index < manifests.length; index += 1) {
    const previous = countLines(manifests[index - 1].code || '');
    const current = countLines(manifests[index].code || '');
    for (const [line, count] of previous) {
      assert.ok(
        (current.get(line) || 0) >= count,
        `${manifests[index].id} dropped a line the previous page showed: ${line.trim()}`,
      );
    }
  }

  // A directive has exactly one home, and no page lists the same path twice: a
  // repeated path renders as a duplicate row and collides its anchor.
  const introduced = new Map<string, string>();
  for (const article of chapter.articles) {
    const seenOnPage = new Set<string>();
    for (const field of article.fields || []) {
      assert.ok(!seenOnPage.has(field.path), `${article.id} lists ${field.path} twice`);
      seenOnPage.add(field.path);
      const owner = introduced.get(field.path);
      assert.equal(owner, undefined, `${field.path} is introduced on two pages: ${owner} and ${article.id}`);
      introduced.set(field.path, article.id);
    }
  }
  // Every pipeline, step, task, and output directive keeps a home, which is what
  // keeps the generated directive index complete after the schema pages were split.
  assert.equal(introduced.size, 77, `expected all 77 manifest directives to be introduced, found ${introduced.size}`);
});

test('every cited repository path exists', () => {
  // Implementation evidence is only worth printing if a reader can open it. A
  // renamed file that leaves the citation behind is worse than no citation.
  const root = resolve(process.cwd(), '..', '..');
  const missing: string[] = [];
  for (const section of wikiSections) {
    for (const article of section.articles) {
      for (const source of article.sources || []) {
        if (!existsSync(resolve(root, source.repositoryPath))) {
          missing.push(`${article.id} -> ${source.repositoryPath}`);
        }
      }
      for (const field of article.fields || []) {
        if (field.evidence && !existsSync(resolve(root, field.evidence))) {
          missing.push(`${article.id} field ${field.path} -> ${field.evidence}`);
        }
      }
    }
  }
  assert.deepEqual(missing, []);
});

test('every page meets the authoring contract for its kind', () => {
  // Generated index articles render an aggregated table instead of authored
  // examples, and a handful of orientation pages are prose by design. Everything
  // else has to give the reader something they can actually run.
  const generatedIndexes = new Set(['directive-index', 'environment-index', 'api-index']);
  const orientationPages = new Set(['what-nopsai-is', 'concepts-glossary', 'known-limits']);

  const withoutRunnableExample: string[] = [];
  const proceduralGaps: string[] = [];

  for (const section of wikiSections) {
    for (const article of section.articles) {
      const steps = article.steps || [];
      // A page documenting only internal routes deliberately carries no runnable
      // example: calling those routes by hand corrupts state, so inviting it
      // would be worse documentation, not better.
      const routes = article.apiRoutes || [];
      const onlyInternalRoutes = routes.length > 0 && routes.every(route => route.depth === 'contract');
      const hasExample =
        (article.examples?.length || 0) > 0 ||
        steps.some(step => (step.commands?.length || 0) > 0) ||
        routes.some(route => Boolean(route.requestSample)) ||
        onlyInternalRoutes;
      if (!hasExample && !generatedIndexes.has(article.id) && !orientationPages.has(article.id)) {
        withoutRunnableExample.push(article.id);
      }

      if (article.docType !== 'how-to' && article.docType !== 'tutorial') continue;
      if ((article.prerequisites?.length || 0) === 0) proceduralGaps.push(`${article.id}: no prerequisites`);
      if (steps.length === 0) proceduralGaps.push(`${article.id}: no steps`);
      else if (!steps.some(step => Boolean(step.verification) || Boolean(step.expectedOutput))) {
        proceduralGaps.push(`${article.id}: no step states an expected result`);
      }
    }
  }

  assert.deepEqual(withoutRunnableExample, []);
  assert.deepEqual(proceduralGaps, []);
});

test('no page lists the same field twice', () => {
  // Two rows with the same path render as a duplicate and share an anchor, which
  // is how a deep link and an expanded row end up pointing at the wrong one.
  const duplicates: string[] = [];
  for (const section of wikiSections) {
    for (const article of section.articles) {
      // Scope is part of a field's identity: an MCP profile and an MCP server
      // both document a `name`, and those are two fields, not one repeated.
      const seen = new Set<string>();
      for (const field of article.fields || []) {
        const key = `${field.scope}::${field.path}`;
        if (seen.has(key)) duplicates.push(`${article.id}: ${field.path} (${field.scope})`);
        seen.add(key);
      }
      const routes = new Set<string>();
      for (const route of article.apiRoutes || []) {
        const key = `${route.method} ${route.path}`;
        if (routes.has(key)) duplicates.push(`${article.id}: ${key}`);
        routes.add(key);
      }
    }
  }
  assert.deepEqual(duplicates, []);
});

test('every inline article link resolves', () => {
  // Inline links name an article ID so a reorganisation cannot rot them, which
  // only holds if a broken ID fails here rather than rendering as plain text.
  const known = new Set(wikiSections.flatMap(section => section.articles.map(article => article.id)));
  const broken: string[] = [];

  const check = (articleID: string, text: string) => {
    for (const match of text.matchAll(/\[[^\]]+\]\(([a-z0-9-]+)\)/g)) {
      if (!known.has(match[1])) broken.push(`${articleID}: ${match[1]}`);
    }
  };

  for (const section of wikiSections) {
    for (const article of section.articles) {
      for (const fact of article.keyFacts) check(article.id, fact);
      for (const detail of article.details) check(article.id, detail);
      for (const step of article.steps || []) {
        check(article.id, step.description);
        if (step.expectedOutput) check(article.id, step.expectedOutput);
        if (step.verification) check(article.id, step.verification);
      }
      for (const limit of article.limits || []) check(article.id, limit);
    }
  }

  assert.deepEqual(broken, []);
});
