import { administrationSection } from './sections/administration.js';
import { aiSection } from './sections/ai.js';
import { automationSection } from './sections/automation.js';
import { eventsSection } from './sections/events.js';
import { getStartedSection } from './sections/get-started.js';
import { installSection } from './sections/install.js';
import { interfacesSection } from './sections/interfaces.js';
import { operationsSection } from './sections/operations.js';
import { referenceSection } from './sections/reference.js';
import { startSection } from './sections/start.js';
import { wikiGroupOrder, type WikiArticle, type WikiGroup, type WikiSection } from './types.js';

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

/** Section order is the reading order in the sidebar and on the home page. */
export const wikiSections: WikiSection[] = [
  startSection,
  getStartedSection,
  automationSection,
  eventsSection,
  aiSection,
  installSection,
  administrationSection,
  operationsSection,
  interfacesSection,
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

export function wikiSectionsByGroup(group: WikiGroup) {
  return wikiSections.filter(section => section.group === group);
}

export function wikiGroupedSections() {
  return wikiGroupOrder
    .map(group => ({ group, sections: wikiSectionsByGroup(group) }))
    .filter(entry => entry.sections.length > 0);
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
  groups: number;
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
    groups: new Set(sections.map(section => section.group)).size,
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
