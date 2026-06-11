import { describe, expect, it } from 'vitest';
import {
  buildInlineSuggestionPreview,
  normalizeAgentProfileSuggestionList,
  normalizeLLMProfileSuggestionList,
  normalizeLabScopeLabel,
  normalizeLabSuggestionList,
  normalizeMCPProfileSuggestionList,
  normalizeVariableSuggestionList,
} from './suggestions';

describe('Lab suggestions', () => {
  it('normalizes scopes and suggestion lists', () => {
    expect(normalizeLabScopeLabel(' default ')).toBe('');
    expect(normalizeLabScopeLabel('/team/dev/')).toBe('team/dev');
    expect(normalizeLabScopeLabel(null)).toBe('');
    expect(normalizeLabSuggestionList([' TOKEN ', { name: 'REGION' }, { id: 'ignored' }, null])).toEqual([
      'TOKEN',
      'REGION',
    ]);
    expect(normalizeLabSuggestionList({ items: [] })).toEqual([]);
    expect(normalizeVariableSuggestionList(['owner/repo/TOKEN', 'TOKEN', 'owner/repo/REGION'])).toEqual([
      'REGION',
      'TOKEN',
    ]);
  });

  it('filters unavailable LLM and MCP profiles', () => {
    expect(
      normalizeAgentProfileSuggestionList({
        profiles: [' direct ', { id: 'sre' }, { id: 'disabled', enabled: false }, null],
      })
    ).toEqual(['direct', 'sre']);
    expect(normalizeAgentProfileSuggestionList(null)).toEqual([]);
    expect(
      normalizeLLMProfileSuggestionList({
        profiles: [' direct ', { name: 'standard' }, { name: 'blocked', allowed_in_scope: false }, null],
      })
    ).toEqual(['direct', 'standard']);
    expect(normalizeLLMProfileSuggestionList([])).toEqual([]);
    expect(
      normalizeMCPProfileSuggestionList({
        profiles: [' direct ', { name: 'github' }, { name: 'disabled', enabled: false }, null],
      })
    ).toEqual(['direct', 'github']);
    expect(normalizeMCPProfileSuggestionList(null)).toEqual([]);
  });

  it('builds previews only for usable matching suggestions', () => {
    const context = {
      type: 'pipeline-key' as const,
      prefix: 'pipe',
      title: 'Pipeline',
      rangeStart: 0,
      rangeEnd: 4,
    };
    expect(buildInlineSuggestionPreview({ label: 'pipeline', value: 'pipelines/release.yaml' }, context)).toBe(
      'lines/release.yaml'
    );
    expect(buildInlineSuggestionPreview({ label: 'pipeline', snippet: 'PIPELINES/release.yaml\nnext' }, context)).toBe(
      'LINES/release.yaml'
    );
    expect(buildInlineSuggestionPreview({ label: 'pipeline', value: 'steps/build.yaml' }, context)).toBe('');
    expect(buildInlineSuggestionPreview({ label: 'pipeline', value: '' }, context)).toBe('');
    expect(buildInlineSuggestionPreview({ label: 'pipeline', value: 'pipelines/release.yaml' }, { ...context, prefix: '' })).toBe(
      'pipelines/release.yaml'
    );
  });
});
