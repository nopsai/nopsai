import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  buildKnowledgeID,
  buildKnowledgeConnectionTeamSummaries,
  buildKnowledgeTeamOptions,
  buildKnowledgeTree,
  clearKnowledgeDraft,
  countTeamDocs,
  decodeKnowledgeRouteID,
  deriveIdentityFromTeam,
  documentTeamPath,
  encodeKnowledgeID,
  findKnowledgeTeam,
  isGitManagedDocument,
  knowledgeDocumentTreePath,
  knowledgeContentSource,
  knowledgeTreePathToTeam,
  loadKnowledgeDraft,
  matchesKnowledgeSourceFilter,
  normalizeTeamPath,
  normalizeKnowledgeSourceFilter,
  normalizeKnowledgeWorkspaceTab,
  normalizeKnowledgeSource,
  parentTeam,
  saveKnowledgeDraft,
  sourceLabel,
  splitKnowledgeContentForPreview,
  summarizeKnowledgeWorkspace,
  splitKnowledgePath,
  validateKnowledgeIdentity,
  type KnowledgeContextListItem,
} from './model';

const documents: KnowledgeContextListItem[] = [
  {
    id: 'platform/restart',
    kind: 'runbook',
    team: 'platform',
    name: 'restart',
    visibility: 'team',
    source: 'database',
  },
];

afterEach(() => {
  sessionStorage.clear();
  vi.restoreAllMocks();
});

describe('Knowledge Context model', () => {
  it('builds and navigates knowledge trees with empty enterprise teams', () => {
    const tree = buildKnowledgeTree(documents, ['platform/security', 'analytics']);
    const runbooks = findKnowledgeTeam(tree, 'runbook');

    expect(runbooks.children.map(child => child.fullPath)).toEqual(['runbook/global', 'runbook/analytics', 'runbook/platform']);
    expect(runbooks.children.find(child => child.name === 'platform')?.docs[0]?.name).toBe('restart');
    expect(findKnowledgeTeam(tree, 'runbook/platform/security').docs).toHaveLength(0);
    expect(findKnowledgeTeam(tree, 'missing')).toBe(tree);
    expect(countTeamDocs(tree)).toBe(1);
    expect(knowledgeDocumentTreePath(documents[0])).toBe('runbook/platform');
  });

  it('builds team options from resource teams and existing knowledge resources', () => {
    expect(
      buildKnowledgeTeamOptions({
        activeTeam: 'runbook/platform',
        activeConnectionTeam: 'platform/security',
        resourceTeamPaths: ['platform', 'platform/security'],
        items: [{ team: 'platform/app' }],
        connections: [{ team: 'wiki' }],
      })
    ).toEqual(['platform', 'platform/security']);
    expect(buildKnowledgeTeamOptions({ fallbackTeam: 'starter' })).toEqual(['starter']);
    expect(buildKnowledgeTeamOptions({})).toEqual([]);
  });

  it('normalizes identities, routes, sources, and team paths', () => {
    expect(encodeKnowledgeID('platform/restart service')).toBe('platform/restart%20service');
    expect(buildKnowledgeID(' runbook ', '/platform/', ' restart ')).toBe('platform/restart');
    expect(splitKnowledgePath('platform/restart')).toEqual({ name: 'restart', team: 'platform' });
    expect(splitKnowledgePath('runbook/platform/restart')).toEqual({ name: 'restart', team: 'platform' });
    expect(normalizeTeamPath(' /runbook//platform/ ')).toBe('runbook/platform');
    expect(parentTeam('runbook/platform')).toBe('runbook');
    expect(decodeKnowledgeRouteID('/knowledge-context/platform/restart%20service')).toBe(
      'platform/restart service'
    );
    expect(decodeKnowledgeRouteID('/pipelines')).toBe('');
    expect(deriveIdentityFromTeam('runbook/platform')).toEqual({ kind: 'runbook', team: 'platform' });
    expect(deriveIdentityFromTeam('platform')).toEqual({ kind: 'architecture', team: 'platform' });
    expect(sourceLabel('git-repository')).toBe('GitOps');
    expect(sourceLabel('notion-page')).toBe('Notion');
    expect(normalizeKnowledgeSource('git-repository')).toBe('git');
    expect(normalizeKnowledgeSource('confluence-page')).toBe('confluence');
    expect(isGitManagedDocument({ source: 'database', managed_by_config_repo: true })).toBe(true);
    expect(knowledgeContentSource({ source: 'notion-page' })).toBe('external');
    expect(knowledgeContentSource({ source: 'database' })).toBe('inline');
    expect(knowledgeTreePathToTeam('guardrail/security/platform')).toBe('security/platform');
    expect(documentTeamPath({ id: 'platform/restart', team: '' })).toBe('platform');
    expect(normalizeKnowledgeWorkspaceTab('connections')).toBe('connections');
    expect(normalizeKnowledgeWorkspaceTab('unknown')).toBe('documents');
    expect(normalizeKnowledgeSourceFilter('gitops')).toBe('gitops');
    expect(normalizeKnowledgeSourceFilter('unknown')).toBe('all');
    expect(matchesKnowledgeSourceFilter({ source: 'git-repository', managed_by_config_repo: true }, 'gitops')).toBe(true);
    expect(matchesKnowledgeSourceFilter({ source: 'confluence-page' }, 'external')).toBe(true);
    expect(matchesKnowledgeSourceFilter({ source: 'confluence-page' }, 'confluence')).toBe(true);
    expect(matchesKnowledgeSourceFilter({ source: 'database' }, 'notion')).toBe(false);
  });

  it('summarizes document and connection workspace state', () => {
    const items: KnowledgeContextListItem[] = [
      documents[0],
      {
        id: 'security/repo-check',
        kind: 'guardrail',
        team: 'security',
        name: 'repo-check',
        visibility: 'restricted',
        source: 'git-repository',
        managed_by_config_repo: true,
        used_by_count: 2,
      },
      {
        id: 'security/source-page',
        kind: 'policy',
        team: 'security',
        name: 'source-page',
        visibility: 'team',
        source: 'confluence-page',
        used_by: ['security/review'],
      },
    ];

    expect(summarizeKnowledgeWorkspace(items)).toEqual({
      documents: 3,
      teams: 2,
      kinds: 3,
      inlineDocuments: 2,
      externalDocuments: 1,
      gitOpsManaged: 1,
      referencedDocuments: 2,
      pipelineReferences: 3,
    });
    expect(buildKnowledgeConnectionTeamSummaries(items, ['platform/empty'])).toEqual([
      {
        teamPath: 'platform',
        documentCount: 1,
        inlineDocuments: 1,
        externalDocuments: 0,
        gitOpsManaged: 0,
        referencedDocuments: 0,
        providers: [],
        connections: [],
      },
      {
        teamPath: 'platform/empty',
        documentCount: 0,
        inlineDocuments: 0,
        externalDocuments: 0,
        gitOpsManaged: 0,
        referencedDocuments: 0,
        providers: [],
        connections: [],
      },
      {
        teamPath: 'security',
        documentCount: 2,
        inlineDocuments: 1,
        externalDocuments: 1,
        gitOpsManaged: 1,
        referencedDocuments: 2,
        providers: ['confluence'],
        connections: [],
      },
    ]);
  });

  it('extracts front matter, YAML content, and leading parameters', () => {
    expect(
      splitKnowledgeContentForPreview('---\nname: restart\nkind: runbook\ndescription: Recovery\n---\n# Restart\nSteps')
    ).toEqual({
      content: '# Restart\nSteps',
      parameters: { name: 'restart', kind: 'runbook', description: 'Recovery' },
    });
    expect(splitKnowledgeContentForPreview('name: restart\nkind: runbook\n\nBody')).toEqual({
      content: 'Body',
      parameters: { name: 'restart', kind: 'runbook' },
    });
    expect(splitKnowledgeContentForPreview('name: restart\ncontent: |\n  First\n  Second')).toEqual({
      content: 'First\nSecond',
      parameters: { name: 'restart' },
    });
    expect(splitKnowledgeContentForPreview('Plain markdown')).toEqual({ content: 'Plain markdown', parameters: {} });
  });

  it('persists best-effort drafts and tolerates invalid storage', () => {
    const snapshot = {
      detail: { ...documents[0], content: '# Restart' },
      content: '# Restart',
    };
    saveKnowledgeDraft(snapshot);
    expect(loadKnowledgeDraft(documents[0].id)).toEqual(snapshot);
    clearKnowledgeDraft(documents[0].id);
    expect(loadKnowledgeDraft(documents[0].id)).toBeNull();

    sessionStorage.setItem('nopsai.knowledge-context.draft.invalid', '{');
    expect(loadKnowledgeDraft('invalid')).toBeNull();
  });

  it('validates supported, unique, path-safe document identities', () => {
    expect(validateKnowledgeIdentity('unsupported', 'platform', 'new', documents)).toBe('Choose a supported kind.');
    expect(validateKnowledgeIdentity('runbook', '', 'new', documents)).toBe('Team is required.');
    expect(validateKnowledgeIdentity('runbook', '../platform', 'new', documents)).toMatch(/invalid path segments/);
    expect(validateKnowledgeIdentity('runbook', 'platform', '', documents)).toBe('Document name is required.');
    expect(validateKnowledgeIdentity('runbook', 'platform', 'bad name', documents)).toMatch(/only contain/);
    expect(validateKnowledgeIdentity('runbook', 'platform', 'restart', documents)).toBe(
      'A knowledge context with that identifier already exists.'
    );
    expect(validateKnowledgeIdentity('runbook', 'platform', 'restart', documents, documents[0].id)).toBe('');
  });
});
