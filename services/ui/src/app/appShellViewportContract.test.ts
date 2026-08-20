import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test } from 'node:test';

const sourceRoot = resolve(process.cwd(), 'src');

test('keeps document scrolling owned by app shell scroll containers', () => {
  const shellSource = readFileSync(resolve(sourceRoot, 'app/AppShell.tsx'), 'utf8');
  const indexStyles = readFileSync(resolve(sourceRoot, 'index.css'), 'utf8');

  assert.match(indexStyles, /html,\s*body,\s*#root\s*\{[\s\S]*?height:\s*100%/);
  assert.match(indexStyles, /body\s*\{[\s\S]*?overflow:\s*hidden/);
  assert.match(indexStyles, /body\s*\{[\s\S]*?overscroll-behavior-y:\s*none/);
  assert.match(shellSource, /<div className="h-screen overflow-auto">[\s\S]*?<LoginPage/);
  assert.match(
    shellSource,
    /id="page-content-wrapper"\s+className="[^"]*\bflex-1\b[^"]*\bmin-h-0\b[^"]*\boverflow-auto\b/
  );
});

/**
 * A sideways swipe over ordinary content is the browser's back and forward
 * gesture. The app used to eat it everywhere: the document and both page scroll
 * containers set `overscroll-behavior: contain` on both axes, so every page
 * rubber-banded a few pixels sideways and sprang back instead. Vertical
 * containment is kept — that is what stops a scrolling pane from dragging the
 * page behind it.
 */
test('leaves horizontal gestures to the browser except where content is too wide', () => {
  const styles = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');
  const indexStyles = readFileSync(resolve(sourceRoot, 'index.css'), 'utf8');
  const shellSource = readFileSync(resolve(sourceRoot, 'app/AppShell.tsx'), 'utf8');

  assert.match(indexStyles, /body\s*\{[\s\S]*?overscroll-behavior-x:\s*auto/);
  assert.doesNotMatch(indexStyles, /body\s*\{[\s\S]*?overscroll-behavior:\s*none/);
  assert.doesNotMatch(shellSource, /overscroll-contain/);
  assert.match(
    styles,
    /#page-content-wrapper\s*\{[\s\S]*?overscroll-behavior-y:\s*contain;[\s\S]*?overscroll-behavior-x:\s*auto;/
  );

  // Only the graph canvas, which pans under the pointer, contains both axes.
  const blanket = styles.match(/^[^\n{}]*\{[^}]*overscroll-behavior:\s*contain[^}]*\}/gm) ?? [];
  assert.equal(blanket.length, 1, 'only the pan/zoom graph may contain both axes');
  assert.match(blanket[0], /graph-container/);

  // Wide content keeps the gesture instead of chaining into a page navigation.
  assert.match(styles, /\.docs-table-scroll,[\s\S]{0,600}overscroll-behavior-x:\s*contain;/);
  for (const selector of ['.pipeline-runs-table-wrap', '.dispatcher-table-wrap', '.kc-demo-table-wrap']) {
    assert.ok(
      styles.includes(selector + ',') || styles.includes(selector + ' {'),
      `${selector} should be listed as a sideways scroller`
    );
  }
});
