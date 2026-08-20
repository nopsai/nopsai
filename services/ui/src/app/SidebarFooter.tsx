import { BookOpen, Info, LogOut, Moon, Sun, UserRound } from 'lucide-react';
import { useState } from 'react';
import { NavLink } from 'react-router-dom';
import { AboutDialog } from './AboutDialog';
import type { CurrentUser, Theme } from './types';
import { currentUserDisplayName } from './userIdentity';
import { usePlatformVersionInfo } from './usePlatformVersion';

/**
 * One row of icons, each a single click.
 *
 * The footer used to stack a Wiki row, a profile row with a dropdown menu, and a
 * version line, which is three bands of chrome for four actions. Everything is
 * now one strip: profile, wiki, about, theme, sign out. The version moved into
 * About, which is also where the licence notice and the policy pointers live.
 */
export function SidebarFooter({
  collapsed,
  currentUser,
  onLogout,
  onNavigate,
  onOpenProfile,
  onToggleTheme,
  theme,
  userLoading,
}: {
  collapsed: boolean;
  currentUser?: CurrentUser | null;
  onLogout?: () => void;
  onNavigate?: () => void;
  onOpenProfile?: () => void;
  onToggleTheme: () => void;
  theme: Theme;
  userLoading?: boolean;
}) {
  const [aboutOpen, setAboutOpen] = useState(false);
  const versionInfo = usePlatformVersionInfo();
  const displayName = currentUserDisplayName(currentUser);
  const signedInLabel = userLoading ? 'Loading...' : displayName;
  const themeLabel = theme === 'dark' ? 'Use light mode' : 'Use dark mode';

  const handleProfileOpen = () => {
    onOpenProfile?.();
    onNavigate?.();
  };

  return (
    <footer className="sidebar-footer" aria-label="Account and help" data-collapsed={collapsed ? 'true' : 'false'}>
      <div className="sidebar-footer-utilities" role="group" aria-label="Account and help actions">
        <button
          type="button"
          className="sidebar-footer-action"
          onClick={handleProfileOpen}
          aria-label={`Open profile for ${signedInLabel}`}
          title={signedInLabel}
        >
          <UserRound className="sidebar-footer-action-icon" aria-hidden="true" />
        </button>
        <NavLink
          to="/docs"
          className={({ isActive }) => `sidebar-footer-action ${isActive ? 'active' : ''}`}
          aria-label="Open product wiki"
          title="Product wiki"
          onClick={onNavigate}
        >
          <BookOpen className="sidebar-footer-action-icon" aria-hidden="true" />
        </NavLink>
        <button
          type="button"
          className={`sidebar-footer-action ${aboutOpen ? 'active' : ''}`}
          onClick={() => setAboutOpen(true)}
          aria-label="About NopsAI, version and licence"
          title={versionInfo ? `About NopsAI ${versionInfo.productVersion}` : 'About NopsAI'}
          aria-haspopup="dialog"
          aria-expanded={aboutOpen}
        >
          <Info className="sidebar-footer-action-icon" aria-hidden="true" />
        </button>
        <button
          type="button"
          className="sidebar-footer-action"
          onClick={onToggleTheme}
          aria-label={themeLabel}
          title={themeLabel}
        >
          {theme === 'dark' ? (
            <Sun className="sidebar-footer-action-icon" aria-hidden="true" />
          ) : (
            <Moon className="sidebar-footer-action-icon" aria-hidden="true" />
          )}
        </button>
        {onLogout ? (
          <button
            type="button"
            className="sidebar-footer-action sidebar-footer-action--logout"
            onClick={onLogout}
            aria-label="Logout"
            title="Logout"
          >
            <LogOut className="sidebar-footer-action-icon" aria-hidden="true" />
          </button>
        ) : null}
      </div>
      {aboutOpen ? <AboutDialog versionInfo={versionInfo} onClose={() => setAboutOpen(false)} /> : null}
    </footer>
  );
}

export default SidebarFooter;
