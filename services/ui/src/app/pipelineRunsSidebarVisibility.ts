import type { RunTabKey } from './types.js';

export function shouldShowPipelineRunsSidebarContext(tab: RunTabKey) {
  return tab !== 'main';
}
