import { MemoryRouter } from 'react-router-dom';
import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { EventAutomationSwitch } from './EventAutomationSwitch';

test('renders event automation resource links with the active route highlighted', () => {
  render(
    <MemoryRouter initialEntries={['/external-triggers/deploy-prod']}>
      <EventAutomationSwitch active="external-triggers" />
    </MemoryRouter>
  );

  const triggers = screen.getByRole('link', { name: 'Triggers' });
  const external = screen.getByRole('link', { name: 'External API' });
  const webhooks = screen.getByRole('link', { name: 'Git webhooks' });

  expect(triggers).toHaveAttribute('href', '/triggers');
  expect(external).toHaveAttribute('href', '/external-triggers');
  expect(webhooks).toHaveAttribute('href', '/git-webhook-sources');
  expect(external).toHaveClass('active');
  expect(external).toHaveAttribute('aria-current', 'page');
});
