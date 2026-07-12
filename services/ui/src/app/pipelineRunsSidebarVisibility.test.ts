import assert from 'node:assert/strict';
import { test } from 'node:test';
import { shouldShowPipelineRunsSidebarContext } from './pipelineRunsSidebarVisibility.js';

test('hides pipeline run sidebar context on overview only', () => {
  assert.equal(shouldShowPipelineRunsSidebarContext('main'), false);
  assert.equal(shouldShowPipelineRunsSidebarContext('recent'), true);
  assert.equal(shouldShowPipelineRunsSidebarContext('events'), true);
});
