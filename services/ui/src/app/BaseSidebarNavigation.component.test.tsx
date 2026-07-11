import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { BaseSidebarNavigation } from './BaseSidebarNavigation';

describe('BaseSidebarNavigation', () => {
  it('renders permitted links and exposes active system sections', () => {
    render(
      <MemoryRouter initialEntries={['/system/access']}>
        <BaseSidebarNavigation
          locationPathname="/system/access"
          navItems={[
            { label: 'Pipeline runs', path: '/pipelineruns/main', icon: <span /> },
            { label: 'LLM Profiles', path: '/llm-profiles', icon: <span /> },
            { label: 'System', path: '/system/config', icon: <span /> },
          ]}
          systemSubNav={[{ label: 'Access', path: '/system/access', icon: <span /> }]}
        />
      </MemoryRouter>
    );

    expect(screen.getByRole('link', { name: 'Pipeline runs' })).toHaveAttribute('href', '/pipelineruns/main');
    const llmProfilesLink = screen.getByRole('link', { name: 'LLM Profiles' });
    expect(llmProfilesLink).toHaveAttribute('href', '/llm-profiles');
    expect(llmProfilesLink.closest('#system-subnavigation')).toBeNull();
    expect(screen.getByText('System')).toBeVisible();
    expect(screen.getByRole('link', { name: 'Access' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Access' })).toHaveClass('active');
    expect(screen.queryByRole('link', { name: 'System' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Schedules' })).not.toBeInTheDocument();
  });
});
