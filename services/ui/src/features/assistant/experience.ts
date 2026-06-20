import type { AssistantConfig, AssistantConversation, AssistantMessage } from './model.js';

export const assistantPromptStarters = [
  'Explain a failed run',
  'Improve this pipeline',
  'Check system health',
  'Help me plan a rollout',
];

export function assistantReadyLine(config: AssistantConfig | null): string {
  if (!config) return 'Loading session details...';
  if (!config.enabled) return 'Disabled by administrator configuration.';
  return config.actions.require_confirmation !== false
    ? 'Ready · changes always need your review.'
    : 'Ready · review policy is relaxed for this environment.';
}

export function assistantProgressLabel(messages: AssistantMessage[], conversation: AssistantConversation | null): string {
  const memorySummary = conversation?.memory?.summary?.trim();
  if (memorySummary) return memorySummary;

  const latestUserContent = [...messages].reverse().find(message => message.role === 'user')?.content.toLowerCase() || '';
  if (/\b(run|failed|failure|logs?|approval)\b/.test(latestUserContent)) {
    return 'Looking through your run history...';
  }
  if (/\b(pipeline|yaml|gitops|schedule|trigger)\b/.test(latestUserContent)) {
    return 'Preparing a safe proposal...';
  }
  if (/\b(health|status|runner|dispatcher|system)\b/.test(latestUserContent)) {
    return 'Checking system health...';
  }
  if (/\b(rollout|release|deploy)\b/.test(latestUserContent)) {
    return 'Mapping the rollout path...';
  }
  return 'Preparing a bounded answer...';
}
