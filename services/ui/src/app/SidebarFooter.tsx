import { BookOpen, ChevronDown, LogOut, Moon, Sun, UserRound } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { NavLink } from 'react-router-dom';
import { appVersionFooterText } from './platformVersion';
import type { CurrentUser, Theme } from './types';
import { currentUserDisplayName } from './userIdentity';
import { usePlatformVersionInfo } from './usePlatformVersion';

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
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const menuButtonRef = useRef<HTMLButtonElement | null>(null);
  const versionInfo = usePlatformVersionInfo();
  const footerText = appVersionFooterText(versionInfo);
  const displayName = currentUserDisplayName(currentUser);
  const signedInLabel = userLoading ? 'Loading...' : displayName;

  useEffect(() => {
    if (!menuOpen) return;
    const handleClick = (event: MouseEvent) => {
      if (!menuRef.current) return;
      if (!menuRef.current.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    };
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setMenuOpen(false);
        requestAnimationFrame(() => menuButtonRef.current?.focus());
      }
    };
    document.addEventListener('mousedown', handleClick);
    document.addEventListener('keydown', handleKey);
    return () => {
      document.removeEventListener('mousedown', handleClick);
      document.removeEventListener('keydown', handleKey);
    };
  }, [menuOpen]);

  const closeMenu = () => setMenuOpen(false);
  const handleProfileOpen = () => {
    closeMenu();
    onOpenProfile?.();
    onNavigate?.();
  };
  const handleThemeToggle = () => {
    closeMenu();
    onToggleTheme();
  };
  const handleLogout = () => {
    closeMenu();
    onLogout?.();
  };

  return (
    <footer className="sidebar-footer" aria-label="Account and help" data-collapsed={collapsed ? 'true' : 'false'}>
      <div className="sidebar-footer-section sidebar-footer-section--help">
        <NavLink
          to="/docs"
          className={({ isActive }) => `sidebar-footer-action ${isActive ? 'active' : ''}`}
          aria-label="Open product wiki"
          title="Product wiki"
          onClick={onNavigate}
        >
          <BookOpen className="sidebar-footer-action-icon" aria-hidden="true" />
          <span className="sidebar-footer-action-label">Wiki</span>
        </NavLink>
      </div>
      <div className="sidebar-footer-section sidebar-footer-section--account">
        <div className="sidebar-footer-profile-wrap" ref={menuRef}>
          <button
            ref={menuButtonRef}
            type="button"
            className={`sidebar-footer-action sidebar-footer-profile ${menuOpen ? 'active' : ''}`}
            onClick={() => setMenuOpen(open => !open)}
            aria-label={`Open user menu for ${displayName}`}
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            aria-controls={menuOpen ? 'sidebar-user-menu' : undefined}
            title={displayName}
          >
            <UserRound className="sidebar-footer-action-icon" aria-hidden="true" />
            <span className="sidebar-footer-profile-text">
              <span className="sidebar-footer-profile-name">{signedInLabel}</span>
              <span className="sidebar-footer-profile-kicker">Profile</span>
            </span>
            <ChevronDown className="sidebar-footer-profile-chevron" aria-hidden="true" />
          </button>
          {menuOpen && (
            <div id="sidebar-user-menu" className="sidebar-user-menu" role="menu" aria-label="User menu">
              <div className="sidebar-user-menu-header">
                <p className="sidebar-user-menu-eyebrow">Signed in as</p>
                <p className="sidebar-user-menu-name">{signedInLabel}</p>
                <p className="sidebar-user-menu-detail">Global access model</p>
              </div>
              <div className="sidebar-user-menu-actions">
                <button role="menuitem" className="sidebar-user-menu-item" type="button" onClick={handleProfileOpen}>
                  <UserRound className="sidebar-user-menu-item-icon" aria-hidden="true" />
                  <span>View profile</span>
                </button>
                <button role="menuitem" className="sidebar-user-menu-item" type="button" onClick={handleThemeToggle}>
                  {theme === 'dark' ? (
                    <Sun className="sidebar-user-menu-item-icon" aria-hidden="true" />
                  ) : (
                    <Moon className="sidebar-user-menu-item-icon" aria-hidden="true" />
                  )}
                  <span>{theme === 'dark' ? 'Use light mode' : 'Use dark mode'}</span>
                </button>
                {onLogout && (
                  <button role="menuitem" className="sidebar-user-menu-item" type="button" onClick={handleLogout}>
                    <LogOut className="sidebar-user-menu-item-icon" aria-hidden="true" />
                    <span>Logout</span>
                  </button>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
      {footerText ? (
        <div className="sidebar-footer-section sidebar-footer-section--version sidebar-footer-version" aria-label="Application version" title={footerText}>
          <span className="sidebar-footer-version-label">Version</span>
          <span className="sidebar-footer-version-value">{versionInfo?.productVersion}</span>
        </div>
      ) : null}
    </footer>
  );
}

export default SidebarFooter;
