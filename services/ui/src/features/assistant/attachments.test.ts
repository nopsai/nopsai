import assert from 'node:assert/strict';
import test from 'node:test';
import {
  appendAssistantAttachment,
  assistantAttachmentBlock,
  assistantAttachmentMaxCharacters,
} from './attachments.js';

test('fences an attachment under its file name', () => {
  assert.equal(assistantAttachmentBlock('pipeline.yaml', 'name: deploy\n'), 'Attached file pipeline.yaml:\n```\nname: deploy\n```');
});

test('escapes content that already contains a fence', () => {
  const block = assistantAttachmentBlock('notes.md', '```yaml\nname: deploy\n```');

  assert.match(block, /^Attached file notes\.md:\n````\n/);
  assert.match(block, /\n````$/);
});

test('truncates content that would not fit in the prompt', () => {
  const block = assistantAttachmentBlock('big.log', 'x'.repeat(assistantAttachmentMaxCharacters + 25));

  assert.match(block, /\.\.\. truncated 25 more characters\n```$/);
});

test('appends to an existing draft without eating it', () => {
  assert.equal(
    appendAssistantAttachment('why is this failing?', 'run.log', 'boom'),
    'why is this failing?\n\nAttached file run.log:\n```\nboom\n```'
  );
  assert.equal(appendAssistantAttachment('   ', 'run.log', 'boom'), 'Attached file run.log:\n```\nboom\n```');
});
