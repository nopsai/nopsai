import { wikiSlug, type WikiApiRoute } from './content/index.js';

/** Anchor for one operation, so search and the API index can link straight to it. */
export function wikiOperationAnchor(route: Pick<WikiApiRoute, 'method' | 'path'>) {
  return `operation-${wikiSlug(`${route.method} ${route.path}`)}`;
}
