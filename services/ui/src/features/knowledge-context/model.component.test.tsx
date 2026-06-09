import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  buildKnowledgeID,
  buildKnowledgeTree,
  clearKnowledgeDraft,
  countFolderDocs,
  decodeKnowledgeRouteID,
  deriveIdentityFromFolder,
  encodeKnowledgeID,
  findKnowledgeFolder,
  isGitManagedDocument,
  loadKnowledgeDraft,
  normalizeFolderPath,
  normalizeKnowledgeSource,
  parentFolder,
  saveKnowledgeDraft,
  sourceLabel,
  splitKnowledgeContentForPreview,
  splitKnowledgePath,
  validateKnowledgeIdentity,
  type KnowledgeContextListItem,
} from './model';

const documents: KnowledgeContextListItem[] = [
  {
    id: 'runbook/platform/restart',
    kind: 'runbook',
    group: 'platform',
    name: 'restart',
    visibility: 'group',
    source: 'database',
  },
];

afterEach(() => {
  sessionStorage.clear();
  vi.restoreAllMocks();
});

describe('Knowledge Context model', () => {
  it('builds and navigates knowledge trees with empty enterprise groups', () => {
    const tree = buildKnowledgeTree(documents, ['platform/security']);
    const runbooks = findKnowledgeFolder(tree, 'runbook');

    expect(runbooks.children.find(child => child.name === 'platform')?.docs[0]?.name).toBe('restart');
    expect(findKnowledgeFolder(tree, 'runbook/platform/security').docs).toHaveLength(0);
    expect(findKnowledgeFolder(tree, 'missing')).toBe(tree);
    expect(countFolderDocs(tree)).toBe(1);
  });

  it('normalizes identities, routes, sources, and folder paths', () => {
    expect(encodeKnowledgeID('runbook/platform/restart service')).toBe('runbook/platform/restart%20service');
    expect(buildKnowledgeID(' runbook ', '/platform/', ' restart ')).toBe('runbook/platform/restart');
    expect(splitKnowledgePath('runbook/platform/restart')).toEqual({ name: 'restart', folder: 'runbook/platform' });
    expect(normalizeFolderPath(' /runbook//platform/ ')).toBe('runbook/platform');
    expect(parentFolder('runbook/platform')).toBe('runbook');
    expect(decodeKnowledgeRouteID('/knowledge-context/runbook/platform/restart%20service')).toBe(
      'runbook/platform/restart service'
    );
    expect(decodeKnowledgeRouteID('/pipelines')).toBe('');
    expect(deriveIdentityFromFolder('runbook/platform')).toEqual({ kind: 'runbook', group: 'platform' });
    expect(deriveIdentityFromFolder('platform')).toEqual({ kind: 'architecture', group: 'platform' });
    expect(sourceLabel('git-repository')).toBe('GitOps');
    expect(normalizeKnowledgeSource('git-repository')).toBe('git');
    expect(isGitManagedDocument({ source: 'database', managed_by_config_repo: true })).toBe(true);
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
    expect(validateKnowledgeIdentity('runbook', '', 'new', documents)).toBe('Group is required.');
    expect(validateKnowledgeIdentity('runbook', '../platform', 'new', documents)).toMatch(/invalid path segments/);
    expect(validateKnowledgeIdentity('runbook', 'platform', '', documents)).toBe('Document name is required.');
    expect(validateKnowledgeIdentity('runbook', 'platform', 'bad name', documents)).toMatch(/only contain/);
    expect(validateKnowledgeIdentity('runbook', 'platform', 'restart', documents)).toBe(
      'A knowledge context with that identifier already exists.'
    );
    expect(validateKnowledgeIdentity('runbook', 'platform', 'restart', documents, documents[0].id)).toBe('');
  });
});
