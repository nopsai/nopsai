import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test } from 'vitest';
import { AccessPolicyRuleFields } from './AccessPolicyRuleFields';
import { createEmptyAccessResourceCatalog } from './resourceCatalog';

function PolicyRuleHarness() {
  const [policy, setPolicy] = useState({
    name: 'Pipeline reader',
    obj: 'pipeline:*',
    act: 'pipeline.read',
  });

  return (
    <>
      <AccessPolicyRuleFields
        policy={policy}
        onChange={setPolicy}
        resourceCatalog={createEmptyAccessResourceCatalog()}
      />
      <output data-testid="policy-value">{JSON.stringify(policy)}</output>
    </>
  );
}

test('updates resource and effect through the focused policy rule editor', async () => {
  const user = userEvent.setup();
  render(<PolicyRuleHarness />);

  await user.selectOptions(screen.getByLabelText('Resource type'), 'scope');
  expect(screen.getByTestId('policy-value')).toHaveTextContent(
    '"obj":"scope:*","act":"scope.read"'
  );

  await user.selectOptions(screen.getByLabelText('Effect'), 'deny');
  expect(screen.getByTestId('policy-value')).toHaveTextContent('"act":"deny scope.read"');
});
