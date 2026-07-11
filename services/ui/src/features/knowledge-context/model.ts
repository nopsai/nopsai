import { insertTeamPath } from '../../lib/resourceTeams.js';

export type KnowledgeContextListItem = {
  id: string;
  uuid?: string;
  kind: string;
  team: string;
  name: string;
  description?: string;
  visibility: string;
  source: string;
  updated_at?: string;
  access?: string;
  used_by_count?: number;
  used_by?: string[];
  config_source_path?: string;
  config_source_commit_sha?: string;
};

export type KnowledgeContextDetail = KnowledgeContextListItem & {
  content: string;
  managed_by_config_repo?: boolean;
};

export type KnowledgeTeamNode = {
  id: string;
  name: string;
  fullPath: string;
  children: KnowledgeTeamNode[];
  docs: KnowledgeContextListItem[];
};

export type KnowledgeDocumentParameters = Partial<Record<'name' | 'kind' | 'description', string>>;

export type KnowledgeDraftSnapshot = {
  detail: KnowledgeContextDetail;
  content: string;
};

export const KNOWLEDGE_CONTEXTS_CHANGED_EVENT = 'nopsai-knowledge-contexts-changed';
export const kindOrder = ['architecture', 'guardrail', 'policy', 'adr', 'guideline', 'runbook', 'reference', 'example'];

export const emptyKnowledgeDraft: KnowledgeContextDetail = {
  id: 'architecture/team-1/new-document',
  kind: 'architecture',
  team: 'team-1',
  name: 'new-document',
  visibility: 'team',
  source: 'database',
  content: '',
};

const knowledgeDraftStoragePrefix = 'nopsai.knowledge-context.draft.';

export function encodeKnowledgeID(id: string) {
  return id.split('/').map(encodeURIComponent).join('/');
}

export function buildKnowledgeID(kind: string, team: string, name: string) {
  return [kind, team, name]
    .map(part => part.trim().replace(/^\/+|\/+$/g, ''))
    .filter(Boolean)
    .join('/');
}

function knowledgeDraftStorageKey(id: string) {
  return `${knowledgeDraftStoragePrefix}${id}`;
}

export function saveKnowledgeDraft(snapshot: KnowledgeDraftSnapshot) {
  if (typeof sessionStorage === 'undefined') return;
  try {
    sessionStorage.setItem(knowledgeDraftStorageKey(snapshot.detail.id), JSON.stringify(snapshot));
  } catch {
    // Draft persistence is best effort.
  }
}

export function loadKnowledgeDraft(id: string): KnowledgeDraftSnapshot | null {
  if (typeof sessionStorage === 'undefined') return null;
  try {
    const raw = sessionStorage.getItem(knowledgeDraftStorageKey(id));
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<KnowledgeDraftSnapshot>;
    return parsed?.detail?.id === id && typeof parsed.content === 'string' ? (parsed as KnowledgeDraftSnapshot) : null;
  } catch {
    return null;
  }
}

export function clearKnowledgeDraft(id: string) {
  if (typeof sessionStorage === 'undefined') return;
  try {
    sessionStorage.removeItem(knowledgeDraftStorageKey(id));
  } catch {
    // Ignore storage failures.
  }
}

export function splitKnowledgePath(id: string) {
  const parts = id.split('/').filter(Boolean);
  const name = parts.pop() || '';
  return { name, team: parts.join('/') };
}

export function normalizeTeamPath(value: string) {
  return value.trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
}

export function parentTeam(path: string) {
  const parts = normalizeTeamPath(path).split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

export function buildKnowledgeTree(items: KnowledgeContextListItem[], teamPaths: string[]): KnowledgeTeamNode {
  const root: KnowledgeTeamNode = { id: 'root', name: 'Knowledge Context', fullPath: '', children: [], docs: [] };
  const nodes = new Map<string, KnowledgeTeamNode>([['', root]]);

  const ensureNode = (fullPath: string): KnowledgeTeamNode => {
    const normalized = normalizeTeamPath(fullPath);
    const existing = nodes.get(normalized);
    if (existing) return existing;
    const parent = ensureNode(parentTeam(normalized));
    const name = normalized.split('/').filter(Boolean).pop() || 'root';
    const node: KnowledgeTeamNode = { id: normalized || 'root', name, fullPath: normalized, children: [], docs: [] };
    parent.children.push(node);
    nodes.set(normalized, node);
    return node;
  };

  kindOrder.forEach(kind => ensureNode(kind));
  kindOrder.forEach(kind => {
    teamPaths.forEach(teamPath => {
      const normalizedTeam = normalizeTeamPath(teamPath);
      if (!normalizedTeam) return;
      insertTeamPath(root, `${kind}/${normalizedTeam}`, (id, name, fullPath) => {
        const node: KnowledgeTeamNode = { id, name, fullPath, children: [], docs: [] };
        nodes.set(fullPath, node);
        return node;
      });
    });
  });

  items.forEach(item => {
    ensureNode(splitKnowledgePath(item.id).team).docs.push(item);
  });

  nodes.forEach(node => {
    node.children.sort((a, b) => {
      const ai = kindOrder.indexOf(a.name);
      const bi = kindOrder.indexOf(b.name);
      return (ai < 0 ? kindOrder.length : ai) - (bi < 0 ? kindOrder.length : bi) || a.name.localeCompare(b.name);
    });
    node.docs.sort((a, b) => a.name.localeCompare(b.name));
  });
  return root;
}

export function findKnowledgeTeam(root: KnowledgeTeamNode, fullPath: string) {
  const normalized = normalizeTeamPath(fullPath);
  if (!normalized) return root;
  let current = root;
  for (const segment of normalized.split('/').filter(Boolean)) {
    const next = current.children.find(child => child.name === segment);
    if (!next) return root;
    current = next;
  }
  return current;
}

export function countTeamDocs(node: KnowledgeTeamNode): number {
  return node.docs.length + node.children.reduce((total, child) => total + countTeamDocs(child), 0);
}

export function decodeKnowledgeRouteID(pathname: string) {
  const prefix = '/knowledge-context/';
  if (!pathname.startsWith(prefix)) return '';
  return pathname
    .slice(prefix.length)
    .split('/')
    .filter(Boolean)
    .map(decodeURIComponent)
    .join('/');
}

export function sourceLabel(source: string) {
  const value = source.toLowerCase();
  if (value.includes('git')) return 'GitOps';
  if (value.includes('repo')) return 'Repo';
  if (value.includes('database')) return 'Database';
  return 'UI';
}

export function deriveIdentityFromTeam(activeTeam: string) {
  const parts = normalizeTeamPath(activeTeam).split('/').filter(Boolean);
  const first = parts[0] || '';
  return kindOrder.includes(first)
    ? { kind: first, team: parts.slice(1).join('/') || 'team-1' }
    : { kind: 'architecture', team: parts.join('/') || 'team-1' };
}

export function normalizeKnowledgeSource(source: string) {
  const label = sourceLabel(source).toLowerCase();
  if (label === 'gitops') return 'git';
  return label || 'database';
}

export function isGitManagedDocument(doc: Pick<KnowledgeContextListItem, 'source'> & { managed_by_config_repo?: boolean }) {
  return Boolean(doc.managed_by_config_repo) || normalizeKnowledgeSource(doc.source) === 'git';
}

export function splitKnowledgeContentForPreview(content: string): { content: string; parameters: KnowledgeDocumentParameters } {
  const normalized = content.replace(/\r\n/g, '\n');
  if (normalized.startsWith('---\n')) {
    const end = normalized.indexOf('\n---', 4);
    if (end >= 0) {
      const bodyStart = end + (normalized[end + 4] === '\n' ? 5 : 4);
      return { content: normalized.slice(bodyStart), parameters: parseKnowledgeParameterLines(normalized.slice(4, end)) };
    }
  }

  const contentLine = findTopLevelKnowledgeKey(normalized, 'content');
  if (contentLine) {
    const suffix = contentLine.text.slice('content:'.length).trim();
    return {
      content: suffix && !suffix.startsWith('|') && !suffix.startsWith('>') ? unquoteYAMLScalar(suffix) : removeCommonIndent(normalized.slice(contentLine.end)),
      parameters: parseKnowledgeParameterLines(normalized.slice(0, contentLine.start)),
    };
  }

  return splitLeadingKnowledgeParameters(normalized) || { content, parameters: {} };
}

function findTopLevelKnowledgeKey(content: string, key: string): { start: number; end: number; text: string } | null {
  let offset = 0;
  for (const line of content.split('\n')) {
    const text = line.replace(/\r$/, '');
    if (text.startsWith(`${key}:`)) {
      return { start: offset, end: Math.min(offset + line.length + 1, content.length), text };
    }
    offset += line.length + 1;
  }
  return null;
}

function splitLeadingKnowledgeParameters(content: string): { content: string; parameters: KnowledgeDocumentParameters } | null {
  const firstLine = content.split('\n', 1)[0] || '';
  if (!isKnowledgeParameterLine(firstLine)) return null;
  const separator = content.indexOf('\n\n');
  const header = separator >= 0 ? content.slice(0, separator) : content;
  if (header.split('\n').some(line => line.trim() && !line.startsWith(' ') && !line.startsWith('\t') && !isKnowledgeParameterLine(line))) {
    return null;
  }
  return {
    content: separator >= 0 ? content.slice(separator + 2) : '',
    parameters: parseKnowledgeParameterLines(header),
  };
}

function isKnowledgeParameterLine(line: string) {
  const key = line.split(':', 1)[0]?.trim();
  return key === 'name' || key === 'kind' || key === 'description' || key === 'access';
}

function parseKnowledgeParameterLines(content: string): KnowledgeDocumentParameters {
  const parameters: KnowledgeDocumentParameters = {};
  content.split('\n').forEach(rawLine => {
    if (!rawLine || rawLine.startsWith(' ') || rawLine.startsWith('\t')) return;
    const separator = rawLine.indexOf(':');
    if (separator < 0) return;
    const key = rawLine.slice(0, separator).trim() as keyof KnowledgeDocumentParameters;
    if (key !== 'name' && key !== 'kind' && key !== 'description') return;
    const value = unquoteYAMLScalar(rawLine.slice(separator + 1).trim());
    if (value) parameters[key] = value;
  });
  return parameters;
}

function unquoteYAMLScalar(value: string) {
  if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
    return value.slice(1, -1);
  }
  return value;
}

function removeCommonIndent(content: string) {
  const lines = content.split('\n');
  const indents = lines
    .filter(line => line.trim())
    .map(line => line.match(/^[ \t]*/)?.[0].length || 0);
  const minIndent = indents.length ? Math.min(...indents) : 0;
  return minIndent ? lines.map(line => line.slice(Math.min(minIndent, line.match(/^[ \t]*/)?.[0].length || 0))).join('\n') : content;
}

export function validateKnowledgeIdentity(
  kind: string,
  team: string,
  name: string,
  existingItems: KnowledgeContextListItem[],
  currentID?: string
) {
  const normalizedKind = kind.trim();
  const normalizedTeam = normalizeTeamPath(team);
  const normalizedName = name.trim().replace(/\.(yaml|yml)$/i, '');
  if (!kindOrder.includes(normalizedKind)) return 'Choose a supported kind.';
  if (!normalizedTeam) return 'Team is required.';
  if (normalizedTeam.split('/').some(part => !part || part === '.' || part === '..')) return 'Team contains invalid path segments.';
  if (!normalizedName) return 'Document name is required.';
  if (!/^[a-zA-Z0-9_.-]+$/.test(normalizedName)) return 'Document name can only contain letters, numbers, dots, underscores, and hyphens.';
  const id = buildKnowledgeID(normalizedKind, normalizedTeam, normalizedName);
  if (id !== currentID && existingItems.some(item => item.id === id)) return 'A knowledge context with that identifier already exists.';
  return '';
}
