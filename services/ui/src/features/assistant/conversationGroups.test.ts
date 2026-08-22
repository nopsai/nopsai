import assert from 'node:assert/strict';
import test from 'node:test';
import {
  assistantConversationGroupLabel,
  assistantConversationTitle,
  groupAssistantConversations,
} from './conversationGroups.js';
import { emptyAssistantConversationUsage, emptyAssistantMemory, type AssistantConversation } from './model.js';

const now = new Date('2026-06-20T10:00:00Z').getTime();

test('buckets conversations into recency groups, newest group first', () => {
  const conversations = [
    conversation('c1', 'Today chat', dayOffsetISO(0)),
    conversation('c2', 'Yesterday chat', dayOffsetISO(1)),
    conversation('c3', 'This week chat', dayOffsetISO(4)),
    conversation('c4', 'This month chat', dayOffsetISO(20)),
    conversation('c5', 'Ancient chat', dayOffsetISO(120)),
  ];

  const groups = groupAssistantConversations(conversations, now);

  assert.deepEqual(groups.map(group => group.label), [
    'Today',
    'Yesterday',
    'Previous 7 days',
    'Previous 30 days',
    'Older',
  ]);
  assert.deepEqual(groups.map(group => group.conversations.map(item => item.id)), [['c1'], ['c2'], ['c3'], ['c4'], ['c5']]);
});

test('keeps input order inside a group and drops empty groups', () => {
  const groups = groupAssistantConversations([
    conversation('c1', 'Newer', dayOffsetISO(0)),
    conversation('c2', 'Older today', dayOffsetISO(0)),
    conversation('c3', 'Last month', dayOffsetISO(25)),
  ], now);

  assert.deepEqual(groups.map(group => group.label), ['Today', 'Previous 30 days']);
  assert.deepEqual(groups[0].conversations.map(item => item.id), ['c1', 'c2']);
});

test('falls back to created_at and sinks undated conversations', () => {
  const createdOnly = { ...conversation('c1', 'Created only', ''), created_at: dayOffsetISO(1) };
  const undated = { ...conversation('c2', 'Undated', ''), created_at: '' };

  assert.equal(assistantConversationGroupLabel(createdOnly, now), 'Yesterday');
  assert.equal(assistantConversationGroupLabel(undated, now), 'Older');
});

test('titles fall back to a readable placeholder', () => {
  assert.equal(assistantConversationTitle(conversation('c1', '  Pipeline slow  ', dayOffsetISO(0))), 'Pipeline slow');
  assert.equal(assistantConversationTitle(conversation('c2', '   ', dayOffsetISO(0))), 'Untitled conversation');
});

/** Anchored to the local start of day so the buckets assert the same way in every timezone. */
function dayOffsetISO(days: number): string {
  const startOfToday = new Date(now);
  startOfToday.setHours(0, 0, 0, 0);
  return new Date(startOfToday.getTime() - days * 24 * 60 * 60 * 1000 + 6 * 60 * 60 * 1000).toISOString();
}

function conversation(id: string, title: string, updatedAt: string): AssistantConversation {
  return {
    id,
    user_id: 'user:viewer',
    title,
    selected_llm_profile: 'assistant',
    docs_version: 'auto',
    scope: '',
    memory: emptyAssistantMemory,
    messages: [],
    usage: emptyAssistantConversationUsage,
    created_at: updatedAt,
    updated_at: updatedAt,
    turn_running: false,
    running_turn_started_at: '',
  };
}
