import { apiSection } from './sections/api.js';
import { automationSection } from './sections/automation.js';
import { gettingStartedSection } from './sections/getting-started.js';
import { operationsSection } from './sections/operations.js';
import { pipelinesSection } from './sections/pipelines.js';
import { platformSection } from './sections/platform.js';
import { referenceSection } from './sections/reference.js';
import { wikiArticlePath, type WikiArticle, type WikiSection } from './types.js';

export * from './types.js';

export const wikiMetadata = {
  title: 'NopsAI wiki',
  tagline: 'Everything the platform does, grounded in the code that implements it.',
  status: 'Current implementation',
  sourceOrder: [
    'Runtime code, schemas, and route behavior',
    'Docker Compose and Helm deployment files',
    'Focused markdown under doc/',
    'Runnable artifacts under examples/',
    'Root README examples',
    'UI help text only when it agrees with implementation',
  ],
};

/**
 * Section order is the reading order in the sidebar and on the home page.
 *
 * Sections are named for what the reader is doing, and each one is a nav group:
 * onboarding first, then the feature chapters, then the lookup material.
 */
export const wikiSections: WikiSection[] = [
  gettingStartedSection,
  pipelinesSection,
  automationSection,
  platformSection,
  operationsSection,
  apiSection,
  referenceSection,
];

export type WikiArticleLocation = { section: WikiSection; article: WikiArticle };

const articleIndex = new Map<string, WikiArticleLocation>();
for (const section of wikiSections) {
  for (const article of section.articles) {
    articleIndex.set(article.id, { section, article });
  }
}

export const wikiArticleLocations: WikiArticleLocation[] = Array.from(articleIndex.values());

export function findWikiArticle(articleID: string) {
  return articleIndex.get(articleID);
}

export function findWikiSection(sectionID: string) {
  return wikiSections.find(section => section.id === sectionID);
}

export function findWikiArticleByPath(pathname: string) {
  const segments = pathname.split('/').filter(Boolean);
  if (segments[0] !== 'docs' || segments.length < 3) return undefined;
  const sectionID = decodeRouteSegment(segments[1] || '');
  const articleID = decodeRouteSegment(segments[2] || '');
  const location = articleIndex.get(articleID);
  return location && location.section.id === sectionID ? location : undefined;
}

/**
 * Pages that were replaced by a successor covering the same ground.
 *
 * The Pipelines chapter split the two large schema references into one page per
 * capability and folded the standalone tutorials into the page that owns the
 * feature. A reader arriving on an old link should land on the successor rather
 * than on the home page.
 */
export const wikiSupersededArticleIDs: Record<string, string> = {
  'pipeline-schema': 'pipeline-anatomy',
  'step-task-directives': 'script-steps',
  'runtime-outputs': 'step-outputs',
  'variables-secrets-scopes': 'pipeline-variables',
  'add-approval-checkpoint': 'approvals',
  'first-ai-assisted-pipeline': 'ai-steps',
  'create-final-deliverable': 'final-deliverables',
  'ai-control-layers': 'ai-context-and-tools',
};

/**
 * Canonical path for a stale `/docs/<section>/<article>` URL.
 *
 * Two things go stale: a page moves between sections, and a page is replaced by
 * a successor. Both resolve by article ID, so an old bookmark still lands on the
 * material it named.
 */
export function findWikiArticleRedirect(pathname: string) {
  const segments = pathname.split('/').filter(Boolean);
  if (segments[0] !== 'docs' || segments.length < 3) return undefined;
  const sectionID = decodeRouteSegment(segments[1] || '');
  const requestedID = decodeRouteSegment(segments[2] || '');
  const articleID = wikiSupersededArticleIDs[requestedID] || requestedID;
  const location = articleIndex.get(articleID);
  if (!location || (location.section.id === sectionID && location.article.id === requestedID)) return undefined;
  return wikiArticlePath(location.section.id, location.article.id);
}

/** Superseded IDs that no longer resolve. Must be empty: a dead alias is a dead link. */
export function findBrokenSupersededArticleIDs(sections: WikiSection[] = wikiSections) {
  const known = new Set(sections.flatMap(section => section.articles.map(article => article.id)));
  return Object.entries(wikiSupersededArticleIDs)
    .filter(([oldID, newID]) => !known.has(newID) || known.has(oldID))
    .map(([oldID, newID]) => ({ oldID, newID }));
}

export function getWikiNeighbors(articleID: string) {
  const index = wikiArticleLocations.findIndex(location => location.article.id === articleID);
  return {
    previous: index > 0 ? wikiArticleLocations[index - 1] : undefined,
    next: index >= 0 && index < wikiArticleLocations.length - 1 ? wikiArticleLocations[index + 1] : undefined,
  };
}

export function getFirstWikiArticleID() {
  return wikiSections[0]?.articles[0]?.id || '';
}

export type WikiSummary = {
  sections: number;
  articles: number;
  fields: number;
  examples: number;
  runbooks: number;
  sources: number;
  tutorials: number;
  proceduralPages: number;
};

export function summarizeWiki(sections: WikiSection[] = wikiSections): WikiSummary {
  const articles = sections.flatMap(section => section.articles);
  const count = <T>(pick: (article: WikiArticle) => T[] | undefined) =>
    articles.reduce((total, article) => total + (pick(article)?.length || 0), 0);

  return {
    sections: sections.length,
    articles: articles.length,
    fields: count(article => article.fields),
    examples: count(article => article.examples),
    runbooks: count(article => article.runbooks),
    sources: count(article => article.sources),
    tutorials: articles.filter(article => article.docType === 'tutorial').length,
    proceduralPages: articles.filter(article => (article.steps?.length || 0) > 0).length,
  };
}

/**
 * Article IDs referenced by `related` that do not resolve.
 *
 * Exported so a test can assert this is empty: a broken cross-link is a silent
 * dead end for a reader, and nothing else would catch it.
 */
export function findBrokenRelatedLinks(sections: WikiSection[] = wikiSections) {
  const known = new Set(sections.flatMap(section => section.articles.map(article => article.id)));
  const broken: { articleID: string; relatedID: string }[] = [];
  for (const section of sections) {
    for (const article of section.articles) {
      for (const relatedID of article.related || []) {
        if (!known.has(relatedID)) broken.push({ articleID: article.id, relatedID });
      }
    }
  }
  return broken;
}

/** Article IDs that appear in more than one section. Must be empty for routing to be unambiguous. */
export function findDuplicateArticleIDs(sections: WikiSection[] = wikiSections) {
  const seen = new Set<string>();
  const duplicates = new Set<string>();
  for (const section of sections) {
    for (const article of section.articles) {
      if (seen.has(article.id)) duplicates.add(article.id);
      seen.add(article.id);
    }
  }
  return Array.from(duplicates);
}

function decodeRouteSegment(segment: string) {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}
