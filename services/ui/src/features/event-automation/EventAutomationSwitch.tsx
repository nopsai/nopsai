import { NavLink } from 'react-router-dom';
import { ObjectIcon } from '../../components/ObjectIcon';
import type { ObjectIconType } from '../../components/objectIconRegistry';

export type EventAutomationResource = 'triggers' | 'external-triggers' | 'git-webhook-sources';

const eventAutomationTabs: Array<{
  id: EventAutomationResource;
  label: string;
  path: string;
  icon: ObjectIconType;
}> = [
  { id: 'triggers', label: 'Triggers', path: '/triggers', icon: 'trigger' },
  { id: 'external-triggers', label: 'External API', path: '/external-triggers', icon: 'external-trigger' },
  { id: 'git-webhook-sources', label: 'Git webhooks', path: '/git-webhook-sources', icon: 'git-webhook-source' },
];

export function EventAutomationSwitch({ active }: { active: EventAutomationResource }) {
  return (
    <nav className="event-automation-switch" aria-label="Event automation resources">
      {eventAutomationTabs.map(tab => (
        <NavLink
          key={tab.id}
          to={tab.path}
          className={({ isActive }) => `event-automation-switch__item ${isActive || tab.id === active ? 'active' : ''}`}
          aria-current={tab.id === active ? 'page' : undefined}
        >
          <ObjectIcon type={tab.icon} className="h-4 w-4" />
          <span>{tab.label}</span>
        </NavLink>
      ))}
    </nav>
  );
}
