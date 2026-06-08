import { NavLink } from 'react-router-dom';
import type { NavItem } from './types';

export function BaseSidebarNavigation({
  navItems,
  systemSubNav,
  locationPathname,
}: {
  navItems: NavItem[];
  systemSubNav: NavItem[];
  locationPathname: string;
}) {
  const isSystemRoute = locationPathname.startsWith('/system');

  return (
    <nav id="sidebar-base-nav" className="px-4 py-4 flex-shrink-0 space-y-1" aria-label="Primary">
      {navItems.map(item => {
        const isSystemItem = item.path.startsWith('/system');
        const isActive = locationPathname.startsWith(item.path);
        return (
          <div key={item.path} className="space-y-1">
            <NavLink
              to={item.path}
              aria-expanded={isSystemItem ? isSystemRoute || isActive : undefined}
              aria-controls={isSystemItem && (isSystemRoute || isActive) ? 'system-subnavigation' : undefined}
              className={({ isActive: linkActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors sidebar-link ${
                  linkActive || (isSystemItem && isSystemRoute)
                    ? 'active text-[var(--text-primary)] bg-[var(--bg-tertiary)]'
                    : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)]'
                }`
              }
            >
              <span className="text-[var(--text-secondary)]">{item.icon}</span>
              <span className="truncate">{item.label}</span>
            </NavLink>
            {isSystemItem && (isSystemRoute || isActive) ? (
              <div id="system-subnavigation" className="pl-9 space-y-1" aria-label="System sections">
                {systemSubNav.map(sub => (
                  <NavLink
                    key={sub.path}
                    to={sub.path}
                    className={({ isActive: subActive }) =>
                      `flex items-center gap-2 px-3 py-1.5 rounded-md text-sm transition-colors ${
                        subActive
                          ? 'text-[var(--text-primary)] bg-[var(--bg-tertiary)]'
                          : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)]'
                      }`
                    }
                  >
                    <span className="text-[var(--text-secondary)]">{sub.icon}</span>
                    <span className="truncate">{sub.label}</span>
                  </NavLink>
                ))}
              </div>
            ) : null}
          </div>
        );
      })}
    </nav>
  );
}
