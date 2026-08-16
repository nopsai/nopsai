import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import ProductDocsPage from './ProductDocs';

function renderWiki(initialEntry = '/docs') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/docs/*" element={<ProductDocsPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('ProductDocsPage', () => {
  beforeEach(() => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('opens on a landing page with task-based entry points instead of an article', () => {
    renderWiki();

    expect(screen.getByRole('heading', { name: 'NopsAI wiki' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Start with what you need' })).toBeVisible();
    expect(screen.getByRole('link', { name: /I am writing a pipeline/ })).toBeVisible();
    expect(screen.getAllByRole('link', { name: 'All YAML directives' }).length).toBeGreaterThan(0);
    expect(screen.queryByRole('heading', { name: 'Field reference' })).not.toBeInTheDocument();
  });

  it('groups the sidebar by reader intent', () => {
    renderWiki();

    const nav = screen.getByRole('navigation', { name: 'Wiki pages' });
    expect(within(nav).getByText('Learn the product')).toBeVisible();
    expect(within(nav).getByText('Build automation')).toBeVisible();
    expect(within(nav).getByText('Run the platform')).toBeVisible();
    expect(within(nav).getByText('Look something up')).toBeVisible();
  });

  it('renders a reference article with a scannable field table', () => {
    renderWiki('/docs/automation/pipeline-schema');

    expect(screen.getByRole('heading', { name: 'Pipeline YAML schema' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Field reference' })).toBeVisible();

    const table = screen.getAllByRole('table')[0];
    expect(within(table).getByRole('columnheader', { name: 'Directive' })).toBeVisible();
    expect(within(table).getByRole('columnheader', { name: 'Required' })).toBeVisible();
    expect(within(table).getByRole('columnheader', { name: 'Default' })).toBeVisible();
    const governance = within(table).getByRole('button', { name: /^governance_level/ });
    expect(governance).toBeVisible();
    expect(within(table).getAllByText('strict').length).toBeGreaterThan(0);
  });

  it('expands a field row to show rules, allowed values, and evidence', async () => {
    const user = userEvent.setup();
    renderWiki('/docs/automation/pipeline-schema');

    await user.click(screen.getByRole('button', { name: /^governance_level/ }));

    expect(screen.getByText('Allowed')).toBeVisible();
    expect(screen.getByText('advisory')).toBeVisible();
    expect(screen.getByText(/pkg\/models\/policy_merge\.go/)).toBeVisible();
  });

  it('never renders placeholder metadata for an undocumented field', () => {
    renderWiki('/docs/automation/step-task-directives');

    expect(screen.queryByText('Not documented')).not.toBeInTheDocument();
    expect(screen.queryByText('Metadata incomplete')).not.toBeInTheDocument();
    expect(screen.queryByText(/have not yet been documented/)).not.toBeInTheDocument();
  });

  it('leads a tutorial with prerequisites and numbered steps', () => {
    renderWiki('/docs/get-started/first-script-pipeline');

    expect(screen.getByRole('heading', { name: 'What you will do' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Before you start' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Steps' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Write the pipeline' })).toBeVisible();
  });

  it('renders the generated directive index with working filters', async () => {
    const user = userEvent.setup();
    renderWiki('/docs/reference/directive-index');

    const filter = screen.getByPlaceholderText(/^Search \d+ directives$/);
    expect(screen.getByText(/^\d+ of \d+$/)).toBeVisible();

    await user.type(filter, 'dashboard.mode');
    const table = screen.getAllByRole('table')[0];
    expect(within(table).getByText('output.items[].dashboard.mode')).toBeVisible();
    expect(within(table).queryByText('cron_expression')).not.toBeInTheDocument();
  });

  it('renders the generated API index with method and access columns', () => {
    renderWiki('/docs/reference/api-index');

    const table = screen.getAllByRole('table')[0];
    expect(within(table).getByRole('columnheader', { name: 'Method' })).toBeVisible();
    expect(within(table).getByRole('columnheader', { name: 'Access' })).toBeVisible();
    expect(within(table).getByText('/v1/setup/preflight')).toBeVisible();
  });

  it('searches article prose, not only titles', async () => {
    const user = userEvent.setup();
    renderWiki();

    await user.type(screen.getByLabelText('Search the wiki'), 'ejected');

    const results = await screen.findByRole('list', { name: 'Search results' });
    expect(within(results).getAllByRole('button').length).toBeGreaterThan(0);
  });

  it('navigates to a field anchor when a search result is selected', async () => {
    const user = userEvent.setup();
    renderWiki();

    await user.type(screen.getByLabelText('Search the wiki'), 'governance_level');
    const results = await screen.findByRole('list', { name: 'Search results' });
    await user.click(within(results).getAllByRole('button')[0]);

    expect(screen.getByRole('heading', { name: 'Pipeline YAML schema' })).toBeVisible();
  });

  it('offers previous and next navigation across section boundaries', () => {
    renderWiki('/docs/start/run-lifecycle');

    const pager = screen.getByRole('navigation', { name: 'Wiki pagination' });
    expect(within(pager).getByText('Install locally with Docker Compose')).toBeVisible();
  });

  it('falls back to the landing page for an unknown article path', () => {
    renderWiki('/docs/start/does-not-exist');

    expect(screen.getByRole('heading', { name: 'NopsAI wiki' })).toBeVisible();
  });
});
