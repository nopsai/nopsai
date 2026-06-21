import { apiClient } from '../../../lib/api.js';
import type { SystemLogSSEEvent, SystemLogSourcesResponse } from './types.js';

type ParsedSSEBlock = { event: string; id: string; data: string };

export function parseSSEBlocks(input: string): { blocks: ParsedSSEBlock[]; rest: string } {
  const normalized = input.replace(/\r\n/g, '\n');
  const parts = normalized.split('\n\n');
  const rest = parts.pop() || '';
  const blocks = parts.flatMap(block => {
    let event = 'message';
    let id = '';
    const data: string[] = [];
    for (const line of block.split('\n')) {
      if (!line || line.startsWith(':')) continue;
      const separator = line.indexOf(':');
      const field = separator >= 0 ? line.slice(0, separator) : line;
      const value = separator >= 0 ? line.slice(separator + 1).replace(/^ /, '') : '';
      if (field === 'event') event = value;
      if (field === 'id') id = value;
      if (field === 'data') data.push(value);
    }
    return data.length ? [{ event, id, data: data.join('\n') }] : [];
  });
  return { blocks, rest };
}

export async function fetchSystemLogSources(): Promise<SystemLogSourcesResponse> {
  return apiClient.json<SystemLogSourcesResponse>('/v1/system/logs/sources', { cache: 'no-store' });
}

export async function streamSystemLogs({
  sourceID,
  cursor,
  tail,
  signal,
  onEvent,
}: {
  sourceID: string;
  cursor?: string;
  tail: number;
  signal: AbortSignal;
  onEvent: (event: SystemLogSSEEvent) => void;
}): Promise<void> {
  const query = new URLSearchParams({ tail: String(tail) });
  if (cursor) query.set('cursor', cursor);
  const response = await apiClient.fetch(
    `/v1/system/logs/sources/${encodeURIComponent(sourceID)}/stream?${query.toString()}`,
    { cache: 'no-store', headers: { Accept: 'text/event-stream' }, signal }
  );
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message.trim() || `System log stream failed (${response.status})`);
  }
  if (!response.body) throw new Error('System log stream is unavailable in this browser');

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    const parsed = parseSSEBlocks(buffer);
    buffer = parsed.rest;
    for (const block of parsed.blocks) {
      if (block.event !== 'status' && block.event !== 'reset' && block.event !== 'log') continue;
      const data = JSON.parse(block.data) as SystemLogSSEEvent['data'];
      if (block.event === 'log') {
        onEvent({ event: 'log', id: block.id, data: data as Extract<SystemLogSSEEvent, { event: 'log' }>['data'] });
      } else if (block.event === 'reset') {
        onEvent({ event: 'reset', data: data as Extract<SystemLogSSEEvent, { event: 'reset' }>['data'] });
      } else {
        onEvent({ event: 'status', data: data as Extract<SystemLogSSEEvent, { event: 'status' }>['data'] });
      }
    }
    if (done) return;
  }
}
