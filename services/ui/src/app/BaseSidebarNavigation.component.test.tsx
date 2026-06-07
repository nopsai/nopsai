import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { BaseSidebarNavigation } from './BaseSidebarNavigation';

describe('BaseSidebarNavigation', () => {
  it('renders permitted links and expands the active system navigation', () => {
    render(
      <MemoryRouter initialEntries={['/system/access']}>
        <BaseSidebarNavigation
          locationPathname="/system/access"
          navItems={[
            { label: 'Pipeline runs', path: '/pipelineruns/main', icon: <span /> },
            { label: 'System', path: '/system/config', icon: <span /> },
          ]}
          systemSubNav={[{ label: 'Access', path: '/system/access', icon: <span /> }]}
        />
      </MemoryRouter>
    );

    expect(screen.getByRole('link', { name: 'Pipeline runs' })).toHaveAttribute('href', '/pipelineruns/main');
    expect(screen.getByRole('link', { name: 'System' })).toHaveClass('active');
    expect(screen.getByRole('link', { name: 'Access' })).toHaveAttribute('aria-current', 'page');
    expect(screen.queryByRole('link', { name: 'Schedules' })).not.toBeInTheDocument();
  });
});
