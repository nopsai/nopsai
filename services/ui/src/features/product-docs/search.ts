import { wikiArticlePath } from './model';
import { documentationSections, type DocumentationSection } from './quality';

export type DocumentationSearchKind = 'article' | 'field' | 'procedure' | 'runbook' | 'source';

export type DocumentationSearchResult = {
  id: string;
  kind: DocumentationSearchKind;
  sectionID: string;
  articleID: string;
  title: string;
  context: string;
  href: string;
  score: number;
};

export function buildDocumentationSearchIndex(
  sections: DocumentationSection[] = documentationSections,
): DocumentationSearchResult[] {
  return sections.flatMap(section => section.articles.flatMap(article => {
    const articleHref = wikiArticlePath(section.id, article.id);
    const records: DocumentationSearchResult[] = [{
      id: `article:${section.id}:${article.id}`,
      kind: 'article',
      sectionID: section.id,
      articleID: article.id,
      title: article.title,
      context: article.summary,
      href: articleHref,
      score: 0,
    }];

    article.configRows.forEach(field => records.push({
      id: `field:${section.id}:${article.id}:${field.anchor}`,
      kind: 'field',
      sectionID: section.id,
      articleID: article.id,
      title: field.path || field.key,
      context: `${article.title} · ${field.description}`,
      href: `${articleHref}#${field.anchor}`,
      score: 0,
    }));

    article.steps.forEach((step, index) => records.push({
      id: `procedure:${section.id}:${article.id}:${index}`,
      kind: 'procedure',
      sectionID: section.id,
      articleID: article.id,
      title: step.title,
      context: `${article.title} · ${step.description}`,
      href: `${articleHref}#procedure`,
      score: 0,
    }));

    article.runbookEntries.forEach(runbook => records.push({
      id: `runbook:${section.id}:${article.id}:${runbook.id}`,
      kind: 'runbook',
      sectionID: section.id,
      articleID: article.id,
      title: runbook.title,
      context: runbook.complete ? `${article.title} · ${runbook.impact}` : `${article.title} · Detailed runbook is not yet documented.`,
      href: `${articleHref}#operations`,
      score: 0,
    }));

    article.sourceLinks.forEach((source, index) => records.push({
      id: `source:${section.id}:${article.id}:${index}`,
      kind: 'source',
      sectionID: section.id,
      articleID: article.id,
      title: source.title,
      context: `${article.title} · ${source.repositoryPath}`,
      href: `${articleHref}#sources`,
      score: 0,
    }));

    return records;
  }));
}

const searchIndex = buildDocumentationSearchIndex();

export function searchDocumentation(query: string, index = searchIndex, limit = 12) {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return [];
  const tokens = normalized.split(/\s+/).filter(Boolean);
  return index
    .map(record => ({ ...record, score: score(record, normalized, tokens) }))
    .filter(record => record.score > 0)
    .sort((left, right) => right.score - left.score || left.title.localeCompare(right.title))
    .slice(0, limit);
}

function score(record: DocumentationSearchResult, query: string, tokens: string[]) {
  const title = record.title.toLowerCase();
  const context = record.context.toLowerCase();
  let value = 0;
  if (title === query) value += record.kind === 'field' ? 140 : 110;
  if (title.startsWith(query)) value += record.kind === 'field' ? 90 : 60;
  if (title.includes(query)) value += record.kind === 'field' ? 70 : 45;
  if (context.includes(query)) value += 25;
  for (const token of tokens) {
    if (title === token) value += 20;
    else if (title.includes(token)) value += 12;
    if (context.includes(token)) value += 5;
  }
  if (record.kind === 'field') value += 8;
  if (record.kind === 'article') value += 4;
  return value;
}
