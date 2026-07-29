import type {
  PipelineTreeNode,
  StepTreeNode,
} from './types.js';
import { insertTeamPath } from '../lib/resourceTeams.js';

export function splitIdentifier(id: string): { name: string; path: string } {
  const parts = id.split('/').filter(Boolean);
  const name = decodeURIComponent(parts.pop() || '');
  const path = parts.map(decodeURIComponent).join('/');
  return { name, path };
}

export function buildPipelineTree(pipelines: string[], resourceTeamPaths: string[]): PipelineTreeNode {
  const root: PipelineTreeNode = { id: '__root__', name: 'All pipelines', fullPath: '', children: [], pipelineIds: [] };
  resourceTeamPaths.forEach(path => {
    insertTeamPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], pipelineIds: [] }));
  });
  pipelines.forEach(id => {
    const parts = id.split('/').filter(Boolean);
    const pipelineName = parts.pop();
    if (!pipelineName) return;
    let current = root;
    let pathSoFar = '';
    parts.forEach(segment => {
      pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
      let child = current.children.find(c => c.name === segment);
      if (!child) {
        child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], pipelineIds: [] };
        current.children.push(child);
        current.children.sort((a, b) => a.name.localeCompare(b.name));
      }
      current = child;
    });
    current.pipelineIds.push(id);
    current.pipelineIds.sort((a, b) => a.localeCompare(b));
  });
  return root;
}

export function buildStepTree(steps: string[], resourceTeamPaths: string[]): StepTreeNode {
  const root: StepTreeNode = { id: '__root__', name: 'All steps', fullPath: '', children: [], stepIds: [] };
  resourceTeamPaths.forEach(path => {
    insertTeamPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], stepIds: [] }));
  });
  steps.forEach(id => {
    const parts = id.split('/').filter(Boolean);
    const stepName = parts.pop();
    if (!stepName) return;
    let current = root;
    let pathSoFar = '';
    parts.forEach(segment => {
      pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
      let child = current.children.find(c => c.name === segment);
      if (!child) {
        child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], stepIds: [] };
        current.children.push(child);
        current.children.sort((a, b) => a.name.localeCompare(b.name));
      }
      current = child;
    });
    current.stepIds.push(id);
    current.stepIds.sort((a, b) => a.localeCompare(b));
  });
  return root;
}
