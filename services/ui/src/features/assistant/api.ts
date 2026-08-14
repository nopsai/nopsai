import { apiClient } from '../../lib/api.js';
import {
  normalizeAssistantConfig,
  normalizeAssistantConversation,
  normalizeAssistantConversationsPayload,
  normalizeAssistantLLMProfilesPayload,
  normalizeAssistantMessagePayload,
  type AssistantConfig,
  type AssistantConversation,
  type AssistantConversationsPayload,
  type AssistantLLMProfilesPayload,
  type AssistantMessagePayload,
} from './model.js';
import {
  assistantPageContextIsEmpty,
  normalizeAssistantPageContext,
  type AssistantPageContext,
} from './pageContext.js';

async function responseError(response: Response, fallback: string) {
  const text = await response.text();
  return assistantReadableErrorText(text, fallback);
}

export function assistantReadableErrorText(text: string, fallback: string) {
  const trimmed = text.trim();
  if (!trimmed) return fallback;
  const isHTML = assistantResponseLooksHTML(trimmed);
  const withoutHTML = isHTML
    ? trimmed
        .replace(/<!--[\s\S]*?-->/g, ' ')
        .replace(/<[^>]*>/g, ' ')
        .replace(/&nbsp;/g, ' ')
        .replace(/&lt;/g, '<')
        .replace(/&gt;/g, '>')
        .replace(/&amp;/g, '&')
        .replace(/&quot;/g, '"')
        .replace(/&#39;/g, "'")
    : trimmed;
  const readable = withoutHTML.replace(/\s+/g, ' ').trim();
  if (!readable) return fallback;
  const summary = `${readable.slice(0, 400)}${readable.length > 400 ? '...' : ''}`;
  return isHTML ? `${fallback}: ${summary}` : summary;
}

function assistantResponseLooksHTML(text: string) {
  const lower = text.trim().toLowerCase();
  return (
    lower.startsWith('<!doctype html') ||
    lower.startsWith('<html') ||
    lower.includes('<body') ||
    lower.includes('<center>')
  );
}

export async function fetchAssistantConfig(): Promise<AssistantConfig> {
  const response = await apiClient.fetch('/v1/assistant/config', { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(await responseError(response, `Failed to load assistant config (${response.status})`));
  }
  return normalizeAssistantConfig(await response.json());
}

export async function fetchAssistantConversations(): Promise<AssistantConversationsPayload> {
  const response = await apiClient.fetch('/v1/assistant/conversations', { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(await responseError(response, `Failed to load conversations (${response.status})`));
  }
  return normalizeAssistantConversationsPayload(await response.json());
}

export async function fetchAssistantLLMProfiles(scope = ''): Promise<AssistantLLMProfilesPayload> {
  const params = new URLSearchParams();
  const normalizedScope = scope.trim().replace(/^\/+|\/+$/g, '');
  if (normalizedScope) params.set('scope', normalizedScope);
  const suffix = params.toString() ? `?${params.toString()}` : '';
  const response = await apiClient.fetch(`/v1/assistant/models${suffix}`, { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(await responseError(response, `Failed to load assistant LLM profiles (${response.status})`));
  }
  return normalizeAssistantLLMProfilesPayload(await response.json());
}

export async function createAssistantConversation(input: {
  selected_llm_profile?: string;
  docs_version?: string;
  scope?: string;
}): Promise<AssistantConversation> {
  const response = await apiClient.fetch('/v1/assistant/conversations', {
    method: 'POST',
    cache: 'no-store',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new Error(await responseError(response, `Failed to create conversation (${response.status})`));
  }
  return normalizeAssistantConversation(await response.json());
}

export async function fetchAssistantConversation(id: string): Promise<AssistantConversation> {
  const response = await apiClient.fetch(`/v1/assistant/conversations/${encodeURIComponent(id)}`, { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(await responseError(response, `Failed to load conversation (${response.status})`));
  }
  return normalizeAssistantConversation(await response.json());
}

export async function deleteAssistantConversation(id: string): Promise<void> {
  const response = await apiClient.fetch(`/v1/assistant/conversations/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    cache: 'no-store',
  });
  if (!response.ok) {
    throw new Error(await responseError(response, `Failed to delete conversation (${response.status})`));
  }
}

export async function sendAssistantMessage(input: {
  conversation_id: string;
  content: string;
  selected_llm_profile?: string;
  page_context?: Partial<AssistantPageContext> | null;
}): Promise<AssistantMessagePayload> {
  const response = await apiClient.fetch(`/v1/assistant/conversations/${encodeURIComponent(input.conversation_id)}/messages`, {
    method: 'POST',
    cache: 'no-store',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(assistantMessageRequestBody(input)),
  });
  if (!response.ok) {
    throw new Error(await responseError(response, `Failed to send message (${response.status})`));
  }
  return normalizeAssistantMessagePayload(await response.json());
}

export function assistantMessageRequestBody(input: {
  content: string;
  selected_llm_profile?: string;
  page_context?: Partial<AssistantPageContext> | null;
}) {
  const body: {
    content: string;
    selected_llm_profile: string;
    page_context?: AssistantPageContext;
  } = {
    content: input.content,
    selected_llm_profile: input.selected_llm_profile || '',
  };
  if (!assistantPageContextIsEmpty(input.page_context)) {
    body.page_context = normalizeAssistantPageContext(input.page_context);
  }
  return body;
}
