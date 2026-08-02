import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchPlatformVersionInfo } from './platformVersionApi';
import { SidebarFooter } from './SidebarFooter';

vi.mock('./platformVersionApi', () => ({
  fetchPlatformVersionInfo: vi.fn(),
}));

const fetchPlatformVersionInfoMock = vi.mocked(fetchPlatformVersionInfo);

describe('SidebarFooter', () => {
  beforeEach(() => {
    fetchPlatformVersionInfoMock.mockReset();
    fetchPlatformVersionInfoMock.mockResolvedValue({ productVersion: 'dev' });
  });

  it('keeps wiki, user profile, and version in the sidebar footer', async () => {
    const onLogout = vi.fn();
    const onNavigate = vi.fn();
    const onOpenProfile = vi.fn();
    const onToggleTheme = vi.fn();
    const user = userEvent.setup();

    render(
      <MemoryRouter initialEntries={['/pipelineruns/main']}>
        <SidebarFooter
          collapsed={false}
          currentUser={{ sub: 'user-1', email: 'operator@example.com' }}
          onLogout={onLogout}
          onNavigate={onNavigate}
          onOpenProfile={onOpenProfile}
          onToggleTheme={onToggleTheme}
          theme="light"
        />
      </MemoryRouter>
    );

    expect(await screen.findByText('dev')).toBeInTheDocument();
    const footerSections = Array.from(document.querySelectorAll('.sidebar-footer > .sidebar-footer-section'));
    expect(footerSections).toHaveLength(3);
    expect(footerSections[0]).toHaveClass('sidebar-footer-section--help');
    expect(footerSections[1]).toHaveClass('sidebar-footer-section--account');
    expect(footerSections[2]).toHaveClass('sidebar-footer-section--version');

    const wikiLink = screen.getByRole('link', { name: 'Open product wiki' });
    expect(wikiLink).toHaveAttribute('href', '/docs');

    await user.click(wikiLink);
    expect(onNavigate).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: 'Open user menu for operator@example.com' }));

    await user.click(screen.getByRole('menuitem', { name: 'Use dark mode' }));
    expect(onToggleTheme).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: 'Open user menu for operator@example.com' }));
    await user.click(screen.getByRole('menuitem', { name: 'View profile' }));
    expect(onOpenProfile).toHaveBeenCalledTimes(1);
    expect(onNavigate).toHaveBeenCalledTimes(2);

    await user.click(screen.getByRole('button', { name: 'Open user menu for operator@example.com' }));
    await user.click(screen.getByRole('menuitem', { name: 'Logout' }));
    expect(onLogout).toHaveBeenCalledTimes(1);
  });
});
