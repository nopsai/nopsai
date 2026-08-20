import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { BaseSidebarNavigation } from './BaseSidebarNavigation';

describe('BaseSidebarNavigation', () => {
  it('renders permitted links and exposes active system sections', () => {
    render(
      <MemoryRouter initialEntries={['/system/access']}>
        <BaseSidebarNavigation
          locationPathname="/system/access"
          navItems={[
            { label: 'Pipeline runs', path: '/pipelineruns/main', icon: <span /> },
            { label: 'Pipelines', path: '/pipelines', icon: <span /> },
            { label: 'Triggers', path: '/triggers', icon: <span /> },
            { label: 'Assistant', path: '/assistant', icon: <span /> },
            { label: 'Agent roles', path: '/agent-roles', icon: <span /> },
            { label: 'Models', path: '/models', icon: <span /> },
            { label: 'Knowledge', path: '/knowledge-context', icon: <span /> },
            { label: 'Teams', path: '/teams', icon: <span /> },
            { label: 'Scopes', path: '/scopes', icon: <span /> },
            { label: 'Credentials', path: '/credentials', icon: <span /> },
            { label: 'System', path: '/system/config', icon: <span /> },
          ]}
          systemSubNav={[{ label: 'Identity & Access', path: '/system/access', icon: <span /> }]}
        />
      </MemoryRouter>
    );

    expect(screen.getByRole('link', { name: 'Pipeline runs' })).toHaveAttribute('href', '/pipelineruns/main');
    expect(screen.getByRole('link', { name: 'Pipeline runs' })).toHaveAttribute('title', 'Pipeline runs');
    expect(screen.getAllByRole('group').map(group => group.getAttribute('aria-label'))).toEqual([
      'Observe navigation',
      'Build & Automate navigation',
      'AI & Knowledge navigation',
      'Workspace navigation',
      'Administration navigation',
    ]);
    expect(screen.getByRole('link', { name: 'Pipelines' }).closest('[aria-label="Build & Automate navigation"]')).not.toBeNull();
    expect(screen.getByRole('link', { name: 'Assistant' }).closest('[aria-label="AI & Knowledge navigation"]')).not.toBeNull();
    expect(screen.getByRole('link', { name: 'Teams' }).closest('[aria-label="Workspace navigation"]')).not.toBeNull();
    const modelsLink = screen.getByRole('link', { name: 'Models' });
    expect(modelsLink).toHaveAttribute('href', '/models');
    expect(modelsLink.closest('[aria-label="Administration navigation"]')).toBeNull();
    expect(screen.getByRole('link', { name: 'Credentials' }).closest('[aria-label="Workspace navigation"]')).not.toBeNull();
    expect(screen.getByRole('button', { name: 'Administration' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('link', { name: 'Identity & Access' })).toHaveAttribute('aria-label', 'Identity & Access');
    expect(screen.getByRole('link', { name: 'Identity & Access' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Identity & Access' })).toHaveClass('active');
    expect(screen.getByRole('link', { name: 'Identity & Access' }).closest('[aria-label="Administration navigation"]')).not.toBeNull();
    expect(screen.queryByRole('link', { name: 'System' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Schedules' })).not.toBeInTheDocument();
  });

  it('renders a dedicated Lab section when it is visible', () => {
    render(
      <MemoryRouter initialEntries={['/lab']}>
        <BaseSidebarNavigation
          locationPathname="/lab"
          navItems={[
            { label: 'Pipeline runs', path: '/pipelineruns/main', icon: <span /> },
            { label: 'Lab', path: '/lab', icon: <span /> },
          ]}
          systemSubNav={[]}
        />
      </MemoryRouter>
    );

    expect(screen.getAllByRole('group').map(group => group.getAttribute('aria-label'))).toEqual([
      'Observe navigation',
      'Lab navigation',
    ]);
    expect(screen.getByRole('link', { name: 'Lab' }).closest('[aria-label="Lab navigation"]')).not.toBeNull();
  });

  it('opens every category, Administration included, and lets them toggle', () => {
    render(
      <MemoryRouter initialEntries={['/pipelines']}>
        <BaseSidebarNavigation
          locationPathname="/pipelines"
          navItems={[
            { label: 'Pipeline runs', path: '/pipelineruns/main', icon: <span /> },
            { label: 'Pipelines', path: '/pipelines', icon: <span /> },
          ]}
          systemSubNav={[{ label: 'General', path: '/system/config', icon: <span /> }]}
        />
      </MemoryRouter>
    );

    const buildButton = screen.getByRole('button', { name: 'Build & Automate' });
    const administrationButton = screen.getByRole('button', { name: 'Administration' });

    // Administration used to start collapsed and open itself only once the user
    // was already on a /system route.
    expect(buildButton).toHaveAttribute('aria-expanded', 'true');
    expect(administrationButton).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('link', { name: 'General' })).toBeVisible();

    fireEvent.click(administrationButton);

    expect(administrationButton).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('link', { name: 'General' })).not.toBeInTheDocument();

    fireEvent.click(buildButton);

    expect(buildButton).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('link', { name: 'Pipelines' })).not.toBeInTheDocument();
  });

  it('stays expanded on a system route without special-casing it', () => {
    render(
      <MemoryRouter initialEntries={['/system/config']}>
        <BaseSidebarNavigation
          locationPathname="/system/config"
          navItems={[{ label: 'Pipelines', path: '/pipelines', icon: <span /> }]}
          systemSubNav={[{ label: 'General', path: '/system/config', icon: <span /> }]}
        />
      </MemoryRouter>
    );

    const administrationButton = screen.getByRole('button', { name: 'Administration' });
    expect(administrationButton).toHaveAttribute('aria-expanded', 'true');

    // Collapsing it on a /system route keeps it collapsed; the old auto-expand
    // fought the user here and needed a remembered path to stay out of the way.
    fireEvent.click(administrationButton);
    expect(administrationButton).toHaveAttribute('aria-expanded', 'false');
  });

  it('gives each category its own icon colour', () => {
    const { container } = render(
      <MemoryRouter initialEntries={['/pipelines']}>
        <BaseSidebarNavigation
          locationPathname="/pipelines"
          navItems={[
            { label: 'Pipeline runs', path: '/pipelineruns/main', icon: <span /> },
            { label: 'Pipelines', path: '/pipelines', icon: <span /> },
            { label: 'Teams', path: '/teams', icon: <span /> },
          ]}
          systemSubNav={[{ label: 'General', path: '/system/config', icon: <span /> }]}
        />
      </MemoryRouter>
    );

    // The colours are CSS keyed on the topic, so what the component owes is a
    // stable topic id on every section.
    const topics = Array.from(container.querySelectorAll('[data-topic-id]')).map(node =>
      node.getAttribute('data-topic-id')
    );
    expect(topics).toEqual(['observe', 'build-automate', 'workspace', 'administration']);
  });

  it('uses one Triggers entry for event automation routes', () => {
    render(
      <MemoryRouter initialEntries={['/external-triggers/deploy-prod']}>
        <BaseSidebarNavigation
          locationPathname="/external-triggers/deploy-prod"
          navItems={[
            { label: 'Triggers', path: '/triggers', icon: <span /> },
            { label: 'External Triggers', path: '/external-triggers', icon: <span /> },
            { label: 'Git Webhook Sources', path: '/git-webhook-sources', icon: <span /> },
          ].filter(item => item.label === 'Triggers')}
          systemSubNav={[]}
        />
      </MemoryRouter>
    );

    expect(screen.getByRole('link', { name: 'Triggers' })).toHaveClass('active');
    expect(screen.queryByRole('link', { name: 'External Triggers' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Git Webhook Sources' })).not.toBeInTheDocument();
  });
});
