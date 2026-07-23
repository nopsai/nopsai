export type WikiArticleLevel = 'Start' | 'Operate' | 'Reference' | 'Admin' | 'Troubleshoot' | 'Security';

export type WikiDocType = 'tutorial' | 'how-to' | 'concept' | 'reference' | 'runbook' | 'troubleshooting';

export type WikiAudience =
  | 'new-user'
  | 'automation-author'
  | 'operator'
  | 'administrator'
  | 'security'
  | 'developer';

export type WikiConfigRow = {
  key: string;
  area: string;
  description: string;
  example: string;
  path?: string;
  type?: string;
  required?: boolean | 'conditional';
  defaultValue?: string;
  allowedValues?: string[];
  scope?: string;
  constraints?: string[];
  inheritedFrom?: string[];
  permission?: string;
  introducedIn?: string;
  deprecatedIn?: string;
  security?: string;
};

export type WikiExample = {
  title: string;
  language: string;
  code: string;
  complete?: boolean;
  expectedOutput?: string;
  placeholderNotes?: string[];
  testedIn?: string;
  permission?: string;
  validationCommand?: string;
  rollback?: string;
};

export type WikiPrerequisite = {
  label: string;
  value: string;
  verification?: string;
};

export type WikiStep = {
  title: string;
  description: string;
  commands?: WikiExample[];
  expectedOutput?: string;
  verification?: string;
  warning?: string;
};

export type WikiSource = {
  title: string;
  repositoryPath: string;
  sourceUrl?: string;
  sourceLines?: string;
  purpose: string;
};

export type WikiRunbook = {
  id: string;
  title: string;
  symptoms: string[];
  impact: string;
  requiredAccess: string;
  initialChecks: string[];
  diagnosticCommands: string[];
  resolution: string[];
  rollback?: string;
  escalation?: string;
  metrics?: string[];
};

export type WikiArticleMetadata = {
  appliesTo: string;
  owner: string;
  introducedIn: string;
  lastVerified: string;
  sourceCommit: string;
  status: 'current' | 'preview' | 'deprecated' | 'archived';
};

export type WikiArticle = {
  id: string;
  title: string;
  level: WikiArticleLevel;
  audience: string;
  docType: WikiDocType;
  audiences: WikiAudience[];
  summary: string;
  keyFacts: string[];
  details: string[];
  prerequisites: WikiPrerequisite[];
  steps: WikiStep[];
  configRows: WikiConfigRow[];
  examples: WikiExample[];
  relatedDocs: string[];
  sourceLinks: WikiSource[];
  runbooks: string[];
  runbookEntries: WikiRunbook[];
  caveats: string[];
  metadata: WikiArticleMetadata;
};

export type WikiSection = {
  id: string;
  title: string;
  owner: string;
  description: string;
  articles: WikiArticle[];
};

export type WikiSummary = {
  sections: number;
  articles: number;
  configKeys: number;
  runbooks: number;
  caveats: number;
  tutorials: number;
  proceduralPages: number;
  sourceLinks: number;
};

type WikiArticleInput = Omit<
  WikiArticle,
  'docType' | 'audiences' | 'prerequisites' | 'steps' | 'sourceLinks' | 'runbookEntries' | 'metadata'
> &
  Partial<Pick<WikiArticle, 'docType' | 'audiences' | 'prerequisites' | 'steps' | 'sourceLinks' | 'runbookEntries'>> & {
    metadata?: Partial<WikiArticleMetadata>;
  };

type WikiSectionInput = Omit<WikiSection, 'articles'> & {
  articles: WikiArticleInput[];
};

export const wikiMetadata = {
  title: 'NopsAI Product Wiki',
  status: 'Repository-grounded current implementation',
  apiVersion: 'Product API v1',
  audience: 'Users, platform engineers, administrators, security teams, automation authors, and operators',
  sourceOrder: [
    'Runtime code, schemas, and route behavior',
    'Docker Compose and Helm deployment files',
    'Focused markdown under doc/',
    'Root README examples',
    'UI help text only when it agrees with implementation',
  ],
};

export const unsupportedClaims = [
  'Terraform modules or cloud-provider-specific EKS, AKS, or GKE automation',
  'Built-in Redis dependency',
  'S3 or generic object-storage artifact and backup backend',
  'Helm-managed PostgreSQL',
  'Documented HPA/autoscaling or Kubernetes NetworkPolicy set',
  'Complete air-gap installation workflow',
  'Product-managed restore workflow',
  'Validated production sizing, throughput, or multi-region HA guarantees',
];

export const supportedDeploymentModels = [
  {
    target: 'Docker Compose',
    purpose: 'Development, demos, evaluation, and small single-host installs',
    execution: 'Docker runner',
    storage: 'Included PostgreSQL 15 with a named database volume',
  },
  {
    target: 'Release bundle',
    purpose: 'Version-pinned single-host deployment',
    execution: 'Docker runner',
    storage: 'Deployment-managed PostgreSQL and deployment-owned backups',
  },
  {
    target: 'Helm/Kubernetes',
    purpose: 'Cluster-based production deployment',
    execution: 'Kubernetes runner',
    storage: 'External PostgreSQL and PVC-backed run workspaces',
  },
  {
    target: 'Hybrid runners',
    purpose: 'Central control plane with runners in separate Docker hosts or Kubernetes namespaces',
    execution: 'Docker and Kubernetes runners',
    storage: 'Control-plane PostgreSQL plus runner-local workspace storage',
  },
];

const DEFAULT_VERIFIED_DATE = 'July 2026';
const DEFAULT_SOURCE_COMMIT = '3558159';

export function wikiArticlePath(sectionID: string, articleID: string) {
  return `/docs/${encodeURIComponent(sectionID)}/${encodeURIComponent(articleID)}`;
}

export function findWikiArticleByPath(sections: WikiSection[], pathname: string) {
  const segments = pathname.split('/').filter(Boolean);
  if (segments[0] !== 'docs' || segments.length < 3) return undefined;
  const sectionID = decodeRouteSegment(segments[1] || '');
  const articleID = decodeRouteSegment(segments[2] || '');
  const section = sections.find(candidate => candidate.id === sectionID);
  const article = section?.articles.find(candidate => candidate.id === articleID);
  return section && article ? { section, article } : undefined;
}

export function wikiDocTypeLabel(type: WikiDocType) {
  return {
    tutorial: 'Tutorial',
    'how-to': 'How-to',
    concept: 'Concept',
    reference: 'Reference',
    runbook: 'Runbook',
    troubleshooting: 'Troubleshooting',
  }[type];
}

export function wikiAudienceLabel(audience: WikiAudience) {
  return {
    'new-user': 'New user',
    'automation-author': 'Automation author',
    operator: 'Operator',
    administrator: 'Administrator',
    security: 'Security',
    developer: 'Developer',
  }[audience];
}

function decodeRouteSegment(segment: string) {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

function normalizeWikiSections(sections: WikiSectionInput[]): WikiSection[] {
  return sections.map(section => ({
    ...section,
    articles: section.articles.map(article => normalizeWikiArticle(section, article)),
  }));
}

function normalizeWikiArticle(section: WikiSectionInput, article: WikiArticleInput): WikiArticle {
  const configRows = article.configRows.map(normalizeConfigRow);
  const sourceLinks = article.sourceLinks || article.relatedDocs.map(doc => sourceFromPath(doc));
  const runbookEntries = article.runbookEntries || article.runbooks.map(title => runbookFromTitle(title, article));

  return {
    ...article,
    docType: article.docType || docTypeFromLevel(article.level),
    audiences: article.audiences || audiencesFromLegacy(article.audience),
    prerequisites: article.prerequisites || [],
    steps: article.steps || [],
    configRows,
    sourceLinks,
    runbookEntries,
    metadata: {
      appliesTo: wikiMetadata.apiVersion,
      owner: section.owner,
      introducedIn: 'current',
      lastVerified: DEFAULT_VERIFIED_DATE,
      sourceCommit: DEFAULT_SOURCE_COMMIT,
      status: 'current',
      ...article.metadata,
    },
  };
}

function normalizeConfigRow(row: WikiConfigRow): WikiConfigRow {
  return {
    path: row.path || row.key,
    type: row.type || inferFieldType(row.example),
    required: row.required ?? inferRequired(row.description),
    defaultValue: row.defaultValue || inferDefault(row.description),
    scope: row.scope || row.area,
    ...row,
  };
}

function sourceFromPath(repositoryPath: string): WikiSource {
  const filename = repositoryPath.split('/').filter(Boolean).pop() || repositoryPath;
  return {
    title: filename,
    repositoryPath,
    purpose: repositoryPath.startsWith('doc/')
      ? 'Repository documentation evidence already summarized in this wiki article.'
      : 'Implementation or deployment evidence used to verify the article.',
  };
}

function runbookFromTitle(title: string, article: WikiArticleInput): WikiRunbook {
  return {
    id: slugify(title),
    title,
    symptoms: [`You need to perform or investigate: ${title}.`],
    impact: article.summary,
    requiredAccess: defaultRunbookAccess(article),
    initialChecks: ['Confirm the target team, scope, repository, pipeline, and run ID before changing state.'],
    diagnosticCommands: [],
    resolution: ['Follow the article details, inspect the listed implementation evidence when needed, then verify the result through UI, API, CLI, or metrics.'],
    escalation: 'Escalate to the article owner when the expected resource, permission, route, or metric is missing.',
  };
}

function docTypeFromLevel(level: WikiArticleLevel): WikiDocType {
  if (level === 'Start') return 'concept';
  if (level === 'Operate' || level === 'Admin') return 'how-to';
  if (level === 'Troubleshoot') return 'troubleshooting';
  if (level === 'Security') return 'reference';
  return 'reference';
}

function audiencesFromLegacy(value: string): WikiAudience[] {
  const normalized = value.toLowerCase();
  const audiences = new Set<WikiAudience>();
  if (normalized.includes('new') || normalized.includes('buyer')) audiences.add('new-user');
  if (normalized.includes('automation') || normalized.includes('author') || normalized.includes('release')) audiences.add('automation-author');
  if (normalized.includes('operator') || normalized.includes('sre') || normalized.includes('support')) audiences.add('operator');
  if (normalized.includes('admin') || normalized.includes('platform')) audiences.add('administrator');
  if (normalized.includes('security') || normalized.includes('audit')) audiences.add('security');
  if (normalized.includes('developer') || normalized.includes('integration')) audiences.add('developer');
  if (audiences.size === 0) audiences.add('operator');
  return Array.from(audiences);
}

function inferFieldType(example: string) {
  const trimmed = example.trim();
  if (trimmed === 'true' || trimmed === 'false') return 'boolean';
  if (/^\d+$/.test(trimmed)) return 'integer';
  if (/^\[.*\]$/.test(trimmed)) return 'array';
  if (/^\{.*\}$/.test(trimmed)) return 'object';
  if (/^\d+[smhd]$/.test(trimmed)) return 'duration';
  if (trimmed.endsWith('.yaml') || trimmed.includes('/')) return 'path';
  return 'string';
}

function inferRequired(description: string): boolean | 'conditional' {
  const normalized = description.toLowerCase();
  if (normalized.includes('required')) return true;
  if (normalized.includes('optional') || normalized.includes('defaults') || normalized.includes('default')) return false;
  return 'conditional';
}

function inferDefault(description: string) {
  const normalized = description.toLowerCase();
  if (normalized.includes('defaults to utc')) return 'UTC';
  if (normalized.includes('defaults to replace')) return 'replace';
  if (normalized.includes('defaults it to latest')) return 'latest';
  if (normalized.includes('defaults to false')) return 'false';
  if (normalized.includes('defaults to true')) return 'true';
  return undefined;
}

function defaultRunbookAccess(article: WikiArticleInput) {
  if (article.audience.toLowerCase().includes('security')) return 'Security administrator or owner for the affected resource.';
  if (article.audience.toLowerCase().includes('administrator')) return 'Platform administrator or owner for the affected resource.';
  return 'Read access to the affected resource, plus write access only for corrective changes.';
}

function slugify(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

const pipelineTopLevelRows: WikiConfigRow[] = [
  {
    key: 'name',
    area: 'Pipeline YAML',
    description: 'Required stable pipeline name. It may contain letters, numbers, underscores, dots, and hyphens.',
    example: 'release-readiness',
  },
  {
    key: 'version',
    area: 'Pipeline YAML',
    description: 'Optional pipeline version. When omitted, runtime validation defaults it to latest.',
    example: '1.0.0',
  },
  {
    key: 'description',
    area: 'Pipeline YAML',
    description: 'Human-readable explanation shown in API/UI contexts and useful during reviews.',
    example: 'Build, test, and deploy the API.',
  },
  {
    key: 'container_image',
    area: 'Pipeline YAML',
    description: 'Default executable image. Required unless every executable step has its own image; approval-only steps do not need one.',
    example: 'alpine:3.20',
  },
  {
    key: 'working_directory',
    area: 'Pipeline YAML',
    description: 'Container working directory. Empty and dot resolve to /workspace; relative paths are rooted under /workspace and may not escape it.',
    example: '/workspace',
  },
  {
    key: 'display_options.github_view',
    area: 'Pipeline YAML',
    description: 'Preferred GitHub check visualization when GitHub integration renders pipeline progress.',
    example: 'mermaid',
  },
  {
    key: 'variables',
    area: 'Pipeline YAML',
    description: 'Required runtime variable references for the run. Bare names resolve in the run scope; scope:name resolves another scope and injects name.',
    example: '[API_VERSION, default:REGION, prod:IMAGE_TAG]',
  },
  {
    key: 'timeout',
    area: 'Pipeline YAML',
    description: 'Optional whole-run timeout using Go duration syntax. Timeout failure stops remaining work.',
    example: '45m',
  },
  {
    key: 'llm_enabled',
    area: 'Pipeline YAML',
    description: 'Set false only for script-only pipelines. It rejects goals, conditions, MCP profile validation, and final outputs.',
    example: 'false',
    type: 'boolean',
    required: false,
    defaultValue: 'true',
    scope: 'pipeline',
    constraints: ['false requires script-only execution and rejects goals, conditions, explicit MCP profiles, and final outputs.'],
  },
  {
    key: 'agent_profile',
    area: 'Pipeline YAML',
    description: 'Pipeline-level AI persona. Step agent_profile can override it; tasks cannot define agent_profile.',
    example: 'release-manager',
    type: 'string',
    required: false,
    scope: 'pipeline',
    permission: 'agent_profile.use',
    constraints: ['Tasks cannot set agent_profile.'],
  },
  {
    key: 'llm_profile',
    area: 'Pipeline YAML',
    description: 'Pipeline-level model profile. It selects provider/model/client settings, not persona or permissions.',
    example: 'standard',
    type: 'string',
    required: 'conditional',
    scope: 'pipeline',
    inheritedFrom: ['configured default LLM profile'],
    permission: 'llm_profile.use',
    constraints: ['Step, task, output, and output item overrides can select a more specific profile where supported.'],
  },
  {
    key: 'mcp_profiles',
    area: 'Pipeline YAML',
    description: 'Approved MCP tool profiles available to LLM goals. Profiles are additive with step and task MCP profiles.',
    example: '[github-pr-review]',
    type: 'array<string>',
    required: false,
    scope: 'pipeline',
    permission: 'mcp_profile.use',
    constraints: ['Valid only for LLM-backed goal work.'],
  },
  {
    key: 'runtime_pool',
    area: 'Pipeline YAML',
    description: 'Kubernetes runtime pool default for steps. Docker runners ignore runtime pools.',
    example: 'high-memory',
  },
  {
    key: 'affinity_enabled',
    area: 'Pipeline YAML',
    description: 'Overrides the Kubernetes runner same-node affinity default for this run.',
    example: 'true',
  },
  {
    key: 'knowledge_context',
    area: 'Pipeline YAML',
    description: 'Pipeline-level knowledge references merged into every LLM condition and goal in the run.',
    example: 'kind: guardrail, ref: security/repo-check',
    type: 'array<object>',
    required: false,
    scope: 'pipeline',
    permission: 'knowledge_context.use',
    constraints: ['Each entry must use exactly one of ref or path. Guardrails and policies fail closed on conflicts.'],
  },
  {
    key: 'llm_content_sharing',
    area: 'Pipeline YAML',
    description: 'Boolean control for whether the agent automatically shares workspace file content in LLM goal context. Defaults to false when omitted; bounded workspace tools can still retrieve current files on demand with identity metadata for stale-write checks.',
    example: 'true',
  },
  {
    key: 'llm_content_include',
    area: 'Pipeline YAML',
    description: 'Path filters limiting which workspace files may be shared with LLM goal context.',
    example: '[services/api/**, Dockerfile]',
  },
  {
    key: 'llm_content_ignore',
    area: 'Pipeline YAML',
    description: 'Path filters excluding workspace files from LLM goal context.',
    example: '[.git, "**/*.pem"]',
  },
  {
    key: 'llm_output_sharing',
    area: 'Pipeline YAML',
    description: 'Default for whether task output is written into execution history for later LLM tasks.',
    example: 'false',
  },
  {
    key: 'output',
    area: 'Pipeline YAML',
    description: 'Post-run final deliverables and dashboard publications generated from completed run context.',
    example: 'items: [{ name: summary, type: markdown, when: success }]',
    type: 'object',
    required: false,
    scope: 'pipeline',
    constraints: ['Invalid when llm_enabled is false.'],
  },
];

const stepTaskRows: WikiConfigRow[] = [
  {
    key: 'steps[].name',
    area: 'Step YAML',
    description: 'Required unique step name. Step dependencies refer to this name.',
    example: 'test',
  },
  {
    key: 'steps[].image',
    area: 'Step YAML',
    description: 'Step-specific container image. Overrides container_image for executable work in that step.',
    example: 'golang:1.24',
  },
  {
    key: 'steps[].depends_on',
    area: 'Step YAML',
    description: 'Step dependencies. Values must name other steps; tasks cannot depend on other steps directly.',
    example: '[build, scan]',
  },
  {
    key: 'steps[].condition',
    area: 'Step YAML',
    description: 'Natural-language LLM condition evaluated before the step runs. False normally skips; with guardrail/policy context it fails closed.',
    example: 'Run only when the latest tag is newer than 2.0.0',
  },
  {
    key: 'steps[].secrets',
    area: 'Step YAML',
    description: 'Secret refs injected as environment variables for the step. Bare names use the run scope; scope:name uses an explicit scope.',
    example: '[DEPLOY_TOKEN, prod:REGISTRY_PASSWORD]',
  },
  {
    key: 'steps[].variables',
    area: 'Step YAML',
    description: 'Inline environment variable overrides applied to all tasks in the step after inherited run variables.',
    example: '{ API_VERSION: "canary" }',
  },
  {
    key: 'steps[].volumes',
    area: 'Step YAML',
    description: 'Named Docker volume mounts using volume:mount-path syntax for Docker execution.',
    example: '[cache:/cache]',
  },
  {
    key: 'steps[].ignore_failure',
    area: 'Step YAML',
    description: 'Marks failures in this step as ignored so downstream dependencies can continue. Approval and blocking policy or guardrail failures still fail closed.',
    example: 'true',
  },
  {
    key: 'steps[].llm_output_sharing',
    area: 'Step YAML',
    description: 'Step-level default for hiding or sharing task output in later LLM execution history.',
    example: 'false',
  },
  {
    key: 'steps[].agent_profile',
    area: 'Step YAML',
    description: 'Step persona override used by the step condition and LLM goals in that step. Tasks cannot override agent_profile.',
    example: 'sre',
  },
  {
    key: 'steps[].llm_profile',
    area: 'Step YAML',
    description: 'Step model-profile override. It selects the LLM provider/model for the step condition and tasks unless a task defines its own llm_profile.',
    example: 'reasoning',
    type: 'string',
    required: 'conditional',
    scope: 'step',
    inheritedFrom: ['pipeline llm_profile', 'configured default LLM profile'],
    permission: 'llm_profile.use',
  },
  {
    key: 'steps[].mcp_profiles',
    area: 'Step YAML',
    description: 'Approved MCP profiles added for LLM goal tasks in the step. Invalid on script-only and include steps.',
    example: '[github-readonly]',
  },
  {
    key: 'steps[].runtime_pool',
    area: 'Step YAML',
    description: 'Kubernetes runtime pool override for this step. It falls back to pipeline runtime_pool and then the runner default.',
    example: 'gpu',
  },
  {
    key: 'steps[].knowledge_context',
    area: 'Step YAML',
    description: 'Step-level knowledge references merged with pipeline and task references for LLM work.',
    example: 'kind: runbook, ref: platform/deploy-api',
  },
  {
    key: 'steps[].goal',
    area: 'Step mode',
    description: 'Single-task LLM goal step. Mutually exclusive with include, tasks, script, and approval.',
    example: 'Review release readiness and summarize risks.',
  },
  {
    key: 'steps[].script',
    area: 'Step mode',
    description: 'Single-task shell script step executed directly. Mutually exclusive with include, tasks, goal, and approval.',
    example: 'go test ./...',
  },
  {
    key: 'steps[].tasks',
    area: 'Step mode',
    description: 'Multi-task step. Each task must define exactly one of goal or script and may depend only on tasks in the same step.',
    example: '- name: unit-test',
  },
  {
    key: 'tasks[].name',
    area: 'Task YAML',
    description: 'Required unique task name within its step.',
    example: 'unit-test',
  },
  {
    key: 'tasks[].goal',
    area: 'Task YAML',
    description: 'LLM-backed task goal. Mutually exclusive with task script and requires LLM to be enabled.',
    example: 'Inspect risky files changed in this PR.',
  },
  {
    key: 'tasks[].script',
    area: 'Task YAML',
    description: 'Direct shell command or script for the task. It can still be guardrail-validated when strict knowledge context is active.',
    example: 'npm test -- --runInBand',
  },
  {
    key: 'tasks[].depends_on',
    area: 'Task YAML',
    description: 'Task dependencies inside the same step only.',
    example: '[install]',
  },
  {
    key: 'tasks[].ignore_failure',
    area: 'Task YAML',
    description: 'Treats this task failure as ignored, allowing dependent graph progress. The parent step flag also applies when set.',
    example: 'true',
  },
  {
    key: 'tasks[].llm_output_sharing',
    area: 'Task YAML',
    description: 'Task-specific override for whether this task output appears in later LLM history.',
    example: 'false',
  },
  {
    key: 'tasks[].llm_profile',
    area: 'Task YAML',
    description: 'Task model-profile override. It is the most specific llm_profile level and controls this task goal client.',
    example: 'fast',
    type: 'string',
    required: 'conditional',
    scope: 'task',
    inheritedFrom: ['step llm_profile', 'pipeline llm_profile', 'configured default LLM profile'],
    permission: 'llm_profile.use',
    constraints: ['Valid for LLM-backed goal tasks.'],
  },
  {
    key: 'tasks[].mcp_profiles',
    area: 'Task YAML',
    description: 'Approved MCP profiles added for this LLM goal task. Invalid on script tasks.',
    example: '[github-pr-review]',
  },
  {
    key: 'tasks[].variables',
    area: 'Task YAML',
    description: 'Task-local environment variable overrides applied after step variables.',
    example: '{ API_VERSION: "inside-task" }',
  },
  {
    key: 'tasks[].knowledge_context',
    area: 'Task YAML',
    description: 'Most-specific knowledge references merged with pipeline and step context.',
    example: 'kind: policy, path: .nopsai/docs/auth-policy.md',
  },
];

const approvalIncludeRows: WikiConfigRow[] = [
  {
    key: 'steps[].include',
    area: 'Include step',
    description: 'Reusable automation reference. Use step:<identifier> for reusable steps or pipeline:<identifier> for child pipelines.',
    example: 'step:team-1/shared/notify',
  },
  {
    key: 'steps[].sync',
    area: 'Include step',
    description: 'Child-pipeline execution mode flag on include steps. Use with pipeline includes when parent behavior should wait for child work.',
    example: 'false',
  },
  {
    key: 'steps[].approval.type',
    area: 'Approval step',
    description: 'Required approval kind. It may contain letters, numbers, underscores, dots, and hyphens.',
    example: 'production-deploy',
  },
  {
    key: 'steps[].approval.teams',
    area: 'Approval step',
    description: 'Required relative team paths whose approvers may approve or reject the checkpoint.',
    example: '[platform/prod]',
  },
  {
    key: 'steps[].approval.allow_self_approval',
    area: 'Approval step',
    description: 'Whether the original requester can approve the same checkpoint.',
    example: 'false',
  },
];

const triggerRows: WikiConfigRow[] = [
  {
    key: 'provider',
    area: 'Trigger manifest',
    description: 'Git provider for a repository trigger. GitHub App triggers use github behavior; non-GitHub triggers need a webhook_source.',
    example: 'gitlab',
  },
  {
    key: 'team',
    area: 'Trigger manifest',
    description: 'NopsAI team/application owner used for run ownership and resource inheritance.',
    example: 'platform',
  },
  {
    key: 'webhook_source',
    area: 'Trigger manifest',
    description: 'Managed Git Webhook Source ID for GitLab, Bitbucket, Gitea, or generic Git events.',
    example: 'gitlab-platform',
  },
  {
    key: 'management',
    area: 'Trigger manifest',
    description: 'Marks whether the trigger is NopsAI-managed rather than repository-file managed.',
    example: 'nopsai',
  },
  {
    key: 'triggers[].on',
    area: 'Trigger rule',
    description: 'Event name to match. all matches any event.',
    example: 'push',
  },
  {
    key: 'triggers[].branches',
    area: 'Trigger rule',
    description: 'Included branch glob patterns for push or pull_request target branches.',
    example: '[main, release/*]',
  },
  {
    key: 'triggers[].skip_branches',
    area: 'Trigger rule',
    description: 'Branch glob patterns excluded after inclusion.',
    example: '[wip/*]',
  },
  {
    key: 'triggers[].tags',
    area: 'Trigger rule',
    description: 'Tag glob patterns for push tag events.',
    example: '[v*]',
  },
  {
    key: 'triggers[].skip_repos',
    area: 'Trigger rule',
    description: 'Repository name patterns that prevent this rule from matching.',
    example: '[archive/*]',
  },
  {
    key: 'triggers[].include_paths',
    area: 'Trigger rule',
    description: 'Changed-file glob patterns that must match after exclusions when changed-file data is known.',
    example: '[services/api/**]',
  },
  {
    key: 'triggers[].exclude_paths',
    area: 'Trigger rule',
    description: 'Changed-file glob patterns ignored before include matching.',
    example: '[docs/**, "**/*.md"]',
  },
  {
    key: 'triggers[].pipelines',
    area: 'Trigger rule',
    description: 'Pipeline identifiers to start when the rule matches. Scalar paths are supported.',
    example: '[platform/api-ci]',
  },
  {
    key: 'triggers[].scope',
    area: 'Trigger rule',
    description: 'Run scope used for variable, secret, runner, profile, and resource authorization resolution.',
    example: 'platform/prod',
  },
  {
    key: 'git_webhook_sources[].auth_mode',
    area: 'Git Webhook Source',
    description: 'Provider request authentication mode. Supported values are hmac, static_token, and none.',
    example: 'hmac',
  },
  {
    key: 'git_webhook_sources[].repository_allowlist',
    area: 'Git Webhook Source',
    description: 'Required owner/repository allowlist with glob support.',
    example: '[platform/api, platform/*]',
  },
];

const scheduleExternalRows: WikiConfigRow[] = [
  {
    key: 'schedules[].schedule_kind',
    area: 'Schedule',
    description: 'Schedule type. Supported values are cron and once; once may be inferred when run_at is set.',
    example: 'cron',
  },
  {
    key: 'schedules[].cron_expression',
    area: 'Schedule',
    description: 'Cron expression required for cron schedules. cron is accepted as an API alias.',
    example: '0 2 * * *',
  },
  {
    key: 'schedules[].run_at',
    area: 'Schedule',
    description: 'One-time schedule timestamp. Enabled one-time schedules must be in the future.',
    example: '2026-08-01T10:00:00Z',
  },
  {
    key: 'schedules[].timezone',
    area: 'Schedule',
    description: 'IANA timezone used to interpret cron and run_at values. Defaults to UTC.',
    example: 'Europe/Amsterdam',
  },
  {
    key: 'schedules[].pipeline',
    area: 'Schedule',
    description: 'Stored pipeline identifier to run.',
    example: 'team-1/services/api/deploy',
  },
  {
    key: 'schedules[].scope',
    area: 'Schedule',
    description: 'Runtime scope used by the scheduled run.',
    example: 'prod',
  },
  {
    key: 'schedules[].run_team_path',
    area: 'Schedule',
    description: 'Pipeline Runs team path and notification lineage for runs created by this schedule. root means unassigned root.',
    example: 'team-1',
  },
  {
    key: 'schedules[].variables',
    area: 'Schedule',
    description: 'Run variable overrides supplied when the schedule starts the pipeline.',
    example: '{ RELEASE_CHANNEL: nightly }',
  },
  {
    key: 'external_triggers[].allowed_callers',
    area: 'External Trigger',
    description: 'Required explicit callers. Types are user, service_account, or auth_team; team is normalized to auth_team.',
    example: '[{ type: service_account, id: servicenow-prod }]',
  },
  {
    key: 'external_triggers[].variable_mapping',
    area: 'External Trigger',
    description: 'Maps event_type, payload paths, variables paths, or literal: values into run variables.',
    example: '{ CHANGE_ID: payload.change.id }',
  },
  {
    key: 'external_triggers[].payload_schema',
    area: 'External Trigger',
    description: 'Small object-schema guard with required fields and basic property type checks.',
    example: '{ type: object, required: [version] }',
  },
  {
    key: 'external_triggers[].rate_limit.per_minute',
    area: 'External Trigger',
    description: 'Per-trigger invocation limit over the previous minute.',
    example: '10',
  },
  {
    key: 'external_triggers.invoke.idempotency_key',
    area: 'External Trigger API',
    description: 'Optional retry key scoped by trigger and caller. Reuse for the same source event; change for a new event.',
    example: 'servicenow:CHG001',
  },
];

const llmProfileRows: WikiConfigRow[] = [
  {
    key: 'setting/system/llm_profile.yaml',
    area: 'GitOps',
    description: 'System LLM profile registry path in a global config repository.',
    example: 'setting/system/llm_profile.yaml',
  },
  {
    key: 'default_profile',
    area: 'LLM profile registry',
    description: 'Fallback profile name when pipeline, step, task, output, and conversation settings do not override it.',
    example: 'standard',
  },
  {
    key: 'profiles[].name',
    area: 'LLM profile registry',
    description: 'Profile ID referenced by llm_profile in pipelines, steps, tasks, outputs, and assistant conversations.',
    example: 'reasoning',
  },
  {
    key: 'profiles[].provider',
    area: 'LLM profile',
    description: 'Provider adapter. Supported values are gemini, lmstudio, openai, anthropic, groq, mistral, ollama, openrouter, and azure-openai.',
    example: 'openai',
  },
  {
    key: 'profiles[].model',
    area: 'LLM profile',
    description: 'Provider model or deployment name. LM Studio may discover a loaded model when omitted.',
    example: 'gpt-4.1-mini',
  },
  {
    key: 'profiles[].base_url',
    area: 'LLM profile',
    description: 'Custom endpoint. Required for LM Studio, Ollama, and Azure OpenAI; hosted providers use defaults when omitted.',
    example: 'http://ollama:11434/v1',
  },
  {
    key: 'profiles[].credential_ref',
    area: 'LLM profile',
    description: 'Credential registry reference for provider API keys. Hosted providers require it.',
    example: 'credential://system/llm/openai-hosted',
  },
  {
    key: 'profiles[].allowed_scopes',
    area: 'LLM profile',
    description: 'Optional scope allowlist. Empty means the profile can run in every scope.',
    example: '[dev, prod]',
  },
  {
    key: 'profiles[].reasoning',
    area: 'LLM profile',
    description: 'LM Studio reasoning level. Supported values are off, low, medium, high, and on. Leave empty for models that do not expose reasoning configuration.',
    example: 'high',
  },
  {
    key: 'profiles[].thinking',
    area: 'LLM profile',
    description: 'LM Studio shortcut used only when reasoning is omitted. true maps to on; false maps to off, which is omitted from LM Studio requests.',
    example: 'false',
  },
  {
    key: 'profiles[].timeout_seconds',
    area: 'LLM profile',
    description: 'Client-side HTTP timeout for provider requests.',
    example: '60',
  },
  {
    key: 'profiles[].max_tokens',
    area: 'LLM profile',
    description: 'Completion token limit translated to each provider adapter wire format.',
    example: '4096',
  },
  {
    key: 'profiles[].temperature',
    area: 'LLM profile',
    description: 'Optional sampling temperature. When omitted, provider or model defaults apply.',
    example: '0.2',
  },
  {
    key: 'profiles[].prompt_cache.mode',
    area: 'LLM profile',
    description: 'Prompt cache preference: auto, required, or disabled. Required fails closed if the selected provider adapter cannot support it.',
    example: 'auto',
  },
  {
    key: 'profiles[].provider_state.mode',
    area: 'LLM profile',
    description: 'Provider conversation-state preference: auto, required, or disabled. NopsAI logical sessions remain the audit identity even when provider state is unavailable.',
    example: 'disabled',
  },
  {
    key: 'profiles[].extra',
    area: 'LLM profile',
    description: 'Provider-specific string options such as OpenRouter http_referer or Azure deployment/api_version.',
    example: '{ x_title: NopsAI }',
  },
];

const mcpRows: WikiConfigRow[] = [
  {
    key: 'setting/system/mcp.yaml',
    area: 'GitOps',
    description: 'System MCP server and profile registry path in a global config repository.',
    example: 'setting/system/mcp.yaml',
  },
  {
    key: 'mcp_servers[].transport',
    area: 'MCP server',
    description: 'External MCP transport. streamable_http is the current default; http/https normalize to http.',
    example: 'streamable_http',
  },
  {
    key: 'mcp_servers[].auth_type',
    area: 'MCP server',
    description: 'Server authentication mode. Supported values are none and bearer_token.',
    example: 'bearer_token',
  },
  {
    key: 'mcp_servers[].headers',
    area: 'MCP server',
    description: 'Static headers sent to the MCP server after whitespace normalization.',
    example: '{ X-Workspace: platform }',
  },
  {
    key: 'mcp_servers[].timeout',
    area: 'MCP server',
    description: 'Per-server timeout. Defaults to 30s when omitted.',
    example: '30s',
  },
  {
    key: 'mcp_profiles[].servers[].tools',
    area: 'MCP profile',
    description: 'Tool allowlist for a server reference. Use * only when the full approved server toolset is acceptable.',
    example: '["*"]',
  },
  {
    key: 'mcp_profiles[].allowed_scopes',
    area: 'MCP profile',
    description: 'Optional runtime scopes where the profile can be used.',
    example: '[dev]',
  },
];

const knowledgeRows: WikiConfigRow[] = [
  {
    key: 'knowledge/',
    area: 'GitOps',
    description: 'Managed Knowledge Context directory grouped by kind and team.',
    example: 'knowledge/guardrail/security/repo-check.md',
  },
  {
    key: 'knowledge_context[].kind',
    area: 'Knowledge Context',
    description: 'Required kind. Supported values are architecture, guardrail, policy, adr, guideline, runbook, reference, and example.',
    example: 'guardrail',
  },
  {
    key: 'knowledge_context[].ref',
    area: 'Knowledge Context',
    description: 'Managed NopsAI document reference using team/document format. Mutually exclusive with path.',
    example: 'security/repo-check',
  },
  {
    key: 'knowledge_context[].path',
    area: 'Knowledge Context',
    description: 'Repo-local markdown path loaded from the run repository at the run commit. Mutually exclusive with ref.',
    example: '.nopsai/docs/backend.md',
  },
  {
    key: 'knowledge_context[].required',
    area: 'Knowledge Context',
    description: 'When true, resolution or authorization failure stops the run before execution.',
    example: 'true',
  },
  {
    key: 'knowledge/<kind>/<team>/<name>.md',
    area: 'Knowledge GitOps',
    description: 'Managed knowledge document path. GitOps documents must provide reusable text through content.',
    example: 'knowledge/policy/platform/release-evidence.md',
  },
];

const finalOutputRows: WikiConfigRow[] = [
  {
    key: 'output.llm_profile',
    area: 'Final output YAML',
    description: 'Default model profile for all final outputs. Item llm_profile overrides it.',
    example: 'report-writer',
    type: 'string',
    required: 'conditional',
    scope: 'pipeline output',
    inheritedFrom: ['pipeline llm_profile', 'configured default LLM profile'],
    permission: 'llm_profile.use',
  },
  {
    key: 'output.items[].name',
    area: 'Final output YAML',
    description: 'Required unique output name inside the pipeline.',
    example: 'Executive summary',
  },
  {
    key: 'output.items[].type',
    area: 'Final output YAML',
    description: 'Output type. Supported values are markdown, pdf, excel, json, html, and dashboard.',
    example: 'pdf',
    type: 'enum',
    required: true,
    allowedValues: ['markdown', 'pdf', 'excel', 'json', 'html', 'dashboard'],
    scope: 'output item',
  },
  {
    key: 'output.items[].when',
    area: 'Final output YAML',
    description: 'Generation condition. Supported values are success, failure, and always; empty behaves as always.',
    example: 'always',
    type: 'enum',
    required: false,
    defaultValue: 'always',
    allowedValues: ['success', 'failure', 'always'],
    scope: 'output item',
  },
  {
    key: 'output.items[].prompt',
    area: 'Final output YAML',
    description: 'Required prompt describing the deliverable to generate from completed run context. Dashboard prompts describe dashboard intent; emitted step output is authoritative for business facts, and NopsAI chooses a suitable structured presentation when no visualization is specified.',
    example: 'Show service metrics from the run evidence and choose the best table, text, bar, or trend view.',
  },
  {
    key: 'output.items[].llm_profile',
    area: 'Final output YAML',
    description: 'Item-specific model profile for this output.',
    example: 'fast-report',
  },
  {
    key: 'output.items[].dashboard.ref',
    area: 'Dashboard output',
    description: 'Required for dashboard outputs. Uses team/dashboard-slug format.',
    example: 'platform/engineering-health',
  },
  {
    key: 'output.items[].dashboard.section',
    area: 'Dashboard output',
    description: 'Required section key where the publication appears.',
    example: 'deployments',
  },
  {
    key: 'output.items[].dashboard.entry_key',
    area: 'Dashboard output',
    description: 'Optional stable entry key used by replace and series publications. When omitted, the output name is used.',
    example: 'payments-api',
  },
  {
    key: 'output.items[].dashboard.mode',
    area: 'Dashboard output',
    description: 'Optional publication mode. Supported values are replace, append, snapshot, and series; empty defaults to replace.',
    example: 'replace',
    type: 'enum',
    required: false,
    defaultValue: 'replace',
    allowedValues: ['replace', 'append', 'snapshot', 'series'],
    scope: 'dashboard output item',
  },
  {
    key: 'output.items[].dashboard.preset',
    area: 'Dashboard output',
    description: 'Optional prompt preset hint. Supported values include auto, report, table, status, timeline, comparison, metrics, and mixed.',
    example: 'metrics',
    type: 'enum',
    required: false,
    allowedValues: ['auto', 'report', 'table', 'status', 'timeline', 'comparison', 'metrics', 'mixed'],
    scope: 'dashboard output item',
  },
  {
    key: 'output.items[].dashboard.ttl',
    area: 'Dashboard output',
    description: 'Optional staleness duration for dashboard content.',
    example: '7d',
    type: 'duration',
    required: false,
    scope: 'dashboard output item',
    constraints: ['Accepts Go durations and day shorthand such as 7d up to the platform maximum retention.'],
  },
];

const requiredEnvironmentRows: WikiConfigRow[] = [
  {
    key: 'DATABASE_URL',
    area: 'Bootstrap',
    description: 'PostgreSQL connection string for runtime state, config sync state, auth records, run records, logs, and evidence.',
    example: 'postgres://nopsai:***@db:5432/nopsai?sslmode=disable',
    type: 'secret URL',
    required: true,
    scope: 'API and AAA',
    security: 'Store in deployment secret management or the Helm bootstrap Secret, not in GitOps files.',
  },
  {
    key: 'NOPSAI_MASTER_KEY',
    area: 'Bootstrap',
    description: 'Root application secret used for encrypted platform data.',
    example: 'openssl rand -base64 32',
    type: 'secret',
    required: true,
    scope: 'API',
    security: 'Production gates require a non-default high-entropy value.',
  },
  {
    key: 'JWT_SIGNING_KEY',
    area: 'Bootstrap',
    description: 'Signs user, personal-token, service-account-token, browser, CLI, automation, and hosted MCP bearer tokens.',
    example: 'openssl rand -base64 48',
    type: 'secret',
    required: true,
    scope: 'API',
    security: 'Keep separate from SERVICE_JWT_SIGNING_KEY.',
  },
  {
    key: 'NOPSAI_BOOTSTRAP_ADMIN_EMAIL',
    area: 'Bootstrap',
    description: 'Email for the first local administrator created during API bootstrap.',
    example: 'platform-admin@example.com',
    type: 'email',
    required: true,
    scope: 'API',
    security: 'Use a real operator mailbox for shared environments.',
  },
  {
    key: 'NOPSAI_BOOTSTRAP_ADMIN_PASSWORD',
    area: 'Bootstrap',
    description: 'Initial password used to create or rotate the local bootstrap administrator.',
    example: 'openssl rand -base64 24',
    type: 'secret',
    required: true,
    scope: 'API',
    security: 'Use NOPSAI_BOOTSTRAP_ADMIN_PASSWORD_FILE when a secret manager mounts the value as a file.',
  },
  {
    key: 'SERVICE_JWT_SIGNING_KEY',
    area: 'Bootstrap',
    description: 'Signs internal service JWTs for nopsai, dispatcher, git-bot, runner, and agent callbacks.',
    example: 'openssl rand -base64 48',
    type: 'secret',
    required: true,
    scope: 'API, dispatcher, git-bot, runners, agents',
    security: 'Use an independent secret from user/API token signing.',
  },
  {
    key: 'AAA_SHARED_INTERNAL_TOKEN',
    area: 'Bootstrap',
    description: 'Shared internal token used when nopsai asks AAA for route and runtime resource-use authorization checks.',
    example: 'openssl rand -base64 32',
    type: 'secret',
    required: true,
    scope: 'API and AAA',
    security: 'Treat as service-to-service authentication material.',
  },
  {
    key: 'AAA_API_URL',
    area: 'Service discovery',
    description: 'Internal URL for the AAA service.',
    example: 'http://aaa:8082',
    type: 'URL',
    required: true,
    scope: 'API',
    constraints: ['Usually a private service DNS name in Compose or Kubernetes.'],
  },
  {
    key: 'NOPSAI_API_URL',
    area: 'Service discovery',
    description: 'Internal API URL used for dispatcher status/log callbacks and git-bot event forwarding.',
    example: 'http://nopsai:8080',
    type: 'URL',
    required: true,
    scope: 'dispatcher and git-bot',
    constraints: ['Use an internal route for service callbacks instead of the public UI URL.'],
  },
  {
    key: 'GIT_BOT_API_URL',
    area: 'Service discovery',
    description: 'Internal git-bot URL used by the API for repository contents, review-branch writes, GitHub events, and check-run updates.',
    example: 'http://nopsai-git-bot:8081',
    type: 'URL',
    required: 'conditional',
    scope: 'API',
    constraints: ['Required when GitHub App or GitOps repository operations are enabled.'],
  },
  {
    key: 'DISPATCHER_GRPC_ADDRESS',
    area: 'Dispatcher',
    description: 'gRPC address used by runners and agents to reach the dispatcher.',
    example: 'dispatcher:9090',
    type: 'host:port',
    required: true,
    scope: 'runners and agents',
    inheritedFrom: ['setting/system/runner.yaml can also publish dispatcher defaults'],
  },
  {
    key: 'DISPATCHER_TLS_MODE',
    area: 'Dispatcher',
    description: 'Transport security mode for dispatcher gRPC.',
    example: 'mtls',
    type: 'enum',
    required: 'conditional',
    defaultValue: 'mtls',
    allowedValues: ['mtls', 'tls', 'disabled'],
    scope: 'dispatcher, runners, agents',
    security: 'Production gates expect tls or mtls, not disabled transport.',
  },
  {
    key: 'DISPATCHER_TLS_SECRET',
    area: 'Dispatcher',
    description: 'Shared high-entropy bootstrap secret used by dispatcher clients when TLS or mTLS is configured. Generated Docker Compose installs include it in .env.',
    example: 'openssl rand -base64 32',
    type: 'secret',
    required: 'conditional',
    scope: 'dispatcher, runners, agents',
    security: 'Rotate as part of dispatcher trust seed rotation.',
  },
  {
    key: 'DOCKER_NETWORK_NAME',
    area: 'Docker runner',
    description: 'Docker network used when the Docker runner starts per-run agent and step containers.',
    example: 'nopsai-net',
    type: 'string',
    required: true,
    defaultValue: 'nopsai-net',
    scope: 'Docker runner',
  },
  {
    key: 'SYSTEM_LOGS_DOCKER_HOST',
    area: 'System Logs',
    description: 'Read-only Docker API endpoint used for allow-listed System Logs sources in Docker deployments.',
    example: 'tcp://docker-socket-proxy:2375',
    type: 'URL',
    required: 'conditional',
    scope: 'API',
    security: 'Use the restricted docker-socket-proxy, not a broad Docker daemon endpoint.',
  },
  {
    key: 'FINAL_OUTPUT_PDF_RENDERER_URL',
    area: 'Final outputs',
    description: 'Gotenberg URL used when final outputs render PDFs.',
    example: 'http://gotenberg:3000',
    type: 'URL',
    required: 'conditional',
    scope: 'API',
    constraints: ['Required for PDF final outputs.'],
  },
  {
    key: 'FINAL_OUTPUT_PDF_TIMEOUT_SECONDS',
    area: 'Final outputs',
    description: 'Server-side timeout for PDF rendering through Gotenberg.',
    example: '60',
    type: 'integer',
    required: false,
    defaultValue: '60',
    scope: 'API',
  },
];

const gitOpsRepositoryRows: WikiConfigRow[] = [
  {
    key: 'pipelines/',
    area: 'GitOps repository',
    description: 'Pipeline definitions grouped by team, service, or application path.',
    example: 'pipelines/team-1/services/api/deploy.yaml',
    type: 'directory',
    required: false,
    scope: 'config repository',
  },
  {
    key: 'steps/',
    area: 'GitOps repository',
    description: 'Reusable step definitions referenced through include steps.',
    example: 'steps/shared/notify.yaml',
    type: 'directory',
    required: false,
    scope: 'config repository',
    permission: 'step.use',
  },
  {
    key: 'schedules/',
    area: 'GitOps repository',
    description: 'One-time and recurring pipeline schedules.',
    example: 'schedules/prod/nightly-api-deploy.yaml',
    type: 'directory',
    required: false,
    scope: 'config repository',
  },
  {
    key: 'triggers/',
    area: 'GitOps repository',
    description: 'Repository trigger manifests for GitHub App and Git Webhook Source events.',
    example: 'triggers/platform/service-api.yaml',
    type: 'directory',
    required: false,
    scope: 'config repository',
  },
  {
    key: 'external-triggers/',
    area: 'GitOps repository',
    description: 'Authenticated external trigger endpoint definitions with caller allowlists, schemas, mappings, and rate limits.',
    example: 'external-triggers/deploy-prod.yaml',
    type: 'directory',
    required: false,
    scope: 'config repository',
  },
  {
    key: 'git-webhook-sources/',
    area: 'GitOps repository',
    description: 'GitLab, Bitbucket, Gitea, and generic Git event source definitions.',
    example: 'git-webhook-sources/gitlab-platform.yaml',
    type: 'directory',
    required: false,
    scope: 'config repository',
  },
  {
    key: 'scopes/',
    area: 'GitOps repository',
    description: 'Scope variables, GitOps secret keys, runtime defaults, and repository-specific values.',
    example: 'scopes/prod/scope.yaml',
    type: 'directory',
    required: false,
    scope: 'config repository',
  },
  {
    key: 'knowledge/',
    area: 'GitOps repository',
    description: 'Managed Knowledge Context documents organized by kind and team.',
    example: 'knowledge/policy/platform/release-evidence.md',
    type: 'directory',
    required: false,
    scope: 'config repository',
    permission: 'knowledge_context.use',
  },
  {
    key: 'config-repositories/',
    area: 'GitOps repository',
    description: 'Group config repository bindings, group structure, and notification ownership.',
    example: 'config-repositories/groups/team-1/structure.yaml',
    type: 'directory',
    required: false,
    scope: 'system config repository',
  },
  {
    key: 'access/',
    area: 'GitOps repository',
    description: 'Users, service accounts, roles, policies, bindings, and product grants.',
    example: 'access/all.yaml',
    type: 'directory',
    required: false,
    scope: 'config repository',
    security: 'Raw generated service-account tokens are not stored in Git.',
  },
  {
    key: 'setting/',
    area: 'GitOps repository',
    description: 'System settings for auth, mail, Agent Profiles, LLM, MCP, runner, GitHub App, and credentials.',
    example: 'setting/system/runner.yaml',
    type: 'directory',
    required: false,
    scope: 'system config repository',
  },
];

const baseWikiSections: WikiSectionInput[] = [
  {
    id: 'architecture',
    title: 'Product and Architecture',
    owner: 'Platform engineering',
    description: 'What the product is, how the control plane and execution plane fit together, and how a run moves through the system.',
    articles: [
      {
        id: 'what-nopsai-is',
        title: 'What NopsAI Is',
        level: 'Start',
        audience: 'New users, buyers, architects, and automation owners',
        summary:
          'NopsAI is a self-hosted automation control plane for executing YAML-defined workflows through Docker or Kubernetes runners.',
        keyFacts: [
          'Pipelines can combine deterministic shell scripts, AI goals, reusable steps, child pipelines, and human approval gates.',
          'Runtime variables, encrypted secrets, governed knowledge, LLM profiles, Agent Profiles, and MCP profiles are resolved before execution.',
          'Runs may be started manually, by schedules, by GitHub App events, by Git webhook sources, by external triggers, or by parent pipelines.',
          'Generated final outputs can be Markdown, JSON, HTML, PDF, or Excel and are stored separately from raw task logs.',
        ],
        details: [
          'The product is built around GitOps-friendly configuration but keeps durable execution, audit, credential, monitoring, and setup state in PostgreSQL.',
          'LLM-backed tasks run inside the per-run agent. There is no separate always-on LLM service in the current repository shape.',
          'The UI, CLI, REST API, hosted MCP surface, and runners all go through the same authentication, authorization, and audit boundaries.',
        ],
        configRows: [],
        examples: [],
        relatedDocs: ['doc/architecture-overview.md', 'doc/feature-reference.md', 'doc/runtime-flows.md'],
        runbooks: ['Map an existing manual operation to a NopsAI pipeline', 'Review GitOps ownership before onboarding a team'],
        caveats: [
          'The current repository does not define cloud-specific infrastructure modules. Cloud installs should treat NopsAI as a portable Kubernetes workload.',
        ],
      },
      {
        id: 'control-execution-plane',
        title: 'Control Plane and Execution Plane',
        level: 'Reference',
        audience: 'Platform engineers and operators',
        summary:
          'The API, AAA service, dispatcher, git-bot, UI, PostgreSQL, Gotenberg, and socket proxy form the control plane; Docker and Kubernetes runners with per-run agents form the execution plane.',
        keyFacts: [
          '`nopsai-api` owns REST APIs, validation, orchestration, setup, monitoring, credentials, notifications, GitOps, auth integration, and run records.',
          '`aaa` owns authorization decisions, policy checks, ACL expansion, filtering, and decision audit records.',
          '`dispatcher` owns runner registration, queueing, routing, capacity selection, and job assignment over gRPC.',
          '`git-bot` owns GitHub App webhooks, repository access, check-runs, and GitHub-specific integration.',
          'Runners start one agent for each assigned run; the agent then starts step containers or pods and reports status back.',
        ],
        details: [
          'The API submits jobs to the dispatcher. Runners maintain long-lived outbound connections to the dispatcher, which keeps runner registration and capacity visible to the control plane.',
          'Docker runners create containers and Docker volumes. Kubernetes runners create an agent pod, PVC-backed workspace, and step pods in their namespace.',
          'Gotenberg is used for PDF rendering. The Docker socket proxy is restricted to allow-listed System Logs reads and is not used for runner execution.',
          'UI and CLI are entry points only. They call authenticated REST routes and do not talk directly to AAA, dispatcher, PostgreSQL, or runners.',
          'services/nopsai owns auth, config sync, Git event ingress, run creation, setup preflight, logs, outputs, metrics, and hosted MCP; it calls Postgres, AAA, git-bot, dispatcher, Gotenberg, and log providers.',
          'services/aaa owns route authorization, product grants, groups, roles, policies, bindings, deny-before-allow decisions, filtering, ACL expansion, and decision audit data.',
          'services/git-bot owns GitHub App credentials, webhook HMAC verification, repository contents, review-branch writes, installation repositories, and check-run updates.',
          'services/dispatcher owns job queueing, runner registration, scope and capacity routing, the gRPC runner bridge, log/status ingestion, and finalization callbacks.',
          'services/agent owns one run orchestration: YAML parsing, task dependency scheduling, LLM calls, MCP profile use, approval checkpointing, child pipelines, step container or pod execution, and final task state.',
        ],
        configRows: [
          {
            key: 'DISPATCHER_GRPC_ADDRESS',
            area: 'Dispatcher',
            description: 'Address used by API, runners, and agents to reach dispatcher gRPC.',
            example: 'dispatcher:9090',
          },
          {
            key: 'GIT_BOT_API_URL',
            area: 'GitHub integration',
            description: 'Internal URL used by the API to reach git-bot.',
            example: 'http://nopsai-git-bot:8081',
          },
          {
            key: 'AAA_API_URL',
            area: 'Authorization',
            description: 'Internal URL used by the API for AAA checks and introspection.',
            example: 'http://aaa:8082',
          },
        ],
        examples: [],
        relatedDocs: ['doc/service-reference.md', 'doc/runtime-flows.md', 'doc/system-logs.md'],
        runbooks: ['Verify service-to-service URLs after a deployment change', 'Check dispatcher and runner health before onboarding workloads'],
        caveats: ['Service bind addresses and client dial addresses are separate; services usually listen on 0.0.0.0 while peers use DNS names.'],
      },
      {
        id: 'normal-run-lifecycle',
        title: 'Normal Run Lifecycle',
        level: 'Reference',
        audience: 'Automation authors, SREs, and support teams',
        summary:
          'A run is authenticated, authorized, validated, dispatched to an eligible runner, executed by an agent, finalized, monitored, audited, and optionally reported back to Git.',
        keyFacts: [
          'Manual, scheduled, Git, external-trigger, and child-pipeline entry points all converge into durable run records.',
          'Required scopes, variables, secrets, profiles, reusable steps, child pipelines, and knowledge documents are resolved before dispatch.',
          'Approvals pause and checkpoint the run without retaining runner capacity.',
          'Final output generation happens after execution status is known and records generation, contract, and render metrics.',
        ],
        details: [
          'GitHub events enter git-bot /webhook, where HMAC validation happens before normalized events are forwarded to nopsai /v1/git/events and GitHub check-runs are queued.',
          'Manual, UI, CLI, and API runs enter nopsai through POST /v1/run or POST /v1/run/{pipelineName...}; the request keeps the original caller as the authorization subject.',
          'nopsai validates bearer tokens, maps routes to actions and resources, calls AAA, and then performs runtime use checks for pipeline, scope, step, secret, variable, runner, and knowledge access.',
          'Config sync resolves files from pipelines, steps, triggers, scopes, knowledge, access, and setting/system. Each run snapshots the relevant YAML, secrets, variables, Git metadata, and approval state.',
          'The dispatcher selects a runner by availability, allowed scope, routing policy, runtime pool, affinity, capacity, and load.',
          'Docker runner creates a vol-<run_id> workspace and starts an agent container. Kubernetes runner creates an agent pod with namespace, service account, storage, affinity, and runtime pool settings.',
          'The agent executes dependency-ready tasks, so independent work can run concurrently where the schema allows it. It runs scripts, asks approved LLM profiles for goals, uses approved MCP profiles, pauses for approvals, and starts child pipelines.',
          'Logs, task status, final-output counts, and completion state stream through dispatcher back to nopsai; GitHub checks and notifications are updated after the run state changes.',
        ],
        configRows: [
          {
            key: 'SERVICE_JWT_SIGNING_KEY',
            area: 'Internal auth',
            description: 'Signs service tokens for dispatcher, runner, agent, API, and git-bot callbacks.',
            example: 'openssl rand -base64 48',
          },
        ],
        examples: [],
        relatedDocs: ['doc/runtime-flows.md', 'doc/final-output-rendering.md'],
        runbooks: ['Trace a stuck run from API record to dispatcher queue to runner logs'],
        caveats: ['A queued run normally means no eligible runner is currently available for the requested scope or route.'],
      },
    ],
  },
  {
    id: 'getting-started',
    title: 'Getting Started',
    owner: 'Developer experience',
    description: 'A first-run journey from local installation through a working script pipeline, AI goal, Git trigger, approval, and final deliverable.',
    articles: [
      {
        id: 'install-local-docker-compose',
        title: 'Install Locally with Docker Compose',
        level: 'Start',
        docType: 'tutorial',
        audiences: ['new-user', 'developer', 'administrator'],
        audience: 'New users, developers, and administrators evaluating NopsAI locally',
        summary:
          'Start a local NopsAI stack, confirm the control-plane services, and verify that a Docker runner can register for first-run testing.',
        keyFacts: [
          'Use the checked-in Compose file for local evaluation and development.',
          'The UI is published on http://localhost/ and the API on http://localhost:8080.',
          'The Docker runner needs Docker socket access because it creates agent and step containers.',
          'Local fallback secrets are present for development only and must be replaced outside a local workstation.',
        ],
        prerequisites: [
          { label: 'NopsAI version', value: 'Current repository checkout', verification: 'git rev-parse --short HEAD' },
          { label: 'Runtime', value: 'Docker 26+ with Compose v2', verification: 'docker compose version' },
          { label: 'Ports', value: '80, 8080, 8081, 9090, and 5432 available on the workstation' },
          { label: 'Permission', value: 'Local Docker access for the current user', verification: 'docker ps' },
        ],
        steps: [
          {
            title: 'Build and start the stack',
            description: 'Run the local Compose topology from the repository root.',
            commands: [
              {
                title: 'Start Compose',
                language: 'bash',
                code: 'docker compose up -d --build',
                complete: true,
                testedIn: DEFAULT_VERIFIED_DATE,
              },
            ],
            expectedOutput: 'Compose creates the nopsai-net network and starts PostgreSQL, API, AAA, dispatcher, git-bot, UI, Gotenberg, socket proxy, and Docker runner services.',
          },
          {
            title: 'Check service health',
            description: 'Confirm that the core services are running before opening the setup flow.',
            commands: [
              {
                title: 'Inspect local services',
                language: 'bash',
                code: 'docker compose ps\ndocker compose logs --tail=80 nopsai aaa dispatcher docker-runner',
                complete: true,
                testedIn: DEFAULT_VERIFIED_DATE,
              },
            ],
            verification: 'Open http://localhost/ and confirm the setup or login page loads.',
          },
          {
            title: 'Keep logs visible during setup',
            description: 'Follow the services that own first-run setup and runner registration while completing the wizard.',
            commands: [
              {
                title: 'Follow setup logs',
                language: 'bash',
                code: 'docker compose logs -f nopsai aaa dispatcher docker-runner',
                complete: true,
                testedIn: DEFAULT_VERIFIED_DATE,
              },
            ],
          },
        ],
        details: [
          'The Compose stack is intentionally optimized for local inspection. It is the fastest path for validating product behavior before you move to a release bundle or Helm deployment.',
          'Use the Docker Compose reference article when you need port, service, and environment-variable detail after the tutorial succeeds.',
        ],
        configRows: [
          {
            key: 'SYSTEM_LOGS_DOCKER_HOST',
            area: 'Compose',
            description: 'Docker socket proxy endpoint used by API System Logs.',
            example: 'tcp://docker-socket-proxy:2375',
            type: 'URL',
            required: true,
            scope: 'system',
            security: 'Use only the restricted socket proxy for API log reads.',
          },
        ],
        examples: [
          {
            title: 'Local startup',
            language: 'bash',
            code: 'docker compose up -d --build\ndocker compose ps',
            complete: true,
            expectedOutput: 'The nopsai, aaa, dispatcher, git-bot, ui, and docker-runner services are running.',
            testedIn: DEFAULT_VERIFIED_DATE,
            rollback: 'docker compose down',
          },
        ],
        relatedDocs: ['docker-compose.yaml', 'doc/enterprise-gates.md'],
        runbooks: ['Start local stack', 'Check runner registration'],
        caveats: ['Do not promote checked-in local fallback secrets to a shared or production environment.'],
      },
      {
        id: 'complete-first-install-wizard',
        title: 'Complete the First-Install Wizard',
        level: 'Start',
        docType: 'tutorial',
        audiences: ['new-user', 'administrator'],
        audience: 'Administrators bootstrapping an empty workspace',
        summary:
          'Use System > Setup to pass preflight checks, create bootstrap resources, and seed a first runnable GitOps-friendly workspace.',
        keyFacts: [
          'The wizard checks database, encryption, JWT, service-secret, internal URL, LLM/MCP, starter content, and runner readiness.',
          'Required gates must pass before the workspace is ready for normal use.',
          'Before completion, authenticated navigation and APIs stay locked to profile password change and setup endpoints.',
          'The login readiness view marks configured required gates with a green tick.',
          'Selected repository teams are seeded as top-level teams; setup does not add an implicit workspace parent above them, and disabling repository teams creates no synthetic team root.',
          'Optional AI and MCP seed data can be skipped and added later.',
          'Production mode does not silently accept unsafe bootstrap defaults.',
        ],
        prerequisites: [
          { label: 'Running stack', value: 'Local Compose or deployed control plane with API and UI reachable' },
          { label: 'Permission', value: 'Initial administrator or setup-capable platform administrator' },
          { label: 'Services', value: 'PostgreSQL, API, AAA, dispatcher, and at least one runner available' },
          { label: 'Verification', value: 'Open the UI and confirm the setup page renders' },
        ],
        steps: [
          {
            title: 'Open setup',
            description: 'Sign in and let the empty workspace redirect to setup.',
            verification: 'Direct URL changes still return to System > Setup until setup is completed.',
          },
          {
            title: 'Resolve required preflight checks',
            description: 'Fix missing database, master key, JWT, service token, service URL, or runner readiness items before continuing.',
            warning: 'Do not accept local fallback secrets for shared environments.',
          },
          {
            title: 'Create starter resources',
            description: 'Seed the first teams, users, scopes, starter pipeline, knowledge documents, profiles, and access bootstrap.',
            expectedOutput: 'The workspace has a first team, a first pipeline, and enough access for an administrator to run it.',
          },
          {
            title: 'Run setup validation',
            description: 'Use the generated first-run content to verify API, dispatcher, runner, and authorization wiring.',
            verification: 'A first pipeline run reaches a terminal status and is visible in Pipeline runs.',
          },
        ],
        details: [
          'The setup wizard owns bootstrap readiness. Until setup is completed once, the app shell and API block normal workspace routes so operators cannot bypass the empty-state gate. After the workspace is live, GitOps repositories and focused system pages own ongoing configuration.',
        ],
        configRows: [
          {
            key: 'NOPSAI_MASTER_KEY',
            area: 'Bootstrap',
            description: 'Required root encryption material used before credential registry access is available.',
            example: 'openssl rand -base64 32',
            type: 'secret',
            required: true,
            scope: 'system',
            security: 'Store in deployment secret management, not in GitOps application config.',
          },
          {
            key: 'JWT_SIGNING_KEY',
            area: 'Bootstrap',
            description: 'Required user/API access-token signing key.',
            example: 'openssl rand -base64 48',
            type: 'secret',
            required: true,
            scope: 'system',
            security: 'Use separate user/API and internal service signing keys.',
          },
          {
            key: 'NOPSAI_BOOTSTRAP_ADMIN_PASSWORD',
            area: 'Bootstrap',
            description: 'Initial local administrator password used when creating or rotating the bootstrap admin.',
            example: 'openssl rand -base64 24',
            type: 'secret',
            required: true,
            scope: 'system',
            security: 'Use NOPSAI_BOOTSTRAP_ADMIN_PASSWORD_FILE when your secret manager mounts files.',
          },
        ],
        examples: [],
        relatedDocs: ['doc/first-install-wizard.md', 'doc/enterprise-gates.md'],
        runbooks: ['Complete setup preflight', 'Set bootstrap administrator password', 'Run setup/first-run'],
        caveats: ['Changing bootstrap secrets usually requires restarting the affected services.'],
      },
      {
        id: 'first-script-pipeline',
        title: 'Create and Run Your First Script Pipeline',
        level: 'Start',
        docType: 'tutorial',
        audiences: ['new-user', 'automation-author', 'developer'],
        audience: 'New users and automation authors creating deterministic automation',
        summary:
          'Create a script-only pipeline that does not require an LLM profile, run it, inspect logs, and validate the terminal result.',
        keyFacts: [
          'Set llm_enabled: false when every executable unit is a script and no final outputs are configured.',
          'Each step must define exactly one execution mode.',
          'Script output is stored as pipeline logs and can be inspected from Pipeline runs.',
          'The run still uses normal pipeline, scope, runner, and secret authorization.',
        ],
        prerequisites: [
          { label: 'Workspace', value: 'Setup completed with a visible team and runner' },
          { label: 'Permission', value: 'pipeline.create or pipeline.update, plus pipeline.run for the target team' },
          { label: 'Runtime', value: 'A Docker or Kubernetes runner eligible for the selected scope' },
          { label: 'Verification', value: 'Open Pipeline runs and confirm at least one runner exists in System > Dispatcher' },
        ],
        steps: [
          {
            title: 'Create the pipeline YAML',
            description: 'Use a minimal script-only pipeline with a base image and one script step.',
            commands: [
              {
                title: 'Minimal script pipeline',
                language: 'yaml',
                code: 'name: first-script\nversion: "1.0"\nllm_enabled: false\ncontainer_image: alpine:3.20\nsteps:\n  - name: hello\n    script: |\n      echo "hello from NopsAI"\n      uname -a',
                complete: true,
                testedIn: DEFAULT_VERIFIED_DATE,
              },
            ],
          },
          {
            title: 'Save and run it',
            description: 'Create the pipeline in the UI or through GitOps, then start a manual run from the selected team and scope.',
            expectedOutput: 'The run moves from queued to running to success.',
          },
          {
            title: 'Inspect logs',
            description: 'Open the run details and confirm the step output is present.',
            verification: 'The log contains hello from NopsAI and the runner-reported system information.',
          },
        ],
        details: [
          'This tutorial validates the deterministic execution path before adding profiles, tools, knowledge, approvals, triggers, or final outputs.',
        ],
        configRows: [
          {
            key: 'llm_enabled',
            area: 'Pipeline YAML',
            description: 'Set false only for script-only pipelines. It rejects goals, conditions, MCP profile validation, and final outputs.',
            example: 'false',
            type: 'boolean',
            required: false,
            defaultValue: 'true',
            scope: 'pipeline',
            constraints: ['Requires every step and task to be script or non-LLM operational work.'],
          },
        ],
        examples: [
          {
            title: 'First script pipeline',
            language: 'yaml',
            code: 'name: first-script\nllm_enabled: false\ncontainer_image: alpine:3.20\nsteps:\n  - name: hello\n    script: echo "hello from NopsAI"',
            complete: true,
            expectedOutput: 'Run status success with the script output in logs.',
            testedIn: DEFAULT_VERIFIED_DATE,
          },
        ],
        relatedDocs: ['doc/feature-reference.md', 'doc/runtime-flows.md'],
        runbooks: ['Validate a new pipeline', 'Debug queued run'],
        caveats: ['Goal tasks, conditions, MCP profiles, and final outputs are invalid when llm_enabled is false.'],
      },
      {
        id: 'first-ai-assisted-pipeline',
        title: 'Create Your First AI-Assisted Pipeline',
        level: 'Start',
        docType: 'tutorial',
        audiences: ['automation-author', 'administrator'],
        audience: 'Automation authors adding governed LLM behavior',
        summary:
          'Register or select an LLM profile, attach an Agent Profile and Knowledge Context, and run a first LLM-backed goal after a deterministic check.',
        keyFacts: [
          'LLM Profiles select provider and model settings; Agent Profiles select persona and instructions.',
          'Knowledge Context attaches governed documents and can enforce guardrail or policy constraints.',
          'AAA decides whether the original caller may use each selected profile and knowledge document.',
          'MCP profiles are optional and additive across pipeline, step, and task levels.',
        ],
        prerequisites: [
          { label: 'LLM credential', value: 'OpenAI, Azure OpenAI, Ollama, LM Studio, or another supported provider configured in an LLM profile' },
          { label: 'Permission', value: 'pipeline.run plus use access for the selected LLM, Agent, MCP, and Knowledge resources' },
          { label: 'Network', value: 'Runner or API path can reach the selected LLM provider or local model endpoint' },
          { label: 'Verification', value: 'System LLM profile test succeeds before running the pipeline' },
        ],
        steps: [
          {
            title: 'Choose profile boundaries',
            description: 'Select the LLM profile for model access, the Agent Profile for instructions, and optional Knowledge Context for governed context.',
          },
          {
            title: 'Create the pipeline',
            description: 'Place deterministic checks before the LLM goal so the model receives useful execution evidence.',
            commands: [
              {
                title: 'First AI-assisted pipeline',
                language: 'yaml',
                code: 'name: first-ai-review\nversion: "1.0"\ncontainer_image: alpine:3.20\nllm_profile: standard\nagent_profile: devops-engineer\nknowledge_context:\n  - kind: guideline\n    ref: platform/automation-style\n    required: false\nsteps:\n  - name: collect\n    script: |\n      echo "service=checkout"\n      echo "status=ready"\n  - name: summarize\n    depends_on: [collect]\n    goal: Summarize the service readiness evidence and list any missing operational checks.',
                complete: true,
                testedIn: DEFAULT_VERIFIED_DATE,
              },
            ],
          },
          {
            title: 'Run and review the result',
            description: 'Start the run and confirm the goal task uses the configured profile without granting extra runtime permissions.',
            verification: 'Run details show the script step succeeded and the AI goal produced a constrained answer.',
          },
        ],
        details: [
          'Keep provider credentials in the credential registry. Pipeline YAML should reference approved profiles, not raw provider secrets or endpoint credentials.',
        ],
        configRows: [
          {
            key: 'llm_profile',
            area: 'Pipeline YAML',
            description: 'Pipeline-level model profile. It selects provider/model/client settings, not persona or permissions.',
            example: 'standard',
            type: 'string',
            required: 'conditional',
            scope: 'pipeline',
            permission: 'llm_profile.use',
          },
          {
            key: 'agent_profile',
            area: 'Pipeline YAML',
            description: 'Pipeline-level AI persona. Step agent_profile can override it; tasks cannot define agent_profile.',
            example: 'devops-engineer',
            type: 'string',
            required: false,
            scope: 'pipeline',
            permission: 'agent_profile.use',
          },
        ],
        examples: [],
        relatedDocs: ['doc/llm-model-selection.md', 'doc/agent-profiles.md', 'doc/knowledge-context.md'],
        runbooks: ['Add LLM profile', 'Find which profile a task uses', 'Review a new Agent Profile'],
        caveats: ['Selecting an LLM profile does not grant secrets, tools, scopes, or Knowledge Context by itself.'],
      },
      {
        id: 'connect-git-repository',
        title: 'Connect a Git Repository',
        level: 'Start',
        docType: 'tutorial',
        audiences: ['automation-author', 'administrator', 'developer'],
        audience: 'Repository owners and platform administrators',
        summary:
          'Connect repository-owned automation to a NopsAI team using GitOps content or a provider webhook source.',
        keyFacts: [
          'GitHub App events enter through git-bot; GitLab, Bitbucket, Gitea, and generic Git events enter through Git Webhook Sources.',
          'Repository triggers should be owned by a team or repository owner with explicit pipeline and scope access.',
          'Config sync is the safest authority for repeatable enterprise repository onboarding.',
          'Generic Git providers require synchronized pipelines and triggers because they do not fetch repository files directly in v1.',
        ],
        prerequisites: [
          { label: 'Repository', value: 'A Git repository that will own or trigger automation' },
          { label: 'Permission', value: 'Team owner or administrator for the target NopsAI team' },
          { label: 'Network', value: 'Public or private webhook ingress reachable by the Git provider' },
          { label: 'Credentials', value: 'GitHub App installation or Git webhook source secret depending on provider' },
        ],
        steps: [
          {
            title: 'Choose the connection mode',
            description: 'Use GitHub App when available. Use a Git Webhook Source for GitLab, Bitbucket, Gitea, or generic webhook delivery.',
          },
          {
            title: 'Bind repository ownership',
            description: 'Assign the repository to the team that owns its pipelines, triggers, notifications, and runtime resources.',
          },
          {
            title: 'Add GitOps files',
            description: 'Commit pipelines, triggers, scopes, access, and knowledge files to the owning config repository where possible.',
            expectedOutput: 'Config sync imports or updates the NopsAI resources and records source path and commit metadata.',
          },
        ],
        details: [
          'For enterprises, repository connection is an ownership problem as much as an integration problem. Tie the repository to a team before adding run triggers.',
        ],
        configRows: [
          {
            key: 'provider',
            area: 'Trigger manifest',
            description: 'Git provider for a repository trigger. GitHub App triggers use github behavior; non-GitHub triggers need a webhook_source.',
            example: 'gitlab',
            type: 'enum',
            required: true,
            allowedValues: ['github', 'gitlab', 'bitbucket', 'gitea', 'generic'],
            scope: 'trigger',
          },
          {
            key: 'webhook_source',
            area: 'Trigger manifest',
            description: 'Managed Git Webhook Source ID for GitLab, Bitbucket, Gitea, or generic Git events.',
            example: 'gitlab-platform',
            type: 'string',
            required: 'conditional',
            scope: 'trigger',
          },
        ],
        examples: [],
        relatedDocs: ['doc/triggering.md', 'doc/git-webhook-sources.md', 'doc/team-resource-ownership-design.md'],
        runbooks: ['Review repository allowlist', 'Resolve GitOps drift'],
        caveats: ['Do not use unauthenticated generic webhook ingress unless it is isolated behind trusted private controls.'],
      },
      {
        id: 'trigger-pipeline-from-git',
        title: 'Trigger a Pipeline from Git',
        level: 'Start',
        docType: 'tutorial',
        audiences: ['automation-author', 'developer', 'operator'],
        audience: 'Repository owners and automation authors validating CI-style triggers',
        summary:
          'Create a trigger manifest, match a branch and path change, start a pipeline run, and verify the Git delivery outcome.',
        keyFacts: [
          'Trigger rules match event, branch, tag, repository, and changed-path constraints.',
          'Path filters fail open when the provider does not send changed-file data.',
          'The selected pipeline and scope are still authorized before a run is created.',
          'Delivery history records accepted events, auth failures, idempotency behavior, and no-match outcomes.',
        ],
        prerequisites: [
          { label: 'Pipeline', value: 'A synchronized or database-created pipeline that can run manually' },
          { label: 'Repository connection', value: 'GitHub App or Git Webhook Source configured' },
          { label: 'Permission', value: 'trigger.create or trigger.update plus resource access for the selected pipeline and scope' },
          { label: 'Verification', value: 'Manual run of the selected pipeline succeeds before wiring the trigger' },
        ],
        steps: [
          {
            title: 'Create the trigger rule',
            description: 'Match the provider event and branch, then point the rule at one or more pipeline identifiers.',
            commands: [
              {
                title: 'Trigger manifest',
                language: 'yaml',
                code: 'provider: gitlab\nteam: platform\nwebhook_source: gitlab-platform\ntriggers:\n  - on: push\n    branches: [main]\n    include_paths:\n      - services/api/**\n    pipelines:\n      - platform/api-ci\n    scope: platform/prod',
                complete: true,
                testedIn: DEFAULT_VERIFIED_DATE,
              },
            ],
          },
          {
            title: 'Send a test event',
            description: 'Push a commit or replay a signed webhook payload that matches the rule.',
            expectedOutput: 'The trigger delivery records an accepted outcome and creates a pipeline run.',
          },
          {
            title: 'Confirm run ownership',
            description: 'Open Pipeline runs and verify team, repository, branch, commit, event ID, selected pipeline, and scope.',
          },
        ],
        details: [
          'Trigger matching is not authorization. A matched trigger still needs permission to use the pipeline, scope, secrets, variables, profiles, and knowledge selected by the run.',
        ],
        configRows: triggerRows,
        examples: [],
        relatedDocs: ['doc/triggering.md', 'doc/git-webhook-sources.md'],
        runbooks: ['Debug GitHub webhook no-run outcome', 'Replay an external trigger safely'],
        caveats: ['Changing trigger YAML in the UI can be overwritten by GitOps unless the change is pushed back to the owning repository.'],
      },
      {
        id: 'add-approval-checkpoint',
        title: 'Add an Approval Checkpoint',
        level: 'Start',
        docType: 'tutorial',
        audiences: ['automation-author', 'operator', 'administrator'],
        audience: 'Release managers and automation authors adding human control gates',
        summary:
          'Insert a durable approval step that pauses a run, releases runner capacity, and resumes from a stored checkpoint after approval.',
        keyFacts: [
          'Approval steps store execution history, completed task keys, variables, pipeline definition, and a compressed workspace archive.',
          'A waiting approval does not keep runner capacity occupied.',
          'Assigned approvers can see pending approvals even across team boundaries when they are named on the approval.',
          'Self-approval can be disabled for production changes.',
        ],
        prerequisites: [
          { label: 'Pipeline', value: 'A pipeline with a deterministic step before the gate' },
          { label: 'Permission', value: 'pipeline.update plus approval permissions for the assigned team' },
          { label: 'Team', value: 'A relative team path that owns approvers for the checkpoint' },
          { label: 'Verification', value: 'The assigned approver can see the approval queue' },
        ],
        steps: [
          {
            title: 'Insert the approval step',
            description: 'Place the approval after evidence-producing steps and before the protected action.',
            commands: [
              {
                title: 'Approval step',
                language: 'yaml',
                code: 'steps:\n  - name: build\n    script: ./scripts/build.sh\n  - name: production-gate\n    depends_on: [build]\n    approval:\n      type: production-deploy\n      teams:\n        - platform/prod\n      allow_self_approval: false\n  - name: deploy\n    depends_on: [production-gate]\n    script: ./scripts/deploy.sh',
                complete: false,
                testedIn: DEFAULT_VERIFIED_DATE,
              },
            ],
          },
          {
            title: 'Run to the checkpoint',
            description: 'Start the pipeline and confirm it enters waiting_approval after the upstream work succeeds.',
          },
          {
            title: 'Approve and verify resume',
            description: 'Approve as an assigned approver and confirm the run resumes on a fresh agent from the checkpoint.',
            expectedOutput: 'The deploy step runs only after approval and the final run state records the approval decision.',
          },
        ],
        details: [
          'Approval steps are the right boundary for production promotion, irreversible changes, and externally reviewed actions.',
        ],
        configRows: approvalIncludeRows.filter(row => row.key.startsWith('steps[].approval')),
        examples: [],
        relatedDocs: ['doc/feature-reference.md', 'doc/access-control.md'],
        runbooks: ['Approve or reject a pending gate', 'Resume an approval checkpoint', 'Audit cross-team include permissions'],
        caveats: ['Rejecting an approval marks the approval task failed and the run rejected.'],
      },
      {
        id: 'create-final-deliverable',
        title: 'Create a Final Deliverable',
        level: 'Start',
        docType: 'tutorial',
        audiences: ['automation-author', 'operator'],
        audience: 'Automation authors and stakeholders who need run outputs',
        summary:
          'Configure a post-run final output that turns completed run context into a Markdown report or a validated dashboard publication.',
        keyFacts: [
          'Final outputs run after execution status is known.',
          'Supported when values are success, failure, and always.',
          'Markdown, JSON, HTML, PDF, Excel, and dashboard output types are supported by current contracts.',
          'Dashboard outputs require a validated DashboardSpec and reject generated HTML, CSS, JavaScript, iframes, forms, and executable links.',
        ],
        prerequisites: [
          { label: 'Pipeline', value: 'A pipeline with useful completed task context' },
          { label: 'LLM profile', value: 'A final-output-capable LLM profile available to the caller' },
          { label: 'Permission', value: 'pipeline.run plus any dashboard.publish permission for dashboard outputs' },
          { label: 'Verification', value: 'The selected profile can complete a small generation request' },
        ],
        steps: [
          {
            title: 'Add an output item',
            description: 'Start with Markdown for a low-risk first deliverable.',
            commands: [
              {
                title: 'Markdown final output',
                language: 'yaml',
                code: 'output:\n  llm_profile: report-writer\n  items:\n    - name: Run summary\n      type: markdown\n      when: always\n      prompt: Summarize the run status, important evidence, and next action.',
                complete: false,
                testedIn: DEFAULT_VERIFIED_DATE,
              },
            ],
          },
          {
            title: 'Run the pipeline',
            description: 'Start a run and wait for execution and final-output generation to complete.',
            expectedOutput: 'Run details show a generated final output attached to the completed run.',
          },
          {
            title: 'Promote to dashboard when needed',
            description: 'Use type: dashboard only when the content should update a team operational view.',
            verification: 'Dashboard publication history records the source run and output item.',
          },
        ],
        details: [
          'Use a dashboard output for team-owned current operational state. Use Markdown, PDF, Excel, HTML, or JSON for run-owned deliverables that should remain attached to the run.',
        ],
        configRows: finalOutputRows,
        examples: [],
        relatedDocs: ['doc/final-output-rendering.md', 'doc/dashboards.md'],
        runbooks: ['Review final-output contract violations', 'Diagnose failed PDF render', 'Validate Excel deliverable schema'],
        caveats: ['Final-output failure is tracked separately from execution failure.'],
      },
    ],
  },
  {
    id: 'installation',
    title: 'Installation',
    owner: 'Platform engineering',
    description: 'First install, local Compose, Helm, runtime secrets, and runner installation boundaries.',
    articles: [
      {
        id: 'first-install-wizard',
        title: 'First-Install Wizard',
        level: 'Admin',
        audience: 'Administrators bootstrapping an empty workspace',
        summary:
          'The System > Setup wizard moves an empty database into a functioning workspace with runtime checks, starter teams, starter users, GitOps files, and optional AI/MCP seed data.',
        keyFacts: [
          'Preflight checks cover database reachability, master key, user JWT key, admin state, internal service secrets, service addresses, LLM/MCP readiness, starter pipeline, and runner health.',
          'The wizard can generate missing internal service secrets and output environment, secret-manager, or container snippets.',
          'Starter GitOps content includes teams, a first-run pipeline, reusable step, triggers, scopes, access bootstrap, knowledge docs, LLM profile, MCP examples, and config repository structure.',
          'Selected starter teams are mirrored as top-level team roots in direct database seeding and per-team GitOps structure files; disabling starter teams creates no synthetic team root.',
          'Production-gate mode does not silently seed an unsafe missing administrator.',
        ],
        details: [
          'Optional integrations may be skipped and configured later, but required readiness gates must be resolved before normal operation.',
          'The bootstrap admin state is reported until secured. For production, set a unique bootstrap administrator password before accepting real workload access.',
        ],
        configRows: [
          {
            key: 'DATABASE_URL',
            area: 'Bootstrap',
            description: 'PostgreSQL connection used by API and AAA.',
            example: 'postgres://nopsai_user:***@nopsai-db:5432/nopsai_db',
          },
          {
            key: 'NOPSAI_MASTER_KEY',
            area: 'Bootstrap',
            description: 'Root encryption material for pipeline secrets and credential wrapping.',
            example: 'openssl rand -base64 32',
          },
          {
            key: 'JWT_SIGNING_KEY',
            area: 'Bootstrap',
            description: 'Signs user/API access tokens for browser and API sessions.',
            example: 'openssl rand -base64 48',
          },
          {
            key: 'NOPSAI_BOOTSTRAP_ADMIN_PASSWORD',
            area: 'Bootstrap',
            description: 'Creates or rotates the first local administrator during production bootstrap.',
            example: 'openssl rand -base64 24',
          },
          {
            key: 'AAA_SHARED_INTERNAL_TOKEN',
            area: 'Bootstrap',
            description: 'Shared internal token used by NopsAI when calling AAA protected endpoints.',
            example: 'openssl rand -base64 32',
          },
        ],
        examples: [],
        relatedDocs: ['doc/first-install-wizard.md', 'doc/enterprise-gates.md'],
        runbooks: ['Complete setup preflight', 'Set bootstrap administrator password', 'Run setup/first-run'],
        caveats: ['Changing generated service secrets usually requires restarting affected services.'],
      },
      {
        id: 'required-envs-service-urls',
        title: 'Required Environment and Service URLs',
        level: 'Admin',
        docType: 'reference',
        audiences: ['administrator', 'operator', 'security'],
        audience: 'Platform administrators, operators, and security reviewers deploying or auditing NopsAI',
        summary:
          'Bootstrap secrets, internal service URLs, dispatcher transport, Docker runner networking, System Logs, and PDF rendering settings must be explicit before production traffic is allowed.',
        keyFacts: [
          'Docker uses environment variables and Helm maps the same bootstrap values to Secret keys such as database-url, master-key, jwt-signing-key, service-jwt-signing-key, aaa-shared-internal-token, dispatcher-tls-secret, and bootstrap-admin-password.',
          'Keep NOPSAI_MASTER_KEY, JWT_SIGNING_KEY, SERVICE_JWT_SIGNING_KEY, AAA_SHARED_INTERNAL_TOKEN, and NOPSAI_BOOTSTRAP_ADMIN_PASSWORD as separate high-entropy values.',
          'Internal URLs such as AAA_API_URL, NOPSAI_API_URL, GIT_BOT_API_URL, and DISPATCHER_GRPC_ADDRESS should point to private service discovery names.',
          'Production dispatcher clients should use tls or mtls with DISPATCHER_TLS_SECRET rather than disabled transport.',
          'Docker System Logs should read through docker-socket-proxy, and PDF final outputs require a reachable Gotenberg renderer.',
        ],
        details: [
          'Deployment owns bootstrap-only secrets. GitOps owns reviewable operating config after the platform can start, but it should not contain the root values needed to decrypt and authenticate the platform itself.',
          'The API, AAA, dispatcher, git-bot, runners, and agents rely on consistent internal addressing. A browser-facing public URL is not a replacement for service-to-service callback URLs.',
          'Helm deployments should create the bootstrap Secret before chart installation and should normally source it from External Secrets, Sealed Secrets, SOPS, or a cluster secret manager.',
          'Compose deployments should use nopsai install docker-compose to generate the service file, local secrets including dispatcher TLS, editable internal service URLs, and a temporary bootstrap password that must be changed after sign-in.',
        ],
        configRows: requiredEnvironmentRows,
        examples: [
          {
            title: 'Generate a Compose install',
            language: 'bash',
            code:
              'nopsai install docker-compose \\\n  --version 2.10.648 \\\n  --output-dir ./nopsai-install \\\n  --bootstrap-admin-email platform-admin@example.com \\\n  --nopsai-api-url http://nopsai:8080 \\\n  --dispatcher-address dispatcher:9090 \\\n  --run',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
            rollback: 'cd ./nopsai-install && docker compose --env-file .env -f docker-compose.yaml down',
          },
          {
            title: 'Helm bootstrap Secret keys',
            language: 'bash',
            code:
              'kubectl -n nopsai create secret generic nopsai-secrets \\\n  --from-literal=database-url="postgres://nopsai:<password>@postgres.example:5432/nopsai?sslmode=require" \\\n  --from-literal=master-key="$(openssl rand -base64 32)" \\\n  --from-literal=jwt-signing-key="$(openssl rand -base64 48)" \\\n  --from-literal=service-jwt-signing-key="$(openssl rand -base64 48)" \\\n  --from-literal=aaa-shared-internal-token="$(openssl rand -base64 32)" \\\n  --from-literal=dispatcher-tls-secret="$(openssl rand -base64 48)" \\\n  --from-literal=bootstrap-admin-password="$(openssl rand -base64 24)"',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
          },
        ],
        relatedDocs: ['doc/cli.md', 'deploy/helm/nopsai/values.yaml', 'doc/enterprise-gates.md'],
        runbooks: ['Verify bootstrap secret separation', 'Fix service callback URL mismatch', 'Rotate dispatcher TLS trust seed'],
        caveats: [
          'A valid route to the public UI or API does not prove internal dispatcher, git-bot, AAA, runner, or renderer callbacks are correct.',
          'Do not copy development fallback secrets from local Compose into shared environments.',
        ],
      },
      {
        id: 'docker-compose',
        title: 'Docker Compose',
        level: 'Start',
        audience: 'Developers, evaluators, and single-host operators',
        summary:
          'The Compose stack starts PostgreSQL 15, API, AAA, dispatcher, git-bot, UI, Gotenberg, Docker socket proxy, Docker runner, and build-only runtime images.',
        keyFacts: [
          'Default local entry points are UI on http://localhost/, API on http://localhost:8080, git-bot on http://localhost:8081, dispatcher on localhost:9090, and PostgreSQL on localhost:5432.',
          'The Docker runner mounts /var/run/docker.sock so it can create agent and step containers.',
          'The API reads System Logs through the restricted Docker socket proxy at tcp://docker-socket-proxy:2375.',
          'Compose contains local-development fallback secrets and must be overridden for non-local use.',
        ],
        details: [
          'The Compose network is `nopsai-net`. Internal URLs use service DNS names, while published host ports are for browsers, API clients, and local tooling.',
          'The checked-in Compose definition persists PostgreSQL with a named volume. Product-generated backups should get a separate durable mount at /data/backups for real deployments.',
        ],
        configRows: [
          {
            key: 'NOPSAI_API_URL',
            area: 'Runner and agent',
            description: 'Internal API URL supplied to runtime containers.',
            example: 'http://nopsai:8080',
          },
          {
            key: 'FINAL_OUTPUT_PDF_RENDERER_URL',
            area: 'Final outputs',
            description: 'Internal Gotenberg URL used by server-owned PDF rendering.',
            example: 'http://gotenberg:3000',
          },
          {
            key: 'SYSTEM_LOGS_DOCKER_HOST',
            area: 'System Logs',
            description: 'Restricted Docker socket proxy endpoint for allow-listed log reads.',
            example: 'tcp://docker-socket-proxy:2375',
          },
          {
            key: 'DOCKER_NETWORK_NAME',
            area: 'Docker runner',
            description: 'Network used for agent and step containers in local Docker execution.',
            example: 'nopsai-net',
          },
        ],
        examples: [
          {
            title: 'Local stack',
            language: 'bash',
            code: 'docker compose up -d --build\ndocker compose ps\ndocker compose logs -f nopsai dispatcher docker-runner',
          },
        ],
        relatedDocs: ['docker-compose.yaml', 'doc/cli.md', 'doc/enterprise-gates.md'],
        runbooks: ['Start local stack', 'Check runner registration', 'Persist /data/backups for non-local Compose'],
        caveats: ['The Docker runner socket mount is a privileged operational boundary and should be isolated to trusted runner hosts.'],
      },
      {
        id: 'helm-kubernetes',
        title: 'Kubernetes and Helm',
        level: 'Admin',
        audience: 'Cluster operators and platform teams',
        summary:
          'The Helm chart deploys API, AAA, dispatcher, git-bot, UI, Gotenberg, and a Kubernetes runner. PostgreSQL and bootstrap secrets are intentionally external.',
        keyFacts: [
          'Create the Secret referenced by `secrets.existingSecret` before installing the chart.',
          'Default secret keys are database-url, master-key, jwt-signing-key, service-jwt-signing-key, and aaa-shared-internal-token.',
          '`topology.dispatcherGRPCAddress` defaults to dispatcher:9090 and feeds API and Kubernetes runner pods.',
          'The chart defaults to Kubernetes System Logs and can create read-only pods and pods/log RBAC for the API service account.',
          'The Kubernetes runner starts one agent pod per run and step pods that share a PVC-backed workspace.',
        ],
        details: [
          'Use External Secrets, Sealed Secrets, SOPS, or a platform secret manager instead of committing secret values to Helm values.',
          'The chart exposes replica counts and resource blocks but leaves sizing empty by default. Establish capacity through load testing and metrics.',
        ],
        configRows: [
          {
            key: 'secrets.existingSecret',
            area: 'Helm',
            description: 'Name of the Kubernetes Secret holding bootstrap values.',
            example: 'nopsai-secrets',
          },
          {
            key: 'k8sRunner.workspace.volumeMode',
            area: 'Runner',
            description: 'Workspace mode for agent and step pods.',
            example: 'pvc',
          },
          {
            key: 'topology.dispatcherGRPCAddress',
            area: 'Helm',
            description: 'Internal dispatcher gRPC endpoint injected into API and Kubernetes runner pods.',
            example: 'dispatcher:9090',
          },
          {
            key: 'k8sRunner.affinityEnabled',
            area: 'Runner',
            description: 'Pins step pods to the agent node for ReadWriteOnce storage compatibility.',
            example: 'true',
          },
          {
            key: 'systemLogs.provider',
            area: 'System Logs',
            description: 'Log provider used by the API in the chart.',
            example: 'kubernetes',
          },
        ],
        examples: [
          {
            title: 'Install from packaged chart',
            language: 'bash',
            code:
              'kubectl create namespace nopsai\nkubectl -n nopsai create secret generic nopsai-secrets \\\n  --from-literal=database-url="postgres://..." \\\n  --from-literal=master-key="..." \\\n  --from-literal=jwt-signing-key="..." \\\n  --from-literal=service-jwt-signing-key="..." \\\n  --from-literal=aaa-shared-internal-token="..."\nhelm upgrade --install nopsai ./nopsai-<version>.tgz \\\n  --namespace nopsai \\\n  --create-namespace \\\n  --set secrets.existingSecret=nopsai-secrets',
          },
        ],
        relatedDocs: ['deploy/helm/nopsai/README.md', 'deploy/helm/nopsai/values.yaml', 'doc/kubernetes-runner.md'],
        runbooks: ['Create bootstrap Secret', 'Render Helm plan', 'Verify Kubernetes runner workspace PVCs'],
        caveats: ['The repository does not provide Terraform modules, managed database provisioning, ingress TLS automation, or cloud IAM wiring.'],
      },
      {
        id: 'additional-runners',
        title: 'Additional Runners',
        level: 'Operate',
        audience: 'Operators scaling or isolating execution capacity',
        summary:
          'Runner installs are generated from System > Dispatcher > Runner Installs and use one-time download tokens to produce Docker or Kubernetes runner configuration.',
        keyFacts: [
          'Runner tokens expire after ten minutes and are consumed after the first successful download.',
          'Each runner should have a unique ID, capacity, allowed scope list, and network path to the dispatcher.',
          'Separate runners are the natural boundary for production versus non-production, region, team, security zone, or workload class.',
        ],
        details: [
          'The dispatcher checks runner availability, scope compatibility, routing, affinity, and load before assignment.',
          'For Kubernetes, runner manifests include namespace, ServiceAccount, namespace-scoped Role, RoleBinding, dispatcher auth Secret, runtime ConfigMap, and Deployment.',
          'Runner defaults and hard routing can live in setting/system/runner.yaml. Dispatcher routing updates are exposed through internal runtime config and do not require a dispatcher container restart.',
        ],
        configRows: [
          {
            key: 'RUNNER_ID',
            area: 'Runner',
            description: 'Stable unique runner identity used by dispatcher routing and monitoring.',
            example: 'prod-eu-1',
          },
          {
            key: 'RUNNER_SCOPES',
            area: 'Runner',
            description: 'Comma-separated scopes the runner may execute.',
            example: 'dev,staging,prod',
          },
          {
            key: 'RUNNER_CAPACITY',
            area: 'Runner',
            description: 'Maximum concurrent jobs for the runner.',
            example: '10',
          },
        ],
        examples: [
          {
            title: 'Runner defaults and dispatcher routing',
            language: 'yaml',
            code:
              'dispatcher_grpc_address: dispatcher:9090\nruntime: docker\nrunner_id: runner-general\nrunner_scopes: dev,prod\nrunner_capacity: 2\n\nassistant:\n  default_llm_profile: standard\n  default_agent_profile: release-manager\n\ndispatcher_routing:\n  prod:\n    - runner-prod-1\n    - runner-prod-2\n  dev:\n    - runner-dev-1\n  "*":\n    - runner-general',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
          },
        ],
        relatedDocs: ['doc/kubernetes-runner.md', 'doc/runtime-flows.md'],
        runbooks: ['Generate one-time runner installer', 'Restrict runner scopes', 'Retire stale runner registration'],
        caveats: ['Increasing every replica count does not by itself guarantee high availability or capacity.'],
      },
    ],
  },
  {
    id: 'platform-admin',
    title: 'Platform Administration',
    owner: 'Administrators and security teams',
    description: 'Networking, storage, GitOps authority, credentials, authentication, AAA, teams, scopes, and runtime ownership.',
    articles: [
      {
        id: 'networking',
        title: 'Networking and Exposure',
        level: 'Admin',
        audience: 'Network, platform, and security operators',
        summary:
          'Expose the UI, API, and required webhook endpoints through TLS; keep AAA, PostgreSQL, Gotenberg, Docker socket proxy, and usually dispatcher private.',
        keyFacts: [
          'Browser and CLI traffic reaches the API on port 8080 by default.',
          'GitHub App webhooks reach git-bot on port 8081 by default.',
          'Generic Git webhook sources enter through the API and should use external HTTPS.',
          'Dispatcher traffic is long-lived gRPC from API, runners, and agents on port 9090 by default.',
        ],
        details: [
          'Required outbound access depends on selected Git providers, container registries, LLM providers, MCP endpoints, SMTP, OIDC, and external PostgreSQL.',
          'Dispatcher transport supports mtls, tls, and disabled modes. Production gates reject disabled dispatcher transport security.',
        ],
        configRows: [
          {
            key: 'DISPATCHER_TLS_MODE',
            area: 'Dispatcher',
            description: 'Transport security mode for dispatcher gRPC.',
            example: 'mtls',
          },
          {
            key: 'DISPATCHER_TLS_SECRET',
            area: 'Dispatcher',
            description: 'Trust seed for derived in-memory dispatcher certificates.',
            example: 'managed secret value',
          },
        ],
        examples: [],
        relatedDocs: ['doc/jwt-authentication.md', 'doc/enterprise-gates.md', 'doc/system-logs.md'],
        runbooks: ['Review public ingress list', 'Rotate dispatcher trust seed', 'Validate outbound provider access'],
        caveats: ['A fully offline or air-gapped installation workflow is not currently defined.'],
      },
      {
        id: 'storage-persistence',
        title: 'Storage and Persistence',
        level: 'Reference',
        audience: 'Database administrators, operators, and auditors',
        summary:
          'PostgreSQL is the primary durable store for product, identity, configuration, execution, monitoring, audit, credential metadata, and setup state.',
        keyFacts: [
          'Compose mounts PostgreSQL at /var/lib/postgresql/data in the `db` named volume.',
          'Docker workspaces are shared named volumes, normally `vol-<run-id>`.',
          'Kubernetes workspaces are PVC-backed so agent and step pods can share files.',
          'Approval checkpoints store compressed workspace archives in PostgreSQL so a run can resume on a new agent.',
        ],
        details: [
          'Final-output source and generation status are stored in PostgreSQL. PDF/HTML use validated DocumentSpec, and Excel uses validated SpreadsheetSpec.',
          'Product-generated backups are gzip-compressed JSON Lines files stored under /data/backups by default, with checksum and file mode metadata.',
        ],
        configRows: [
          {
            key: 'FINAL_OUTPUT_PDF_TIMEOUT_SECONDS',
            area: 'Final outputs',
            description: 'Timeout for PDF rendering through Gotenberg.',
            example: '45',
          },
        ],
        examples: [
          {
            title: 'Persist product backups in Compose',
            language: 'yaml',
            code: 'services:\n  nopsai:\n    volumes:\n      - nopsai-backups:/data/backups\n\nvolumes:\n  nopsai-backups:',
          },
        ],
        relatedDocs: ['doc/final-output-rendering.md', 'doc/kubernetes-runner.md'],
        runbooks: ['Verify backup file checksum', 'Test database recovery outside NopsAI', 'Check workspace PVC events'],
        caveats: [
          'The default Compose topology does not mount dedicated durable storage at /data/backups.',
          'The repository does not implement an S3 or generic object-storage backup backend.',
        ],
      },
      {
        id: 'gitops-authority',
        title: 'Configuration Authority and GitOps',
        level: 'Admin',
        audience: 'Platform administrators and repository owners',
        summary:
          'Bootstrap environment, durable runtime settings, and GitOps repositories form separate configuration layers with different ownership and recovery behavior.',
        keyFacts: [
          'Bootstrap values include database URL, master key, JWT signing keys, AAA token, dispatcher trust, service addresses, renderer, and System Logs topology.',
          'GitOps can own pipelines, steps, schedules, triggers, external triggers, Git webhook sources, scopes, knowledge, access, config repositories, and setting/system files.',
          'Global configuration sync runs before delegated team repositories.',
          'UI edits to GitOps-managed resources create database overrides until the change is pushed back to Git.',
        ],
        details: [
          'Canonical system files include credentials.yaml, github.yaml, runner.yaml, auth.yaml, mail.yaml, llm_profile.yaml, mcp.yaml, and agent-profiles.yaml.',
          'Config sync can import, update, prune Git-managed resources, detect drift, generate commit-ready changes, push to a review branch, and adopt database-created resources when matching files exist.',
          'A system/global repository can own shared platform settings and delegate group-owned repositories. Runtime records stay in PostgreSQL; declarative intent lives in Git.',
          'Write-enabled repositories push generated files to a review branch before merge. Sync should not mutate the protected sync branch directly.',
        ],
        configRows: [
          ...gitOpsRepositoryRows,
          {
            key: 'setting/system/runner.yaml',
            area: 'GitOps',
            description: 'Runtime topology, runner defaults, dispatcher routing, and assistant settings.',
            example: 'setting/system/runner.yaml',
          },
          {
            key: 'setting/system/credentials.yaml',
            area: 'GitOps',
            description: 'Encrypted credential envelopes for integration credentials.',
            example: 'setting/system/credentials.yaml',
          },
        ],
        examples: [
          {
            title: 'Global config repository binding',
            language: 'bash',
            code:
              'curl -X PUT -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d \'{"repo_url":"https://github.com/acme/nopsai-config","branch":"main","base_path":"","enabled":true,"write_enabled":true,"write_branch":"nopsai/ui-changes"}\' \\\n  http://localhost:8080/v1/system/config-repo\n\ncurl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  http://localhost:8080/v1/system/config-repos/sync\n\ncurl -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  http://localhost:8080/v1/system/config-repo/drift',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
            permission: 'config_repo.manage',
          },
          {
            title: 'System setting file map',
            language: 'text',
            code:
              'setting/system/credentials.yaml\nsetting/system/github.yaml\nsetting/system/runner.yaml\nsetting/system/auth.yaml\nsetting/system/mail.yaml\nsetting/system/llm_profile.yaml\nsetting/system/mcp.yaml\nsetting/system/agent-profiles.yaml',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
          },
        ],
        relatedDocs: ['doc/mcp-pipeline-integration.md', 'doc/credential-management.md', 'doc/git-webhook-sources.md'],
        runbooks: ['Resolve GitOps drift', 'Promote a UI override back to Git', 'Review delegated team repository ownership'],
        caveats: ['Database-only edits to GitOps-managed resources may be overwritten or recreated by the next sync.'],
      },
      {
        id: 'credentials-secrets',
        title: 'Credentials, Secrets, and Variables',
        level: 'Security',
        audience: 'Security teams, administrators, and automation authors',
        summary:
          'Bootstrap secrets remain in deployment Secrets; integration credentials live in the encrypted credential registry; pipeline secrets and variables resolve by scope and repository.',
        keyFacts: [
          'Credential references use stable URIs such as credential://system/llm/openai-primary or credential://team/platform/llm/openai.',
          'Credential versions use envelope encryption with AES-256-GCM and versioned key wrapping.',
          'Human-facing reads return credential metadata only; normal credential-value read is not available.',
          'Pipeline secrets are encrypted at rest, selected by scope/repository, delivered only in protected run payloads, and masked from logs and history.',
        ],
        details: [
          'Resolution prefers the selected run scope; unscoped defaults are used only for unscoped runs. Repository-specific values override global values within the same scope.',
          'Variables can come from scope configuration, repository-specific values, manual run overrides, schedule overrides, external trigger mappings, step-level overrides, and task-level overrides.',
        ],
        configRows: [
          {
            key: 'credential://system/llm/openai-primary',
            area: 'Credential reference',
            description: 'Example stable reference for a system LLM credential.',
            example: 'credential://system/llm/openai-primary',
          },
          {
            key: 'credential://system/mcp/github-readonly',
            area: 'Credential reference',
            description: 'Example stable reference for an MCP bearer token.',
            example: 'credential://system/mcp/github-readonly',
          },
        ],
        examples: [],
        relatedDocs: ['doc/credential-management.md', 'doc/access-control.md'],
        runbooks: ['Rotate an integration credential', 'Review credential consumer access logs', 'Add scoped pipeline secret'],
        caveats: ['Bootstrap secrets cannot be stored inside the same database they unlock.'],
      },
      {
        id: 'auth-aaa-teams-scopes',
        title: 'Authentication, AAA, Teams, and Scopes',
        level: 'Security',
        audience: 'Security administrators and enterprise platform owners',
        summary:
          'NopsAI authenticates callers, maps routes to actions/resources, asks AAA for decisions, and keeps runtime authorization tied to the original caller.',
        keyFacts: [
          'Supported auth paths include local access JWTs, refresh tokens, personal tokens, service-account tokens, internal service JWTs, dispatcher service JWTs, and OIDC login with PKCE.',
          'AAA handles Check, BatchCheck, Filter, ACL expansion, inheritance, and decision auditing, with a short-outage in-process fallback in the API.',
          'Product roles are viewer, developer, owner, and platform admin; admin is granted only on the platform resource.',
          'Scopes are runtime context such as dev, test, production, or platform/prod. They are not run-navigation parents.',
        ],
        details: [
          'Teams form hierarchical product boundaries for ownership, access, run navigation, config repository delegation, notifications, team AI overlays, and repository-to-application matching.',
          'Cross-team execution requires access to every concrete resource used by the run, not only the pipeline.',
        ],
        configRows: [
          {
            key: 'setting/system/auth.yaml',
            area: 'GitOps',
            description: 'Local login and OIDC provider settings.',
            example: 'setting/system/auth.yaml',
          },
          {
            key: 'auth_identity_providers.client_credential_ref',
            area: 'OIDC',
            description: 'Credential reference for OIDC client secret.',
            example: 'credential://system/oidc/corporate/client-secret',
          },
        ],
        examples: [
          {
            title: 'OIDC provider sketch',
            language: 'yaml',
            code:
              'oidc:\n  enabled: true\n  auto_create_users: true\n  providers:\n    corporate:\n      type: oidc\n      display_name: Company SSO\n      issuer: https://idp.company.com\n      client_id: nopsai\n      client_credential_ref: credential://system/oidc/corporate/client-secret\n      scopes: [openid, email, profile]',
          },
        ],
        relatedDocs: ['doc/access-control.md', 'doc/jwt-authentication.md', 'doc/local-keycloak-sso.md', 'doc/team-resource-ownership-design.md'],
        runbooks: ['Create least-privilege service account', 'Audit cross-team resource grants', 'Configure corporate OIDC provider'],
        caveats: ['Public pipeline visibility does not grant dependent scopes, secrets, variables, runners, credentials, or knowledge context.'],
      },
    ],
  },
  {
    id: 'automation',
    title: 'Automation Authoring',
    owner: 'Automation teams',
    description: 'Pipeline YAML, steps, dependencies, approvals, schedules, Git triggers, Git webhook sources, and external triggers.',
    articles: [
      {
        id: 'pipeline-schema',
        title: 'Pipeline YAML Schema',
        level: 'Reference',
        audience: 'Automation authors and platform reviewers',
        summary:
          'Pipelines define container images, variables, steps, timeouts, AI controls, profiles, runtime pools, knowledge context, and final outputs.',
        keyFacts: [
          'Top-level fields include name, version, description, container_image, working_directory, display_options, variables, steps, timeout, llm_enabled, agent_profile, llm_profile, mcp_profiles, runtime_pool, affinity_enabled, knowledge_context, output, and LLM sharing controls.',
          'Every step must contain exactly one mode: include, tasks, goal, script, or approval.',
          'Independent ready tasks may execute concurrently; depends_on defines graph edges.',
          'Script-only pipelines can set llm_enabled: false to avoid requiring an LLM registry.',
        ],
        details: [
          'Pipeline YAML is a reviewed automation contract. It should contain the desired execution graph and references to approved resources, not raw provider credentials, arbitrary MCP server URLs, or environment-specific bootstrap secrets.',
          'Within one step, NopsAI reuses a step container or pod. Across steps, separate containers or pods may be used. All steps share the run workspace.',
          'Script tasks with effective guardrail or policy knowledge are submitted for LLM validation before execution and fail closed on conflicts or unavailable validation.',
          'Profile directives are intentionally separate. agent_profile changes the persona and instructions; llm_profile selects the provider and model client; mcp_profiles selects approved external tools; knowledge_context supplies governed documents.',
          'Runtime references such as variables and secrets may be bare names, default:NAME for unscoped/default values, or scope/path:NAME for explicit scoped lookup. The injected environment variable name is the final NAME part.',
        ],
        configRows: pipelineTopLevelRows,
        examples: [
          {
            title: 'Release readiness pipeline',
            language: 'yaml',
            code:
              'name: release-readiness\nversion: "1.0"\ncontainer_image: alpine:3.20\nworking_directory: /workspace\ntimeout: 2h\nvariables: [ENVIRONMENT, RELEASE_SHA]\nagent_profile: devops-engineer\nllm_profile: standard\nmcp_profiles: [github-pr-review]\nknowledge_context:\n  - kind: guardrail\n    ref: security/production-deployment\n    required: true\nsteps:\n  - name: test\n    image: golang:1.24\n    script: go test ./...\n  - name: reliability-review\n    depends_on: [test]\n    goal: Review release reliability, rollback readiness, and operational evidence.\n  - name: production-approval\n    depends_on: [reliability-review]\n    approval:\n      type: production-deploy\n      teams: [platform/production]\n      allow_self_approval: false',
          },
        ],
        relatedDocs: ['doc/feature-reference.md', 'doc/runtime-flows.md'],
        runbooks: ['Validate a new pipeline', 'Review dependency graph before production use', 'Convert manual shell script to script-only pipeline'],
        caveats: ['Goal tasks, conditions, and explicit MCP profiles are invalid when LLM behavior is disabled.'],
      },
      {
        id: 'step-task-directives',
        title: 'Step and Task Directives',
        level: 'Reference',
        audience: 'Automation authors reading or reviewing pipeline YAML',
        summary:
          'Steps choose one execution mode and add operational controls; tasks are the executable units inside multi-task steps.',
        keyFacts: [
          'A step must define exactly one of include, tasks, goal, script, or approval.',
          'Step depends_on points to other steps. Task depends_on points only to tasks in the same step.',
          'steps[].llm_profile means this step uses that model profile for its condition and LLM tasks unless a task has a more specific llm_profile.',
          'steps[].agent_profile changes prompt persona for the step; tasks cannot define agent_profile.',
          'mcp_profiles can be set on pipeline, step, and goal task levels, but not on script or include work.',
        ],
        details: [
          'Goal steps and goal tasks ask the selected LLM profile for structured actions. Those actions can execute commands, replace files under the working directory, return an answer, or call an approved MCP tool.',
          'Script steps and script tasks run the provided shell script directly. When guardrail or policy Knowledge Context is active, the script is first validated as the exact proposed command and fails closed if validation is unavailable or reports a conflict.',
          'llm_output_sharing is about history, not logs. Logs still record masked task output; the setting controls whether output is fed into later LLM task history.',
          'variables are layered in this order: pipeline/runtime values, inherited system values such as GIT_* and SCOPE, step variables, then task variables.',
        ],
        configRows: stepTaskRows,
        examples: [
          {
            title: 'Mixed task step with profile overrides',
            language: 'yaml',
            code:
              'steps:\n  - name: review\n    image: golang:1.24\n    llm_profile: reasoning\n    agent_profile: sre\n    mcp_profiles: [github-readonly]\n    variables:\n      REVIEW_DEPTH: full\n    tasks:\n      - name: unit-tests\n        script: go test ./...\n      - name: risk-review\n        depends_on: [unit-tests]\n        llm_profile: fast\n        goal: Summarize reliability risk from the test output and repository context.\n        llm_output_sharing: false',
          },
          {
            title: 'Script-only pipeline',
            language: 'yaml',
            code:
              'name: shell-check\nllm_enabled: false\ncontainer_image: alpine:3.20\nsteps:\n  - name: lint\n    script: ./scripts/lint.sh\n  - name: test\n    depends_on: [lint]\n    script: ./scripts/test.sh',
          },
        ],
        relatedDocs: ['doc/feature-reference.md', 'doc/agent-profiles.md', 'doc/llm-model-selection.md', 'doc/mcp-pipeline-integration.md'],
        runbooks: ['Find which profile a task uses', 'Debug a skipped condition', 'Split a large step into independent tasks'],
        caveats: ['Task-level agent_profile is rejected by YAML parsing. Put persona overrides on the pipeline or step instead.'],
      },
      {
        id: 'approvals-reuse-children',
        title: 'Approvals, Reusable Steps, and Child Pipelines',
        level: 'Operate',
        audience: 'Release managers and workflow authors',
        summary:
          'Human approvals checkpoint work and release runner capacity; reusable steps and child pipelines let shared automation be composed with explicit authorization.',
        keyFacts: [
          'Approval steps store execution history, completed task keys, variables, pipeline definition, and a compressed workspace archive.',
          'Approval moves a run to waiting_approval and exits the agent so no runner capacity stays occupied.',
          'Approval resumes with a fresh agent that restores the checkpoint.',
          'Include steps can reference step:<identifier> or pipeline:<identifier>.',
        ],
        details: [
          'Cross-team reusable-step and child-pipeline includes require explicit step.use or pipeline.use authorization.',
          'Parent run aggregation remains active while direct child runs execute and surfaces child failures.',
          'Approval teams must be relative team paths. Absolute paths, home-directory prefixes, dot segments, and duplicate team entries are rejected during validation.',
          'Pending approvals are visible to assigned approvers even when the pipeline belongs to another team, so approval queues do not require broad pipeline ownership.',
        ],
        configRows: approvalIncludeRows,
        examples: [
          {
            title: 'Approval step',
            language: 'yaml',
            code: 'steps:\n  - name: deploy-gate\n    approval:\n      type: production-deploy\n      teams:\n        - platform/prod\n      allow_self_approval: false',
          },
        ],
        relatedDocs: ['doc/feature-reference.md', 'doc/access-control.md'],
        runbooks: ['Approve or reject a pending gate', 'Resume an approval checkpoint', 'Audit cross-team include permissions'],
        caveats: ['Rejecting an approval marks the approval task failed and the run rejected.'],
      },
      {
        id: 'git-triggers',
        title: 'Git Triggers and Git Webhook Sources',
        level: 'Reference',
        audience: 'Repository owners and integration authors',
        summary:
          'GitHub events enter through git-bot; GitLab, Bitbucket, Gitea, and generic Git events enter through managed Git Webhook Sources in the API.',
        keyFacts: [
          'GitHub webhook flow verifies HMAC, forwards the payload to the API, matches triggers, authorizes repository access, creates a run, and updates GitHub checks.',
          'Git Webhook Sources support provider, enabled state, team path, visibility, hmac/static_token/none auth modes, credential_ref, repository allowlist, and rate limits.',
          'Trigger manifests support events, branches, skip_branches, tags, skip_repos, include_paths, exclude_paths, pipelines, and scope.',
          'When changed-file metadata is unavailable, path matching fails open so automation is not silently skipped.',
        ],
        details: [
          'Non-GitHub providers do not currently fetch pipeline or trigger files directly from the source repository. Their triggers and pipelines must already exist in NopsAI, normally through configuration sync.',
          'GitHub App payloads are verified by git-bot first; forwarding from git-bot to nopsai /v1/git/events is an internal service-token call, not public webhook ingress.',
          'Delivery history records accepted events, authentication failures, idempotency behavior, and no_match outcomes.',
          'Branch, tag, repository, and changed-file matching uses simple glob semantics. Single star matches one path segment, double star can span directories, and question mark matches one non-slash character.',
          'Path filters are applied only when the provider supplies changed-file data. If the changed-file list is unavailable, NopsAI treats the rule as eligible so CI is not silently skipped.',
        ],
        configRows: [
          ...triggerRows,
          {
            key: 'setting/system/github.yaml',
            area: 'GitHub App',
            description: 'System GitHub App settings path for git-bot URL, app ID, installation ID, and credential references.',
            example: 'setting/system/github.yaml',
            type: 'path',
            required: 'conditional',
            scope: 'system config repository',
          },
          {
            key: 'github_private_key_credential_ref',
            area: 'GitHub App',
            description: 'Credential reference for the GitHub App private key.',
            example: 'credential://system/github/app-private-key',
            type: 'credential reference',
            required: true,
            scope: 'system',
            security: 'Store the private key as a credential, not plaintext GitOps YAML.',
          },
          {
            key: 'github_webhook_credential_ref',
            area: 'GitHub App',
            description: 'Credential reference for the webhook HMAC secret that git-bot uses to verify X-Hub-Signature-256.',
            example: 'credential://system/github/webhook-secret',
            type: 'credential reference',
            required: true,
            scope: 'system',
            security: 'Store the webhook secret as a credential, not plaintext GitOps YAML.',
          },
        ],
        examples: [
          {
            title: 'GitHub App settings',
            language: 'yaml',
            code:
              'git_bot_api_url: http://git-bot:8081\ngithub_app_id: "123456"\ngithub_installation_id: "987654"\ngithub_private_key_credential_ref: credential://system/github/app-private-key\ngithub_webhook_credential_ref: credential://system/github/webhook-secret',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
            permission: 'config_repo.manage and credential.use for referenced credentials',
          },
          {
            title: 'Repository trigger',
            language: 'yaml',
            code:
              'provider: gitlab\nteam: platform\nwebhook_source: gitlab-platform\ntriggers:\n  - on: push\n    branches: [main, release/*]\n    include_paths:\n      - services/api/**\n    exclude_paths:\n      - docs/**\n    pipelines:\n      - platform/api-ci\n    scope: platform/prod',
          },
        ],
        relatedDocs: ['doc/triggering.md', 'doc/git-webhook-sources.md'],
        runbooks: ['Debug GitHub webhook no-run outcome', 'Rotate webhook credential', 'Review repository allowlist'],
        caveats: ['Use auth_mode: none only on isolated private ingress.'],
      },
      {
        id: 'schedules-external-triggers',
        title: 'Schedules and External Triggers',
        level: 'Operate',
        audience: 'Automation authors and integration owners',
        summary:
          'Schedules run stored pipelines on time; External Triggers let a user or service account invoke one configured pipeline with payload validation and variable mapping.',
        keyFacts: [
          'Schedules support cron, one-time timestamps, timezone, enabled state, scope, variable overrides, run team, immediate run, latest-run linkage, and status.',
          'Schedules run under a schedule-owned service account with explicit grants for pipeline and referenced resources.',
          'External triggers support allowed callers, payload schema, variable mapping, rate limit, idempotency key, and invocation history.',
          'External trigger callers must appear in allowed_callers and pass external_trigger.invoke.',
        ],
        details: [
          'Variable mappings can come from event_type, payload.<path>, variables.<name>, direct payload paths, or literal:<value> sources.',
          'Payload schema validation supports object schemas, required fields, and basic property types: string, number, integer, boolean, object, and array.',
          'Run team controls where scheduled or externally triggered runs appear and which notification policy lineage receives events.',
          'Schedules and external triggers still use normal run authorization. The caller or schedule service account must be allowed to execute the selected pipeline and use the selected scope, profiles, secrets, variables, and knowledge context.',
        ],
        configRows: scheduleExternalRows,
        examples: [
          {
            title: 'Cron schedule',
            language: 'yaml',
            code:
              'name: nightly-api-deploy\npipeline: team-1/services/api/deploy\nschedule_kind: cron\ncron_expression: "0 2 * * *"\ntimezone: UTC\nenabled: true\nscope: prod\nrun_team_path: team-1\nvariables:\n  RELEASE_CHANNEL: nightly',
          },
          {
            title: 'Authenticated external trigger',
            language: 'yaml',
            code:
              'id: deploy-prod\nname: Deploy prod from ServiceNow\nenabled: true\npipeline: platform/prod/platform-maintenance\nscope: prod\nrun_team_path: platform/prod\nallowed_callers:\n  - type: service_account\n    id: servicenow-prod\nvariable_mapping:\n  VERSION: payload.version\n  CHANGE_ID: payload.change.id\npayload_schema:\n  type: object\n  required: [version]\nrate_limit:\n  per_minute: 10',
          },
        ],
        relatedDocs: ['doc/feature-reference.md', 'doc/runtime-flows.md'],
        runbooks: ['Run a schedule immediately', 'Investigate missed schedule', 'Replay an external trigger safely'],
        caveats: ['GitOps-managed schedules and triggers can be replaced by sync unless changes are pushed back to Git.'],
      },
    ],
  },
  {
    id: 'ai-mcp-knowledge',
    title: 'AI, MCP, and Knowledge',
    owner: 'AI platform team',
    description: 'Agent Profiles, LLM Profiles, external MCP, hosted MCP, Knowledge Context, and final-output generation.',
    articles: [
      {
        id: 'ai-control-layers',
        title: 'AI Control Layers',
        level: 'Reference',
        audience: 'AI platform teams and security reviewers',
        summary:
          'Agent Profiles, LLM Profiles, MCP Profiles, Knowledge Context, AAA, and pipeline schema each own a different part of AI-enabled automation.',
        keyFacts: [
          'Agent Profiles control persona, role, and prompt instructions.',
          'LLM Profiles control provider, model, endpoint, credential, token settings, and generation settings.',
          'MCP Profiles control approved external servers and tools.',
          'Knowledge Context supplies architecture, rules, policies, examples, and runbooks to tasks.',
          'Selecting an Agent Profile does not select an LLM, grant tools, reveal secrets, or grant permissions.',
        ],
        details: [
          'Profile resolution is independent and layered. Step settings can override pipeline settings, and task settings can override LLM profile selection where supported.',
          'AAA still decides whether the original caller may use each selected profile, tool boundary, knowledge document, secret, or scope.',
          'For a single goal task, the final effective AI context is the resolved Agent Profile persona, the resolved LLM Profile client, the additive MCP tool profile set, the merged Knowledge Context, the runtime variables, the workspace directory listing allowed by llm_content_* settings, and the masked execution history allowed by llm_output_sharing.',
        ],
        configRows: [
          {
            key: 'setting/system/agent-profiles.yaml',
            area: 'GitOps',
            description: 'System AI personas and default persona.',
            example: 'setting/system/agent-profiles.yaml',
          },
          {
            key: 'ai-profiles.yaml',
            area: 'Team GitOps',
            description: 'Team-level AI profile overlays.',
            example: 'ai-profiles.yaml',
          },
        ],
        examples: [],
        relatedDocs: ['doc/agent-profiles.md', 'doc/llm-model-selection.md'],
        runbooks: ['Review a new Agent Profile', 'Limit AI profile use by scope', 'Audit team AI overlays'],
        caveats: ['Team profiles overlay the system catalog for runs owned by that team.'],
      },
      {
        id: 'llm-profiles',
        title: 'LLM Profiles',
        level: 'Admin',
        audience: 'AI platform owners and administrators',
        summary:
          'LLM Profiles define provider, model, endpoint, credential reference, allowed scopes, timeouts, tokens, temperature, and provider-specific extra settings.',
        keyFacts: [
          'Supported providers include gemini, lmstudio, openai, anthropic, groq, mistral, ollama, openrouter, and azure-openai.',
          'Resolution order is task, step, pipeline, then default profile.',
          'Generic reasoning and thinking fields are supported only for LM Studio; other providers reject those generic settings.',
          'Prompt cache and provider-state modes can be auto, required, or disabled; required fails closed when the provider adapter cannot satisfy the feature.',
          'Script-only pipelines with llm_enabled: false can run without a configured LLM registry.',
        ],
        details: [
          'Profiles should use credential_ref for hosted provider API keys instead of plaintext secrets.',
          'allowed_scopes lets administrators restrict model use by runtime context.',
          'The selected llm_profile is validated before agent launch. A run is rejected when the default profile is missing, a referenced profile does not exist, the profile is not allowed in the requested scope, or the provider configuration is invalid.',
          'Provider prompt caches and provider conversation state are transport optimizations. NopsAI still owns the logical session, scoped context, policy precedence, and cache identity used for audit and invalidation.',
          'Final outputs have their own resolution path: output.items[].llm_profile overrides output.llm_profile, which overrides the pipeline llm_profile, which finally falls back to the configured default profile.',
        ],
        configRows: llmProfileRows,
        examples: [
          {
            title: 'LLM profile registry',
            language: 'yaml',
            code:
              'default_profile: standard\nprofiles:\n  - name: standard\n    provider: openai\n    model: gpt-4.1-mini\n    credential_ref: credential://system/llm/openai-hosted\n    timeout_seconds: 60\n    max_tokens: 4096\n  - name: local\n    provider: ollama\n    model: qwen2.5-coder:14b\n    base_url: http://ollama:11434/v1',
          },
        ],
        relatedDocs: ['doc/llm-model-selection.md', 'doc/credential-management.md'],
        runbooks: ['Add LLM profile', 'Test profile reachability', 'Migrate default model profile'],
        caveats: ['Provider-specific reasoning fields should be validated per provider instead of copied between providers.'],
      },
      {
        id: 'mcp',
        title: 'External and Hosted MCP',
        level: 'Admin',
        audience: 'AI platform teams, tool owners, and automation authors',
        summary:
          'Pipelines may reference approved external streamable-HTTP MCP profiles, and NopsAI also exposes a first-party hosted MCP endpoint at POST /v1/mcp.',
        keyFacts: [
          'External MCP servers define display name, provider, transport, URL, auth type, credential reference, timeout, enabled state, and tools.',
          'MCP profile inheritance for goals is additive across pipeline, step, and task profiles.',
          'Explicit MCP profiles are rejected on script steps, script tasks, and include placeholders.',
          'When MCP profiles resolve for a goal, the agent requires at least one successful MCP tool call before accepting the final action.',
          'Hosted NopsAI MCP filters tools and resources against the authenticated subject AAA permissions.',
        ],
        details: [
          'Hosted MCP supports initialize, tools/list, tools/call, resources/list, and resources/read JSON-RPC methods.',
          'Mutation tools require confirmation. GitOps proposal tools return commit-ready files with applies: false rather than silently applying production changes.',
          'External MCP profile resolution is additive: pipeline profiles, step profiles, and task profiles are combined with duplicates removed. The tool set is then checked against enabled profiles and scope allowlists.',
          'Pipeline YAML can reference approved MCP profile names only. It cannot define arbitrary server URLs or credentials inline.',
        ],
        configRows: mcpRows,
        examples: [
          {
            title: 'External MCP profile',
            language: 'yaml',
            code:
              'mcp_servers:\n  github:\n    display_name: GitHub MCP\n    enabled: true\n    provider: github\n    transport: streamable_http\n    url: https://api.githubcopilot.com/mcp/x/all/readonly\n    auth_type: bearer_token\n    credential_ref: credential://system/mcp/github-readonly\n    timeout: 30s\nmcp_profiles:\n  github-pr-review:\n    enabled: true\n    servers:\n      - server: github\n        tools: ["*"]',
          },
        ],
        relatedDocs: ['doc/mcp-pipeline-integration.md', 'doc/mcp-feature-coverage.md'],
        runbooks: ['Register MCP server', 'Restrict MCP tools', 'Review hosted MCP confirmation policy'],
        caveats: ['Stdio and sidecar MCP server transports are future extensions, not current deployment behavior.'],
      },
      {
        id: 'knowledge-context',
        title: 'Knowledge Context',
        level: 'Reference',
        audience: 'Automation authors, architects, and governance teams',
        summary:
          'Knowledge Context lets pipelines attach governed project knowledge to LLM-backed work and snapshots the resolved content with each run.',
        keyFacts: [
          'Supported kinds are architecture, guardrail, policy, adr, guideline, runbook, reference, and example.',
          'Knowledge may be declared at pipeline, step, and task level, then merged into effective task context.',
          'Managed documents require knowledge_context.use. Repo-local files are loaded from the run repository at the run commit.',
          'Guardrails and policies are strict prompt constraints for goals, commands, scripts, file writes, MCP calls, MCP arguments, and conditions.',
        ],
        details: [
          'A reference must use kind plus ref for managed knowledge, or kind plus a safe relative path for repo-local markdown.',
          'If a guardrail or policy conflicts with a requested action, the agent should explain the block rather than perform the prohibited action, and that block is treated as task failure.',
          'Effective context is merged from pipeline-level references, then step-level references, then task-level references. Required duplicates win over optional duplicates.',
          'Managed refs use knowledge_context.use authorization and are snapshotted into pipeline_run_knowledge_contexts so completed runs preserve exactly what the model saw.',
          'Policy snapshots are pinned by scope, then recomputed as pipeline, step, and task scopes start. Emergency policy response cancels active runs instead of mutating already-resolved policy.',
        ],
        configRows: knowledgeRows,
        examples: [
          {
            title: 'Knowledge references',
            language: 'yaml',
            code:
              'knowledge_context:\n  - kind: guardrail\n    ref: security/repository-policy\n    required: true\n  - kind: architecture\n    path: .nopsai/docs/backend.md\n    required: true',
          },
        ],
        relatedDocs: ['doc/knowledge-context.md', 'doc/sample-config-repo/README.md'],
        runbooks: ['Publish release knowledge context', 'Investigate required knowledge resolution failure', 'Review policy and guardrail changes'],
        caveats: ['Repo-local paths must be safe relative paths and cannot include path traversal.'],
      },
      {
        id: 'final-deliverables',
        title: 'Final Deliverables',
        level: 'Reference',
        audience: 'Automation authors and stakeholders consuming run results',
        summary:
          'Final outputs are generated after execution and can produce Markdown, JSON, PDF, HTML, Excel, and dashboard deliverables from completed run context.',
        keyFacts: [
          'Supported when values are success, failure, and always.',
          'Providers must return a single <final_output> envelope.',
          'Malformed, duplicate, empty, or missing envelopes fail validation and allow one corrective retry.',
          'PDF and HTML use validated DocumentSpec; Excel uses typed SpreadsheetSpec and rejects formulas and object/array cell values.',
          'Dashboard outputs publish a validated DashboardSpec into a team dashboard with replace, append, snapshot, or series behavior.',
          'Emitted step stdout/stderr, including plain log lines and JSON/NDJSON, is treated as authoritative dashboard evidence for artifact names, versions, durations, services, and subjects.',
          'Dashboard prompts are intent-driven: when the prompt does not name a visualization, NopsAI guides the model to choose text/callout, status/progress/properties, table, bar, line/area, or pie/donut based on the data shape.',
          'Generated dashboard sections[].blocks, sections[].widgets, top-level widgets, and nested blocks/widgets wrappers are normalized into flat DashboardSpec blocks before strict validation.',
          'Generated properties and display key aliases are normalized to DashboardSpec items and labels before strict validation.',
          'Generation duration is measured from generation_started_at, so queued outputs do not inherit time spent behind earlier outputs.',
          'Run lists, Pipeline Runs Overview, and related recent-run panels expose lightweight final-output status summaries without generated content.',
        ],
        details: [
          'PDF rendering uses Gotenberg through FINAL_OUTPUT_PDF_RENDERER_URL. Pipeline YAML never contains renderer infrastructure URLs.',
          'Provider and network failures are not retried by this feature. Contract and schema violations get one corrective retry.',
          'When output.items[].when is omitted or empty, the runtime treats it as always.',
          'Ready run-detail output rows are clickable preview toggles. Dashboard outputs also link to the configured dashboard and section when dashboard_target metadata is present.',
          'Dashboard output configuration is valid only when output.items[].type is dashboard. Non-dashboard outputs that set dashboard.* fields are rejected.',
          'DashboardSpec content is sanitized and validated before persistence. Generated HTML, CSS, JavaScript, iframes, forms, executable links, oversized tables, and oversized chart series are rejected for standard dashboard content.',
        ],
        configRows: finalOutputRows,
        examples: [
          {
            title: 'Final output configuration',
            language: 'yaml',
            code:
              'output:\n  llm_profile: report-writer\n  items:\n    - name: Executive summary\n      type: markdown\n      when: success\n      prompt: Summarize the successful run.\n    - name: Failure analysis\n      type: pdf\n      when: failure\n      prompt: Explain the failure and recommend corrective actions.',
          },
        ],
        relatedDocs: ['doc/final-output-rendering.md'],
        runbooks: ['Diagnose failed PDF render', 'Review final-output contract violations', 'Validate Excel deliverable schema'],
        caveats: ['Final-output failure is tracked separately from execution failure.'],
      },
    ],
  },
  {
    id: 'operations',
    title: 'Operations',
    owner: 'SRE and operations',
    description: 'Monitoring, metrics, logs, notifications, backups, cleanup, troubleshooting, and release integrity.',
    articles: [
      {
        id: 'monitoring-metrics',
        title: 'Monitoring and Metrics',
        level: 'Operate',
        audience: 'SREs, administrators, and reliability owners',
        summary:
          'Monitoring covers runs, pipelines, steps, tasks, triggers, external triggers, runners, LLM usage, reliability, efficiency, and security, with authorization-filtered responses.',
        keyFacts: [
          'Monitoring filters include time range, team, pipeline, repository, run ID, trigger source, status, subject identity, external trigger, schedule, duration, and AI dimensions.',
          'Monitoring responses are filtered through pipeline_run.list so users see only authorized runs.',
          'The API exposes Prometheus metrics at GET /metrics.',
          'Metrics cover run status, queue duration, final outputs, external triggers, LLM usage, runner utilization, approvals, audit, notifications, System Logs, and build identity.',
        ],
        details: [
          'The Helm API Service includes Prometheus scrape annotations by default.',
          'Use monitoring data to size runners and resource requests because the repository does not publish validated production sizing tiers.',
          'Monitoring aggregate endpoints filter candidate run IDs through AAA before aggregation, so charts and summaries stay aligned with the caller permissions.',
          'AI usage records include run, step, task, provider, model, profile, feature, prompt hashes, static cache keys, provider-state IDs/support, cached input tokens, cache-write tokens when providers expose them, revision markers, workspace retrieval sizes, and assistant chat dimensions where the runtime records them.',
        ],
        configRows: [
          {
            key: 'api.service.annotations.prometheus.io/scrape',
            area: 'Helm',
            description: 'Prometheus scrape annotation for the API Service.',
            example: 'true',
          },
        ],
        examples: [
          {
            title: 'Monitoring and metrics checks',
            language: 'bash',
            code:
              'nopsai api request GET /v1/monitoring/summary\nnopsai api request GET "/v1/monitoring/runs/analytics?status=failure"\nnopsai api request GET "/v1/monitoring/ai-usage?pipelineName=release"\nnopsai api request GET /v1/monitoring/recommendations\ncurl http://localhost:8080/metrics',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
            permission: 'pipeline_run.list plus monitoring route access',
          },
        ],
        relatedDocs: ['doc/system-logs.md', 'doc/enterprise-gates.md'],
        runbooks: ['Build runner capacity dashboard', 'Review AI usage by profile', 'Alert on approval wait time'],
        caveats: ['Do not infer production throughput from default Helm resource blocks; they are intentionally empty.'],
      },
      {
        id: 'system-logs',
        title: 'Pipeline Logs and System Logs',
        level: 'Troubleshoot',
        audience: 'Operators and support engineers',
        summary:
          'Pipeline logs are durable run records in PostgreSQL; System Logs are live allow-listed platform container or pod logs streamed through authenticated SSE.',
        keyFacts: [
          'System Logs allow-listed sources are nopsai, aaa, dispatcher, git-bot, ui, docker-runner, and k8s-runner.',
          'Docker deployments read through a least-privilege socket proxy.',
          'Kubernetes deployments read label-selected pods through read-only pods and pods/log RBAC.',
          'System Logs redaction is best effort and system_log.read should be narrowly assigned.',
        ],
        details: [
          'System Logs apply line and stream limits, buffer recent sanitized entries in memory, and do not copy platform logs into pipeline_run_logs.',
          'Build-only base, agent, and pipeline images are not exposed as System Logs sources.',
          'Source-level system_log.read permissions control who can list or stream each platform source.',
          'Docker health `none` means no container healthcheck exists; System Logs falls back to the container state, such as `running`, instead of showing `none` as a service status.',
        ],
        configRows: [
          {
            key: 'systemLogs.enabled',
            area: 'Helm',
            description: 'Enables System Logs support in the chart.',
            example: 'true',
          },
          {
            key: 'systemLogs.kubernetes.rbac.create',
            area: 'Helm',
            description: 'Creates read-only pods and pods/log permissions for API System Logs.',
            example: 'true',
          },
        ],
        examples: [
          {
            title: 'Stream dispatcher System Logs through the CLI',
            language: 'bash',
            code:
              'nopsai --timeout 0 api call GET \\\n  "/v1/system/logs/sources/{sourceID}/stream" \\\n  --path sourceID=dispatcher \\\n  --accept text/event-stream',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
            permission: 'system_log.read for dispatcher',
          },
        ],
        relatedDocs: ['doc/system-logs.md'],
        runbooks: ['Inspect dispatcher logs', 'Audit system_log.read grants', 'Tune System Logs line limits'],
        caveats: ['System Logs are live operational evidence, not durable pipeline history.'],
      },
      {
        id: 'notifications',
        title: 'Notifications',
        level: 'Operate',
        audience: 'Operations and team administrators',
        summary:
          'SMTP settings use credential references, and team notification policies route run and approval events by team, pipeline, repository, branch, recipients, exclusions, and throttle behavior.',
        keyFacts: [
          'Mail settings include enabled, from_address, smtp_host, smtp_port, smtp_start_tls, smtp_username, and smtp_password_credential_ref.',
          'Supported events include running, pending, success, failure, cancelled, approval requested, approval approved, and approval rejected.',
          'Notification policies can inherit through team lineage.',
          'Delivery records and dedupe behavior are persisted for operational review.',
        ],
        details: [
          'SMTP password is a credential reference, never plaintext configuration.',
          'Run team path is important for schedule and external-trigger notification routing.',
        ],
        configRows: [
          {
            key: 'setting/system/mail.yaml',
            area: 'GitOps',
            description: 'SMTP notification settings.',
            example: 'setting/system/mail.yaml',
          },
          {
            key: 'smtp_password_credential_ref',
            area: 'Mail',
            description: 'Credential reference for SMTP password.',
            example: 'credential://system/mail/smtp-primary',
          },
        ],
        examples: [],
        relatedDocs: ['doc/feature-reference.md', 'doc/team-resource-ownership-design.md'],
        runbooks: ['Test SMTP settings', 'Review notification delivery failures', 'Add production approval route'],
        caveats: ['Notification routing follows run ownership, not only pipeline source repository.'],
      },
      {
        id: 'backups-cleanup',
        title: 'Backups, Cleanup, and Restore Boundary',
        level: 'Operate',
        audience: 'Operators and data-management owners',
        summary:
          'The product can create, list, checksum, download, and clean up gzip-compressed JSON Lines backups, but a complete product-managed restore workflow is not currently implemented.',
        keyFacts: [
          'Backup types are full, runs, and logs.',
          'Cleanup targets are runs and logs.',
          'Cleanup modes include keep_last, older_than_days, all_terminal_runs, and all_logs.',
          'Cleanup may create a backup before deleting data and has scheduled job metadata.',
        ],
        details: [
          'Backup files are written with mode 0600 and the backup directory is created with mode 0700.',
          'Database recovery should use an operator-controlled, tested process until a formal NopsAI restore workflow exists.',
        ],
        configRows: [
          {
            key: '/data/backups',
            area: 'Filesystem',
            description: 'Default API container path for product-generated backup files.',
            example: '/data/backups',
          },
        ],
        examples: [],
        relatedDocs: ['doc/feature-reference.md', 'doc/enterprise-gates.md'],
        runbooks: ['Download and verify backup checksum', 'Run cleanup with pre-cleanup backup', 'Test external database restore'],
        caveats: ['Product backup files can disappear after container restart unless /data/backups is mounted or exported.'],
      },
      {
        id: 'release-integrity',
        title: 'Release Integrity and Compatibility',
        level: 'Admin',
        audience: 'Release engineers and platform operators',
        summary:
          'A production release publishes versioned images, a Helm chart, CLI archives, changelog, and checksums; deployment files are generated by the CLI for the selected version.',
        keyFacts: [
          'The install CLI generates Docker Compose files and Helm values from the selected semantic version.',
          'Do not combine services from different product versions or deploy floating tags.',
          'Advanced manifest-based Helm deploys remain available for internal CI workflows that explicitly provide a manifest.',
          'Release locks are GitOps-readable evidence for deployed version state.',
          'GET /version is public and returns build, API, CLI, runner, and capability identity without deployment secrets.',
          'The authenticated UI reads GET /version and shows a version-only footer at the far right of the app chrome.',
          'Breaking API, CLI, runner protocol, or deployment changes require a new major compatibility line.',
        ],
        details: [
          'Enterprise gates test release compatibility, versioned install generation, chart rendering, /version, and build metadata.',
          'The user-facing CLI binary is `nopsai`; the control-plane server binary is `nopsai-api`.',
        ],
        configRows: [
          {
            key: 'global.releaseVersion',
            area: 'Helm',
            description: 'Release version written into chart values.',
            example: '1.2.3',
          },
          {
            key: 'global.sourceCommit',
            area: 'Helm',
            description: 'Source commit associated with a release.',
            example: '494d57c',
          },
        ],
        examples: [
          {
            title: 'Version compatibility payload',
            language: 'json',
            code:
              '{\n  "productVersion": "2.7.0",\n  "apiVersion": "v1",\n  "cliCompatibility": ">=2.0.0 <3.0.0",\n  "runnerCompatibility": ">=2.0.0 <3.0.0",\n  "capabilities": [\n    "api.v1",\n    "cli.api-catalog.v1",\n    "config-sync.v1",\n    "mcp.v1",\n    "monitoring.v1",\n    "platform.helm",\n    "runner.docker",\n    "runner.kubernetes"\n  ]\n}',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
          },
        ],
        relatedDocs: ['doc/release-bundles.md', 'doc/cli.md', 'doc/enterprise-gates.md'],
        runbooks: ['Generate versioned install files', 'Render Helm deployment plan', 'Preserve deployment release lock'],
        caveats: ['Platform services should not be selected independently in production.'],
      },
    ],
  },
  {
    id: 'interfaces',
    title: 'Interfaces',
    owner: 'Developer experience',
    description: 'REST API, CLI, hosted MCP, API discovery, and support diagnostics.',
    articles: [
      {
        id: 'api',
        title: 'REST API',
        level: 'Reference',
        audience: 'Integration authors and support engineers',
        summary:
          'The API covers authentication, setup, teams, pipelines, runs, approvals, outputs, schedules, triggers, scopes, credentials, knowledge, profiles, monitoring, logs, dispatch, access, backups, cleanup, and hosted MCP.',
        keyFacts: [
          'Protected requests go through bearer authentication, route authorization, resource checks, and audit regardless of UI, CLI, or API origin.',
          'API categories include auth, setup, teams, pipelines, steps, runs, schedules, triggers, Git webhook sources, external triggers, scopes, credentials, knowledge, LLM, Agent, MCP, notifications, monitoring, System Logs, dispatcher, access, users, service accounts, identity providers, audit, backups, cleanup, and hosted MCP.',
          'Binary downloads and SSE responses are preserved by CLI generic API commands.',
          'Except for public setup and identity discovery endpoints, API requests require bearer authentication.',
          'Personal tokens and service-account tokens inherit the same authorization checks as browser sessions.',
        ],
        details: [
          'Route registration parity is tested against the generated CLI API catalog so new server APIs are discoverable through the CLI.',
          'Stored pipelines can be run by name through POST /v1/run/{pipelineName...}. Details, status, logs, approvals, reruns, cancellation, deletion, and output downloads remain separate authorized routes.',
        ],
        configRows: [],
        examples: [
          {
            title: 'Authenticate and call',
            language: 'bash',
            code:
              'curl -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  http://localhost:8080/v1/runs\n\ncurl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  http://localhost:8080/v1/system/config-repos/sync',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
          },
          {
            title: 'Run lifecycle routes',
            language: 'bash',
            code:
              'curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  http://localhost:8080/v1/run/team-1/services/api/deploy\n\ncurl -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  http://localhost:8080/v1/runs/<run-id>/logs\n\ncurl -OJ -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  http://localhost:8080/v1/runs/<run-id>/outputs/<output-id>/download',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
          },
        ],
        relatedDocs: ['doc/api.md', 'doc/cli.md'],
        runbooks: ['Validate API token', 'Describe a route from the CLI', 'Replay a safe read-only request'],
        caveats: ['Use route-level and resource-level authorization together; route access alone is not enough for runtime resource use.'],
      },
      {
        id: 'cli',
        title: 'CLI',
        level: 'Reference',
        audience: 'Operators and automation authors',
        summary:
          'The released `nopsai` CLI manages contexts, credentials, generic API access, platform diagnostics, completion files, GitOps deployment, generated Kubernetes install notes, and release verification.',
        keyFacts: [
          'Accepted token types are access JWTs, nopat_ personal tokens, and nopsat_ service-account tokens.',
          'Local config and credential files are atomically written with 0600 permissions inside a 0700 directory.',
          'The CLI rejects credential files with broader permissions.',
          'platform doctor checks local tools, API readiness, setup preflight, metrics, token acceptance, dispatcher monitoring, and runner count.',
          'The generated route catalog grants discovery only. api call and api request still traverse bearer authentication, route authorization, resource filtering, and compatibility checks.',
          'Interactive screens use the Contextual Zen terminal layout across the full CLI surface: fixed home-style header/footer chrome, centered control, breadcrumb above nested menus, fixed 20-row menu viewport for large lists, pinned guide/details section beneath the menu, bold standalone Guide/Example/Validation keys with indented content, inline Parameters progress lists on all live forms and wizards in step order, visible selected multiline values, Enter-to-skip blank optional parameters, fixed Result sections with breadcrumb scroll ranges, cursor-style empty active values, API catalog calls, raw API requests, route discovery, context management, token login/logout, install, platform doctor/release, completion, guides, help, and result viewers.',
          'Interactive actions pause on an equivalent `nopsai ...` command preview before API sends, local config changes, completion generation, platform checks/releases, install generation, Docker Compose startup, or Helm deployment begins.',
          'Generated Docker Compose installs reject the built-in development admin password, include dispatcher TLS in .env, and create bootstrap local admin credentials that are temporary by default and require first-login rotation.',
          'Generated Kubernetes installs include README.md with prerequisites, expected Secret keys, Secret creation examples, CLI deploy commands, direct Helm commands, and verification commands.',
          'Released CLIs check GET /version before mutating API requests. Development builds keep a deliberate bypass until release metadata is injected.',
        ],
        details: [
          'Optional missing local tools and missing dispatcher-read permission are warnings. API readiness, connectivity, malformed responses, metrics failures, and rejected tokens are errors.',
          'The CLI preserves binary downloads, streaming responses, YAML, JSON, and raw response bytes without implementing a separate API behavior layer.',
          'Interactive API workflows cover catalog calls, raw concrete-path requests, route filters, and route descriptions while reusing the same transport, AAA, compatibility, and output-file behavior as noninteractive commands.',
          'Shell completion is generated explicitly and does not silently modify shell startup files.',
        ],
        configRows: [
          {
            key: 'nopsai context add',
            area: 'CLI',
            description: 'Adds a named API context.',
            example: 'nopsai context add prod --api https://api.nopsai.example',
          },
          {
            key: 'nopsai platform doctor',
            area: 'CLI',
            description: 'Runs platform diagnostics.',
            example: 'nopsai platform doctor --output json',
          },
        ],
        examples: [
          {
            title: 'Context and login',
            language: 'bash',
            code:
              'nopsai platform doctor\nnopsai context add prod --api https://api.nopsai.example\nnopsai context use prod\nNOPSAI_TOKEN=nopat_<secret> nopsai login --token\nnopsai context list',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
          },
          {
            title: 'Generic API access',
            language: 'bash',
            code:
              'nopsai api routes --domain monitoring --method GET\nnopsai api routes --audience public --output json\nnopsai api describe GET "/v1/pipelines/{pipelineName...}"\nnopsai api call --interactive\nnopsai api call GET "/v1/runs/{runID}" --path runID=<run-id>\nnopsai api request GET /v1/monitoring/summary',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
          },
          {
            title: 'Completion and release operations',
            language: 'bash',
            code:
              'nopsai completion zsh --output-dir ./completion\nnopsai platform release --interactive\n\nnopsai install kubernetes --version 2.10.648 \\\n  --output-dir ./nopsai-prod \\\n  --values-file values.yaml',
            complete: false,
            testedIn: DEFAULT_VERIFIED_DATE,
          },
        ],
        relatedDocs: ['doc/cli.md', 'doc/release-bundles.md'],
        runbooks: ['Create production CLI context', 'Run platform doctor after deploy', 'Download support evidence safely'],
        caveats: ['Install or upgrade local Helm when release chart validation requires a newer Helm version.'],
      },
      {
        id: 'assistant-chat',
        title: 'Assistant Chat',
        level: 'Operate',
        audience: 'Operators, automation authors, and support engineers',
        summary:
          'The NopsAI Assistant is a docked and full-page chat interface that uses scoped LLM profiles plus permission-filtered hosted MCP tools.',
        keyFacts: [
          'Conversations persist per authenticated subject and keep bounded conversation memory for follow-up questions.',
          'Assistant turns use the selected or default LLM profile for planning and synthesis, then execute only validated hosted MCP tool calls for the current AAA subject.',
          'Generated YAML and configuration edits are proposals unless a confirmed mutation path is explicitly used.',
          'Docked chat can attach current-page route context so prompts like "explain this run" can resolve the visible target.',
        ],
        details: [
          'Current-page context contains metadata only: route, page area, active tab, team or scope, selected resource IDs, and allow-listed filters.',
          'The composer shows the attached context as a removable chip. Removing it omits that context from profile scoping and message sends.',
          'Assistant context never scrapes rendered page text, logs, secrets, credentials, or arbitrary query parameters.',
          'Explicit user targets override page context, and page context is preferred over older conversation memory.',
        ],
        configRows: [
          {
            key: 'assistant.default_llm_profile',
            area: 'System runner config',
            description: 'Default LLM profile used by assistant conversations when no chat-specific selection is made.',
            example: 'standard',
          },
        ],
        examples: [
          {
            title: 'Context-aware assistant question',
            language: 'text',
            code: 'Open a failed pipeline run, open Assistant, keep the context chip attached, and ask: "What is happening in this run?"',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
          },
        ],
        relatedDocs: ['doc/assistant-capabilities.md', 'doc/mcp-pipeline-integration.md'],
        runbooks: ['Ask the assistant to explain a failed run', 'Remove page context before asking a general question', 'Audit assistant MCP evidence'],
        caveats: ['If the planner is unavailable or invalid, no hosted MCP tools run and the assistant reports that no changes were applied.'],
      },
      {
        id: 'hosted-mcp-interface',
        title: 'Hosted NopsAI MCP Interface',
        level: 'Reference',
        audience: 'Assistant authors and automation integrators',
        summary:
          'NopsAI exposes itself as a first-party MCP server with tool and resource filtering backed by authenticated user permissions.',
        keyFacts: [
          'The endpoint is POST /v1/mcp.',
          'Supported JSON-RPC methods include initialize, tools/list, tools/call, resources/list, and resources/read.',
          'Capabilities include setup inspection, pipeline listing/search/validation, run analysis, approvals, schedules, knowledge, Git webhook sources, external triggers, config sync, notifications, monitoring, credentials metadata, runner controls, access, audit, users, service accounts, identity providers, backups, and cleanup.',
        ],
        details: [
          'Confirmed run operations and mutation tools require explicit confirmation, preserving enterprise change-control boundaries.',
          'Hosted tools and resources are listed and called as the current authenticated subject. GitOps write tools return proposals instead of silently applying config.',
          'Pipeline-run listing tools include lightweight final_output_status summaries for runs that define or store final outputs, but generated output content remains behind the authorized final-output read path.',
        ],
        configRows: [],
        examples: [
          {
            title: 'List hosted MCP tools through the CLI',
            language: 'bash',
            code:
              'printf \'{"jsonrpc":"2.0","id":1,"method":"tools/list"}\' | \\\n  nopsai api request POST /v1/mcp --data -\n\nnopsai api describe POST /v1/mcp',
            complete: true,
            testedIn: DEFAULT_VERIFIED_DATE,
            permission: 'Bearer subject access determines visible hosted MCP tools',
          },
        ],
        relatedDocs: ['doc/mcp-feature-coverage.md', 'doc/mcp-pipeline-integration.md'],
        runbooks: ['Review hosted MCP tool allow-list', 'Confirm a mutation action', 'Audit assistant-triggered recommendations'],
        caveats: ['Hosted MCP exposes only what the authenticated subject may see or do.'],
      },
    ],
  },
  {
    id: 'security-reference',
    title: 'Security and Reference',
    owner: 'Security and enterprise platform owners',
    description: 'Production hardening, troubleshooting, data model, feature combinations, and confirmed implementation gaps.',
    articles: [
      {
        id: 'production-hardening',
        title: 'Production Hardening Checklist',
        level: 'Security',
        audience: 'Enterprise administrators and security reviewers',
        summary:
          'Production mode requires strong independent secrets, dispatcher transport security, private control-plane networking, least-privilege runners, metrics, backups, and release locks.',
        keyFacts: [
          'Set NOPSAI_ENVIRONMENT=production or NOPSAI_REQUIRE_PRODUCTION_GATES=true.',
          'Replace every local fallback secret and use separate user/API and internal service JWT signing keys.',
          'Configure DISPATCHER_TLS_MODE=mtls or tls and protect the dispatcher trust seed.',
          'Keep PostgreSQL, AAA, Gotenberg, and Docker socket proxy private.',
          'Scrape /metrics, test SMTP, test LLM/MCP profiles, run platform doctor, and test recovery procedures.',
        ],
        details: [
          'Startup gates cover critical secret and transport requirements, but they do not replace infrastructure hardening, access reviews, external database backups, alerting, or recovery drills.',
        ],
        configRows: [
          {
            key: 'NOPSAI_ENVIRONMENT',
            area: 'Production gates',
            description: 'Enables production environment behavior.',
            example: 'production',
          },
          {
            key: 'NOPSAI_REQUIRE_PRODUCTION_GATES',
            area: 'Production gates',
            description: 'Fails closed when hardening gates are incomplete.',
            example: 'true',
          },
        ],
        examples: [],
        relatedDocs: ['doc/enterprise-gates.md', 'doc/license-compliance.md', 'doc/release-bundles.md'],
        runbooks: ['Run enterprise gates locally', 'Review production preflight', 'Perform quarterly access review'],
        caveats: ['Production gates are a baseline, not a full security program.'],
      },
      {
        id: 'troubleshooting',
        title: 'Troubleshooting Index',
        level: 'Troubleshoot',
        audience: 'Support and operations engineers',
        summary:
          'Common failures usually map to bootstrap configuration, dispatcher/runner reachability, storage, Git trigger matching, LLM/MCP validation, PDF rendering, GitOps drift, or backup durability.',
        keyFacts: [
          'Login setup preflight usually points to DATABASE_URL, NOPSAI_MASTER_KEY, or JWT_SIGNING_KEY.',
          'Runner registration failures usually involve dispatcher reachability, service JWT mismatch, TLS mode/trust mismatch, duplicate runner ID, invalid scopes, or missing Kubernetes RBAC.',
          'Runs remain queued when no runner is eligible or capacity is exhausted.',
          'Git webhook no-run outcomes often involve source auth, allowlists, trigger assignment, branch/path filters, synchronized pipelines, or resource access.',
          'PDF failures usually involve Gotenberg health, FINAL_OUTPUT_PDF_RENDERER_URL, timeout, or API-to-Gotenberg networking.',
        ],
        details: [
          'Config sync reverting a UI change means the resource is GitOps-managed. Commit the desired change to the owning config repository or push drift through review-branch workflow.',
        ],
        configRows: [],
        examples: [],
        relatedDocs: ['doc/system-logs.md', 'doc/git-webhook-sources.md', 'doc/final-output-rendering.md'],
        runbooks: ['Debug queued run', 'Investigate Kubernetes workspace mount failure', 'Resolve GitOps sync revert'],
        caveats: ['System Logs redaction is best effort; avoid attaching unrestricted logs to support cases.'],
      },
      {
        id: 'known-limits',
        title: 'Confirmed Gaps and Limits',
        level: 'Reference',
        audience: 'Product, sales, security, and implementation teams',
        summary:
          'The current implementation should not claim unsupported deployment automation, storage backends, autoscaling, air-gap, restore, sizing, or HA guarantees without new code and docs.',
        keyFacts: unsupportedClaims,
        details: [
          'Generic Git providers cannot fetch repository pipeline files in v1.',
          'Docker runners ignore Kubernetes runtime pools and affinity.',
          'Kubernetes emptyDir is not used for shared run workspaces.',
          'Credential values are intentionally not readable after submission.',
          'Final-output provider and network errors are not retried by final-output generation.',
        ],
        configRows: [],
        examples: [],
        relatedDocs: ['doc/README.md', 'doc/enterprise-refactor-roadmap.md'],
        runbooks: ['Check a sales or docs claim against implementation evidence', 'Track future-state capability in roadmap docs'],
        caveats: ['Future additions are valid roadmap items, but they should not appear in the current-version wiki as implemented behavior.'],
      },
    ],
  },
];

export const wikiSections: WikiSection[] = normalizeWikiSections(baseWikiSections);

export function summarizeWiki(sections: WikiSection[] = wikiSections): WikiSummary {
  const articles = sections.flatMap(section => section.articles);
  return {
    sections: sections.length,
    articles: articles.length,
    configKeys: collectWikiConfigKeys(sections).length,
    runbooks: new Set(articles.flatMap(article => article.runbooks)).size,
    caveats: articles.reduce((total, article) => total + article.caveats.length, 0),
    tutorials: articles.filter(article => article.docType === 'tutorial').length,
    proceduralPages: articles.filter(article => article.steps.length > 0 || article.prerequisites.length > 0).length,
    sourceLinks: articles.reduce((total, article) => total + article.sourceLinks.length, 0),
  };
}

export function collectWikiConfigKeys(sections: WikiSection[] = wikiSections) {
  return Array.from(new Set(sections.flatMap(section => section.articles.flatMap(article => article.configRows.map(row => row.key))))).sort();
}

export function getFirstWikiArticleID(sections: WikiSection[] = wikiSections) {
  return sections[0]?.articles[0]?.id || '';
}

export function findWikiArticle(sections: WikiSection[], articleID: string) {
  for (const section of sections) {
    const article = section.articles.find(candidate => candidate.id === articleID);
    if (article) return article;
  }
  return undefined;
}

export function findWikiSectionForArticle(sections: WikiSection[], articleID: string) {
  return sections.find(section => section.articles.some(article => article.id === articleID));
}

export function filterWikiSections(sections: WikiSection[], query: string) {
  const normalized = normalizeWikiQuery(query);
  if (!normalized) return sections;

  return sections
    .map(section => {
      const sectionMatches = sectionHaystack(section).includes(normalized);
      const articles = section.articles.filter(article => sectionMatches || articleHaystack(article).includes(normalized));
      return { ...section, articles };
    })
    .filter(section => section.articles.length > 0);
}

export function countWikiArticles(sections: WikiSection[]) {
  return sections.reduce((total, section) => total + section.articles.length, 0);
}

function normalizeWikiQuery(query: string) {
  return query.trim().toLowerCase();
}

function sectionHaystack(section: WikiSection) {
  return [section.id, section.title, section.owner, section.description].join(' ').toLowerCase();
}

function articleHaystack(article: WikiArticle) {
  return [
    article.id,
    article.title,
    article.level,
    article.audience,
    article.docType,
    ...article.audiences,
    article.metadata.appliesTo,
    article.metadata.owner,
    article.metadata.introducedIn,
    article.metadata.lastVerified,
    article.metadata.sourceCommit,
    article.metadata.status,
    article.summary,
    ...article.keyFacts,
    ...article.details,
    ...article.prerequisites.flatMap(item => [item.label, item.value, item.verification || '']),
    ...article.steps.flatMap(step => [
      step.title,
      step.description,
      step.expectedOutput || '',
      step.verification || '',
      step.warning || '',
      ...(step.commands || []).flatMap(command => [
        command.title,
        command.language,
        command.code,
        command.expectedOutput || '',
        command.validationCommand || '',
      ]),
    ]),
    ...article.configRows.flatMap(row => [
      row.key,
      row.path || '',
      row.area,
      row.type || '',
      String(row.required ?? ''),
      row.defaultValue || '',
      ...(row.allowedValues || []),
      row.scope || '',
      row.description,
      ...(row.constraints || []),
      ...(row.inheritedFrom || []),
      row.permission || '',
      row.introducedIn || '',
      row.deprecatedIn || '',
      row.security || '',
      row.example,
    ]),
    ...article.examples.flatMap(example => [
      example.title,
      example.language,
      example.code,
      example.expectedOutput || '',
      ...(example.placeholderNotes || []),
      example.testedIn || '',
      example.permission || '',
      example.validationCommand || '',
      example.rollback || '',
    ]),
    ...article.relatedDocs,
    ...article.sourceLinks.flatMap(source => [
      source.title,
      source.repositoryPath,
      source.sourceUrl || '',
      source.sourceLines || '',
      source.purpose,
    ]),
    ...article.runbooks,
    ...article.runbookEntries.flatMap(runbook => [
      runbook.id,
      runbook.title,
      ...runbook.symptoms,
      runbook.impact,
      runbook.requiredAccess,
      ...runbook.initialChecks,
      ...runbook.diagnosticCommands,
      ...runbook.resolution,
      runbook.rollback || '',
      runbook.escalation || '',
      ...(runbook.metrics || []),
    ]),
    ...article.caveats,
  ]
    .join(' ')
    .toLowerCase();
}
