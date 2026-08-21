export type AIUsageSummary = {
  /** The single figure shown for AI usage. */
  spend_usd?: number;
  /**
   * LLM calls whose cost could not be determined. Above zero means the spend
   * figure is missing part of what was actually spent.
   */
  unpriced_calls?: number;
};

export type RunListItem = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  pipeline_version?: string;
  pipeline_source?: string;
  status: string;
  created_at?: string;
  git_commit_sha?: string;
  git_commit_url?: string;
  git_commit_message?: string;
  git_commit_author_name?: string;
  git_commit_author_email?: string;
  git_commit_author_username?: string;
  git_repo_name?: string;
  git_repo_owner?: string;
  git_clone_url?: string;
  git_ssh_url?: string;
  git_ref?: string;
  git_target_ref?: string;
  git_pusher_email?: string;
  git_check_run_id?: number;
  git_pusher_name?: string;
  started_at?: string;
  finished_at?: string;
  timeout_at?: string;
  duration?: string;
  is_complete?: boolean;
  parent_run_id?: string | null;
  scope?: string;
  team_id?: number;
  requested_by_type?: string;
  requested_by_id?: string;
  effective_subject_type?: string;
  effective_subject_id?: string;
  runtime_variable_overrides?: Record<string, unknown>;
  trigger_source?: string;
  schedule_id?: string;
  schedule_name?: string;
  schedule_path?: string;
  trigger_event_id?: string;
  external_trigger_id?: string;
  external_trigger_name?: string;
  external_trigger_event_type?: string;
  external_trigger_caller_type?: string;
  external_trigger_caller_id?: string;
  external_trigger_idempotency_key?: string;
  parent_step_name?: string;
  failure_reason?: string;
  ai_usage?: AIUsageSummary;
  final_output_status?: RunFinalOutputStatus;
};

export type TaskDefinition = {
  name: string;
  goal?: string;
  script?: string;
  depends_on?: string[];
  ignore_failure?: boolean;
  model?: string;
  mcp_profiles?: string[];
  variables?: Record<string, string>;
};

export type ApprovalDefinition = {
  type?: string;
  teams?: string[];
  allow_self_approval?: boolean;
  timeout?: string;
};

export type StepConfiguration = {
  include?: string;
  sync?: boolean;
  approval?: ApprovalDefinition;
  image?: string;
  secrets?: string[];
  volumes?: string[];
  variables?: Record<string, string>;
  ignore_failure?: boolean;
  model?: string;
  mcp_profiles?: string[];
  runtime_pool?: string;
  goal?: string;
  script?: string;
  tasks?: TaskDefinition[];
};

export type TaskDetail = {
  task_id: string;
  step_name: string;
  task_name: string;
  status: string;
  exit_code?: number | null;
  started_at?: string;
  finished_at?: string;
  task_index: number;
  ai_usage?: AIUsageSummary;
};

export type StepDetail = {
  name: string;
  status: string;
  depends_on: string[];
  tasks: TaskDetail[];
  duration?: string;
  started_at?: string;
  finished_at?: string;
  configuration?: StepConfiguration;
  ai_usage?: AIUsageSummary;
};

export type PipelineDisplayOption = 'list' | 'graph';

export type PipelineDefinition = {
  name?: string;
  description?: string;
  version?: string;
  display_option?: PipelineDisplayOption;
  model?: string;
  mcp_profiles?: string[];
  runtime_pool?: string;
  affinity_enabled?: boolean;
  output?: {
    model?: string;
    items?: Array<{
      name: string;
      type: 'markdown' | 'pdf' | 'excel' | 'json' | 'html' | string;
      when?: 'always' | 'success' | 'failure' | string;
      prompt: string;
      model?: string;
      dashboard?: PipelineOutputDashboardTarget;
    }>;
  };
  steps?: Array<{
    name: string;
    description?: string;
    depends_on?: string[];
    approval?: ApprovalDefinition;
    tasks?: TaskDefinition[];
    goal?: string;
    script?: string;
    model?: string;
    mcp_profiles?: string[];
    runtime_pool?: string;
  }>;
};

export type PipelineOutputDashboardTarget = {
  ref?: string;
  section?: string;
  entry_key?: string;
  mode?: string;
  preset?: string;
  ttl?: string;
};

export type PipelineRunFinalOutput = {
  id: string;
  name: string;
  type: 'markdown' | 'pdf' | 'excel' | 'json' | 'html' | string;
  status: 'pending' | 'generating' | 'success' | 'failure' | 'cancelled' | string;
  content?: string;
  error?: string;
  model?: string;
  dashboard_target?: PipelineOutputDashboardTarget;
  generation_attempts?: number;
  contract_violations?: number;
  render_attempts?: number;
  render_failures?: number;
  created_at?: string;
  generation_started_at?: string;
  updated_at?: string;
  generation_duration?: string;
  generation_duration_seconds?: number;
};

export type RunFinalOutputStatus = {
  status: string;
  configured: number;
  total: number;
  pending: number;
  generating: number;
  generated: number;
  failed: number;
  cancelled: number;
  updated_at?: string;
};

export type GraphStatus = 'success' | 'warning' | 'failed' | 'running' | 'pending' | 'skipped' | 'cancelled';

export type GraphPoint = { x: number; y: number };
export type GraphSize = { width: number; height: number };
export type GraphLayoutNode<T> = GraphPoint & GraphSize & { data: T; level: number };
export type GraphLayoutEdge = { id: string; from: string; to: string; points: GraphPoint[]; status: GraphStatus };
export type GraphLayout<T> = { nodes: GraphLayoutNode<T>[]; edges: GraphLayoutEdge[]; width: number; height: number };

export type GraphTask = {
  id: string;
  name: string;
  status: GraphStatus;
  duration?: string;
  dependsOn?: string[];
};

export type GraphStep = {
  id: string;
  name: string;
  status: GraphStatus;
  duration?: string;
  dependsOn?: string[];
  tasks: GraphTask[];
  includeLabel?: string;
  childRun?: RunListItem | null;
};

export type TaskGraphLayout = GraphLayout<GraphTask> & {
  orientation: 'horizontal' | 'vertical';
  taskCount: number;
  dependencyCount: number;
};
