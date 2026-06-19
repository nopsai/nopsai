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
  created_at: string;
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

export function normalizeAssistantConversation(value: unknown): AssistantConversation {
  const record = asRecord(value) || {};
  return {
    id: readString(record.id),
    user_id: readString(record.user_id),
    title: readString(record.title),
    selected_llm_profile: readString(record.selected_llm_profile),
    docs_version: readString(record.docs_version) || 'auto',
    scope: readString(record.scope),
    memory: normalizeAssistantMemory(record.memory),
    messages: Array.isArray(record.messages) ? record.messages.map(normalizeAssistantMessage) : [],
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

export function normalizeAssistantMessage(value: unknown): AssistantMessage {
  const record = asRecord(value) || {};
  return {
    id: readString(record.id),
    conversation_id: readString(record.conversation_id),
    role: readString(record.role),
    content: readString(record.content),
    tool_calls: Array.isArray(record.tool_calls) ? record.tool_calls.map(normalizeAssistantToolActivity) : [],
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
