import { apiRouteIndex, directiveIndex, environmentIndex } from './indexes.js';
import { findWikiArticle, wikiArticleLocations, wikiArticlePath, type WikiArticle } from './content/index.js';

export type WikiSearchKind = 'article' | 'directive' | 'environment' | 'endpoint' | 'example' | 'runbook';

export type WikiSearchResult = {
  id: string;
  kind: WikiSearchKind;
  title: string;
  /** Where the hit lives, shown under the title. */
  context: string;
  href: string;
  score: number;
};

export const wikiSearchKindLabels: Record<WikiSearchKind, string> = {
  article: 'Page',
  directive: 'Directive',
  environment: 'Env var',
  endpoint: 'Endpoint',
  example: 'Example',
  runbook: 'Runbook',
};

/**
 * Every searchable string in the wiki.
 *
 * Article records deliberately index the full body — key facts, details, limits
 * and keywords — because a reader searching "fails open" or "ejected runner" is
 * looking for a sentence, not a title.
 */
function buildSearchIndex(): WikiSearchResult[] {
  const records: WikiSearchResult[] = [];

  for (const { section, article } of wikiArticleLocations) {
    const href = wikiArticlePath(section.id, article.id);
    records.push({
      id: `article:${article.id}`,
      kind: 'article',
      title: article.title,
      context: `${section.title} · ${article.summary}`,
      href,
      score: 0,
    });

    for (const [index, example] of (article.examples || []).entries()) {
      records.push({
        id: `example:${article.id}:${index}`,
        kind: 'example',
        title: example.title,
        context: `${article.title} · ${example.language} example`,
        href: `${href}#examples`,
        score: 0,
      });
    }

    for (const runbook of article.runbooks || []) {
      records.push({
        id: `runbook:${article.id}:${runbook.id}`,
        kind: 'runbook',
        title: runbook.title,
        context: `${article.title} · ${runbook.impact}`,
        href: `${href}#runbooks`,
        score: 0,
      });
    }
  }

  for (const row of directiveIndex) {
    records.push({
      id: `directive:${row.scope}:${row.path}`,
      kind: 'directive',
      title: row.path,
      context: `${row.scope} · ${row.description}`,
      href: row.href,
      score: 0,
    });
  }

  for (const row of environmentIndex) {
    records.push({
      id: `env:${row.path}`,
      kind: 'environment',
      title: row.path,
      context: `${row.scope} · ${row.description}`,
      href: row.href,
      score: 0,
    });
  }

  // Endpoint hits land on the API index wherever that page currently lives.
  const apiIndexLocation = findWikiArticle('api-index');
  const apiIndexHref = apiIndexLocation
    ? wikiArticlePath(apiIndexLocation.section.id, apiIndexLocation.article.id)
    : '';

  for (const route of apiRouteIndex) {
    records.push({
      id: `endpoint:${route.method}:${route.path}`,
      kind: 'endpoint',
      title: `${route.method} ${route.path}`,
      context: `${route.area} · ${route.purpose}`,
      href: apiIndexHref,
      score: 0,
    });
  }

  return records;
}

/** Full body text per article, used to match prose without putting it in the result list. */
function buildArticleBodies() {
  const bodies = new Map<string, string>();
  for (const { article } of wikiArticleLocations) {
    bodies.set(`article:${article.id}`, articleBody(article).toLowerCase());
  }
  return bodies;
}

function articleBody(article: WikiArticle) {
  return [
    article.summary,
    ...(article.keywords || []),
    ...article.keyFacts,
    ...article.details,
    ...(article.limits || []),
    ...(article.prerequisites || []).map(item => `${item.label} ${item.value}`),
    ...(article.steps || []).map(step => `${step.title} ${step.description}`),
    ...(article.fields || []).map(field => `${field.path} ${field.description}`),
  ].join(' ');
}

const searchIndex = buildSearchIndex();
const articleBodies = buildArticleBodies();

export function searchWiki(query: string, index = searchIndex, limit = 14): WikiSearchResult[] {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return [];
  const tokens = normalized.split(/\s+/).filter(Boolean);

  return index
    .map(record => ({ ...record, score: scoreRecord(record, normalized, tokens) }))
    .filter(record => record.score > 0)
    .sort((left, right) => right.score - left.score || left.title.localeCompare(right.title))
    .slice(0, limit);
}

function scoreRecord(record: WikiSearchResult, query: string, tokens: string[]) {
  const title = record.title.toLowerCase();
  const context = record.context.toLowerCase();
  const exactBoost = record.kind === 'directive' || record.kind === 'environment' ? 160 : 110;

  let value = 0;
  if (title === query) value += exactBoost;
  else if (title.startsWith(query)) value += 80;
  else if (title.includes(query)) value += 55;
  if (context.includes(query)) value += 25;

  for (const token of tokens) {
    if (title === token) value += 24;
    else if (title.includes(token)) value += 14;
    if (context.includes(token)) value += 6;
  }

  // Prose match: lower weight, but it is what makes "fails open" findable.
  const body = articleBodies.get(record.id);
  if (body) {
    if (body.includes(query)) value += 18;
    for (const token of tokens) {
      if (body.includes(token)) value += 3;
    }
  }

  if (record.kind === 'directive' || record.kind === 'environment') value += 6;
  if (record.kind === 'article') value += 4;
  return value;
}
