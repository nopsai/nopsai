import { asRecord, normalizeStringArray, readString } from '../system/data.js';

export type AssistantToolActivity = {
  name: string;
  input: Record<string, unknown>;
  output: Record<string, unknown>;
  status: string;
  resource_uris: string[];
  source?: string;
  phase?: string;
  confidence?: string;
  purpose?: string;
};

export type AssistantExecutionPlanStep = {
  index: number;
  title: string;
  source: string;
  phase: string;
  confidence: string;
  tool: string;
  reason: string;
  status: string;
};

export type AssistantExecutionPlan = {
  goal: string;
  intent: string;
  summary: string;
  requires_confirmation: boolean;
  steps: AssistantExecutionPlanStep[];
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
  estimated: boolean;
  duration_ms: number;
  llm_calls: number;
  /** Absent when the turn could not be priced, which is not the same as free. */
  cost_usd?: number;
};

export type AssistantConversationUsage = {
  message_count: number;
  /** The single figure the assistant panel reports. */
  spend_usd: number;
  /** Turns whose cost could not be determined; above zero means spend_usd is partial. */
  unpriced_turns?: number;
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
  estimated: false,
  duration_ms: 0,
  llm_calls: 0,
};

export const emptyAssistantConversationUsage: AssistantConversationUsage = {
  message_count: 0,
  spend_usd: 0,
  unpriced_turns: 0,
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

export function assistantExecutionPlanFromMessage(message: AssistantMessage): AssistantExecutionPlan | null {
  const tool = message.tool_calls.find(activity => activity.name === 'nopsai.assistant.execution_plan');
  if (!tool) return null;
  return normalizeAssistantExecutionPlan(tool.output.execution_plan || tool.output);
}

export function assistantMessageAuthorLabel(message: AssistantMessage): string {
  if (message.role === 'user') return 'You';
  if (message.role === 'assistant') return 'Assistant';
  return message.role || 'System';
}

export function assistantMessageUsageLabel(message: AssistantMessage): string {
  const usage = message.usage || emptyAssistantMessageUsage;
  const parts: string[] = [];
  if (usage.llm_calls > 0) {
    // An unpriced turn shows as unpriced rather than as costing nothing.
    parts.push(usage.cost_usd === undefined ? 'not priced' : formatAssistantSpend(usage.cost_usd));
  }
  if (usage.duration_ms > 0) parts.push(formatAssistantDuration(usage.duration_ms));
  if (usage.llm_calls > 1) parts.push(`${usage.llm_calls} LLM calls`);
  return parts.join(' · ');
}

export function assistantConversationUsageLabel(conversation: AssistantConversation | null): string {
  const usage = conversation?.usage || emptyAssistantConversationUsage;
  if (!conversation || usage.message_count === 0) return 'No usage recorded yet';
  const parts = [
    formatAssistantSpend(usage.spend_usd),
    `${usage.message_count} message${usage.message_count === 1 ? '' : 's'}`,
  ];
  if (usage.duration_ms > 0) parts.push(formatAssistantDuration(usage.duration_ms));
  const unpriced = usage.unpriced_turns || 0;
  if (unpriced > 0) parts.push(`${unpriced} turn${unpriced === 1 ? '' : 's'} not priced`);
  return parts.join(' · ');
}

/** Sub-cent amounts keep four decimals so a long conversation is not shown as free. */
export function formatAssistantSpend(value: number): string {
  const amount = Number(value || 0);
  if (!Number.isFinite(amount) || amount <= 0) return '$0.00';
  const fractionDigits = amount < 0.01 ? 4 : 2;
  return amount.toLocaleString(undefined, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  });
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

/** Money keeps its fraction: flooring would report every sub-dollar conversation as free. */
function readNonNegativeAmount(value: unknown): number {
  const number = readNumber(value, 0);
  return number > 0 ? number : 0;
}

function normalizeAssistantMessageUsage(value: unknown): AssistantMessageUsage {
  const record = asRecord(value) || {};
  // cost_usd stays undefined when the server could not price the turn; that is
  // deliberately distinct from a cost of zero.
  const cost = typeof record.cost_usd === 'number' && Number.isFinite(record.cost_usd) ? record.cost_usd : undefined;
  return {
    estimated: readBoolean(record.estimated, false),
    duration_ms: readNonNegativeNumber(record.duration_ms),
    llm_calls: readNonNegativeNumber(record.llm_calls),
    cost_usd: cost,
  };
}

function normalizeAssistantConversationUsage(value: unknown): AssistantConversationUsage {
  const record = asRecord(value) || {};
  return {
    message_count: readNonNegativeNumber(record.message_count),
    spend_usd: readNonNegativeAmount(record.spend_usd),
    unpriced_turns: readNonNegativeNumber(record.unpriced_turns),
    duration_ms: readNonNegativeNumber(record.duration_ms),
    llm_calls: readNonNegativeNumber(record.llm_calls),
  };
}

function assistantConversationUsageFromMessages(messages: AssistantMessage[]): AssistantConversationUsage {
  return messages.reduce<AssistantConversationUsage>((usage, message) => {
    usage.message_count += 1;
    usage.duration_ms += message.usage.duration_ms;
    usage.llm_calls += message.usage.llm_calls;
    if (message.usage.llm_calls > 0) {
      if (message.usage.cost_usd === undefined) {
        usage.unpriced_turns = (usage.unpriced_turns || 0) + 1;
      } else {
        usage.spend_usd += message.usage.cost_usd;
      }
    }
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
    source: readString(record.source).trim(),
    phase: readString(record.phase).trim(),
    confidence: readString(record.confidence).trim(),
    purpose: readString(record.purpose).trim(),
  };
}

function normalizeAssistantExecutionPlan(value: unknown): AssistantExecutionPlan | null {
  const record = asRecord(value);
  if (!record) return null;
  const steps = Array.isArray(record.steps)
    ? record.steps.map(normalizeAssistantExecutionPlanStep).filter(step => step.title || step.source || step.tool)
    : [];
  const plan = {
    goal: readString(record.goal).trim(),
    intent: readString(record.intent).trim(),
    summary: readString(record.summary).trim(),
    requires_confirmation: readBoolean(record.requires_confirmation, false),
    steps,
  };
  if (!plan.goal && !plan.summary && plan.steps.length === 0) return null;
  return plan;
}

function normalizeAssistantExecutionPlanStep(value: unknown): AssistantExecutionPlanStep {
  const record = asRecord(value) || {};
  return {
    index: readNonNegativeNumber(record.index),
    title: readString(record.title).trim(),
    source: readString(record.source).trim(),
    phase: readString(record.phase).trim(),
    confidence: readString(record.confidence).trim(),
    tool: readString(record.tool).trim(),
    reason: readString(record.reason).trim(),
    status: readString(record.status).trim(),
  };
}

function formatAssistantDuration(durationMs: number): string {
  if (durationMs < 1000) return `${durationMs}ms`;
  const seconds = durationMs / 1000;
  if (seconds < 10) return `${seconds.toFixed(1).replace(/\.0$/, '')}s`;
  return `${Math.round(seconds)}s`;
}
