import { NavLink } from 'react-router-dom';
import type { NavItem } from './types';

const primaryNavLabels = new Set(['Pipeline runs', 'Monitoring', 'Teams']);

function navItemClass(active: boolean) {
  return `sidebar-nav-link ${active ? 'active' : ''}`;
}

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
  const primaryNav = navItems.filter(item => primaryNavLabels.has(item.label));
  const resourceNav = navItems.filter(item => !primaryNavLabels.has(item.label) && !item.path.startsWith('/system'));
  const showSystemNav = systemSubNav.length > 0 || navItems.some(item => item.path.startsWith('/system'));

  return (
    <nav id="sidebar-base-nav" className="sidebar-nav" aria-label="Primary">
      <SidebarNavSection items={primaryNav} locationPathname={locationPathname} />
      <SidebarNavSection items={resourceNav} locationPathname={locationPathname} />
      {showSystemNav ? (
        <div className="sidebar-nav-section" id="system-subnavigation" aria-label="System sections">
          <div className="sidebar-nav-separator" aria-hidden="true" />
          <p className="sidebar-nav-label">System</p>
          {systemSubNav.map(sub => (
            <NavLink
              key={sub.path}
              to={sub.path}
              className={({ isActive: subActive }) => navItemClass(subActive || (isSystemRoute && locationPathname === sub.path))}
            >
              <span className="sidebar-nav-icon">{sub.icon}</span>
              <span className="truncate">{sub.label}</span>
            </NavLink>
          ))}
        </div>
      ) : null}
    </nav>
  );
}

function SidebarNavSection({
  items,
  locationPathname,
}: {
  items: NavItem[];
  locationPathname: string;
}) {
  if (items.length === 0) return null;
  return (
    <div className="sidebar-nav-section">
      {items.map(item => (
        <NavLink
          key={item.path}
          to={item.path}
          className={({ isActive }) => navItemClass(isActive || locationPathname.startsWith(item.path))}
        >
          <span className="sidebar-nav-icon">{item.icon}</span>
          <span className="truncate">{item.label}</span>
        </NavLink>
      ))}
    </div>
  );
}
