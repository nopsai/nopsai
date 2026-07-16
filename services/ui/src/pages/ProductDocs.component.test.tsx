import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import ProductDocsPage from './ProductDocs';

function renderDocs(initialEntry = '/docs') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/docs/*" element={<ProductDocsPage />} />
      </Routes>
    </MemoryRouter>
  );
}

describe('ProductDocsPage', () => {
  it('renders the product wiki with repository-grounded status and navigation', () => {
    renderDocs();

    expect(screen.getByRole('heading', { name: 'NopsAI Product Wiki' })).toBeVisible();
    expect(screen.getByText('Repository-grounded current implementation')).toBeVisible();
    expect(screen.getByRole('heading', { name: 'What NopsAI Is' })).toBeVisible();
    expect(screen.getAllByText('Product and Architecture').length).toBeGreaterThan(0);
    expect(screen.queryByText('Wiki Coverage')).not.toBeInTheDocument();
    expect(screen.queryByText('Source Priority')).not.toBeInTheDocument();
    expect(screen.queryByText('REDIS_URL')).not.toBeInTheDocument();
    expect(screen.queryByText('NOPS_STORAGE_BACKEND')).not.toBeInTheDocument();
  });

  it('opens installation articles and shows current Compose details', async () => {
    const user = userEvent.setup();
    renderDocs();

    await user.click(screen.getByRole('button', { name: 'Installation' }));
    await user.click(screen.getByRole('button', { name: /Docker Compose/ }));

    expect(screen.getByRole('heading', { name: 'Docker Compose' })).toBeVisible();
    expect(screen.getByText('http://localhost:8080', { exact: false })).toBeVisible();
    expect(screen.getByText('SYSTEM_LOGS_DOCKER_HOST')).toBeVisible();
    expect(screen.getByText('tcp://docker-socket-proxy:2375')).toBeVisible();
  });

  it('filters pages through wiki search', async () => {
    const user = userEvent.setup();
    renderDocs();

    await user.type(screen.getByLabelText(/Search wiki pages/), 'Gotenberg');

    expect(screen.getByText(/\d+ matching pages/)).toBeVisible();
    expect(screen.getByRole('button', { name: /Final Deliverables/ })).toBeVisible();
    expect(screen.getByRole('button', { name: /Docker Compose/ })).toBeVisible();
    expect(screen.queryByRole('button', { name: /First-Install Wizard/ })).not.toBeInTheDocument();
  });

  it('explains step-level llm_profile directives through search', async () => {
    const user = userEvent.setup();
    renderDocs();

    await user.type(screen.getByLabelText(/Search wiki pages/), 'steps[].llm_profile');
    await user.click(screen.getByRole('button', { name: /Step and Task Directives/ }));

    expect(screen.getByRole('heading', { name: 'Step and Task Directives' })).toBeVisible();
    expect(screen.getAllByText('steps[].llm_profile').length).toBeGreaterThan(0);
    expect(screen.getByText(/selects the LLM provider\/model/)).toBeVisible();
  });

  it('surfaces known implementation limits as wiki boundaries', async () => {
    const user = userEvent.setup();
    renderDocs();

    await user.click(screen.getByRole('button', { name: 'Security and Reference' }));
    await user.click(screen.getByRole('button', { name: /Confirmed Gaps and Limits/ }));

    expect(screen.getByRole('heading', { name: 'Confirmed Gaps and Limits' })).toBeVisible();
    const boundaries = screen.getByRole('heading', { name: 'Boundaries' }).closest('section');
    expect(boundaries).not.toBeNull();
    expect(within(boundaries as HTMLElement).getByText(/future-state capability/)).toBeVisible();
    expect(screen.getByText(/Built-in Redis dependency/)).toBeVisible();
  });

  it('deep-links to tutorial articles with prerequisites, procedure, metadata, and sources', () => {
    renderDocs('/docs/getting-started/first-script-pipeline');

    expect(screen.getByRole('heading', { name: 'Create and Run Your First Script Pipeline' })).toBeVisible();
    expect(screen.getByText('Content type')).toBeVisible();
    expect(screen.getAllByText('Tutorial').length).toBeGreaterThan(0);
    expect(screen.getByRole('heading', { name: 'Prerequisites' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Procedure' })).toBeVisible();
    expect(screen.getAllByText('llm_enabled').length).toBeGreaterThan(0);
    expect(screen.getByRole('heading', { name: 'Runbooks' })).toBeVisible();
    expect(screen.getByText('doc/runtime-flows.md')).toBeVisible();
  });
});
