import {
  wikiSections,
  type WikiArticle,
  type WikiConfigRow,
  type WikiRunbook,
  type WikiSection,
  type WikiSource,
} from './model.js';

export const documentationMetadata = {
  title: 'NopsAI Documentation',
  status: 'Current implementation, grounded in repository sources',
  repositoryBranch: 'main',
};

export type DocumentationField = WikiConfigRow & {
  anchor: string;
  metadataStatus: 'verified' | 'partial';
  displayType: string;
  displayRequired: string;
  displayDefault: string;
};

export type DocumentationRunbook = WikiRunbook & { complete: boolean };

export type DocumentationArticle = Omit<WikiArticle, 'configRows' | 'sourceLinks' | 'runbookEntries'> & {
  configRows: DocumentationField[];
  sourceLinks: WikiSource[];
  runbookEntries: DocumentationRunbook[];
};

export type DocumentationSection = Omit<WikiSection, 'articles'> & {
  articles: DocumentationArticle[];
};

export const documentationSections: DocumentationSection[] = wikiSections.map(section => ({
  ...section,
  articles: section.articles.map(normalizeArticle),
}));

export function normalizeArticle(article: WikiArticle): DocumentationArticle {
  return {
    ...article,
    configRows: uniquifyFieldAnchors(article.configRows.map(normalizeField)),
    sourceLinks: article.sourceLinks.map(normalizeSource),
    runbookEntries: article.runbookEntries.map(normalizeRunbook),
    metadata: {
      ...article.metadata,
      introducedIn: article.metadata.introducedIn === 'current' ? 'Not recorded' : article.metadata.introducedIn,
      lastVerified: /^\d{4}-\d{2}-\d{2}$/.test(article.metadata.lastVerified) ? article.metadata.lastVerified : 'Not recorded',
      sourceCommit: documentationMetadata.repositoryBranch,
    },
  };
}

function uniquifyFieldAnchors(fields: DocumentationField[]): DocumentationField[] {
  const counts = new Map<string, number>();
  return fields.map(field => {
    const count = counts.get(field.anchor) || 0;
    counts.set(field.anchor, count + 1);
    if (count === 0) return field;
    return { ...field, anchor: `${field.anchor}-${count + 1}` };
  });
}

export function normalizeField(row: WikiConfigRow): DocumentationField {
  const metadataStatus = isFieldMetadataVerified(row) ? 'verified' : 'partial';
  return {
    ...row,
    anchor: `field-${slugify(row.path || row.key)}`,
    metadataStatus,
    displayType: metadataStatus === 'verified' ? row.type || 'Not documented' : 'Not documented',
    displayRequired: metadataStatus === 'verified' ? formatRequired(row.required) : 'Not documented',
    displayDefault: metadataStatus === 'verified' ? row.defaultValue || 'None' : 'Not documented',
  };
}

export function isFieldMetadataVerified(row: WikiConfigRow) {
  return Boolean(
    row.allowedValues?.length || row.constraints?.length || row.inheritedFrom?.length ||
    row.permission || row.introducedIn || row.deprecatedIn || row.security,
  );
}

export function normalizeSource(source: WikiSource): WikiSource {
  return {
    ...source,
    sourceUrl: undefined,
  };
}

export function normalizeRunbook(runbook: WikiRunbook): DocumentationRunbook {
  return {
    ...runbook,
    complete: runbook.diagnosticCommands.length > 0 ||
      runbook.resolution.some(item => !item.startsWith('Follow the article details, inspect the listed implementation evidence')),
  };
}

export function findDocumentationArticleByPath(pathname: string) {
  const segments = pathname.split('/').filter(Boolean);
  if (segments[0] !== 'docs' || segments.length < 3) return undefined;
  const section = documentationSections.find(candidate => candidate.id === decodeRouteSegment(segments[1] || ''));
  const article = section?.articles.find(candidate => candidate.id === decodeRouteSegment(segments[2] || ''));
  return section && article ? { section, article } : undefined;
}

export function findDocumentationSectionForArticle(articleID: string) {
  return documentationSections.find(section => section.articles.some(article => article.id === articleID));
}

export function findDocumentationArticle(articleID: string) {
  for (const section of documentationSections) {
    const article = section.articles.find(candidate => candidate.id === articleID);
    if (article) return article;
  }
  return undefined;
}

export function getDocumentationNeighbors(articleID: string) {
  const flattened = documentationSections.flatMap(section => section.articles.map(article => ({ section, article })));
  const index = flattened.findIndex(item => item.article.id === articleID);
  return {
    previous: index > 0 ? flattened[index - 1] : undefined,
    next: index >= 0 && index < flattened.length - 1 ? flattened[index + 1] : undefined,
  };
}

export function getArticleSectionOrder(article: DocumentationArticle) {
  const tail = ['examples', 'operations', 'boundaries', 'sources'];
  if (article.docType === 'tutorial' || article.docType === 'how-to') {
    return ['prerequisites', 'procedure', 'key-facts', 'details', 'configuration', ...tail];
  }
  if (article.docType === 'troubleshooting' || article.docType === 'runbook') {
    return ['operations', 'key-facts', 'details', 'configuration', 'examples', 'boundaries', 'sources'];
  }
  if (article.docType === 'reference') {
    return ['key-facts', 'configuration', 'examples', 'details', 'operations', 'boundaries', 'sources'];
  }
  return ['key-facts', 'details', 'examples', 'configuration', 'operations', 'boundaries', 'sources'];
}

export function slugify(value: string) {
  return value.trim().toLowerCase().replace(/\[\]/g, '').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
}

function formatRequired(value: WikiConfigRow['required']) {
  if (value === true) return 'Yes';
  if (value === false) return 'No';
  return 'Conditional';
}

function decodeRouteSegment(segment: string) {
  try { return decodeURIComponent(segment); } catch { return segment; }
}
