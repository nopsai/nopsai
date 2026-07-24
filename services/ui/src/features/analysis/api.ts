import {
  assistantReadableErrorText,
  fetchAssistantConfig,
  fetchAssistantLLMProfiles,
} from '../assistant/api.js';
import type { AssistantConfig, AssistantLLMProfile } from '../assistant/model.js';
import { asRecord, normalizeNumber, readString } from '../system/data.js';
import { apiClient } from '../../lib/api.js';
import type { AnalysisResult } from './model.js';
import {
  analysisAssistantPageContext,
  buildAnalysisAiPrompt,
  parseAnalysisAiEvaluation,
  type AnalysisAiPromptContext,
  type StructuredAnalysisAiEvaluation,
} from './ai.js';

export type AnalysisAiEvaluation = {
  evaluation: StructuredAnalysisAiEvaluation;
  generatedAt: string;
  modelLabel: string;
  profileName: string;
  usage: {
    totalTokens: number;
    durationMs: number;
  };
};

export async function requestAnalysisAiEvaluation(
  result: AnalysisResult,
  context?: AnalysisAiPromptContext | null
): Promise<AnalysisAiEvaluation> {
  const config = await fetchAssistantConfig();
  assertAnalysisAiAvailable(config, result);
  const pageContext = analysisAssistantPageContext(result);
  const profilePayload = await fetchAssistantLLMProfiles();
  const selectedProfile = selectAnalysisLLMProfile(profilePayload.default_profile, profilePayload.profiles);

  const payload = await postAnalysisEvaluation({
    subject_type: result.subjectType,
    subject_id: result.subjectId,
    subject_label: result.subjectLabel,
    scope: pageContext.scope || '',
    selected_llm_profile: selectedProfile.name,
    prompt: buildAnalysisAiPrompt(result, context),
  });

  return {
    evaluation: parseAnalysisAiEvaluation(payload.content),
    generatedAt: payload.generatedAt,
    modelLabel: payload.model || selectedProfile.model || payload.provider || selectedProfile.provider || 'AI Evaluation',
    profileName: payload.profileName || selectedProfile.name,
    usage: {
      totalTokens: payload.usage.totalTokens,
      durationMs: payload.usage.durationMs,
    },
  };
}

type AnalysisEvaluationRequest = {
  subject_type: AnalysisResult['subjectType'];
  subject_id: string;
  subject_label: string;
  scope: string;
  selected_llm_profile: string;
  prompt: string;
};

type AnalysisEvaluationPayload = {
  content: string;
  profileName: string;
  provider: string;
  model: string;
  generatedAt: string;
  usage: {
    totalTokens: number;
    durationMs: number;
  };
};

async function postAnalysisEvaluation(input: AnalysisEvaluationRequest): Promise<AnalysisEvaluationPayload> {
  const response = await apiClient.fetch('/v1/analysis/evaluate', {
    method: 'POST',
    cache: 'no-store',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(assistantReadableErrorText(text, `Failed to generate AI evaluation (${response.status})`));
  }
  return normalizeAnalysisEvaluationPayload(await response.json());
}

function normalizeAnalysisEvaluationPayload(value: unknown): AnalysisEvaluationPayload {
  const record = asRecord(value) || {};
  const usage = asRecord(record.usage) || {};
  const generatedAt = readString(record.generated_at) || new Date().toISOString();
  return {
    content: readString(record.content),
    profileName: readString(record.profile_name),
    provider: readString(record.provider),
    model: readString(record.model),
    generatedAt,
    usage: {
      totalTokens: normalizeNumber(usage.total_tokens),
      durationMs: normalizeNumber(usage.duration_ms),
    },
  };
}

function assertAnalysisAiAvailable(config: AssistantConfig, result: AnalysisResult) {
  if (!config.enabled) {
    throw new Error('Assistant AI is disabled by administrator configuration. Deterministic analysis is still available.');
  }
  if (result.subjectType === 'run' && !config.features.pipeline_debugging) {
    throw new Error('Assistant run debugging is disabled. Deterministic run analysis is still available.');
  }
  if (result.subjectType !== 'run' && !config.features.maintenance_recommendations) {
    throw new Error('Assistant maintenance recommendations are disabled. Deterministic analysis is still available.');
  }
}

function selectAnalysisLLMProfile(defaultProfile: string, profiles: AssistantLLMProfile[]): AssistantLLMProfile {
  const selectable = profiles.filter(profile =>
    profile.status === 'valid' && profile.allowed_in_scope && profile.name.trim()
  );
  const preferred = selectable.find(profile => profile.name === defaultProfile) || selectable[0];
  if (preferred) return preferred;
  const reasons = profiles
    .slice(0, 3)
    .map(profile => profile.disabled_reason || profile.validation || `${profile.name || 'profile'} is not selectable`)
    .filter(Boolean);
  const suffix = reasons.length > 0 ? ` Current profile issue: ${reasons.join('; ')}.` : '';
  throw new Error(`No usable LLM profile is available for AI Evaluation. Define or fix a profile in LLM Profiles with provider, model, credential reference when required, and scope access.${suffix}`);
}
