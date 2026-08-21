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

function renderReviewOutput() {
  render(
    <SetupReviewOutput
      aiEnabled={false}
      normalizedRepositoryTeams={[{ name: 'platform', repositories: ['acme/api'] }]}
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
    />
  );
}

test('renders setup review output and delegates env downloads', async () => {
  const user = userEvent.setup();
  const createObjectURL = vi.fn(() => 'blob:setup-env');
  const revokeObjectURL = vi.fn();
  const anchorClick = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL });
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL });

  renderReviewOutput();

  expect(screen.getByText(/LLM profile setup was skipped/i)).toBeInTheDocument();
  expect(screen.getByText('AAA_SHARED_INTERNAL_TOKEN=<generate-strong-value>')).toBeInTheDocument();
  expect(screen.getByText(/configure provider webhook settings on the git-bot deployment/i)).toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: /download all env/i }));

  expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
  expect(anchorClick).toHaveBeenCalledOnce();
  expect(revokeObjectURL).toHaveBeenCalledWith('blob:setup-env');
});

// Config resources reach the config repository through GitOps sync, so the
// finished setup page does not offer a starter-file preview or a zip to commit.
test('offers no GitOps preview or download', () => {
  renderReviewOutput();

  expect(screen.queryByRole('button', { name: /gitops/i })).not.toBeInTheDocument();
  expect(screen.queryByText('access/bootstrap.yaml')).not.toBeInTheDocument();
  expect(screen.queryByText(/download gitops zip/i)).not.toBeInTheDocument();
});
