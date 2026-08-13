import { apiRoutes } from './content/fields/api.js';
import { allEnvironmentFields } from './content/fields/environment.js';
import {
  wikiArticleLocations,
  wikiArticlePath,
  wikiFieldAnchor,
  type WikiApiRoute,
  type WikiField,
} from './content/index.js';

/**
 * Reference indexes are derived, never hand-maintained.
 *
 * Every directive row on the index comes from the same `fields` array the
 * owning article renders, so the index cannot drift from the page that explains
 * the directive.
 */
export type IndexedField = WikiField & {
  articleID: string;
  articleTitle: string;
  sectionID: string;
  href: string;
};

function collectFields(predicate: (field: WikiField, articleID: string) => boolean): IndexedField[] {
  const rows: IndexedField[] = [];
  const seen = new Set<string>();
  for (const { section, article } of wikiArticleLocations) {
    for (const field of article.fields || []) {
      if (!predicate(field, article.id)) continue;
      const key = `${field.scope}::${field.path}`;
      if (seen.has(key)) continue;
      seen.add(key);
      rows.push({
        ...field,
        articleID: article.id,
        articleTitle: article.title,
        sectionID: section.id,
        href: `${wikiArticlePath(section.id, article.id)}#${wikiFieldAnchor(field.path)}`,
      });
    }
  }
  return rows;
}

/** Scopes that belong to the environment index rather than the directive index. */
const ENVIRONMENT_SCOPES = new Set(allEnvironmentFields.map(field => `${field.scope}::${field.path}`));

/** Every YAML directive documented anywhere in the wiki. */
export const directiveIndex: IndexedField[] = collectFields(
  field => !ENVIRONMENT_SCOPES.has(`${field.scope}::${field.path}`),
).sort((left, right) => left.scope.localeCompare(right.scope) || left.path.localeCompare(right.path));

/** Every environment variable documented anywhere in the wiki, in authored order. */
export const environmentIndex: IndexedField[] = (() => {
  const documented = collectFields(field => ENVIRONMENT_SCOPES.has(`${field.scope}::${field.path}`));
  const byPath = new Map(documented.map(row => [`${row.scope}::${row.path}`, row]));
  return allEnvironmentFields
    .map(field => byPath.get(`${field.scope}::${field.path}`))
    .filter((row): row is IndexedField => Boolean(row));
})();

export const apiRouteIndex: WikiApiRoute[] = apiRoutes;

export function directiveScopes() {
  return Array.from(new Set(directiveIndex.map(row => row.scope))).sort();
}

export function environmentScopes() {
  return Array.from(new Set(environmentIndex.map(row => row.scope))).sort();
}

export function apiAreas() {
  const areas: string[] = [];
  for (const route of apiRouteIndex) {
    if (!areas.includes(route.area)) areas.push(route.area);
  }
  return areas;
}

export function filterIndexedFields(rows: IndexedField[], query: string, scope: string) {
  const normalized = query.trim().toLowerCase();
  return rows.filter(row => {
    if (scope && row.scope !== scope) return false;
    if (!normalized) return true;
    return (
      row.path.toLowerCase().includes(normalized) ||
      row.description.toLowerCase().includes(normalized) ||
      row.scope.toLowerCase().includes(normalized) ||
      (row.allowedValues || []).some(value => value.toLowerCase().includes(normalized))
    );
  });
}

export function filterApiRoutes(routes: WikiApiRoute[], query: string, area: string) {
  const normalized = query.trim().toLowerCase();
  return routes.filter(route => {
    if (area && route.area !== area) return false;
    if (!normalized) return true;
    return (
      route.path.toLowerCase().includes(normalized) ||
      route.purpose.toLowerCase().includes(normalized) ||
      route.method.toLowerCase() === normalized ||
      route.area.toLowerCase().includes(normalized)
    );
  });
}

/** Index article IDs that render a generated table instead of normal article blocks. */
export const indexArticleIDs = ['directive-index', 'environment-index', 'api-index'] as const;

export type IndexArticleID = (typeof indexArticleIDs)[number];

export function isIndexArticle(articleID: string): articleID is IndexArticleID {
  return (indexArticleIDs as readonly string[]).includes(articleID);
}
