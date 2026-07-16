import type { AssistantConfig, AssistantConversation, AssistantMessage } from './model.js';

export type AssistantProgressStep = {
  label: string;
  state: 'done' | 'active' | 'pending';
};

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
  return assistantProgressSteps(messages, conversation).find(step => step.state === 'active')?.label || 'Preparing a bounded answer...';
}

export function assistantProgressSteps(
  messages: AssistantMessage[],
  conversation: AssistantConversation | null,
  elapsedMs = 0,
): AssistantProgressStep[] {
  const labels = assistantProgressStepLabels(messages, conversation);
  const activeIndex = assistantProgressActiveIndex(elapsedMs, labels.length);
  return labels.map((label, index) => ({
    label,
    state: index < activeIndex ? 'done' : index === activeIndex ? 'active' : 'pending',
  }));
}

export function assistantProgressElapsedLabel(elapsedMs: number): string {
  if (elapsedMs < 1000) return 'just started';
  const seconds = Math.floor(elapsedMs / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${seconds % 60}s`;
}

function assistantProgressStepLabels(messages: AssistantMessage[], conversation: AssistantConversation | null): string[] {
  const memorySummary = conversation?.memory?.summary?.trim();
  const latestUserContent = latestAssistantUserContent(messages);
  const steps = ['Plan the request with current permissions'];

  if (memorySummary) {
    steps.push(memorySummary);
  } else if (/\b(llm|ai|token|tokens|provider|model|profile|cost)\b/.test(latestUserContent)) {
    steps.push('Read AI usage, profile, and cost evidence');
    steps.push('Compare recorded usage with configured profiles');
  } else if (/\b(run|failed|failure|logs?|approval)\b/.test(latestUserContent)) {
    steps.push('Read run metadata and bounded logs');
    steps.push('Identify the first actionable failure signal');
  } else if (/\b(pipeline|yaml|gitops|schedule|trigger)\b/.test(latestUserContent)) {
    steps.push('Load or validate the relevant definition');
    steps.push('Prepare a GitOps-safe answer when changes are involved');
  } else if (/\b(health|status|runner|dispatcher|system)\b/.test(latestUserContent)) {
    steps.push('Check system and runner health evidence');
    steps.push('Separate warnings from blocking issues');
  } else if (/\b(rollout|release|deploy)\b/.test(latestUserContent)) {
    steps.push('Map the rollout context and constraints');
    steps.push('Prepare the next safe operational step');
  } else {
    steps.push('Gather the smallest useful NopsAI evidence set');
  }

  steps.push('Synthesize an evidence-backed answer');
  steps.push('Save and reconcile the chat result');
  return steps;
}

function assistantProgressActiveIndex(elapsedMs: number, stepCount: number): number {
  if (stepCount <= 1) return 0;
  if (elapsedMs < 2500) return 0;
  if (elapsedMs < 10000) return Math.min(1, stepCount - 1);
  if (elapsedMs < 25000) return Math.min(2, stepCount - 1);
  return stepCount - 1;
}

function latestAssistantUserContent(messages: AssistantMessage[]): string {
  return [...messages].reverse().find(message => message.role === 'user')?.content.toLowerCase() || '';
}
