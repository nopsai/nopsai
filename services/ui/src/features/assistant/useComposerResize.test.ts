import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { assistantComposerMaxHeight, assistantComposerMinHeight, clampComposerHeight } from './useComposerResize.js';

describe('assistant composer resize helpers', () => {
  it('clamps composer height to the supported vertical range', () => {
    assert.equal(clampComposerHeight(24), assistantComposerMinHeight);
    assert.equal(clampComposerHeight(180.4), 180);
    assert.equal(clampComposerHeight(999), assistantComposerMaxHeight);
    assert.equal(clampComposerHeight(Number.NaN), assistantComposerMinHeight);
  });
});
