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

async function responseError(response: Response, fallback: string) {
  const text = await response.text();
  return text || fallback;
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
  const response = await apiClient.fetch(`/v1/assistant/llm-profiles${suffix}`, { cache: 'no-store' });
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
}): Promise<AssistantMessagePayload> {
  const response = await apiClient.fetch(`/v1/assistant/conversations/${encodeURIComponent(input.conversation_id)}/messages`, {
    method: 'POST',
    cache: 'no-store',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      content: input.content,
      selected_llm_profile: input.selected_llm_profile || '',
    }),
  });
  if (!response.ok) {
    throw new Error(await responseError(response, `Failed to send message (${response.status})`));
  }
  return normalizeAssistantMessagePayload(await response.json());
}
