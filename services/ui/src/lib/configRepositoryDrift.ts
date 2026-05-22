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
