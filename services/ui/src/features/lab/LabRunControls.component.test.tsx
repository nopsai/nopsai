import { MemoryRouter } from 'react-router-dom';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { LabRunControls } from './LabRunControls';

test('coordinates pipeline, scope, and run actions', async () => {
  const onPipelineChange = vi.fn();
  const onScopeChange = vi.fn();
  const onRun = vi.fn();
  const user = userEvent.setup();
  render(
    <MemoryRouter>
      <LabRunControls
        pipelines={[{ id: 'platform/release' }]}
        pipelinesLoading={false}
        yamlLoading={false}
        selectedPipelineId=""
        scopeOptions={['production']}
        scopeValue=""
        runPending={false}
        validationErrorCount={0}
        accessLoading={false}
        accessError={null}
        accessBlocked={false}
        accessChecks={[
          {
            allowed: true,
            action: 'pipeline.use',
            resource_type: 'pipeline',
            resource_id: 'platform/release',
          },
        ]}
        feedback={{ tone: 'success', message: 'Run started!', runId: 'run-1' }}
        onPipelineChange={onPipelineChange}
        onScopeChange={onScopeChange}
        onRun={onRun}
      />
    </MemoryRouter>
  );

  await user.selectOptions(screen.getByLabelText('Pipeline selection'), 'platform/release');
  await user.selectOptions(screen.getByLabelText('Target scope selection'), 'production');
  await user.click(screen.getByRole('button', { name: 'Run' }));

  expect(onPipelineChange).toHaveBeenCalledWith('platform/release');
  expect(onScopeChange).toHaveBeenCalledWith('production');
  expect(onRun).toHaveBeenCalledOnce();
  expect(screen.getByRole('link', { name: 'View' })).toHaveAttribute('href', '/pipelineruns/main');
});
