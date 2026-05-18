import { useCallback, useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Eye, FileText, Plus, Save, Search, Trash2 } from 'lucide-react';

import ResourceAccessCard from '../components/ResourceAccessCard';
import { buildApiUrl } from '../lib/api';

type KnowledgeContextListItem = {
  id: string;
  uuid?: string;
  kind: string;
  group: string;
  name: string;
  title: string;
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
  content_format: string;
  managed_by_config_repo?: boolean;
};

type KnowledgeContextPageProps = {
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
};

const kindOrder = ['architecture', 'guardrail', 'policy', 'adr', 'guideline', 'runbook', 'reference', 'example'];

const emptyDraft: KnowledgeContextDetail = {
  id: 'architecture/team-1/new-document',
  kind: 'architecture',
  group: 'team-1',
  name: 'new-document',
  title: 'New Document',
  visibility: 'group',
  source: 'database',
  content: '',
  content_format: 'markdown',
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
  return 'UI';
}

function kindLabel(kind: string) {
  if (kind === 'architecture') return 'Architecture';
  if (kind === 'adr') return 'ADRs';
  if (kind === 'policy') return 'Policies';
  return `${kind.charAt(0).toUpperCase()}${kind.slice(1)}s`;
}

function formatDate(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleString();
}

function readError(response: Response, fallback: string) {
  return response.text().then(text => text.trim() || fallback);
}

export default function KnowledgeContextPage({ canWriteKnowledge, canDeleteKnowledge }: KnowledgeContextPageProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const selectedID = decodeRouteID(location.pathname);
  const [items, setItems] = useState<KnowledgeContextListItem[]>([]);
  const [listLoading, setListLoading] = useState(false);
  const [listError, setListError] = useState<string | null>(null);
  const [detail, setDetail] = useState<KnowledgeContextDetail | null>(null);
  const [editorValue, setEditorValue] = useState('');
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [search, setSearch] = useState('');
  const [mode, setMode] = useState<'edit' | 'preview'>('edit');
  const [draftID, setDraftID] = useState<string | null>(null);

  const loadList = useCallback(async () => {
    setListLoading(true);
    setListError(null);
    try {
      const response = await fetch(buildApiUrl('/v1/knowledge-contexts'), { cache: 'no-store' });
      if (!response.ok) throw new Error(await readError(response, `Unable to load knowledge contexts (${response.status})`));
      const payload = await response.json();
      setItems(Array.isArray(payload) ? payload : []);
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
    if (!selectedID) {
      setDetail(null);
      setEditorValue('');
      setDraftID(null);
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

  const filteredItems = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return items;
    return items.filter(item => [item.kind, item.group, item.name, item.title, item.description].some(value => (value || '').toLowerCase().includes(term)));
  }, [items, search]);

  const grouped = useMemo(() => {
    const groups = new Map<string, Map<string, KnowledgeContextListItem[]>>();
    for (const item of filteredItems) {
      if (!groups.has(item.kind)) groups.set(item.kind, new Map());
      const groupMap = groups.get(item.kind)!;
      const group = item.group || 'ungrouped';
      if (!groupMap.has(group)) groupMap.set(group, []);
      groupMap.get(group)!.push(item);
    }
    for (const groupMap of groups.values()) {
      for (const docs of groupMap.values()) docs.sort((a, b) => a.name.localeCompare(b.name));
    }
    return Array.from(groups.entries()).sort((a, b) => {
      const ai = kindOrder.indexOf(a[0]);
      const bi = kindOrder.indexOf(b[0]);
      return (ai < 0 ? kindOrder.length : ai) - (bi < 0 ? kindOrder.length : bi) || a[0].localeCompare(b[0]);
    });
  }, [filteredItems]);

  const beginCreate = useCallback(() => {
    const draft = { ...emptyDraft };
    setDetail(draft);
    setEditorValue('');
    setDraftID(draft.id);
    navigate(`/knowledge-context/${encodeKnowledgeID(draft.id)}`);
  }, [navigate]);

  const updateDraftIdentity = useCallback((field: 'kind' | 'group' | 'name', value: string) => {
    setDetail(prev => {
      if (!prev) return prev;
      const next = { ...prev, [field]: value };
      next.id = buildKnowledgeID(next.kind, next.group, next.name);
      return next;
    });
  }, []);

  const saveDetail = useCallback(async () => {
    if (!detail || !canWriteKnowledge || saving) return;
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
          title: detail.title,
          description: detail.description || '',
          content: editorValue,
          content_format: 'markdown',
          visibility: detail.visibility || 'group',
        }),
      });
      if (!response.ok) throw new Error(await readError(response, `Unable to save document (${response.status})`));
      const payload = await response.json();
      setDetail(payload);
      setEditorValue(payload.content || '');
      setDraftID(null);
      navigate(`/knowledge-context/${encodeKnowledgeID(payload.id)}`, { replace: true });
      await loadList();
    } catch (err) {
      setDetailError(err instanceof Error ? err.message : 'Unable to save document');
    } finally {
      setSaving(false);
    }
  }, [canWriteKnowledge, detail, editorValue, loadList, navigate, saving]);

  const deleteDetail = useCallback(async () => {
    if (!detail || !canDeleteKnowledge || saving) return;
    setSaving(true);
    setDetailError(null);
    try {
      const response = await fetch(buildApiUrl(`/v1/knowledge-contexts/${encodeKnowledgeID(detail.id)}`), { method: 'DELETE' });
      if (!response.ok) throw new Error(await readError(response, `Unable to delete document (${response.status})`));
      await loadList();
      navigate('/knowledge-context');
    } catch (err) {
      setDetailError(err instanceof Error ? err.message : 'Unable to delete document');
    } finally {
      setSaving(false);
    }
  }, [canDeleteKnowledge, detail, loadList, navigate, saving]);

  return (
    <div className="h-full grid grid-cols-1 lg:grid-cols-[minmax(260px,320px)_1fr] bg-[var(--bg-primary)] text-[var(--text-primary)]">
      <aside className="border-r border-[var(--border-primary)] bg-[var(--bg-secondary)] overflow-y-auto">
        <div className="p-4 border-b border-[var(--border-primary)] space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h1 className="text-lg font-semibold">Knowledge Context</h1>
            {canWriteKnowledge && (
              <button className="glass-button-ghost inline-flex items-center gap-2 px-3 py-1.5 text-sm" onClick={beginCreate}>
                <Plus className="h-4 w-4" />
                New
              </button>
            )}
          </div>
          <label className="relative block">
            <Search className="absolute left-3 top-2.5 h-4 w-4 text-[var(--text-secondary)]" />
            <input
              className="form-input w-full pl-9"
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder="Search"
            />
          </label>
        </div>
        <div className="p-3">
          {listLoading ? <p className="text-sm text-[var(--text-secondary)] px-2 py-3">Loading...</p> : null}
          {listError ? <p className="text-sm text-red-500 px-2 py-3">{listError}</p> : null}
          {!listLoading && grouped.length === 0 ? <p className="text-sm text-[var(--text-secondary)] px-2 py-3">No documents</p> : null}
          <div className="space-y-4">
            {grouped.map(([kind, groupMap]) => (
              <section key={kind} className="space-y-2">
                <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)] px-2">{kindLabel(kind)}</p>
                {Array.from(groupMap.entries()).map(([group, docs]) => (
                  <div key={`${kind}-${group}`} className="space-y-1">
                    <p className="text-xs text-[var(--text-secondary)] px-2">{group}</p>
                    {docs.map(doc => (
                      <button
                        key={doc.id}
                        onClick={() => navigate(`/knowledge-context/${encodeKnowledgeID(doc.id)}`)}
                        className={`w-full text-left rounded-md px-2 py-2 hover:bg-[var(--bg-tertiary)] ${selectedID === doc.id ? 'bg-[var(--bg-tertiary)]' : ''}`}
                      >
                        <span className="flex items-center gap-2 text-sm font-medium">
                          <FileText className="h-4 w-4 text-[var(--text-secondary)]" />
                          <span className="truncate">{doc.title || doc.name}</span>
                        </span>
                        <span className="mt-1 block text-xs text-[var(--text-secondary)] truncate">
                          {sourceLabel(doc.source)} - {doc.visibility} - used by {doc.used_by_count || 0}
                        </span>
                      </button>
                    ))}
                  </div>
                ))}
              </section>
            ))}
          </div>
        </div>
      </aside>

      <main className="overflow-y-auto">
        {!detail && !detailLoading ? (
          <div className="h-full flex items-center justify-center text-[var(--text-secondary)]">Select a document</div>
        ) : null}
        {detailLoading ? <div className="p-8 text-[var(--text-secondary)]">Loading...</div> : null}
        {detail ? (
          <div className="p-6 space-y-5 max-w-6xl">
            {draftID && !detail.uuid ? (
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                <label className="text-sm">
                  <span className="block text-xs text-[var(--text-secondary)] mb-1">Kind</span>
                  <select className="form-input w-full" value={detail.kind} onChange={event => updateDraftIdentity('kind', event.target.value)}>
                    {kindOrder.map(kind => (
                      <option key={kind} value={kind}>{kind}</option>
                    ))}
                  </select>
                </label>
                <label className="text-sm">
                  <span className="block text-xs text-[var(--text-secondary)] mb-1">Group</span>
                  <input className="form-input w-full" value={detail.group} onChange={event => updateDraftIdentity('group', event.target.value)} />
                </label>
                <label className="text-sm">
                  <span className="block text-xs text-[var(--text-secondary)] mb-1">Name</span>
                  <input className="form-input w-full" value={detail.name} onChange={event => updateDraftIdentity('name', event.target.value)} />
                </label>
              </div>
            ) : null}
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="space-y-1 min-w-0">
                <input
                  className="text-2xl font-semibold bg-transparent border border-transparent focus:border-[var(--border-primary)] rounded-md px-1 py-0.5 w-full"
                  value={detail.title}
                  disabled={!canWriteKnowledge || Boolean(detail.managed_by_config_repo)}
                  onChange={event => setDetail(prev => (prev ? { ...prev, title: event.target.value } : prev))}
                />
                <div className="text-sm text-[var(--text-secondary)]">
                  {detail.kind} / {detail.group} / {detail.name}
                </div>
              </div>
              <div className="flex items-center gap-2">
                {!draftID || detail.uuid ? <ResourceAccessCard resourceType="knowledge_context" resourceID={detail.id} label="knowledge context" /> : null}
                {canDeleteKnowledge && !detail.managed_by_config_repo ? (
                  <button className="glass-button-ghost inline-flex items-center gap-2 px-3 py-2 text-sm" onClick={deleteDetail} disabled={saving}>
                    <Trash2 className="h-4 w-4" />
                    Delete
                  </button>
                ) : null}
                {canWriteKnowledge && !detail.managed_by_config_repo ? (
                  <button className="glass-button inline-flex items-center gap-2 px-3 py-2 text-sm" onClick={saveDetail} disabled={saving}>
                    <Save className="h-4 w-4" />
                    Save
                  </button>
                ) : null}
              </div>
            </div>

            {detailError ? <div className="rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-500">{detailError}</div> : null}

            <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 text-sm">
              <Info label="Kind" value={detail.kind} />
              <Info label="Visibility" value={detail.visibility} />
              <Info label="Source" value={sourceLabel(detail.source)} />
              <Info label="Updated" value={formatDate(detail.updated_at)} />
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-[1fr_260px] gap-5">
              <section className="space-y-3">
                <div className="flex items-center justify-between">
                  <div className="inline-flex rounded-md border border-[var(--border-primary)] overflow-hidden">
                    <button className={`px-3 py-1.5 text-sm ${mode === 'edit' ? 'bg-[var(--bg-tertiary)]' : ''}`} onClick={() => setMode('edit')}>Editor</button>
                    <button className={`px-3 py-1.5 text-sm inline-flex items-center gap-2 ${mode === 'preview' ? 'bg-[var(--bg-tertiary)]' : ''}`} onClick={() => setMode('preview')}>
                      <Eye className="h-4 w-4" />
                      Preview
                    </button>
                  </div>
                </div>
                {mode === 'edit' ? (
                  <textarea
                    className="form-input font-mono min-h-[520px] w-full resize-y leading-6"
                    value={editorValue}
                    disabled={!canWriteKnowledge || Boolean(detail.managed_by_config_repo)}
                    onChange={event => setEditorValue(event.target.value)}
                  />
                ) : (
                  <article className="min-h-[520px] rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-5 whitespace-pre-wrap leading-7">
                    {editorValue || ' '}
                  </article>
                )}
              </section>
              <aside className="space-y-4">
                <Info label="GitOps path" value={detail.config_source_path || '-'} />
                <Info label="Used by" value={detail.used_by?.length ? detail.used_by.join(', ') : '-'} />
              </aside>
            </div>
          </div>
        ) : null}
      </main>
    </div>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 min-w-0">
      <div className="text-xs text-[var(--text-secondary)]">{label}</div>
      <div className="truncate">{value}</div>
    </div>
  );
}
