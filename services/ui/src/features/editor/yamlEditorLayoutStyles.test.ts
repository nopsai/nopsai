import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, it } from 'node:test';

const styles = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8');

function cssBlock(selector: string): string {
  const selectorPattern = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const selectorMatch = styles.match(new RegExp(`(^|\\n)${selectorPattern} \\{`));
  const start = selectorMatch?.index ?? -1;
  assert.ok(start >= 0, `expected ${selector} CSS block`);
  const bodyStart = styles.indexOf('{', start);
  const bodyEnd = styles.indexOf('\n}', bodyStart);
  assert.ok(bodyStart >= 0 && bodyEnd > bodyStart, `expected ${selector} CSS body`);
  return styles.slice(bodyStart + 1, bodyEnd);
}

function declarationValue(block: string, property: string): string {
  const escapedProperty = property.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = block.match(new RegExp(`${escapedProperty}:\\s*([^;]+);`));
  assert.ok(match, `expected ${property} declaration`);
  return match[1].trim();
}

describe('YAML editor layout styles', () => {
  it('keeps pipeline and step Definition YAML frames viewport-responsive', () => {
    const definitionLayout = cssBlock('.pipeline-detail-definition-layout');
    assert.equal(
      declarationValue(definitionLayout, '--pipeline-detail-yaml-frame-height'),
      'max(34rem, calc(100dvh - 24rem))'
    );

    const normalEditorFrame = cssBlock(
      '.pipeline-detail-definition-main .resource-yaml-editor-shell--normal #editor-container,\n' +
      '.pipeline-detail-definition-main .resource-yaml-editor-shell--normal #step-editor-container'
    );
    assert.equal(declarationValue(normalEditorFrame, 'height'), 'var(--pipeline-detail-yaml-frame-height)');
    assert.equal(declarationValue(normalEditorFrame, 'max-height'), 'none');

    const readOnlyFrame = cssBlock(
      '.pipeline-detail-definition-main #pipeline-yaml-content,\n' +
      '.pipeline-detail-definition-main #step-yaml-content'
    );
    assert.equal(declarationValue(readOnlyFrame, 'max-height'), 'var(--pipeline-detail-yaml-frame-height)');

    assert.match(
      styles,
      /@media \(max-width: 560px\) \{\n\s+\.pipeline-detail-definition-layout \{\n\s+--pipeline-detail-yaml-frame-height: max\(22rem, calc\(100dvh - 18rem\)\);/
    );
    assert.doesNotMatch(styles, /height:\s*clamp\(34rem, calc\(100dvh - 24rem\), 48rem\);/);
  });

  it('lets fullscreen pipeline YAML override the generic editor-container sizing', () => {
    const fullscreenEditorFrame = cssBlock(
      '.resource-yaml-editor-shell--fullscreen #editor-container,\n' +
      '.resource-yaml-editor-shell--fullscreen #step-editor-container,\n' +
      '.resource-yaml-editor-shell--fullscreen .editor-container'
    );
    assert.equal(declarationValue(fullscreenEditorFrame, 'height'), '100%');
    assert.equal(declarationValue(fullscreenEditorFrame, 'max-height'), 'none');

    const genericEditorFrame = cssBlock('#editor-container');
    assert.equal(
      declarationValue(genericEditorFrame, 'height'),
      'var(--yaml-editor-frame-height, max(30rem, calc(100dvh - 25rem)))'
    );
    assert.equal(declarationValue(genericEditorFrame, 'max-height'), 'none');
    assert.doesNotMatch(genericEditorFrame, /height:\s*640px/);
  });

  it('keeps YAML editor rows one-to-one with line numbers by disabling soft wrap', () => {
    const highlightContent = cssBlock('.yaml-editor-highlight__content');
    assert.equal(declarationValue(highlightContent, 'white-space'), 'pre');
    assert.equal(declarationValue(highlightContent, 'overflow-wrap'), 'normal');
    assert.equal(declarationValue(highlightContent, 'min-width'), 'max-content');

    const textarea = cssBlock('.yaml-editor-stage textarea');
    assert.equal(declarationValue(textarea, 'white-space'), 'pre');
    assert.equal(declarationValue(textarea, 'overflow'), 'auto');
  });
});
