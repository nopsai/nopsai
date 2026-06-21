import assert from 'node:assert/strict';
import { test } from 'node:test';
import { parseSSEBlocks } from './api.js';

test('parses complete SSE blocks and retains partial input', () => {
  const parsed = parseSSEBlocks(
    ': heartbeat\r\n\r\nid: cursor-1\r\nevent: log\r\ndata: {"line":"one"}\r\n\r\nevent: sta'
  );
  assert.deepEqual(parsed.blocks, [{ event: 'log', id: 'cursor-1', data: '{"line":"one"}' }]);
  assert.equal(parsed.rest, 'event: sta');
});

test('joins multiline SSE data fields', () => {
  const parsed = parseSSEBlocks('event: status\ndata: {"state":\ndata: "connected"}\n\n');
  assert.deepEqual(parsed.blocks, [{ event: 'status', id: '', data: '{"state":\n"connected"}' }]);
  assert.equal(parsed.rest, '');
});
