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

const versionPayload = {
  productVersion: 'dev',
  commit: 'abc1234',
  buildDate: '2026-08-20',
  apiVersion: 'v1',
  cliCompatibility: '>=0.9',
  runnerCompatibility: '>=0.9',
  runnerProtocolVersion: '3',
  releaseManifestDigest: 'sha256:feed',
};

function renderFooter(overrides: Partial<Parameters<typeof SidebarFooter>[0]> = {}) {
  const props = {
    collapsed: false,
    currentUser: { sub: 'user-1', email: 'operator@example.com' },
    onLogout: vi.fn(),
    onNavigate: vi.fn(),
    onOpenProfile: vi.fn(),
    onToggleTheme: vi.fn(),
    theme: 'light' as const,
    ...overrides,
  };
  render(
    <MemoryRouter initialEntries={['/pipelineruns/main']}>
      <SidebarFooter {...props} />
    </MemoryRouter>
  );
  return props;
}

describe('SidebarFooter', () => {
  beforeEach(() => {
    fetchPlatformVersionInfoMock.mockReset();
    fetchPlatformVersionInfoMock.mockResolvedValue(versionPayload);
  });

  it('puts profile, wiki, about, theme, and logout on one icon row', async () => {
    const user = userEvent.setup();
    const props = renderFooter();

    // One strip, one click each: the profile dropdown that used to hold theme
    // and logout is gone, and so is the stacked version line.
    const strip = screen.getByRole('group', { name: 'Account and help actions' });
    expect(strip.children).toHaveLength(5);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();

    const wikiLink = screen.getByRole('link', { name: 'Open product wiki' });
    expect(wikiLink).toHaveAttribute('href', '/docs');
    await user.click(wikiLink);
    expect(props.onNavigate).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: 'Open profile for operator@example.com' }));
    expect(props.onOpenProfile).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: 'Use dark mode' }));
    expect(props.onToggleTheme).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: 'Logout' }));
    expect(props.onLogout).toHaveBeenCalledTimes(1);
  });

  it('moves the version into an about dialog that also carries the licence', async () => {
    const user = userEvent.setup();
    renderFooter();

    await user.click(await screen.findByRole('button', { name: 'About NopsAI, version and licence' }));

    const dialog = screen.getByRole('dialog', { name: 'About NopsAI' });
    // The whole of /version, not just the number the footer used to print.
    expect(dialog).toHaveTextContent('dev');
    expect(dialog).toHaveTextContent('abc1234');
    expect(dialog).toHaveTextContent('sha256:feed');
    expect(dialog).toHaveTextContent('NopsAI Licence');
    expect(dialog).toHaveTextContent('contact@nopsai.com');
    expect(screen.getByRole('link', { name: 'nopsai.com/security' })).toHaveAttribute(
      'href',
      'https://nopsai.com/security/'
    );

    // Portalled out of the sidebar: the sidebar is transformed and clips its
    // overflow, so a dialog rendered inside it is sized to the rail and cut off.
    expect(screen.getByRole('group', { name: 'Account and help actions' })).not.toContainElement(dialog);
    expect(dialog.parentElement?.parentElement).toBe(document.body);

    await user.click(screen.getByRole('button', { name: 'Close about dialog' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('still opens about when the server does not answer /version', async () => {
    fetchPlatformVersionInfoMock.mockResolvedValue(null);
    const user = userEvent.setup();
    renderFooter();

    await user.click(screen.getByRole('button', { name: 'About NopsAI, version and licence' }));

    const dialog = screen.getByRole('dialog', { name: 'About NopsAI' });
    expect(dialog).toHaveTextContent('Build information is unavailable from this server.');
    expect(dialog).toHaveTextContent('NopsAI Licence');
  });

  it('closes the about dialog on Escape', async () => {
    const user = userEvent.setup();
    renderFooter();

    await user.click(screen.getByRole('button', { name: 'About NopsAI, version and licence' }));
    expect(screen.getByRole('dialog', { name: 'About NopsAI' })).toBeVisible();

    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('hides logout when the shell provides no handler', () => {
    renderFooter({ onLogout: undefined });

    expect(screen.queryByRole('button', { name: 'Logout' })).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Account and help actions' }).children).toHaveLength(4);
  });
});
