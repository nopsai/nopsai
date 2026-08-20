import type { WikiField } from '../types.js';

/**
 * The field rows for the directives a page introduces, in the order given.
 *
 * Field data has one home per domain module, but a feature page documents only
 * the handful of directives it introduces. Selecting by path keeps the data
 * unduplicated: a page names what it covers, and `content.test.ts` checks that
 * every path resolves and that no directive is introduced on two pages.
 */
export function pickWikiFields(source: WikiField[], paths: string[]): WikiField[] {
  return paths
    .map(path => source.find(field => field.path === path))
    .filter((field): field is WikiField => Boolean(field));
}
