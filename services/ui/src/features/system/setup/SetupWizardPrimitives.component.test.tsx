import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { SetupStepNavigation, StepIntro, WarningCallout } from './SetupWizardPrimitives';

test('renders setup wizard primitives and delegates step navigation', async () => {
  const user = userEvent.setup();
  const onSelectStep = vi.fn();

  render(
    <>
      <StepIntro title="Prepare setup" icon={<span aria-hidden="true">icon</span>}>
        Follow the setup contract.
      </StepIntro>
      <WarningCallout>Review runtime secrets before saving.</WarningCallout>
      <SetupStepNavigation wizardStepIndex={0} onSelectStep={onSelectStep} />
    </>
  );

  expect(screen.getByText('Prepare setup')).toBeInTheDocument();
  expect(screen.getByText('Review runtime secrets before saving.')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /readiness required/i })).toHaveAttribute('aria-current', 'step');

  await user.click(screen.getByRole('button', { name: /github app optional/i }));

  expect(onSelectStep).toHaveBeenCalledWith(3);
});
