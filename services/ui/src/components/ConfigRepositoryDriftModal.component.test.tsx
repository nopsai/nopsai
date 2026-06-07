import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ConfigRepositoryDriftModal } from './ConfigRepositoryDriftModal';

describe('ConfigRepositoryDriftModal', () => {
  it('shows changed files and dispatches refresh and push actions', async () => {
    const onRefresh = vi.fn(async () => undefined);
    const onPush = vi.fn(async () => undefined);
    render(
      <ConfigRepositoryDriftModal
        title="Platform config"
        drift={{
          can_push: true,
          base_branch: 'main',
          push_branch: 'nopsai/ui-changes',
          summary: { added: 1, modified: 0, deleted: 0, unchanged: 0 },
          items: [
            {
              path: 'pipelines/deploy.yaml',
              status: 'added',
              desired_content: 'name: deploy',
              git_content: '',
            },
          ],
        }}
        loading={false}
        error={null}
        pushing={false}
        pushResult={null}
        canPush
        onClose={vi.fn()}
        onRefresh={onRefresh}
        onPush={onPush}
      />
    );

    expect(screen.getAllByText('pipelines/deploy.yaml')).toHaveLength(2);
    expect(screen.getByText('name: deploy')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await userEvent.click(screen.getByRole('button', { name: 'Push to Git' }));
    expect(onRefresh).toHaveBeenCalledOnce();
    expect(onPush).toHaveBeenCalledOnce();
  });
});
