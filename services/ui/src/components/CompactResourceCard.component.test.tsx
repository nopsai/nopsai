import { fireEvent, render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { CompactResourceCard } from './CompactResourceCard';

test('renders compact resource identity, metadata, selection, and independent actions', () => {
  const onSelect = vi.fn();
  const onAction = vi.fn();

  render(
    <CompactResourceCard
      icon={<svg data-testid="resource-icon" />}
      tone="violet"
      title="Nightly deploy"
      subtitle="/platform"
      description="Runs the deployment pipeline overnight."
      badges={<span>Enabled</span>}
      facts={[
        { label: 'Pipeline', value: 'platform/deploy', mono: true, title: 'platform/deploy' },
        { label: 'Next run', value: 'Tomorrow' },
      ]}
      selected
      selectionLabel="Select Nightly deploy"
      onSelect={onSelect}
      headingActions={<button type="button">Run now</button>}
      footerActions={<button type="button">Latest run</button>}
      actions={<button type="button" onClick={onAction}>Edit</button>}
    />
  );

  const card = screen.getByRole('article');
  expect(card).toHaveClass('glass-card', 'pipeline-card');
  expect(card).toHaveClass('compact-resource-card--selected');
  expect(card).toHaveClass('compact-resource-card--violet');
  expect(card).toHaveAttribute('data-selected', 'true');
  expect(card.querySelector('.compact-resource-card__heading')).not.toBeNull();
  const identityRow = card.querySelector('.compact-resource-card__identity-row');
  const icon = screen.getByTestId('resource-icon');
  expect(identityRow).not.toBeNull();
  expect(icon.parentElement).toHaveClass('compact-resource-card__icon');
  expect(card.querySelector('.compact-resource-card__description-slot')).not.toBeNull();
  expect(card.querySelector('.compact-resource-card__actions')).not.toBeNull();
  const identity = card.querySelector('.compact-resource-card__identity');
  const footer = card.querySelector('.compact-resource-card__footer');
  const headingActions = card.querySelector('.compact-resource-card__heading-actions');
  const badges = card.querySelector('.compact-resource-card__badges');
  expect(identity).not.toBeNull();
  expect(footer).not.toBeNull();
  expect(headingActions).not.toBeNull();
  expect(badges).not.toBeNull();
  expect(identityRow).toContainElement(headingActions);
  expect(headingActions).toContainElement(screen.getByRole('button', { name: 'Run now' }));
  expect(card.querySelector('.compact-resource-card__heading .compact-resource-card__badges')).toBeNull();
  expect(footer).toContainElement(badges);
  expect(card.querySelector('.compact-resource-card__footer-actions')).toContainElement(
    screen.getByRole('button', { name: 'Latest run' })
  );
  expect(screen.getByText('Runs the deployment pipeline overnight.')).toBeVisible();
  expect(screen.queryByText('Schedule')).not.toBeInTheDocument();
  expect(screen.getByText('platform/deploy')).toHaveClass('font-mono');
  expect(screen.getByText('platform/deploy')).toHaveAttribute('title', 'platform/deploy');

  const selectionButton = screen.getByRole('button', { name: 'Select Nightly deploy' });
  expect(selectionButton).not.toContainElement(screen.getByRole('heading', { name: 'Nightly deploy' }));

  fireEvent.click(selectionButton);
  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));

  expect(onSelect).toHaveBeenCalledOnce();
  expect(onAction).toHaveBeenCalledOnce();
});

test('supports a static card without optional content', () => {
  render(<CompactResourceCard icon={<span>Icon</span>} title="Static resource" facts={[]} />);

  expect(screen.getByRole('heading', { name: 'Static resource' })).toBeVisible();
  expect(screen.queryByRole('button')).not.toBeInTheDocument();
  expect(screen.getByRole('article')).not.toHaveAttribute('data-selected');
  expect(screen.getByRole('article')).toHaveClass('compact-resource-card--static');
});
