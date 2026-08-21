import assert from 'node:assert/strict';
import test from 'node:test';
import {
  assistantFailureDetailBody,
  assistantFailureTitle,
  assistantMessageFailure,
  assistantMessageProse,
  assistantSendFailure,
} from './failures.js';
import { emptyAssistantMessageUsage, type AssistantMessage, type AssistantToolActivity } from './model.js';

test('reads the provider reason from the last planner or synthesis call', () => {
  const failure = assistantMessageFailure(assistantReply(
    'I could not create a validated NopsAI tool plan for that request because the assistant LLM planner was unavailable or returned an invalid plan: failed to discover lm studio models: dial tcp 172.16.205.64:1234: i/o timeout. No changes were applied.',
    [llmActivity('nopsai.llm.plan', 'error', 'failed to discover lm studio models: dial tcp 172.16.205.64:1234: i/o timeout')],
  ));

  assert.equal(failure?.title, 'Connection timeout');
  assert.equal(failure?.detail, 'failed to discover lm studio models: dial tcp 172.16.205.64:1234: i/o timeout');
});

test('treats a recovered retry as a success', () => {
  const failure = assistantMessageFailure(assistantReply('Pipeline loaded.', [
    llmActivity('nopsai.llm.plan', 'error', 'invalid json'),
    llmActivity('nopsai.llm.plan', 'success', ''),
    llmActivity('nopsai.llm.complete', 'success', ''),
  ]));

  assert.equal(failure, null);
});

test('ignores user messages and replies without a provider reason', () => {
  assert.equal(assistantMessageFailure({ ...assistantReply('hi', []), role: 'user' }), null);
  assert.equal(assistantMessageFailure(assistantReply('Pipeline loaded.', [llmActivity('nopsai.llm.complete', 'error', '')])), null);
  assert.equal(assistantMessageFailure(assistantReply('Pipeline loaded.', [])), null);
});

test('classifies the common provider failures', () => {
  assert.equal(assistantFailureTitle('Error 429: RESOURCE_EXHAUSTED, exceeded its monthly spending cap'), 'Rate limit or quota exceeded');
  assert.equal(assistantFailureTitle('Get "http://host:1234/api": context deadline exceeded'), 'Connection timeout');
  assert.equal(assistantFailureTitle('dial tcp 10.0.0.2:11434: connect: connection refused'), 'Provider unreachable');
  assert.equal(assistantFailureTitle('401 Unauthorized: invalid api key'), 'Provider authentication failed');
  assert.equal(assistantFailureTitle('model not found: llama3.2'), 'Model not available');
  assert.equal(assistantFailureTitle('502 Bad Gateway'), 'Provider error');
  assert.equal(assistantFailureTitle('planner response is not valid json'), 'Invalid model response');
  assert.equal(assistantFailureTitle('something unexpected happened'), 'Assistant error');
});

test('indents an embedded json payload and keeps its prefix', () => {
  const body = assistantFailureDetailBody('gemini call failed: {"error":{"code":429,"status":"RESOURCE_EXHAUSTED"}}');

  assert.equal(body, 'gemini call failed:\n{\n  "error": {\n    "code": 429,\n    "status": "RESOURCE_EXHAUSTED"\n  }\n}');
  assert.equal(assistantFailureDetailBody('plain text failure'), 'plain text failure');
  assert.equal(assistantFailureDetailBody('broken {not json'), 'broken {not json');
});

test('drops the inlined reason from the prose the card already shows', () => {
  const failure = assistantSendFailure('dial tcp: i/o timeout');
  const prose = assistantMessageProse('I could not answer: dial tcp: i/o timeout. No changes were applied.', failure);

  assert.equal(prose, 'I could not answer. No changes were applied.');
  assert.equal(assistantMessageProse('Pipeline loaded.', null), 'Pipeline loaded.');
});

function assistantReply(content: string, toolCalls: AssistantToolActivity[]): AssistantMessage {
  return {
    id: 'm1',
    conversation_id: 'c1',
    role: 'assistant',
    content,
    tool_calls: toolCalls,
    usage: emptyAssistantMessageUsage,
    created_at: '2026-06-20T00:00:00Z',
  };
}

function llmActivity(name: string, status: string, fallbackReason: string): AssistantToolActivity {
  return {
    name,
    input: {},
    output: fallbackReason ? { fallback_reason: fallbackReason } : {},
    status,
    resource_uris: [],
  };
}
