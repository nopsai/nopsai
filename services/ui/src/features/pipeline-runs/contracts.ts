export type AIUsageSummary = {
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
  total_cost_usd?: number;
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
};

export type TaskDefinition = {
  name: string;
  goal?: string;
  script?: string;
  depends_on?: string[];
  ignore_failure?: boolean;
  llm_profile?: string;
  mcp_profiles?: string[];
  llm_output_sharing?: boolean;
  variables?: Record<string, string>;
};

export type ApprovalDefinition = {
  type?: string;
  teams?: string[];
  allow_self_approval?: boolean;
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
  llm_profile?: string;
  mcp_profiles?: string[];
  runtime_pool?: string;
  llm_output_sharing?: boolean;
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

export type PipelineDefinition = {
  name?: string;
  description?: string;
  version?: string;
  llm_profile?: string;
  mcp_profiles?: string[];
  runtime_pool?: string;
  affinity_enabled?: boolean;
  output?: {
    llm_profile?: string;
    items?: Array<{
      name: string;
      type: 'markdown' | 'pdf' | 'excel' | 'json' | 'html' | string;
      when?: 'always' | 'success' | 'failure' | string;
      prompt: string;
      llm_profile?: string;
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
    llm_profile?: string;
    mcp_profiles?: string[];
    runtime_pool?: string;
    llm_output_sharing?: boolean;
  }>;
};

export type PipelineRunFinalOutput = {
  id: string;
  name: string;
  type: 'markdown' | 'pdf' | 'excel' | 'json' | 'html' | string;
  status: 'pending' | 'generating' | 'success' | 'failure' | string;
  content?: string;
  error?: string;
  llm_profile?: string;
  generation_attempts?: number;
  contract_violations?: number;
  render_attempts?: number;
  render_failures?: number;
  created_at?: string;
  updated_at?: string;
};

export type GraphStatus = 'success' | 'failed' | 'running' | 'pending' | 'skipped' | 'cancelled';

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
