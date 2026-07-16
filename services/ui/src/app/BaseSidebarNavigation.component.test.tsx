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
            { label: 'Teams', path: '/teams', icon: <span /> },
            { label: 'LLM Profiles', path: '/llm-profiles', icon: <span /> },
            { label: 'Credentials', path: '/credentials', icon: <span /> },
            { label: 'System', path: '/system/config', icon: <span /> },
          ]}
          systemSubNav={[{ label: 'Access', path: '/system/access', icon: <span /> }]}
        />
      </MemoryRouter>
    );

    expect(screen.getByRole('link', { name: 'Pipeline runs' })).toHaveAttribute('href', '/pipelineruns/main');
    expect(screen.getByRole('link', { name: 'Pipeline runs' })).toHaveAttribute('title', 'Pipeline runs');
    expect(screen.getAllByRole('group').map(group => group.getAttribute('aria-label'))).toEqual([
      'Operate navigation',
      'Build & Automate navigation',
      'Organization navigation',
      'AI & Knowledge navigation',
      'Platform navigation',
      'System Settings navigation',
    ]);
    expect(screen.getByRole('link', { name: 'Pipelines' }).closest('[aria-label="Build & Automate navigation"]')).not.toBeNull();
    expect(screen.getByRole('link', { name: 'Assistant' }).closest('[aria-label="AI & Knowledge navigation"]')).not.toBeNull();
    expect(screen.getByRole('link', { name: 'Teams' }).closest('[aria-label="Organization navigation"]')).not.toBeNull();
    const llmProfilesLink = screen.getByRole('link', { name: 'LLM Profiles' });
    expect(llmProfilesLink).toHaveAttribute('href', '/llm-profiles');
    expect(llmProfilesLink.closest('[aria-label="System Settings navigation"]')).toBeNull();
    expect(screen.getByRole('link', { name: 'Credentials' }).closest('[aria-label="Platform navigation"]')).not.toBeNull();
    expect(screen.getByRole('button', { name: 'System Settings' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('link', { name: 'Access' })).toHaveAttribute('aria-label', 'Access');
    expect(screen.getByRole('link', { name: 'Access' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Access' })).toHaveClass('active');
    expect(screen.getByRole('link', { name: 'Access' }).closest('[aria-label="System Settings navigation"]')).not.toBeNull();
    expect(screen.queryByRole('link', { name: 'System' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Schedules' })).not.toBeInTheDocument();
  });

  it('defaults System Settings to collapsed and lets categories toggle', () => {
    render(
      <MemoryRouter initialEntries={['/pipelines']}>
        <BaseSidebarNavigation
          locationPathname="/pipelines"
          navItems={[
            { label: 'Pipeline runs', path: '/pipelineruns/main', icon: <span /> },
            { label: 'Pipelines', path: '/pipelines', icon: <span /> },
          ]}
          systemSubNav={[{ label: 'Config', path: '/system/config', icon: <span /> }]}
        />
      </MemoryRouter>
    );

    const buildButton = screen.getByRole('button', { name: 'Build & Automate' });
    const systemButton = screen.getByRole('button', { name: 'System Settings' });

    expect(buildButton).toHaveAttribute('aria-expanded', 'true');
    expect(systemButton).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('link', { name: 'Config' })).not.toBeInTheDocument();

    fireEvent.click(systemButton);

    expect(systemButton).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('link', { name: 'Config' })).toBeVisible();

    fireEvent.click(buildButton);

    expect(buildButton).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('link', { name: 'Pipelines' })).not.toBeInTheDocument();
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
