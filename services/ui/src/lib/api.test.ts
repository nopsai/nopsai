import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  ApiClient,
  buildOIDCStartUrl,
  clearSession,
  consumeNextSSOLoginPrompt,
  getStoredSession,
  logoutCurrentSession,
  persistSession,
  requirePromptForNextSSOLogin,
} from './api.js';

class MemoryStorage implements Storage {
  private store = new Map<string, string>();

  get length() {
    return this.store.size;
  }

  clear() {
    this.store.clear();
  }

  getItem(key: string) {
    return this.store.get(key) ?? null;
  }

  key(index: number) {
    return Array.from(this.store.keys())[index] ?? null;
  }

  removeItem(key: string) {
    this.store.delete(key);
  }

  setItem(key: string, value: string) {
    this.store.set(key, value);
  }
}

function installMemoryStorage() {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: new MemoryStorage(),
  });
}

test.beforeEach(() => {
  installMemoryStorage();
  clearSession();
});

test('adds bearer auth and refreshes once on unauthorized API responses', async () => {
  persistSession({ accessToken: 'old-token', refreshToken: 'refresh-token', sub: 'operator' });
  const calls: Array<{ url: string; authorization: string | null }> = [];
  const client = new ApiClient(async (input, init) => {
    const url = String(input);
    calls.push({
      url,
      authorization: new Headers(init?.headers).get('Authorization'),
    });

    if (url.endsWith('/v1/auth/refresh')) {
      return new Response(JSON.stringify({ access_token: 'new-token', refresh_token: 'refresh-token' }), {
        headers: { 'Content-Type': 'application/json' },
        status: 200,
      });
    }

    if (calls.filter(call => call.url.endsWith('/v1/runs')).length === 1) {
      return new Response('', { status: 401 });
    }

    return new Response(JSON.stringify({ ok: true }), {
      headers: { 'Content-Type': 'application/json' },
      status: 200,
    });
  });

  const response = await client.fetch('/v1/runs');

  assert.equal(response.status, 200);
  assert.equal(calls[0].authorization, 'Bearer old-token');
  assert.equal(calls[1].authorization, null);
  assert.equal(calls[2].authorization, 'Bearer new-token');
  assert.equal(getStoredSession().accessToken, 'new-token');
});

test('supports unauthenticated requests without attaching stored credentials', async () => {
  persistSession({ accessToken: 'token' });
  const seenAuthHeaders: Array<string | null> = [];
  const client = new ApiClient(async (_input, init) => {
    seenAuthHeaders.push(new Headers(init?.headers).get('Authorization'));
    return new Response('{}', { status: 200 });
  });

  await client.fetch('/v1/auth/login', { auth: false });

  assert.deepEqual(seenAuthHeaders, [null]);
});

test('marks the next SSO login to prompt only once', () => {
  requirePromptForNextSSOLogin();

  assert.equal(consumeNextSSOLoginPrompt(), true);
  assert.equal(consumeNextSSOLoginPrompt(), false);
  assert.match(buildOIDCStartUrl('nopsai', '/pipelineruns/main', { prompt: 'login' }), /prompt=login/);
});

test('logout revokes the refresh token, clears local state, and returns provider logout URL', async () => {
  persistSession({ accessToken: 'access-token', refreshToken: 'refresh-token', sub: 'operator' });
  const originalFetch = globalThis.fetch;
  const bodies: string[] = [];
  globalThis.fetch = (async (_input, init) => {
    bodies.push(String(init?.body || ''));
    return new Response(JSON.stringify({ logout_url: 'http://keycloak/logout' }), {
      headers: { 'Content-Type': 'application/json' },
      status: 200,
    });
  }) as typeof fetch;

  try {
    const result = await logoutCurrentSession();

    assert.deepEqual(result, { logoutURL: 'http://keycloak/logout' });
    assert.deepEqual(JSON.parse(bodies[0]), { refresh_token: 'refresh-token' });
    assert.equal(getStoredSession().accessToken, undefined);
    assert.equal(consumeNextSSOLoginPrompt(), true);
  } finally {
    globalThis.fetch = originalFetch;
  }
});
