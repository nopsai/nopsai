import { apiClient } from '../../lib/api';
import { encodeKnowledgeID, type KnowledgeContextDetail, type KnowledgeContextListItem } from './model';

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
  const response = await apiClient.fetch(`/v1/knowledge-contexts/${encodeKnowledgeID(detail.id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      kind: detail.kind,
      group: detail.group,
      name: detail.name,
      description: detail.description || '',
      content,
    }),
  });
  if (!response.ok) throw new Error(await readError(response, `Unable to save document (${response.status})`));
  return (await response.json()) as KnowledgeContextDetail;
}

export async function deleteKnowledgeContext(id: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/knowledge-contexts/${encodeKnowledgeID(id)}`, { method: 'DELETE' });
  if (!response.ok) throw new Error(await readError(response, `Unable to delete document (${response.status})`));
}
