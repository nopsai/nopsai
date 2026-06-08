import { afterEach, expect, test, vi } from 'vitest';
import { apiClient } from '../lib/api';
import {
  fetchRunSidebarDetail,
  fetchRunSidebarGroups,
  fetchRunSidebarRecentRuns,
  fetchRunSidebarRepositoryRuns,
} from './runSidebarApi';

afterEach(() => {
  vi.restoreAllMocks();
});

test('returns empty sidebar data when requests fail or responses are invalid', async () => {
  const fetchMock = vi.spyOn(apiClient, 'fetch');
  fetchMock.mockRejectedValueOnce(new Error('offline'));
  fetchMock.mockResolvedValueOnce(new Response('unavailable', { status: 503 }));
  fetchMock.mockResolvedValueOnce(new Response('{invalid-json', {
    status: 200,
    headers: { 'content-type': 'application/json' },
  }));
  fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ unexpected: true }), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  }));

  await expect(fetchRunSidebarGroups()).resolves.toEqual([]);
  await expect(fetchRunSidebarRecentRuns(0, 200)).resolves.toEqual([]);
  await expect(fetchRunSidebarRepositoryRuns(7)).resolves.toBeNull();
  await expect(fetchRunSidebarGroups()).resolves.toEqual([]);
});

test('uses encoded run routes and returns successful sidebar payloads', async () => {
  const fetchMock = vi.spyOn(apiClient, 'fetch');
  fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
    run_info: { run_id: 'run/with space', pipeline_name: 'Deploy', status: 'running' },
  }), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  }));
  fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ main: [] }), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  }));

  await expect(fetchRunSidebarDetail('run/with space')).resolves.toEqual({
    run_info: expect.objectContaining({ pipeline_name: 'Deploy' }),
  });
  await expect(fetchRunSidebarRepositoryRuns(42)).resolves.toEqual({ main: [] });
  expect(fetchMock).toHaveBeenNthCalledWith(1, '/v1/runs/run%2Fwith%20space', { cache: 'no-store' });
  expect(fetchMock).toHaveBeenNthCalledWith(2, '/v1/runs?groupId=42', { cache: 'no-store' });
});
