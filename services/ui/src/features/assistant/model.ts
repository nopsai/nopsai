import { asRecord, normalizeStringArray, readString } from '../system/data.js';

export type AssistantToolActivity = {
  name: string;
  input: Record<string, unknown>;
  output: Record<string, unknown>;
  status: string;
  resource_uris: string[];
};

export type AssistantMessage = {
  id: string;
  conversation_id: string;
  role: 'user' | 'assistant' | 'system' | string;
  content: string;
  tool_calls: AssistantToolActivity[];
  usage: AssistantMessageUsage;
  created_at: string;
};

export type AssistantMessageUsage = {
  content_tokens: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  estimated: boolean;
  duration_ms: number;
  llm_calls: number;
};

export type AssistantConversationUsage = {
  message_count: number;
  content_tokens: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  estimated_token_messages: number;
  duration_ms: number;
  llm_calls: number;
};

export type AssistantMemory = {
  summary: string;
  entities: Record<string, unknown>;
  open_tasks: string[];
  previous_proposed_fixes: string[];
  selected_run: string;
  selected_pipeline: string;
  selected_scope: string;
  selected_docs_version: string;
};

export type AssistantConversation = {
  id: string;
  user_id: string;
  title: string;
  selected_llm_profile: string;
  docs_version: string;
  scope: string;
  memory: AssistantMemory;
  messages: AssistantMessage[];
  usage: AssistantConversationUsage;
  created_at: string;
  updated_at: string;
};

export type AssistantConversationsPayload = {
  conversations: AssistantConversation[];
};

export type AssistantMessagePayload = {
  conversation: AssistantConversation;
  user_message: AssistantMessage;
  reply: AssistantMessage;
};

export type AssistantLLMProfile = {
  name: string;
  provider: string;
  model: string;
  status: string;
  validation?: string;
  allowed_in_scope: boolean;
  disabled_reason?: string;
};

export type AssistantLLMProfilesPayload = {
  default_profile: string;
  profiles: AssistantLLMProfile[];
};

export type AssistantConfig = {
  enabled: boolean;
  provider: string;
  model: string;
  default_docs_version: string;
  conversation_retention_days: number;
  max_input_logs_bytes: number;
  max_conversation_turns: number;
  docs_enabled: boolean;
  docs_version_aware: boolean;
  credential_configured: boolean;
  dedicated_profile: string;
  memory: {
    enabled: boolean;
    scope: string;
  };
  mcp: {
    enabled: boolean;
  };
  features: {
    docs: boolean;
    pipeline_debugging: boolean;
    config_generation: boolean;
    statistics_insights: boolean;
    maintenance_recommendations: boolean;
    cost_recommendations: boolean;
    action_execution: boolean;
  };
  actions: {
    require_confirmation: boolean;
  };
};

export const emptyAssistantMemory: AssistantMemory = {
  summary: '',
  entities: {},
  open_tasks: [],
  previous_proposed_fixes: [],
  selected_run: '',
  selected_pipeline: '',
  selected_scope: '',
  selected_docs_version: '',
};

export const emptyAssistantMessageUsage: AssistantMessageUsage = {
  content_tokens: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  total_tokens: 0,
  estimated: false,
  duration_ms: 0,
  llm_calls: 0,
};

export const emptyAssistantConversationUsage: AssistantConversationUsage = {
  message_count: 0,
  content_tokens: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  total_tokens: 0,
  estimated_token_messages: 0,
  duration_ms: 0,
  llm_calls: 0,
};

export function normalizeAssistantConversation(value: unknown): AssistantConversation {
  const record = asRecord(value) || {};
  const messages = Array.isArray(record.messages) ? record.messages.map(normalizeAssistantMessage) : [];
  const usage = normalizeAssistantConversationUsage(record.usage);
  return {
    id: readString(record.id),
    user_id: readString(record.user_id),
    title: readString(record.title),
    selected_llm_profile: readString(record.selected_llm_profile),
    docs_version: readString(record.docs_version) || 'auto',
    scope: readString(record.scope),
    memory: normalizeAssistantMemory(record.memory),
    messages,
    usage: usage.message_count > 0 || messages.length === 0 ? usage : assistantConversationUsageFromMessages(messages),
    created_at: readString(record.created_at),
    updated_at: readString(record.updated_at),
  };
}

export function normalizeAssistantConversationsPayload(value: unknown): AssistantConversationsPayload {
  const record = asRecord(value);
  const conversations = record && Array.isArray(record.conversations)
    ? record.conversations.map(normalizeAssistantConversation)
    : [];
  return { conversations };
}

export function normalizeAssistantMessagePayload(value: unknown): AssistantMessagePayload {
  const record = asRecord(value) || {};
  return {
    conversation: normalizeAssistantConversation(record.conversation),
    user_message: normalizeAssistantMessage(record.user_message),
    reply: normalizeAssistantMessage(record.reply),
  };
}

export function normalizeAssistantLLMProfilesPayload(value: unknown): AssistantLLMProfilesPayload {
  const record = asRecord(value);
  const profiles = record && Array.isArray(record.profiles)
    ? record.profiles.map(normalizeAssistantLLMProfile).filter(profile => profile.name)
    : [];
  profiles.sort((a, b) => a.name.localeCompare(b.name));
  return {
    default_profile: readString(record?.default_profile).trim(),
    profiles,
  };
}

export function normalizeAssistantConfig(value: unknown): AssistantConfig {
  const record = asRecord(value) || {};
  const memory = asRecord(record.memory) || {};
  const mcp = asRecord(record.mcp) || {};
  const features = asRecord(record.features) || {};
  const actions = asRecord(record.actions) || {};
  return {
    enabled: readBoolean(record.enabled, false),
    provider: readString(record.provider),
    model: readString(record.model),
    default_docs_version: readString(record.default_docs_version) || 'auto',
    conversation_retention_days: readNumber(record.conversation_retention_days, 30),
    max_input_logs_bytes: readNumber(record.max_input_logs_bytes, 120000),
    max_conversation_turns: readNumber(record.max_conversation_turns, 30),
    docs_enabled: readBoolean(record.docs_enabled, true),
    docs_version_aware: readBoolean(record.docs_version_aware, true),
    credential_configured: readBoolean(record.credential_configured, false),
    dedicated_profile: readString(record.dedicated_profile),
    memory: {
      enabled: readBoolean(memory.enabled, false),
      scope: readString(memory.scope) || 'conversation',
    },
    mcp: {
      enabled: readBoolean(mcp.enabled, false),
    },
    features: {
      docs: readBoolean(features.docs, true),
      pipeline_debugging: readBoolean(features.pipeline_debugging, true),
      config_generation: readBoolean(features.config_generation, true),
      statistics_insights: readBoolean(features.statistics_insights, true),
      maintenance_recommendations: readBoolean(features.maintenance_recommendations, true),
      cost_recommendations: readBoolean(features.cost_recommendations, true),
      action_execution: readBoolean(features.action_execution, false),
    },
    actions: {
      require_confirmation: readBoolean(actions.require_confirmation, true),
    },
  };
}

export function normalizeAssistantMessage(value: unknown): AssistantMessage {
  const record = asRecord(value) || {};
  return {
    id: readString(record.id),
    conversation_id: readString(record.conversation_id),
    role: readString(record.role),
    content: readString(record.content),
    tool_calls: Array.isArray(record.tool_calls) ? record.tool_calls.map(normalizeAssistantToolActivity) : [],
    usage: normalizeAssistantMessageUsage(record.usage),
    created_at: readString(record.created_at),
  };
}

function normalizeAssistantLLMProfile(value: unknown): AssistantLLMProfile {
  const record = asRecord(value) || {};
  return {
    name: readString(record.name).trim(),
    provider: readString(record.provider).trim(),
    model: readString(record.model).trim(),
    status: readString(record.status).trim() || 'unknown',
    validation: readString(record.validation).trim() || undefined,
    allowed_in_scope: typeof record.allowed_in_scope === 'boolean' ? record.allowed_in_scope : true,
    disabled_reason: readString(record.disabled_reason).trim() || undefined,
  };
}

export function normalizeAssistantMemory(value: unknown): AssistantMemory {
  const record = asRecord(value);
  const entities = asRecord(record?.entities);
  return {
    summary: readString(record?.summary),
    entities: entities || {},
    open_tasks: normalizeAssistantStringArray(record?.open_tasks),
    previous_proposed_fixes: normalizeAssistantStringArray(record?.previous_proposed_fixes),
    selected_run: readString(record?.selected_run),
    selected_pipeline: readString(record?.selected_pipeline),
    selected_scope: readString(record?.selected_scope),
    selected_docs_version: readString(record?.selected_docs_version),
  };
}

export function assistantLastUserMessage(messages: AssistantMessage[]): AssistantMessage | null {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index];
    if (message.role === 'user' && message.content.trim()) return message;
  }
  return null;
}

export function assistantMessageClipboardText(message: AssistantMessage): string {
  return message.content.trim();
}

export function assistantConversationClipboardText(conversation: AssistantConversation | null): string {
  if (!conversation) return '';
  return conversation.messages
    .map(message => `${assistantMessageAuthorLabel(message)}:\n${message.content.trim()}`)
    .filter(Boolean)
    .join('\n\n');
}

export function assistantVisibleToolActivity(messages: AssistantMessage[]): AssistantToolActivity[] {
  return messages
    .flatMap(message => message.tool_calls)
    .filter(tool => !assistantToolActivityIsInternal(tool));
}

export function assistantToolActivityIsInternal(tool: AssistantToolActivity): boolean {
  return tool.name === 'nopsai.llm.plan' || tool.name === 'nopsai.llm.complete';
}

export function assistantMessageAuthorLabel(message: AssistantMessage): string {
  if (message.role === 'user') return 'You';
  if (message.role === 'assistant') return 'Assistant';
  return message.role || 'System';
}

export function assistantMessageUsageLabel(message: AssistantMessage): string {
  const usage = message.usage || emptyAssistantMessageUsage;
  const parts: string[] = [];
  if (usage.total_tokens > 0) {
    const source = usage.llm_calls > 0 ? 'LLM token' : 'visible token';
    parts.push(`${formatAssistantNumber(usage.total_tokens)} ${source}${usage.total_tokens === 1 ? '' : 's'}${usage.estimated ? ' est.' : ''}`);
  } else if (usage.content_tokens > 0) {
    parts.push(`${formatAssistantNumber(usage.content_tokens)} visible token${usage.content_tokens === 1 ? '' : 's'} est.`);
  }
  if (usage.duration_ms > 0) parts.push(formatAssistantDuration(usage.duration_ms));
  if (usage.llm_calls > 1) parts.push(`${usage.llm_calls} LLM calls`);
  return parts.join(' · ');
}

export function assistantConversationUsageLabel(conversation: AssistantConversation | null): string {
  const usage = conversation?.usage || emptyAssistantConversationUsage;
  if (!conversation || usage.message_count === 0) return 'No usage recorded yet';
  const parts = [
    `${formatAssistantNumber(usage.total_tokens)} tokens`,
    `${usage.message_count} message${usage.message_count === 1 ? '' : 's'}`,
  ];
  if (usage.duration_ms > 0) parts.push(formatAssistantDuration(usage.duration_ms));
  if (usage.estimated_token_messages > 0) parts.push(`${usage.estimated_token_messages} estimated`);
  return parts.join(' · ');
}

export type ProposedChange = {
  key: string;
  title: string;
  body: string;
  note: string;
};

export function proposedChangesFromMessages(messages: AssistantMessage[]): ProposedChange[] {
  return messages.flatMap(message => message.tool_calls.flatMap((tool, index) => {
    const proposalType = readRecordString(tool.output, 'proposal_type');
    const yaml = readRecordString(tool.output, 'yaml');
    if (!proposalType && !yaml) return [];

    const body = yaml || prettyPrintRecord(tool.output['target']) || prettyPrintRecord(tool.output);
    return [{
      key: `${message.id}-${tool.name}-${index}`,
      title: proposalTitle(proposalType || tool.name),
      body,
      note: tool.output['applies'] === false ? 'Proposal only. No changes were applied.' : '',
    }];
  }));
}

function normalizeAssistantStringArray(value: unknown): string[] {
  const seen = new Set<string>();
  const normalized: string[] = [];
  normalizeStringArray(value).forEach(item => {
    if (seen.has(item)) return;
    seen.add(item);
    normalized.push(item);
  });
  return normalized;
}

function readBoolean(value: unknown, fallback: boolean): boolean {
  return typeof value === 'boolean' ? value : fallback;
}

function readNumber(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function readNonNegativeNumber(value: unknown): number {
  const number = readNumber(value, 0);
  return number > 0 ? Math.floor(number) : 0;
}

function normalizeAssistantMessageUsage(value: unknown): AssistantMessageUsage {
  const record = asRecord(value) || {};
  return {
    content_tokens: readNonNegativeNumber(record.content_tokens),
    prompt_tokens: readNonNegativeNumber(record.prompt_tokens),
    completion_tokens: readNonNegativeNumber(record.completion_tokens),
    total_tokens: readNonNegativeNumber(record.total_tokens),
    estimated: readBoolean(record.estimated, false),
    duration_ms: readNonNegativeNumber(record.duration_ms),
    llm_calls: readNonNegativeNumber(record.llm_calls),
  };
}

function normalizeAssistantConversationUsage(value: unknown): AssistantConversationUsage {
  const record = asRecord(value) || {};
  return {
    message_count: readNonNegativeNumber(record.message_count),
    content_tokens: readNonNegativeNumber(record.content_tokens),
    prompt_tokens: readNonNegativeNumber(record.prompt_tokens),
    completion_tokens: readNonNegativeNumber(record.completion_tokens),
    total_tokens: readNonNegativeNumber(record.total_tokens),
    estimated_token_messages: readNonNegativeNumber(record.estimated_token_messages),
    duration_ms: readNonNegativeNumber(record.duration_ms),
    llm_calls: readNonNegativeNumber(record.llm_calls),
  };
}

function assistantConversationUsageFromMessages(messages: AssistantMessage[]): AssistantConversationUsage {
  return messages.reduce<AssistantConversationUsage>((usage, message) => {
    usage.message_count += 1;
    usage.content_tokens += message.usage.content_tokens;
    usage.prompt_tokens += message.usage.prompt_tokens;
    usage.completion_tokens += message.usage.completion_tokens;
    usage.total_tokens += message.usage.total_tokens;
    usage.duration_ms += message.usage.duration_ms;
    usage.llm_calls += message.usage.llm_calls;
    if (message.usage.estimated) usage.estimated_token_messages += 1;
    return usage;
  }, { ...emptyAssistantConversationUsage });
}

function normalizeAssistantToolActivity(value: unknown): AssistantToolActivity {
  const record = asRecord(value) || {};
  return {
    name: readString(record.name),
    input: asRecord(record.input) || {},
    output: asRecord(record.output) || {},
    status: readString(record.status),
    resource_uris: normalizeStringArray(record.resource_uris),
  };
}

function formatAssistantNumber(value: number): string {
  return new Intl.NumberFormat('en-US').format(value);
}

function formatAssistantDuration(durationMs: number): string {
  if (durationMs < 1000) return `${durationMs}ms`;
  const seconds = durationMs / 1000;
  if (seconds < 10) return `${seconds.toFixed(1).replace(/\.0$/, '')}s`;
  return `${Math.round(seconds)}s`;
}

function proposalTitle(value: string): string {
  return value
    .replace(/^nopsai\./, '')
    .replace(/_/g, ' ')
    .replace(/\b\w/g, char => char.toUpperCase());
}

function readRecordString(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  return typeof value === 'string' ? value.trim() : '';
}

function prettyPrintRecord(value: unknown): string {
  if (!value) return '';
  try {
    return JSON.stringify(value, null, 2) || '';
  } catch {
    return String(value);
  }
}
