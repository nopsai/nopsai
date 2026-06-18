import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test } from 'vitest';
import { useState } from 'react';
import { RuntimePoolsEditor } from './RuntimePoolsEditor';
import type { RuntimePoolsConfig } from './model';

function RuntimePoolsHarness({ initial }: { initial: RuntimePoolsConfig }) {
  const [pools, setPools] = useState(initial);
  return (
    <>
      <RuntimePoolsEditor value={pools} metadata={{ scope: 'runtime_live', label: 'Runtime pools', section: 'Runtime', apply: 'Applied live' }} disabled={false} onChange={setPools} />
      <pre data-testid="runtime-pools-state">{JSON.stringify(pools)}</pre>
    </>
  );
}

test('edits runtime pools with node selectors and resource maps', async () => {
  const user = userEvent.setup();
  render(<RuntimePoolsHarness initial={{}} />);

  expect(screen.getByText('No runtime pools configured. Kubernetes runners use their default scheduling.')).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Add runtime pool' }));
  expect(screen.getByLabelText('Runtime pool name default')).toHaveValue('default');

  await user.click(screen.getByRole('button', { name: 'Add node selector to default' }));
  await user.type(screen.getByLabelText('default Node selector value 1'), 'nopsai');

  await user.click(screen.getByRole('button', { name: 'Add resource requests to default' }));
  await user.type(screen.getByLabelText('default Resource requests value 1'), '4Gi');

  await user.click(screen.getByRole('button', { name: 'Add resource limits to default' }));
  await user.type(screen.getByLabelText('default Resource limits value 1'), '16Gi');

  expect(screen.getByTestId('runtime-pools-state')).toHaveTextContent(
    JSON.stringify({
      default: {
        node_selector: { 'node-class': 'nopsai' },
        resources: {
          requests: { memory: '4Gi' },
          limits: { memory: '16Gi' },
        },
      },
    })
  );
});

test('adds, renames, and removes runtime pools', async () => {
  const user = userEvent.setup();
  render(
    <RuntimePoolsHarness
      initial={{
        default: {
          node_selector: { workload: 'nopsai' },
          resources: { requests: {}, limits: {} },
        },
      }}
    />
  );

  await user.click(screen.getByRole('button', { name: 'Add runtime pool' }));
  await user.click(screen.getByLabelText('Runtime pool name pool-1'));
  await user.keyboard('{Control>}a{/Control}high-memory{Enter}');
  await user.click(screen.getAllByRole('button', { name: 'Remove pool' })[0]);

  expect(screen.getByTestId('runtime-pools-state')).toHaveTextContent(
    JSON.stringify({
      'high-memory': {
        node_selector: {},
        resources: {
          requests: {},
          limits: {},
        },
      },
    })
  );
});
