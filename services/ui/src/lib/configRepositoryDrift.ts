export type ConfigRepositoryDriftItem = {
  path: string;
  status: string;
  git_content?: string | null;
  desired_content?: string | null;
  delete?: boolean;
};

export type ConfigRepositoryDriftResponse = {
  base_branch: string;
  push_branch: string;
  items: ConfigRepositoryDriftItem[];
  summary?: Record<string, number>;
  can_push: boolean;
  push_message?: string;
};

export type ConfigRepositoryCommitResponse = {
  branch?: string;
  commit_sha?: string;
  commit_url?: string;
  files_changed?: number;
};

export type ConfigRepositoryWriteFile = {
  path: string;
  content?: string;
  delete?: boolean;
};

export type ConfigRepositoryDriftLineKind = 'context' | 'added' | 'removed';

export type ConfigRepositoryDriftLine = {
  number: number;
  text: string;
  kind: ConfigRepositoryDriftLineKind;
};

export type ConfigRepositoryContentDiff = {
  git: ConfigRepositoryDriftLine[];
  desired: ConfigRepositoryDriftLine[];
  changedLines: number;
};

export function changedConfigRepositoryDriftItems(drift: ConfigRepositoryDriftResponse | null | undefined) {
  return (drift?.items ?? []).filter(item => item.status !== 'unchanged');
}

export function buildConfigRepositoryWriteFiles(drift: ConfigRepositoryDriftResponse | null | undefined): ConfigRepositoryWriteFile[] {
  return changedConfigRepositoryDriftItems(drift).map(item => ({
    path: item.path,
    content: item.delete ? undefined : (item.desired_content ?? ''),
    delete: Boolean(item.delete),
  }));
}

export function buildConfigRepositoryContentDiff(item: ConfigRepositoryDriftItem): ConfigRepositoryContentDiff {
  const gitSourceLines = splitDriftContentLines(item.git_content);
  const desiredSourceLines = item.delete ? [] : splitDriftContentLines(item.desired_content);

  if (item.delete) {
    return {
      git: gitSourceLines.map((text, index) => ({ number: index + 1, text, kind: 'removed' })),
      desired: [],
      changedLines: gitSourceLines.length,
    };
  }
  if (item.status === 'added') {
    return {
      git: [],
      desired: desiredSourceLines.map((text, index) => ({ number: index + 1, text, kind: 'added' })),
      changedLines: desiredSourceLines.length,
    };
  }

  const operations = buildLineOperations(gitSourceLines, desiredSourceLines);
  const git: ConfigRepositoryDriftLine[] = [];
  const desired: ConfigRepositoryDriftLine[] = [];
  let gitLineNumber = 1;
  let desiredLineNumber = 1;
  let changedLines = 0;
  operations.forEach(operation => {
    switch (operation.kind) {
      case 'context':
        git.push({ number: gitLineNumber, text: operation.text, kind: 'context' });
        desired.push({ number: desiredLineNumber, text: operation.text, kind: 'context' });
        gitLineNumber += 1;
        desiredLineNumber += 1;
        break;
      case 'removed':
        git.push({ number: gitLineNumber, text: operation.text, kind: 'removed' });
        gitLineNumber += 1;
        changedLines += 1;
        break;
      case 'added':
        desired.push({ number: desiredLineNumber, text: operation.text, kind: 'added' });
        desiredLineNumber += 1;
        changedLines += 1;
        break;
    }
  });
  return { git, desired, changedLines };
}

type LineOperation = {
  kind: ConfigRepositoryDriftLineKind;
  text: string;
};

function splitDriftContentLines(value?: string | null) {
  const normalized = typeof value === 'string' ? value.replace(/\r\n/g, '\n').replace(/\r/g, '\n') : '';
  if (normalized === '') return [];
  const lines = normalized.split('\n');
  if (lines[lines.length - 1] === '') lines.pop();
  return lines;
}

function buildLineOperations(left: string[], right: string[]): LineOperation[] {
  const distances = Array.from({ length: left.length + 1 }, () => Array<number>(right.length + 1).fill(0));
  for (let leftIndex = left.length - 1; leftIndex >= 0; leftIndex -= 1) {
    for (let rightIndex = right.length - 1; rightIndex >= 0; rightIndex -= 1) {
      distances[leftIndex][rightIndex] =
        left[leftIndex] === right[rightIndex]
          ? distances[leftIndex + 1][rightIndex + 1] + 1
          : Math.max(distances[leftIndex + 1][rightIndex], distances[leftIndex][rightIndex + 1]);
    }
  }

  const operations: LineOperation[] = [];
  let leftIndex = 0;
  let rightIndex = 0;
  while (leftIndex < left.length || rightIndex < right.length) {
    if (leftIndex < left.length && rightIndex < right.length && left[leftIndex] === right[rightIndex]) {
      operations.push({ kind: 'context', text: left[leftIndex] });
      leftIndex += 1;
      rightIndex += 1;
    } else if (rightIndex >= right.length || (leftIndex < left.length && distances[leftIndex + 1][rightIndex] >= distances[leftIndex][rightIndex + 1])) {
      operations.push({ kind: 'removed', text: left[leftIndex] });
      leftIndex += 1;
    } else {
      operations.push({ kind: 'added', text: right[rightIndex] });
      rightIndex += 1;
    }
  }
  return operations;
}
