import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import AppHelp from './AppHelp';
import { buildDocumentationHref, resolveHelpTopicKey } from './appHelpModel';

function RouteHarness() {
  const navigate = useNavigate();
  return (
    <>
      <button type="button" onClick={() => navigate('/scopes/default')}>
        Go to scopes
      </button>
      <AppHelp />
    </>
  );
}

describe('AppHelp helpers', () => {
  it('resolves exact, fallback, and default topics', () => {
    expect(resolveHelpTopicKey('/pipelineruns/recent/run-1')).toBe('pipelineruns/recent');
    expect(resolveHelpTopicKey('/pipelineruns')).toBe('pipelineruns/main');
    expect(resolveHelpTopicKey('/system')).toBe('system/config');
    expect(resolveHelpTopicKey('/knowledge-context/runbook/platform/restart')).toBe('knowledge-context');
    expect(resolveHelpTopicKey('/unknown')).toBe('unknown');
    expect(resolveHelpTopicKey('/')).toBe('default');
  });

  it('constructs normalized documentation links only when a base URL is configured', () => {
    expect(buildDocumentationHref('/pipelines', ' https://docs.example.test/nopsai/ ')).toBe(
      'https://docs.example.test/nopsai/pipelines'
    );
    expect(buildDocumentationHref('scopes', '')).toBe('');
  });
});

describe('AppHelp', () => {
  it('exposes accessible trigger state and restores focus after Escape', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={['/triggers']}>
        <AppHelp />
      </MemoryRouter>
    );

    const trigger = screen.getByRole('button', { name: 'Help for Triggers' });
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(trigger).not.toHaveAttribute('aria-controls');

    await user.click(trigger);

    const dialog = screen.getByRole('dialog', { name: 'Triggers' });
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
    expect(trigger).toHaveAttribute('aria-controls', dialog.id);
    expect(screen.getByRole('button', { name: 'Close help' })).toHaveFocus();

    await user.keyboard('{Escape}');

    expect(screen.queryByRole('dialog', { name: 'Triggers' })).not.toBeInTheDocument();
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(trigger).toHaveFocus();
  });

  it('closes on outside pointer interaction', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={['/pipelines']}>
        <div>
          <AppHelp />
          <button type="button">Outside</button>
        </div>
      </MemoryRouter>
    );

    await user.click(screen.getByRole('button', { name: 'Help for Pipelines' }));
    expect(screen.getByRole('dialog', { name: 'Pipelines' })).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByRole('button', { name: 'Outside' }));

    await waitFor(() => {
      expect(screen.queryByRole('dialog', { name: 'Pipelines' })).not.toBeInTheDocument();
    });
  });

  it('closes stale help content when the route changes', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={['/triggers']}>
        <RouteHarness />
      </MemoryRouter>
    );

    await user.click(screen.getByRole('button', { name: 'Help for Triggers' }));
    expect(screen.getByRole('dialog', { name: 'Triggers' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Go to scopes' }));

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Help for Scopes' })).toHaveAttribute('aria-expanded', 'false');
  });
});
