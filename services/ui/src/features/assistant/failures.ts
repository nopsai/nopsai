import type { AssistantMessage, AssistantToolActivity } from './model.js';

export type AssistantFailure = {
  title: string;
  detail: string;
};

/** The turn fails or succeeds with the last planner/synthesis call, so that call decides. */
const assistantLLMToolNames = new Set(['nopsai.llm.plan', 'nopsai.llm.complete']);

export function assistantMessageFailure(message: AssistantMessage): AssistantFailure | null {
  if (message.role === 'user') return null;
  const llmCalls = message.tool_calls.filter(tool => assistantLLMToolNames.has(tool.name));
  const lastCall = llmCalls[llmCalls.length - 1];
  if (!lastCall || lastCall.status === 'success') return null;
  const detail = assistantToolFallbackReason(lastCall);
  if (!detail) return null;
  return { title: assistantFailureTitle(detail), detail };
}

export function assistantSendFailure(error: string): AssistantFailure {
  const detail = error.trim();
  return { title: assistantFailureTitle(detail), detail };
}

export function assistantFailureTitle(detail: string): string {
  const text = detail.toLowerCase();
  if (/(429|rate limit|resource_exhausted|quota|spending cap)/.test(text)) return 'Rate limit or quota exceeded';
  if (/(timeout|timed out|deadline exceeded)/.test(text)) return 'Connection timeout';
  if (/(connection refused|no such host|dial tcp|network is unreachable|failed to fetch)/.test(text)) return 'Provider unreachable';
  if (/(401|403|unauthorized|forbidden|api key|credential|permission denied)/.test(text)) return 'Provider authentication failed';
  if (/(404|model not found|unknown model|no such model)/.test(text)) return 'Model not available';
  if (/(500|502|503|504|internal server error)/.test(text)) return 'Provider error';
  if (/(schema|invalid plan|parse|unmarshal|json)/.test(text)) return 'Invalid model response';
  return 'Assistant error';
}

/** Provider payloads are usually JSON on one line, which is unreadable until it is indented. */
export function assistantFailureDetailBody(detail: string): string {
  const start = detail.indexOf('{');
  const end = detail.lastIndexOf('}');
  if (start < 0 || end <= start) return detail.trim();
  try {
    const parsed: unknown = JSON.parse(detail.slice(start, end + 1));
    const prefix = detail.slice(0, start).trim();
    const body = JSON.stringify(parsed, null, 2);
    return prefix ? `${prefix}\n${body}` : body;
  } catch {
    return detail.trim();
  }
}

/** The saved reply inlines the provider reason, which the failure card already shows in full. */
export function assistantMessageProse(content: string, failure: AssistantFailure | null): string {
  const text = content.trim();
  if (!failure?.detail) return text;
  return text.replace(`: ${failure.detail}`, '').trim();
}

function assistantToolFallbackReason(tool: AssistantToolActivity): string {
  const reason = tool.output['fallback_reason'];
  return typeof reason === 'string' ? reason.trim() : '';
}
