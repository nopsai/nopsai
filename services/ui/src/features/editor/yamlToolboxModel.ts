export type YamlEditorResourceKind = 'pipeline' | 'step' | 'trigger';

export type YamlToolboxParameter = {
  key: string;
  description: string;
  valueHint?: string;
  validValues?: string[];
  structure?: string;
};

export type YamlToolboxParameterGroup = {
  id: string;
  title: string;
  description: string;
  parameters: YamlToolboxParameter[];
};

export type YamlToolboxSnippet = {
  id: string;
  label: string;
  description: string;
  yaml: string;
};

export type YamlToolboxSnippetGroup = {
  id: string;
  title: string;
  description: string;
  snippets: YamlToolboxSnippet[];
};

export type YamlToolboxSample = {
  title: string;
  yaml: string;
};

export type YamlToolboxSpec = {
  title: string;
  parameterGroups: YamlToolboxParameterGroup[];
  snippetGroups: YamlToolboxSnippetGroup[];
  samples: YamlToolboxSample[];
};

export type YamlSnippetInsertResult = {
  nextValue: string;
  nextCursor: number;
};

const BOOLEAN_VALUES = ['true', 'false'];
const POLICY_VALUES = ['restrictive', 'override', 'fail_on_conflict'];

const pipelineParameterGroups: YamlToolboxParameterGroup[] = [
  {
    id: 'pipeline-root',
    title: 'Pipeline Parameters',
    description: 'Top-level fields accepted by pipeline and Lab YAML.',
    parameters: [
      { key: 'name', description: 'Stable pipeline name.', valueHint: 'letters, numbers, dots, underscores, hyphens' },
      { key: 'version', description: 'Optional schema or release version.', valueHint: 'latest or semantic version' },
      { key: 'description', description: 'Human-readable pipeline summary.', valueHint: 'plain text or block scalar' },
      { key: 'container_image', description: 'Default image for executable steps.', valueHint: 'image:tag' },
      { key: 'working_directory', description: 'Run workspace directory.', valueHint: '/workspace or relative path under /workspace' },
      { key: 'variables', description: 'Run variable references.', structure: 'variables: [ENVIRONMENT, prod:IMAGE_TAG]' },
      { key: 'steps', description: 'Ordered list of step objects.', structure: 'steps:\n  - name: build\n    script: make build' },
      { key: 'timeout', description: 'Whole-run timeout.', valueHint: 'Go duration, for example 45m or 2h' },
      { key: 'llm_enabled', description: 'Allows goal, condition, MCP, and final-output LLM behavior.', validValues: BOOLEAN_VALUES },
      { key: 'agent_profile', description: 'Pipeline-level AI persona.', valueHint: 'approved agent profile name' },
      { key: 'llm_profile', description: 'Pipeline-level model profile.', valueHint: 'approved LLM profile name' },
      { key: 'mcp_profiles', description: 'Approved MCP tool profiles for LLM goals.', structure: 'mcp_profiles: [github-readonly]' },
      { key: 'policy_merge_mode', description: 'Policy conflict behavior.', validValues: POLICY_VALUES },
      { key: 'runtime_pool', description: 'Kubernetes runtime pool default. Docker ignores it.', valueHint: 'configured runtime pool name' },
      { key: 'affinity_enabled', description: 'Kubernetes same-node affinity override.', validValues: BOOLEAN_VALUES },
      { key: 'knowledge_context', description: 'Governed knowledge refs.', structure: '- kind: guardrail\n  ref: security/release' },
      { key: 'output', description: 'Final deliverables and dashboard publications.', structure: 'output:\n  items:\n    - name: summary\n      type: markdown' },
      { key: 'llm_output_sharing', description: 'Default output-history sharing for LLM work.', validValues: BOOLEAN_VALUES },
      { key: 'llm_content_sharing', description: 'Share workspace files with LLM goals.', validValues: BOOLEAN_VALUES },
      { key: 'llm_content_include', description: 'Only share matching workspace paths with LLM goals.', structure: 'llm_content_include:\n  - src/**\n  - README.md' },
      { key: 'llm_content_ignore', description: 'Exclude matching workspace paths from LLM context.', structure: 'llm_content_ignore:\n  - .git\n  - node_modules/**' },
      { key: 'display_options', description: 'UI rendering preferences for pipeline output.', structure: 'display_options:\n  github_view: list' },
    ],
  },
];

const stepParameterGroup: YamlToolboxParameterGroup = {
  id: 'step-structure',
  title: 'Step Parameters',
  description: 'A step must use exactly one mode: include, tasks, goal, script, or approval.',
  parameters: [
    { key: 'name', description: 'Required unique step name.' },
    { key: 'description', description: 'Reusable step summary shown in the library.' },
    { key: 'include', description: 'Reusable step or child pipeline reference.', valueHint: 'step:team/shared or pipeline:team/child' },
    { key: 'sync', description: 'Wait for child pipeline completion.', validValues: BOOLEAN_VALUES },
    { key: 'approval', description: 'Human checkpoint.', structure: 'approval:\n  type: production-deploy\n  teams: [platform/prod]' },
    { key: 'image', description: 'Step image override.', valueHint: 'image:tag' },
    { key: 'secrets', description: 'Secret refs injected into the step.', structure: 'secrets: [DEPLOY_TOKEN, prod:REGISTRY_PASSWORD]' },
    { key: 'volumes', description: 'Volume or PVC mounts.', structure: 'volumes: [cache:/cache]' },
    { key: 'variables', description: 'Step-level variable overrides.', structure: 'variables:\n  RELEASE_CHANNEL: canary' },
    { key: 'tasks', description: 'Nested executable task list.', structure: 'tasks:\n  - name: test\n    script: make test' },
    { key: 'condition', description: 'LLM condition before the step runs.' },
    { key: 'goal', description: 'Single LLM goal step.' },
    { key: 'script', description: 'Single script step.' },
    { key: 'depends_on', description: 'Upstream steps or qualified producer tasks.', structure: 'depends_on: [build, prepare.generate-tag]' },
    { key: 'outputs', description: 'Runtime outputs produced by the step.', structure: 'outputs:\n  - name: image_tag' },
    { key: 'ignore_failure', description: 'Allow downstream progress when this step fails.', validValues: BOOLEAN_VALUES },
    { key: 'agent_profile', description: 'Step persona override.' },
    { key: 'llm_profile', description: 'Step model profile override.' },
    { key: 'mcp_profiles', description: 'Additional MCP profiles for goal tasks.', structure: 'mcp_profiles: [github-readonly]' },
    { key: 'policy_merge_mode', description: 'Policy conflict behavior.', validValues: POLICY_VALUES },
    { key: 'runtime_pool', description: 'Step Kubernetes runtime pool override.' },
    { key: 'knowledge_context', description: 'Step-level governed knowledge refs.' },
    { key: 'llm_output_sharing', description: 'Share step LLM output with later LLM history.', validValues: BOOLEAN_VALUES },
    { key: 'artifacts', description: 'Reusable step artifacts collected after execution.', structure: 'artifacts:\n  - dist/**' },
    { key: 'access', description: 'Reusable step access metadata.', structure: 'access:\n  teams:\n    - platform' },
  ],
};

const taskParameterGroup: YamlToolboxParameterGroup = {
  id: 'task-structure',
  title: 'Task Parameters',
  description: 'Tasks live inside steps[].tasks and each task uses either goal or script.',
  parameters: [
    { key: 'name', description: 'Required unique task name within the step.' },
    { key: 'goal', description: 'LLM-backed task goal.' },
    { key: 'script', description: 'Shell script task.' },
    { key: 'depends_on', description: 'Same-step task names or qualified upstream producer tasks.', structure: 'depends_on: [install]' },
    { key: 'outputs', description: 'Runtime outputs produced by the task.', structure: 'outputs:\n  - name: image_tag' },
    { key: 'ignore_failure', description: 'Treat this task failure as ignored.', validValues: BOOLEAN_VALUES },
    { key: 'llm_profile', description: 'Most-specific model profile for a goal task.' },
    { key: 'mcp_profiles', description: 'Additional MCP profiles for this goal task.', structure: 'mcp_profiles: [github-readonly]' },
    { key: 'policy_merge_mode', description: 'Policy conflict behavior.', validValues: POLICY_VALUES },
    { key: 'variables', description: 'Task-local variable overrides.', structure: 'variables:\n  TEST_SUITE: smoke' },
    { key: 'knowledge_context', description: 'Task-level governed knowledge refs.' },
    { key: 'llm_output_sharing', description: 'Share this task output with later LLM history.', validValues: BOOLEAN_VALUES },
  ],
};

const triggerParameterGroups: YamlToolboxParameterGroup[] = [
  {
    id: 'trigger-root',
    title: 'Trigger Parameters',
    description: 'Top-level fields for a Git trigger manifest.',
    parameters: [
      { key: 'provider', description: 'Git provider.', validValues: ['github', 'gitlab', 'bitbucket', 'gitea', 'generic'] },
      { key: 'team', description: 'Run ownership team path.', valueHint: 'platform/service' },
      { key: 'team_path', description: 'Alternate team path field accepted by trigger manifests.', valueHint: 'platform/service' },
      { key: 'webhook_source', description: 'Managed webhook source for non-GitHub providers.' },
      { key: 'management', description: 'Manifest ownership mode.', validValues: ['nopsai', 'repository'] },
      { key: 'triggers', description: 'List of event rules.', structure: 'triggers:\n  - on: push\n    pipelines: [platform/api-ci]' },
    ],
  },
  {
    id: 'trigger-rule',
    title: 'Trigger Rule Structure',
    description: 'Fields accepted inside triggers[].',
    parameters: [
      { key: 'on', description: 'Event type.', validValues: ['push', 'pull_request', 'schedule'] },
      { key: 'branches', description: 'Branch allowlist globs.', structure: 'branches: [main, release/*]' },
      { key: 'skip_branches', description: 'Branch denylist globs.', structure: 'skip_branches: [wip/*]' },
      { key: 'tags', description: 'Tag allowlist globs.', structure: 'tags: [v*]' },
      { key: 'include_paths', description: 'Changed-file include globs.', structure: 'include_paths:\n  - services/api/**' },
      { key: 'exclude_paths', description: 'Changed-file exclude globs.', structure: 'exclude_paths:\n  - docs/**' },
      { key: 'pipelines', description: 'Stored pipelines to run.', structure: 'pipelines:\n  - platform/api-ci' },
      { key: 'scope', description: 'Run scope used for variables and secrets.', valueHint: 'prod or platform/prod' },
    ],
  },
];

const pipelineSnippetGroups: YamlToolboxSnippetGroup[] = [
  {
    id: 'step-snippets',
    title: 'Step Structures',
    description: 'Insert a valid step mode into a pipeline steps list.',
    snippets: [
      {
        id: 'script-step',
        label: 'Script step',
        description: 'A direct shell step.',
        yaml: '- name: build\n  image: alpine:3.20\n  script: |\n    echo "build"',
      },
      {
        id: 'goal-step',
        label: 'Goal step',
        description: 'A single LLM-backed goal step.',
        yaml: '- name: review\n  goal: Review the release readiness and summarize risks.',
      },
      {
        id: 'approval-step',
        label: 'Approval step',
        description: 'A human approval checkpoint.',
        yaml: '- name: production-approval\n  approval:\n    type: production-deploy\n    teams:\n      - platform/prod\n    allow_self_approval: false',
      },
      {
        id: 'reusable-step',
        label: 'Reusable step',
        description: 'Include a reusable step from the step library.',
        yaml: '- name: notify\n  include: step:platform/shared/notify',
      },
      {
        id: 'child-pipeline',
        label: 'Child pipeline',
        description: 'Start another pipeline from this step.',
        yaml: '- name: deploy-child\n  include: pipeline:platform/deploy\n  sync: true',
      },
    ],
  },
  {
    id: 'task-snippets',
    title: 'Task Structures',
    description: 'Insert tasks under a step tasks list.',
    snippets: [
      {
        id: 'script-task',
        label: 'Script task',
        description: 'A shell task inside tasks.',
        yaml: '- name: unit-tests\n  script: |\n    npm test',
      },
      {
        id: 'goal-task',
        label: 'Goal task',
        description: 'An LLM task that can use approved profiles.',
        yaml: '- name: risk-review\n  depends_on: [unit-tests]\n  llm_profile: standard\n  goal: Summarize reliability risk from the test output.',
      },
      {
        id: 'output-task',
        label: 'Output task',
        description: 'A task that declares a runtime output.',
        yaml: '- name: package\n  script: |\n    echo "IMAGE_TAG=dev" >> "$NOPSAI_OUTPUTS_FILE"\n  outputs:\n    - name: IMAGE_TAG',
      },
    ],
  },
  {
    id: 'pipeline-snippets',
    title: 'Pipeline Blocks',
    description: 'Top-level blocks that are commonly added while authoring.',
    snippets: [
      {
        id: 'variables',
        label: 'Variables',
        description: 'Pipeline variable refs.',
        yaml: 'variables:\n  - ENVIRONMENT\n  - prod:IMAGE_TAG',
      },
      {
        id: 'final-output',
        label: 'Final output',
        description: 'Markdown final output delivered after a run.',
        yaml: 'output:\n  items:\n    - name: release-summary\n      type: markdown\n      when: success\n      prompt: Summarize the release evidence.',
      },
    ],
  },
];

const stepSnippetGroups: YamlToolboxSnippetGroup[] = [
  {
    id: 'step-root',
    title: 'Step Structures',
    description: 'Insert a valid reusable step shape.',
    snippets: [
      {
        id: 'script-step-root',
        label: 'Script step',
        description: 'Reusable direct shell step.',
        yaml: 'name: build-image\ndescription: Build the service image.\nimage: docker:27\nscript: |\n  docker build -t "$IMAGE_TAG" .',
      },
      {
        id: 'task-step-root',
        label: 'Task list step',
        description: 'Reusable step with multiple tasks.',
        yaml: 'name: verify\ndescription: Install dependencies and run tests.\ntasks:\n  - name: install\n    script: npm ci\n  - name: test\n    depends_on: [install]\n    script: npm test',
      },
      {
        id: 'goal-step-root',
        label: 'Goal step',
        description: 'Reusable LLM-backed step.',
        yaml: 'name: release-review\ndescription: Review release evidence.\nllm_profile: standard\ngoal: Review release readiness and summarize risks.',
      },
    ],
  },
  {
    id: 'step-task-snippets',
    title: 'Task Structures',
    description: 'Add tasks to a reusable step.',
    snippets: [
      {
        id: 'step-script-task',
        label: 'Script task',
        description: 'Shell task inside a reusable step.',
        yaml: '- name: lint\n  script: npm run lint',
      },
      {
        id: 'step-goal-task',
        label: 'Goal task',
        description: 'LLM task inside a reusable step.',
        yaml: '- name: summarize\n  depends_on: [lint]\n  goal: Summarize the lint result and next actions.',
      },
    ],
  },
];

const triggerSnippetGroups: YamlToolboxSnippetGroup[] = [
  {
    id: 'trigger-manifest',
    title: 'Trigger Structures',
    description: 'Insert complete or partial trigger manifest blocks.',
    snippets: [
      {
        id: 'trigger-root',
        label: 'Manifest header',
        description: 'Provider and team ownership fields.',
        yaml: 'provider: gitlab\nteam: platform\nwebhook_source: gitlab-platform',
      },
      {
        id: 'push-trigger',
        label: 'Push rule',
        description: 'Run a pipeline on branch pushes.',
        yaml: '- on: push\n  branches: [main]\n  include_paths:\n    - services/api/**\n  pipelines:\n    - platform/api-ci\n  scope: platform/prod',
      },
      {
        id: 'pr-trigger',
        label: 'Pull request rule',
        description: 'Run validation on pull requests.',
        yaml: '- on: pull_request\n  branches: [main]\n  pipelines:\n    - platform/pr-check\n  scope: platform/dev',
      },
      {
        id: 'trigger-list',
        label: 'Triggers list',
        description: 'Top-level triggers array with one push rule.',
        yaml: 'triggers:\n  - on: push\n    branches: [main]\n    pipelines:\n      - platform/api-ci\n    scope: platform/prod',
      },
    ],
  },
];

const pipelineSamples: YamlToolboxSample[] = [
  {
    title: 'Small script pipeline',
    yaml: 'name: shell-check\nllm_enabled: false\ncontainer_image: alpine:3.20\nsteps:\n  - name: lint\n    script: ./scripts/lint.sh',
  },
  {
    title: 'Step with tasks',
    yaml: 'steps:\n  - name: verify\n    tasks:\n      - name: install\n        script: npm ci\n      - name: test\n        depends_on: [install]\n        script: npm test',
  },
];

const stepSamples: YamlToolboxSample[] = [
  {
    title: 'Reusable script step',
    yaml: 'name: notify\ndescription: Send a notification.\nscript: |\n  echo "done"',
  },
];

const triggerSamples: YamlToolboxSample[] = [
  {
    title: 'GitLab push trigger',
    yaml: 'provider: gitlab\nteam: platform\nwebhook_source: gitlab-platform\ntriggers:\n  - on: push\n    branches: [main]\n    pipelines:\n      - platform/api-ci\n    scope: platform/prod',
  },
];

export function getYamlToolboxSpec(kind: YamlEditorResourceKind): YamlToolboxSpec {
  if (kind === 'trigger') {
    return {
      title: 'Trigger Toolbox',
      parameterGroups: triggerParameterGroups,
      snippetGroups: triggerSnippetGroups,
      samples: triggerSamples,
    };
  }

  if (kind === 'step') {
    return {
      title: 'Step Toolbox',
      parameterGroups: [stepParameterGroup, taskParameterGroup],
      snippetGroups: stepSnippetGroups,
      samples: stepSamples,
    };
  }

  return {
    title: 'Pipeline Toolbox',
    parameterGroups: [...pipelineParameterGroups, stepParameterGroup, taskParameterGroup],
    snippetGroups: pipelineSnippetGroups,
    samples: pipelineSamples,
  };
}

export function insertYamlSnippetAtCursor(
  value: string,
  selectionStart: number,
  selectionEnd: number,
  snippet: string
): YamlSnippetInsertResult {
  const text = typeof value === 'string' ? value : '';
  const start = clampIndex(selectionStart, text.length);
  const end = clampIndex(selectionEnd, text.length);
  const rangeStart = Math.min(start, end);
  const rangeEnd = Math.max(start, end);
  const normalizedSnippet = String(snippet || '').replace(/\r\n/g, '\n').replace(/\n+$/, '');

  if (!normalizedSnippet) {
    return { nextValue: text, nextCursor: rangeEnd };
  }

  const lineStart = text.lastIndexOf('\n', Math.max(0, rangeStart - 1)) + 1;
  const nextLineBreak = text.indexOf('\n', rangeEnd);
  const lineEnd = nextLineBreak === -1 ? text.length : nextLineBreak;
  const lineBeforeSelection = text.slice(lineStart, rangeStart);
  const lineAfterSelection = text.slice(rangeEnd, lineEnd);
  const indent = lineBeforeSelection.match(/^\s*/)?.[0] ?? '';
  const isBlankLineSelection =
    rangeStart === rangeEnd &&
    lineBeforeSelection.trim().length === 0 &&
    lineAfterSelection.trim().length === 0;
  const preparedSnippet = normalizedSnippet
    .split('\n')
    .map(line => (line && indent ? `${indent}${line}` : line))
    .join('\n');

  if (isBlankLineSelection) {
    const before = text.slice(0, lineStart);
    const after = text.slice(lineEnd);
    return {
      nextValue: `${before}${preparedSnippet}${after}`,
      nextCursor: before.length + preparedSnippet.length,
    };
  }

  const before = text.slice(0, rangeStart);
  const after = text.slice(rangeEnd);
  const prefix = before.length > 0 && !before.endsWith('\n') ? '\n' : '';
  const suffix = after.length > 0 && !after.startsWith('\n') ? '\n' : '';
  const insert = `${prefix}${preparedSnippet}${suffix}`;

  return {
    nextValue: `${before}${insert}${after}`,
    nextCursor: before.length + prefix.length + preparedSnippet.length,
  };
}

function clampIndex(index: number, length: number) {
  if (!Number.isFinite(index)) return 0;
  return Math.max(0, Math.min(Math.floor(index), length));
}
