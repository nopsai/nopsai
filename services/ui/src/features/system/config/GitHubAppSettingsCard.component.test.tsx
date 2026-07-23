import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, test } from 'vitest';
import GitHubAppSettingsCard from './GitHubAppSettingsCard';
import { initialConfig, type ConfigFieldMetadata, type ConfigFormState } from './model';

const fieldMetadata: Record<string, ConfigFieldMetadata> = {
  github_app_id: {
    scope: 'runtime_reload',
    label: 'GitHub App ID',
    section: 'GitHub App',
    apply: 'Applies after reconnect',
  },
};

function GitHubAppSettingsHarness() {
  const [config, setConfig] = useState<ConfigFormState>({
    ...initialConfig,
    github_app_id: '123456',
    github_installation_id: '987654',
    github_private_key_credential_ref: 'credential://system/github/app-private-key',
    github_webhook_credential_ref: 'credential://system/github/webhook-secret',
  });

  return (
    <GitHubAppSettingsCard
      config={config}
      fieldMetadata={fieldMetadata}
      disabled={false}
      onChange={setConfig}
    />
  );
}

test('renders editable git-bot GitHub App runtime settings', async () => {
  const user = userEvent.setup();
  render(
    <MemoryRouter>
      <GitHubAppSettingsHarness />
    </MemoryRouter>
  );

  expect(screen.getByText('git-bot application')).toBeVisible();
  expect(screen.getByText('Applies after reconnect')).toBeVisible();
  expect(screen.getByLabelText(/GitHub App ID/)).toHaveValue('123456');
  expect(screen.getByLabelText(/GitHub installation ID/)).toHaveValue('987654');
  expect(screen.getByLabelText(/Private key credential ref/)).toHaveValue('credential://system/github/app-private-key');
  expect(screen.getByText('Expected type: private_key')).toBeVisible();
  expect(screen.getAllByRole('link', { name: 'Open credential' })[0]).toHaveAttribute(
    'href',
    '/credentials?credential=credential%3A%2F%2Fsystem%2Fgithub%2Fapp-private-key'
  );
  expect(screen.getByLabelText(/Webhook secret credential ref/)).toHaveValue('credential://system/github/webhook-secret');
  expect(screen.getByText('Expected type: webhook_secret')).toBeVisible();

  await user.clear(screen.getByLabelText(/GitHub App ID/));
  await user.type(screen.getByLabelText(/GitHub App ID/), '654321');

  expect(screen.getByLabelText(/GitHub App ID/)).toHaveValue('654321');
});
