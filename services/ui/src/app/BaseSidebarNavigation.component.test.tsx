import { render, screen } from '@testing-library/react';
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
    expect(screen.getByRole('group', { name: 'Operate navigation' })).toBeVisible();
    expect(screen.getByRole('group', { name: 'Build & Automate navigation' })).toBeVisible();
    expect(screen.getByRole('group', { name: 'AI & Knowledge navigation' })).toBeVisible();
    expect(screen.getByRole('group', { name: 'Organization navigation' })).toBeVisible();
    expect(screen.getByRole('group', { name: 'Platform navigation' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Pipelines' }).closest('[aria-label="Build & Automate navigation"]')).not.toBeNull();
    expect(screen.getByRole('link', { name: 'Assistant' }).closest('[aria-label="AI & Knowledge navigation"]')).not.toBeNull();
    expect(screen.getByRole('link', { name: 'Teams' }).closest('[aria-label="Organization navigation"]')).not.toBeNull();
    const llmProfilesLink = screen.getByRole('link', { name: 'LLM Profiles' });
    expect(llmProfilesLink).toHaveAttribute('href', '/llm-profiles');
    expect(llmProfilesLink.closest('#system-subnavigation')).toBeNull();
    expect(screen.getByRole('link', { name: 'Credentials' }).closest('[aria-label="Platform navigation"]')).not.toBeNull();
    expect(screen.getByRole('link', { name: 'Access' })).toHaveAttribute('aria-label', 'Access');
    expect(screen.getByRole('link', { name: 'Access' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Access' })).toHaveClass('active');
    expect(screen.getByRole('link', { name: 'Access' }).closest('#system-subnavigation')).not.toBeNull();
    expect(screen.queryByRole('link', { name: 'System' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Schedules' })).not.toBeInTheDocument();
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
