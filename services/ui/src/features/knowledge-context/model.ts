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
  managed_by_config_repo?: boolean;
  connection_id?: string;
  connection_ref?: string;
  external_provider?: string;
  external_page_id?: string;
  external_page_url?: string;
  external_page_title?: string;
  sync_mode?: string;
  failure_mode?: string;
  sync_status?: string;
  last_synced_at?: string;
  sync_error?: string;
  source_modified_at?: string;
  content_hash?: string;
};

export type KnowledgeContextDetail = KnowledgeContextListItem & {
  content: string;
  assets?: KnowledgeContextAsset[];
};

export type KnowledgeContextAsset = {
  id: string;
  provider: string;
  external_page_id?: string;
  source_block_id: string;
  source_block_type: string;
  kind: string;
  title?: string;
  url?: string;
  media_type?: string;
  content_hash?: string;
  metadata?: Record<string, unknown>;
  updated_at?: string;
};

export type KnowledgeWorkspaceTab = 'documents' | 'connections';

export type KnowledgeContentSource = 'inline' | 'external';

export type KnowledgeSourceFilter = 'all' | 'inline' | 'external' | 'gitops' | 'notion' | 'confluence';

export type KnowledgeSyncMode = 'manual' | 'before_run' | 'periodic';

export type KnowledgeFailureMode = 'fail' | 'use_cached' | 'skip';

export const knowledgeSourceFilterOptions: Array<{ value: KnowledgeSourceFilter; label: string }> = [
  { value: 'all', label: 'All sources' },
  { value: 'inline', label: 'Inline' },
  { value: 'external', label: 'External pages' },
  { value: 'gitops', label: 'GitOps' },
  { value: 'notion', label: 'Notion' },
  { value: 'confluence', label: 'Confluence' },
];

export type KnowledgeWorkspaceMetrics = {
  documents: number;
  teams: number;
  kinds: number;
  inlineDocuments: number;
  externalDocuments: number;
  gitOpsManaged: number;
  referencedDocuments: number;
  pipelineReferences: number;
};

export type KnowledgeConnectionTeamSummary = {
  teamPath: string;
  documentCount: number;
  inlineDocuments: number;
  externalDocuments: number;
  gitOpsManaged: number;
  referencedDocuments: number;
  providers: string[];
  connections: KnowledgeConnectionListItem[];
};

export type KnowledgeConnectionProvider = 'notion' | 'confluence' | 'wiki';

export type KnowledgeConnectionListItem = {
  id: string;
  uuid?: string;
  team: string;
  name: string;
  display_name: string;
  provider: KnowledgeConnectionProvider | string;
  status: string;
  disabled?: boolean;
  base_url?: string;
  credential_visibility?: string;
  scopes?: Record<string, unknown>;
  config?: Record<string, unknown>;
  last_checked_at?: string;
  last_error?: string;
  updated_at?: string;
  document_count?: number;
  external_document_count?: number;
  used_by?: string[];
};

export type KnowledgeConnectionDraft = {
  team: string;
  provider: KnowledgeConnectionProvider;
  name: string;
  display_name: string;
  base_url: string;
  credential_ref: string;
  disabled?: boolean;
};

export type KnowledgeExternalPageDraft = {
  connection_id: string;
  external_page_id: string;
  external_page_url: string;
  sync_mode: KnowledgeSyncMode;
  failure_mode: KnowledgeFailureMode;
  content: string;
};

export type KnowledgeExternalPageSummary = {
  id: string;
  title: string;
  url?: string;
  modified_at?: string;
  snippet?: string;
};

export type KnowledgeExternalPageSearchResult = {
  pages: KnowledgeExternalPageSummary[];
  next_cursor?: string;
};

export type KnowledgeExternalPagePreview = KnowledgeExternalPageSummary & {
  text: string;
  hash?: string;
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

export const knowledgeConnectionProviders: Array<{ value: KnowledgeConnectionProvider; label: string }> = [
  { value: 'notion', label: 'Notion' },
  { value: 'confluence', label: 'Confluence' },
  { value: 'wiki', label: 'Wiki page' },
];

export const knowledgeSyncModeOptions: Array<{ value: KnowledgeSyncMode; label: string }> = [
  { value: 'manual', label: 'Manual' },
  { value: 'before_run', label: 'Before run' },
  { value: 'periodic', label: 'Periodic' },
];

export const knowledgeFailureModeOptions: Array<{ value: KnowledgeFailureMode; label: string }> = [
  { value: 'fail', label: 'Fail run' },
  { value: 'use_cached', label: 'Use cached content' },
  { value: 'skip', label: 'Skip optional context' },
];

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

export function collectKnowledgeTeamDocs(node: KnowledgeTeamNode): KnowledgeContextListItem[] {
  return [...node.docs, ...node.children.flatMap(child => collectKnowledgeTeamDocs(child))];
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
  if (value.includes('notion')) return 'Notion';
  if (value.includes('confluence')) return 'Confluence';
  if (value.includes('external') || value.includes('wiki')) return 'External page';
  if (value.includes('git')) return 'GitOps';
  if (value.includes('repo')) return 'Repo';
  if (value.includes('database')) return 'Database';
  return 'UI';
}

export function deriveIdentityFromTeam(activeTeam: string, fallbackTeam = '') {
  const parts = normalizeTeamPath(activeTeam).split('/').filter(Boolean);
  const first = parts[0] || '';
  return kindOrder.includes(first)
    ? { kind: first, team: parts.slice(1).join('/') || fallbackTeam }
    : { kind: 'architecture', team: parts.join('/') || fallbackTeam };
}

export function buildKnowledgeTeamOptions({
  activeTeam = '',
  activeConnectionTeam = '',
  resourceTeamPaths = [],
  items = [],
  connections = [],
  fallbackTeam = '',
}: {
  activeTeam?: string;
  activeConnectionTeam?: string;
  resourceTeamPaths?: string[];
  items?: Array<Pick<KnowledgeContextListItem, 'team'>>;
  connections?: Array<Pick<KnowledgeConnectionListItem, 'team'>>;
  fallbackTeam?: string;
}) {
  const activeIdentity = deriveIdentityFromTeam(activeTeam, '');
  const teams = Array.from(
    new Set(
      [
        activeIdentity.team,
        activeConnectionTeam,
        ...resourceTeamPaths,
        ...items.map(item => item.team),
        ...connections.map(connection => connection.team),
      ]
        .map(team => normalizeTeamPath(team))
        .filter(Boolean)
    )
  ).sort((a, b) => a.localeCompare(b));
  if (!teams.length && fallbackTeam) {
    const fallback = normalizeTeamPath(fallbackTeam);
    if (fallback) teams.push(fallback);
  }
  return teams;
}

export function normalizeKnowledgeSource(source: string) {
  const label = sourceLabel(source).toLowerCase();
  if (label === 'notion') return 'notion';
  if (label === 'confluence') return 'confluence';
  if (label === 'external page') return 'external';
  if (label === 'gitops') return 'git';
  return label || 'database';
}

export function isGitManagedDocument(doc: Pick<KnowledgeContextListItem, 'source'> & { managed_by_config_repo?: boolean }) {
  return Boolean(doc.managed_by_config_repo) || normalizeKnowledgeSource(doc.source) === 'git';
}

export function knowledgeContentSource(doc: Pick<KnowledgeContextListItem, 'source'>): KnowledgeContentSource {
  const source = normalizeKnowledgeSource(doc.source);
  return source === 'notion' || source === 'confluence' || source === 'external' ? 'external' : 'inline';
}

export function isExternalKnowledgeDocument(
  doc: Pick<KnowledgeContextListItem, 'source'> & Pick<Partial<KnowledgeContextListItem>, 'connection_id' | 'connection_ref' | 'external_page_id' | 'external_page_url'>
) {
  return knowledgeContentSource(doc) === 'external' ||
    Boolean(doc.connection_id || doc.connection_ref || doc.external_page_id || doc.external_page_url);
}

export function knowledgeTreePathToTeam(path: string) {
  const parts = normalizeTeamPath(path).split('/').filter(Boolean);
  if (parts.length && kindOrder.includes(parts[0])) return parts.slice(1).join('/');
  return parts.join('/');
}

export function documentTeamPath(item: Pick<KnowledgeContextListItem, 'id' | 'team'>) {
  return normalizeTeamPath(item.team || knowledgeTreePathToTeam(splitKnowledgePath(item.id).team));
}

export function normalizeKnowledgeWorkspaceTab(value?: string | null): KnowledgeWorkspaceTab {
  return value === 'connections' ? 'connections' : 'documents';
}

export function normalizeKnowledgeSourceFilter(value?: string | null): KnowledgeSourceFilter {
  return knowledgeSourceFilterOptions.some(option => option.value === value) ? (value as KnowledgeSourceFilter) : 'all';
}

export function matchesKnowledgeSourceFilter(
  item: Pick<KnowledgeContextListItem, 'source'> & { managed_by_config_repo?: boolean },
  filter: KnowledgeSourceFilter
) {
  if (filter === 'all') return true;
  if (filter === 'inline') return knowledgeContentSource(item) === 'inline';
  if (filter === 'external') return knowledgeContentSource(item) === 'external';
  if (filter === 'gitops') return isGitManagedDocument(item);
  return normalizeKnowledgeSource(item.source) === filter;
}

export function summarizeKnowledgeWorkspace(items: KnowledgeContextListItem[]): KnowledgeWorkspaceMetrics {
  const teamPaths = new Set<string>();
  const kinds = new Set<string>();
  let inlineDocuments = 0;
  let externalDocuments = 0;
  let gitOpsManaged = 0;
  let referencedDocuments = 0;
  let pipelineReferences = 0;

  items.forEach(item => {
    const teamPath = documentTeamPath(item);
    if (teamPath) teamPaths.add(teamPath);
    if (item.kind) kinds.add(item.kind);
    if (knowledgeContentSource(item) === 'external') externalDocuments += 1;
    else inlineDocuments += 1;
    if (isGitManagedDocument(item)) gitOpsManaged += 1;
    const usedBy = item.used_by_count ?? item.used_by?.length ?? 0;
    if (usedBy > 0) referencedDocuments += 1;
    pipelineReferences += usedBy;
  });

  return {
    documents: items.length,
    teams: teamPaths.size,
    kinds: kinds.size,
    inlineDocuments,
    externalDocuments,
    gitOpsManaged,
    referencedDocuments,
    pipelineReferences,
  };
}

export function buildKnowledgeConnectionTeamSummaries(
  items: KnowledgeContextListItem[],
  teamPaths: string[] = [],
  connections: KnowledgeConnectionListItem[] = []
): KnowledgeConnectionTeamSummary[] {
  const summaries = new Map<string, KnowledgeConnectionTeamSummary>();
  const ensureSummary = (teamPath: string) => {
    const normalized = normalizeTeamPath(teamPath || 'root');
    const existing = summaries.get(normalized);
    if (existing) return existing;
    const summary: KnowledgeConnectionTeamSummary = {
      teamPath: normalized,
      documentCount: 0,
      inlineDocuments: 0,
      externalDocuments: 0,
      gitOpsManaged: 0,
      referencedDocuments: 0,
      providers: [],
      connections: [],
    };
    summaries.set(normalized, summary);
    return summary;
  };

  teamPaths.forEach(teamPath => {
    const normalized = normalizeTeamPath(teamPath);
    if (normalized) ensureSummary(normalized);
  });

  const documentsByConnection = new Map<string, string[]>();
  const addConnectionDocument = (connectionID: string | undefined, documentID: string) => {
    const normalizedID = (connectionID || '').trim();
    if (!normalizedID) return;
    const existing = documentsByConnection.get(normalizedID) || [];
    if (!existing.includes(documentID)) existing.push(documentID);
    documentsByConnection.set(normalizedID, existing);
  };

  items.forEach(item => {
    const summary = ensureSummary(documentTeamPath(item));
    summary.documentCount += 1;
    if (knowledgeContentSource(item) === 'external') summary.externalDocuments += 1;
    else summary.inlineDocuments += 1;
    if (isGitManagedDocument(item)) summary.gitOpsManaged += 1;
    if ((item.used_by_count ?? item.used_by?.length ?? 0) > 0) summary.referencedDocuments += 1;
    const source = normalizeKnowledgeSource(item.source);
    if ((source === 'notion' || source === 'confluence' || source === 'external') && !summary.providers.includes(source)) {
      summary.providers.push(source);
    }
    addConnectionDocument(item.connection_ref, item.id);
    addConnectionDocument(item.connection_id, item.id);
  });

  connections.forEach(connection => {
    const summary = ensureSummary(connection.team);
    const linkedDocuments = [
      ...(connection.used_by || []),
      ...(documentsByConnection.get(connection.id) || []),
      ...(documentsByConnection.get(connection.uuid || '') || []),
    ].filter((id, index, list) => id && list.indexOf(id) === index);
    summary.connections.push({
      ...connection,
      used_by: linkedDocuments,
      document_count: connection.document_count ?? linkedDocuments.length,
      external_document_count: connection.external_document_count ?? linkedDocuments.length,
    });
    const provider = normalizeKnowledgeConnectionProvider(connection.provider);
    if (provider && !summary.providers.includes(provider)) summary.providers.push(provider);
  });

  summaries.forEach(summary => {
    summary.connections.sort((a, b) => a.name.localeCompare(b.name));
    summary.providers.sort();
  });

  return Array.from(summaries.values()).sort((a, b) => a.teamPath.localeCompare(b.teamPath));
}

export function normalizeKnowledgeConnectionProvider(provider: string) {
  const value = provider.trim().toLowerCase();
  if (value === 'notion') return 'notion';
  if (value === 'confluence') return 'confluence';
  if (value === 'wiki' || value === 'external' || value === 'external_page') return 'wiki';
  return value;
}

export function knowledgeConnectionProviderLabel(provider: string) {
  const value = normalizeKnowledgeConnectionProvider(provider);
  if (value === 'notion') return 'Notion';
  if (value === 'confluence') return 'Confluence';
  if (value === 'wiki') return 'Wiki page';
  return provider || 'External page';
}

export function knowledgeConnectionStatusLabel(status: string, disabled?: boolean) {
  if (disabled) return 'Disabled';
  switch (status) {
    case 'connected':
      return 'Connected';
    case 'authentication_required':
      return 'Auth required';
    case 'permission_denied':
      return 'Permission denied';
    case 'provider_unavailable':
      return 'Integration pending';
    case 'disabled':
      return 'Disabled';
    default:
      return status ? status.replace(/_/g, ' ') : 'Pending';
  }
}

export function knowledgeConnectionDisplayName(connection: Pick<KnowledgeConnectionListItem, 'display_name' | 'name'>) {
  return connection.display_name || connection.name;
}

export function knowledgeConnectionIdentifier(connection: Pick<KnowledgeConnectionListItem, 'id' | 'uuid'>) {
  return connection.id || connection.uuid || '';
}

export function knowledgeConnectionMatchesIdentifier(connection: KnowledgeConnectionListItem, identifier: string) {
  return connection.id === identifier || connection.uuid === identifier;
}

export function knowledgeSyncStatusLabel(status: string | undefined, isExternal = false): { label: string; tone: string } {
  if (!isExternal) return { label: '', tone: 'neutral' };
  switch ((status || '').toLowerCase()) {
    case 'up_to_date':
    case 'synced':
      return { label: 'Up to date', tone: 'green' };
    case 'cached':
      return { label: 'Cached', tone: 'green' };
    case 'syncing':
      return { label: 'Syncing', tone: 'blue' };
    case 'needs_sync':
      return { label: 'Needs sync', tone: 'amber' };
    case 'failed':
    case 'error':
      return { label: 'Failed', tone: 'amber' };
    case 'page_unavailable':
      return { label: 'Page unavailable', tone: 'amber' };
    case 'authentication_required':
      return { label: 'Auth required', tone: 'amber' };
    case 'not_synced':
    case '':
      return { label: 'Never synced', tone: 'neutral' };
    default:
      return { label: (status || 'Never synced').replace(/_/g, ' '), tone: 'neutral' };
  }
}

export function validateKnowledgeConnectionDraft(
  draft: KnowledgeConnectionDraft,
  existing: KnowledgeConnectionListItem[] = [],
  currentID = ''
) {
  const team = normalizeTeamPath(draft.team);
  const name = draft.name.trim();
  if (!team) return 'Team is required.';
  if (team.split('/').some(part => !part || part === '.' || part === '..')) return 'Team contains invalid path segments.';
  if (!knowledgeConnectionProviders.some(provider => provider.value === draft.provider)) return 'Choose a supported provider.';
  if (!name) return 'Connection name is required.';
  if (!/^[a-zA-Z0-9_.-]+$/.test(name)) return 'Connection name can only contain letters, numbers, dots, underscores, and hyphens.';
  const id = `${team}/${name}`;
  if (existing.some(connection => connection.id === id && connection.id !== currentID && connection.uuid !== currentID)) {
    return 'A connection with that identifier already exists.';
  }
  return '';
}

export function validateKnowledgeExternalDraft(
  draft: KnowledgeExternalPageDraft,
  team: string,
  connections: KnowledgeConnectionListItem[] = []
) {
  const connection = connections.find(item => knowledgeConnectionMatchesIdentifier(item, draft.connection_id));
  if (!connection) return 'Choose a knowledge connection for this team.';
  if (normalizeTeamPath(connection.team) !== normalizeTeamPath(team)) return 'The connection must belong to the selected team.';
  if (!draft.external_page_id.trim() && !draft.external_page_url.trim()) return 'Enter a provider page ID or page URL.';
  if (draft.external_page_url.trim()) {
    try {
      const parsed = new URL(draft.external_page_url.trim());
      if (!parsed.protocol.startsWith('http')) return 'Page URL must be an HTTP or HTTPS URL.';
    } catch {
      return 'Enter a valid page URL.';
    }
  }
  if (!knowledgeSyncModeOptions.some(option => option.value === draft.sync_mode)) return 'Choose a supported sync mode.';
  if (!knowledgeFailureModeOptions.some(option => option.value === draft.failure_mode)) return 'Choose a supported failure behavior.';
  if (draft.failure_mode === 'skip' && (team === '' || draft.sync_mode === 'before_run')) {
    return 'Skip is only safe for optional contexts; use Fail run for before-run required guardrails and policies.';
  }
  return '';
}

export function deriveKnowledgeConnectionName(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, '-')
    .replace(/^[._-]+|[._-]+$/g, '');
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
