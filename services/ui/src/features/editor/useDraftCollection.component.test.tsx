import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { useDraftCollection } from './useDraftCollection';

type TestDraft = { id: string; yaml: string };

const changedEvent = 'test:drafts:changed';
const storageKey = (scope: string) => `drafts:${scope}`;

function load(scope: string): TestDraft[] {
  return JSON.parse(localStorage.getItem(storageKey(scope)) || '[]') as TestDraft[];
}

function save(scope: string, drafts: TestDraft[]) {
  localStorage.setItem(storageKey(scope), JSON.stringify(drafts));
  window.dispatchEvent(new Event(changedEvent));
  return drafts;
}

const upsert = vi.fn((draft: TestDraft, scope: string) => {
  const drafts = load(scope).filter(item => item.id !== draft.id);
  return save(scope, [...drafts, draft]);
});

const remove = vi.fn((id: string, scope: string) =>
  save(scope, load(scope).filter(item => item.id !== id))
);

beforeEach(() => {
  localStorage.clear();
  upsert.mockClear();
  remove.mockClear();
});

afterEach(() => {
  vi.useRealTimers();
});

test('subscribes to draft changes and exposes typed mutations', () => {
  save('team', [{ id: 'release', yaml: 'name: release' }]);
  const { result } = renderHook(() =>
    useDraftCollection({
      enabled: true,
      scope: 'team',
      changedEvent,
      getStorageKey: storageKey,
      load,
      upsert,
      remove,
    })
  );

  expect(result.current.draftsByID.get('release')?.yaml).toBe('name: release');

  act(() => {
    result.current.upsertDraft({ id: 'build', yaml: 'name: build' });
  });
  expect(result.current.drafts.map(draft => draft.id)).toEqual(['release', 'build']);

  act(() => {
    result.current.removeDraft('release');
  });
  expect(result.current.drafts.map(draft => draft.id)).toEqual(['build']);
});

test('disables persistence without access or scope', () => {
  const { result } = renderHook(() =>
    useDraftCollection({
      enabled: false,
      scope: 'team',
      changedEvent,
      getStorageKey: storageKey,
      load,
      upsert,
      remove,
    })
  );

  act(() => {
    result.current.upsertDraft({ id: 'release', yaml: 'name: release' });
    result.current.removeDraft('release');
  });
  expect(result.current.drafts).toEqual([]);
  expect(upsert).not.toHaveBeenCalled();
  expect(remove).not.toHaveBeenCalled();
});

test('debounces autosave for the active draft', async () => {
  vi.useFakeTimers();
  renderHook(() =>
    useDraftCollection({
      enabled: true,
      scope: 'team',
      changedEvent,
      getStorageKey: storageKey,
      load,
      upsert,
      remove,
      autosave: {
        active: true,
        id: 'release',
        yaml: 'name: release',
        delay: 500,
      },
    })
  );

  await act(async () => {
    await vi.advanceTimersByTimeAsync(499);
  });
  expect(upsert).not.toHaveBeenCalled();

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(upsert).toHaveBeenCalledWith(
    { id: 'release', yaml: 'name: release' },
    'team'
  );
});
