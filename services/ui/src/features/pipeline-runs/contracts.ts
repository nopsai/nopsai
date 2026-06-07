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
