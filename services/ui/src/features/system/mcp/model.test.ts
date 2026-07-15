import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  countMCPProfileTools,
  formatMCPScopes,
  mcpProfileFormFromRecord,
  mcpProfilePayloadFromForm,
  mcpServerFormFromRecord,
  mcpServerPayloadFromForm,
  normalizeMCPProfileTestMessage,
  normalizeMCPProfilesPayload,
  normalizeMCPServersPayload,
  parseHeadersJSON,
  setProfileServerToolText,
  splitToolNames,
  toggleProfileToolSelection,
  type MCPProfileFormState,
  type MCPProfileRecord,
  type MCPServerRecord,
} from './model.js';

test('normalizes MCP servers with defaults, headers, tools, and stable ordering', () => {
  const servers = normalizeMCPServersPayload({
    servers: [
      {
        name: 'z-github',
        display_name: 'GitHub',
        provider: 'github',
        headers: { 'X-MCP-Readonly': true },
        tools: [{ name: 'issues_list', description: 'List issues' }, { name: '' }],
      },
      { name: 'alpha', enabled: false },
      { name: ' ' },
    ],
  });

  assert.deepEqual(
    servers.map(server => server.name),
    ['alpha', 'z-github']
  );
  assert.equal(servers[0]?.enabled, false);
  assert.equal(servers[0]?.transport, 'streamable_http');
  assert.equal(servers[0]?.timeout, '30s');
  assert.deepEqual(servers[1]?.headers, { 'X-MCP-Readonly': 'true' });
  assert.equal(servers[1]?.tools[0]?.name, 'issues_list');
});

test('builds MCP server form state and payloads', () => {
  const server: MCPServerRecord = {
    name: 'github',
    display_name: 'GitHub MCP',
    enabled: true,
    provider: 'github',
    transport: 'streamable_http',
    url: 'https://example.test/mcp',
    auth_type: 'bearer_token',
    credential_ref: 'credential://system/mcp/github',
    headers: { 'X-MCP-Toolsets': 'repos,issues' },
    timeout: '45s',
    allowed_scopes: ['dev', 'prod'],
    tools: [],
  };

  const form = mcpServerFormFromRecord(server);
  assert.equal(form.headers_json, '{\n  "X-MCP-Toolsets": "repos,issues"\n}');
  assert.equal(form.allowed_scopes, 'dev, prod');

  assert.deepEqual(
    mcpServerPayloadFromForm({
      ...form,
      name: ' github ',
      display_name: ' GitHub MCP ',
      allowed_scopes: ' dev, prod, ',
      headers_json: '{" X-MCP-Readonly ":" true "}',
    }),
    {
      name: 'github',
      display_name: 'GitHub MCP',
      enabled: true,
      provider: 'github',
      transport: 'streamable_http',
      url: 'https://example.test/mcp',
      auth_type: 'bearer_token',
      credential_ref: 'credential://system/mcp/github',
      headers: { 'X-MCP-Readonly': 'true' },
      timeout: '45s',
      allowed_scopes: ['dev', 'prod'],
    }
  );

  assert.equal(mcpServerPayloadFromForm({ ...form, headers_json: '[]' }), null);
  assert.equal(parseHeadersJSON('{"X-Test": 123}'), null);
});

test('normalizes MCP profiles and profile tool form state', () => {
  const profiles = normalizeMCPProfilesPayload({
    profiles: [
      {
        name: 'review',
        description: 'PR review',
        servers: [{ server: 'github', tools: ['issues_list', 'repos_get'] }, { server: '' }],
        allowed_scopes: ['dev'],
      },
    ],
  });
  assert.equal(profiles[0]?.enabled, true);
  assert.deepEqual(profiles[0]?.servers[0], { server: 'github', tools: ['issues_list', 'repos_get'] });

  const form = mcpProfileFormFromRecord(profiles[0] as MCPProfileRecord);
  assert.equal(form.tool_text.github, 'issues_list\nrepos_get');
  assert.deepEqual(mcpProfilePayloadFromForm(form), {
    name: 'review',
    description: 'PR review',
    enabled: true,
    servers: [{ server: 'github', tools: ['issues_list', 'repos_get'] }],
    allowed_scopes: ['dev'],
  });
});

test('updates MCP profile tool selections consistently', () => {
  const form: MCPProfileFormState = {
    name: 'review',
    description: '',
    enabled: true,
    selected_tools: { github: ['repos_get'] },
    tool_text: { github: 'repos_get' },
    allowed_scopes: '',
  };

  const toggled = toggleProfileToolSelection(form, 'github', 'issues_list');
  assert.deepEqual(toggled.selected_tools.github, ['issues_list', 'repos_get']);
  assert.equal(toggled.tool_text.github, 'issues_list\nrepos_get');

  const removed = toggleProfileToolSelection(toggleProfileToolSelection(toggled, 'github', 'issues_list'), 'github', 'repos_get');
  assert.deepEqual(removed.selected_tools, {});
  assert.deepEqual(removed.tool_text, {});

  assert.deepEqual(splitToolNames('repos_get\nissues_list,repos_get'), ['issues_list', 'repos_get']);
  assert.deepEqual(setProfileServerToolText(form, 'github', 'issues_list\nrepos_get').selected_tools.github, ['issues_list', 'repos_get']);
});

test('normalizes MCP profile test responses', () => {
  assert.equal(normalizeMCPProfileTestMessage({ warnings: ['limited scope', 'missing tool'] }), 'limited scope; missing tool');
  assert.equal(normalizeMCPProfileTestMessage({ message: 'ok: 2 tools' }), 'ok: 2 tools');
  assert.equal(normalizeMCPProfileTestMessage(null), 'ok');
});

test('formats MCP profile tool counts and scope labels', () => {
  const profile: MCPProfileRecord = {
    name: 'review',
    description: '',
    enabled: true,
    servers: [
      { server: 'github', tools: ['issues_list', 'repos_get'] },
      { server: 'slack', tools: ['search'] },
    ],
    allowed_scopes: [],
  };

  assert.equal(countMCPProfileTools(profile), 3);
  assert.equal(formatMCPScopes([]), 'All scopes');
  assert.equal(formatMCPScopes(['dev', 'prod']), 'dev, prod');
});
