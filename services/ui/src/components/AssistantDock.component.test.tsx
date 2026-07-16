import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import { fetchAssistantConfig } from '../features/assistant/api.js';
import { AssistantPanel } from '../features/assistant/AssistantPanel.js';
import AssistantDock from './AssistantDock';

vi.mock('../features/assistant/api.js', () => ({
  fetchAssistantConfig: vi.fn(),
}));

vi.mock('../features/assistant/AssistantPanel.js', () => ({
  AssistantPanel: vi.fn(() => <div>Assistant overlay</div>),
}));

const fetchAssistantConfigMock = vi.mocked(fetchAssistantConfig);
const AssistantPanelMock = vi.mocked(AssistantPanel);
const enabledAssistantConfig = { enabled: true } as Awaited<ReturnType<typeof fetchAssistantConfig>>;

beforeEach(() => {
  fetchAssistantConfigMock.mockReset();
  AssistantPanelMock.mockClear();
});

test('hides the floating assistant trigger on the assistant page', () => {
  fetchAssistantConfigMock.mockResolvedValue(enabledAssistantConfig);

  render(
    <MemoryRouter initialEntries={['/assistant']}>
      <AssistantDock />
    </MemoryRouter>
  );

  expect(screen.queryByRole('button', { name: 'Open Nopsai AI Assistant' })).not.toBeInTheDocument();
  expect(fetchAssistantConfigMock).not.toHaveBeenCalled();
});

test('shows the floating assistant trigger away from the assistant page when enabled', async () => {
  fetchAssistantConfigMock.mockResolvedValue(enabledAssistantConfig);

  render(
    <MemoryRouter initialEntries={['/pipelines']}>
      <AssistantDock />
    </MemoryRouter>
  );

  expect(await screen.findByRole('button', { name: 'Open Nopsai AI Assistant' })).toBeVisible();
});

test('opens the floating assistant as a fresh chat', async () => {
  const user = userEvent.setup();
  fetchAssistantConfigMock.mockResolvedValue(enabledAssistantConfig);

  render(
    <MemoryRouter initialEntries={['/pipelines']}>
      <AssistantDock />
    </MemoryRouter>
  );

  await user.click(await screen.findByRole('button', { name: 'Open Nopsai AI Assistant' }));

  expect(screen.getByText('Assistant overlay')).toBeVisible();
  expect(AssistantPanelMock.mock.calls[0]?.[0]).toEqual(expect.objectContaining({ variant: 'dock', startFresh: true }));
});
