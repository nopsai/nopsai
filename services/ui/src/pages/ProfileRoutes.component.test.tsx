import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { expect, test, vi } from 'vitest';

vi.mock('../features/system/LLMProfilesPanel', () => ({
  default: ({
    selectedProfileName,
    onSelectedProfileNameChange,
  }: {
    selectedProfileName: string;
    onSelectedProfileNameChange: (profileName: string) => void;
  }) => (
    <section>
      <span data-testid="selected-llm-profile">{selectedProfileName}</span>
      <button type="button" onClick={() => onSelectedProfileNameChange('platform/ml/reasoning')}>
        Open LLM profile
      </button>
    </section>
  ),
}));

vi.mock('../features/system/AgentProfilesPanel', () => ({
  default: ({
    selectedProfileID,
    onSelectedProfileIDChange,
  }: {
    selectedProfileID: string;
    onSelectedProfileIDChange: (profileID: string) => void;
  }) => (
    <section>
      <span data-testid="selected-agent-profile">{selectedProfileID}</span>
      <button type="button" onClick={() => onSelectedProfileIDChange('platform/ml/security-reviewer')}>
        Open agent profile
      </button>
    </section>
  ),
}));

import AgentProfilesPage from './AgentProfiles';
import LLMProfilesPage from './LLMProfiles';

function LocationProbe() {
  const location = useLocation();
  return <span data-testid="location">{location.pathname}{location.search}</span>;
}

test('LLM profile pages use nested profile routes', async () => {
  const user = userEvent.setup();
  render(
    <MemoryRouter initialEntries={['/llm-profiles?team=platform%2Fml']}>
      <LLMProfilesPage canManage />
      <LocationProbe />
    </MemoryRouter>
  );

  await user.click(screen.getByRole('button', { name: 'Open LLM profile' }));

  await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/llm-profiles/platform/ml/reasoning?team=platform%2Fml'));
  expect(screen.getByTestId('selected-llm-profile')).toHaveTextContent('platform/ml/reasoning');
});

test('Agent profile pages read direct nested profile routes', () => {
  render(
    <MemoryRouter initialEntries={['/agent-profiles/platform/ml/security-reviewer']}>
      <AgentProfilesPage canManage />
      <LocationProbe />
    </MemoryRouter>
  );

  expect(screen.getByTestId('location')).toHaveTextContent('/agent-profiles/platform/ml/security-reviewer');
  expect(screen.getByTestId('selected-agent-profile')).toHaveTextContent('platform/ml/security-reviewer');
});
