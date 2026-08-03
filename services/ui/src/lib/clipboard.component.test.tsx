import { afterEach, assert, test, vi } from 'vitest';
import { copyTextToClipboard } from './clipboard';

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test('copies with navigator clipboard when available', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal('navigator', { clipboard: { writeText } });

  await copyTextToClipboard('run command');

  assert.deepEqual(writeText.mock.calls, [['run command']]);
});

test('falls back to selection copy when navigator clipboard fails', async () => {
  const writeText = vi.fn().mockRejectedValue(new Error('blocked'));
  vi.stubGlobal('navigator', { clipboard: { writeText } });
  const execCommand = vi.fn().mockReturnValue(true);
  Object.defineProperty(document, 'execCommand', {
    value: execCommand,
    configurable: true,
  });

  await copyTextToClipboard('kubectl apply -f file.yaml');

  assert.deepEqual(writeText.mock.calls, [['kubectl apply -f file.yaml']]);
  assert.deepEqual(execCommand.mock.calls, [['copy']]);
  assert.equal(document.querySelector('textarea'), null);
});

test('falls back to selection copy when navigator clipboard is unavailable', async () => {
  vi.stubGlobal('navigator', {});
  const execCommand = vi.fn().mockReturnValue(true);
  Object.defineProperty(document, 'execCommand', {
    value: execCommand,
    configurable: true,
  });

  await copyTextToClipboard('name: deploy');

  assert.deepEqual(execCommand.mock.calls, [['copy']]);
  assert.equal(document.querySelector('textarea'), null);
});
