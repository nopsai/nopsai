import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test } from 'node:test';

const sourceRoot = resolve(process.cwd(), 'src');
const styles = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');
const shellSource = readFileSync(resolve(sourceRoot, 'app/AppShell.tsx'), 'utf8');
const docsShellSource = readFileSync(resolve(sourceRoot, 'features/product-docs/DocsShell.tsx'), 'utf8');

function ruleBody(selector: string) {
  const index = styles.indexOf(`\n${selector} {`);
  assert.ok(index >= 0, `expected a ${selector} rule`);
  const start = styles.indexOf('{', index);
  const end = styles.indexOf('}', start);
  return styles.slice(start + 1, end);
}

test('renders the wiki outside the operator app chrome', () => {
  assert.match(shellSource, /const isDocsRoute = location\.pathname === '\/docs'/);
  assert.match(shellSource, /isDocsRoute \? \(\s*<div id="page-content-wrapper"/);
  assert.match(docsShellSource, /className="docs-shell"/);
  assert.match(docsShellSource, /aria-label="Wiki pages"/);
  assert.match(docsShellSource, /Back to NopsAI/);
});

test('keeps the documentation layout metrics from the design contract', () => {
  const shell = ruleBody('.docs-shell');
  assert.match(shell, /--docs-header-height:\s*64px/);
  assert.match(shell, /--docs-sidebar-width:\s*250px/);
  assert.match(shell, /--docs-toc-width:\s*190px/);
  assert.match(shell, /font-size:\s*14px/);

  assert.match(ruleBody('.docs-layout'), /max-width:\s*1400px/);
  assert.match(ruleBody('.docs-layout'), /minmax\(0,\s*850px\)/);
  assert.match(ruleBody('.docs-main'), /padding:\s*54px 60px 100px/);
  assert.match(ruleBody('.docs-h1'), /font-size:\s*34px/);
  assert.match(ruleBody('.docs-h2'), /font-size:\s*22px/);
  assert.match(ruleBody('.docs-prose'), /line-height:\s*1\.75/);
});

test('defines the documentation palette for both themes', () => {
  const light = ruleBody('.docs-shell');
  const dark = ruleBody('html.dark .docs-shell');

  for (const token of [
    '--docs-bg',
    '--docs-sidebar-bg',
    '--docs-border',
    '--docs-text',
    '--docs-muted',
    '--docs-accent',
    '--docs-accent-soft',
    '--docs-code-bg',
  ]) {
    assert.match(light, new RegExp(`${token}:`), `light palette must define ${token}`);
    assert.match(dark, new RegExp(`${token}:`), `dark palette must redefine ${token}`);
  }

  // Wiki components consume the app tokens; the docs subtree rebinds them so a
  // single palette drives the whole page in both themes.
  assert.match(light, /--text-primary: var\(--docs-text\)/);
  assert.match(light, /--border-primary: var\(--docs-border\)/);
});

test('keeps the sidebar and the on-this-page rail in their own columns', () => {
  // Both rails were `position: fixed` with no offsets, which resolves to the
  // element's static position — a grid item's static position is not reliably
  // its column, so the rail could paint on top of the sidebar. Sticky keeps them
  // in flow horizontally and pinned vertically.
  const sidebar = ruleBody('.docs-sidebar');
  const toc = ruleBody('.docs-toc');

  assert.match(sidebar, /grid-column:\s*1/);
  assert.match(sidebar, /position:\s*sticky/);
  assert.match(toc, /grid-column:\s*3/);
  assert.match(toc, /position:\s*sticky/);
  assert.doesNotMatch(sidebar, /position:\s*fixed/);
  assert.doesNotMatch(toc, /position:\s*fixed/);
  assert.match(ruleBody('.docs-layout'), /align-items:\s*start/);
});

test('hides the on-this-page rail and sidebar at the documented breakpoints', () => {
  assert.match(styles, /@media \(max-width: 1050px\) \{[\s\S]*?\.docs-toc \{\s*display: none;/);
  assert.match(styles, /@media \(max-width: 700px\) \{[\s\S]*?\.docs-sidebar \{\s*display: none;/);
  // The mobile menu is the one case that leaves the flow, so it overlays the
  // article rather than pushing it down the page.
  assert.match(styles, /@media \(max-width: 700px\) \{[\s\S]*?\.docs-sidebar\.is-open \{[\s\S]*?position: fixed;/);
});

test('never links to a fragment without its route', () => {
  // Under a HashRouter the URL hash is the route, so a bare `href="#section"`
  // navigates away from the article instead of scrolling within it.
  const docsSources = [
    'features/product-docs/DocsShell.tsx',
    'features/product-docs/WikiArticle.tsx',
    'features/product-docs/WikiIndexes.tsx',
    'features/product-docs/WikiNavigation.tsx',
    'features/product-docs/WikiPrimitives.tsx',
    'features/product-docs/WikiApiOperations.tsx',
  ];
  for (const relativePath of docsSources) {
    const source = readFileSync(resolve(sourceRoot, relativePath), 'utf8');
    assert.doesNotMatch(
      source.replace(/\/\*[\s\S]*?\*\//g, ''),
      /href=(\{`#|"#)/,
      `${relativePath} must route fragment links through the router`,
    );
  }
});
