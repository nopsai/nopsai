import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import ProductDocsPage from './ProductDocs';

function renderWiki(initialEntry = '/docs', props: { theme?: 'light' | 'dark'; onToggleTheme?: () => void } = {}) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/docs/*" element={<ProductDocsPage {...props} />} />
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

  it('groups the sidebar by what the reader is doing', () => {
    renderWiki();

    const nav = screen.getByRole('navigation', { name: 'Wiki pages' });
    for (const group of ['Getting started', 'Pipelines', 'Automation', 'Platform', 'Operations', 'API', 'Reference']) {
      expect(within(nav).getByText(group)).toBeVisible();
    }
  });

  it('redirects a bookmark from the previous section layout to the page it named', () => {
    renderWiki('/docs/automation/pipeline-schema');

    expect(screen.getByRole('heading', { level: 1, name: 'Pipeline anatomy' })).toBeVisible();
    const breadcrumb = screen.getByRole('navigation', { name: 'Breadcrumb' });
    expect(within(breadcrumb).getByText('Pipelines')).toBeVisible();
  });

  it('renders a reference article with a scannable field table', () => {
    renderWiki('/docs/pipelines/ai-context-and-tools');

    expect(screen.getByRole('heading', { name: 'Agent roles, knowledge, and tools' })).toBeVisible();
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
    renderWiki('/docs/pipelines/ai-context-and-tools');

    await user.click(screen.getByRole('button', { name: /^governance_level/ }));

    expect(screen.getByText('Allowed')).toBeVisible();
    // The page prose also mentions the value, so scope the assertion to the row.
    const row = screen.getByRole('button', { name: /^governance_level/ }).closest('tr')?.parentElement;
    expect(within(row as HTMLElement).getAllByText('advisory').length).toBeGreaterThan(0);
    expect(screen.getByText(/pkg\/models\/policy_merge\.go/)).toBeVisible();
  });

  it('never renders placeholder metadata for an undocumented field', () => {
    renderWiki('/docs/pipelines/script-steps');

    expect(screen.queryByText('Not documented')).not.toBeInTheDocument();
    expect(screen.queryByText('Metadata incomplete')).not.toBeInTheDocument();
    expect(screen.queryByText(/have not yet been documented/)).not.toBeInTheDocument();
  });

  it('leads a tutorial with prerequisites and numbered steps', () => {
    renderWiki('/docs/getting-started/first-script-pipeline');

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
    renderWiki('/docs/api/api-index');

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

    expect(screen.getByRole('heading', { name: 'Agent roles, knowledge, and tools' })).toBeVisible();
  });

  it('offers previous and next navigation across section boundaries', () => {
    renderWiki('/docs/getting-started/first-run-logs-history');

    const pager = screen.getByRole('navigation', { name: 'Wiki pagination' });
    expect(within(pager).getByText('Trigger a run from outside')).toBeVisible();
    expect(within(pager).getByText('Pipeline anatomy')).toBeVisible();
  });

  it('falls back to the landing page for an unknown article path', () => {
    renderWiki('/docs/getting-started/does-not-exist');

    expect(screen.getByRole('heading', { name: 'NopsAI wiki' })).toBeVisible();
  });
  it('renders standalone documentation chrome with a header search and a way back to the app', () => {
    renderWiki();

    expect(screen.getByRole('link', { name: /NopsAI docs/ })).toBeVisible();
    expect(screen.getByPlaceholderText('Search documentation')).toBeVisible();
    expect(screen.getByRole('link', { name: 'Back to NopsAI' })).toHaveAttribute('href', '/pipelineruns/main');
  });

  it('shows the full breadcrumb trail and an on-this-page rail on an article', () => {
    renderWiki('/docs/pipelines/pipeline-schema');

    const breadcrumb = screen.getByRole('navigation', { name: 'Breadcrumb' });
    expect(within(breadcrumb).getByRole('link', { name: 'NopsAI' })).toBeVisible();
    expect(within(breadcrumb).getByText('Pipelines')).toBeVisible();

    const onThisPage = screen.getByRole('complementary', { name: 'On this page' });
    expect(within(onThisPage).getByRole('link', { name: 'Field reference' })).toHaveAttribute(
      'href',
      '/docs/pipelines/pipeline-anatomy#fields',
    );
  });

  it('marks the open page in the sidebar', () => {
    renderWiki('/docs/pipelines/pipeline-anatomy');

    const nav = screen.getByRole('navigation', { name: 'Wiki pages' });
    expect(within(nav).getByRole('link', { name: 'Pipeline anatomy' })).toHaveAttribute('aria-current', 'page');
  });

  it('offers a theme control so the docs shell is readable in both themes', async () => {
    const user = userEvent.setup();
    const onToggleTheme = vi.fn();
    renderWiki('/docs', { theme: 'dark', onToggleTheme });

    await user.click(screen.getByRole('button', { name: 'Use light theme' }));
    expect(onToggleTheme).toHaveBeenCalledTimes(1);
  });

  it('filters the sidebar while searching', async () => {
    const user = userEvent.setup();
    renderWiki();

    const nav = screen.getByRole('navigation', { name: 'Wiki pages' });
    expect(within(nav).getByRole('link', { name: 'Schedules' })).toBeVisible();

    await user.type(screen.getByPlaceholderText('Search documentation'), 'schedules');

    expect(within(nav).getByRole('link', { name: 'Schedules' })).toBeVisible();
    expect(within(nav).queryByRole('link', { name: 'Approvals' })).not.toBeInTheDocument();
  });
  it('keeps similarly named field rows independent', async () => {
    const user = userEvent.setup();
    renderWiki('/docs/pipelines/script-steps');

    // The step directive and the limit that bounds it are two rows whose names
    // differ only by a suffix; expanding one must not expand the other.
    const directive = screen.getByRole('button', { name: /Named Docker volume or Kubernetes PVC mounts/ });
    const limit = screen.getByRole('button', { name: /Maximum volume mounts declared on a single step/ });
    expect(directive).not.toBe(limit);

    await user.click(directive);
    expect(screen.getByText('Mounting the reserved runtime output path /nopsai/outputs is rejected.')).toBeVisible();
    expect(limit).toBeVisible();
    expect(screen.queryByText(/has 33 volumes; maximum is 32/)).not.toBeInTheDocument();
  });

  it('does not carry an expanded field row onto the next page', async () => {
    const user = userEvent.setup();
    renderWiki('/docs/pipelines/script-steps');

    await user.click(screen.getByRole('button', { name: /Named Docker volume or Kubernetes PVC mounts/ }));
    expect(screen.getByText('Mounting the reserved runtime output path /nopsai/outputs is rejected.')).toBeVisible();

    const nav = screen.getByRole('navigation', { name: 'Wiki pages' });
    await user.click(within(nav).getByRole('link', { name: 'Final deliverables' }));

    expect(screen.getByRole('heading', { level: 1, name: 'Final deliverables' })).toBeVisible();
    expect(screen.queryByRole('button', { name: /steps\[\]\.volumes/ })).not.toBeInTheDocument();
    expect(
      screen.queryByText('Mounting the reserved runtime output path /nopsai/outputs is rejected.'),
    ).not.toBeInTheDocument();
  });
  it('keeps on-this-page links on the current article', async () => {
    const user = userEvent.setup();
    renderWiki('/docs/pipelines/script-steps');

    const onThisPage = screen.getByRole('complementary', { name: 'On this page' });
    const fieldsLink = within(onThisPage).getByRole('link', { name: 'Field reference' });

    // The app runs under a HashRouter, so a raw `#fields` href would replace the
    // route rather than move within the page.
    expect(fieldsLink.getAttribute('href')).toContain('/docs/pipelines/script-steps');
    expect(fieldsLink.getAttribute('href')).toMatch(/#fields$/);

    await user.click(fieldsLink);
    expect(screen.getByRole('heading', { level: 1, name: 'Script steps' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Field reference' })).toBeVisible();
  });
  it('links onboarding prose to the page that owns the concept', async () => {
    const user = userEvent.setup();
    renderWiki('/docs/getting-started/first-run-logs-history');

    // The same title appears in the sidebar and the related-pages block, so the
    // assertion targets the link inside the article body.
    const body = screen.getByRole('article');
    await user.click(within(body).getAllByRole('link', { name: 'Pipeline runs' })[0]);

    expect(screen.getByRole('heading', { level: 1, name: 'Pipeline runs' })).toBeVisible();
  });

  it('marks the section being read in the on-this-page rail', () => {
    renderWiki('/docs/pipelines/pipeline-anatomy');

    const onThisPage = screen.getByRole('complementary', { name: 'On this page' });
    const entries = within(onThisPage).getAllByRole('link');
    for (const entry of entries) {
      expect(entry.getAttribute('href')).toContain('/docs/pipelines/pipeline-anatomy#');
    }

    // Exactly one entry is current at a time, and it is one the rail actually
    // lists. Marking several was the failure of the earlier spy, which asked
    // "is this section on screen?" — true for a tall block long after the
    // reader had scrolled past its heading — instead of "which heading did the
    // reader last pass?".
    const current = entries.filter(entry => entry.getAttribute('aria-current') === 'true');
    expect(current).toHaveLength(1);
    expect(entries).toContain(current[0]);
  });

  it('shows the brand and version in the docs header', () => {
    renderWiki();

    const brand = screen.getByRole('link', { name: /NopsAI docs/ });
    expect(brand).toHaveAttribute('href', '/docs');
    expect(within(brand).getByRole('img', { name: 'NopsAI' })).toBeVisible();
  });
});
