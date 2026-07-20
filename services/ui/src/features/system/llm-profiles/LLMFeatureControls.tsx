import type { Dispatch, SetStateAction } from 'react';
import {
  LLM_FEATURE_MODE_OPTIONS,
  llmFeatureFormValueWithMode,
  llmFeatureModeFromFormValue,
  type LLMFeatureMode,
  type LLMProfileFormState,
} from './model';

type LLMFeatureControlsProps = {
  form: LLMProfileFormState;
  setForm: Dispatch<SetStateAction<LLMProfileFormState>>;
  disabled?: boolean;
};

export function LLMFeatureControls({ form, setForm, disabled }: LLMFeatureControlsProps) {
  const updateMode = (field: 'prompt_cache' | 'provider_state', mode: LLMFeatureMode) => {
    setForm(prev => ({
      ...prev,
      [field]: llmFeatureFormValueWithMode(prev[field], mode),
    }));
  };

  return (
    <fieldset className="space-y-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3">
      <legend className="px-1 text-xs font-semibold uppercase tracking-[0.08em] text-[var(--text-secondary)]">Advanced</legend>
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="flex flex-col gap-1 text-sm">
          <span title="Provider-side prompt cache preference.">Prompt cache</span>
          <select
            className="pipelines-input"
            value={llmFeatureModeFromFormValue(form.prompt_cache)}
            onChange={event => updateMode('prompt_cache', event.target.value as LLMFeatureMode)}
            disabled={disabled}
          >
            {LLM_FEATURE_MODE_OPTIONS.map(option => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span title="Provider-side conversation state preference.">Provider state</span>
          <select
            className="pipelines-input"
            value={llmFeatureModeFromFormValue(form.provider_state)}
            onChange={event => updateMode('provider_state', event.target.value as LLMFeatureMode)}
            disabled={disabled}
          >
            {LLM_FEATURE_MODE_OPTIONS.map(option => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
        </label>
      </div>
    </fieldset>
  );
}
