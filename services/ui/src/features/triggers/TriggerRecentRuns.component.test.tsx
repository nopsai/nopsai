import { createRef } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { TriggerRecentRuns } from './TriggerRecentRuns';

test('renders recent trigger runs and delegates run navigation', async () => {
  const onOpenRun = vi.fn();
  const user = userEvent.setup();
  render(
    <TriggerRecentRuns
      runs={[
        {
          run_id: '12345678-abcd',
          pipeline_name: 'release',
          status: 'success',
          git_ref: 'refs/heads/main',
          started_at: new Date().toISOString(),
          trigger_event_id: '87654321-dcba',
        },
      ]}
      loading={false}
      error={null}
      scrollable
      listRef={createRef<HTMLUListElement>()}
      onScroll={() => undefined}
      onOpenRun={onOpenRun}
    />
  );

  expect(screen.getByText('Success')).toBeVisible();
  expect(screen.getByText('main')).toBeVisible();
  await user.click(screen.getByRole('button', { name: /release/i }));
  expect(onOpenRun).toHaveBeenCalledWith('12345678-abcd');
});
