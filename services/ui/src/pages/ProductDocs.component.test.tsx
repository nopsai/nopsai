import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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
  beforeEach(() => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

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

    fireEvent.change(screen.getByLabelText('Search documentation'), { target: { value: 'steps[].llm_profile' } });

    const result = screen.getByText('steps[].llm_profile').closest('button');
    expect(result).toBeTruthy();
    await user.click(result as HTMLElement);

    expect(screen.getByRole('heading', { name: 'Step and Task Directives' })).toBeVisible();
    expect(screen.getAllByText('steps[].llm_profile').length).toBeGreaterThan(0);
  });

  it('starts a selected article at the top of the page', async () => {
    const user = userEvent.setup();
    const originalRAF = window.requestAnimationFrame;
    const scrollTo = vi.mocked(window.scrollTo);
    window.requestAnimationFrame = (callback: FrameRequestCallback) => {
      callback(0);
      return 0;
    };

    try {
      renderDocs('/docs/installation/docker-compose');
      scrollTo.mockClear();

      await user.click(screen.getByRole('button', { name: 'Kubernetes and Helm' }));

      expect(screen.getByRole('heading', { name: 'Kubernetes and Helm' })).toBeVisible();
      expect(scrollTo).toHaveBeenCalledWith({ top: 0, left: 0, behavior: 'auto' });
    } finally {
      window.requestAnimationFrame = originalRAF;
      scrollTo.mockRestore();
    }
  });

  it('labels unverified field metadata instead of presenting inferred values as facts', async () => {
    const user = userEvent.setup();
    renderDocs('/docs/automation/pipeline-schema');

    const nameField = screen.getByText('name', { selector: 'code' });
    await user.click(nameField);

    expect(screen.getAllByText('Metadata incomplete').length).toBeGreaterThan(0);
    expect(screen.getAllByText(/has not been explicitly verified/i).length).toBeGreaterThan(0);
  });

  it('renders implementation evidence without outbound documentation links', async () => {
    renderDocs('/docs/getting-started/first-script-pipeline');

    const user = userEvent.setup();
    await user.click(screen.getByText('Implementation evidence', { selector: 'summary' }));

    expect(screen.getByText('runtime-flows.md')).toBeVisible();
    expect(screen.getByText('doc/runtime-flows.md')).toBeVisible();
    expect(screen.queryByRole('link', { name: /runtime-flows\.md/i })).not.toBeInTheDocument();
  });

  it('does not present placeholder operational tasks as complete runbooks', () => {
    renderDocs('/docs/security-reference/troubleshooting');

    expect(screen.getByRole('heading', { name: 'Operational guidance' })).toBeVisible();
    expect(screen.getByText('Related operational tasks')).toBeVisible();
    expect(screen.getByText(/detailed runbook steps have not yet been documented/i)).toBeVisible();
    expect(screen.queryByText('Diagnostic commands')).not.toBeInTheDocument();
  });
});
