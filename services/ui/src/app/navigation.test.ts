import { describe, expect, it } from 'vitest';
import { pipelineRunsNavPath } from './navigationModel.js';

describe('pipelineRunsNavPath', () => {
  it('keeps the current pipeline runs tab for primary navigation', () => {
    expect(pipelineRunsNavPath('/pipelineruns/recent')).toBe('/pipelineruns/recent');
    expect(pipelineRunsNavPath('/pipelineruns/events')).toBe('/pipelineruns/events');
    expect(pipelineRunsNavPath('/pipelineruns/main')).toBe('/pipelineruns/main');
    expect(pipelineRunsNavPath('/pipelines')).toBe('/pipelineruns/main');
  });
});
