import { NavLink } from 'react-router-dom';
import type { ReactNode } from 'react';
import { groupNavItemsByTopic, SIDEBAR_NAV_PLATFORM_TOPIC_ID, sidebarNavItemIsActive } from './navigationModel';
import type { NavItem } from './types';

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
  const topLevelNav = navItems.filter(item => !item.path.startsWith('/system'));
  const topics = groupNavItemsByTopic(topLevelNav);
  const platformTopic = topics.find(topic => topic.id === SIDEBAR_NAV_PLATFORM_TOPIC_ID);
  const regularTopics = topics.filter(topic => topic.id !== SIDEBAR_NAV_PLATFORM_TOPIC_ID);
  const showSystemNav = systemSubNav.length > 0 || navItems.some(item => item.path.startsWith('/system'));
  const platformItems = platformTopic?.items || [];

  return (
    <nav id="sidebar-base-nav" className="sidebar-nav" aria-label="Primary">
      {regularTopics.map(topic => (
        <SidebarNavSection
          key={topic.id}
          label={topic.label}
          items={topic.items}
          locationPathname={locationPathname}
        />
      ))}
      {platformItems.length > 0 || showSystemNav ? (
        <SidebarNavSection
          sectionID="platform-navigation"
          label={platformTopic?.label || 'Platform'}
          items={platformItems}
          locationPathname={locationPathname}
        >
          {showSystemNav ? (
            <div id="system-subnavigation" className="sidebar-nav-subsection" aria-label="System sections">
              {platformItems.length > 0 ? <div className="sidebar-nav-subseparator" aria-hidden="true" /> : null}
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
        </SidebarNavSection>
      ) : null}
    </nav>
  );
}

function SidebarNavSection({
  children,
  items,
  label,
  locationPathname,
  sectionID,
}: {
  children?: ReactNode;
  items: NavItem[];
  label: string;
  locationPathname: string;
  sectionID?: string;
}) {
  if (items.length === 0 && !children) return null;
  return (
    <div id={sectionID} className="sidebar-nav-section" role="group" aria-label={`${label} navigation`}>
      <p className="sidebar-nav-label">{label}</p>
      {items.map(item => (
        <NavLink
          key={item.path}
          to={item.path}
          className={({ isActive }) => navItemClass(isActive || sidebarNavItemIsActive(item.path, locationPathname))}
        >
          <span className="sidebar-nav-icon">{item.icon}</span>
          <span className="truncate">{item.label}</span>
        </NavLink>
      ))}
      {children}
    </div>
  );
}
