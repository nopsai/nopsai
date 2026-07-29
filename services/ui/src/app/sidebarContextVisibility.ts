import { shouldShowPipelineRunsSidebarContext } from './pipelineRunsSidebarVisibility.js';
import type { RunTabKey } from './types.js';

export function shouldShowSidebarContextNav(pathname: string, pipelineRunsTab: RunTabKey) {
  if (pathname.startsWith('/triggers')) return false;
  if (pathname.startsWith('/knowledge-context')) return false;
  if (pathname.startsWith('/scopes')) return false;
  if (pathname.startsWith('/pipelineruns')) return shouldShowPipelineRunsSidebarContext(pipelineRunsTab);
  return true;
}
