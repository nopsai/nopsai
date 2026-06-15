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
  name: string;
  kind: CredentialKind;
  description: string;
  value: string;
  expires_at: string;
};

export const emptyCredentialForm: CredentialFormState = {
  namespace: 'system',
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

export type CredentialGroup = {
  key: string;
  namespace: string;
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
  return {
    reference: buildCredentialReference(form.namespace, form.name),
    kind: form.kind,
    description: form.description.trim(),
    value: form.value,
    expires_at: form.expires_at ? new Date(form.expires_at).toISOString() : undefined,
  };
}

export function buildCredentialReference(namespace: string, name: string): string {
  const normalizedNamespace = namespace.trim().toLowerCase() || 'system';
  const normalizedName = name.trim().toLowerCase().replace(/^\/+|\/+$/g, '');
  return `credential://${normalizedNamespace}/${normalizedName}`;
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
    if (namespace !== 'all' && reference.namespace !== namespace) return false;
    if (!normalizedQuery) return true;
    return [
      reference.name,
      reference.displayName,
      credential.kind,
      credential.description,
      credential.status,
    ].some(value => value.toLowerCase().includes(normalizedQuery));
  });
}

export function groupCredentials(credentials: CredentialRecord[]): CredentialGroup[] {
  const groups = new Map<string, CredentialGroup>();
  credentials.forEach(credential => {
    const reference = parseCredentialReference(credential.reference);
    const key = `${reference.namespace}/${reference.category}`;
    const group = groups.get(key) || {
      key,
      namespace: reference.namespace,
      category: reference.category,
      credentials: [],
    };
    group.credentials.push(credential);
    groups.set(key, group);
  });
  return [...groups.values()]
    .map(group => ({
      ...group,
      credentials: [...group.credentials].sort((left, right) =>
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
