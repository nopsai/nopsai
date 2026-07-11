import type { PipelineDefinition, PipelineRunFinalOutput, RunListItem, StepDetail } from './contracts';
import type { ParentRunInfo } from './runPresentation';

export type PipelineRunsTabKey = 'main' | 'recent' | 'events';

export type PipelineRunDetail = {
  run_info: RunListItem;
  steps: StepDetail[];
  pipeline_definition?: PipelineDefinition;
  pipeline_definition_yaml?: string;
  final_outputs?: PipelineRunFinalOutput[];
  child_runs: RunListItem[];
  parent_run_info?: ParentRunInfo | null;
  approvals?: PipelineApproval[];
};

export type PipelineApproval = {
  id: string;
  run_id: string;
  step_name: string;
  task_name: string;
  approval_type: string;
  assigned_teams: string[];
  allow_self_approval: boolean;
  status: string;
  requested_at: string;
  requested_by_type?: string;
  requested_by_id?: string;
  decided_by_email?: string;
  decided_at?: string;
  decision_comment?: string;
};

export type PipelineRunsTriggerTeam = {
  id: string;
  runs: RunListItem[];
  status: string;
  latestRun?: RunListItem;
};
