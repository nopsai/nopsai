import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  ArrowLeft,
  BookOpen,
  Braces,
  Copy,
  Download,
  FileText,
  Folder,
  Lock,
  Plus,
  Search,
  ShieldCheck,
  Trash2,
  type LucideIcon,
} from 'lucide-react';

import ResourceAccessCard from '../components/ResourceAccessCard';
import { buildApiUrl } from '../lib/api';
import { fetchResourceGroupPaths, insertGroupPath } from '../lib/resourceGroups';

type KnowledgeContextListItem = {
  id: string;
  uuid?: string;
  kind: string;
  group: string;
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

type KnowledgeContextDetail = KnowledgeContextListItem & {
  content: string;
  managed_by_config_repo?: boolean;
};

type KnowledgeContextPageProps = {
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
};

const kindOrder = ['architecture', 'guardrail', 'policy', 'adr', 'guideline', 'runbook', 'reference', 'example'];

type KnowledgeFormModalState = {
  mode: 'create' | 'clone';
  kind: string;
  group: string;
  name: string;
  content: string;
  pending: boolean;
  error?: string;
};

type KnowledgeDeleteModalState = {
  id: string;
  name: string;
  pending: boolean;
  error?: string;
};

type ToastMessage = {
  id: number;
  message: string;
  tone: 'success' | 'error' | 'info';
};

type KnowledgeFolderNode = {
  id: string;
  name: string;
  fullPath: string;
  children: KnowledgeFolderNode[];
  docs: KnowledgeContextListItem[];
};

type KnowledgeDocumentParameters = Partial<Record<'name' | 'kind' | 'description', string>>;

type KnowledgeDraftSnapshot = {
  detail: KnowledgeContextDetail;
  content: string;
};

const KNOWLEDGE_CONTEXTS_CHANGED_EVENT = 'nopsai-knowledge-contexts-changed';
const knowledgeDraftStoragePrefix = 'nopsai.knowledge-context.draft.';

const emptyDraft: KnowledgeContextDetail = {
  id: 'architecture/team-1/new-document',
  kind: 'architecture',
  group: 'team-1',
  name: 'new-document',
  visibility: 'group',
  source: 'database',
  content: '',
};

function encodeKnowledgeID(id: string) {
  return id.split('/').map(encodeURIComponent).join('/');
}

function buildKnowledgeID(kind: string, group: string, name: string) {
  return [kind, group, name]
    .map(part => part.trim().replace(/^\/+|\/+$/g, ''))
    .filter(Boolean)
    .join('/');
}

function knowledgeDraftStorageKey(id: string) {
  return `${knowledgeDraftStoragePrefix}${id}`;
}

function saveKnowledgeDraft(snapshot: KnowledgeDraftSnapshot) {
  if (typeof sessionStorage === 'undefined') return;
  try {
    sessionStorage.setItem(knowledgeDraftStorageKey(snapshot.detail.id), JSON.stringify(snapshot));
  } catch {
    // Best effort only; refs still cover the normal in-memory route transition.
  }
}

function loadKnowledgeDraft(id: string): KnowledgeDraftSnapshot | null {
  if (typeof sessionStorage === 'undefined') return null;
  try {
    const raw = sessionStorage.getItem(knowledgeDraftStorageKey(id));
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<KnowledgeDraftSnapshot>;
    if (!parsed?.detail || parsed.detail.id !== id || typeof parsed.content !== 'string') return null;
    return parsed as KnowledgeDraftSnapshot;
  } catch {
    return null;
  }
}

function clearKnowledgeDraft(id: string) {
  if (typeof sessionStorage === 'undefined') return;
  try {
    sessionStorage.removeItem(knowledgeDraftStorageKey(id));
  } catch {
    // Ignore storage failures.
  }
}

function splitKnowledgePath(id: string) {
  const parts = id.split('/').filter(Boolean);
  const name = parts.pop() || '';
  return {
    name,
    folder: parts.join('/'),
  };
}

function normalizeFolderPath(value: string) {
  return value.trim().replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
}

function parentFolder(path: string) {
  const parts = normalizeFolderPath(path).split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

function buildKnowledgeTree(items: KnowledgeContextListItem[], groupPaths: string[]): KnowledgeFolderNode {
  const root: KnowledgeFolderNode = { id: 'root', name: 'Knowledge Context', fullPath: '', children: [], docs: [] };
  const nodes = new Map<string, KnowledgeFolderNode>([['', root]]);

  const ensureNode = (fullPath: string) => {
    const normalized = normalizeFolderPath(fullPath);
    if (nodes.has(normalized)) return nodes.get(normalized)!;
    const parentPath = parentFolder(normalized);
    const parent = ensureNode(parentPath);
    const name = normalized.split('/').filter(Boolean).pop() || 'root';
    const node: KnowledgeFolderNode = { id: normalized || 'root', name, fullPath: normalized, children: [], docs: [] };
    parent.children.push(node);
    nodes.set(normalized, node);
    return node;
  };

  kindOrder.forEach(kind => ensureNode(kind));
  kindOrder.forEach(kind => {
    groupPaths.forEach(groupPath => {
      const normalizedGroup = normalizeFolderPath(groupPath);
      if (!normalizedGroup) return;
      insertGroupPath(root, `${kind}/${normalizedGroup}`, (id, name, fullPath) => {
        const node: KnowledgeFolderNode = { id, name, fullPath, children: [], docs: [] };
        nodes.set(fullPath, node);
        return node;
      });
    });
  });

  for (const item of items) {
    const { folder } = splitKnowledgePath(item.id);
    ensureNode(folder).docs.push(item);
  }

  for (const node of nodes.values()) {
    node.children.sort((a, b) => {
      const ai = kindOrder.indexOf(a.name);
      const bi = kindOrder.indexOf(b.name);
      return (ai < 0 ? kindOrder.length : ai) - (bi < 0 ? kindOrder.length : bi) || a.name.localeCompare(b.name);
    });
    node.docs.sort((a, b) => a.name.localeCompare(b.name));
  }

  return root;
}

function findKnowledgeFolder(root: KnowledgeFolderNode, fullPath: string) {
  const normalized = normalizeFolderPath(fullPath);
  if (!normalized) return root;
  const segments = normalized.split('/').filter(Boolean);
  let current = root;
  for (const segment of segments) {
    const next = current.children.find(child => child.name === segment);
    if (!next) return root;
    current = next;
  }
  return current;
}

function countFolderDocs(node: KnowledgeFolderNode): number {
  return node.docs.length + node.children.reduce((total, child) => total + countFolderDocs(child), 0);
}

function decodeRouteID(pathname: string) {
  const prefix = '/knowledge-context/';
  if (!pathname.startsWith(prefix)) return '';
  return pathname
    .slice(prefix.length)
    .split('/')
    .filter(Boolean)
    .map(decodeURIComponent)
    .join('/');
}

function sourceLabel(source: string) {
  const value = source.toLowerCase();
  if (value.includes('git')) return 'GitOps';
  if (value.includes('repo')) return 'Repo';
  if (value.includes('database')) return 'Database';
  return 'UI';
}

function kindTitle(kind: string) {
  if (kind === 'adr') return 'ADR';
  return `${kind.charAt(0).toUpperCase()}${kind.slice(1)}`;
}

function kindPlural(kind: string) {
  if (kind === 'architecture') return 'Architecture';
  if (kind === 'adr') return 'ADRs';
  if (kind === 'policy') return 'Policies';
  if (kind === 'reference') return 'References';
  return `${kindTitle(kind)}s`;
}

function kindIcon(kind: string): LucideIcon {
  switch (kind) {
    case 'guardrail':
      return ShieldCheck;
    case 'policy':
      return Lock;
    case 'runbook':
      return Braces;
    case 'reference':
      return BookOpen;
    case 'example':
      return FileText;
    default:
      return FileText;
  }
}

function deriveIdentityFromFolder(activeFolder: string) {
  const parts = normalizeFolderPath(activeFolder).split('/').filter(Boolean);
  const first = parts[0] || '';
  if (kindOrder.includes(first)) {
    return {
      kind: first,
      group: parts.slice(1).join('/') || 'team-1',
    };
  }
  return {
    kind: 'architecture',
    group: parts.join('/') || 'team-1',
  };
}

function normalizeKnowledgeSource(source: string) {
  const label = sourceLabel(source).toLowerCase();
  if (label === 'gitops') return 'git';
  if (label === 'repo') return 'repo';
  if (label === 'database') return 'database';
  return label || 'database';
}

function formatKnowledgeDate(value?: string) {
  if (!value) return '-';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString();
}

function isGitManagedDocument(doc: Pick<KnowledgeContextListItem, 'source'> & { managed_by_config_repo?: boolean }) {
  return Boolean(doc.managed_by_config_repo) || normalizeKnowledgeSource(doc.source) === 'git';
}

function splitKnowledgeContentForPreview(content: string): { content: string; parameters: KnowledgeDocumentParameters } {
  const normalized = content.replace(/\r\n/g, '\n');
  if (normalized.startsWith('---\n')) {
    const end = normalized.indexOf('\n---', 4);
    if (end >= 0) {
      const bodyStart = end + (normalized[end + 4] === '\n' ? 5 : 4);
      return {
        content: normalized.slice(bodyStart),
        parameters: parseKnowledgeParameterLines(normalized.slice(4, end)),
      };
    }
  }

  const contentLine = findTopLevelKnowledgeKey(normalized, 'content');
  if (contentLine) {
    const header = normalized.slice(0, contentLine.start);
    const suffix = contentLine.text.slice('content:'.length).trim();
    const rawBody = normalized.slice(contentLine.end);
    return {
      content: suffix && !suffix.startsWith('|') && !suffix.startsWith('>') ? unquoteYAMLScalar(suffix) : removeCommonIndent(rawBody),
      parameters: parseKnowledgeParameterLines(header),
    };
  }

  const leading = splitLeadingKnowledgeParameters(normalized);
  if (leading) return leading;

  return { content, parameters: {} };
}

function findTopLevelKnowledgeKey(content: string, key: string): { start: number; end: number; text: string } | null {
  const lines = content.split('\n');
  let offset = 0;
  for (const line of lines) {
    const text = line.replace(/\r$/, '');
    if (text.startsWith(`${key}:`)) {
      const next = offset + line.length + 1;
      return { start: offset, end: Math.min(next, content.length), text };
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
  const body = separator >= 0 ? content.slice(separator + 2) : '';
  const headerLines = header.split('\n');
  if (headerLines.some(line => line.trim() && !line.startsWith(' ') && !line.startsWith('\t') && !isKnowledgeParameterLine(line))) {
    return null;
  }
  return {
    content: body,
    parameters: parseKnowledgeParameterLines(header),
  };
}

function isKnowledgeParameterLine(line: string) {
  const key = line.split(':', 1)[0]?.trim();
  return key === 'name' || key === 'kind' || key === 'description' || key === 'access';
}

function parseKnowledgeParameterLines(content: string): KnowledgeDocumentParameters {
  const parameters: KnowledgeDocumentParameters = {};
  for (const rawLine of content.split('\n')) {
    if (!rawLine || rawLine.startsWith(' ') || rawLine.startsWith('\t')) continue;
    const separator = rawLine.indexOf(':');
    if (separator < 0) continue;
    const key = rawLine.slice(0, separator).trim() as keyof KnowledgeDocumentParameters;
    if (key !== 'name' && key !== 'kind' && key !== 'description') continue;
    const value = unquoteYAMLScalar(rawLine.slice(separator + 1).trim());
    if (value) parameters[key] = value;
  }
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
    .map(line => {
      const match = line.match(/^[ \t]*/);
      return match ? match[0].length : 0;
    });
  const minIndent = indents.length ? Math.min(...indents) : 0;
  return minIndent ? lines.map(line => line.slice(Math.min(minIndent, line.match(/^[ \t]*/)?.[0].length || 0))).join('\n') : content;
}

function validateKnowledgeIdentity(kind: string, group: string, name: string, existingItems: KnowledgeContextListItem[], currentID?: string) {
  const normalizedKind = kind.trim();
  const normalizedGroup = normalizeFolderPath(group);
  const normalizedName = name.trim().replace(/\.(yaml|yml)$/i, '');
  if (!kindOrder.includes(normalizedKind)) return 'Choose a supported kind.';
  if (!normalizedGroup) return 'Group is required.';
  if (normalizedGroup.split('/').some(part => !part || part === '.' || part === '..')) return 'Group contains invalid path segments.';
  if (!normalizedName) return 'Document name is required.';
  if (!/^[a-zA-Z0-9_.-]+$/.test(normalizedName)) return 'Document name can only contain letters, numbers, dots, underscores, and hyphens.';
  const id = buildKnowledgeID(normalizedKind, normalizedGroup, normalizedName);
  if (id !== currentID && existingItems.some(item => item.id === id)) return 'A knowledge context with that identifier already exists.';
  return '';
}

function readError(response: Response, fallback: string) {
  return response.text().then(text => text.trim() || fallback);
}

export default function KnowledgeContextPage({ canWriteKnowledge, canDeleteKnowledge }: KnowledgeContextPageProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const selectedID = decodeRouteID(location.pathname);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const toastCounterRef = useRef(0);
  const editSessionOriginalRef = useRef<{ detail: KnowledgeContextDetail; content: string } | null>(null);
  const pendingDraftRef = useRef<{ detail: KnowledgeContextDetail; content: string } | null>(null);
  const [items, setItems] = useState<KnowledgeContextListItem[]>([]);
  const [listLoading, setListLoading] = useState(false);
  const [listError, setListError] = useState<string | null>(null);
  const [resourceGroupPaths, setResourceGroupPaths] = useState<string[]>([]);
  const [detail, setDetail] = useState<KnowledgeContextDetail | null>(null);
  const [editorValue, setEditorValue] = useState('');
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [search, setSearch] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [draftID, setDraftID] = useState<string | null>(null);
  const [isEditing, setIsEditing] = useState(false);
  const [formModal, setFormModal] = useState<KnowledgeFormModalState | null>(null);
  const [deleteModal, setDeleteModal] = useState<KnowledgeDeleteModalState | null>(null);
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const addToast = useCallback((message: string, tone: ToastMessage['tone'] = 'info') => {
    toastCounterRef.current += 1;
    const id = toastCounterRef.current;
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

  const loadList = useCallback(async () => {
    setListLoading(true);
    setListError(null);
    try {
      const response = await fetch(buildApiUrl('/v1/knowledge-contexts'), { cache: 'no-store' });
      if (!response.ok) throw new Error(await readError(response, `Unable to load knowledge contexts (${response.status})`));
      const payload = await response.json();
      setItems(Array.isArray(payload) ? payload : []);
      window.dispatchEvent(new Event(KNOWLEDGE_CONTEXTS_CHANGED_EVENT));
    } catch (err) {
      setListError(err instanceof Error ? err.message : 'Unable to load knowledge contexts');
    } finally {
      setListLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadList();
  }, [loadList]);

  useEffect(() => {
    let cancelled = false;
    const loadGroups = async () => {
      const paths = await fetchResourceGroupPaths();
      if (!cancelled) setResourceGroupPaths(paths);
    };
    void loadGroups();
    const handleGroupsChanged = () => void loadGroups();
    window.addEventListener('nopsai-resource-groups-changed', handleGroupsChanged);
    return () => {
      cancelled = true;
      window.removeEventListener('nopsai-resource-groups-changed', handleGroupsChanged);
    };
  }, []);

  useEffect(() => {
    if (!selectedID) {
      setDetail(null);
      setEditorValue('');
      setDraftID(null);
      setIsEditing(false);
      editSessionOriginalRef.current = null;
      pendingDraftRef.current = null;
      return;
    }
    const pendingDraft = pendingDraftRef.current;
    const originalDraft = editSessionOriginalRef.current;
    const storedDraft = loadKnowledgeDraft(selectedID);
    const activeDraft =
      pendingDraft?.detail.id === selectedID
        ? pendingDraft
        : originalDraft?.detail.id === selectedID && !originalDraft.detail.uuid
          ? originalDraft
          : storedDraft;
    if (activeDraft) {
      setDetail(activeDraft.detail);
      setEditorValue(activeDraft.content);
      setDraftID(selectedID);
      setIsEditing(true);
      editSessionOriginalRef.current = activeDraft;
      setDetailLoading(false);
      setDetailError(null);
      return;
    }
    if (draftID === selectedID) {
      setDetailLoading(false);
      setDetailError(null);
      return;
    }
    let cancelled = false;
    setDetailLoading(true);
    setDetailError(null);
    fetch(buildApiUrl(`/v1/knowledge-contexts/${encodeKnowledgeID(selectedID)}`), { cache: 'no-store' })
      .then(async response => {
        if (!response.ok) throw new Error(await readError(response, `Unable to load document (${response.status})`));
        return response.json();
      })
      .then(payload => {
        if (cancelled) return;
        setDetail(payload);
        setEditorValue(typeof payload?.content === 'string' ? payload.content : '');
        setDraftID(null);
        setIsEditing(false);
        editSessionOriginalRef.current = null;
      })
      .catch(err => {
        if (!cancelled) {
          setDetail(null);
          setDetailError(err instanceof Error ? err.message : 'Unable to load document');
        }
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [draftID, selectedID]);

  useEffect(() => {
    if (!draftID || !detail || detail.uuid || detail.id !== draftID) return;
    saveKnowledgeDraft({ detail, content: editorValue });
  }, [detail, draftID, editorValue]);

  const filteredItems = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return items;
    return items.filter(item => [item.kind, item.group, item.name, item.description].some(value => (value || '').toLowerCase().includes(term)));
  }, [items, search]);

  const activeFolder = useMemo(() => normalizeFolderPath(new URLSearchParams(location.search).get('folder') || ''), [location.search]);
  const knowledgeTree = useMemo(() => buildKnowledgeTree(items, resourceGroupPaths), [items, resourceGroupPaths]);
  const activeFolderNode = useMemo(() => findKnowledgeFolder(knowledgeTree, activeFolder), [activeFolder, knowledgeTree]);
  const visibleDocuments = useMemo(() => {
    if (search.trim()) return filteredItems;
    return activeFolderNode.docs;
  }, [activeFolderNode.docs, filteredItems, search]);
  const visibleFolders = search.trim() ? [] : activeFolderNode.children;
  const previewDocument = useMemo(() => splitKnowledgeContentForPreview(editorValue), [editorValue]);
  const previewContent = previewDocument.content;

  const contentMetrics = useMemo(() => {
    const trimmed = previewContent.trim();
    return {
      lines: previewContent ? previewContent.split('\n').length : 0,
      words: trimmed ? trimmed.split(/\s+/).filter(Boolean).length : 0,
      chars: previewContent.length,
    };
  }, [previewContent]);

  const openFolder = useCallback(
    (folder: string) => {
      const normalized = normalizeFolderPath(folder);
      navigate(normalized ? `/knowledge-context?folder=${encodeURIComponent(normalized)}` : '/knowledge-context');
    },
    [navigate]
  );

  const handleSelectDocument = useCallback(
    (id: string) => {
      navigate(`/knowledge-context/${encodeKnowledgeID(id)}`);
    },
    [navigate]
  );

  const handleBackToList = useCallback(() => {
    const folder = detail ? splitKnowledgePath(detail.id).folder : activeFolder;
    openFolder(folder);
  }, [activeFolder, detail, openFolder]);

  const openCreateModal = useCallback(() => {
    if (!canWriteKnowledge) {
      addToast('You have read-only access to knowledge contexts.', 'info');
      return;
    }
    const identity = deriveIdentityFromFolder(activeFolder);
    setFormModal({
      mode: 'create',
      kind: identity.kind,
      group: identity.group,
      name: '',
      content: '',
      pending: false,
    });
  }, [activeFolder, addToast, canWriteKnowledge]);

  const openCloneModal = useCallback(() => {
    if (!detail) {
      addToast('Select a knowledge context to clone.', 'info');
      return;
    }
    if (!canWriteKnowledge) {
      addToast('You have read-only access to knowledge contexts.', 'info');
      return;
    }
    let candidateName = `${detail.name || 'document'}-copy`;
    let suffix = 2;
    while (items.some(item => item.id === buildKnowledgeID(detail.kind, detail.group, candidateName))) {
      candidateName = `${detail.name || 'document'}-copy-${suffix}`;
      suffix += 1;
    }
    setFormModal({
      mode: 'clone',
      kind: detail.kind,
      group: detail.group,
      name: candidateName,
      content: editorValue,
      pending: false,
    });
  }, [addToast, canWriteKnowledge, detail, editorValue, items]);

  const submitFormModal = useCallback(() => {
    if (!formModal) return;
    const error = validateKnowledgeIdentity(formModal.kind, formModal.group, formModal.name, items);
    if (error) {
      setFormModal(prev => (prev ? { ...prev, error } : prev));
      return;
    }
    const kind = formModal.kind.trim();
    const group = normalizeFolderPath(formModal.group);
    const name = formModal.name.trim().replace(/\.(yaml|yml)$/i, '');
    const id = buildKnowledgeID(kind, group, name);
    const content = formModal.content || '';
    const draft: KnowledgeContextDetail = {
      ...emptyDraft,
      id,
      kind,
      group,
      name,
      description: '',
      visibility: 'group',
      source: 'database',
      content,
      managed_by_config_repo: false,
      used_by: [],
      used_by_count: 0,
    };
    setDetail(draft);
    setEditorValue(content);
    setDraftID(id);
    setIsEditing(true);
    editSessionOriginalRef.current = { detail: draft, content };
    pendingDraftRef.current = { detail: draft, content };
    saveKnowledgeDraft({ detail: draft, content });
    setDetailError(null);
    setFormModal(null);
    navigate(`/knowledge-context/${encodeKnowledgeID(id)}`);
    addToast(formModal.mode === 'clone' ? 'Draft knowledge context cloned.' : 'Draft knowledge context created.', 'success');
  }, [addToast, formModal, items, navigate]);

  const startEditing = useCallback(() => {
    if (!detail) return;
    if (!canWriteKnowledge || isGitManagedDocument(detail)) {
      addToast('This knowledge context is read-only. Clone it to customize.', 'info');
      return;
    }
    editSessionOriginalRef.current = { detail: { ...detail }, content: editorValue };
    setIsEditing(true);
  }, [addToast, canWriteKnowledge, detail, editorValue]);

  const discardEditing = useCallback(() => {
    const original = editSessionOriginalRef.current;
    if (original) {
      setDetail(original.detail);
      setEditorValue(original.content);
    }
    setIsEditing(false);
    setDetailError(null);
    editSessionOriginalRef.current = null;
  }, []);

  const saveDetail = useCallback(async () => {
    if (!detail || !canWriteKnowledge || saving) return;
    if (isGitManagedDocument(detail)) {
      addToast('Git-managed knowledge contexts are read-only. Clone it to customize.', 'info');
      return;
    }
    const validationError = validateKnowledgeIdentity(detail.kind, detail.group, detail.name, items, draftID || detail.id);
    if (validationError) {
      setDetailError(validationError);
      addToast('Resolve the document identity before saving.', 'error');
      return;
    }
    setSaving(true);
    setDetailError(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/knowledge-contexts/${encodeKnowledgeID(detail.id)}`), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          kind: detail.kind,
          group: detail.group,
          name: detail.name,
          description: detail.description || '',
          content: editorValue,
        }),
      });
      if (!response.ok) throw new Error(await readError(response, `Unable to save document (${response.status})`));
      const payload = await response.json();
      setDetail(payload);
      setEditorValue(payload.content || '');
      setDraftID(null);
      setIsEditing(false);
      editSessionOriginalRef.current = null;
      pendingDraftRef.current = null;
      clearKnowledgeDraft(detail.id);
      if (payload.id && payload.id !== detail.id) clearKnowledgeDraft(payload.id);
      navigate(`/knowledge-context/${encodeKnowledgeID(payload.id)}`, { replace: true });
      await loadList();
      addToast('Knowledge context saved.', 'success');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Unable to save document';
      setDetailError(message);
      addToast(message, 'error');
    } finally {
      setSaving(false);
    }
  }, [addToast, canWriteKnowledge, detail, draftID, editorValue, items, loadList, navigate, saving]);

  const openDeleteModal = useCallback(
    (doc: Pick<KnowledgeContextListItem, 'id' | 'name' | 'source'> & { managed_by_config_repo?: boolean }) => {
      if (!canDeleteKnowledge) {
        addToast('You do not have permission to delete knowledge contexts.', 'info');
        return;
      }
      if (isGitManagedDocument(doc)) {
        addToast('This knowledge context is managed via Git. Clone it to customize instead of deleting.', 'info');
        return;
      }
      setDeleteModal({ id: doc.id, name: doc.name || doc.id, pending: false });
    },
    [addToast, canDeleteKnowledge]
  );

  const confirmDelete = useCallback(async () => {
    if (!deleteModal) return;
    setSaving(true);
    setDetailError(null);
    setDeleteModal(prev => (prev ? { ...prev, pending: true, error: undefined } : prev));
    try {
      if (deleteModal.id === draftID && !detail?.uuid) {
        setDetail(null);
        setEditorValue('');
        setDraftID(null);
        setIsEditing(false);
        editSessionOriginalRef.current = null;
        pendingDraftRef.current = null;
        clearKnowledgeDraft(deleteModal.id);
      } else {
        const response = await fetch(buildApiUrl(`/v1/knowledge-contexts/${encodeKnowledgeID(deleteModal.id)}`), { method: 'DELETE' });
        if (!response.ok) throw new Error(await readError(response, `Unable to delete document (${response.status})`));
        await loadList();
      }
      setDeleteModal(null);
      addToast('Knowledge context deleted.', 'success');
      if (detail?.id === deleteModal.id) {
        openFolder(splitKnowledgePath(deleteModal.id).folder);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Unable to delete document';
      setDeleteModal(prev => (prev ? { ...prev, pending: false, error: message } : prev));
      addToast(message, 'error');
    } finally {
      setSaving(false);
    }
  }, [addToast, deleteModal, detail?.id, detail?.uuid, draftID, loadList, openFolder]);

  const handleCopy = useCallback(async () => {
    if (!detail) return;
    try {
      await navigator.clipboard.writeText(previewContent);
      setDetailError(null);
      addToast('Content copied to clipboard.', 'success');
    } catch {
      setDetailError('Unable to copy document content.');
      addToast('Unable to copy content.', 'error');
    }
  }, [addToast, detail, previewContent]);

  const handleDownload = useCallback(() => {
    if (!detail) return;
    const blob = new Blob([previewContent], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${detail.name || 'knowledge-context'}.txt`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  }, [detail, previewContent]);

  const handleAccessChange = useCallback((access: { resource_id?: string; visibility?: string }) => {
    const visibility = access.visibility;
    const resourceID = access.resource_id || detail?.id;
    if (!visibility || !resourceID) return;
    setDetail(prev => (prev && prev.id === resourceID ? { ...prev, visibility } : prev));
    setItems(prev => prev.map(item => (item.id === resourceID ? { ...item, visibility } : item)));
  }, [detail?.id]);

  const canEditSelected = Boolean(detail && canWriteKnowledge && !isGitManagedDocument(detail));
  const selectedCanEdit = Boolean(canEditSelected && isEditing);
  const CurrentKindIcon = detail ? kindIcon(detail.kind) : FileText;

  const renderFolderCard = (node: KnowledgeFolderNode) => {
    const folderDepth = node.fullPath.split('/').filter(Boolean).length;
    const Icon = folderDepth === 1 ? kindIcon(node.name) : Folder;
    const folderName = folderDepth === 1 ? kindPlural(node.name) : node.name;
    return (
      <article
        key={`folder-${node.id}`}
        className="glass-card pipeline-card kc-folder-card border border-[var(--border-primary)] rounded-xl p-4"
        onClick={() => openFolder(node.fullPath)}
      >
        <div className="pipeline-card-header">
          <div className="pipeline-card-info">
            <span className="pipeline-card-icon" aria-hidden="true">
              <Icon />
            </span>
            <div className="pipeline-card-text">
              <h3 className="pipeline-card-title">{folderName}</h3>
              <p className="pipeline-card-path">{node.fullPath || 'root'}</p>
              <p className="pipeline-card-description">Knowledge folder</p>
            </div>
          </div>
          <span className="pipeline-folder-chevron">›</span>
        </div>
        <div className="pipeline-card-meta">
          <div className="pipeline-card-meta-row">
            <span className="pipeline-card-meta-label">Documents</span>
            <span className="pipeline-card-meta-value">{countFolderDocs(node)}</span>
          </div>
          <div className="pipeline-card-meta-row">
            <span className="pipeline-card-meta-label">Sub groups</span>
            <span className="pipeline-card-meta-value">{node.children.length}</span>
          </div>
        </div>
      </article>
    );
  };

  const renderDocumentCard = (doc: KnowledgeContextListItem) => {
    const Icon = kindIcon(doc.kind);
    const { folder } = splitKnowledgePath(doc.id);
    const canDeleteThisDocument = canDeleteKnowledge && !isGitManagedDocument(doc);
    return (
      <article
        key={doc.id}
        className={`glass-card pipeline-card kc-document-card border border-[var(--border-primary)] rounded-xl p-4 ${selectedID === doc.id ? 'kc-document-card--active' : ''}`}
        onClick={() => handleSelectDocument(doc.id)}
      >
        <div className="pipeline-card-header">
          <div className="pipeline-card-info">
            <span className="pipeline-card-icon" aria-hidden="true">
              <Icon />
            </span>
            <div className="pipeline-card-text">
              <h3 className="pipeline-card-title">{doc.name}</h3>
              <p className="pipeline-card-path">{folder || 'root'}</p>
              <p className="pipeline-card-description">{doc.description || `${kindTitle(doc.kind)} knowledge context.`}</p>
            </div>
          </div>
          <div className="pipeline-card-actions">
            {canDeleteThisDocument ? (
              <button
                type="button"
                className="pipelines-delete-button"
                title="Delete knowledge context"
                onClick={event => {
                  event.stopPropagation();
                  openDeleteModal(doc);
                }}
                aria-label="Delete knowledge context"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            ) : null}
          </div>
        </div>
        <div className="pipeline-card-meta">
          <div className="pipeline-card-meta-row">
            <span className="pipeline-card-meta-label">Source</span>
            <span className="pipeline-card-meta-value">{normalizeKnowledgeSource(doc.source)}</span>
          </div>
          <div className="pipeline-card-meta-row">
            <span className="pipeline-card-meta-label">Used by</span>
            <span className="pipeline-card-meta-value">{doc.used_by_count || 0}</span>
          </div>
        </div>
      </article>
    );
  };

  const renderList = () => (
    <div id="knowledge-context-list-view" className="pipelines-view">
      <div className="space-y-3">
        {listLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading knowledge contexts...</div>
        ) : listError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load knowledge contexts: {listError}</div>
        ) : (
          <>
            {search.trim() ? (
              <div className="triggers-search-summary">
                Showing {visibleDocuments.length} result{visibleDocuments.length === 1 ? '' : 's'} for "{search.trim()}"
              </div>
            ) : null}

            {visibleDocuments.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines">{visibleDocuments.map(doc => renderDocumentCard(doc))}</div>
            ) : null}

            {search.trim() ? null : visibleFolders.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">
                {visibleFolders.map(folder => renderFolderCard(folder))}
              </div>
            ) : null}

            {!visibleDocuments.length && !visibleFolders.length ? (
              <div id="knowledge-context-empty" className="pipelines-empty">
                <h3 className="text-base font-semibold text-[var(--text-primary)]">No knowledge contexts found</h3>
                <p className="text-sm text-[var(--text-secondary)]">
                  {canWriteKnowledge ? 'Create a new document or adjust your filters.' : 'Adjust your filters or browse another group.'}
                </p>
              </div>
            ) : null}
          </>
        )}
      </div>
    </div>
  );

  const renderDetail = () => {
    if (!detail) {
      return (
        <div id="knowledge-context-detail-view" className="pipelines-view">
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Select a knowledge context to see details.</div>
        </div>
      );
    }

    const nameLabel = detail.name || detail.id;
    const descriptionLabel = (detail.description || '').trim();
    const source = normalizeKnowledgeSource(detail.source);
    const isDraftDocument = Boolean(draftID && !detail.uuid);
    const updatedLabel = isDraftDocument ? 'Unsaved' : formatKnowledgeDate(detail.updated_at);

    return (
      <div id="knowledge-context-detail-view" className="pipelines-view">
        <div className="min-w-0 space-y-6">
          <div className="glass-card p-6">
            <div className="flex items-start justify-between gap-4 w-full mb-4">
              <div className="min-w-0 flex items-start gap-3">
                <span className={`kc-document-header__icon kc-kind-mark kc-kind-mark--${detail.kind} mt-1`} aria-hidden="true">
                  <CurrentKindIcon className="h-5 w-5" />
                </span>
                <div className="min-w-0">
                  <h2 id="knowledge-context-detail-name" className="text-3xl font-bold text-[var(--text-primary)] truncate">
                    {nameLabel}
                  </h2>
                  {descriptionLabel ? (
                    <p id="knowledge-context-detail-description" className="text-sm text-[var(--text-secondary)] mt-1">
                      {descriptionLabel}
                    </p>
                  ) : null}
                  <dl className="kc-header-details">
                    <div>
                      <dt>Identifier</dt>
                      <dd>{detail.id}</dd>
                    </div>
                    <div>
                      <dt>Kind</dt>
                      <dd>{detail.kind}</dd>
                    </div>
                    <div>
                      <dt>Group</dt>
                      <dd>{detail.group || 'Root'}</dd>
                    </div>
                    <div>
                      <dt>Source</dt>
                      <dd>{isDraftDocument ? 'draft' : source}</dd>
                    </div>
                    <div>
                      <dt>Updated</dt>
                      <dd>{updatedLabel}</dd>
                    </div>
                  </dl>
                </div>
              </div>
              <button id="knowledge-context-back-btn" className="glass-button-ghost" onClick={handleBackToList}>
                <ArrowLeft className="h-4 w-4" />
                <span>Back to list</span>
              </button>
            </div>
          </div>

          {detailError ? <div className="kc-alert kc-alert--error">{detailError}</div> : null}

          <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(16rem,1fr)]">
            <div className="min-w-0 space-y-6">
              <div className="glass-card overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-[var(--border-primary)]">
                  <div className="kc-editor-heading">
                    <h3 className="text-lg font-semibold text-[var(--text-primary)]">Knowledge Content</h3>
                    <span className="kc-editor-stat">{contentMetrics.words} words</span>
                  </div>
                  <div className="flex items-center gap-2 flex-wrap">
                    {!isEditing ? (
                      <>
                        <button className="glass-button-ghost" onClick={handleCopy} title="Copy content" aria-label="Copy content">
                          <Copy className="h-4 w-4" />
                        </button>
                        <button className="glass-button-ghost" onClick={handleDownload} title="Download content" aria-label="Download content">
                          <Download className="h-4 w-4" />
                        </button>
                        {detail.uuid && !draftID ? (
                          <ResourceAccessCard
                            resourceType="knowledge_context"
                            resourceID={detail.id}
                            label="knowledge context"
                            buttonClassName="glass-button-ghost"
                            onAccessChange={handleAccessChange}
                          />
                        ) : null}
                        {canEditSelected ? (
                          <button className="glass-button-primary" onClick={startEditing}>
                            Edit
                          </button>
                        ) : null}
                        {canWriteKnowledge ? (
                          <button className={isGitManagedDocument(detail) ? 'glass-button-primary' : 'glass-button-subtle'} onClick={openCloneModal}>
                            Clone
                          </button>
                        ) : null}
                      </>
                    ) : (
                      <>
                        <button className="glass-button-ghost" onClick={discardEditing}>
                          Discard
                        </button>
                        <button className="glass-button-primary" onClick={saveDetail} disabled={saving}>
                          {saving ? 'Saving...' : 'Save'}
                        </button>
                      </>
                    )}
                    {canDeleteKnowledge && !isGitManagedDocument(detail) ? (
                      <button className="glass-button-danger" onClick={() => openDeleteModal(detail)} disabled={saving}>
                        <Trash2 className="h-4 w-4" />
                        Delete
                      </button>
                    ) : null}
                  </div>
                </div>

                {isEditing ? (
                  <div className="kc-description-editor border-b border-[var(--border-primary)] p-4">
                    <label className="block text-sm font-medium text-[var(--text-secondary)]" htmlFor="knowledge-context-description">
                      Description
                    </label>
                    <textarea
                      id="knowledge-context-description"
                      className="kc-description-input mt-2"
                      value={detail.description || ''}
                      disabled={!selectedCanEdit}
                      onChange={event => setDetail(current => (current ? { ...current, description: event.target.value } : current))}
                      placeholder="Optional description"
                    />
                  </div>
                ) : null}

                <div className="kc-editor-body">
                  {isEditing ? (
                    <textarea
                      className="kc-editor-textarea"
                      value={editorValue}
                      disabled={!selectedCanEdit}
                      onChange={event => setEditorValue(event.target.value)}
                      spellCheck={false}
                    />
                  ) : (
                    <TextPreview content={previewContent} />
                  )}
                </div>
              </div>
            </div>

            <div className="min-w-0 space-y-6">
              <div className="glass-card overflow-hidden">
                <div className="p-4 border-b border-[var(--border-primary)]">
                  <h3 className="text-lg font-semibold text-[var(--text-primary)]">Used in Pipelines</h3>
                  <p className="text-xs text-[var(--text-secondary)] mt-1">Pipelines currently referencing this knowledge context.</p>
                </div>
                <div className="p-4">
                  {detail.used_by?.length ? (
                    <ul className={`triggers-pipeline-list ${detail.used_by.length > 5 ? 'triggers-pipelines-scroll' : ''}`}>
                      {detail.used_by.map(pipelineID => (
                        <li key={pipelineID} className="triggers-pipeline-item">
                          <button
                            type="button"
                            className="triggers-pipeline-link"
                            title={`Open ${pipelineID}`}
                            onClick={() => navigate(`/pipelines/${pipelineID.split('/').map(encodeURIComponent).join('/')}`)}
                          >
                            <span className="triggers-pipeline-name">{pipelineID}</span>
                            <dl className="triggers-detail-grid triggers-pipeline-details">
                              <dt className="triggers-detail-label">Action:</dt>
                              <dd className="triggers-detail-value">Open pipeline</dd>
                            </dl>
                          </button>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="text-sm text-[var(--text-secondary)]">No pipelines reference this document.</p>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  };

  return (
    <div data-page="knowledge-context" className="active h-full flex flex-col">
      {!selectedID && (
        <div className="px-6 pt-6 pb-4">
          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              className="glass-button-ghost"
              aria-label="Back"
              onClick={() => openFolder(parentFolder(activeFolder))}
              disabled={!activeFolder}
            >
              <ArrowLeft className="h-4 w-4" />
            </button>
            <div className={`pipelines-search-shell ${searchOpen ? 'open' : ''}`}>
              <button
                type="button"
                className="pipelines-search-toggle"
                aria-label="Search knowledge contexts"
                onClick={() => {
                  setSearchOpen(true);
                  requestAnimationFrame(() => searchInputRef.current?.focus());
                }}
              >
                <Search className="h-4 w-4" />
              </button>
              <input
                ref={searchInputRef}
                id="knowledge-context-search"
                type="text"
                placeholder="Search knowledge contexts"
                className="pipelines-search-input"
                value={search}
                onChange={event => {
                  setSearch(event.target.value);
                  if (event.target.value && !searchOpen) setSearchOpen(true);
                }}
                onBlur={() => {
                  if (!search.trim()) setSearchOpen(false);
                }}
              />
              {(search || searchOpen) && (
                <button
                  type="button"
                  className="pipelines-search-clear"
                  onClick={() => {
                    setSearch('');
                    setSearchOpen(false);
                    searchInputRef.current?.blur();
                  }}
                  aria-label="Clear search"
                >
                  x
                </button>
              )}
            </div>
            {canWriteKnowledge ? (
              <button id="knowledge-context-new-btn" type="button" className="pipelines-icon-only" aria-label="Create new knowledge context" title="New Knowledge Context" onClick={openCreateModal}>
                <Plus className="h-4 w-4" />
              </button>
            ) : null}
          </div>
        </div>
      )}

      <div className="flex-1 overflow-auto px-6 pb-8 triggers-content">
        {!selectedID ? renderList() : detailLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading knowledge context...</div>
        ) : detailError && !detail ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load knowledge context: {detailError}</div>
        ) : (
          renderDetail()
        )}
      </div>

      {formModal && (
        <div id={formModal.mode === 'create' ? 'knowledge-context-new-modal' : 'knowledge-context-clone-modal'} className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show">
          <div className="pipelines-modal-card max-w-lg w-full">
            <header className="pipelines-modal-header">
              <div>
                <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">
                  {formModal.mode === 'create' ? 'New knowledge context' : 'Clone knowledge context'}
                </p>
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">
                  {formModal.mode === 'create' ? 'Create document' : 'Clone document'}
                </h3>
              </div>
              <button className="glass-button-ghost" onClick={() => setFormModal(null)} disabled={formModal.pending}>
                Close
              </button>
            </header>
            <div className="pipelines-modal-body space-y-4">
              <div className="grid gap-3 sm:grid-cols-[180px_1fr]">
                <label className="block text-sm font-medium text-[var(--text-secondary)]">
                  Kind
                  <select className="pipelines-input w-full mt-1" value={formModal.kind} onChange={event => setFormModal(prev => (prev ? { ...prev, kind: event.target.value, error: undefined } : prev))}>
                    {kindOrder.map(kind => (
                      <option key={kind} value={kind}>{kind}</option>
                    ))}
                  </select>
                </label>
                <label className="block text-sm font-medium text-[var(--text-secondary)]">
                  Group
                  <input
                    className="pipelines-input w-full mt-1"
                    placeholder="team-1"
                    value={formModal.group}
                    onChange={event => setFormModal(prev => (prev ? { ...prev, group: event.target.value, error: undefined } : prev))}
                  />
                </label>
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)]">
                  Name
                  <input
                    className="pipelines-input w-full mt-1"
                    placeholder="repo-check"
                    value={formModal.name}
                    onChange={event => setFormModal(prev => (prev ? { ...prev, name: event.target.value, error: undefined } : prev))}
                  />
                </label>
              </div>
              {formModal.error ? <p className="text-sm text-red-500">{formModal.error}</p> : null}
            </div>
            <div className="pipelines-modal-footer">
              <div className="pipelines-modal-actions">
                <button className="glass-button-ghost" onClick={() => setFormModal(null)} disabled={formModal.pending}>
                  Cancel
                </button>
                <button className="glass-button-primary" onClick={submitFormModal} disabled={formModal.pending}>
                  {formModal.mode === 'clone' ? 'Clone' : 'Create'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {deleteModal && (
        <div id="knowledge-context-delete-modal" className="fixed inset-0 bg-[var(--bg-overlay)] flex items-center justify-center z-50 show">
          <div className="pipelines-modal-card max-w-md w-full">
            <header className="pipelines-modal-header">
              <div>
                <p className="pipelines-modal-kicker text-xs text-[var(--text-secondary)]">Delete knowledge context</p>
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">Remove {deleteModal.name}?</h3>
              </div>
              <button className="glass-button-ghost" onClick={() => setDeleteModal(null)} disabled={deleteModal.pending}>
                Close
              </button>
            </header>
            <div className="pipelines-modal-body space-y-3">
              <p className="text-sm text-[var(--text-secondary)]">This action cannot be undone.</p>
              {deleteModal.error ? <p className="text-sm text-red-500">{deleteModal.error}</p> : null}
            </div>
            <div className="pipelines-modal-footer">
              <div className="pipelines-modal-actions">
                <button className="glass-button-ghost" onClick={() => setDeleteModal(null)} disabled={deleteModal.pending}>
                  Cancel
                </button>
                <button className="glass-button-danger" onClick={confirmDelete} disabled={deleteModal.pending}>
                  {deleteModal.pending ? 'Deleting...' : 'Delete'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {toasts.length > 0 && (
        <div className="fixed top-6 right-6 z-[100] w-full max-w-sm space-y-3">
          {toasts.map(toast => (
            <div key={toast.id} className={`pipelines-toast pipelines-toast--${toast.tone} show`}>
              <div className="pipelines-toast__content">{toast.message}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function TextPreview({ content }: { content: string }) {
  if (!content.trim()) return <article className="kc-markdown kc-markdown--empty">No content</article>;
  return (
    <article className="kc-markdown">
      <pre>
        <code>{content}</code>
      </pre>
    </article>
  );
}
