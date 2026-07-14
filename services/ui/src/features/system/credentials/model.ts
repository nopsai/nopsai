import { asRecord, readOptionalString, readString } from '../data.js';

export const CREDENTIAL_KINDS = [
  'api_key',
  'password',
  'bearer_token',
  'private_key',
  'webhook_secret',
  'client_secret',
] as const;

export type CredentialKind = (typeof CREDENTIAL_KINDS)[number];

export type CredentialVersionRecord = {
  version: number;
  created_by?: string;
  created_at: string;
  activated_at?: string;
  revoked_at?: string;
};

export type CredentialRecord = {
  id: string;
  reference: string;
  kind: string;
  description: string;
  status: string;
  has_value: boolean;
  active_version: number;
  expires_at?: string;
  last_rotated_at?: string;
  managed_by_config_repo: boolean;
  config_source_path?: string;
  config_source_commit_sha?: string;
  created_by?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
  versions: CredentialVersionRecord[];
};

export type CredentialFormState = {
  namespace: string;
  team_path: string;
  name: string;
  kind: CredentialKind;
  description: string;
  value: string;
  expires_at: string;
};

export const emptyCredentialForm: CredentialFormState = {
  namespace: 'system',
  team_path: '',
  name: '',
  kind: 'api_key',
  description: '',
  value: '',
  expires_at: '',
};

export type CredentialReferenceParts = {
  namespace: string;
  name: string;
  category: string;
  displayName: string;
  parentPath: string;
};

export type CredentialSummary = {
  total: number;
  active: number;
  disabled: number;
  pending: number;
};

export type CredentialTeam = {
  key: string;
  namespace: string;
  category: string;
  credentials: CredentialRecord[];
};

export type CredentialReferenceDisplayParts = CredentialReferenceParts & {
  scopeKind: 'team' | 'system';
  scopePath: string;
  scopeLabel: string;
};

export type CredentialCatalogGroup = {
  key: string;
  namespace: string;
  scopeKind: 'team' | 'system';
  scopePath: string;
  scopeLabel: string;
  categories: CredentialCatalogCategory[];
  credentials: CredentialRecord[];
};

export type CredentialCatalogCategory = {
  key: string;
  category: string;
  credentials: CredentialRecord[];
};

export function normalizeCredentialsPayload(value: unknown): CredentialRecord[] {
  const record = asRecord(value);
  const items = record && Array.isArray(record.credentials) ? record.credentials : [];
  return items
    .map(normalizeCredential)
    .filter((credential): credential is CredentialRecord => credential !== null)
    .sort((left, right) => left.reference.localeCompare(right.reference));
}

export function normalizeCredential(value: unknown): CredentialRecord | null {
  const record = asRecord(value);
  if (!record) return null;
  const id = readString(record.id).trim();
  const reference = readString(record.reference).trim();
  if (!id || !reference) return null;

  const versions = Array.isArray(record.versions)
    ? record.versions
        .map(normalizeCredentialVersion)
        .filter((version): version is CredentialVersionRecord => version !== null)
        .sort((left, right) => right.version - left.version)
    : [];

  return {
    id,
    reference,
    kind: readString(record.kind).trim(),
    description: readString(record.description).trim(),
    status: readString(record.status).trim() || 'pending',
    has_value: Boolean(record.has_value),
    active_version: readNumber(record.active_version),
    expires_at: readOptionalString(record.expires_at),
    last_rotated_at: readOptionalString(record.last_rotated_at),
    managed_by_config_repo: Boolean(record.managed_by_config_repo),
    config_source_path: readOptionalString(record.config_source_path),
    config_source_commit_sha: readOptionalString(record.config_source_commit_sha),
    created_by: readOptionalString(record.created_by),
    updated_by: readOptionalString(record.updated_by),
    created_at: readString(record.created_at),
    updated_at: readString(record.updated_at),
    versions,
  };
}

export function credentialPayloadFromForm(form: CredentialFormState) {
  const teamPath = normalizeCredentialTeamPath(form.team_path);
  return {
    reference: buildCredentialReference(form.namespace, form.name, teamPath),
    team_path: teamPath || undefined,
    kind: form.kind,
    description: form.description.trim(),
    value: form.value,
    expires_at: form.expires_at ? new Date(form.expires_at).toISOString() : undefined,
  };
}

export function buildCredentialReference(namespace: string, name: string, teamPath = ''): string {
  const normalizedTeamPath = normalizeCredentialTeamPath(teamPath);
  const normalizedNamespace = normalizedTeamPath ? 'team' : namespace.trim().toLowerCase() || 'system';
  const normalizedName = name.trim().toLowerCase().replace(/^\/+|\/+$/g, '');
  const teamRelativeName = stripCredentialTeamPathPrefix(normalizedName, normalizedTeamPath);
  const scopedName = normalizedTeamPath ? `${normalizedTeamPath}/${teamRelativeName}` : normalizedName;
  return `credential://${normalizedNamespace}/${scopedName}`;
}

export function normalizeCredentialTeamPath(teamPath: string): string {
  return teamPath.trim().toLowerCase().replace(/^\/+|\/+$/g, '');
}

export function isCredentialReference(reference: string): boolean {
  return /^credential:\/\/[^/]+\/.+/i.test(reference.trim());
}

export function isTeamCredentialReference(reference: string): boolean {
  return parseCredentialReference(reference).namespace.toLowerCase() === 'team';
}

export function credentialReferenceRoute(reference: string): string {
  const trimmed = reference.trim();
  return `/credentials?credential=${encodeURIComponent(trimmed)}`;
}

export function parseCredentialReference(reference: string): CredentialReferenceParts {
  const body = reference.trim().replace(/^credential:\/\//i, '');
  const slashIndex = body.indexOf('/');
  const namespace = slashIndex >= 0 ? body.slice(0, slashIndex) : 'system';
  const name = slashIndex >= 0 ? body.slice(slashIndex + 1) : body;
  const segments = name.split('/').filter(Boolean);
  const category = segments[0] || 'general';
  const displayName = segments.at(-1) || name || 'Unnamed credential';
  const parentPath = segments.slice(1, -1).join(' / ');
  return { namespace, name, category, displayName, parentPath };
}

export function credentialReferenceDisplay(
  reference: string,
  teamPaths: string[] = []
): CredentialReferenceDisplayParts {
  const base = parseCredentialReference(reference);
  const segments = base.name.split('/').filter(Boolean);
  if (base.namespace.toLowerCase() !== 'team') {
    return {
      ...base,
      scopeKind: 'system',
      scopePath: base.namespace,
      scopeLabel: base.namespace,
    };
  }

  const knownTeamPath = findMatchingTeamPath(segments, teamPaths);
  const inferredTeamSegments = knownTeamPath
    ? knownTeamPath.split('/').filter(Boolean)
    : segments.length > 1
      ? segments.slice(0, 1)
      : [];
  const credentialSegments = stripRepeatedTeamSegments(segments.slice(inferredTeamSegments.length), inferredTeamSegments);
  const category = credentialSegments.length > 1 ? credentialSegments[0] : 'general';
  const displayName = credentialSegments.at(-1) || base.displayName;
  const parentPath = credentialSegments.slice(1, -1).join(' / ');
  const scopePath = inferredTeamSegments.join('/');

  return {
    namespace: base.namespace,
    name: base.name,
    category,
    displayName,
    parentPath,
    scopeKind: 'team',
    scopePath,
    scopeLabel: scopePath || 'team',
  };
}

export function credentialNamespaces(credentials: CredentialRecord[]): string[] {
  return [...new Set(credentials.map(credential => parseCredentialReference(credential.reference).namespace))]
    .sort((left, right) => left.localeCompare(right));
}

export function credentialSummary(credentials: CredentialRecord[]): CredentialSummary {
  return credentials.reduce<CredentialSummary>((summary, credential) => {
    summary.total += 1;
    if (credential.status === 'active') summary.active += 1;
    if (credential.status === 'disabled') summary.disabled += 1;
    if (credential.status === 'pending') summary.pending += 1;
    return summary;
  }, { total: 0, active: 0, disabled: 0, pending: 0 });
}

export function filterCredentials(
  credentials: CredentialRecord[],
  query: string,
  status: string,
  namespace: string
): CredentialRecord[] {
  const normalizedQuery = query.trim().toLowerCase();
  return credentials.filter(credential => {
    const reference = parseCredentialReference(credential.reference);
    if (status !== 'all' && credential.status !== status) return false;
    if (namespace === 'team' && reference.namespace !== 'team') return false;
    if (namespace === 'system' && reference.namespace !== 'system') return false;
    if (!['all', 'team', 'system'].includes(namespace) && reference.namespace !== namespace) return false;
    if (!normalizedQuery) return true;
    return [
      credential.reference,
      reference.name,
      reference.displayName,
      credential.kind,
      credential.description,
      credential.status,
    ].some(value => value.toLowerCase().includes(normalizedQuery));
  });
}

export function credentialCatalogGroups(
  credentials: CredentialRecord[],
  teamPaths: string[] = []
): CredentialCatalogGroup[] {
  const groups = new Map<string, CredentialCatalogGroup>();
  credentials.forEach(credential => {
    const reference = credentialReferenceDisplay(credential.reference, teamPaths);
    const key = `${reference.scopeKind}/${reference.scopePath || reference.namespace}`;
    const group = groups.get(key) || {
      key,
      namespace: reference.namespace,
      scopeKind: reference.scopeKind,
      scopePath: reference.scopePath,
      scopeLabel: reference.scopeLabel,
      categories: [],
      credentials: [],
    };
    group.credentials.push(credential);
    groups.set(key, group);
  });

  return [...groups.values()]
    .map(group => enrichCredentialCatalogGroup(group, teamPaths))
    .sort(compareCredentialCatalogGroups);
}

export function recentlyUpdatedCredentials(credentials: CredentialRecord[], limit = 5): CredentialRecord[] {
  return [...credentials]
    .sort((left, right) => credentialTimestamp(right) - credentialTimestamp(left))
    .slice(0, Math.max(0, limit));
}

export function teamCredentials(credentials: CredentialRecord[]): CredentialTeam[] {
  const teams = new Map<string, CredentialTeam>();
  credentials.forEach(credential => {
    const reference = parseCredentialReference(credential.reference);
    const key = `${reference.namespace}/${reference.category}`;
    const team = teams.get(key) || {
      key,
      namespace: reference.namespace,
      category: reference.category,
      credentials: [],
    };
    team.credentials.push(credential);
    teams.set(key, team);
  });
  return [...teams.values()]
    .map(team => ({
      ...team,
      credentials: [...team.credentials].sort((left, right) =>
        parseCredentialReference(left.reference).name.localeCompare(parseCredentialReference(right.reference).name)
      ),
    }))
    .sort((left, right) => left.key.localeCompare(right.key));
}

function normalizeCredentialVersion(value: unknown): CredentialVersionRecord | null {
  const record = asRecord(value);
  if (!record) return null;
  const version = readNumber(record.version);
  if (version <= 0) return null;
  return {
    version,
    created_by: readOptionalString(record.created_by),
    created_at: readString(record.created_at),
    activated_at: readOptionalString(record.activated_at),
    revoked_at: readOptionalString(record.revoked_at),
  };
}

function readNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function findMatchingTeamPath(segments: string[], teamPaths: string[]): string {
  const normalizedTeamPaths = teamPaths
    .map(normalizeCredentialTeamPath)
    .filter(Boolean)
    .sort((left, right) => right.split('/').length - left.split('/').length || left.localeCompare(right));

  return normalizedTeamPaths.find(teamPath => {
    const teamSegments = teamPath.split('/').filter(Boolean);
    if (teamSegments.length > segments.length) return false;
    return teamSegments.every((segment, index) => segment === segments[index]);
  }) || '';
}

function compareCredentialCatalogGroups(left: CredentialCatalogGroup, right: CredentialCatalogGroup): number {
  if (left.scopeKind !== right.scopeKind) return left.scopeKind === 'team' ? -1 : 1;
  const scopeComparison = systemScopeRank(left) - systemScopeRank(right)
    || left.scopeLabel.localeCompare(right.scopeLabel);
  if (scopeComparison !== 0) return scopeComparison;
  return left.key.localeCompare(right.key);
}

function systemScopeRank(group: CredentialCatalogGroup): number {
  if (group.scopeKind === 'team') return 0;
  if (group.namespace === 'system') return 1;
  if (group.namespace === 'global') return 2;
  return 3;
}

function enrichCredentialCatalogGroup(group: CredentialCatalogGroup, teamPaths: string[]): CredentialCatalogGroup {
  const categories = new Map<string, CredentialCatalogCategory>();
  const sortedCredentials = [...group.credentials].sort((left, right) =>
    credentialReferenceDisplay(left.reference, teamPaths).displayName.localeCompare(
      credentialReferenceDisplay(right.reference, teamPaths).displayName
    )
  );

  sortedCredentials.forEach(credential => {
    const reference = credentialReferenceDisplay(credential.reference, teamPaths);
    const key = `${group.key}/${reference.category}`;
    const category = categories.get(key) || {
      key,
      category: reference.category,
      credentials: [],
    };
    category.credentials.push(credential);
    categories.set(key, category);
  });

  return {
    ...group,
    credentials: sortedCredentials,
    categories: [...categories.values()].sort((left, right) => compareCredentialCategories(left, right)),
  };
}

function compareCredentialCategories(left: CredentialCatalogCategory, right: CredentialCatalogCategory): number {
  return credentialCategoryRank(left.category) - credentialCategoryRank(right.category)
    || left.category.localeCompare(right.category);
}

function credentialCategoryRank(category: string): number {
  if (category === 'general') return 99;
  return 0;
}

function stripCredentialTeamPathPrefix(name: string, teamPath: string): string {
  if (!teamPath) return name;
  if (name === teamPath) return 'credential';
  const prefix = `${teamPath}/`;
  return name.startsWith(prefix) ? name.slice(prefix.length) : name;
}

function stripRepeatedTeamSegments(credentialSegments: string[], teamSegments: string[]): string[] {
  if (teamSegments.length === 0) return credentialSegments;
  const hasRepeatedTeam = teamSegments.every((segment, index) => credentialSegments[index] === segment);
  return hasRepeatedTeam ? credentialSegments.slice(teamSegments.length) : credentialSegments;
}

function credentialTimestamp(credential: CredentialRecord): number {
  const updatedAt = Date.parse(credential.updated_at);
  if (Number.isFinite(updatedAt)) return updatedAt;
  const createdAt = Date.parse(credential.created_at);
  return Number.isFinite(createdAt) ? createdAt : 0;
}
