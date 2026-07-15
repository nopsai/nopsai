import {
  AI_RESOURCE_TEAM_FILTER_ALL,
  AI_RESOURCE_TEAM_FILTER_GLOBAL,
  aiResourceTeamScope,
  normalizeAIResourceTeamPath,
} from './aiResourceTeams.js';

export type AIResourceTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: AIResourceTreeNode[];
  resourceIDs: string[];
};

export const AI_RESOURCE_TREE_ROOT_ID = '__root__';
export const AI_RESOURCE_TREE_GLOBAL_ID = '__global__';

export function buildAIResourceTree(resourceIDs: string[], knownTeamPaths: string[] = []): AIResourceTreeNode {
  const root: AIResourceTreeNode = {
    id: AI_RESOURCE_TREE_ROOT_ID,
    name: 'All teams',
    fullPath: '',
    children: [],
    resourceIDs: [],
  };
  const nodes = new Map<string, AIResourceTreeNode>([[root.id, root]]);

  const ensureTeamNode = (teamPath: string) => {
    const normalized = normalizeAIResourceTeamPath(teamPath);
    if (!normalized) return root;

    let parent = root;
    const segments = normalized.split('/').filter(Boolean);
    segments.forEach((segment, index) => {
      const fullPath = segments.slice(0, index + 1).join('/');
      const id = aiResourceTreeTeamID(fullPath);
      let node = nodes.get(id);
      if (!node) {
        node = {
          id,
          name: segment,
          fullPath,
          children: [],
          resourceIDs: [],
        };
        nodes.set(id, node);
        parent.children.push(node);
      }
      parent = node;
    });
    return parent;
  };

  knownTeamPaths.forEach(path => ensureTeamNode(path));

  resourceIDs.forEach(resourceID => {
    const normalizedID = normalizeAIResourceTeamPath(resourceID);
    if (!normalizedID) return;
    const scope = aiResourceTeamScope(normalizedID);
    if (!scope.teamPath) {
      root.resourceIDs.push(normalizedID);
      return;
    }
    ensureTeamNode(scope.teamPath).resourceIDs.push(normalizedID);
  });

  sortAIResourceTree(root);
  return root;
}

export function aiResourceTreeTeamID(teamPath: string) {
  const normalized = normalizeAIResourceTeamPath(teamPath);
  return normalized ? `team:${normalized}` : AI_RESOURCE_TREE_ROOT_ID;
}

export function aiResourceTreeAncestorIDs(teamPath: string) {
  const normalized = normalizeAIResourceTeamPath(teamPath);
  const ids = [AI_RESOURCE_TREE_ROOT_ID];
  if (!normalized) return ids;
  const segments = normalized.split('/').filter(Boolean);
  segments.forEach((_, index) => {
    ids.push(aiResourceTreeTeamID(segments.slice(0, index + 1).join('/')));
  });
  return ids;
}

export function countAIResourceTreeResources(node: AIResourceTreeNode): number {
  return node.resourceIDs.length + node.children.reduce((total, child) => total + countAIResourceTreeResources(child), 0);
}

export function aiResourceTreeFilterForResource(resourceID: string) {
  const teamPath = aiResourceTeamScope(resourceID).teamPath;
  return teamPath || AI_RESOURCE_TEAM_FILTER_GLOBAL;
}

export function aiResourceTreeFilterIsAll(value: string) {
  return !value || value === AI_RESOURCE_TEAM_FILTER_ALL;
}

export function aiResourceTreeFilterIsGlobal(value: string) {
  return value === AI_RESOURCE_TEAM_FILTER_GLOBAL;
}

function sortAIResourceTree(node: AIResourceTreeNode) {
  node.children.sort((left, right) => left.name.localeCompare(right.name, undefined, { sensitivity: 'base' }));
  node.resourceIDs.sort((left, right) => left.localeCompare(right, undefined, { sensitivity: 'base' }));
  node.children.forEach(sortAIResourceTree);
}
