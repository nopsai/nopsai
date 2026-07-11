import {
  BASIC_ROLE_ADMIN,
  ROOT_ACCESS_SCOPE,
  accessGrantEditKey,
  accessGrantTargetKey,
} from './model';
import type { AccessGrantRecord, EditableAccessGrant } from './model';

export type BasicGrantDraft = {
  role: string;
  scope: string;
};

export type BasicGrantChangeSet = {
  grantsToDelete: AccessGrantRecord[];
  grantsToAdd: EditableAccessGrant[];
};

export function createEditableBasicGrant(draft: BasicGrantDraft, localID: string): EditableAccessGrant | null {
  const role = draft.role.trim().toLowerCase();
  if (!role) return null;
  return {
    localID,
    role,
    resourceType: role === BASIC_ROLE_ADMIN ? 'platform' : 'team',
    resourceID: role === BASIC_ROLE_ADMIN ? 'platform' : draft.scope || ROOT_ACCESS_SCOPE,
    inherit: role !== BASIC_ROLE_ADMIN,
  };
}

export function isBasicGrantDraftDuplicate(draft: BasicGrantDraft, entries: EditableAccessGrant[]) {
  const grant = createEditableBasicGrant(draft, 'duplicate-check');
  if (!grant) return false;
  const draftKey = accessGrantEditKey(grant);
  return entries.some(entry => accessGrantEditKey(entry) === draftKey);
}

export function stageBasicGrant(
  entries: EditableAccessGrant[],
  draft: BasicGrantDraft,
  localID: string
): { entries: EditableAccessGrant[]; error: string | null } {
  const nextGrant = createEditableBasicGrant(draft, localID);
  if (!nextGrant) return { entries, error: 'Choose an access level.' };
  if (isBasicGrantDraftDuplicate(draft, entries)) {
    return { entries, error: 'This basic role is already listed.' };
  }

  const targetKey = accessGrantTargetKey(nextGrant);
  let replaced = false;
  const nextEntries: EditableAccessGrant[] = [];
  entries.forEach(entry => {
    if (accessGrantTargetKey(entry) !== targetKey) {
      nextEntries.push(entry);
      return;
    }
    if (replaced) return;
    nextEntries.push({
      ...nextGrant,
      localID: entry.localID,
      id: entry.id,
      grantedBy: entry.grantedBy,
    });
    replaced = true;
  });
  if (!replaced) nextEntries.push(nextGrant);
  return { entries: nextEntries, error: null };
}

export function areBasicGrantEntriesDirty(original: AccessGrantRecord[], entries: EditableAccessGrant[]) {
  const originalKeys = new Set(original.map(grant => accessGrantEditKey(grant)));
  const draftKeys = new Set(entries.map(grant => accessGrantEditKey(grant)));
  if (originalKeys.size !== draftKeys.size) return true;
  return Array.from(originalKeys).some(key => !draftKeys.has(key));
}

export function buildBasicGrantChangeSet(
  original: AccessGrantRecord[],
  entries: EditableAccessGrant[]
): BasicGrantChangeSet {
  const normalizedEntries = Array.from(
    entries.reduce(
      (grants, grant) => grants.set(accessGrantTargetKey(grant), grant),
      new Map<string, EditableAccessGrant>()
    ).values()
  );
  const draftKeys = new Set(normalizedEntries.map(grant => accessGrantEditKey(grant)));
  const originalByKey = new Map(original.map(grant => [accessGrantEditKey(grant), grant]));
  return {
    grantsToDelete: original.filter(grant => !draftKeys.has(accessGrantEditKey(grant))),
    grantsToAdd: normalizedEntries.filter(grant => !originalByKey.has(accessGrantEditKey(grant))),
  };
}
