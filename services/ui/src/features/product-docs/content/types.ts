/**
 * Product wiki content model.
 *
 * Every value in this model is authored. Nothing here is inferred from prose or
 * generated from a title: an article that does not carry a field reference, a
 * runbook, or implementation evidence renders without that block instead of
 * rendering a placeholder. See `doc/wiki` for the repository-side source map.
 */

/** Reader groups the wiki is written for. */
export type WikiAudience =
  | 'new-user'
  | 'automation-author'
  | 'operator'
  | 'administrator'
  | 'security'
  | 'developer';

/** Purpose of a page, independent of who reads it. */
export type WikiDocType = 'tutorial' | 'how-to' | 'concept' | 'reference' | 'runbook' | 'troubleshooting';

/** Top-level shelf a section belongs to on the wiki home page. */
export type WikiGroup = 'learn' | 'build' | 'run' | 'look-up';

export type WikiStatus = 'current' | 'preview' | 'deprecated';

/**
 * One documented field: a YAML directive, environment variable, API route,
 * CLI flag, or settings key.
 *
 * `type`, `required` and `defaultValue` are required because a field reference
 * that cannot answer "what do I put here and do I have to?" is not a reference.
 */
export type WikiField = {
  /** Fully qualified path, e.g. `steps[].approval.timeout` or `DISPATCHER_TLS_MODE`. */
  path: string;
  /** Where the field is valid, e.g. `pipeline`, `step`, `task`, `api`, `runner`. */
  scope: string;
  type: string;
  required: boolean | 'conditional';
  /** Literal default, or `None` when unset means absent. */
  defaultValue: string;
  description: string;
  example: string;
  allowedValues?: string[];
  /** Hard rules the code enforces: regexes, limits, mutual exclusivity. */
  constraints?: string[];
  /** More specific levels that override this value, most specific last. */
  overriddenBy?: string[];
  /** AAA action or role needed to set or use the field. */
  permission?: string;
  /** Why the field matters to a security or compliance reviewer. */
  security?: string;
  deprecatedIn?: string;
  /** Repository path that proves the documented behavior. */
  evidence?: string;
};

/** Access class of a REST route, before resource-level AAA checks. */
export type WikiRouteAccess =
  /** No token required. */
  | 'public'
  /** Any authenticated user, service account, or personal access token. */
  | 'authenticated'
  /** Authenticated plus an AAA action check on the target resource. */
  | 'authorized'
  /** Platform administrator role. */
  | 'admin'
  /** Internal service token only; not part of the public surface. */
  | 'service';

export type WikiApiRoute = {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'ANY';
  path: string;
  /** Functional area, used to group the API index. */
  area: string;
  access: WikiRouteAccess;
  purpose: string;
  /** AAA action or role the handler checks, when the route names one. */
  permission?: string;
  notes?: string;
};

export const wikiRouteAccessLabels: Record<WikiRouteAccess, string> = {
  public: 'Public',
  authenticated: 'Authenticated',
  authorized: 'Authorized',
  admin: 'Administrator',
  service: 'Service token',
};

export type WikiExample = {
  title: string;
  language: string;
  code: string;
  /** What the reader should see when the example works. */
  expectedOutput?: string;
  /** Placeholders the reader must replace before running it. */
  placeholders?: string[];
  validationCommand?: string;
};

export type WikiPrerequisite = {
  label: string;
  value: string;
  /** Command or check that confirms the prerequisite is met. */
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

/**
 * An operational response procedure. Only authored runbooks exist: a runbook
 * without diagnostics and resolution steps is not published.
 */
export type WikiRunbook = {
  id: string;
  title: string;
  symptoms: string[];
  impact: string;
  requiredAccess: string;
  initialChecks: string[];
  diagnostics: string[];
  resolution: string[];
  rollback?: string;
  escalation?: string;
};

/** Repository evidence for a claim. Rendered as a path, never as a link. */
export type WikiSource = {
  repositoryPath: string;
  purpose: string;
};

export type WikiArticle = {
  id: string;
  title: string;
  docType: WikiDocType;
  audiences: WikiAudience[];
  /** One sentence answering "what is this page for?". Shown in search and cards. */
  summary: string;
  /** Extra terms that should match this page in search but do not appear in the copy. */
  keywords?: string[];
  keyFacts: string[];
  details: string[];
  prerequisites?: WikiPrerequisite[];
  steps?: WikiStep[];
  fields?: WikiField[];
  examples?: WikiExample[];
  runbooks?: WikiRunbook[];
  /** Confirmed current limits. Not a roadmap. */
  limits?: string[];
  /** Article IDs worth reading next. */
  related?: string[];
  sources?: WikiSource[];
  status?: WikiStatus;
};

export type WikiSection = {
  id: string;
  title: string;
  group: WikiGroup;
  owner: string;
  /** One sentence answering "what will I find in here?". */
  description: string;
  articles: WikiArticle[];
};

export const wikiGroupLabels: Record<WikiGroup, string> = {
  learn: 'Learn the product',
  build: 'Build automation',
  run: 'Run the platform',
  'look-up': 'Look something up',
};

export const wikiGroupDescriptions: Record<WikiGroup, string> = {
  learn: 'What NopsAI is, how a run actually executes, and guided first tasks.',
  build: 'Authoring pipelines, wiring triggers, and configuring the AI control layers.',
  run: 'Installing, administering, and operating the platform day to day.',
  'look-up': 'Complete indexes, interface contracts, and confirmed limits.',
};

export const wikiGroupOrder: WikiGroup[] = ['learn', 'build', 'run', 'look-up'];

export const wikiDocTypeLabels: Record<WikiDocType, string> = {
  tutorial: 'Tutorial',
  'how-to': 'How-to',
  concept: 'Concept',
  reference: 'Reference',
  runbook: 'Runbook',
  troubleshooting: 'Troubleshooting',
};

export const wikiAudienceLabels: Record<WikiAudience, string> = {
  'new-user': 'New user',
  'automation-author': 'Automation author',
  operator: 'Operator',
  administrator: 'Administrator',
  security: 'Security',
  developer: 'Developer',
};

export function wikiDocTypeLabel(docType: WikiDocType) {
  return wikiDocTypeLabels[docType];
}

export function wikiAudienceLabel(audience: WikiAudience) {
  return wikiAudienceLabels[audience];
}

export function formatWikiRequired(required: WikiField['required']) {
  if (required === true) return 'Required';
  if (required === false) return 'Optional';
  return 'Conditional';
}

export function wikiHomePath() {
  return '/docs';
}

export function wikiArticlePath(sectionID: string, articleID: string) {
  return `/docs/${encodeURIComponent(sectionID)}/${encodeURIComponent(articleID)}`;
}

export function wikiFieldAnchor(path: string) {
  return `field-${wikiSlug(path)}`;
}

export function wikiSlug(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/\[\]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}
