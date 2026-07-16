import { render, screen } from '@testing-library/react';
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
    </MemoryRouter>,
  );
}

describe('ProductDocsPage', () => {
  it('renders a calm documentation shell with route-backed navigation', () => {
    renderDocs();

    expect(screen.getByRole('heading', { name: 'NopsAI Documentation' })).toBeVisible();
    expect(screen.getByText('Current implementation, grounded in repository sources')).toBeVisible();
    expect(screen.getByRole('heading', { name: 'What NopsAI Is' })).toBeVisible();
    expect(screen.getByLabelText('Search documentation')).toBeVisible();
    expect(screen.getByText('Page details')).toBeVisible();
    expect(screen.queryByText('Source Priority')).not.toBeInTheDocument();
  });

  it('opens a reference field without using a wide configuration table', async () => {
    const user = userEvent.setup();
    renderDocs('/docs/installation/docker-compose');

    expect(screen.getByRole('heading', { name: 'Docker Compose' })).toBeVisible();
    const fieldSummary = screen.getByText('SYSTEM_LOGS_DOCKER_HOST');
    expect(fieldSummary).toBeVisible();
    await user.click(fieldSummary);
    expect(screen.getByText('tcp://docker-socket-proxy:2375')).toBeVisible();
    expect(screen.queryByRole('columnheader', { name: 'Field path' })).not.toBeInTheDocument();
  });

  it('returns ranked field-level search results and opens the matching article', async () => {
    const user = userEvent.setup();
    renderDocs();

    await user.type(screen.getByLabelText('Search documentation'), 'steps[].llm_profile');

    const result = screen.getByRole('button', { name: /field steps\[\]\.llm_profile/i });
    expect(result).toBeVisible();
    await user.click(result);

    expect(screen.getByRole('heading', { name: 'Step and Task Directives' })).toBeVisible();
    expect(screen.getByText('steps[].llm_profile')).toBeVisible();
  });

  it('labels unverified field metadata instead of presenting inferred values as facts', async () => {
    const user = userEvent.setup();
    renderDocs('/docs/automation/pipeline-schema');

    const nameField = screen.getByText('name', { selector: 'code' });
    await user.click(nameField);

    expect(screen.getAllByText('Metadata incomplete').length).toBeGreaterThan(0);
    expect(screen.getByText(/has not been explicitly verified/i)).toBeVisible();
  });

  it('renders repository sources as links', async () => {
    renderDocs('/docs/getting-started/first-script-pipeline');

    const user = userEvent.setup();
    await user.click(screen.getByText('Sources', { selector: 'summary' }));
    const source = screen.getByRole('link', { name: /runtime-flows\.md/i });
    expect(source).toHaveAttribute('href', expect.stringContaining('/blob/main/doc/runtime-flows.md'));
  });

  it('does not present placeholder operational tasks as complete runbooks', () => {
    renderDocs('/docs/security-reference/troubleshooting');

    expect(screen.getByRole('heading', { name: 'Operational guidance' })).toBeVisible();
    expect(screen.getByText('Related operational tasks')).toBeVisible();
    expect(screen.getByText(/detailed runbook steps have not yet been documented/i)).toBeVisible();
    expect(screen.queryByText('Diagnostic commands')).not.toBeInTheDocument();
  });
});
