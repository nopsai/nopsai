import { fireEvent, render, screen } from '@testing-library/react';
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

    expect(screen.getByRole('dialog', { name: 'Platform config' })).toBeVisible();
    expect(screen.getAllByRole('button', { name: 'Close' })[0]).toHaveFocus();
    expect(screen.getAllByText('pipelines/deploy.yaml')).toHaveLength(2);
    expect(screen.getByText('name: deploy')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await userEvent.click(screen.getByRole('button', { name: 'Push to Git' }));
    expect(onRefresh).toHaveBeenCalledOnce();
    expect(onPush).toHaveBeenCalledOnce();
  });

  it('highlights changed lines in modified drift content', () => {
    render(
      <ConfigRepositoryDriftModal
        title="Platform config"
        drift={{
          can_push: true,
          base_branch: 'main',
          push_branch: 'nopsai/ui-changes',
          summary: { added: 0, modified: 1, deleted: 0, unchanged: 0 },
          items: [
            {
              path: 'setting/system/runner.yaml',
              status: 'modified',
              git_content: 'runner:\n  capacity: 1\n  scopes: dev\n',
              desired_content: 'runner:\n  capacity: 2\n  scopes: dev\n',
            },
          ],
        }}
        loading={false}
        error={null}
        pushing={false}
        pushResult={null}
        canPush
        onClose={vi.fn()}
        onRefresh={vi.fn(async () => undefined)}
        onPush={vi.fn(async () => undefined)}
      />
    );

    expect(screen.getByText('Will be written to Git - 2 highlighted lines')).toBeVisible();
    expect(screen.getByText('capacity: 1').closest('[data-drift-line-kind]')).toHaveAttribute('data-drift-line-kind', 'removed');
    expect(screen.getByText('capacity: 2').closest('[data-drift-line-kind]')).toHaveAttribute('data-drift-line-kind', 'added');
    expect(screen.getAllByText('scopes: dev')[0].closest('[data-drift-line-kind]')).toHaveAttribute('data-drift-line-kind', 'context');
  });

  it('keeps Git and Nopsai drift panes scrolled together', () => {
    const gitContent = Array.from({ length: 48 }, (_, index) => `key_${index}: git-${index}`).join('\n');
    const desiredContent = Array.from({ length: 48 }, (_, index) => `key_${index}: nopsai-${index}`).join('\n');
    render(
      <ConfigRepositoryDriftModal
        title="Platform config"
        drift={{
          can_push: true,
          base_branch: 'main',
          push_branch: 'nopsai/ui-changes',
          summary: { added: 0, modified: 1, deleted: 0, unchanged: 0 },
          items: [
            {
              path: 'setting/system/runtime.yaml',
              status: 'modified',
              git_content: gitContent,
              desired_content: desiredContent,
            },
          ],
        }}
        loading={false}
        error={null}
        pushing={false}
        pushResult={null}
        canPush
        onClose={vi.fn()}
        onRefresh={vi.fn(async () => undefined)}
        onPush={vi.fn(async () => undefined)}
      />
    );

    const gitPane = screen.getByLabelText('Git highlighted drift');
    const nopsaiPane = screen.getByLabelText('Nopsai highlighted drift');

    gitPane.scrollTop = 220;
    gitPane.scrollLeft = 36;
    fireEvent.scroll(gitPane);
    expect(nopsaiPane.scrollTop).toBe(220);
    expect(nopsaiPane.scrollLeft).toBe(36);

    nopsaiPane.scrollTop = 80;
    nopsaiPane.scrollLeft = 12;
    fireEvent.scroll(nopsaiPane);
    expect(gitPane.scrollTop).toBe(80);
    expect(gitPane.scrollLeft).toBe(12);
  });

  it('keeps the file list and diff panes inside their own scroll areas', () => {
    render(
      <ConfigRepositoryDriftModal
        title="Platform config"
        drift={{
          can_push: true,
          base_branch: 'main',
          push_branch: 'nopsai/ui-changes',
          summary: { added: 0, modified: 2, deleted: 0, unchanged: 0 },
          items: Array.from({ length: 24 }, (_, index) => ({
            path: `pipelines/pipeline-${index}.yaml`,
            status: 'modified' as const,
            git_content: Array.from({ length: 200 }, (_, line) => `key_${line}: git`).join('\n'),
            desired_content: Array.from({ length: 200 }, (_, line) => `key_${line}: nopsai`).join('\n'),
          })),
        }}
        loading={false}
        error={null}
        pushing={false}
        pushResult={null}
        canPush
        onClose={vi.fn()}
        onRefresh={vi.fn(async () => undefined)}
        onPush={vi.fn(async () => undefined)}
      />
    );

    const dialog = screen.getByRole('dialog', { name: 'Platform config' });
    const body = dialog.querySelector('.lg\\:overflow-hidden');
    expect(body).not.toBeNull();

    const fileList = screen.getAllByRole('button', { name: /pipelines\/pipeline-0\.yaml/ })[0].parentElement;
    expect(fileList?.className).toContain('overflow-y-auto');
    expect(fileList?.className).toContain('lg:flex-1');

    for (const label of ['Git highlighted drift', 'Nopsai highlighted drift']) {
      const pane = screen.getByLabelText(label);
      expect(pane.className).toContain('overflow-auto');
      expect(pane.className).toContain('lg:flex-1');
    }
  });
});
