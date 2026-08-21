import type { AssistantConversation } from './model.js';

export type AssistantConversationGroup = {
  label: string;
  conversations: AssistantConversation[];
};

const dayMS = 24 * 60 * 60 * 1000;

/** Sidebar buckets, newest first. Conversations arrive already sorted by recency. */
const assistantConversationGroupOrder = [
  'Today',
  'Yesterday',
  'Previous 7 days',
  'Previous 30 days',
  'Older',
] as const;

export function groupAssistantConversations(
  conversations: AssistantConversation[],
  now: number = Date.now(),
): AssistantConversationGroup[] {
  const buckets = new Map<string, AssistantConversation[]>();
  conversations.forEach(conversation => {
    const label = assistantConversationGroupLabel(conversation, now);
    const bucket = buckets.get(label);
    if (bucket) {
      bucket.push(conversation);
      return;
    }
    buckets.set(label, [conversation]);
  });
  return assistantConversationGroupOrder
    .filter(label => buckets.has(label))
    .map(label => ({ label, conversations: buckets.get(label) || [] }));
}

export function assistantConversationGroupLabel(conversation: AssistantConversation, now: number = Date.now()): string {
  const timestamp = assistantConversationTimestamp(conversation);
  // An undated conversation cannot honestly claim a day, so it sinks to the bottom.
  if (timestamp === null) return 'Older';
  const today = startOfDay(now);
  if (timestamp >= today) return 'Today';
  if (timestamp >= today - dayMS) return 'Yesterday';
  if (timestamp >= today - 7 * dayMS) return 'Previous 7 days';
  if (timestamp >= today - 30 * dayMS) return 'Previous 30 days';
  return 'Older';
}

export function assistantConversationTimestamp(conversation: AssistantConversation): number | null {
  const updated = Date.parse(conversation.updated_at);
  if (Number.isFinite(updated)) return updated;
  const created = Date.parse(conversation.created_at);
  return Number.isFinite(created) ? created : null;
}

export function assistantConversationTitle(conversation: AssistantConversation): string {
  return conversation.title.trim() || 'Untitled conversation';
}

function startOfDay(value: number): number {
  const date = new Date(value);
  date.setHours(0, 0, 0, 0);
  return date.getTime();
}
