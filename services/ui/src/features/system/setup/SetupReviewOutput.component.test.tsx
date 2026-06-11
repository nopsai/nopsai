import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { SetupReviewOutput } from './SetupReviewOutput';

const originalCreateObjectURL = URL.createObjectURL;
const originalRevokeObjectURL = URL.revokeObjectURL;

afterEach(() => {
  vi.restoreAllMocks();
  restoreURLMethod('createObjectURL', originalCreateObjectURL);
  restoreURLMethod('revokeObjectURL', originalRevokeObjectURL);
});

function restoreURLMethod(name: 'createObjectURL' | 'revokeObjectURL', value: typeof URL.createObjectURL | typeof URL.revokeObjectURL) {
  if (value) {
    Object.defineProperty(URL, name, { configurable: true, value });
    return;
  }
  Reflect.deleteProperty(URL, name);
}

test('renders setup review output and delegates preview, zip, and env downloads', async () => {
  const user = userEvent.setup();
  const onLoadTemplates = vi.fn();
  const onDownloadGitOpsZip = vi.fn();
  const createObjectURL = vi.fn(() => 'blob:setup-env');
  const revokeObjectURL = vi.fn();
  const anchorClick = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL });
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL });

  render(
    <SetupReviewOutput
      aiEnabled={false}
      normalizedRepositoryGroups={[{ name: 'platform', repositories: ['acme/api'] }]}
      repositories={['acme/api']}
      userCount={2}
      runtimeEnvSections={[
        {
          title: 'shared by services',
          fileName: 'shared.env',
          lines: ['AAA_SHARED_INTERNAL_TOKEN=<generate-strong-value>'],
        },
      ]}
      environmentSnippet="# shared by services\nAAA_SHARED_INTERNAL_TOKEN=<generate-strong-value>"
      gitOpsStructureSnippet="platform:\n  apps:\n    - name: api"
      gitOpsFiles={['access/bootstrap.yaml', 'pipelines/setup/first-run.yaml']}
      gitBotWebhookURL="https://hooks.example.test/webhook"
      templateLoading={false}
      templatesLoaded={false}
      downloadingGitOpsZip={false}
      onLoadTemplates={onLoadTemplates}
      onDownloadGitOpsZip={onDownloadGitOpsZip}
    />
  );

  expect(screen.getByText(/LLM profile setup was skipped/i)).toBeInTheDocument();
  expect(screen.getByText('AAA_SHARED_INTERNAL_TOKEN=<generate-strong-value>')).toBeInTheDocument();
  expect(screen.getByText('access/bootstrap.yaml')).toBeInTheDocument();
  expect(screen.getByText(/https:\/\/hooks\.example\.test\/webhook/)).toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: /preview gitops files/i }));
  await user.click(screen.getByRole('button', { name: /download gitops zip/i }));
  await user.click(screen.getByRole('button', { name: /download all env/i }));

  expect(onLoadTemplates).toHaveBeenCalledOnce();
  expect(onDownloadGitOpsZip).toHaveBeenCalledOnce();
  expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
  expect(anchorClick).toHaveBeenCalledOnce();
  expect(revokeObjectURL).toHaveBeenCalledWith('blob:setup-env');
});
