import type { AssistantConfig, AssistantConversation } from './model.js';

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

// What the running turn is doing, as reported by the turn itself.
//
// These labels used to be guessed in the browser from keywords in the question
// and advanced by a timer, so the panel claimed to be reading logs whether or
// not anything read logs. The server publishes the real step as it happens; the
// browser only renders it, and says nothing when it has nothing to report.
export function assistantProgressLabel(conversation: AssistantConversation | null): string {
  return conversation?.turn_progress?.trim() || 'Working on it';
}

export function assistantProgressElapsedLabel(elapsedMs: number): string {
  if (elapsedMs < 1000) return 'just started';
  const seconds = Math.floor(elapsedMs / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${seconds % 60}s`;
}


