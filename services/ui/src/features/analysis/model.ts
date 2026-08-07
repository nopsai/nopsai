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
  id: 'overview' | AnalysisCategory | 'reuse' | 'unused' | 'performance';
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
    total_tokens?: number;
    total_cost_usd?: number;
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
      total_tokens?: number;
      total_cost_usd?: number;
    };
  }>;
  ai_usage?: {
    total_tokens?: number;
    total_cost_usd?: number;
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
  { id: 'reuse', label: 'Reuse' },
  { id: 'unused', label: 'Unused' },
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

export function buildTeamResourceAnalysis(options: TeamResourceAnalysisOptions): AnalysisResult {
  const generatedAt = (options.now || new Date()).toISOString();
  const subjectType: AnalysisSubjectType = options.activeResource ? 'resource' : 'team';
  const subjectId = options.activeResource?.id || options.subjectId;
  const subjectLabel = options.activeResource?.label || options.subjectLabel;
  const scopePath = normalizePath(options.activeResource?.teamPath || options.scopePath);
  const snapshotRevision = stableSnapshotRevision({
    kind: 'team-resources',
    subjectId,
    scopePath,
    activeResource: options.activeResource?.id,
    resources: options.resources.map(redactResourceForSnapshot),
  });
  const drafts = options.activeResource
    ? buildIndividualResourceFindings(options.activeResource, options.resources, options.scopePath)
    : buildTeamResourceFindings(options.resources, options.scopePath, subjectLabel);

  return createAnalysisResult({
    title: options.activeResource ? 'Resource Analysis' : 'Team Resource Analysis',
    subjectType,
    subjectId,
    subjectLabel,
    scopePath,
    generatedAt,
    snapshotRevision,
    summary: teamResourceSummary(options.resources, drafts, options.activeResource),
    tabs: TEAM_RESOURCE_ANALYSIS_TABS,
    findings: drafts,
    scoreCategories: ['security', 'organization', 'efficiency', 'maintainability'],
    scoreInputs: options.activeResource ? [
      'selected resource metadata',
      'resource kind, label, team path, source, and link',
      'nearby visible resources used for duplicate and reuse checks',
      'redacted credential-like descriptions only',
    ] : [
      'visible team/application resource catalog',
      'resource kind, label, team path, source, and link metadata',
      'GitOps versus database source markers',
      'redacted credential-like descriptions only',
    ],
  });
}

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
  if (tabId === 'reuse') {
    return findings.filter(finding => /duplicate|similar|reuse|reusable|general/i.test(`${finding.title} ${finding.summary}`));
  }
  if (tabId === 'unused') {
    return findings.filter(finding => /unused|stale|disabled|archive|inactive/i.test(`${finding.title} ${finding.summary}`));
  }
  if (tabId === 'performance') {
    return findings.filter(finding => finding.category === 'cost' || finding.category === 'efficiency');
  }
  return findings.filter(finding => finding.category === tabId);
}

export function analysisCategoryLabel(category: AnalysisCategory): string {
  return CATEGORY_LABELS[category];
}

function buildTeamResourceFindings(
  resources: AnalyzableResource[],
  scopePath: string,
  subjectLabel: string
): FindingDraft[] {
  const findings: FindingDraft[] = [];
  const resourcesByKind = groupBy(resources, resource => resource.kind);
  const applications = resourcesByKind.get('application') || [];
  const automations = resources.filter(resource => resource.kind !== 'application');
  const scopedPath = normalizePath(scopePath);

  if (resources.length === 0) {
    findings.push({
      category: 'organization',
      severity: 'high',
      title: 'No visible team resources',
      summary: `${subjectLabel} has no visible applications, automations, scopes, credentials, or knowledge resources in this snapshot.`,
      evidence: [{ label: 'Visible resources', value: '0', kind: 'metric' }],
      affectedResources: [],
      recommendations: [{
        title: 'Connect ownership explicitly',
        detail: 'Attach the relevant applications and resource manifests to this team through GitOps or team-scoped records.',
      }],
      confidence: 86,
    });
    return findings;
  }

  for (const [kind, kindResources] of resourcesByKind) {
    duplicateResourceGroups(kindResources).forEach(group => {
      findings.push({
        category: kind === 'credential' ? 'security' : 'efficiency',
        severity: kind === 'credential' ? 'high' : 'medium',
        title: `Duplicate ${kindLabel(kind)} resources`,
        summary: `${group.length} ${kindLabel(kind)} resources share the same normalized name or purpose.`,
        evidence: [
          { label: 'Duplicate key', value: normalizeResourceName(group[0]?.label || ''), kind: 'fact' },
          { label: 'Affected count', value: String(group.length), kind: 'metric' },
        ],
        affectedResources: group.map(resourceReference),
        recommendations: [{
          title: 'Consolidate or parameterize',
          detail: 'Keep one canonical resource where possible and replace variants with parameters, explicit ownership, or a reusable template.',
        }],
        confidence: 82,
      });
    });
  }

  similarResourceGroups(resources.filter(resource => reusableKind(resource.kind))).forEach(group => {
    findings.push({
      category: 'efficiency',
      severity: 'opportunity',
      title: 'Similar resources can become reusable templates',
      summary: `${group.length} resources have highly similar names and likely overlapping responsibilities.`,
      evidence: [
        { label: 'Similarity threshold', value: '>= 0.80 token overlap', kind: 'metric' },
        { label: 'Reuse level', value: scopedPath ? 'Team reuse' : 'Organization reuse', kind: 'inference' },
      ],
      affectedResources: group.map(resourceReference),
      recommendations: [{
        title: 'Generate a reusable design',
        detail: 'Extract shared behavior into a reusable step, pipeline template, or profile and keep application-specific values as parameters.',
      }],
      confidence: 72,
    });
  });

  const inherited = resources.filter(resource => scopedPath && !normalizePath(resource.teamPath));
  if (inherited.length > 0) {
    findings.push({
      category: 'security',
      severity: inherited.some(resource => sensitiveKind(resource.kind)) ? 'high' : 'medium',
      title: 'Inherited global resources in team scope',
      summary: `${inherited.length} visible resources are inherited globally instead of owned by ${scopePath}.`,
      evidence: [
        { label: 'Team scope', value: scopePath, kind: 'fact' },
        { label: 'Inherited resources', value: String(inherited.length), kind: 'metric' },
      ],
      affectedResources: inherited.slice(0, 8).map(resourceReference),
      recommendations: [{
        title: 'Review scope boundaries',
        detail: 'Promote only mature reusable resources globally; move team-specific credentials, triggers, and profiles into team ownership.',
      }],
      confidence: 78,
    });
  }

  const sources = new Set(resources.map(resource => normalizeSource(resource.source)).filter(Boolean));
  if (sources.has('git') && sources.has('database')) {
    findings.push({
      category: 'organization',
      severity: 'medium',
      title: 'Mixed GitOps and database-managed resources',
      summary: 'This scope contains both GitOps-managed and database-managed resources, which can create drift and unclear ownership.',
      evidence: [
        { label: 'Sources', value: Array.from(sources).join(', '), kind: 'fact' },
        { label: 'GitOps compatibility', value: 'Policy review required', kind: 'inference' },
      ],
      affectedResources: resources.filter(resource => normalizeSource(resource.source) === 'database').slice(0, 8).map(resourceReference),
      recommendations: [{
        title: 'Standardize management source',
        detail: 'Move long-lived resources into GitOps manifests or document which database-managed resources are intentionally runtime-only.',
      }],
      confidence: 80,
    });
  }

  const inactive = resources.filter(resource => /disabled|deprecated|stale|inactive/i.test(`${resource.source || ''} ${resource.description}`));
  if (inactive.length > 0) {
    findings.push({
      category: 'efficiency',
      severity: 'opportunity',
      title: 'Inactive resources still require management',
      summary: `${inactive.length} resources appear disabled, stale, deprecated, or inactive.`,
      evidence: [{ label: 'Inactive candidates', value: String(inactive.length), kind: 'metric' }],
      affectedResources: inactive.slice(0, 8).map(resourceReference),
      recommendations: [{
        title: 'Archive unused resources',
        detail: 'Confirm consumers and archive resources that have no current runs, schedules, triggers, or application links.',
      }],
      confidence: 70,
    });
  }

  const credentials = resourcesByKind.get('credential') || [];
  const automationConsumers = resources.filter(resource => ['pipeline', 'step', 'trigger', 'schedule', 'external_trigger'].includes(resource.kind));
  const privilegedCredentials = credentials.filter(resource => /admin|root|prod|production|write/i.test(`${resource.label} ${resource.description}`));
  if (privilegedCredentials.length > 0 && automationConsumers.length > 2) {
    findings.push({
      category: 'security',
      severity: 'high',
      title: 'Privileged credential metadata needs consumer review',
      summary: `${privilegedCredentials.length} credential references look privileged while ${automationConsumers.length} automation resources are visible in the same scope.`,
      evidence: [
        { label: 'Credential values', value: 'Redacted; metadata only', kind: 'redacted' },
        { label: 'Automation consumers', value: String(automationConsumers.length), kind: 'metric' },
      ],
      affectedResources: privilegedCredentials.map(resourceReference),
      recommendations: [{
        title: 'Apply least privilege',
        detail: 'Separate read-only, CI, and deployment credentials, then reserve administrative credentials for production deployment operations.',
      }],
      confidence: 74,
    });
  }

  applications.forEach(application => {
    const appToken = lastPathSegment(application.teamPath || application.label);
    if (!appToken) return;
    const linked = automations.some(resource => resourceMatchesToken(resource, appToken));
    if (!linked) {
      findings.push({
        category: 'organization',
        severity: 'medium',
        title: 'Application has no linked automation resources',
        summary: `${application.label} is visible but no matching pipeline, schedule, trigger, scope, credential, or knowledge context was found.`,
        evidence: [
          { label: 'Application token', value: appToken, kind: 'fact' },
          { label: 'Association method', value: 'Path/name/repository metadata', kind: 'inference' },
        ],
        affectedResources: [resourceReference(application)],
        recommendations: [{
          title: 'Add authoritative resource ownership',
          detail: 'Link application resources through explicit team/app ownership instead of relying only on naming or path conventions.',
        }],
        confidence: 68,
      });
    }
  });

  const deepResources = resources.filter(resource => normalizePath(resource.teamPath).split('/').filter(Boolean).length > 3);
  if (deepResources.length > 0) {
    findings.push({
      category: 'organization',
      severity: 'low',
      title: 'Deep hierarchy increases ownership cost',
      summary: `${deepResources.length} resources sit four or more levels deep in the hierarchy.`,
      evidence: [{ label: 'Hierarchy depth threshold', value: '> 3 path segments', kind: 'metric' }],
      affectedResources: deepResources.slice(0, 6).map(resourceReference),
      recommendations: [{
        title: 'Flatten low-value nesting',
        detail: 'Keep team and application boundaries shallow unless the extra level maps to a real ownership or security boundary.',
      }],
      confidence: 64,
    });
  }

  return findings;
}

function buildIndividualResourceFindings(
  resource: AnalyzableResource,
  resources: AnalyzableResource[],
  scopePath: string
): FindingDraft[] {
  const findings: FindingDraft[] = [];
  const peers = resources.filter(item => item.id !== resource.id && item.kind === resource.kind);
  const matchingPeers = peers.filter(peer => normalizeResourceName(peer.label) === normalizeResourceName(resource.label));
  const similarPeers = peers.filter(peer => resourceSimilarity(peer, resource) >= 0.8 && normalizeResourceName(peer.label) !== normalizeResourceName(resource.label));

  if (matchingPeers.length > 0 || similarPeers.length > 0) {
    const matches = [...matchingPeers, ...similarPeers].slice(0, 6);
    findings.push({
      category: 'efficiency',
      severity: matchingPeers.length > 0 ? 'medium' : 'opportunity',
      title: 'Comparable resource already exists',
      summary: `${resource.label} overlaps with ${matches.length} visible ${kindLabel(resource.kind)} resource${matches.length === 1 ? '' : 's'}.`,
      evidence: [
        { label: 'Comparison method', value: matchingPeers.length > 0 ? 'Normalized name match' : 'Token similarity', kind: 'inference' },
      ],
      affectedResources: [resource, ...matches].map(resourceReference),
      recommendations: [{
        title: 'Reuse before adding another resource',
        detail: 'Check whether the existing resource can be parameterized or promoted to a reusable team template.',
      }],
      confidence: matchingPeers.length > 0 ? 82 : 70,
    });
  }

  if (scopePath && !normalizePath(resource.teamPath)) {
    findings.push({
      category: sensitiveKind(resource.kind) ? 'security' : 'organization',
      severity: sensitiveKind(resource.kind) ? 'high' : 'medium',
      title: 'Resource is inherited globally',
      summary: `${resource.label} is visible in ${scopePath} without a team-local owner.`,
      evidence: [
        { label: 'Team scope', value: scopePath, kind: 'fact' },
        { label: 'Resource scope', value: 'Global', kind: 'fact' },
      ],
      affectedResources: [resourceReference(resource)],
      recommendations: [{
        title: 'Confirm reuse boundary',
        detail: 'Keep the resource global only if it is intentionally shared; otherwise create a team-owned resource with least-privilege access.',
      }],
      confidence: 78,
    });
  }

  if (normalizeSource(resource.source) === 'database') {
    findings.push({
      category: 'organization',
      severity: 'medium',
      title: 'Database-managed resource needs GitOps review',
      summary: `${resource.label} is not marked as GitOps-managed in this snapshot.`,
      evidence: [{ label: 'Source', value: resource.source || 'database', kind: 'fact' }],
      affectedResources: [resourceReference(resource)],
      recommendations: [{
        title: 'Move durable configuration to GitOps',
        detail: 'Store durable resource definitions in the config repository, or document why this item must remain runtime-managed.',
      }],
      confidence: 76,
    });
  }

  if (/disabled|deprecated|stale|inactive/i.test(`${resource.source || ''} ${resource.description}`)) {
    findings.push({
      category: 'efficiency',
      severity: 'opportunity',
      title: 'Resource appears inactive',
      summary: `${resource.label} has disabled, stale, deprecated, or inactive metadata.`,
      evidence: [{ label: 'Metadata', value: redactInline(`${resource.source || ''} ${resource.description}`), kind: 'fact' }],
      affectedResources: [resourceReference(resource)],
      recommendations: [{
        title: 'Archive after consumer check',
        detail: 'Confirm no schedule, trigger, pipeline, or application still depends on this resource, then archive it through GitOps.',
      }],
      confidence: 74,
    });
  }

  if (resource.kind === 'credential') {
    findings.push(...buildCredentialResourceFindings(resource));
  } else if (resource.kind === 'mcp_profile') {
    findings.push(...buildMCPResourceFindings(resource));
  } else if (resource.kind === 'knowledge_context') {
    findings.push(...buildKnowledgeResourceFindings(resource));
  } else if (resource.kind === 'pipeline') {
    findings.push(...buildPipelineResourceCatalogFindings(resource));
  } else if (resource.kind === 'schedule') {
    findings.push(...buildScheduleResourceFindings(resource, peers));
  } else if (resource.kind === 'trigger' || resource.kind === 'external_trigger' || resource.kind === 'git_webhook_source') {
    findings.push(...buildTriggerResourceFindings(resource));
  }

  if (findings.length === 0) {
    findings.push({
      category: 'maintainability',
      severity: 'low',
      title: 'No immediate metadata risk detected',
      summary: `${resource.label} has a clear kind, route, and visible ownership metadata in this snapshot.`,
      evidence: [
        { label: 'Resource kind', value: kindLabel(resource.kind), kind: 'fact' },
        { label: 'Resource scope', value: resource.teamPath || 'Global', kind: 'fact' },
      ],
      affectedResources: [resourceReference(resource)],
      recommendations: [{
        title: 'Keep ownership metadata explicit',
        detail: 'Continue managing the resource through the expected ownership and GitOps workflow.',
      }],
      confidence: 62,
    });
  }

  return findings;
}

function buildCredentialResourceFindings(resource: AnalyzableResource): FindingDraft[] {
  const findings: FindingDraft[] = [];
  const label = `${resource.label} ${resource.description}`;
  const privileged = /admin|root|owner|prod|production|write/i.test(label);

  if (privileged) {
    findings.push({
      category: 'security',
      severity: 'high',
      title: 'Credential appears privileged',
      summary: `${resource.label} metadata suggests administrative, production, owner, or write-level access.`,
      evidence: [
        { label: 'Credential value', value: 'Redacted; metadata only', kind: 'redacted' },
        { label: 'Privilege signal', value: redactInline(label), kind: 'inference' },
      ],
      affectedResources: [resourceReference(resource)],
      recommendations: [{
        title: 'Split credential scopes',
        detail: 'Reserve this credential for the narrow deployment path and use read-only credentials for documentation, checks, and release-note jobs.',
      }],
      confidence: 76,
    });
  }

  return findings;
}

function buildMCPResourceFindings(resource: AnalyzableResource): FindingDraft[] {
  const serverCount = resource.description.split(',').map(value => value.trim()).filter(Boolean).length;
  if (serverCount <= 3) return [];
  return [{
    category: 'security',
    severity: 'medium',
    title: 'MCP profile exposes many server connections',
    summary: `${resource.label} references ${serverCount} MCP server entries in metadata.`,
    evidence: [{ label: 'Server count', value: String(serverCount), kind: 'metric' }],
    affectedResources: [resourceReference(resource)],
    recommendations: [{
      title: 'Reduce tool reach',
      detail: 'Separate high-risk tools into a narrower profile and grant it only to pipelines that need those tools.',
    }],
    confidence: 68,
  }];
}

function buildKnowledgeResourceFindings(resource: AnalyzableResource): FindingDraft[] {
  if (!/public/i.test(resource.description)) return [];
  return [{
    category: 'security',
    severity: 'medium',
    title: 'Public knowledge context attached to automation scope',
    summary: `${resource.label} is marked public in metadata and may be reachable by more pipelines than intended.`,
    evidence: [{ label: 'Visibility signal', value: redactInline(resource.description), kind: 'fact' }],
    affectedResources: [resourceReference(resource)],
    recommendations: [{
      title: 'Review data classification',
      detail: 'Keep private runbooks, policies, and architecture context team-owned unless they are approved for organization-wide reuse.',
    }],
    confidence: 72,
  }];
}

function buildPipelineResourceCatalogFindings(resource: AnalyzableResource): FindingDraft[] {
  if (!/deploy|release|prod|production/i.test(`${resource.label} ${resource.description}`)) return [];
  return [{
    category: 'security',
    severity: 'medium',
    title: 'Deployment pipeline needs approval and credential review',
    summary: `${resource.label} appears to affect deployment or release operations.`,
    evidence: [{ label: 'Pipeline signal', value: redactInline(`${resource.label} ${resource.description}`), kind: 'inference' }],
    affectedResources: [resourceReference(resource)],
    recommendations: [{
      title: 'Verify production guardrails',
      detail: 'Confirm production approval, least-privilege credentials, rollback notes, and runbook links are present in the pipeline definition.',
    }],
    confidence: 66,
  }];
}

function buildScheduleResourceFindings(resource: AnalyzableResource, peers: AnalyzableResource[]): FindingDraft[] {
  const duplicateSchedule = peers.some(peer => normalizeResourceName(peer.description) === normalizeResourceName(resource.description));
  if (!duplicateSchedule) return [];
  return [{
    category: 'efficiency',
    severity: 'medium',
    title: 'Schedule appears duplicated',
    summary: `${resource.label} shares timing or target metadata with another schedule.`,
    evidence: [{ label: 'Schedule metadata', value: redactInline(resource.description), kind: 'inference' }],
    affectedResources: [resourceReference(resource)],
    recommendations: [{
      title: 'Consolidate schedule triggers',
      detail: 'Keep one schedule per operational intent and remove duplicate timing paths that run the same pipeline unnecessarily.',
    }],
    confidence: 67,
  }];
}

function buildTriggerResourceFindings(resource: AnalyzableResource): FindingDraft[] {
  if (normalizePath(resource.teamPath)) return [];
  return [{
    category: 'security',
    severity: 'medium',
    title: 'Global trigger source needs scope review',
    summary: `${resource.label} is visible without team-local ownership metadata.`,
    evidence: [{ label: 'Trigger scope', value: 'Global', kind: 'fact' }],
    affectedResources: [resourceReference(resource)],
    recommendations: [{
      title: 'Restrict event reach',
      detail: 'Use repository allowlists, team run paths, and event validation to avoid broad trigger access.',
    }],
    confidence: 70,
  }];
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

  const tokens = run.ai_usage?.total_tokens || 0;
  const peerTokens = peers.map(peer => peer.ai_usage?.total_tokens || 0).filter(value => value > 0);
  const medianTokens = medianNumber(peerTokens);
  if (tokens > 0 && medianTokens > 0 && tokens > medianTokens * 2) {
    findings.push({
      category: 'cost',
      severity: 'opportunity',
      title: 'AI token usage is elevated',
      summary: `This run used ${tokens.toLocaleString()} tokens, more than 2x the peer median.`,
      evidence: [
        { label: 'Current tokens', value: tokens.toLocaleString(), kind: 'metric' },
        { label: 'Peer median tokens', value: Math.round(medianTokens).toLocaleString(), kind: 'metric' },
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

function duplicateResourceGroups(resources: AnalyzableResource[]) {
  return Array.from(groupBy(resources, resource => normalizeResourceName(resource.label)).values())
    .filter(group => group.length > 1);
}

function similarResourceGroups(resources: AnalyzableResource[]) {
  const groups: AnalyzableResource[][] = [];
  const used = new Set<string>();
  resources.forEach(resource => {
    if (used.has(resource.id)) return;
    const group = resources.filter(other => other.id !== resource.id && !used.has(other.id) && resourceSimilarity(resource, other) >= 0.8);
    if (group.length === 0) return;
    const fullGroup = [resource, ...group].slice(0, 6);
    fullGroup.forEach(item => used.add(item.id));
    groups.push(fullGroup);
  });
  return groups;
}

function resourceSimilarity(left: AnalyzableResource, right: AnalyzableResource) {
  if (left.kind !== right.kind) return 0;
  const leftTokens = new Set(resourceTokens(left));
  const rightTokens = new Set(resourceTokens(right));
  if (!leftTokens.size || !rightTokens.size) return 0;
  const intersection = Array.from(leftTokens).filter(token => rightTokens.has(token)).length;
  const union = new Set([...leftTokens, ...rightTokens]).size;
  return intersection / union;
}

function resourceTokens(resource: AnalyzableResource) {
  return `${resource.label} ${resource.description}`
    .toLowerCase()
    .split(/[^a-z0-9]+/)
    .filter(token => token.length > 2 && !['the', 'and', 'for', 'with', 'pipeline', 'enabled', 'global', 'team'].includes(token));
}

function resourceMatchesToken(resource: AnalyzableResource, token: string) {
  const normalizedToken = normalizeResourceName(token);
  return resourceTokens(resource).some(candidate => candidate === normalizedToken || candidate.includes(normalizedToken) || normalizedToken.includes(candidate));
}

function reusableKind(kind: string) {
  return ['pipeline', 'step', 'schedule', 'trigger', 'agent_profile', 'llm_profile', 'mcp_profile'].includes(kind);
}

function sensitiveKind(kind: string) {
  return ['credential', 'mcp_profile', 'llm_profile', 'agent_profile', 'scope', 'knowledge_context', 'trigger', 'external_trigger', 'git_webhook_source'].includes(kind);
}

function resourceReference(resource: AnalyzableResource): ResourceReference {
  return {
    type: kindLabel(resource.kind),
    id: resource.id,
    label: resource.label,
    href: resource.href,
  };
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

function groupBy<T>(items: T[], keyFor: (item: T) => string): Map<string, T[]> {
  const map = new Map<string, T[]>();
  items.forEach(item => {
    const key = keyFor(item);
    const group = map.get(key) || [];
    group.push(item);
    map.set(key, group);
  });
  return map;
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

function normalizeSource(value?: string) {
  const source = String(value || '').trim().toLowerCase();
  if (!source) return '';
  if (source.includes('git')) return 'git';
  if (source.includes('db') || source.includes('database')) return 'database';
  if (source.includes('disabled')) return 'disabled';
  return source;
}

function kindLabel(kind: string) {
  return kind.replace(/_/g, ' ');
}

function lastPathSegment(value: string) {
  return normalizePath(value).split('/').filter(Boolean).pop() || normalizeResourceName(value);
}

function teamResourceSummary(resources: AnalyzableResource[], findings: FindingDraft[], activeResource?: AnalyzableResource | null) {
  if (activeResource) {
    return `Reviewed ${activeResource.label} as a ${kindLabel(activeResource.kind)} using visible metadata only.`;
  }
  const criticalHigh = findings.filter(finding => finding.severity === 'critical' || finding.severity === 'high').length;
  return `Reviewed ${resources.length} resources across ${new Set(resources.map(resource => resource.kind)).size} resource kinds. ${criticalHigh} critical or high findings.`;
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

function redactResourceForSnapshot(resource: AnalyzableResource) {
  return {
    ...resource,
    description: redactInline(resource.description),
  };
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
