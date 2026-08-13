import type { WikiArticle } from './content/index.js';

/** Sections an article page can render, in the order they may appear. */
export type WikiBlockID =
  | 'key-facts'
  | 'prerequisites'
  | 'procedure'
  | 'details'
  | 'fields'
  | 'examples'
  | 'runbooks'
  | 'limits'
  | 'related'
  | 'sources';

const blockTitles: Record<WikiBlockID, string> = {
  'key-facts': 'Key points',
  prerequisites: 'Before you start',
  procedure: 'Steps',
  details: 'How it works',
  fields: 'Field reference',
  examples: 'Examples',
  runbooks: 'Runbooks',
  limits: 'Limits',
  related: 'Related pages',
  sources: 'Implementation evidence',
};

/**
 * Block order follows the page purpose: a tutorial leads with steps, a
 * reference leads with the field table, a troubleshooting page leads with the
 * runbook. A single fixed order would bury the thing each reader came for.
 */
export function wikiBlockOrder(article: WikiArticle): WikiBlockID[] {
  const tail: WikiBlockID[] = ['limits', 'related', 'sources'];
  if (article.docType === 'tutorial' || article.docType === 'how-to') {
    return ['key-facts', 'prerequisites', 'procedure', 'details', 'fields', 'examples', 'runbooks', ...tail];
  }
  if (article.docType === 'reference') {
    return ['key-facts', 'fields', 'examples', 'details', 'runbooks', ...tail];
  }
  if (article.docType === 'runbook' || article.docType === 'troubleshooting') {
    return ['key-facts', 'runbooks', 'procedure', 'details', 'fields', 'examples', ...tail];
  }
  return ['key-facts', 'details', 'fields', 'examples', 'runbooks', ...tail];
}

/** Only blocks the article actually carries. Empty blocks are never rendered as placeholders. */
export function visibleWikiBlocks(article: WikiArticle): WikiBlockID[] {
  const present: Record<WikiBlockID, boolean> = {
    'key-facts': article.keyFacts.length > 0,
    prerequisites: (article.prerequisites?.length || 0) > 0,
    procedure: (article.steps?.length || 0) > 0,
    details: article.details.length > 0,
    fields: (article.fields?.length || 0) > 0,
    examples: (article.examples?.length || 0) > 0,
    runbooks: (article.runbooks?.length || 0) > 0,
    limits: (article.limits?.length || 0) > 0,
    related: (article.related?.length || 0) > 0,
    sources: (article.sources?.length || 0) > 0,
  };
  return wikiBlockOrder(article).filter(block => present[block]);
}

export function wikiBlockTitle(block: WikiBlockID, article?: WikiArticle) {
  if (block === 'key-facts' && article?.docType === 'tutorial') return 'What you will do';
  return blockTitles[block];
}
