import type { ReactNode } from 'react';
import type { StoredSession } from '../lib/api.js';

export type Theme = 'light' | 'dark';

export type NavItem = {
  label: string;
  path: string;
  icon: ReactNode;
};

export type PipelineTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: PipelineTreeNode[];
  pipelineIds: string[];
};

export type StepTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: StepTreeNode[];
  stepIds: string[];
};

export type ResourceCapabilities = {
  write?: boolean;
  delete?: boolean;
};

export type ReadCapabilities = {
  read?: boolean;
  write?: boolean;
  delete?: boolean;
};

export type SystemCapabilities = {
  configRead?: boolean;
  configWrite?: boolean;
  llmProfilesRead?: boolean;
  llmProfilesWrite?: boolean;
  agentProfilesRead?: boolean;
  agentProfilesWrite?: boolean;
  mcpRead?: boolean;
  mcpWrite?: boolean;
  credentialsRead?: boolean;
  credentialsWrite?: boolean;
  configReposRead?: boolean;
  configReposWrite?: boolean;
  dispatcherRead?: boolean;
  dispatcherWrite?: boolean;
  logsRead?: boolean;
  access?: boolean;
};

export type SetupStatusSummary = {
  completed?: boolean;
};

export type CurrentUser = {
  sub: string;
  email?: string;
  displayName?: string;
  roles?: string[];
  capabilities?: {
    pipelines?: ResourceCapabilities;
    schedules?: ReadCapabilities;
    dashboards?: ReadCapabilities;
    steps?: ResourceCapabilities;
    triggers?: ReadCapabilities;
    external_triggers?: ReadCapabilities;
    git_webhook_sources?: ReadCapabilities;
    scopes?: ReadCapabilities;
    knowledge_contexts?: ReadCapabilities;
    knowledge_connections?: ReadCapabilities;
    system?: SystemCapabilities;
  };
};

export type AuthSession = StoredSession;

export type RunTeam = {
  id: number;
  name: string;
  kind?: 'team' | 'app' | string;
  parent_id?: number | null;
  path?: string;
  team_path?: string;
  description?: string;
  repo_url?: string;
  repository_full_name?: string;
};

export type RunListItem = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  pipeline_version?: string;
  pipeline_source?: string;
  status: string;
  git_commit_sha?: string;
  git_repo_name?: string;
  git_repo_owner?: string;
  git_ref?: string;
  git_target_ref?: string;
  git_pusher_name?: string;
  started_at?: string;
  finished_at?: string;
  duration?: string;
  is_complete?: boolean;
  parent_run_id?: string | null;
  trigger_event_id?: string;
};

export type RunDetail = {
  run_info?: RunListItem;
};

export type RunTabKey = 'main' | 'recent' | 'events';
