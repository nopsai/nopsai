import type { EditorAutocompleteSuggestion } from '../editor/EditorAutocompleteMenu';
import {
  TRIGGER_EVENT_OPTIONS,
  TRIGGER_KEYS,
  TRIGGER_ROOT_KEYS,
} from './model';

export type TriggerAutocompleteMetadata = {
  pipelines: string[];
  scopes: string[];
};

export type TriggerEditorSuggestion = EditorAutocompleteSuggestion & {
  replaceStart: number;
  replaceEnd: number;
  appendColon: boolean;
};

type TriggerEditorSuggestionParams = {
  text: string;
  cursor: number;
  force?: boolean;
  metadata: TriggerAutocompleteMetadata;
};

export function buildTriggerEditorSuggestion({
  text,
  cursor,
  force,
  metadata,
}: TriggerEditorSuggestionParams): TriggerEditorSuggestion | null {
  const before = text.slice(0, cursor);
  const lineStart = before.lastIndexOf('\n') + 1;
  const lineBeforeCursor = text.slice(lineStart, cursor);
  const prefixMatch = lineBeforeCursor.match(/[A-Za-z0-9_./-]+$/);
  const prefix = prefixMatch ? prefixMatch[0] : '';
  const replaceStart = cursor - prefix.length;
  const replaceEnd = cursor;

  const lines = text.split('\n');
  const lineIndex = before.split('\n').length - 1;
  const currentLine = lines[lineIndex] || '';
  const currentIndent = currentLine.match(/^\s*/)?.[0].length ?? 0;
  const currentKeyMatch = currentLine.match(/^\s*-?\s*([A-Za-z0-9_.-]+)\s*:\s*/);
  const currentKey = currentKeyMatch?.[1] || '';
  const parentKey = findParentKey(lines, lineIndex, currentIndent);

  const trimmedLineBefore = lineBeforeCursor.trim();
  const isKeyContext = !trimmedLineBefore.includes(':') && /^-?\s*[A-Za-z0-9_.-]*$/.test(trimmedLineBefore.replace(/^-/, '').trim());
  const pipelineValueContext = parentKey === 'pipelines';
  const scopeValueContext =
    currentKey === 'scope' || /^\s*scope\s*:\s*[A-Za-z0-9_./-]*$/.test(trimmedLineBefore);
  const onValueContext = currentKey === 'on' || /^\s*on\s*:\s*[A-Za-z0-9_.-]*$/.test(trimmedLineBefore);

  let title = 'Suggestions';
  let pool: string[] = [];
  let appendColon = false;

  if (pipelineValueContext) {
    title = 'Pipelines';
    pool = metadata.pipelines;
  } else if (scopeValueContext) {
    title = 'Scopes';
    pool = metadata.scopes;
  } else if (onValueContext) {
    title = 'Events';
    pool = TRIGGER_EVENT_OPTIONS;
  } else if (isKeyContext) {
    appendColon = true;
    title = currentIndent === 0 ? 'Root keys' : 'Trigger keys';
    pool = currentIndent === 0 ? TRIGGER_ROOT_KEYS : TRIGGER_KEYS;
  }

  const normalizedPrefix = prefix.toLowerCase();
  const filtered = pool
    .filter(item => item.toLowerCase().startsWith(normalizedPrefix))
    .sort((a, b) => a.localeCompare(b));

  if (
    !force &&
    !lineBeforeCursor.trim() &&
    !prefix &&
    !pipelineValueContext &&
    !scopeValueContext &&
    !onValueContext
  ) {
    return null;
  }

  const hasContext = pipelineValueContext || scopeValueContext || onValueContext || isKeyContext;
  if (!force && !hasContext && !filtered.length) {
    return null;
  }

  return {
    title,
    items: filtered.slice(0, 50),
    activeIndex: 0,
    replaceStart,
    replaceEnd,
    appendColon,
  };
}

function findParentKey(lines: string[], lineIndex: number, currentIndent: number) {
  for (let i = lineIndex; i >= 0; i -= 1) {
    const rawLine = lines[i];
    const trimmed = rawLine.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const indent = rawLine.match(/^\s*/)?.[0].length ?? 0;
    if (indent < currentIndent) {
      const match = rawLine.match(/^\s*-?\s*([A-Za-z0-9_.-]+)\s*:\s*/);
      if (match) return match[1];
    }
  }
  return '';
}
