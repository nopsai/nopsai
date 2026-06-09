import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { ServiceAccountTokenPanel, ServiceAccountTokenReveal } from './ServiceAccountTokenPanel';

test('handles service account token creation, reveal, and revocation', async () => {
  const onCreate = vi.fn();
  const onRevoke = vi.fn();
  const onCopy = vi.fn();
  const onTokenNameChange = vi.fn();
  const user = userEvent.setup();
  const token = {
    id: 'token-1',
    name: 'rotation',
    token: 'secret-token',
    token_suffix: 'abcd',
    created_at: '2026-06-09T10:00:00Z',
  };

  render(
    <>
      <ServiceAccountTokenReveal token={token} copyLabel="Copy" onCopy={onCopy} />
      <ServiceAccountTokenPanel
        tokens={[token]}
        loading={false}
        error={null}
        tokenName="next"
        onTokenNameChange={onTokenNameChange}
        onCreate={onCreate}
        onRevoke={onRevoke}
      />
    </>
  );

  await user.click(screen.getByRole('button', { name: /copy/i }));
  await user.click(screen.getByRole('button', { name: /create token/i }));
  await user.click(screen.getByRole('button', { name: /revoke/i }));

  expect(onCopy).toHaveBeenCalledOnce();
  expect(onCreate).toHaveBeenCalledOnce();
  expect(onRevoke).toHaveBeenCalledWith('token-1');
});
