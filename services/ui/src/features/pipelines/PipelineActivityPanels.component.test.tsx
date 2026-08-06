import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { PipelineActivityPanels } from './PipelineActivityPanels';
import { parsePipelineDependencyReference } from './model';

test('routes trigger, dependency, copy, and run actions through callbacks', async () => {
  const onOpenTrigger = vi.fn();
  const onOpenDependency = vi.fn();
  const onCopyDependency = vi.fn();
  const onOpenRun = vi.fn();
  const user = userEvent.setup();

  render(
    <PipelineActivityPanels
      pipelineLabel="release"
      triggers={[{ repoSlug: 'acme/api', source: 'git', trigger: { on: 'push', branches: ['main'] } }]}
      triggersLoading={false}
      triggersError={null}
      dependencies={[
        parsePipelineDependencyReference('pipeline:platform/deploy'),
        parsePipelineDependencyReference('step:build-image'),
      ]}
      runs={[{
        run_id: 'run-123456789',
        pipeline_name: 'release',
        status: 'success',
        git_ref: 'refs/heads/main',
        final_output_status: {
          status: 'success',
          configured: 1,
          total: 1,
          pending: 0,
          generating: 0,
          generated: 1,
          failed: 0,
          cancelled: 0,
        },
      }]}
      runsLoading={false}
      runsError={null}
      onOpenTrigger={onOpenTrigger}
      onOpenDependency={onOpenDependency}
      onCopyDependency={onCopyDependency}
      onOpenRun={onOpenRun}
    />
  );

  await user.click(screen.getByTitle('Open trigger acme/api'));
  await user.click(screen.getByTitle('Open platform/deploy'));
  await user.click(screen.getByTitle('Open build-image'));
  await user.click(screen.getByTitle('Open run run-123456789'));

  expect(onOpenTrigger).toHaveBeenCalledWith('acme/api');
  expect(onOpenDependency).toHaveBeenCalledWith(expect.objectContaining({ kind: 'pipeline', identifier: 'platform/deploy' }));
  expect(onOpenDependency).toHaveBeenCalledWith(expect.objectContaining({ kind: 'step', identifier: 'build-image' }));
  expect(onCopyDependency).not.toHaveBeenCalled();
  expect(onOpenRun).toHaveBeenCalledWith('run-123456789');
  expect(screen.getByText('Output generated')).toHaveClass('runner-pill--ok');
});
