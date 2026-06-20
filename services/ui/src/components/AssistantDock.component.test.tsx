import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import { fetchAssistantConfig } from '../features/assistant/api.js';
import AssistantDock from './AssistantDock';

vi.mock('../features/assistant/api.js', () => ({
  fetchAssistantConfig: vi.fn(),
}));

const fetchAssistantConfigMock = vi.mocked(fetchAssistantConfig);
const enabledAssistantConfig = { enabled: true } as Awaited<ReturnType<typeof fetchAssistantConfig>>;

beforeEach(() => {
  fetchAssistantConfigMock.mockReset();
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
