import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { LabVariableOverrides } from './LabVariableOverrides';

function OverridesHarness({
  onAdd,
  onUpdate,
  onRemove,
}: {
  onAdd: () => void;
  onUpdate: (id: number, field: 'key' | 'value', value: string) => void;
  onRemove: (id: number) => void;
}) {
  const [overrides, setOverrides] = useState([{ id: 7, key: 'REGION', value: 'eu' }]);
  return (
    <LabVariableOverrides
      overrides={overrides}
      onAdd={onAdd}
      onUpdate={(id, field, value) => {
        setOverrides(current => current.map(row => (row.id === id ? { ...row, [field]: value } : row)));
        onUpdate(id, field, value);
      }}
      onRemove={id => {
        setOverrides(current => current.filter(row => row.id !== id));
        onRemove(id);
      }}
    />
  );
}

test('renders accessible override fields and delegates add, edit, and remove actions', async () => {
  const user = userEvent.setup();
  const onAdd = vi.fn();
  const onUpdate = vi.fn();
  const onRemove = vi.fn();

  render(<OverridesHarness onAdd={onAdd} onUpdate={onUpdate} onRemove={onRemove} />);

  await user.click(screen.getByRole('button', { name: 'Add variable override' }));
  await user.type(screen.getByLabelText('Override key'), '_CODE');
  await user.type(screen.getByLabelText('Override value'), '-west');
  await user.click(screen.getByRole('button', { name: 'Remove override' }));

  expect(onAdd).toHaveBeenCalledOnce();
  expect(onUpdate).toHaveBeenCalledWith(7, 'key', 'REGION_CODE');
  expect(onUpdate).toHaveBeenCalledWith(7, 'value', 'eu-west');
  expect(onRemove).toHaveBeenCalledWith(7);
});
