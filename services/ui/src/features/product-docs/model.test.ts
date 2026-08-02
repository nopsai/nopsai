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
  wikiMetadata,
  wikiSections,
} from './model.js';

test('summarizes the product wiki and keeps a stable first article', () => {
  const summary = summarizeWiki();

  assert.equal(summary.sections, 9);
  assert.equal(summary.articles, 46);
  assert.ok(summary.configKeys > 20);
  assert.ok(summary.runbooks > 20);
  assert.equal(summary.tutorials, 8);
  assert.ok(summary.proceduralPages >= 8);
  assert.ok(summary.sourceLinks > 30);
  assert.equal(getFirstWikiArticleID(), 'what-nopsai-is');
  assert.equal(findWikiArticle(wikiSections, 'assistant-chat')?.title, 'Assistant Chat');
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
  assert.ok(configKeys.includes('SERVICE_JWT_SIGNING_KEY'));
  assert.ok(configKeys.includes('DISPATCHER_TLS_MODE'));
  assert.ok(configKeys.includes('FINAL_OUTPUT_PDF_RENDERER_URL'));
  assert.ok(configKeys.includes('DISPATCHER_GRPC_ADDRESS'));
});

test('documents required deployment environment and service URL settings as first-class wiki content', () => {
  const article = findWikiArticle(wikiSections, 'required-envs-service-urls');
  assert.ok(article);
  assert.equal(article.docType, 'reference');
  assert.ok(article.audiences.includes('administrator'));
  assert.ok(article.keyFacts.some(fact => fact.includes('SERVICE_JWT_SIGNING_KEY')));
  assert.ok(article.configRows.some(row => row.key === 'SERVICE_JWT_SIGNING_KEY' && row.security?.includes('independent secret')));
  assert.ok(article.configRows.some(row => row.key === 'SYSTEM_LOGS_DOCKER_HOST' && row.security?.includes('docker-socket-proxy')));
  assert.ok(article.examples.some(example => example.title === 'Helm bootstrap Secret keys'));
});

test('documents private registry runner auth as an installed capability', () => {
  const article = findWikiArticle(wikiSections, 'private-registry-runner-auth');

  assert.ok(article);
  assert.equal(article.docType, 'reference');
  assert.ok(article.audiences.includes('security'));
  assert.ok(article.keyFacts.some(fact => fact.includes('docker_config_json')));
  assert.ok(article.keyFacts.some(fact => fact.includes('RegistryAuth')));
  assert.ok(article.configRows.some(row => row.key === 'runner_registry_credentials'));
  assert.ok(article.relatedDocs.includes('doc/runner-registry-auth.md'));
  assert.ok(filterWikiSections(wikiSections, 'imagePullSecrets').some(section => section.id === 'installation'));
});

test('documents SSO examples and the GitOps identity-provider boundary', () => {
  const article = findWikiArticle(wikiSections, 'auth-aaa-teams-scopes');

  assert.ok(article);
  assert.ok(article.keyFacts.some(fact => fact.includes('examples/sso/keycloak')));
  assert.ok(article.keyFacts.some(fact => fact.includes('setting/system/auth.yaml')));
  assert.ok(article.keyFacts.some(fact => fact.includes('runtime state')));
  assert.ok(article.configRows.some(row => row.key === 'examples/sso/idp-test-pack'));
  assert.ok(article.relatedDocs.includes('examples/sso/README.md'));
  assert.ok(article.sourceLinks.some(source => source.repositoryPath === 'examples/sso/keycloak/docker-compose.yaml'));
  assert.ok(article.sourceLinks.some(source => source.repositoryPath === 'examples/sso/idp-test-pack/test-matrix.yaml'));
  assert.ok(filterWikiSections(wikiSections, 'idp-test-pack').some(section => section.id === 'platform-admin'));
});

test('documents line-level GitOps drift review highlighting', () => {
  const article = findWikiArticle(wikiSections, 'gitops-authority');

  assert.ok(article);
  assert.ok(article.details.some(detail => detail.includes('removed Git lines') && detail.includes('added desired lines')));
  assert.ok(article.details.some(detail => detail.includes('synchronized scrolling')));
  assert.ok(filterWikiSections(wikiSections, 'synchronized scrolling').some(section => section.id === 'platform-admin'));
});

test('documents read-only analysis reviewers in the product wiki', () => {
  const article = findWikiArticle(wikiSections, 'analysis-reviewers');

  assert.ok(article);
  assert.equal(article.docType, 'how-to');
  assert.ok(article.audiences.includes('operator'));
  assert.ok(article.keyFacts.some(fact => fact.includes('Analyse Pipeline')));
  assert.ok(article.keyFacts.some(fact => fact.includes('Analyse Run')));
  assert.ok(article.keyFacts.some(fact => fact.includes('critical x 25')));
  assert.ok(article.keyFacts.some(fact => fact.includes('AI Evaluation')));
  assert.ok(article.keyFacts.some(fact => fact.includes('POST /v1/analysis/evaluate')));
  assert.ok(article.keyFacts.some(fact => fact.includes('Problem, Why This Score')));
  assert.ok(article.keyFacts.some(fact => fact.includes('credential values')));
  assert.ok(article.configRows.some(row => row.key === 'Analyse Resources' && row.security?.includes('redacts')));
  assert.ok(article.sourceLinks.some(source => source.repositoryPath === 'services/ui/src/features/analysis/model.ts'));
  assert.ok(article.sourceLinks.some(source => source.repositoryPath === 'services/ui/src/features/analysis/api.ts'));
  assert.ok(filterWikiSections(wikiSections, 'first failed execution point').some(section => section.id === 'operations'));
});

test('documents extension-owned browser console warnings', () => {
  const article = findWikiArticle(wikiSections, 'browser-console-troubleshooting');

  assert.ok(article);
  assert.equal(article.docType, 'troubleshooting');
  assert.ok(article.audiences.includes('operator'));
  assert.ok(article.audiences.includes('developer'));
  assert.ok(article.keyFacts.some(fact => fact.includes('ObjectMultiplex')));
  assert.ok(article.keyFacts.some(fact => fact.includes('AAA, GitOps, MCP')));
  assert.ok(article.steps.some(step => step.title === 'Reproduce without extensions'));
  assert.ok(article.sourceLinks.some(source => source.repositoryPath === 'doc/browser-console-troubleshooting.md'));
  assert.ok(article.runbookEntries.some(runbook => runbook.diagnosticCommands.some(command => command.includes('app-init-liveness'))));
  assert.ok(filterWikiSections(wikiSections, 'background-liveness').some(section => section.id === 'security-reference'));
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

test('documents examples as runnable source evidence', () => {
  assert.ok(wikiMetadata.sourceOrder.some(source => source.includes('examples/')));

  const knowledgeArticle = findWikiArticle(wikiSections, 'knowledge-context');
  assert.ok(knowledgeArticle);
  assert.ok(knowledgeArticle.relatedDocs.includes('examples/sample-config-repo/README.md'));
  assert.ok(filterWikiSections(wikiSections, 'examples/sample-config-repo').some(section => section.id === 'ai-mcp-knowledge'));
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
