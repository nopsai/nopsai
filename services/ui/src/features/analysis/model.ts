import * as yaml from 'js-yaml';

export type AnalysisSubjectType = 'team' | 'resource' | 'pipeline' | 'run';

export type AnalysisCategory =
  | 'security'
  | 'reliability'
  | 'organization'
  | 'efficiency'
  | 'monitoring'
  | 'maintainability'
  | 'bug'
  | 'cost';

export type AnalysisSeverity = 'critical' | 'high' | 'medium' | 'low' | 'opportunity';

export type AnalysisEvidence = {
  label: string;
  value: string;
  kind?: 'fact' | 'metric' | 'inference' | 'redacted';
};

export type ResourceReference = {
  type: string;
  id: string;
  label: string;
  href?: string;
};

export type Recommendation = {
  title: string;
  detail: string;
  suggestedYamlChange?: string;
};

export type AnalysisFinding = {
  id: string;
  subjectType: AnalysisSubjectType;
  subjectId: string;
  category: AnalysisCategory;
  severity: AnalysisSeverity;
  title: string;
  summary: string;
  evidence: AnalysisEvidence[];
  affectedResources: ResourceReference[];
  recommendations: Recommendation[];
  confidence: number;
  generatedAt: string;
  snapshotRevision: string;
};

export type AnalysisScore = {
  category: AnalysisCategory;
  label: string;
  score: number;
  findingCount: number;
  deduction: number;
  basis: string;
};

export type AnalysisScoreBasis = {
  baseline: number;
  formula: string;
  severityWeights: Record<AnalysisSeverity, number>;
  findingCount: number;
  totalDeduction: number;
  severityCounts: Record<AnalysisSeverity, number>;
  inputs: string[];
  limitations: string[];
};

export type AnalysisTab = {
  id: 'overview' | AnalysisCategory | 'performance';
  label: string;
};

export type AnalysisComparisonItem = {
  label: string;
  before: string;
  after: string;
  changed: boolean;
};

export type AnalysisResult = {
  title: string;
  subjectType: AnalysisSubjectType;
  subjectId: string;
  subjectLabel: string;
  scopePath?: string;
  generatedAt: string;
  snapshotRevision: string;
  summary: string;
  healthScore: number;
  scoreBasis: AnalysisScoreBasis;
  scores: AnalysisScore[];
  findings: AnalysisFinding[];
  counts: Record<AnalysisSeverity, number>;
  tabs: AnalysisTab[];
  safeguards: string[];
  comparison?: AnalysisComparisonItem[];
  primaryDiagnosis?: {
    domain: RunRootCauseDomain;
    confidence: number;
  };
};

export type AnalyzableResource = {
  id: string;
  kind: string;
  label: string;
  description: string;
  href?: string;
  teamPath?: string;
  source?: string;
};

export type TeamResourceAnalysisOptions = {
  subjectId: string;
  subjectLabel: string;
  scopePath: string;
  resources: AnalyzableResource[];
  activeResource?: AnalyzableResource | null;
  now?: Date;
};

export type PipelineAnalysisScope =
  | 'complete'
  | 'security'
  | 'reliability'
  | 'monitoring'
  | 'performance'
  | 'maintainability'
  | 'pre-execution';

export type PipelineAnalysisRun = {
  run_id: string;
  pipeline_name?: string;
  pipeline_path?: string;
  status?: string;
  duration?: string;
  started_at?: string;
  final_output_status?: {
    status?: string;
    failed?: number;
    generated?: number;
    total?: number;
  };
};

export type PipelineAnalysisStep = {
  name: string;
  status?: string;
  depends_on?: string[];
  configuration?: {
    image?: string;
    secrets?: string[];
    variables?: Record<string, string>;
    ignore_failure?: boolean;
    approval?: {
      type?: string;
      teams?: string[];
      allow_self_approval?: boolean;
    };
    runtime_pool?: string;
    goal?: string;
    script?: string;
    tasks?: Array<{
      name: string;
      goal?: string;
      script?: string;
      depends_on?: string[];
      ignore_failure?: boolean;
      variables?: Record<string, string>;
    }>;
  };
};

export type PipelineAnalysisInput = {
  detail: {
    id: string;
    name: string;
    description?: string;
    version?: string;
    path?: string;
    rawYaml: string;
    stepNames?: string[];
    includedDependencies?: string[];
    dependencyEdges?: Array<{ from: string; to: string }>;
    containerImage?: string;
    source?: string;
  };
  graphData: {
    steps: PipelineAnalysisStep[];
    error: string | null;
  };
  triggers: Array<{ repoSlug: string; source?: string; trigger?: Record<string, unknown> }>;
  recentRuns: PipelineAnalysisRun[];
  scope: PipelineAnalysisScope;
  includeRunHistory: boolean;
  now?: Date;
};

export type RunRootCauseDomain =
  | 'Pipeline definition'
  | 'Application code'
  | 'Application tests'
  | 'Configuration'
  | 'Credential or authorization'
  | 'Runner infrastructure'
  | 'External dependency'
  | 'Trigger or input'
  | 'Approval or policy'
  | 'AI provider/model'
  | 'Timeout or capacity'
  | 'Unknown';

export type RunAnalysisInput = {
  detail: {
    run_info: RunAnalysisRunInfo;
    steps: RunAnalysisStep[];
    pipeline_definition?: {
      name?: string;
      description?: string;
      version?: string;
      steps?: Array<{
        name: string;
        description?: string;
        depends_on?: string[];
        approval?: unknown;
        tasks?: unknown[];
        goal?: string;
        script?: string;
      }>;
    };
    pipeline_definition_yaml?: string;
    child_runs?: RunAnalysisRunInfo[];
    approvals?: Array<{ status?: string; step_name?: string; task_name?: string; approval_type?: string }>;
    final_outputs?: Array<{ name: string; status?: string; error?: string; generation_attempts?: number }>;
  };
  comparisonRuns: RunAnalysisRunInfo[];
  now?: Date;
};

export type RunAnalysisRunInfo = {
  run_id: string;
  pipeline_name: string;
  pipeline_path?: string;
  pipeline_version?: string;
  pipeline_source?: string;
  status: string;
  created_at?: string;
  started_at?: string;
  finished_at?: string;
  duration?: string;
  is_complete?: boolean;
  git_commit_sha?: string;
  git_ref?: string;
  git_target_ref?: string;
  scope?: string;
  trigger_source?: string;
  trigger_event_id?: string;
  runtime_variable_overrides?: Record<string, unknown>;
  failure_reason?: string;
  ai_usage?: {
    spend_usd?: number;
    unpriced_calls?: number;
  };
};

export type RunAnalysisStep = {
  name: string;
  status: string;
  depends_on?: string[];
  duration?: string;
  started_at?: string;
  finished_at?: string;
  configuration?: {
    ignore_failure?: boolean;
    approval?: unknown;
    runtime_pool?: string;
    script?: string;
    goal?: string;
    tasks?: Array<{ name: string; script?: string; goal?: string; ignore_failure?: boolean }>;
  };
  tasks: Array<{
    task_id: string;
    step_name: string;
    task_name: string;
    status: string;
    exit_code?: number | null;
    started_at?: string;
    finished_at?: string;
    task_index: number;
    ai_usage?: {
      spend_usd?: number;
      unpriced_calls?: number;
    };
  }>;
  ai_usage?: {
    spend_usd?: number;
    unpriced_calls?: number;
  };
};

type FindingDraft = Omit<AnalysisFinding, 'id' | 'subjectType' | 'subjectId' | 'generatedAt' | 'snapshotRevision'>;

const DEFAULT_SAFEGUARDS = [
  'Read-only analysis. No edits, reruns, deletes, or permission changes are performed.',
  'Credential and secret values are never displayed; findings use metadata and redacted evidence only.',
  'Evidence and inference are separated, and every finding carries a confidence score.',
];

const CATEGORY_LABELS: Record<AnalysisCategory, string> = {
  security: 'Security',
  reliability: 'Reliability',
  organization: 'Organization',
  efficiency: 'Efficiency',
  monitoring: 'Observability',
  maintainability: 'Maintainability',
  bug: 'Bug',
  cost: 'Cost',
};

const SEVERITIES: AnalysisSeverity[] = ['critical', 'high', 'medium', 'low', 'opportunity'];
const SCORE_BASELINE = 100;
const SEVERITY_SCORE_WEIGHTS: Record<AnalysisSeverity, number> = {
  critical: 25,
  high: 15,
  medium: 8,
  low: 3,
  opportunity: 1,
};

export const TEAM_RESOURCE_ANALYSIS_TABS: AnalysisTab[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'security', label: 'Security' },
  { id: 'organization', label: 'Organization' },
  { id: 'efficiency', label: 'Efficiency' },
];

export const PIPELINE_ANALYSIS_TABS: AnalysisTab[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'security', label: 'Security' },
  { id: 'reliability', label: 'Reliability' },
  { id: 'monitoring', label: 'Monitoring' },
  { id: 'performance', label: 'Performance' },
  { id: 'maintainability', label: 'Maintainability' },
];

export const RUN_ANALYSIS_TABS: AnalysisTab[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'bug', label: 'Diagnosis' },
  { id: 'reliability', label: 'Reliability' },
  { id: 'monitoring', label: 'Monitoring' },
  { id: 'cost', label: 'Cost' },
];

export function buildPipelineAnalysis(input: PipelineAnalysisInput): AnalysisResult {
  const generatedAt = (input.now || new Date()).toISOString();
  const snapshotRevision = stableSnapshotRevision({
    kind: 'pipeline',
    scope: input.scope,
    includeRunHistory: input.includeRunHistory,
    detail: {
      id: input.detail.id,
      rawYaml: input.detail.rawYaml,
      source: input.detail.source,
    },
    graphData: input.graphData,
    triggers: input.triggers,
    recentRuns: input.includeRunHistory ? input.recentRuns.slice(0, 30) : [],
  });
  const allDrafts = buildPipelineFindingDrafts(input);
  const findings = filterPipelineFindings(allDrafts, input.scope);
  const preExecution = input.scope === 'pre-execution';
  const blocking = findings.filter(finding => finding.severity === 'critical' || finding.severity === 'high').length;

  return createAnalysisResult({
    title: preExecution ? 'Pre-execution Analysis' : 'Pipeline Analysis',
    subjectType: 'pipeline',
    subjectId: input.detail.id,
    subjectLabel: input.detail.name || input.detail.id,
    generatedAt,
    snapshotRevision,
    summary: preExecution
      ? `Ready to execute: ${blocking === 0 ? 'Yes' : 'No'}`
      : `Reviewed definition${input.includeRunHistory ? ` and ${Math.min(input.recentRuns.length, 30)} recent runs` : ''}.`,
    tabs: PIPELINE_ANALYSIS_TABS,
    findings,
    scoreCategories: ['security', 'reliability', 'monitoring', 'maintainability', 'efficiency'],
    scoreInputs: [
      'saved pipeline YAML snapshot',
      'parsed step graph and dependency edges',
      'trigger metadata visible on the pipeline detail page',
      input.includeRunHistory ? 'last 30 visible runs for failure and duration signals' : 'run history excluded by reviewer option',
      'static pre-execution readiness rules',
    ],
  });
}

export function buildRunAnalysis(input: RunAnalysisInput): AnalysisResult {
  const generatedAt = (input.now || new Date()).toISOString();
  const run = input.detail.run_info;
  const snapshotRevision = stableSnapshotRevision({
    kind: 'run',
    run: redactRunForSnapshot(run),
    steps: input.detail.steps,
    approvals: input.detail.approvals,
    outputs: input.detail.final_outputs,
    comparisonRuns: input.comparisonRuns.map(redactRunForSnapshot),
  });
  const diagnosis = classifyRun(input.detail);
  const findings = buildRunFindingDrafts(input, diagnosis);
  const lastSuccess = findLastSuccessfulPeerRun(run, input.comparisonRuns);

  return createAnalysisResult({
    title: 'Run Analysis',
    subjectType: 'run',
    subjectId: run.run_id,
    subjectLabel: `${run.pipeline_name} / ${shortID(run.run_id)}`,
    generatedAt,
    snapshotRevision,
    summary: isFailureStatus(run.status, run.is_complete)
      ? `Primary diagnosis: ${diagnosis.domain}`
      : 'Run succeeded; analysis focused on degradation, hidden warnings, and cost signals.',
    tabs: RUN_ANALYSIS_TABS,
    findings,
    scoreCategories: ['bug', 'reliability', 'monitoring', 'cost'],
    scoreInputs: [
      'run status, timing, trigger, commit, branch, scope, and failure metadata',
      'step and task statuses with exit codes when available',
      'approval, child-run, and final-output state',
      'last visible successful peer run for comparison when available',
      'redacted failure text and metadata only; raw secret values and private logs are not included in the score',
    ],
    comparison: lastSuccess ? buildRunComparison(run, lastSuccess) : undefined,
    primaryDiagnosis: diagnosis,
  });
}

export function formatAnalysisReport(result: AnalysisResult): string {
  const lines = [
    result.title,
    `Subject: ${result.subjectLabel}`,
    `Overall health: ${result.healthScore}/100`,
    `Score basis: ${result.scoreBasis.formula}`,
    `Score inputs: ${result.scoreBasis.inputs.join('; ')}`,
    `Generated: ${result.generatedAt}`,
    `Snapshot: ${result.snapshotRevision}`,
    '',
    result.summary,
    '',
    'Findings:',
  ];

  if (result.findings.length === 0) {
    lines.push('- No findings in the visible snapshot.');
  } else {
    result.findings.forEach((finding, index) => {
      lines.push(`${index + 1}. [${finding.severity.toUpperCase()}] ${finding.title}`);
      lines.push(`   Category: ${CATEGORY_LABELS[finding.category]}`);
      lines.push(`   Confidence: ${finding.confidence}%`);
      lines.push(`   Summary: ${finding.summary}`);
      finding.evidence.forEach(evidence => {
        lines.push(`   Evidence - ${evidence.label}: ${evidence.value}`);
      });
      finding.recommendations.forEach(recommendation => {
        lines.push(`   Recommendation - ${recommendation.title}: ${recommendation.detail}`);
      });
    });
  }

  if (result.comparison?.length) {
    lines.push('', 'Changed since last success:');
    result.comparison.forEach(item => {
      lines.push(`- ${item.label}: ${item.before} -> ${item.after}${item.changed ? '' : ' (unchanged)'}`);
    });
  }

  lines.push('', 'Safeguards:');
  result.safeguards.forEach(safeguard => lines.push(`- ${safeguard}`));
  return lines.join('\n');
}

export function filterFindingsForTab(findings: AnalysisFinding[], tabId: AnalysisTab['id']): AnalysisFinding[] {
  if (tabId === 'overview') return findings;
  if (tabId === 'performance') {
    return findings.filter(finding => finding.category === 'cost' || finding.category === 'efficiency');
  }
  return findings.filter(finding => finding.category === tabId);
}

export function analysisCategoryLabel(category: AnalysisCategory): string {
  return CATEGORY_LABELS[category];
}

function buildPipelineFindingDrafts(input: PipelineAnalysisInput): FindingDraft[] {
  const findings: FindingDraft[] = [];
  const rawYaml = input.detail.rawYaml || '';
  const parsed = parsePipelineYaml(rawYaml);
  const steps = input.graphData.steps || [];
  const parsedRecord = parsed.ok ? asRecord(parsed.value) : null;

  if (!parsed.ok) {
    findings.push({
      category: 'maintainability',
      severity: 'critical',
      title: 'Pipeline YAML does not parse',
      summary: 'The analyser could not parse the pipeline definition snapshot.',
      evidence: [{ label: 'Parser error', value: parsed.error, kind: 'fact' }],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Fix YAML before execution',
        detail: 'Resolve the syntax error and re-run validation before executing or saving the pipeline.',
      }],
      confidence: 96,
    });
    return findings;
  }

  if (input.graphData.error) {
    findings.push({
      category: 'maintainability',
      severity: 'high',
      title: 'Dependency graph could not be rendered',
      summary: 'The pipeline definition parsed but the dependency graph builder reported an error.',
      evidence: [{ label: 'Graph error', value: input.graphData.error, kind: 'fact' }],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Repair dependency metadata',
        detail: 'Check `steps`, `depends_on`, reusable includes, and duplicate step names.',
      }],
      confidence: 90,
    });
  }

  if (!String(input.detail.description || parsedRecord?.description || '').trim()) {
    findings.push({
      category: 'maintainability',
      severity: 'medium',
      title: 'Pipeline has no operator description',
      summary: 'Operators cannot quickly tell what the pipeline does, which application it affects, or when it should be used.',
      evidence: [{ label: 'Description', value: 'Missing', kind: 'fact' }],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Add generated description',
        detail: 'Document application, trigger, target environments, required inputs, produced outputs, rollback path, and ownership.',
        suggestedYamlChange: [
          'description: |',
          `  Builds, verifies, and operates ${input.detail.name || input.detail.id}.`,
          '  Document owner, trigger conditions, required inputs, outputs, and rollback procedure.',
        ].join('\n'),
      }],
      confidence: 88,
    });
  }

  if (steps.length === 0) {
    findings.push({
      category: 'reliability',
      severity: 'critical',
      title: 'Pipeline has no executable steps',
      summary: 'No steps were found in the parsed definition or graph snapshot.',
      evidence: [{ label: 'Step count', value: '0', kind: 'metric' }],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Define executable workflow steps',
        detail: 'Add at least one script, goal, reusable step include, child pipeline include, or approval step.',
      }],
      confidence: 94,
    });
  }

  const secretLines = detectSecretLiterals(rawYaml);
  if (secretLines.length > 0) {
    findings.push({
      category: 'security',
      severity: 'critical',
      title: 'Secret-like values are embedded in YAML',
      summary: `${secretLines.length} line${secretLines.length === 1 ? '' : 's'} contain token, secret, password, or key-like literals.`,
      evidence: secretLines.slice(0, 5).map(line => ({
        label: `Line ${line.line}`,
        value: line.redacted,
        kind: 'redacted' as const,
      })),
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Move secrets to credential or scope references',
        detail: 'Store sensitive values in the credential broker or scoped secrets and pass only references into the pipeline.',
      }],
      confidence: 92,
    });
  }

  const unpinnedImages = collectPipelineImages(input, parsedRecord).filter(image => isUnpinnedImage(image.value));
  if (unpinnedImages.length > 0) {
    findings.push({
      category: 'security',
      severity: 'high',
      title: 'Container images are not pinned',
      summary: 'Mutable or untagged container images reduce reproducibility and increase supply-chain risk.',
      evidence: unpinnedImages.slice(0, 6).map(image => ({
        label: image.label,
        value: image.value,
        kind: 'fact' as const,
      })),
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Pin approved images',
        detail: 'Use immutable digests for production paths and keep base-image upgrades explicit in GitOps review.',
        suggestedYamlChange: 'container_image: registry.example.com/team/runner@sha256:<approved-digest>',
      }],
      confidence: 86,
    });
  }

  if (/\bprivileged\s*:\s*true\b/i.test(rawYaml)) {
    findings.push({
      category: 'security',
      severity: 'high',
      title: 'Privileged container execution requested',
      summary: 'Privileged execution expands runner and host access and should be exceptional.',
      evidence: [{ label: 'Privilege flag', value: 'privileged: true', kind: 'fact' }],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Remove or isolate privileged execution',
        detail: 'Use a hardened runner pool and approval policy if privileged execution is required.',
      }],
      confidence: 90,
    });
  }

  const riskyScripts = collectScripts(steps).filter(script => riskyShellScript(script.script));
  if (riskyScripts.length > 0) {
    findings.push({
      category: 'security',
      severity: 'high',
      title: 'Shell script uses unsafe external input or installer pattern',
      summary: 'One or more scripts combine shell execution with external input, remote installers, or broad command interpolation.',
      evidence: riskyScripts.slice(0, 5).map(script => ({
        label: script.location,
        value: redactInline(script.script),
        kind: 'redacted' as const,
      })),
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Use structured inputs',
        detail: 'Validate enumerated inputs and pass arguments through structured command APIs instead of interpolating untrusted shell strings.',
      }],
      confidence: 76,
    });
  }

  if (!hasTimeout(parsedRecord, rawYaml)) {
    findings.push({
      category: 'reliability',
      severity: 'medium',
      title: 'Pipeline timeout is missing',
      summary: 'A run can remain active indefinitely when no timeout boundary is visible.',
      evidence: [{ label: 'Timeout', value: 'Missing', kind: 'fact' }],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Add timeout boundary',
        detail: 'Set a pipeline-level timeout and tighter step-level timeouts for external calls.',
        suggestedYamlChange: 'timeout: 30m',
      }],
      confidence: 88,
    });
  }

  if (mentionsProduction(rawYaml) && !steps.some(step => Boolean(step.configuration?.approval))) {
    findings.push({
      category: 'security',
      severity: 'high',
      title: 'Production path has no visible approval step',
      summary: 'The definition references production but no approval gate is visible in the graph snapshot.',
      evidence: [
        { label: 'Production signal', value: 'prod/production appears in definition', kind: 'inference' },
        { label: 'Approval steps', value: '0', kind: 'metric' },
      ],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Add production approval',
        detail: 'Require an approval gate before production deployment and avoid self-approval for sensitive paths.',
        suggestedYamlChange: ['approval:', '  type: production-deploy', '  teams:', '    - platform/prod', '  allow_self_approval: false'].join('\n'),
      }],
      confidence: 80,
    });
  }

  const dependencyIssues = detectDependencyIssues(steps);
  dependencyIssues.forEach(issue => findings.push(issue));

  if (!parsedRecord?.output && !/artifact|junit|report|summary|metrics|correlation/i.test(rawYaml)) {
    findings.push({
      category: 'monitoring',
      severity: 'medium',
      title: 'Pipeline has limited operational output',
      summary: 'No final outputs, report artifacts, metrics, correlation IDs, or structured summaries are visible in the definition.',
      evidence: [{ label: 'Output metadata', value: 'No output/report/metrics indicators found', kind: 'inference' }],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Publish operator-ready outputs',
        detail: 'Emit test summaries, deployment identifiers, failure classification, and runbook links for critical steps.',
      }],
      confidence: 70,
    });
  }

  const sequentialOpportunity = detectSequentialOpportunity(steps);
  if (sequentialOpportunity) findings.push(sequentialOpportunity);

  if (input.triggers.length === 0) {
    findings.push({
      category: 'maintainability',
      severity: 'low',
      title: 'No trigger metadata is attached',
      summary: 'The detail view has no repository, schedule, or external trigger linked to this pipeline snapshot.',
      evidence: [{ label: 'Triggers', value: '0', kind: 'metric' }],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Document invocation model',
        detail: 'Add trigger metadata or make the manual execution contract explicit so operators know how the pipeline starts.',
      }],
      confidence: 66,
    });
  }

  if (input.includeRunHistory) {
    findings.push(...buildPipelineHistoryFindings(input));
  }

  if (input.scope === 'pre-execution') {
    findings.push(...buildPreExecutionFindings(input, parsedRecord));
  }

  return findings;
}

function buildPipelineHistoryFindings(input: PipelineAnalysisInput): FindingDraft[] {
  const findings: FindingDraft[] = [];
  const runs = input.recentRuns.slice(0, 30);
  if (runs.length === 0) return findings;

  const completeRuns = runs.filter(run => run.status);
  const failures = completeRuns.filter(run => isFailureStatus(run.status));
  const failureRate = completeRuns.length ? failures.length / completeRuns.length : 0;
  if (completeRuns.length >= 5 && failureRate >= 0.25) {
    findings.push({
      category: 'reliability',
      severity: failureRate >= 0.5 ? 'high' : 'medium',
      title: 'Recent run history is failure-heavy',
      summary: `${failures.length} of the last ${completeRuns.length} visible runs failed or were cancelled.`,
      evidence: [
        { label: 'Failure rate', value: `${Math.round(failureRate * 100)}%`, kind: 'metric' },
        { label: 'Run sample', value: `${completeRuns.length} runs`, kind: 'metric' },
      ],
      affectedResources: [pipelineReference(input.detail), ...failures.slice(0, 5).map(runReference)],
      recommendations: [{
        title: 'Stabilize before expanding reuse',
        detail: 'Review the first failed step in recent runs and separate flaky infrastructure failures from deterministic application failures.',
      }],
      confidence: 82,
    });
  }

  const durations = runs.map(run => parseDurationSeconds(run.duration)).filter(isFiniteNumber);
  const median = medianNumber(durations);
  const slowRuns = runs.filter(run => {
    const seconds = parseDurationSeconds(run.duration);
    return median > 0 && seconds > median * 1.8;
  });
  if (median > 0 && slowRuns.length > 0) {
    findings.push({
      category: 'cost',
      severity: 'opportunity',
      title: 'Recent runs show duration outliers',
      summary: `${slowRuns.length} run${slowRuns.length === 1 ? '' : 's'} exceeded 1.8x the median duration.`,
      evidence: [
        { label: 'Median duration', value: formatSeconds(median), kind: 'metric' },
        { label: 'Outlier count', value: String(slowRuns.length), kind: 'metric' },
      ],
      affectedResources: [pipelineReference(input.detail), ...slowRuns.slice(0, 4).map(runReference)],
      recommendations: [{
        title: 'Investigate slow steps and cache opportunities',
        detail: 'Compare outliers against dependency-install, artifact, AI usage, queue-time, and external-service timing signals.',
      }],
      confidence: 68,
    });
  }

  const failedOutputs = runs.filter(run => (run.final_output_status?.failed || 0) > 0);
  if (failedOutputs.length > 0) {
    findings.push({
      category: 'monitoring',
      severity: 'medium',
      title: 'Final output generation failed recently',
      summary: `${failedOutputs.length} recent run${failedOutputs.length === 1 ? '' : 's'} reported failed final outputs.`,
      evidence: [{ label: 'Failed output runs', value: String(failedOutputs.length), kind: 'metric' }],
      affectedResources: [pipelineReference(input.detail), ...failedOutputs.slice(0, 5).map(runReference)],
      recommendations: [{
        title: 'Check report contracts',
        detail: 'Review output prompts, renderer limits, and data availability so generated reports remain reliable.',
      }],
      confidence: 78,
    });
  }

  return findings;
}

function buildPreExecutionFindings(input: PipelineAnalysisInput, parsedRecord: Record<string, unknown> | null): FindingDraft[] {
  const findings: FindingDraft[] = [];
  const rawYaml = input.detail.rawYaml || '';
  const credentialRefs = collectCredentialReferences(rawYaml);
  const runnerPools = collectRunnerPools(input.graphData.steps, parsedRecord);

  if (credentialRefs.length === 0 && /deploy|release|registry|aws|gcp|azure|prod/i.test(rawYaml)) {
    findings.push({
      category: 'security',
      severity: 'high',
      title: 'Deployment-like pipeline has no declared credential reference',
      summary: 'The definition looks like a deployment or release workflow but no credential reference was detected.',
      evidence: [
        { label: 'Credential references', value: '0', kind: 'metric' },
        { label: 'Detection', value: 'Metadata and YAML names only', kind: 'inference' },
      ],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Declare credentials explicitly',
        detail: 'Use credential references or scoped secrets instead of relying on ambient runner credentials.',
      }],
      confidence: 64,
    });
  }

  if (runnerPools.length === 0) {
    findings.push({
      category: 'reliability',
      severity: 'medium',
      title: 'Runner pool is implicit',
      summary: 'No `runtime_pool` was found at pipeline or step level, so capacity and isolation are harder to pre-check.',
      evidence: [{ label: 'Runtime pools', value: 'Implicit default', kind: 'fact' }],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Set runtime pool for critical workloads',
        detail: 'Declare the runner pool expected for production, GPU, privileged, or large workloads.',
      }],
      confidence: 70,
    });
  }

  if (mentionsProduction(rawYaml) && !/allow_self_approval\s*:\s*false/i.test(rawYaml)) {
    findings.push({
      category: 'security',
      severity: 'medium',
      title: 'Production approval self-approval boundary is unclear',
      summary: 'Production appears in the definition but no explicit `allow_self_approval: false` setting was found.',
      evidence: [{ label: 'Approval boundary', value: 'Not explicit', kind: 'fact' }],
      affectedResources: [pipelineReference(input.detail)],
      recommendations: [{
        title: 'Make approval policy explicit',
        detail: 'Set `allow_self_approval: false` on production gates and assign an accountable approver team.',
      }],
      confidence: 72,
    });
  }

  return findings;
}

function buildRunFindingDrafts(
  input: RunAnalysisInput,
  diagnosis: { domain: RunRootCauseDomain; confidence: number; evidence: AnalysisEvidence[] }
): FindingDraft[] {
  const findings: FindingDraft[] = [];
  const run = input.detail.run_info;
  const firstFailure = findFirstFailedExecution(input.detail.steps);
  const lastSuccess = findLastSuccessfulPeerRun(run, input.comparisonRuns);
  const runFailed = isFailureStatus(run.status, run.is_complete);

  if (runFailed) {
    findings.push({
      category: 'bug',
      severity: diagnosis.domain === 'Unknown' ? 'medium' : 'high',
      title: 'Pipeline versus application diagnosis',
      summary: `Likely domain: ${diagnosis.domain}.`,
      evidence: diagnosis.evidence,
      affectedResources: [runReference(run), ...(firstFailure ? [stepReference(firstFailure.step.name)] : [])],
      recommendations: [{
        title: diagnosisActionTitle(diagnosis.domain),
        detail: diagnosisActionDetail(diagnosis.domain, firstFailure),
      }],
      confidence: diagnosis.confidence,
    });
  }

  if (run.failure_reason) {
    findings.push({
      category: /credential|auth|permission|forbidden/i.test(run.failure_reason) ? 'security' : 'reliability',
      severity: 'high',
      title: 'Run recorded a failure reason',
      summary: 'The run has a top-level failure reason that may explain a pre-step or orchestration failure.',
      evidence: [{ label: 'Failure reason', value: redactInline(run.failure_reason), kind: 'redacted' }],
      affectedResources: [runReference(run)],
      recommendations: [{
        title: 'Resolve the first orchestration error',
        detail: 'Check pipeline definition, credentials, runner capacity, and trigger inputs before rerunning.',
      }],
      confidence: 86,
    });
  }

  if (firstFailure) {
    findings.push({
      category: 'bug',
      severity: firstFailure.task ? 'high' : 'medium',
      title: 'First failed execution point detected',
      summary: firstFailure.task
        ? `Task ${firstFailure.task.task_name} failed in step ${firstFailure.step.name}.`
        : `Step ${firstFailure.step.name} failed before a specific failed task was recorded.`,
      evidence: [
        { label: 'Step', value: firstFailure.step.name, kind: 'fact' },
        ...(firstFailure.task ? [
          { label: 'Task', value: firstFailure.task.task_name, kind: 'fact' as const },
          { label: 'Exit code', value: firstFailure.task.exit_code == null ? '-' : String(firstFailure.task.exit_code), kind: 'fact' as const },
        ] : []),
      ],
      affectedResources: [runReference(run), stepReference(firstFailure.step.name)],
      recommendations: [{
        title: 'Inspect first failure before downstream errors',
        detail: 'Open the step logs at the first failed task and ignore later failures until this root point is explained.',
      }],
      confidence: 88,
    });
  }

  if (lastSuccess) {
    const comparison = buildRunComparison(run, lastSuccess);
    const changed = comparison.filter(item => item.changed);
    if (changed.length > 0 && runFailed) {
      findings.push({
        category: 'bug',
        severity: 'medium',
        title: 'Run differs from last successful peer',
        summary: `${changed.length} tracked field${changed.length === 1 ? '' : 's'} changed since the last visible successful run.`,
        evidence: changed.map(item => ({
          label: item.label,
          value: `${item.before} -> ${item.after}`,
          kind: 'fact' as const,
        })),
        affectedResources: [runReference(run), runReference(lastSuccess)],
        recommendations: [{
          title: 'Start from the first changed input',
          detail: 'Compare application commit, pipeline revision, scope, trigger source, and runtime inputs before changing the pipeline definition.',
        }],
        confidence: 74,
      });
    }
  }

  if (!runFailed) {
    findings.push(...buildSuccessfulRunFindings(input, lastSuccess));
  }

  const pendingApprovals = (input.detail.approvals || []).filter(approval => normalizeStatus(approval.status) === 'pending');
  if (pendingApprovals.length > 0) {
    findings.push({
      category: 'reliability',
      severity: 'medium',
      title: 'Approval can block execution',
      summary: `${pendingApprovals.length} approval${pendingApprovals.length === 1 ? '' : 's'} are pending in this run snapshot.`,
      evidence: pendingApprovals.slice(0, 4).map(approval => ({
        label: approval.step_name || 'Approval',
        value: approval.approval_type || 'pending',
        kind: 'fact' as const,
      })),
      affectedResources: [runReference(run)],
      recommendations: [{
        title: 'Check approval ownership',
        detail: 'Confirm assigned teams, escalation, timeout expectations, and self-approval boundaries.',
      }],
      confidence: 82,
    });
  }

  const outputFailures = (input.detail.final_outputs || []).filter(output => isFailureStatus(output.status));
  if (outputFailures.length > 0) {
    findings.push({
      category: 'monitoring',
      severity: 'medium',
      title: 'Final output generation failed',
      summary: `${outputFailures.length} generated output${outputFailures.length === 1 ? '' : 's'} failed after the run context was collected.`,
      evidence: outputFailures.slice(0, 4).map(output => ({
        label: output.name,
        value: output.error ? redactInline(output.error) : output.status || 'failure',
        kind: output.error ? 'redacted' as const : 'fact' as const,
      })),
      affectedResources: [runReference(run)],
      recommendations: [{
        title: 'Repair output prompt or renderer contract',
        detail: 'Review output generation attempts, contract violations, and renderer errors separately from pipeline execution.',
      }],
      confidence: 80,
    });
  }

  const childFailures = (input.detail.child_runs || []).filter(child => isFailureStatus(child.status, child.is_complete));
  if (childFailures.length > 0) {
    findings.push({
      category: 'reliability',
      severity: 'high',
      title: 'Child pipeline failure affects this run',
      summary: `${childFailures.length} child run${childFailures.length === 1 ? '' : 's'} failed or were cancelled.`,
      evidence: childFailures.slice(0, 5).map(child => ({
        label: child.pipeline_name,
        value: `${child.status} / ${shortID(child.run_id)}`,
        kind: 'fact' as const,
      })),
      affectedResources: [runReference(run), ...childFailures.slice(0, 5).map(runReference)],
      recommendations: [{
        title: 'Analyze the failed child run first',
        detail: 'A parent may only be surfacing failure propagation; inspect the child run first failure point before changing the parent.',
      }],
      confidence: 84,
    });
  }

  if (findings.length === 0) {
    findings.push({
      category: 'monitoring',
      severity: 'low',
      title: 'No degradation signal detected',
      summary: 'The visible run metadata does not show failures, pending approvals, output errors, or duration outliers.',
      evidence: [{ label: 'Run status', value: run.status || 'unknown', kind: 'fact' }],
      affectedResources: [runReference(run)],
      recommendations: [{
        title: 'Keep comparison data available',
        detail: 'Retain recent successful runs so future failure analysis can separate pipeline, application, infrastructure, and input changes.',
      }],
      confidence: 60,
    });
  }

  return findings;
}

function buildSuccessfulRunFindings(input: RunAnalysisInput, lastSuccess: RunAnalysisRunInfo | null): FindingDraft[] {
  const findings: FindingDraft[] = [];
  const run = input.detail.run_info;
  const peers = input.comparisonRuns.filter(peer => samePipeline(run, peer) && peer.run_id !== run.run_id);
  const peerDurations = peers.map(peer => parseDurationSeconds(peer.duration)).filter(isFiniteNumber);
  const median = medianNumber(peerDurations);
  const currentDuration = parseDurationSeconds(run.duration);

  if (median > 0 && currentDuration > median * 1.4) {
    findings.push({
      category: 'cost',
      severity: 'opportunity',
      title: 'Successful run is slower than peer median',
      summary: `This successful run took ${formatSeconds(currentDuration)}, above the visible peer median of ${formatSeconds(median)}.`,
      evidence: [
        { label: 'Current duration', value: formatSeconds(currentDuration), kind: 'metric' },
        { label: 'Peer median', value: formatSeconds(median), kind: 'metric' },
      ],
      affectedResources: [runReference(run), ...(lastSuccess ? [runReference(lastSuccess)] : [])],
      recommendations: [{
        title: 'Check degradation before it becomes failure',
        detail: 'Compare slow steps, external dependency retries, cache hit rate, and AI token usage against previous successful runs.',
      }],
      confidence: 70,
    });
  }

  const failedIgnoredSteps = input.detail.steps.filter(step => isIgnoredFailureStepStatus(step.status) && step.configuration?.ignore_failure);
  if (failedIgnoredSteps.length > 0) {
    findings.push({
      category: 'reliability',
      severity: 'medium',
      title: 'Run succeeded with ignored failed steps',
      summary: `${failedIgnoredSteps.length} failed step${failedIgnoredSteps.length === 1 ? '' : 's'} were configured to ignore failure.`,
      evidence: failedIgnoredSteps.map(step => ({ label: 'Ignored step', value: step.name, kind: 'fact' as const })),
      affectedResources: [runReference(run), ...failedIgnoredSteps.map(step => stepReference(step.name))],
      recommendations: [{
        title: 'Review ignored failure policy',
        detail: 'Keep ignored failures only for explicitly non-blocking checks and publish their warnings as operational output.',
      }],
      confidence: 78,
    });
  }

  const spend = run.ai_usage?.spend_usd || 0;
  const peerSpend = peers.map(peer => peer.ai_usage?.spend_usd || 0).filter(value => value > 0);
  const medianSpend = medianNumber(peerSpend);
  if (spend > 0 && medianSpend > 0 && spend > medianSpend * 2) {
    findings.push({
      category: 'cost',
      severity: 'opportunity',
      title: 'AI spend is elevated',
      summary: `This run cost ${formatAnalysisSpend(spend)}, more than 2x the peer median.`,
      evidence: [
        { label: 'Current spend', value: formatAnalysisSpend(spend), kind: 'metric' },
        { label: 'Peer median spend', value: formatAnalysisSpend(medianSpend), kind: 'metric' },
      ],
      affectedResources: [runReference(run)],
      recommendations: [{
        title: 'Review AI work scope',
        detail: 'Reduce repeated knowledge retrieval, narrow prompts, or use a lower-cost profile for low-risk steps.',
      }],
      confidence: 70,
    });
  }

  return findings;
}

function createAnalysisResult({
  title,
  subjectType,
  subjectId,
  subjectLabel,
  scopePath,
  generatedAt,
  snapshotRevision,
  summary,
  tabs,
  findings,
  scoreCategories,
  scoreInputs,
  comparison,
  primaryDiagnosis,
}: {
  title: string;
  subjectType: AnalysisSubjectType;
  subjectId: string;
  subjectLabel: string;
  scopePath?: string;
  generatedAt: string;
  snapshotRevision: string;
  summary: string;
  tabs: AnalysisTab[];
  findings: FindingDraft[];
  scoreCategories: AnalysisCategory[];
  scoreInputs: string[];
  comparison?: AnalysisComparisonItem[];
  primaryDiagnosis?: AnalysisResult['primaryDiagnosis'];
}): AnalysisResult {
  const hydrated = findings.map((finding, index) => ({
    ...finding,
    id: findingId(subjectType, subjectId, finding, index),
    subjectType,
    subjectId,
    generatedAt,
    snapshotRevision,
  }));
  return {
    title,
    subjectType,
    subjectId,
    subjectLabel,
    scopePath,
    generatedAt,
    snapshotRevision,
    summary,
    healthScore: scoreFindings(hydrated).score,
    scoreBasis: scoreFindings(hydrated, scoreInputs),
    scores: scoreCategories.map(category => {
      const categoryFindings = hydrated.filter(finding => finding.category === category);
      const score = scoreFindings(categoryFindings);
      const label = CATEGORY_LABELS[category];
      return {
        category,
        label,
        score: score.score,
        findingCount: score.findingCount,
        deduction: score.totalDeduction,
        basis: `${label} starts at ${score.baseline} and subtracts ${score.totalDeduction} point${score.totalDeduction === 1 ? '' : 's'} from ${score.findingCount} ${score.findingCount === 1 ? 'finding' : 'findings'}.`,
      };
    }),
    findings: hydrated.sort(compareFindings),
    counts: countFindings(hydrated),
    tabs,
    safeguards: DEFAULT_SAFEGUARDS,
    comparison,
    primaryDiagnosis,
  };
}

function filterPipelineFindings(findings: FindingDraft[], scope: PipelineAnalysisScope): FindingDraft[] {
  if (scope === 'complete' || scope === 'pre-execution') return findings;
  const categoryByScope: Partial<Record<PipelineAnalysisScope, AnalysisCategory[]>> = {
    security: ['security'],
    reliability: ['reliability'],
    monitoring: ['monitoring'],
    performance: ['cost', 'efficiency'],
    maintainability: ['maintainability', 'organization'],
  };
  const categories = new Set(categoryByScope[scope] || []);
  return findings.filter(finding => categories.has(finding.category));
}

function detectDependencyIssues(steps: PipelineAnalysisStep[]): FindingDraft[] {
  const findings: FindingDraft[] = [];
  const stepNames = new Set(steps.map(step => step.name));
  const missing = steps.flatMap(step =>
    (step.depends_on || [])
      .filter(dependency => !stepNames.has(dependency))
      .map(dependency => ({ step: step.name, dependency }))
  );
  if (missing.length > 0) {
    findings.push({
      category: 'reliability',
      severity: 'high',
      title: 'Step dependency references are missing',
      summary: `${missing.length} dependency reference${missing.length === 1 ? '' : 's'} point to steps that are not present.`,
      evidence: missing.slice(0, 6).map(item => ({
        label: item.step,
        value: `depends_on ${item.dependency}`,
        kind: 'fact' as const,
      })),
      affectedResources: missing.slice(0, 6).map(item => stepReference(item.step)),
      recommendations: [{
        title: 'Fix dependency names',
        detail: 'Correct dependency names or add the missing steps before running.',
      }],
      confidence: 92,
    });
  }

  if (hasCycle(steps)) {
    findings.push({
      category: 'reliability',
      severity: 'critical',
      title: 'Cyclic step dependency detected',
      summary: 'The step graph contains a cycle, so execution order cannot be resolved safely.',
      evidence: [{ label: 'Cycle check', value: 'Cycle detected in depends_on graph', kind: 'fact' }],
      affectedResources: steps.map(step => stepReference(step.name)).slice(0, 8),
      recommendations: [{
        title: 'Break dependency cycle',
        detail: 'Remove one dependency edge or split setup/output work into separate acyclic steps.',
      }],
      confidence: 94,
    });
  }

  return findings;
}

function detectSequentialOpportunity(steps: PipelineAnalysisStep[]): FindingDraft | null {
  if (steps.length < 3) return null;
  const chainCount = steps.filter((step, index) => index > 0 && (step.depends_on || []).includes(steps[index - 1]?.name || '')).length;
  const names = steps.map(step => step.name.toLowerCase()).join(' ');
  if (chainCount < steps.length - 1 || !/\b(test|lint|scan|build)\b/.test(names)) return null;

  return {
    category: 'efficiency',
    severity: 'opportunity',
    title: 'Independent checks may be overly sequential',
    summary: 'Build, lint, scan, or test-like steps are arranged as a mostly linear chain.',
    evidence: [
      { label: 'Sequential edges', value: `${chainCount}/${Math.max(steps.length - 1, 1)}`, kind: 'metric' },
      { label: 'Potential impact', value: 'Parallel execution review', kind: 'inference' },
    ],
    affectedResources: steps.slice(0, 6).map(step => stepReference(step.name)),
    recommendations: [{
      title: 'Parallelize safe checks',
      detail: 'Run independent validation steps in parallel and keep deployment, publication, and cleanup ordered.',
    }],
    confidence: 64,
  };
}

function classifyRun(detail: RunAnalysisInput['detail']): {
  domain: RunRootCauseDomain;
  confidence: number;
  evidence: AnalysisEvidence[];
} {
  const run = detail.run_info;
  const firstFailure = findFirstFailedExecution(detail.steps);
  const text = [
    run.failure_reason,
    firstFailure?.step.name,
    firstFailure?.task?.task_name,
    firstFailure?.task?.status,
    firstFailure?.step.status,
  ].filter(Boolean).join(' ').toLowerCase();

  if (!isFailureStatus(run.status, run.is_complete)) {
    return {
      domain: 'Unknown',
      confidence: 54,
      evidence: [{ label: 'Run status', value: run.status || 'success', kind: 'fact' }],
    };
  }

  if (/yaml|schema|definition|depends_on|dependency|variable|missing step|invalid output/.test(text)) {
    return {
      domain: 'Pipeline definition',
      confidence: 84,
      evidence: [
        { label: 'Definition signal', value: redactInline(text), kind: 'inference' },
        { label: 'Pipeline setup', value: 'Failure references definition or dependency metadata', kind: 'fact' },
      ],
    };
  }

  if (/credential|permission|forbidden|unauthorized|auth|secret|token/.test(text)) {
    return {
      domain: 'Credential or authorization',
      confidence: 82,
      evidence: [
        { label: 'Authorization signal', value: redactInline(text), kind: 'redacted' },
        { label: 'Secret handling', value: 'Credential values redacted', kind: 'redacted' },
      ],
    };
  }

  if (/runner|kubernetes|pod|node|image pull|pullbackoff|network|storage|volume|heartbeat|capacity|queued/.test(text)) {
    return {
      domain: 'Runner infrastructure',
      confidence: 78,
      evidence: [
        { label: 'Infrastructure signal', value: redactInline(text), kind: 'inference' },
        { label: 'Failure location', value: firstFailure ? firstFailure.step.name : 'Before step completion', kind: 'fact' },
      ],
    };
  }

  if (/timeout|deadline|timed out|capacity|quota/.test(text)) {
    return {
      domain: 'Timeout or capacity',
      confidence: 78,
      evidence: [{ label: 'Timeout signal', value: redactInline(text), kind: 'inference' }],
    };
  }

  if (/approval|policy|rejected|denied/.test(text)) {
    return {
      domain: 'Approval or policy',
      confidence: 78,
      evidence: [{ label: 'Policy signal', value: redactInline(text), kind: 'inference' }],
    };
  }

  if (/openai|anthropic|gemini|llm|model|rate limit|tokens/.test(text)) {
    return {
      domain: 'AI provider/model',
      confidence: 72,
      evidence: [{ label: 'AI provider signal', value: redactInline(text), kind: 'inference' }],
    };
  }

  if (/test|spec|junit|assert|coverage/.test(text)) {
    return {
      domain: 'Application tests',
      confidence: 82,
      evidence: [
        { label: 'Failure location', value: firstFailure ? `${firstFailure.step.name}${firstFailure.task ? ` / ${firstFailure.task.task_name}` : ''}` : 'Unknown', kind: 'fact' },
        { label: 'Test signal', value: redactInline(text), kind: 'inference' },
      ],
    };
  }

  if (/build|compile|package|lint|static|health|exception|crash/.test(text)) {
    return {
      domain: 'Application code',
      confidence: 74,
      evidence: [
        { label: 'Failure location', value: firstFailure ? `${firstFailure.step.name}${firstFailure.task ? ` / ${firstFailure.task.task_name}` : ''}` : 'Unknown', kind: 'fact' },
        { label: 'Application signal', value: redactInline(text), kind: 'inference' },
      ],
    };
  }

  if (/input|trigger|payload|branch|ref/.test(text)) {
    return {
      domain: 'Trigger or input',
      confidence: 68,
      evidence: [{ label: 'Input signal', value: redactInline(text), kind: 'inference' }],
    };
  }

  return {
    domain: 'Unknown',
    confidence: firstFailure ? 58 : 45,
    evidence: [
      { label: 'Run status', value: run.status || 'failure', kind: 'fact' },
      { label: 'More evidence required', value: 'Step logs, runner logs, and previous successful run details', kind: 'inference' },
    ],
  };
}

function diagnosisActionTitle(domain: RunRootCauseDomain): string {
  if (domain === 'Application code' || domain === 'Application tests') return 'Inspect application change';
  if (domain === 'Pipeline definition') return 'Inspect pipeline definition';
  if (domain === 'Credential or authorization') return 'Review credential and grant scope';
  if (domain === 'Runner infrastructure') return 'Inspect runner infrastructure';
  if (domain === 'Timeout or capacity') return 'Review timeout and capacity';
  if (domain === 'Approval or policy') return 'Review approval or policy gate';
  return 'Collect more evidence';
}

function diagnosisActionDetail(
  domain: RunRootCauseDomain,
  firstFailure: ReturnType<typeof findFirstFailedExecution>
): string {
  const location = firstFailure
    ? `${firstFailure.step.name}${firstFailure.task ? ` / ${firstFailure.task.task_name}` : ''}`
    : 'the first recorded failure';
  if (domain === 'Application code' || domain === 'Application tests') {
    return `Start at ${location}, then compare the application commit against the last successful run.`;
  }
  if (domain === 'Pipeline definition') {
    return `Open the pipeline definition around ${location} and check variable names, dependencies, output references, and runner requirements.`;
  }
  if (domain === 'Credential or authorization') {
    return `Check declared credential references and AAA grants for ${location}; do not expose credential values in the diagnosis.`;
  }
  if (domain === 'Runner infrastructure') {
    return `Inspect runner pool health, image pulls, storage, network, and scheduling for ${location}.`;
  }
  if (domain === 'Timeout or capacity') {
    return `Compare duration, runner queue, external calls, and timeout settings around ${location}.`;
  }
  if (domain === 'Approval or policy') {
    return `Check assigned approval teams, policy decision, and whether the run is blocked by a pending or rejected gate.`;
  }
  return 'Open logs and compare with the last successful run before changing the pipeline or rerunning.';
}

function findFirstFailedExecution(steps: RunAnalysisStep[]) {
  for (const step of steps) {
    const failedTask = [...(step.tasks || [])]
      .sort((left, right) => left.task_index - right.task_index)
      .find(task => isFailureStatus(task.status));
    if (failedTask) return { step, task: failedTask };
    if (isFailureStatus(step.status)) return { step, task: null };
  }
  return null;
}

function findLastSuccessfulPeerRun(run: RunAnalysisRunInfo, peers: RunAnalysisRunInfo[]): RunAnalysisRunInfo | null {
  return peers
    .filter(peer => peer.run_id !== run.run_id && samePipeline(run, peer) && isSuccessStatus(peer.status, peer.is_complete))
    .sort((left, right) => runTime(right) - runTime(left))[0] || null;
}

function buildRunComparison(run: RunAnalysisRunInfo, previous: RunAnalysisRunInfo): AnalysisComparisonItem[] {
  return [
    comparisonItem('Application commit', previous.git_commit_sha, run.git_commit_sha, 12),
    comparisonItem('Pipeline revision', previous.pipeline_version, run.pipeline_version),
    comparisonItem('Pipeline source', previous.pipeline_source, run.pipeline_source),
    comparisonItem('Runner scope', previous.scope, run.scope),
    comparisonItem('Trigger source', previous.trigger_source, run.trigger_source),
    comparisonItem('Git ref', previous.git_ref, run.git_ref),
    comparisonItem('Runtime inputs', runtimeInputDigest(previous.runtime_variable_overrides), runtimeInputDigest(run.runtime_variable_overrides)),
  ];
}

function comparisonItem(label: string, before?: string, after?: string, shortLength?: number): AnalysisComparisonItem {
  const normalizedBefore = displayValue(before, shortLength);
  const normalizedAfter = displayValue(after, shortLength);
  return {
    label,
    before: normalizedBefore,
    after: normalizedAfter,
    changed: normalizedBefore !== normalizedAfter,
  };
}

function runtimeInputDigest(value?: Record<string, unknown>) {
  const keys = Object.keys(value || {}).sort();
  if (!keys.length) return '';
  return keys.map(key => `${key}=${safeScalar(value?.[key])}`).join(', ');
}

function safeScalar(value: unknown) {
  if (value == null || value === '') return '-';
  if (typeof value === 'string') return redactInline(value);
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return stableSnapshotRevision(value);
}

function displayValue(value?: string, shortLength?: number) {
  const normalized = String(value || '').trim();
  if (!normalized) return '—';
  return shortLength && normalized.length > shortLength ? normalized.slice(0, shortLength) : normalized;
}

function samePipeline(left: RunAnalysisRunInfo, right: RunAnalysisRunInfo) {
  return normalizePath(left.pipeline_path) === normalizePath(right.pipeline_path) &&
    normalizeResourceName(left.pipeline_name) === normalizeResourceName(right.pipeline_name);
}

function runTime(run: RunAnalysisRunInfo) {
  return Date.parse(run.started_at || run.finished_at || run.created_at || '') || 0;
}

function parsePipelineYaml(rawYaml: string): { ok: true; value: unknown } | { ok: false; error: string } {
  try {
    return { ok: true, value: yaml.load(rawYaml || '') };
  } catch (error) {
    return { ok: false, error: error instanceof Error ? error.message : 'Unknown YAML parse error' };
  }
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

function collectPipelineImages(input: PipelineAnalysisInput, parsedRecord: Record<string, unknown> | null) {
  const images: Array<{ label: string; value: string }> = [];
  const topImage = String(input.detail.containerImage || parsedRecord?.container_image || '').trim();
  if (topImage) images.push({ label: 'container_image', value: topImage });
  input.graphData.steps.forEach(step => {
    const image = String(step.configuration?.image || '').trim();
    if (image) images.push({ label: `step ${step.name}`, value: image });
  });
  return uniqueBy(images, item => `${item.label}:${item.value}`);
}

function collectScripts(steps: PipelineAnalysisStep[]) {
  return steps.flatMap(step => {
    const scripts: Array<{ location: string; script: string }> = [];
    if (step.configuration?.script) scripts.push({ location: `Step ${step.name}`, script: step.configuration.script });
    (step.configuration?.tasks || []).forEach(task => {
      if (task.script) scripts.push({ location: `Step ${step.name} / task ${task.name}`, script: task.script });
    });
    return scripts;
  });
}

function riskyShellScript(script: string) {
  const normalized = script.toLowerCase();
  if (/curl\b.+\|\s*(sh|bash)/s.test(normalized) || /wget\b.+\|\s*(sh|bash)/s.test(normalized)) return true;
  if (/\b(eval|source)\b/.test(normalized) && /[$`]/.test(script)) return true;
  if (/\b(kubectl|helm|docker|ssh|scp|bash|sh)\b/.test(normalized) && /\$\{?[A-Z0-9_]*(target|environment|branch|ref|command|script)[A-Z0-9_]*\}?/i.test(script)) {
    return true;
  }
  return false;
}

function detectSecretLiterals(rawYaml: string) {
  const lines = rawYaml.split(/\r?\n/);
  return lines
    .map((line, index) => ({ line, lineNumber: index + 1 }))
    .filter(item => {
      const match = item.line.match(/^\s*[\w.-]*(?:token|secret|password|api[_-]?key|private[_-]?key)[\w.-]*\s*:\s*(.+)$/i);
      if (!match) return false;
      const value = (match[1] || '').trim().replace(/^['"]|['"]$/g, '');
      if (!value || value === 'true' || value === 'false') return false;
      return !/^(credential|secret):\/\//i.test(value) &&
        !/^\$\{[^}]+}$/.test(value) &&
        !/^ENC\[/.test(value) &&
        !/^\*+$/.test(value) &&
        value !== '<redacted>';
    })
    .map(item => ({
      line: item.lineNumber,
      redacted: item.line.replace(/:\s*(.+)$/u, ': [redacted]'),
    }));
}

function isUnpinnedImage(image: string) {
  if (!image || image.includes('@sha256:')) return false;
  const lastSegment = image.split('/').pop() || image;
  if (!lastSegment.includes(':')) return true;
  return /:latest$/i.test(lastSegment);
}

function hasTimeout(parsedRecord: Record<string, unknown> | null, rawYaml: string) {
  return Boolean(parsedRecord?.timeout) || /^\s*timeout\s*:/im.test(rawYaml);
}

function mentionsProduction(rawYaml: string) {
  return /\b(prod|production)\b/i.test(rawYaml);
}

function collectCredentialReferences(rawYaml: string) {
  const matches = rawYaml.match(/credential:\/\/[^\s'",\]]+|credentials?:\s*[^\n]+|secrets?:\s*[^\n]+/gi) || [];
  return matches.map(match => redactInline(match));
}

function collectRunnerPools(steps: PipelineAnalysisStep[], parsedRecord: Record<string, unknown> | null) {
  const pools = new Set<string>();
  const top = String(parsedRecord?.runtime_pool || '').trim();
  if (top) pools.add(top);
  steps.forEach(step => {
    const pool = String(step.configuration?.runtime_pool || '').trim();
    if (pool) pools.add(pool);
  });
  return Array.from(pools);
}

function hasCycle(steps: PipelineAnalysisStep[]) {
  const graph = new Map(steps.map(step => [step.name, step.depends_on || []]));
  const visiting = new Set<string>();
  const visited = new Set<string>();

  const visit = (name: string): boolean => {
    if (visiting.has(name)) return true;
    if (visited.has(name)) return false;
    visiting.add(name);
    for (const dependency of graph.get(name) || []) {
      if (graph.has(dependency) && visit(dependency)) return true;
    }
    visiting.delete(name);
    visited.add(name);
    return false;
  };

  return steps.some(step => visit(step.name));
}

function pipelineReference(detail: PipelineAnalysisInput['detail']): ResourceReference {
  return {
    type: 'Pipeline',
    id: detail.id,
    label: detail.name || detail.id,
    href: `/pipelines/${detail.id.split('/').map(encodeURIComponent).join('/')}`,
  };
}

function runReference(run: RunAnalysisRunInfo | PipelineAnalysisRun): ResourceReference {
  return {
    type: 'Run',
    id: run.run_id,
    label: shortID(run.run_id),
    href: `/pipelineruns/recent?run=${encodeURIComponent(run.run_id)}`,
  };
}

function stepReference(stepName: string): ResourceReference {
  return {
    type: 'Step',
    id: stepName,
    label: stepName,
  };
}

function uniqueBy<T>(items: T[], keyFor: (item: T) => string): T[] {
  const seen = new Set<string>();
  return items.filter(item => {
    const key = keyFor(item);
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

function normalizePath(value?: string) {
  return String(value || '').trim().replace(/\/+/g, '/').replace(/^\/+|\/+$/g, '').toLowerCase();
}

function normalizeResourceName(value: string) {
  return String(value || '')
    .trim()
    .toLowerCase()
    .replace(/[_\s]+/g, '-')
    .replace(/[^a-z0-9.-]+/g, '')
    .replace(/^-+|-+$/g, '');
}

function scoreFindings(findings: AnalysisFinding[], inputs: string[] = []): AnalysisScoreBasis & { score: number } {
  const severityCounts = countFindings(findings);
  const totalDeduction = SEVERITIES.reduce(
    (total, severity) => total + severityCounts[severity] * SEVERITY_SCORE_WEIGHTS[severity],
    0
  );
  const findingCount = findings.length;
  const formula = `Starts at ${SCORE_BASELINE}; subtracts critical x ${SEVERITY_SCORE_WEIGHTS.critical}, high x ${SEVERITY_SCORE_WEIGHTS.high}, medium x ${SEVERITY_SCORE_WEIGHTS.medium}, low x ${SEVERITY_SCORE_WEIGHTS.low}, opportunity x ${SEVERITY_SCORE_WEIGHTS.opportunity}; clamps between 0 and 100.`;
  return {
    baseline: SCORE_BASELINE,
    formula,
    severityWeights: SEVERITY_SCORE_WEIGHTS,
    findingCount,
    totalDeduction,
    severityCounts,
    inputs,
    limitations: [
      'The score is a deterministic reviewer score, not a production SLO, risk register, or uptime metric.',
      'Only data already visible in the current UI snapshot contributes to the score.',
      'A structured AI evaluation can create a cached reviewed score for the same snapshot revision.',
    ],
    score: clamp(Math.round(SCORE_BASELINE - totalDeduction), 0, 100),
  };
}

function countFindings(findings: AnalysisFinding[]): Record<AnalysisSeverity, number> {
  return SEVERITIES.reduce((counts, severity) => {
    counts[severity] = findings.filter(finding => finding.severity === severity).length;
    return counts;
  }, {
    critical: 0,
    high: 0,
    medium: 0,
    low: 0,
    opportunity: 0,
  } satisfies Record<AnalysisSeverity, number>);
}

function compareFindings(left: AnalysisFinding, right: AnalysisFinding) {
  const severityCompare = severityRank(left.severity) - severityRank(right.severity);
  if (severityCompare !== 0) return severityCompare;
  return left.title.localeCompare(right.title);
}

function severityRank(severity: AnalysisSeverity) {
  return SEVERITIES.indexOf(severity);
}

function findingId(subjectType: AnalysisSubjectType, subjectId: string, finding: FindingDraft, index: number) {
  return slugify([subjectType, subjectId, finding.category, finding.severity, finding.title, String(index)].join('-'));
}

function slugify(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'finding';
}

function stableSnapshotRevision(value: unknown) {
  const text = stableStringify(value);
  let hash = 0x811c9dc5;
  for (let index = 0; index < text.length; index += 1) {
    hash ^= text.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return `snap-${(hash >>> 0).toString(16).padStart(8, '0')}`;
}

function stableStringify(value: unknown): string {
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`;
  const record = value as Record<string, unknown>;
  return `{${Object.keys(record).sort().map(key => `${JSON.stringify(key)}:${stableStringify(record[key])}`).join(',')}}`;
}

function redactRunForSnapshot(run: RunAnalysisRunInfo) {
  return {
    ...run,
    failure_reason: run.failure_reason ? redactInline(run.failure_reason) : undefined,
    runtime_variable_overrides: run.runtime_variable_overrides
      ? Object.fromEntries(Object.entries(run.runtime_variable_overrides).map(([key, value]) => [key, safeScalar(value)]))
      : undefined,
  };
}

function redactInline(value: string) {
  return String(value || '')
    .replace(/(token|secret|password|api[_-]?key|private[_-]?key)(\s*[=:]\s*)("[^"]+"|'[^']+'|[^\s,;]+)/gi, '$1$2[redacted]')
    .replace(/(credential:\/\/)[^\s'",\]]+/gi, '$1[redacted]')
    .slice(0, 260);
}

function isFailureStatus(status?: string, complete?: boolean) {
  const normalized = normalizeStatus(status);
  if (normalized === 'warning' || normalized === 'failure_(ignored)') return false;
  if (normalized === 'failure' || normalized === 'failed' || normalized === 'error' || normalized === 'cancelled' || normalized === 'rejected') return true;
  return complete === true && normalized !== 'success' && normalized !== 'succeeded' && normalized !== 'warning' && normalized !== 'skipped' && Boolean(normalized);
}

function isSuccessStatus(status?: string, complete?: boolean) {
  const normalized = normalizeStatus(status);
  return normalized === 'success' || normalized === 'succeeded' || normalized === 'warning' || normalized === 'failure_(ignored)' || (complete === true && normalized === 'completed');
}

function isIgnoredFailureStepStatus(status?: string) {
  const normalized = normalizeStatus(status);
  return normalized === 'warning' || normalized === 'failure_(ignored)' || isFailureStatus(status);
}

function normalizeStatus(status?: string) {
  return String(status || '').trim().toLowerCase().replace(/\s+/g, '_');
}

function parseDurationSeconds(value?: string) {
  const raw = String(value || '').trim();
  if (!raw || raw === '-') return Number.NaN;
  const clock = raw.match(/^(\d+):(\d{2})(?::(\d{2}))?$/);
  if (clock) {
    const first = Number(clock[1]);
    const second = Number(clock[2]);
    const third = Number(clock[3] || 0);
    return clock[3] ? first * 3600 + second * 60 + third : first * 60 + second;
  }
  let total = 0;
  const pattern = /(\d+(?:\.\d+)?)\s*(d|day|days|h|hr|hrs|hour|hours|m|min|mins|minute|minutes|s|sec|secs|second|seconds|ms|millisecond|milliseconds)\b/gi;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(raw))) {
    const amount = Number(match[1]);
    const unit = match[2].toLowerCase();
    if (unit.startsWith('d')) total += amount * 86400;
    else if (unit.startsWith('h')) total += amount * 3600;
    else if (unit.startsWith('m') && unit !== 'ms' && !unit.startsWith('milli')) total += amount * 60;
    else if (unit === 'ms' || unit.startsWith('milli')) total += amount / 1000;
    else total += amount;
  }
  return total > 0 ? total : Number.NaN;
}

/** Sub-cent amounts keep four decimals so cheap-but-frequent work is visible. */
function formatAnalysisSpend(value: number) {
  const fractionDigits = value > 0 && value < 0.01 ? 4 : 2;
  return value.toLocaleString(undefined, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  });
}

function medianNumber(values: number[]) {
  const sorted = values.filter(isFiniteNumber).sort((left, right) => left - right);
  if (!sorted.length) return 0;
  const middle = Math.floor(sorted.length / 2);
  if (sorted.length % 2) return sorted[middle] || 0;
  return ((sorted[middle - 1] || 0) + (sorted[middle] || 0)) / 2;
}

function isFiniteNumber(value: number): value is number {
  return Number.isFinite(value);
}

function formatSeconds(seconds: number) {
  if (!Number.isFinite(seconds) || seconds <= 0) return '-';
  const rounded = Math.round(seconds);
  const hours = Math.floor(rounded / 3600);
  const minutes = Math.floor((rounded % 3600) / 60);
  const rest = rounded % 60;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m ${rest}s`;
  return `${rest}s`;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

function shortID(value: string) {
  return value.length > 12 ? value.slice(0, 12) : value;
}
