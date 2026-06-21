import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { pipelineRunsNavPath } from './navigationModel.js';

describe('pipelineRunsNavPath', () => {
  it('keeps the current pipeline runs tab for primary navigation', () => {
    assert.equal(pipelineRunsNavPath('/pipelineruns/recent'), '/pipelineruns/recent');
    assert.equal(pipelineRunsNavPath('/pipelineruns/events'), '/pipelineruns/events');
    assert.equal(pipelineRunsNavPath('/pipelineruns/main'), '/pipelineruns/main');
    assert.equal(pipelineRunsNavPath('/pipelines'), '/pipelineruns/main');
  });
});
