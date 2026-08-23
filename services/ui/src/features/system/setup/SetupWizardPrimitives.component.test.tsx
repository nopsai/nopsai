import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { WIZARD_STEPS } from './model';
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
  // Derived from the step list for the same reason as the assertion below: the
  // first step is whichever step the wizard puts first.
  const firstStep = WIZARD_STEPS[0];
  expect(
    screen.getByRole('button', { name: new RegExp(`${firstStep.label} ${firstStep.required ? 'Required' : 'Optional'}`, 'i') })
  ).toHaveAttribute('aria-current', 'step');

  await user.click(screen.getByRole('button', { name: /teams optional/i }));

  // Derived from the step list so adding or reordering steps does not make this
  // navigation test assert a stale position.
  expect(onSelectStep).toHaveBeenCalledWith(WIZARD_STEPS.findIndex(step => step.id === 'repositories'));
});
