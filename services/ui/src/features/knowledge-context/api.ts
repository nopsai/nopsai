import { apiClient } from '../../lib/api';
import {
  encodeKnowledgeID,
  isExternalKnowledgeDocument,
  knowledgeConnectionIdentifier,
  type KnowledgeExternalPagePreview,
  type KnowledgeExternalPageSearchResult,
  type KnowledgeConnectionDraft,
  type KnowledgeConnectionListItem,
  type KnowledgeContextDetail,
  type KnowledgeContextListItem,
} from './model';

async function readError(response: Response, fallback: string) {
  const text = await response.text();
  return text.trim() || fallback;
}

export async function fetchKnowledgeContexts(): Promise<KnowledgeContextListItem[]> {
  const response = await apiClient.fetch('/v1/knowledge-contexts', { cache: 'no-store' });
  if (!response.ok) throw new Error(await readError(response, `Unable to load knowledge contexts (${response.status})`));
  const payload = await response.json();
  return Array.isArray(payload) ? payload : [];
}

export async function fetchKnowledgeContext(id: string): Promise<KnowledgeContextDetail> {
  const response = await apiClient.fetch(`/v1/knowledge-contexts/${encodeKnowledgeID(id)}`, { cache: 'no-store' });
  if (!response.ok) throw new Error(await readError(response, `Unable to load document (${response.status})`));
  return (await response.json()) as KnowledgeContextDetail;
}

export async function saveKnowledgeContext(detail: KnowledgeContextDetail, content: string): Promise<KnowledgeContextDetail> {
  const isExternal = isExternalKnowledgeDocument(detail);
  const response = await apiClient.fetch(`/v1/knowledge-contexts/${encodeKnowledgeID(detail.id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      kind: detail.kind,
      team: detail.team,
      name: detail.name,
      description: detail.description || '',
      content,
      content_source: isExternal ? 'external' : 'inline',
      connection_id: isExternal ? detail.connection_ref || detail.connection_id || '' : '',
      external_page_id: isExternal ? detail.external_page_id || '' : '',
      external_page_url: isExternal ? detail.external_page_url || '' : '',
      external_page_title: isExternal ? detail.external_page_title || '' : '',
      sync_mode: isExternal ? detail.sync_mode || 'manual' : '',
      failure_mode: isExternal ? detail.failure_mode || 'fail' : '',
    }),
  });
  if (!response.ok) throw new Error(await readError(response, `Unable to save document (${response.status})`));
  return (await response.json()) as KnowledgeContextDetail;
}

export async function syncKnowledgeContext(id: string): Promise<KnowledgeContextDetail> {
  const response = await apiClient.fetch(`/v1/knowledge-contexts/${encodeKnowledgeID(id)}/sync`, { method: 'POST' });
  if (!response.ok) throw new Error(await readError(response, `Unable to sync document (${response.status})`));
  return (await response.json()) as KnowledgeContextDetail;
}

export async function deleteKnowledgeContext(id: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/knowledge-contexts/${encodeKnowledgeID(id)}`, { method: 'DELETE' });
  if (!response.ok) throw new Error(await readError(response, `Unable to delete document (${response.status})`));
}

export async function fetchKnowledgeConnections(team?: string): Promise<KnowledgeConnectionListItem[]> {
  const query = team ? `?team=${encodeURIComponent(team)}` : '';
  const response = await apiClient.fetch(`/v1/knowledge-context-connections${query}`, { cache: 'no-store' });
  if (!response.ok) throw new Error(await readError(response, `Unable to load knowledge connections (${response.status})`));
  const payload = await response.json();
  return Array.isArray(payload) ? payload : [];
}

export async function createKnowledgeConnection(draft: KnowledgeConnectionDraft): Promise<KnowledgeConnectionListItem> {
  const response = await apiClient.fetch('/v1/knowledge-context-connections', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(draft),
  });
  if (!response.ok) throw new Error(await readError(response, `Unable to create knowledge connection (${response.status})`));
  return (await response.json()) as KnowledgeConnectionListItem;
}

export async function updateKnowledgeConnection(
  connection: KnowledgeConnectionListItem,
  draft: Partial<KnowledgeConnectionDraft>
): Promise<KnowledgeConnectionListItem> {
  const response = await apiClient.fetch(`/v1/knowledge-context-connections/${encodeKnowledgeID(knowledgeConnectionIdentifier(connection))}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      team: connection.team,
      name: connection.name,
      display_name: draft.display_name ?? connection.display_name,
      provider: draft.provider ?? connection.provider,
      base_url: draft.base_url ?? connection.base_url ?? '',
      credential_ref: draft.credential_ref ?? '',
      scopes: connection.scopes ?? {},
      config: connection.config ?? {},
      disabled: draft.disabled ?? connection.disabled ?? false,
    }),
  });
  if (!response.ok) throw new Error(await readError(response, `Unable to update knowledge connection (${response.status})`));
  return (await response.json()) as KnowledgeConnectionListItem;
}

export async function testKnowledgeConnection(connection: KnowledgeConnectionListItem): Promise<{ status: string; ok: boolean; message: string }> {
  const identifier = connection.uuid || connection.id;
  const response = await apiClient.fetch(`/v1/knowledge-context-connections/${encodeKnowledgeID(identifier)}/test`, { method: 'POST' });
  if (!response.ok) throw new Error(await readError(response, `Unable to test knowledge connection (${response.status})`));
  return (await response.json()) as { status: string; ok: boolean; message: string };
}

export async function searchKnowledgeConnectionPages(
  connection: KnowledgeConnectionListItem,
  query: string,
  cursor = ''
): Promise<KnowledgeExternalPageSearchResult> {
  const identifier = connection.uuid || connection.id;
  const params = new URLSearchParams();
  if (query.trim()) params.set('query', query.trim());
  if (cursor.trim()) params.set('cursor', cursor.trim());
  const suffix = params.toString() ? `?${params.toString()}` : '';
  const response = await apiClient.fetch(`/v1/knowledge-context-connections/${encodeKnowledgeID(identifier)}/pages/search${suffix}`, { cache: 'no-store' });
  if (!response.ok) throw new Error(await readError(response, `Unable to search provider pages (${response.status})`));
  const payload = await response.json();
  return {
    pages: Array.isArray(payload?.pages) ? payload.pages : [],
    next_cursor: typeof payload?.next_cursor === 'string' ? payload.next_cursor : '',
  };
}

export async function resolveKnowledgeConnectionPage(
  connection: KnowledgeConnectionListItem,
  input: { page_id?: string; page_url?: string }
): Promise<KnowledgeExternalPagePreview> {
  const identifier = connection.uuid || connection.id;
  const response = await apiClient.fetch(`/v1/knowledge-context-connections/${encodeKnowledgeID(identifier)}/resolve-page`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
  if (!response.ok) throw new Error(await readError(response, `Unable to load provider page (${response.status})`));
  return (await response.json()) as KnowledgeExternalPagePreview;
}

export async function deleteKnowledgeConnection(connection: KnowledgeConnectionListItem, confirm = false): Promise<void> {
  const identifier = connection.uuid || connection.id;
  const suffix = confirm ? '?confirm=true' : '';
  const response = await apiClient.fetch(`/v1/knowledge-context-connections/${encodeKnowledgeID(identifier)}${suffix}`, { method: 'DELETE' });
  if (!response.ok) throw new Error(await readError(response, `Unable to delete knowledge connection (${response.status})`));
}
