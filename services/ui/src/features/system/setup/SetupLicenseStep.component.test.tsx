import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import { SetupLicenseStep } from './SetupLicenseStep';

const apiMocks = vi.hoisted(() => ({
  fetchSetupLicense: vi.fn(),
  acceptSetupLicense: vi.fn(),
}));

vi.mock('./api', () => apiMocks);

const noticeText = 'NopsAI Licence\n\nFree for any noncommercial purpose. Commercial use requires a written agreement.';
const digest = 'a'.repeat(64);

const unaccepted = {
  text: noticeText,
  accepted: false,
  document_version: '2026-01',
  document_sha256: digest,
  reacceptance_required: false,
};

beforeEach(() => {
  vi.clearAllMocks();
  apiMocks.fetchSetupLicense.mockResolvedValue(unaccepted);
  apiMocks.acceptSetupLicense.mockResolvedValue({
    ...unaccepted,
    accepted: true,
    accepted_at: '2026-08-23T10:00:00Z',
    accepted_by: 'admin',
    accepted_version: '2026-01',
  });
});

test('shows the full notice rather than a link to it', async () => {
  render(<SetupLicenseStep canManage onAccepted={vi.fn()} />);
  expect(await screen.findByText(/Commercial use requires a written agreement/)).toBeInTheDocument();
});

test('acceptance requires ticking the box first', async () => {
  render(<SetupLicenseStep canManage onAccepted={vi.fn()} />);

  const accept = await screen.findByRole('button', { name: /accept and continue/i });
  expect(accept).toBeDisabled();

  await userEvent.click(screen.getByRole('checkbox'));
  expect(accept).toBeEnabled();
});

test('records acceptance against the served digest and reports it upward', async () => {
  const onAccepted = vi.fn();
  render(<SetupLicenseStep canManage onAccepted={onAccepted} />);

  await userEvent.click(await screen.findByRole('checkbox'));
  await userEvent.click(screen.getByRole('button', { name: /accept and continue/i }));

  await waitFor(() => expect(apiMocks.acceptSetupLicense).toHaveBeenCalledWith(digest));
  expect(onAccepted).toHaveBeenCalled();
});

test('an administrator without system rights cannot accept', async () => {
  render(<SetupLicenseStep canManage={false} onAccepted={vi.fn()} />);

  expect(await screen.findByRole('checkbox')).toBeDisabled();
  expect(screen.getByRole('button', { name: /accept and continue/i })).toBeDisabled();
  expect(screen.getByText(/cannot change system configuration/i)).toBeInTheDocument();
});

test('an already accepted notice reports upward without asking again', async () => {
  apiMocks.fetchSetupLicense.mockResolvedValue({
    ...unaccepted,
    accepted: true,
    accepted_at: '2026-08-23T10:00:00Z',
    accepted_by: 'admin',
    accepted_version: '2026-01',
  });
  const onAccepted = vi.fn();
  render(<SetupLicenseStep canManage onAccepted={onAccepted} />);

  await waitFor(() => expect(onAccepted).toHaveBeenCalled());
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
});

test('a changed notice asks a previously accepting installation to accept again', async () => {
  apiMocks.fetchSetupLicense.mockResolvedValue({
    ...unaccepted,
    accepted: false,
    accepted_version: '2025-06',
    reacceptance_required: true,
  });
  render(<SetupLicenseStep canManage onAccepted={vi.fn()} />);

  expect(await screen.findByText(/previously accepted notice version 2025-06/i)).toBeInTheDocument();
  expect(screen.getByRole('checkbox')).toBeInTheDocument();
});

test('a failed acceptance surfaces the reason instead of appearing to succeed', async () => {
  apiMocks.acceptSetupLicense.mockRejectedValue(new Error('licence document has changed'));
  const onAccepted = vi.fn();
  render(<SetupLicenseStep canManage onAccepted={onAccepted} />);

  await userEvent.click(await screen.findByRole('checkbox'));
  await userEvent.click(screen.getByRole('button', { name: /accept and continue/i }));

  expect(await screen.findByRole('alert')).toHaveTextContent(/licence document has changed/i);
  expect(onAccepted).not.toHaveBeenCalled();
});
