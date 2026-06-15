import { asRecord, normalizeStringArray, normalizeStringMap, readOptionalString, readString } from '../data.js';

export type MCPToolRecord = {
  server_name: string;
  name: string;
  description?: string;
  input_schema?: string;
  schema_hash?: string;
  last_seen_at?: string;
};

export type MCPServerRecord = {
  name: string;
  display_name: string;
  enabled: boolean;
  provider: string;
  transport: string;
  url: string;
  auth_type: string;
  credential_ref: string;
  headers: Record<string, string>;
  timeout: string;
  allowed_scopes: string[];
  last_test_status?: string;
  last_test_message?: string;
  last_tested_at?: string;
  last_discovered_at?: string;
  discovered_server_name?: string;
  discovered_version?: string;
  discovered_protocol?: string;
  tools: MCPToolRecord[];
};

export type MCPProfileServerRef = {
  server: string;
  tools: string[];
};

export type MCPProfileRecord = {
  name: string;
  description: string;
  enabled: boolean;
  servers: MCPProfileServerRef[];
  allowed_scopes: string[];
};

export type MCPServerFormState = {
  name: string;
  display_name: string;
  enabled: boolean;
  provider: string;
  transport: string;
  url: string;
  auth_type: string;
  credential_ref: string;
  headers_json: string;
  timeout: string;
  allowed_scopes: string;
};

export type MCPProfileFormState = {
  name: string;
  description: string;
  enabled: boolean;
  selected_tools: Record<string, string[]>;
  tool_text: Record<string, string>;
  allowed_scopes: string;
};

export type MCPPanelMode = 'server-create' | 'server-edit' | 'profile-create' | 'profile-edit';

export type MCPServerPayload = {
  name: string;
  display_name: string;
  enabled: boolean;
  provider: string;
  transport: string;
  url: string;
  auth_type: string;
  credential_ref: string;
  headers: Record<string, string>;
  timeout: string;
  allowed_scopes: string[];
};

export type MCPProfilePayload = {
  name: string;
  description: string;
  enabled: boolean;
  servers: MCPProfileServerRef[];
  allowed_scopes: string[];
};

export const emptyMCPServerForm: MCPServerFormState = {
  name: '',
  display_name: '',
  enabled: true,
  provider: '',
  transport: 'streamable_http',
  url: '',
  auth_type: 'none',
  credential_ref: '',
  headers_json: '',
  timeout: '30s',
  allowed_scopes: '',
};

export const emptyMCPProfileForm: MCPProfileFormState = {
  name: '',
  description: '',
  enabled: true,
  selected_tools: {},
  tool_text: {},
  allowed_scopes: '',
};

export function normalizeMCPServersPayload(value: unknown): MCPServerRecord[] {
  const record = asRecord(value);
  const serversRaw = record && Array.isArray(record.servers) ? record.servers : [];
  const servers = serversRaw
    .map(item => {
      const server = asRecord(item);
      if (!server) return null;
      const name = readString(server.name).trim();
      if (!name) return null;
      return {
        name,
        display_name: readString(server.display_name).trim(),
        enabled: typeof server.enabled === 'boolean' ? server.enabled : true,
        provider: readString(server.provider).trim(),
        transport: readString(server.transport).trim() || 'streamable_http',
        url: readString(server.url).trim(),
        auth_type: readString(server.auth_type).trim() || 'none',
        credential_ref: readString(server.credential_ref).trim(),
        headers: normalizeStringMap(server.headers),
        timeout: readString(server.timeout).trim() || '30s',
        allowed_scopes: normalizeStringArray(server.allowed_scopes),
        last_test_status: readOptionalString(server.last_test_status),
        last_test_message: readOptionalString(server.last_test_message),
        last_tested_at: readOptionalString(server.last_tested_at),
        last_discovered_at: readOptionalString(server.last_discovered_at),
        discovered_server_name: readOptionalString(server.discovered_server_name),
        discovered_version: readOptionalString(server.discovered_version),
        discovered_protocol: readOptionalString(server.discovered_protocol),
        tools: Array.isArray(server.tools) ? server.tools.map(normalizeMCPTool).filter((tool): tool is MCPToolRecord => Boolean(tool)) : [],
      } satisfies MCPServerRecord;
    })
    .filter(Boolean) as MCPServerRecord[];
  return servers.sort((a, b) => a.name.localeCompare(b.name));
}

export function normalizeMCPTool(value: unknown): MCPToolRecord | null {
  const record = asRecord(value);
  if (!record) return null;
  const name = readString(record.name).trim();
  if (!name) return null;
  return {
    server_name: readString(record.server_name).trim(),
    name,
    description: readOptionalString(record.description),
    input_schema: readOptionalString(record.input_schema),
    schema_hash: readOptionalString(record.schema_hash),
    last_seen_at: readOptionalString(record.last_seen_at),
  };
}

export function normalizeMCPProfilesPayload(value: unknown): MCPProfileRecord[] {
  const record = asRecord(value);
  const profilesRaw = record && Array.isArray(record.profiles) ? record.profiles : [];
  const profiles = profilesRaw
    .map(item => {
      const profile = asRecord(item);
      if (!profile) return null;
      const name = readString(profile.name).trim();
      if (!name) return null;
      const refsRaw = Array.isArray(profile.servers) ? profile.servers : [];
      const refs = refsRaw
        .map(refItem => {
          const ref = asRecord(refItem);
          if (!ref) return null;
          const server = readString(ref.server).trim();
          if (!server) return null;
          return { server, tools: normalizeStringArray(ref.tools) } satisfies MCPProfileServerRef;
        })
        .filter(Boolean) as MCPProfileServerRef[];
      return {
        name,
        description: readString(profile.description).trim(),
        enabled: typeof profile.enabled === 'boolean' ? profile.enabled : true,
        servers: refs,
        allowed_scopes: normalizeStringArray(profile.allowed_scopes),
      } satisfies MCPProfileRecord;
    })
    .filter(Boolean) as MCPProfileRecord[];
  return profiles.sort((a, b) => a.name.localeCompare(b.name));
}

export function mcpServerFormFromRecord(server: MCPServerRecord): MCPServerFormState {
  return {
    name: server.name,
    display_name: server.display_name || '',
    enabled: server.enabled,
    provider: server.provider || '',
    transport: server.transport || 'streamable_http',
    url: server.url || '',
    auth_type: server.auth_type || 'none',
    credential_ref: server.credential_ref || '',
    headers_json: formatHeadersJSON(server.headers),
    timeout: server.timeout || '30s',
    allowed_scopes: server.allowed_scopes.join(', '),
  };
}

export function mcpProfileFormFromRecord(profile: MCPProfileRecord): MCPProfileFormState {
  const selectedTools: Record<string, string[]> = {};
  const toolText: Record<string, string> = {};
  profile.servers.forEach(ref => {
    selectedTools[ref.server] = [...ref.tools];
    toolText[ref.server] = ref.tools.join('\n');
  });
  return {
    name: profile.name,
    description: profile.description || '',
    enabled: profile.enabled,
    selected_tools: selectedTools,
    tool_text: toolText,
    allowed_scopes: profile.allowed_scopes.join(', '),
  };
}

export function mcpServerPayloadFromForm(form: MCPServerFormState): MCPServerPayload | null {
  const headers = parseHeadersJSON(form.headers_json);
  if (headers == null) return null;
  return {
    name: form.name.trim(),
    display_name: form.display_name.trim(),
    enabled: form.enabled,
    provider: form.provider.trim(),
    transport: form.transport.trim(),
    url: form.url.trim(),
    auth_type: form.auth_type.trim(),
    credential_ref: form.credential_ref.trim(),
    headers,
    timeout: form.timeout.trim() || '30s',
    allowed_scopes: splitCSV(form.allowed_scopes),
  };
}

export function mcpProfilePayloadFromForm(form: MCPProfileFormState): MCPProfilePayload {
  const servers = Object.entries(form.selected_tools)
    .filter(([, tools]) => tools.length > 0)
    .map(([server, tools]) => ({ server, tools }));

  return {
    name: form.name.trim(),
    description: form.description.trim(),
    enabled: form.enabled,
    servers,
    allowed_scopes: splitCSV(form.allowed_scopes),
  };
}

export function toggleProfileToolSelection(form: MCPProfileFormState, serverName: string, toolName: string): MCPProfileFormState {
  const current = new Set(form.selected_tools[serverName] || []);
  if (current.has(toolName)) current.delete(toolName);
  else current.add(toolName);

  const selectedTools = { ...form.selected_tools, [serverName]: Array.from(current).sort((a, b) => a.localeCompare(b)) };
  if (selectedTools[serverName].length === 0) delete selectedTools[serverName];

  const toolText = { ...form.tool_text, [serverName]: (selectedTools[serverName] || []).join('\n') };
  if (!selectedTools[serverName]) delete toolText[serverName];

  return { ...form, selected_tools: selectedTools, tool_text: toolText };
}

export function setProfileServerToolText(form: MCPProfileFormState, serverName: string, value: string): MCPProfileFormState {
  const tools = splitToolNames(value);
  const selectedTools = { ...form.selected_tools };
  if (tools.length > 0) selectedTools[serverName] = tools;
  else delete selectedTools[serverName];

  return { ...form, selected_tools: selectedTools, tool_text: { ...form.tool_text, [serverName]: value } };
}

export function normalizeMCPProfileTestMessage(value: unknown): string {
  const record = asRecord(value);
  const warnings = normalizeStringArray(record?.warnings);
  return warnings.length ? warnings.join('; ') : readString(record?.message) || 'ok';
}

export function splitCSV(value: string): string[] {
  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
}

export function splitToolNames(value: string): string[] {
  const seen = new Set<string>();
  return value
    .split(/[\n,]/)
    .map(item => item.trim())
    .filter(item => {
      if (!item || seen.has(item)) return false;
      seen.add(item);
      return true;
    })
    .sort((a, b) => a.localeCompare(b));
}

export function formatHeadersJSON(headers: Record<string, string>): string {
  if (Object.keys(headers || {}).length === 0) return '';
  return JSON.stringify(headers, null, 2);
}

export function parseHeadersJSON(value: string): Record<string, string> | null {
  const trimmed = value.trim();
  if (!trimmed) return {};
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    return null;
  }
  const record = asRecord(parsed);
  if (!record || Array.isArray(parsed)) return null;
  const headers: Record<string, string> = {};
  for (const [key, headerValue] of Object.entries(record)) {
    const headerName = key.trim();
    if (!headerName) continue;
    if (typeof headerValue !== 'string') return null;
    headers[headerName] = headerValue.trim();
  }
  return headers;
}
