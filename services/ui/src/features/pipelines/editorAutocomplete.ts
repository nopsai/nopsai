import { findParentBlock } from '../../lib/lab';
import type { EditorAutocompleteSuggestion } from '../editor/EditorAutocompleteMenu';
import {
  PIPELINE_DIRECTIVES,
  STEP_DIRECTIVES,
  TASK_DIRECTIVES,
  parsePipelineYaml,
  type PipelineDetail,
} from './model';

export type PipelineAutocompleteMetadata = {
  secrets: string[];
  variables: string[];
  llmProfiles: string[];
  mcpProfiles: string[];
  reusableSteps: string[];
  secretScopes: Array<{ scope: string; items: string[] }>;
  variableScopes: Array<{ scope: string; items: string[] }>;
};

export type PipelineEditorSuggestion = EditorAutocompleteSuggestion & {
  replaceStart: number;
  replaceEnd: number;
  appendColon: boolean;
};

type PipelineEditorSuggestionParams = {
  text: string;
  cursor: number;
  force?: boolean;
  metadata: PipelineAutocompleteMetadata;
  detail: PipelineDetail | null;
};

export function buildPipelineEditorSuggestion({
  text,
  cursor,
  force,
  metadata,
  detail,
}: PipelineEditorSuggestionParams): PipelineEditorSuggestion | null {
  const before = text.slice(0, cursor);
  const lineStart = before.lastIndexOf('\n') + 1;
  const lineBeforeCursor = text.slice(lineStart, cursor);
  const prefixMatch = lineBeforeCursor.match(/[A-Za-z0-9_.-]+$/);
  const prefix = prefixMatch ? prefixMatch[0] : '';
  const replaceStart = cursor - prefix.length;
  const replaceEnd = cursor;

  const lines = text.split('\n');
  const lineIndex = before.split('\n').length - 1;
  const currentLine = lines[lineIndex] || '';
  const currentIndent = currentLine.match(/^\s*/)?.[0].length ?? 0;

  const currentKeyMatch = currentLine.match(/^\s*-?\s*([A-Za-z0-9_.-]+)\s*:\s*/);
  const currentKey = currentKeyMatch?.[1] || '';

  const beforeLineText = text.slice(0, lineStart);
  const ancestorKey = findParentBlock(beforeLineText, ['secrets', 'variables', 'depends_on', 'mcp_profiles', 'tasks', 'steps'], currentIndent) || '';
  const containerBlock = findParentBlock(beforeLineText, ['tasks', 'steps'], currentIndent) || '';

  const includeValueContext =
    currentKey === 'include' ||
    /^\s*include\s*:\s*[A-Za-z0-9_.-]*$/.test(lineBeforeCursor.trim());
  const llmProfileValueContext =
    currentKey === 'llm_profile' ||
    /^\s*llm_profile\s*:\s*[A-Za-z0-9_.-]*$/.test(lineBeforeCursor.trim());

  let title = 'Suggestions';
  let pool: string[] = [];
  let appendColon = false;
  let groupedSections: Array<{ label: string; items: string[]; totalCount: number }> | undefined;

  if (includeValueContext) {
    title = 'Reusable steps';
    pool = metadata.reusableSteps;
  } else if (llmProfileValueContext) {
    title = 'LLM profiles';
    pool = metadata.llmProfiles;
  } else if (ancestorKey === 'mcp_profiles') {
    title = 'MCP profiles';
    pool = metadata.mcpProfiles;
  } else if (ancestorKey === 'secrets') {
    title = 'Secrets';
    groupedSections = buildGroupedSections(metadata.secretScopes, metadata.secrets, prefix);
    pool = groupedSections.flatMap(section => section.items);
  } else if (ancestorKey === 'variables') {
    title = 'Variables';
    groupedSections = buildGroupedSections(metadata.variableScopes, metadata.variables, prefix);
    pool = groupedSections.flatMap(section => section.items);
  } else if (ancestorKey === 'depends_on') {
    title = 'Step dependencies';
    pool = resolveStepNames(text, detail);
  } else {
    appendColon = true;
    if (containerBlock === 'tasks') {
      title = 'Task keys';
      pool = TASK_DIRECTIVES;
    } else if (containerBlock === 'steps') {
      title = 'Step keys';
      pool = STEP_DIRECTIVES;
    } else {
      title = 'Pipeline keys';
      pool = PIPELINE_DIRECTIVES;
    }
  }

  const normalizedPrefix = prefix.toLowerCase();
  const filtered = pool
    .filter(item => item.toLowerCase().startsWith(normalizedPrefix))
    .sort((a, b) => a.localeCompare(b));

  const hasContext =
    includeValueContext ||
    llmProfileValueContext ||
    ancestorKey === 'mcp_profiles' ||
    ancestorKey === 'secrets' ||
    ancestorKey === 'variables' ||
    ancestorKey === 'depends_on';
  const isRootLine = !containerBlock && currentIndent === 0 && !currentKey;
  const shouldShow = force || hasContext || filtered.length > 0 || containerBlock === 'tasks' || containerBlock === 'steps';

  if (!shouldShow || (!force && isRootLine && !prefix)) {
    return null;
  }

  return {
    title,
    items: filtered.slice(0, 50),
    activeIndex: 0,
    replaceStart,
    replaceEnd,
    appendColon,
    groupedSections,
  };
}

function buildGroupedSections(
  scopedItems: Array<{ scope: string; items: string[] }>,
  fallbackItems: string[],
  prefix: string
) {
  const base = scopedItems.length ? scopedItems : [{ scope: '', items: fallbackItems }];
  let remaining = 50;
  return base
    .map(entry => {
      const filteredItems = entry.items.filter(item => item.toLowerCase().startsWith(prefix.toLowerCase()));
      if (!filteredItems.length) return null;
      const slice = filteredItems.slice(0, remaining);
      remaining -= slice.length;
      return {
        label: entry.scope ? `/${entry.scope}` : 'Default scope',
        items: slice,
        totalCount: filteredItems.length,
      };
    })
    .filter(Boolean) as Array<{ label: string; items: string[]; totalCount: number }>;
}

function resolveStepNames(text: string, detail: PipelineDetail | null) {
  if (!detail) return [];
  try {
    return parsePipelineYaml(text, detail.id, detail.source).stepNames;
  } catch {
    return [];
  }
}
