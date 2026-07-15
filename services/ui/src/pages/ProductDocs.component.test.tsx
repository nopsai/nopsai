import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import ProductDocsPage from './ProductDocs';

describe('ProductDocsPage', () => {
  it('renders the product wiki with repository-grounded status and navigation', () => {
    render(<ProductDocsPage />);

    expect(screen.getByRole('heading', { name: 'NopsAI Product Wiki' })).toBeVisible();
    expect(screen.getByText('Repository-grounded current implementation')).toBeVisible();
    expect(screen.getByRole('heading', { name: 'What NopsAI Is' })).toBeVisible();
    expect(screen.getByText('Product and Architecture')).toBeVisible();
    expect(screen.queryByText('Wiki Coverage')).not.toBeInTheDocument();
    expect(screen.queryByText('Source Priority')).not.toBeInTheDocument();
    expect(screen.queryByText('REDIS_URL')).not.toBeInTheDocument();
    expect(screen.queryByText('NOPS_STORAGE_BACKEND')).not.toBeInTheDocument();
  });

  it('opens installation articles and shows current Compose details', async () => {
    const user = userEvent.setup();
    render(<ProductDocsPage />);

    await user.click(screen.getByRole('button', { name: 'Installation' }));
    await user.click(screen.getByRole('button', { name: /Docker Compose/ }));

    expect(screen.getByRole('heading', { name: 'Docker Compose' })).toBeVisible();
    expect(screen.getByText('http://localhost:8080', { exact: false })).toBeVisible();
    expect(screen.getByText('SYSTEM_LOGS_DOCKER_HOST')).toBeVisible();
    expect(screen.getByText('tcp://docker-socket-proxy:2375')).toBeVisible();
  });

  it('filters pages through wiki search', async () => {
    const user = userEvent.setup();
    render(<ProductDocsPage />);

    await user.type(screen.getByLabelText(/Search wiki pages/), 'Gotenberg');

    expect(screen.getByText(/\d+ matching pages/)).toBeVisible();
    expect(screen.getByRole('button', { name: /Final Deliverables/ })).toBeVisible();
    expect(screen.getByRole('button', { name: /Docker Compose/ })).toBeVisible();
    expect(screen.queryByRole('button', { name: /First-Install Wizard/ })).not.toBeInTheDocument();
  });

  it('surfaces known implementation limits as wiki boundaries', async () => {
    const user = userEvent.setup();
    render(<ProductDocsPage />);

    await user.click(screen.getByRole('button', { name: 'Security and Reference' }));
    await user.click(screen.getByRole('button', { name: /Confirmed Gaps and Limits/ }));

    expect(screen.getByRole('heading', { name: 'Confirmed Gaps and Limits' })).toBeVisible();
    const boundaries = screen.getByRole('heading', { name: 'Boundaries' }).closest('section');
    expect(boundaries).not.toBeNull();
    expect(within(boundaries as HTMLElement).getByText(/future-state capability/)).toBeVisible();
    expect(screen.getByText(/Built-in Redis dependency/)).toBeVisible();
  });
});
