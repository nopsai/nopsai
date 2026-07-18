import { ChevronDown } from 'lucide-react';
import { useMemo, useState } from 'react';
import { NavLink } from 'react-router-dom';
import {
  groupNavItemsByTopic,
  SIDEBAR_NAV_SYSTEM_SETTINGS_TOPIC_ID,
  SIDEBAR_NAV_SYSTEM_SETTINGS_TOPIC_LABEL,
  sidebarNavItemIsActive,
} from './navigationModel';
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
  const [collapsedSectionState, setCollapsedSections] = useState<Set<string>>(
    () => new Set(isSystemRoute ? [] : [SIDEBAR_NAV_SYSTEM_SETTINGS_TOPIC_ID])
  );
  const [systemSettingsManualCollapsePath, setSystemSettingsManualCollapsePath] = useState('');
  const collapsedSections = useMemo(() => {
    const shouldAutoExpandSystemSettings =
      isSystemRoute &&
      systemSubNav.length > 0 &&
      systemSettingsManualCollapsePath !== locationPathname &&
      collapsedSectionState.has(SIDEBAR_NAV_SYSTEM_SETTINGS_TOPIC_ID);
    if (!shouldAutoExpandSystemSettings) return collapsedSectionState;
    const next = new Set(collapsedSectionState);
    next.delete(SIDEBAR_NAV_SYSTEM_SETTINGS_TOPIC_ID);
    return next;
  }, [collapsedSectionState, isSystemRoute, locationPathname, systemSettingsManualCollapsePath, systemSubNav.length]);

  const toggleSection = (sectionID: string) => {
    if (sectionID === SIDEBAR_NAV_SYSTEM_SETTINGS_TOPIC_ID && isSystemRoute && systemSubNav.length > 0) {
      setSystemSettingsManualCollapsePath(collapsedSections.has(sectionID) ? '' : locationPathname);
    }
    setCollapsedSections(current => {
      const next = new Set(current);
      if (next.has(sectionID)) {
        next.delete(sectionID);
      } else {
        next.add(sectionID);
      }
      return next;
    });
  };

  return (
    <nav id="sidebar-base-nav" className="sidebar-nav" aria-label="Primary">
      {topics.map(topic => (
        <SidebarNavSection
          key={topic.id}
          sectionID={`${topic.id}-navigation`}
          topicID={topic.id}
          label={topic.label}
          items={topic.items}
          locationPathname={locationPathname}
          expanded={!collapsedSections.has(topic.id)}
          onToggle={() => toggleSection(topic.id)}
        />
      ))}
      {systemSubNav.length > 0 ? (
        <SidebarNavSection
          sectionID={`${SIDEBAR_NAV_SYSTEM_SETTINGS_TOPIC_ID}-navigation`}
          topicID={SIDEBAR_NAV_SYSTEM_SETTINGS_TOPIC_ID}
          label={SIDEBAR_NAV_SYSTEM_SETTINGS_TOPIC_LABEL}
          items={systemSubNav}
          locationPathname={locationPathname}
          expanded={!collapsedSections.has(SIDEBAR_NAV_SYSTEM_SETTINGS_TOPIC_ID)}
          onToggle={() => toggleSection(SIDEBAR_NAV_SYSTEM_SETTINGS_TOPIC_ID)}
        />
      ) : null}
    </nav>
  );
}

function SidebarNavSection({
  items,
  label,
  locationPathname,
  sectionID,
  topicID,
  expanded,
  onToggle,
}: {
  items: NavItem[];
  label: string;
  locationPathname: string;
  sectionID: string;
  topicID: string;
  expanded: boolean;
  onToggle: () => void;
}) {
  if (items.length === 0) return null;
  const contentID = `${sectionID}-items`;
  return (
    <div id={sectionID} className="sidebar-nav-section" role="group" aria-label={`${label} navigation`} data-topic-id={topicID}>
      <button
        type="button"
        className="sidebar-nav-label sidebar-nav-toggle"
        aria-expanded={expanded}
        aria-controls={contentID}
        onClick={onToggle}
      >
        <span className="sidebar-nav-label-text">{label}</span>
        <ChevronDown className="sidebar-nav-toggle-chevron" aria-hidden="true" />
      </button>
      <div id={contentID} className="sidebar-nav-section-content" hidden={!expanded}>
        {items.map(item => (
          <NavLink
            key={item.path}
            to={item.path}
            className={({ isActive }) => navItemClass(isActive || sidebarNavItemIsActive(item.path, locationPathname))}
            aria-label={item.label}
            title={item.label}
          >
            <span className="sidebar-nav-icon">{item.icon}</span>
            <span className="truncate">{item.label}</span>
          </NavLink>
        ))}
      </div>
    </div>
  );
}
