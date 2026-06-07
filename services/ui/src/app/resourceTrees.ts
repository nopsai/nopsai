import { KNOWLEDGE_CONTEXT_KIND_ORDER } from './constants.js';
import type {
  KnowledgeContextTreeNode,
  PipelineTreeNode,
  ScopeTreeNode,
  StepTreeNode,
  TriggerTreeNode,
} from './types.js';
import { insertGroupPath } from '../lib/resourceGroups.js';

export function normalizeScopeLabel(value: unknown): string {
  if (value == null) return '';
  const normalized = String(value)
    .trim()
    .replace(/^\/+|\/+$/g, '');
  return normalized.toLowerCase() === 'default' ? '' : normalized;
}

export function splitIdentifier(id: string): { name: string; path: string } {
  const parts = id.split('/').filter(Boolean);
  const name = decodeURIComponent(parts.pop() || '');
  const path = parts.map(decodeURIComponent).join('/');
  return { name, path };
}

export function buildPipelineTree(pipelines: string[], resourceGroupPaths: string[]): PipelineTreeNode {
  const root: PipelineTreeNode = { id: '__root__', name: 'All pipelines', fullPath: '', children: [], pipelineIds: [] };
  resourceGroupPaths.forEach(path => {
    insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], pipelineIds: [] }));
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

export function buildTriggerTree(triggers: string[], resourceGroupPaths: string[]): TriggerTreeNode {
  const root: TriggerTreeNode = { id: '__root__', name: 'All triggers', fullPath: '', children: [], triggerSlugs: [] };
  resourceGroupPaths.forEach(path => {
    insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], triggerSlugs: [] }));
  });
  triggers.forEach(slug => {
    const parts = slug.split('/').filter(Boolean);
    const repoName = parts.pop();
    if (!repoName) return;
    let current = root;
    let pathSoFar = '';
    parts.forEach(segment => {
      pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
      let child = current.children.find(c => c.name === segment);
      if (!child) {
        child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], triggerSlugs: [] };
        current.children.push(child);
        current.children.sort((a, b) => a.name.localeCompare(b.name));
      }
      current = child;
    });
    current.triggerSlugs.push(slug);
    current.triggerSlugs.sort((a, b) => a.localeCompare(b));
  });
  return root;
}

export function buildStepTree(steps: string[], resourceGroupPaths: string[]): StepTreeNode {
  const root: StepTreeNode = { id: '__root__', name: 'All steps', fullPath: '', children: [], stepIds: [] };
  resourceGroupPaths.forEach(path => {
    insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], stepIds: [] }));
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

export function buildScopeTree(scopes: string[], resourceGroupPaths: string[]): ScopeTreeNode {
  const root: ScopeTreeNode = { id: '__root__', name: 'All scopes', fullPath: '', children: [], scopes: [] };
  resourceGroupPaths.forEach(path => {
    insertGroupPath(root, path, (id, name, fullPath) => ({ id, name, fullPath, children: [], scopes: [] }));
  });
  scopes.forEach(scope => {
    const normalized = normalizeScopeLabel(scope);
    const parts = normalized.split('/').filter(Boolean);
    if (!parts.length) {
      root.scopes.push('');
      return;
    }
    let current = root;
    let pathSoFar = '';
    parts.forEach(segment => {
      pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
      let child = current.children.find(c => c.name === segment);
      if (!child) {
        child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], scopes: [] };
        current.children.push(child);
        current.children.sort((a, b) => a.name.localeCompare(b.name));
      }
      current = child;
    });
    current.scopes.push(normalized);
    current.scopes.sort((a, b) => a.localeCompare(b));
  });
  return root;
}

export function buildKnowledgeContextTree(
  knowledgeContexts: string[],
  resourceGroupPaths: string[]
): KnowledgeContextTreeNode {
  const root: KnowledgeContextTreeNode = { id: '__root__', name: 'knowledge contexts', fullPath: '', children: [], knowledgeContextIds: [] };
  const folderRank = (name: string) => {
    const index = KNOWLEDGE_CONTEXT_KIND_ORDER.indexOf(name);
    return index < 0 ? KNOWLEDGE_CONTEXT_KIND_ORDER.length : index;
  };
  const sortChildren = (node: KnowledgeContextTreeNode) => {
    node.children.sort((a, b) => folderRank(a.name) - folderRank(b.name) || a.name.localeCompare(b.name));
    node.knowledgeContextIds.sort((a, b) => a.localeCompare(b));
    node.children.forEach(sortChildren);
  };
  const ensureChild = (parent: KnowledgeContextTreeNode, segment: string, fullPath: string) => {
    let child = parent.children.find(c => c.name === segment);
    if (!child) {
      child = { id: fullPath, name: segment, fullPath, children: [], knowledgeContextIds: [] };
      parent.children.push(child);
    }
    return child;
  };

  KNOWLEDGE_CONTEXT_KIND_ORDER.forEach(kind => {
    ensureChild(root, kind, kind);
  });
  KNOWLEDGE_CONTEXT_KIND_ORDER.forEach(kind => {
    resourceGroupPaths.forEach(groupPath => {
      const normalizedGroup = groupPath.split('/').map(part => part.trim()).filter(Boolean).join('/');
      if (!normalizedGroup) return;
      insertGroupPath(root, `${kind}/${normalizedGroup}`, (id, name, fullPath) => ({ id, name, fullPath, children: [], knowledgeContextIds: [] }));
    });
  });

  knowledgeContexts.forEach(id => {
    const parts = id.split('/').filter(Boolean);
    const documentName = parts.pop();
    if (!documentName) return;
    let current = root;
    let pathSoFar = '';
    parts.forEach(segment => {
      pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
      current = ensureChild(current, segment, pathSoFar);
    });
    current.knowledgeContextIds.push(id);
  });

  sortChildren(root);
  return root;
}
