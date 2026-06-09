import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import { Copy, Edit3, Plus, RefreshCw, Search, Server, Trash2, X } from 'lucide-react';
import {
  ACCESS_UI_BUILD_ID,
  DEFAULT_ADMIN_ROLE,
  POLICY_TEMPLATE_ROLE,
  ROOT_ACCESS_SCOPE,
  accessGrantMatchesServiceAccount,
  accessGrantMatchesUser,
  accessGrantSortLabel,
  assignmentKey,
  basicAccessGrantLabel,
  editableAccessGrantFromRecord,
  isBasicAccessGrant,
  isDefaultAdminUser,
  isProtectedAccessRole,
  normalizeEditableBasicGrants,
  policyKey,
  policyLabel,
} from './access/model';
import { BasicAccessGrantEditor } from './access/BasicAccessGrantEditor';
import { AccessEditorEmptyState, AccessModal } from './access/AccessModal';
import { AccessPolicyRuleFields } from './access/AccessPolicyRuleFields';
import {
  areBasicGrantEntriesDirty,
  buildBasicGrantChangeSet,
  stageBasicGrant,
} from './access/basicGrantModel';
import {
  formatAccessActionSummary,
  formatAccessResourceSummary,
  parseAAAActionValue,
  summarizeRoleCoverage,
} from './access/policyRuleModel';
import type { AccessResourceCatalog } from './access/resourceCatalog';
import type {
  AccessGrantRecord,
  BasicGrantInput,
  EditableAccessGrant,
  RoleDefinition,
  RolePermission,
  RolePolicyDraft,
  ServiceAccountSummary,
  ServiceAccountToken,
  UserSummary,
} from './access/model';
import {
  ACCESS_SECTION_CONTENT,
  accessPresetForRole,
  accessPresetToneClass,
  formatAccessCount,
  formatAccessTimestamp,
  matchesAccessSearch,
} from './access/presentation';

export type AccessPanelProps = {
  users: UserSummary[];
  loading: boolean;
  error: string | null;
  serviceAccounts: ServiceAccountSummary[];
  serviceAccountsLoading: boolean;
  serviceAccountsError: string | null;
  accessGrants: AccessGrantRecord[];
  accessGrantsLoading: boolean;
  accessGrantsError: string | null;
  policies: RolePermission[];
  policiesLoading: boolean;
  policiesError: string | null;
  resourceCatalog: AccessResourceCatalog;
  newUser: { sub: string; email: string; password: string; roles: string[] };
  newServiceAccount: { sub: string; email: string; tokenName: string; roles: string[] };
  policyTemplates: RolePermission[];
  onChangeUser: (next: { sub: string; email: string; password: string; roles: string[] }) => void;
  onCreateUser: (e: FormEvent<HTMLFormElement>, options?: { basicGrants?: BasicGrantInput[] }) => Promise<boolean>;
  onChangeServiceAccount: (next: { sub: string; email: string; tokenName: string; roles: string[] }) => void;
  onCreateServiceAccount: (e: FormEvent<HTMLFormElement>, options?: { basicGrants?: BasicGrantInput[] }) => Promise<ServiceAccountToken | null>;
  onReloadUsers: () => void;
  onReloadServiceAccounts: () => void;
  onCreatePermission: (e: FormEvent<HTMLFormElement>) => Promise<void>;
  newPermission: { name: string; obj: string; act: string };
  onChangePermission: (next: { name: string; obj: string; act: string }) => void;
  onDeleteUser: (userId: string) => Promise<void>;
  onDeleteServiceAccount: (serviceAccountId: string) => Promise<void>;
  onDeletePolicy: (policy: RolePermission) => Promise<void>;
  onDeleteRoleDefinition: (role: RoleDefinition) => Promise<void>;
  onSaveRoleDefinition: (input: { role: string; policies: RolePolicyDraft[]; original?: RolePermission[] }) => Promise<void>;
  onEditPolicy: (current: RolePermission, next: { role: string; name: string; obj: string; act: string }) => Promise<void>;
  onUpdateUserRoles: (userId: string, nextRoles: string[], previousRoles: string[]) => Promise<void>;
  onUpdateServiceAccountRoles: (serviceAccountId: string, nextRoles: string[], previousRoles: string[]) => Promise<void>;
  onCreateAccessGrant: (input: { userID: string; role: string; resourceType: string; resourceID: string; inherit?: boolean }) => Promise<void>;
  onCreateServiceAccountAccessGrant: (input: { serviceAccountSub: string; role: string; resourceType: string; resourceID: string; inherit?: boolean }) => Promise<void>;
  onDeleteAccessGrant: (grantID: string) => Promise<void>;
  onReloadAccessGrants: () => void;
  onReloadPolicies: () => void;
  onUpdateUser: (userId: string, input: { email?: string; status?: string; password?: string }) => Promise<void>;
  onUpdateServiceAccount: (serviceAccountId: string, input: { email?: string; status?: string }) => Promise<void>;
  onLoadServiceAccountTokens: (serviceAccountId: string) => Promise<ServiceAccountToken[]>;
  onCreateServiceAccountToken: (serviceAccountId: string, name: string) => Promise<ServiceAccountToken>;
  onRevokeServiceAccountToken: (serviceAccountId: string, tokenId: string) => Promise<void>;
};

function AccessPanel({
  users,
  loading,
  error,
  serviceAccounts,
  serviceAccountsLoading,
  serviceAccountsError,
  accessGrants,
  accessGrantsLoading,
  accessGrantsError,
  policies,
  policiesLoading,
  policiesError,
  resourceCatalog,
  newUser,
  newServiceAccount,
  policyTemplates,
  onChangeUser,
  onCreateUser,
  onChangeServiceAccount,
  onCreateServiceAccount,
  onReloadUsers,
  onReloadServiceAccounts,
  onCreatePermission,
  newPermission,
  onChangePermission,
  onDeleteUser,
  onDeleteServiceAccount,
  onDeletePolicy,
  onDeleteRoleDefinition,
  onSaveRoleDefinition,
  onEditPolicy,
  onUpdateUserRoles,
  onUpdateServiceAccountRoles,
  onCreateAccessGrant,
  onCreateServiceAccountAccessGrant,
  onDeleteAccessGrant,
  onReloadAccessGrants,
  onReloadPolicies,
  onUpdateUser,
  onUpdateServiceAccount,
  onLoadServiceAccountTokens,
  onCreateServiceAccountToken,
  onRevokeServiceAccountToken,
}: AccessPanelProps) {
  const [accessMode, setAccessMode] = useState<'basic' | 'advanced'>('basic');
  const [activeSection, setActiveSection] = useState<'users' | 'service-accounts' | 'roles' | 'policies'>('users');
  const [showUserModal, setShowUserModal] = useState(false);
  const [showServiceAccountModal, setShowServiceAccountModal] = useState(false);
  const [showPolicyModal, setShowPolicyModal] = useState(false);
  const [roleEditor, setRoleEditor] = useState<{
    mode: 'create' | 'edit';
    role: string;
    policies: RolePolicyDraft[];
    original?: RolePermission[];
  } | null>(null);
  const [policyEditor, setPolicyEditor] = useState<{
    original: RolePermission;
    role: string;
    name: string;
    obj: string;
    act: string;
  } | null>(null);
  const [confirmDialog, setConfirmDialog] = useState<{ message: string; onConfirm: () => Promise<void> | void } | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [userAccessEditor, setUserAccessEditor] = useState<{
    user: UserSummary;
    entries: string[];
    original: string[];
    email: string;
    status: string;
    password: string;
  } | null>(null);
  const [serviceAccountEditor, setServiceAccountEditor] = useState<{
    account: ServiceAccountSummary;
    entries: string[];
    original: string[];
    email: string;
    status: string;
    tokenName: string;
    tokens: ServiceAccountToken[];
    tokensLoading: boolean;
    tokensError: string | null;
  } | null>(null);
  const [createdServiceAccountToken, setCreatedServiceAccountToken] = useState<ServiceAccountToken | null>(null);
  const [copyServiceAccountTokenLabel, setCopyServiceAccountTokenLabel] = useState('Copy');
  const [savingRoleEditor, setSavingRoleEditor] = useState(false);
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [savingUserAccess, setSavingUserAccess] = useState(false);
  const [savingServiceAccountAccess, setSavingServiceAccountAccess] = useState(false);
  const [creatingUserInline, setCreatingUserInline] = useState(false);
  const [creatingServiceAccountInline, setCreatingServiceAccountInline] = useState(false);
  const [creatingPolicyInline, setCreatingPolicyInline] = useState(false);
  const [awaitingUserCreateReset, setAwaitingUserCreateReset] = useState(false);
  const [awaitingServiceAccountCreateReset, setAwaitingServiceAccountCreateReset] = useState(false);
  const [awaitingPolicyCreateReset, setAwaitingPolicyCreateReset] = useState(false);
  const [basicGrantDraft, setBasicGrantDraft] = useState({ role: '', scope: ROOT_ACCESS_SCOPE });
  const [basicGrantEntries, setBasicGrantEntries] = useState<EditableAccessGrant[]>([]);
  const [basicGrantSaving, setBasicGrantSaving] = useState(false);
  const [basicGrantError, setBasicGrantError] = useState<string | null>(null);
  const policyOptions = useMemo(() => {
    const seen = new Set<string>();
    return policyTemplates
      .filter(policy => {
        const key = `${policy.obj}::${policy.act}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      })
      .map(policy => ({ key: `${policy.obj}::${policy.act}`, obj: policy.obj, act: policy.act, name: policy.name, label: policyLabel(policy) }))
      .sort((a, b) => a.label.localeCompare(b.label));
  }, [policyTemplates]);

  const roleAssignments = useMemo(
    () =>
      [
        ...users.flatMap(user =>
          (user.roles || []).map(role => ({
            id: `${user.id}-${role.role}`,
            role: role.role,
            user: user.sub,
            userId: user.id,
            email: user.email,
            status: user.status,
            kind: 'user',
          }))
        ),
        ...serviceAccounts.flatMap(account =>
          (account.roles || []).map(role => ({
            id: `${account.id}-${role.role}`,
            role: role.role,
            user: account.sub,
            userId: account.id,
            email: account.email,
            status: account.status,
            kind: 'service-account',
          }))
        ),
      ].sort((a, b) => a.role.localeCompare(b.role) || a.user.localeCompare(b.user)),
    [serviceAccounts, users]
  );

  const roleDefinitions = useMemo(() => {
    const map = new Map<string, RoleDefinition>();
    policies.forEach(policy => {
      if (!map.has(policy.role)) {
        map.set(policy.role, { id: policy.role, role: policy.role, policies: [] });
      }
      map.get(policy.role)?.policies.push(policy);
    });
    return Array.from(map.values()).sort((a, b) => a.role.localeCompare(b.role));
  }, [policies]);

  const allRoleOptions = useMemo(() => {
    const set = new Set<string>();
    roleDefinitions.forEach(role => set.add(role.role));
    set.add(DEFAULT_ADMIN_ROLE);
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [roleDefinitions]);

  const roleUserMap = useMemo(() => {
    const map = new Map<string, { user: string; userId: string; email: string; kind: string }[]>();
    roleAssignments.forEach(item => {
      const existing = map.get(item.role) || [];
      existing.push({ user: item.user, userId: item.userId, email: item.email, kind: item.kind });
      map.set(item.role, existing);
    });
    return map;
  }, [roleAssignments]);

  const tabItems = [
    { id: 'users', label: 'Users', count: users.length },
    { id: 'service-accounts', label: 'Service accounts', count: serviceAccounts.length },
    { id: 'roles', label: 'Roles', count: roleDefinitions.length },
    { id: 'policies', label: 'Policies', count: policyTemplates.length },
  ] as const;

  const openCreateRoleEditor = useCallback(() => {
    setNextPolicyKey('');
    setRoleEditor({
      mode: 'create',
      role: '',
      policies: [],
    });
  }, []);

  const openCreateUserEditor = useCallback(() => {
    setNextUserRole('');
    setAwaitingUserCreateReset(false);
    setUserAccessEditor(null);
    setServiceAccountEditor(null);
    setBasicGrantError(null);
    setBasicGrantDraft({ role: '', scope: ROOT_ACCESS_SCOPE });
    setBasicGrantEntries([]);
    setShowServiceAccountModal(false);
    setShowUserModal(true);
  }, []);

  const openCreateServiceAccountEditor = useCallback(() => {
    setNextUserRole('');
    setAwaitingServiceAccountCreateReset(false);
    setUserAccessEditor(null);
    setServiceAccountEditor(null);
    setCreatedServiceAccountToken(null);
    setBasicGrantError(null);
    setBasicGrantDraft({ role: '', scope: ROOT_ACCESS_SCOPE });
    setBasicGrantEntries([]);
    setShowUserModal(false);
    setShowServiceAccountModal(true);
  }, []);

  const openCreatePolicyEditor = useCallback(() => {
    setAwaitingPolicyCreateReset(false);
    setPolicyEditor(null);
    setShowPolicyModal(true);
  }, []);

  const openEditRoleEditor = (role: RoleDefinition) => {
    if (isProtectedAccessRole(role.role)) return;
    setShowPolicyModal(false);
    setRoleEditor({
      mode: 'edit',
      role: role.role,
      policies: role.policies.map(p => ({ name: p.name || policyLabel(p), obj: p.obj, act: p.act })),
      original: role.policies,
    });
  };

  const openPolicyEditModal = (policy: RolePermission) => {
    if (isProtectedAccessRole(policy.role)) return;
    setAwaitingPolicyCreateReset(false);
    setShowPolicyModal(false);
    setPolicyEditor({
      original: policy,
      role: policy.role,
      name: policy.name || policyLabel(policy),
      obj: policy.obj,
      act: policy.act,
    });
  };

  const openUserAccessModal = (user: UserSummary) => {
    setAwaitingUserCreateReset(false);
    setShowUserModal(false);
    setShowServiceAccountModal(false);
    setServiceAccountEditor(null);
    setBasicGrantError(null);
    setBasicGrantDraft({ role: '', scope: ROOT_ACCESS_SCOPE });
    setBasicGrantEntries((basicUserGrantMap.get(user.id) || []).map(editableAccessGrantFromRecord));
    const entries = (user.roles || []).map(role => role.role);
    const nextEntries = entries.length > 0 ? entries : [];
    setUserAccessEditor({
      user,
      entries: nextEntries,
      original: entries,
      email: user.email || '',
      status: user.status || 'active',
      password: '',
    });
  };

  const openServiceAccountAccessModal = (account: ServiceAccountSummary) => {
    setAwaitingServiceAccountCreateReset(false);
    setShowServiceAccountModal(false);
    setShowUserModal(false);
    setUserAccessEditor(null);
    setCreatedServiceAccountToken(null);
    setBasicGrantError(null);
    setBasicGrantDraft({ role: '', scope: ROOT_ACCESS_SCOPE });
    setBasicGrantEntries((basicServiceAccountGrantMap.get(account.sub) || []).map(editableAccessGrantFromRecord));
    const entries = (account.roles || []).map(role => role.role);
    setServiceAccountEditor({
      account,
      entries,
      original: entries,
      email: account.email || '',
      status: account.status || 'active',
      tokenName: 'rotation',
      tokens: [],
      tokensLoading: true,
      tokensError: null,
    });
    void onLoadServiceAccountTokens(account.id)
      .then(tokens => {
        setServiceAccountEditor(prev => (prev?.account.id === account.id ? { ...prev, tokens, tokensLoading: false, tokensError: null } : prev));
      })
      .catch(error => {
        setServiceAccountEditor(prev => (
          prev?.account.id === account.id
            ? { ...prev, tokensLoading: false, tokensError: error instanceof Error ? error.message : 'Unable to load tokens' }
            : prev
        ));
      });
  };

  const removeRolePolicyDraft = (index: number) => {
    setRoleEditor(prev => {
      if (!prev) return prev;
      const nextPolicies = prev.policies.filter((_, i) => i !== index);
      return {
        ...prev,
        policies: nextPolicies,
      };
    });
  };

  const addExistingPolicyDraft = (key: string) => {
    setRoleEditor(prev => {
      if (!prev) return prev;
      const match = policyOptions.find(p => p.key === key);
      if (!match) return prev;
      const already = prev.policies.some(p => p.obj === match.obj && p.act === match.act);
      if (already) return prev;
      return { ...prev, policies: [...prev.policies, { name: match.name || match.label, obj: match.obj, act: match.act }] };
    });
  };

  const handleSaveRoleEditor = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!roleEditor) return;
    setSavingRoleEditor(true);
    try {
      await onSaveRoleDefinition({
        role: roleEditor.role,
        policies: roleEditor.policies,
        original: roleEditor.original,
      });
      setRoleEditor(null);
    } catch (error) {
      console.error('Failed to save role', error);
    } finally {
      setSavingRoleEditor(false);
    }
  };

  const handleSavePolicyEdit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!policyEditor) return;
    setSavingPolicy(true);
    try {
      await onEditPolicy(policyEditor.original, {
        role: policyEditor.role,
        name: policyEditor.name,
        obj: policyEditor.obj,
        act: policyEditor.act,
      });
      setPolicyEditor(null);
    } catch (error) {
      console.error('Failed to update policy', error);
    } finally {
      setSavingPolicy(false);
    }
  };

  const handleSaveUserAccess = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!userAccessEditor) return;
    const deduped = userAccessEditor.entries
      .map(role => role.trim())
      .filter(Boolean)
      .filter((role, index, roles) => roles.indexOf(role) === index);
    setSavingUserAccess(true);
    try {
      const updatePayload: { email?: string; status?: string; password?: string } = {};
      const emailTrimmed = userAccessEditor.email.trim();
      if (emailTrimmed && emailTrimmed !== userAccessEditor.user.email) {
        updatePayload.email = emailTrimmed;
      }
      const statusTrimmed = userAccessEditor.status.trim();
      if (statusTrimmed && statusTrimmed !== userAccessEditor.user.status) {
        updatePayload.status = statusTrimmed;
      }
      const passwordTrimmed = userAccessEditor.password.trim();
      if (passwordTrimmed) {
        updatePayload.password = passwordTrimmed;
      }
      if (Object.keys(updatePayload).length > 0) {
        await onUpdateUser(userAccessEditor.user.id, updatePayload);
      }
      if (!isDefaultAdminUser(userAccessEditor.user)) {
        await onUpdateUserRoles(userAccessEditor.user.id, deduped, userAccessEditor.original);
      }
      if (!isDefaultAdminUser(userAccessEditor.user) && basicGrantDirty) {
        await saveBasicGrantsForUser(userAccessEditor.user);
      }
      setUserAccessEditor(null);
    } catch (error) {
      console.error('Failed to update user access', error);
    } finally {
      setSavingUserAccess(false);
    }
  };

  const handleSaveServiceAccountAccess = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!serviceAccountEditor) return;
    const deduped = serviceAccountEditor.entries
      .map(role => role.trim())
      .filter(Boolean)
      .filter((role, index, roles) => roles.indexOf(role) === index);
    setSavingServiceAccountAccess(true);
    try {
      const updatePayload: { email?: string; status?: string } = {};
      const emailTrimmed = serviceAccountEditor.email.trim();
      if (emailTrimmed && emailTrimmed !== serviceAccountEditor.account.email) {
        updatePayload.email = emailTrimmed;
      }
      const statusTrimmed = serviceAccountEditor.status.trim();
      if (statusTrimmed && statusTrimmed !== serviceAccountEditor.account.status) {
        updatePayload.status = statusTrimmed;
      }
      if (Object.keys(updatePayload).length > 0) {
        await onUpdateServiceAccount(serviceAccountEditor.account.id, updatePayload);
      }
      await onUpdateServiceAccountRoles(serviceAccountEditor.account.id, deduped, serviceAccountEditor.original);
      if (basicGrantDirty) {
        await saveBasicGrantsForServiceAccount(serviceAccountEditor.account);
      }
      setServiceAccountEditor(null);
    } catch (error) {
      console.error('Failed to update service account access', error);
    } finally {
      setSavingServiceAccountAccess(false);
    }
  };

  const addUserAccessEntry = () => {
    setUserAccessEditor(prev => {
      if (!prev) return prev;
      if (isDefaultAdminUser(prev.user)) return prev;
      const roleName = nextAccessRole.trim();
      if (!roleName) return prev;
      if (prev.entries.some(entry => assignmentKey(entry) === assignmentKey(roleName))) return prev;
      return { ...prev, entries: [...prev.entries, roleName] };
    });
    setNextAccessRole('');
  };

  const removeUserAccessEntry = (index: number) => {
    setUserAccessEditor(prev => {
      if (!prev) return prev;
      if (isDefaultAdminUser(prev.user)) return prev;
      const next = prev.entries.filter((_, i) => i !== index);
      return { ...prev, entries: next };
    });
  };

  const addServiceAccountAccessEntry = () => {
    setServiceAccountEditor(prev => {
      if (!prev) return prev;
      const roleName = nextAccessRole.trim();
      if (!roleName) return prev;
      if (prev.entries.some(entry => assignmentKey(entry) === assignmentKey(roleName))) return prev;
      return { ...prev, entries: [...prev.entries, roleName] };
    });
    setNextAccessRole('');
  };

  const removeServiceAccountAccessEntry = (index: number) => {
    setServiceAccountEditor(prev => {
      if (!prev) return prev;
      const next = prev.entries.filter((_, i) => i !== index);
      return { ...prev, entries: next };
    });
  };

  const updateNewUserRoleEntry = (index: number, value: string) => {
    const next = [...(newUser.roles || [])];
    next[index] = value;
    onChangeUser({ ...newUser, roles: next });
  };

  const removeNewUserRoleEntry = (index: number) => {
    const next = (newUser.roles || []).filter((_, i) => i !== index);
    onChangeUser({ ...newUser, roles: next });
  };

  const appendUserRoleFromPicker = () => {
    const roleName = nextUserRole.trim();
    if (!roleName) return;
    const existing = (newUser.roles || []).some(entry => entry.trim().toLowerCase() === roleName.toLowerCase());
    if (existing) {
      setNextUserRole('');
      return;
    }
    onChangeUser({ ...newUser, roles: [...(newUser.roles || []), roleName] });
    setNextUserRole('');
  };

  const updateNewServiceAccountRoleEntry = (index: number, value: string) => {
    const next = [...(newServiceAccount.roles || [])];
    next[index] = value;
    onChangeServiceAccount({ ...newServiceAccount, roles: next });
  };

  const removeNewServiceAccountRoleEntry = (index: number) => {
    const next = (newServiceAccount.roles || []).filter((_, i) => i !== index);
    onChangeServiceAccount({ ...newServiceAccount, roles: next });
  };

  const appendServiceAccountRoleFromPicker = () => {
    const roleName = nextUserRole.trim();
    if (!roleName) return;
    const existing = (newServiceAccount.roles || []).some(entry => entry.trim().toLowerCase() === roleName.toLowerCase());
    if (existing) {
      setNextUserRole('');
      return;
    }
    onChangeServiceAccount({ ...newServiceAccount, roles: [...(newServiceAccount.roles || []), roleName] });
    setNextUserRole('');
  };

  const statusKey = (value: string) => {
    const key = (value || '').toLowerCase();
    if (key.includes('active')) return 'ok';
    if (key.includes('pending')) return 'warn';
    if (key.includes('blocked') || key.includes('disabled')) return 'danger';
    return 'muted';
  };

  const visiblePolicies = useMemo(() => {
    const combined = [
      ...policyTemplates,
      ...policies.filter(policy => policy.role !== POLICY_TEMPLATE_ROLE && !isProtectedAccessRole(policy.role)),
    ];
    const seen = new Set<string>();
    return combined
      .filter(policy => {
        const key = policyKey(policy);
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      })
      .sort((a, b) => a.role.localeCompare(b.role) || policyLabel(a).localeCompare(policyLabel(b)));
  }, [policies, policyTemplates]);
  const policyCount = visiblePolicies.length;
  const [nextPolicyKey, setNextPolicyKey] = useState('');
  const [nextUserRole, setNextUserRole] = useState('');
  const [nextAccessRole, setNextAccessRole] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const searchQuery = searchTerm.trim().toLowerCase();
  const basicAccessGrants = useMemo(() => accessGrants.filter(isBasicAccessGrant), [accessGrants]);
  const basicGrantOptions = useMemo(
    () => [{ value: ROOT_ACCESS_SCOPE, label: 'Root' }, ...resourceCatalog.folderOptions],
    [resourceCatalog.folderOptions]
  );
  const basicUserGrantMap = useMemo(() => {
    const map = new Map<string, AccessGrantRecord[]>();
    basicAccessGrants.forEach(grant => {
      const user = users.find(entry => accessGrantMatchesUser(grant, entry));
      const key = user?.id || grant.subjectID;
      const entries = map.get(key) || [];
      entries.push(grant);
      map.set(key, entries);
    });
    map.forEach(entries => {
      entries.sort(
        (a, b) =>
          accessGrantSortLabel(a).localeCompare(accessGrantSortLabel(b), undefined, { sensitivity: 'base' }) ||
          a.role.localeCompare(b.role, undefined, { sensitivity: 'base' })
      );
    });
    return map;
  }, [basicAccessGrants, users]);

  const basicServiceAccountGrantMap = useMemo(() => {
    const map = new Map<string, AccessGrantRecord[]>();
    basicAccessGrants.forEach(grant => {
      const account = serviceAccounts.find(entry => accessGrantMatchesServiceAccount(grant, entry));
      const key = account?.sub || grant.subjectID;
      if (!key) return;
      const entries = map.get(key) || [];
      entries.push(grant);
      map.set(key, entries);
    });
    map.forEach(entries => {
      entries.sort(
        (a, b) =>
          accessGrantSortLabel(a).localeCompare(accessGrantSortLabel(b), undefined, { sensitivity: 'base' }) ||
          a.role.localeCompare(b.role, undefined, { sensitivity: 'base' })
      );
    });
    return map;
  }, [basicAccessGrants, serviceAccounts]);

  const sectionContent = ACCESS_SECTION_CONTENT[activeSection];
  const filteredUsers = useMemo(() => {
    if (!searchQuery) return users;
    return users.filter(user => {
      const grants = basicUserGrantMap.get(user.id) || [];
      return matchesAccessSearch(
        searchQuery,
        user.sub,
        user.email,
        user.status,
        (user.roles || []).map(role => role.role).join(' '),
        grants.map(grant => basicAccessGrantLabel(grant)).join(' ')
      );
    });
  }, [basicUserGrantMap, searchQuery, users]);

  const filteredServiceAccounts = useMemo(() => {
    if (!searchQuery) return serviceAccounts;
    return serviceAccounts.filter(account => {
      const grants = basicServiceAccountGrantMap.get(account.sub) || [];
      return matchesAccessSearch(
        searchQuery,
        account.sub,
        account.email,
        account.status,
        String(account.token_count || 0),
        (account.roles || []).map(role => role.role).join(' '),
        grants.map(grant => basicAccessGrantLabel(grant)).join(' ')
      );
    });
  }, [basicServiceAccountGrantMap, searchQuery, serviceAccounts]);

  const filteredRoleDefinitions = useMemo(() => {
    if (!searchQuery) return roleDefinitions;
    return roleDefinitions.filter(role => {
      const assignedUsers = roleUserMap.get(role.id) || [];
      const preset = accessPresetForRole(role.role);
      return matchesAccessSearch(
        searchQuery,
        role.role,
        preset?.label,
        preset?.description,
        role.policies.map(policy => `${policyLabel(policy)} ${policy.obj} ${policy.act}`).join(' '),
        assignedUsers.map(item => `${item.user} ${item.email}`).join(' ')
      );
    });
  }, [roleDefinitions, roleUserMap, searchQuery]);

  const filteredPolicies = useMemo(() => {
    if (!searchQuery) return visiblePolicies;
    return visiblePolicies.filter(policy =>
      matchesAccessSearch(
        searchQuery,
        policy.role,
        policy.name,
        policy.obj,
        policy.act,
        policyLabel(policy),
        formatAccessResourceSummary(policy.obj),
        formatAccessActionSummary(policy.act)
      )
    );
  }, [searchQuery, visiblePolicies]);

  const isNewUserPristine =
    !newUser.sub.trim() &&
    !newUser.email.trim() &&
    !newUser.password &&
    (newUser.roles || []).length === 0 &&
    basicGrantEntries.length === 0;
  const isNewServiceAccountPristine =
    !newServiceAccount.sub.trim() &&
    !newServiceAccount.email.trim() &&
    (newServiceAccount.tokenName.trim() === '' || newServiceAccount.tokenName.trim() === 'default') &&
    (newServiceAccount.roles || []).length === 0 &&
    basicGrantEntries.length === 0 &&
    !createdServiceAccountToken;
  const isNewPolicyPristine =
    !newPermission.name.trim() && newPermission.obj.trim() === 'pipeline:*' && newPermission.act.trim() === 'pipeline.read';
  const selectedBasicUserID = userAccessEditor?.user.id || '';
  const selectedBasicUser = userAccessEditor?.user ?? null;
  const selectedBasicServiceAccountSub = serviceAccountEditor?.account.sub || '';
  const selectedBasicServiceAccount = serviceAccountEditor?.account ?? null;
  const userRoleAssignmentsLocked = isDefaultAdminUser(userAccessEditor?.user);
  const selectedBasicUserGrants = useMemo(
    () => (selectedBasicUserID ? basicUserGrantMap.get(selectedBasicUserID) || [] : []),
    [basicUserGrantMap, selectedBasicUserID]
  );
  const selectedBasicServiceAccountGrants = useMemo(
    () => (selectedBasicServiceAccountSub ? basicServiceAccountGrantMap.get(selectedBasicServiceAccountSub) || [] : []),
    [basicServiceAccountGrantMap, selectedBasicServiceAccountSub]
  );
  const selectedBasicGrants = selectedBasicServiceAccount ? selectedBasicServiceAccountGrants : selectedBasicUserGrants;
  const basicGrantDirty = useMemo(
    () => areBasicGrantEntriesDirty(selectedBasicGrants, basicGrantEntries),
    [basicGrantEntries, selectedBasicGrants]
  );

  useEffect(() => {
    setNextAccessRole('');
  }, [userAccessEditor]);

  useEffect(() => {
    setBasicGrantEntries(selectedBasicUserGrants.map(editableAccessGrantFromRecord));
  }, [selectedBasicUserGrants]);

  useEffect(() => {
    setSearchTerm('');
    setSearchOpen(false);
    setBasicGrantError(null);
    setUserAccessEditor(null);
    setBasicGrantEntries([]);
  }, [accessMode]);

  useEffect(() => {
    setBasicGrantError(null);
    setBasicGrantDraft({ role: '', scope: ROOT_ACCESS_SCOPE });
  }, [userAccessEditor?.user.id]);

  useEffect(() => {
    setSearchTerm('');
    setSearchOpen(false);
    setShowUserModal(false);
    setShowPolicyModal(false);
    setRoleEditor(null);
    setPolicyEditor(null);
    setUserAccessEditor(null);
    setAwaitingUserCreateReset(false);
    setAwaitingPolicyCreateReset(false);
  }, [activeSection]);

  useEffect(() => {
    if (!showUserModal || !awaitingUserCreateReset || !isNewUserPristine) return;
    setShowUserModal(false);
    setAwaitingUserCreateReset(false);
  }, [awaitingUserCreateReset, isNewUserPristine, showUserModal]);

  useEffect(() => {
    if (!showServiceAccountModal || !awaitingServiceAccountCreateReset || !isNewServiceAccountPristine) return;
    setShowServiceAccountModal(false);
    setAwaitingServiceAccountCreateReset(false);
  }, [awaitingServiceAccountCreateReset, isNewServiceAccountPristine, showServiceAccountModal]);

  useEffect(() => {
    if (!showPolicyModal || !awaitingPolicyCreateReset || !isNewPolicyPristine) return;
    setShowPolicyModal(false);
    setAwaitingPolicyCreateReset(false);
  }, [awaitingPolicyCreateReset, isNewPolicyPristine, showPolicyModal]);

  const handleCreateUserInline = async (e: FormEvent<HTMLFormElement>) => {
    setCreatingUserInline(true);
    setAwaitingUserCreateReset(true);
    try {
      const created = await onCreateUser(e, { basicGrants: normalizeEditableBasicGrants(basicGrantEntries) });
      if (created) {
        setBasicGrantError(null);
        setBasicGrantDraft({ role: '', scope: ROOT_ACCESS_SCOPE });
        setBasicGrantEntries([]);
      }
    } finally {
      setCreatingUserInline(false);
    }
  };

  const handleCreateServiceAccountInline = async (e: FormEvent<HTMLFormElement>) => {
    setCreatingServiceAccountInline(true);
    setAwaitingServiceAccountCreateReset(true);
    try {
      const token = await onCreateServiceAccount(e, { basicGrants: normalizeEditableBasicGrants(basicGrantEntries) });
      if (token) {
        setCreatedServiceAccountToken(token);
        setCopyServiceAccountTokenLabel('Copy');
        setBasicGrantError(null);
        setBasicGrantDraft({ role: '', scope: ROOT_ACCESS_SCOPE });
        setBasicGrantEntries([]);
      }
    } finally {
      setCreatingServiceAccountInline(false);
    }
  };

  const handleCreatePolicyInline = async (e: FormEvent<HTMLFormElement>) => {
    setCreatingPolicyInline(true);
    setAwaitingPolicyCreateReset(true);
    try {
      await onCreatePermission(e);
    } finally {
      setCreatingPolicyInline(false);
    }
  };

  const openConfirmDialog = (message: string, onConfirm: () => Promise<void> | void) => {
    setConfirmDialog({ message, onConfirm });
  };

  const confirmDeleteUser = (userId: string) => {
    openConfirmDialog('Delete this user? This cannot be undone.', () => onDeleteUser(userId));
  };

  const confirmDeleteServiceAccount = (serviceAccountId: string) => {
    openConfirmDialog('Delete this service account and revoke its tokens? This cannot be undone.', () => onDeleteServiceAccount(serviceAccountId));
  };

  const confirmDeleteRoleDefinition = (role: RoleDefinition) => {
    if (isProtectedAccessRole(role.role)) return;
    openConfirmDialog('Delete this role and its policies? This cannot be undone.', () => onDeleteRoleDefinition(role));
  };

  const confirmDeletePolicy = (policy: RolePermission) => {
    if (isProtectedAccessRole(policy.role)) return;
    openConfirmDialog('Delete this policy? This cannot be undone.', () => onDeletePolicy(policy));
  };

  const handleConfirmDialog = async () => {
    if (!confirmDialog) return;
    setConfirming(true);
    try {
      await confirmDialog.onConfirm();
    } finally {
      setConfirming(false);
      setConfirmDialog(null);
    }
  };

  const handleRefresh = useCallback(() => {
    if (accessMode === 'basic') {
      void onReloadUsers();
      void onReloadAccessGrants();
      return;
    }
    if (activeSection === 'users') {
      void onReloadUsers();
      void onReloadAccessGrants();
      return;
    }
    if (activeSection === 'service-accounts') {
      void onReloadServiceAccounts();
      void onReloadAccessGrants();
      return;
    }
    if (activeSection === 'policies') {
      void onReloadPolicies();
      return;
    }
    void onReloadUsers();
    void onReloadServiceAccounts();
    if (activeSection === 'roles') {
      void onReloadPolicies();
    }
  }, [accessMode, activeSection, onReloadAccessGrants, onReloadPolicies, onReloadServiceAccounts, onReloadUsers]);

  const handleStageBasicGrant = (e?: FormEvent<HTMLFormElement>) => {
    e?.preventDefault();
    const creatingUser = showUserModal && !userAccessEditor;
    const creatingServiceAccount = showServiceAccountModal && !serviceAccountEditor;
    if (!selectedBasicUser && !selectedBasicServiceAccount && !creatingUser && !creatingServiceAccount) {
      setBasicGrantError('Select an account first.');
      return;
    }
    if (selectedBasicUser && isDefaultAdminUser(selectedBasicUser)) {
      setBasicGrantError('Default admin role assignments are locked.');
      return;
    }
    if (creatingUser && newUser.sub.trim().toLowerCase() === 'admin') {
      setBasicGrantError('Default admin role assignments are locked.');
      return;
    }

    const result = stageBasicGrant(
      basicGrantEntries,
      basicGrantDraft,
      `draft-${Date.now()}-${Math.random().toString(36).slice(2)}`
    );
    setBasicGrantError(result.error);
    if (result.error) return;
    setBasicGrantEntries(result.entries);
    setBasicGrantDraft(prev => ({ ...prev, role: '' }));
  };

  const removeBasicGrantDraft = (localID: string) => {
    if (isDefaultAdminUser(selectedBasicUser)) return;
    setBasicGrantEntries(prev => prev.filter(grant => grant.localID !== localID));
    setBasicGrantError(null);
  };

  const resetBasicGrantDrafts = () => {
    if (isDefaultAdminUser(selectedBasicUser)) return;
    setBasicGrantEntries(selectedBasicGrants.map(editableAccessGrantFromRecord));
    setBasicGrantError(null);
  };

  const saveBasicGrantsForUser = async (user: UserSummary) => {
    if (!selectedBasicUser) {
      setBasicGrantError('Select a user first.');
      return;
    }
    if (isDefaultAdminUser(user)) {
      setBasicGrantError('Default admin role assignments are locked.');
      return;
    }
    if (!basicGrantDirty) return;
    const { grantsToDelete, grantsToAdd } = buildBasicGrantChangeSet(selectedBasicUserGrants, basicGrantEntries);

    setBasicGrantSaving(true);
    setBasicGrantError(null);
    try {
      for (const grant of grantsToDelete) {
        await onDeleteAccessGrant(grant.id);
      }
      for (const grant of grantsToAdd) {
        await onCreateAccessGrant({
          userID: user.id,
          role: grant.role,
          resourceType: grant.resourceType,
          resourceID: grant.resourceID,
          inherit: grant.inherit,
        });
      }
    } catch (error) {
      setBasicGrantError(error instanceof Error ? error.message : 'Failed to save basic roles');
      throw error;
    } finally {
      setBasicGrantSaving(false);
    }
  };

  const saveBasicGrantsForServiceAccount = async (account: ServiceAccountSummary) => {
    if (!selectedBasicServiceAccount) {
      setBasicGrantError('Select a service account first.');
      return;
    }
    if (!basicGrantDirty) return;
    const { grantsToDelete, grantsToAdd } = buildBasicGrantChangeSet(
      selectedBasicServiceAccountGrants,
      basicGrantEntries
    );

    setBasicGrantSaving(true);
    setBasicGrantError(null);
    try {
      for (const grant of grantsToDelete) {
        await onDeleteAccessGrant(grant.id);
      }
      for (const grant of grantsToAdd) {
        await onCreateServiceAccountAccessGrant({
          serviceAccountSub: account.sub,
          role: grant.role,
          resourceType: grant.resourceType,
          resourceID: grant.resourceID,
          inherit: grant.inherit,
        });
      }
    } catch (error) {
      setBasicGrantError(error instanceof Error ? error.message : 'Failed to save basic roles');
      throw error;
    } finally {
      setBasicGrantSaving(false);
    }
  };

  const handleCreateServiceAccountToken = async () => {
    if (!serviceAccountEditor) return;
    const name = serviceAccountEditor.tokenName.trim();
    if (!name) {
      setServiceAccountEditor(prev => (prev ? { ...prev, tokensError: 'Token name is required.' } : prev));
      return;
    }
    setServiceAccountEditor(prev => (prev ? { ...prev, tokensLoading: true, tokensError: null } : prev));
    try {
      const token = await onCreateServiceAccountToken(serviceAccountEditor.account.id, name);
      const tokens = await onLoadServiceAccountTokens(serviceAccountEditor.account.id);
      setCreatedServiceAccountToken(token);
      setCopyServiceAccountTokenLabel('Copy');
      setServiceAccountEditor(prev => (
        prev?.account.id === serviceAccountEditor.account.id
          ? { ...prev, tokenName: 'rotation', tokens, tokensLoading: false, tokensError: null }
          : prev
      ));
    } catch (error) {
      setServiceAccountEditor(prev => (
        prev ? { ...prev, tokensLoading: false, tokensError: error instanceof Error ? error.message : 'Failed to create token' } : prev
      ));
    }
  };

  const handleRevokeServiceAccountToken = async (tokenID: string) => {
    if (!serviceAccountEditor) return;
    setServiceAccountEditor(prev => (prev ? { ...prev, tokensLoading: true, tokensError: null } : prev));
    try {
      await onRevokeServiceAccountToken(serviceAccountEditor.account.id, tokenID);
      const tokens = await onLoadServiceAccountTokens(serviceAccountEditor.account.id);
      setServiceAccountEditor(prev => (
        prev?.account.id === serviceAccountEditor.account.id ? { ...prev, tokens, tokensLoading: false, tokensError: null } : prev
      ));
    } catch (error) {
      setServiceAccountEditor(prev => (
        prev ? { ...prev, tokensLoading: false, tokensError: error instanceof Error ? error.message : 'Failed to revoke token' } : prev
      ));
    }
  };

  const copyCreatedServiceAccountToken = async () => {
    const token = createdServiceAccountToken?.token;
    if (!token || !navigator.clipboard) return;
    await navigator.clipboard.writeText(token);
    setCopyServiceAccountTokenLabel('Copied');
    window.setTimeout(() => setCopyServiceAccountTokenLabel('Copy'), 1800);
  };

  const availablePoliciesForRoleEditor = roleEditor ? policyOptions : [];

  const createUserEditor = (
    <div className="access-editor-surface">
      <div className="access-editor-header">
        <div>
          <p className="access-editor-kicker">Create user</p>
          <h5 className="access-editor-title">New local account</h5>
          <p className="access-editor-text">Create a local account.</p>
        </div>
        <button
          type="button"
          className="access-inline-btn access-inline-btn--pill"
          onClick={() => {
            setAwaitingUserCreateReset(false);
            setShowUserModal(false);
            setBasicGrantError(null);
            setBasicGrantDraft({ role: '', scope: ROOT_ACCESS_SCOPE });
            setBasicGrantEntries([]);
          }}
        >
          Close
        </button>
      </div>
      <form className="access-editor-form" onSubmit={handleCreateUserInline}>
        <div className="access-editor-grid">
          <label className="access-minimal-label">
            <span>Username (sub)</span>
            <input
              className="pipelines-input"
              value={newUser.sub}
              onChange={e => onChangeUser({ ...newUser, sub: e.target.value })}
              placeholder="alice"
              required
            />
          </label>
          <label className="access-minimal-label">
            <span>Email</span>
            <input
              className="pipelines-input"
              type="email"
              value={newUser.email}
              onChange={e => onChangeUser({ ...newUser, email: e.target.value })}
              placeholder="name@example.com"
            />
          </label>
        </div>
        <label className="access-minimal-label">
          <span>Password</span>
          <input
            className="pipelines-input"
            type="password"
            value={newUser.password}
            onChange={e => onChangeUser({ ...newUser, password: e.target.value })}
            placeholder="••••••••"
            required
          />
        </label>
        <div className="access-editor-section">
          <div className="access-minimal-section__header">
            <p className="text-sm font-medium text-[var(--text-primary)]">Access roles</p>
            <span className="text-[11px] text-[var(--text-secondary)]">Optional with basic roles</span>
          </div>
          <div className="space-y-2">
            {newUser.roles.length === 0 && <p className="text-[11px] text-[var(--text-secondary)]">Add access roles here or use basic roles below.</p>}
            {newUser.roles.map((entry, idx) => (
              <div key={`new-user-role-${idx}`} className="access-minimal-row">
                <select
                  className="pipelines-input flex-1"
                  value={entry}
                  onChange={e => updateNewUserRoleEntry(idx, e.target.value)}
                  required
                  disabled={allRoleOptions.length === 0}
                >
                  <option value="" disabled>
                    {allRoleOptions.length === 0 ? 'No roles available' : 'Pick a role'}
                  </option>
                  {allRoleOptions.map(role => (
                    <option key={`role-opt-${role}`} value={role}>
                      {role}
                    </option>
                  ))}
                </select>
                <button type="button" className="access-inline-btn access-inline-btn--danger" onClick={() => removeNewUserRoleEntry(idx)} title="Remove role">
                  <TrashIcon />
                </button>
              </div>
            ))}
            <div className="access-editor-inline-add">
              <select
                className="pipelines-input flex-1"
                value={nextUserRole}
                onChange={e => setNextUserRole(e.target.value)}
                disabled={allRoleOptions.length === 0}
              >
                <option value="">{allRoleOptions.length === 0 ? 'No roles available' : 'Pick a role'}</option>
                {allRoleOptions.map(role => (
                  <option key={`new-role-opt-${role}`} value={role}>
                    {role}
                  </option>
                ))}
              </select>
              <button type="button" className="glass-button-subtle" onClick={appendUserRoleFromPicker} disabled={allRoleOptions.length === 0}>
                Add access role
              </button>
            </div>
          </div>
        </div>
        <BasicAccessGrantEditor
          entries={basicGrantEntries}
          draft={basicGrantDraft}
          options={basicGrantOptions}
          error={basicGrantError}
          saving={creatingUserInline}
          addLabel="Add basic role"
          toneClassForRole={accessPresetToneClass}
          onDraftChange={setBasicGrantDraft}
          onAdd={() => handleStageBasicGrant()}
          onRemove={removeBasicGrantDraft}
        />
        <div className="access-editor-footer">
          <button type="submit" className="glass-button-primary" disabled={creatingUserInline}>
            {creatingUserInline ? 'Saving…' : 'Save user'}
          </button>
        </div>
      </form>
    </div>
  );

  const serviceAccountTokenReveal = createdServiceAccountToken?.token ? (
    <div className="access-token-reveal">
      <div className="min-w-0">
        <p className="access-card__label">One-time token</p>
        <code>{createdServiceAccountToken.token}</code>
        <p className="text-[11px] text-[var(--text-secondary)] mt-2">Store this value now. It will not be shown again.</p>
      </div>
      <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={copyCreatedServiceAccountToken}>
        <Copy className="h-4 w-4" />
        <span>{copyServiceAccountTokenLabel}</span>
      </button>
    </div>
  ) : null;

  const createServiceAccountEditor = (
    <div className="access-editor-surface">
      <div className="access-editor-header">
        <div>
          <p className="access-editor-kicker">Create service account</p>
          <h5 className="access-editor-title">New integration identity</h5>
          <p className="access-editor-text">Create an account that authenticates only with service account tokens.</p>
        </div>
        <button
          type="button"
          className="access-inline-btn access-inline-btn--pill"
          onClick={() => {
            setAwaitingServiceAccountCreateReset(false);
            setShowServiceAccountModal(false);
            setCreatedServiceAccountToken(null);
            setBasicGrantError(null);
            setBasicGrantDraft({ role: '', scope: ROOT_ACCESS_SCOPE });
            setBasicGrantEntries([]);
          }}
        >
          Close
        </button>
      </div>
      {serviceAccountTokenReveal}
      {!createdServiceAccountToken && (
        <form className="access-editor-form" onSubmit={handleCreateServiceAccountInline}>
          <div className="access-editor-grid">
            <label className="access-minimal-label">
              <span>Service account ID</span>
              <input
                className="pipelines-input"
                value={newServiceAccount.sub}
                onChange={e => onChangeServiceAccount({ ...newServiceAccount, sub: e.target.value })}
                placeholder="deploy-bot"
                required
              />
            </label>
            <label className="access-minimal-label">
              <span>Contact email</span>
              <input
                className="pipelines-input"
                type="email"
                value={newServiceAccount.email}
                onChange={e => onChangeServiceAccount({ ...newServiceAccount, email: e.target.value })}
                placeholder="platform@example.com"
              />
            </label>
          </div>
          <label className="access-minimal-label">
            <span>Initial token name</span>
            <input
              className="pipelines-input"
              value={newServiceAccount.tokenName}
              onChange={e => onChangeServiceAccount({ ...newServiceAccount, tokenName: e.target.value })}
              placeholder="default"
              required
            />
          </label>
          <div className="access-editor-section">
            <div className="access-minimal-section__header">
              <p className="text-sm font-medium text-[var(--text-primary)]">Access roles</p>
              <span className="text-[11px] text-[var(--text-secondary)]">Optional with basic roles</span>
            </div>
            <div className="space-y-2">
              {newServiceAccount.roles.length === 0 && <p className="text-[11px] text-[var(--text-secondary)]">Add access roles here or use basic roles below.</p>}
              {newServiceAccount.roles.map((entry, idx) => (
                <div key={`new-service-account-role-${idx}`} className="access-minimal-row">
                  <select
                    className="pipelines-input flex-1"
                    value={entry}
                    onChange={e => updateNewServiceAccountRoleEntry(idx, e.target.value)}
                    required
                    disabled={allRoleOptions.length === 0}
                  >
                    <option value="" disabled>
                      {allRoleOptions.length === 0 ? 'No roles available' : 'Pick a role'}
                    </option>
                    {allRoleOptions.map(role => (
                      <option key={`service-role-opt-${role}`} value={role}>
                        {role}
                      </option>
                    ))}
                  </select>
                  <button type="button" className="access-inline-btn access-inline-btn--danger" onClick={() => removeNewServiceAccountRoleEntry(idx)} title="Remove role">
                    <TrashIcon />
                  </button>
                </div>
              ))}
              <div className="access-editor-inline-add">
                <select
                  className="pipelines-input flex-1"
                  value={nextUserRole}
                  onChange={e => setNextUserRole(e.target.value)}
                  disabled={allRoleOptions.length === 0}
                >
                  <option value="">{allRoleOptions.length === 0 ? 'No roles available' : 'Pick a role'}</option>
                  {allRoleOptions.map(role => (
                    <option key={`new-service-role-opt-${role}`} value={role}>
                      {role}
                    </option>
                  ))}
                </select>
                <button type="button" className="glass-button-subtle" onClick={appendServiceAccountRoleFromPicker} disabled={allRoleOptions.length === 0}>
                  Add access role
                </button>
              </div>
            </div>
          </div>
          <BasicAccessGrantEditor
            entries={basicGrantEntries}
            draft={basicGrantDraft}
            options={basicGrantOptions}
            error={basicGrantError}
            saving={creatingServiceAccountInline}
            addLabel="Add basic role"
            toneClassForRole={accessPresetToneClass}
            onDraftChange={setBasicGrantDraft}
            onAdd={() => handleStageBasicGrant()}
            onRemove={removeBasicGrantDraft}
          />
          <div className="access-editor-footer">
            <button type="submit" className="glass-button-primary" disabled={creatingServiceAccountInline}>
              {creatingServiceAccountInline ? 'Saving…' : 'Save service account'}
            </button>
          </div>
        </form>
      )}
    </div>
  );
  const accessSearchPlaceholder = accessMode === 'basic' ? 'Search by username, email, role, or group' : sectionContent.searchPlaceholder;
  const accessSearchControl = (
    <div className={`pipelines-search-shell access-search-shell ${searchOpen ? 'open' : ''}`}>
      <button
        type="button"
        className="pipelines-search-toggle"
        aria-label={accessSearchPlaceholder}
        onClick={() => {
          setSearchOpen(true);
          requestAnimationFrame(() => searchInputRef.current?.focus());
        }}
      >
        <SearchIcon />
      </button>
      <input
        ref={searchInputRef}
        id={`access-${accessMode}-${activeSection}-search`}
        type="text"
        placeholder={accessSearchPlaceholder}
        className="pipelines-search-input"
        value={searchTerm}
        onChange={event => {
          setSearchTerm(event.target.value);
          if (event.target.value && !searchOpen) setSearchOpen(true);
        }}
        onBlur={() => {
          if (!searchTerm.trim()) setSearchOpen(false);
        }}
      />
      {(searchTerm || searchOpen) && (
        <button
          type="button"
          className="pipelines-search-clear"
          onClick={() => {
            setSearchTerm('');
            setSearchOpen(false);
            searchInputRef.current?.blur();
          }}
          aria-label="Clear search"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  );
  const usersWorkspace = (
    <div className="access-workspace">
      <div className="space-y-4 access-workspace__list">
        {(error || accessGrantsError) && (
          <div className="access-error-banner">
            {error ? `Failed to load users: ${error}` : `Failed to load basic roles: ${accessGrantsError}`}
          </div>
        )}
        {loading || accessGrantsLoading ? (
          <div className="access-empty-card">
            <p className="font-medium text-[var(--text-primary)]">Loading people…</p>
            <p className="text-sm text-[var(--text-secondary)]">Fetching accounts and current role assignments.</p>
          </div>
        ) : users.length === 0 ? (
          <div className="access-empty-card">
            <p className="font-medium text-[var(--text-primary)]">No users yet</p>
            <p className="text-sm text-[var(--text-secondary)]">Create a local account, then assign access and basic roles.</p>
          </div>
        ) : filteredUsers.length === 0 ? (
          <div className="access-empty-card">
            <p className="font-medium text-[var(--text-primary)]">No people match this search</p>
            <p className="text-sm text-[var(--text-secondary)]">Try a username, email address, role, or group path.</p>
          </div>
        ) : (
          <div className="access-entity-grid access-entity-grid--users">
            {filteredUsers.map(user => {
              const userRoles = user.roles || [];
              const grants = basicUserGrantMap.get(user.id) || [];
              const isSelected = userAccessEditor?.user.id === user.id;
              return (
                <article key={user.id} className={`access-card access-card--user ${isSelected ? 'access-card--selected' : ''}`}>
                  <div className="access-card__header">
                    <div className="min-w-0 flex items-center gap-3">
                      <div className="access-avatar">{(user.sub || user.email || 'U').charAt(0).toUpperCase()}</div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          <p className="access-card__title">{user.sub}</p>
                          <span className={`access-status access-status--${statusKey(user.status)}`}>{user.status || 'unknown'}</span>
                        </div>
                        <p className="access-card__subtitle">{user.email || 'No email address'}</p>
                        <p className="access-card__meta-line">
                          {user.last_login ? `Last sign-in ${formatAccessTimestamp(user.last_login)}` : 'Never signed in'}
                        </p>
                      </div>
                    </div>
                    <div className="access-card__actions">
                      <button
                        type="button"
                        className="access-card-action"
                        title="Edit user"
                        aria-label={`Edit ${user.sub || user.email || 'user'}`}
                        onClick={() => openUserAccessModal(user)}
                      >
                        <EditIcon />
                      </button>
                      <button
                        type="button"
                        className="access-card-action access-card-action--danger"
                        title="Delete user"
                        aria-label={`Delete ${user.sub || user.email || 'user'}`}
                        onClick={() => confirmDeleteUser(user.id)}
                        disabled={loading}
                      >
                        <TrashIcon />
                      </button>
                    </div>
                  </div>
                  <div className="space-y-2">
                    <p className="access-card__label">Access roles</p>
                    <div className="flex flex-wrap gap-2">
                      {userRoles.length > 0 ? (
                        userRoles.map(role => (
                          <span key={`${user.id}-${role.role}`} className={`access-chip ${accessPresetToneClass(role.role)}`}>
                            {role.role}
                          </span>
                        ))
                      ) : (
                        <span className="text-sm text-[var(--text-secondary)]">No roles assigned yet</span>
                      )}
                    </div>
                  </div>
                  <div className="space-y-2">
                    <p className="access-card__label">Basic roles</p>
                    <div className="flex flex-wrap gap-2">
                      {grants.length > 0 ? (
                        grants.slice(0, 4).map(grant => (
                          <span key={grant.id} className={`access-chip ${accessPresetToneClass(grant.role)}`}>
                            {basicAccessGrantLabel(grant)}
                          </span>
                        ))
                      ) : (
                        <span className="text-sm text-[var(--text-secondary)]">No basic roles yet</span>
                      )}
                      {grants.length > 4 && <span className="access-chip access-chip--muted">+ {grants.length - 4} more</span>}
                    </div>
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </div>
      <aside className="access-editor-pane">
        {userAccessEditor ? (
          <div className="access-editor-surface access-editor-surface--minimal">
            <div className="access-editor-header">
              <div>
                <p className="access-editor-kicker">Edit user</p>
                <h5 className="access-editor-title">{userAccessEditor.user.sub}</h5>
                <p className="access-editor-text">Manage account details, access roles, and group-scoped basic roles.</p>
              </div>
              <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={() => setUserAccessEditor(null)}>
                Close
              </button>
            </div>
            <form className="access-editor-form access-editor-form--compact" onSubmit={handleSaveUserAccess}>
              <div className="access-editor-grid">
                <label className="access-minimal-label">
                  <span>Email</span>
                  <input
                    className="pipelines-input"
                    type="email"
                    value={userAccessEditor.email}
                    onChange={e => setUserAccessEditor(prev => (prev ? { ...prev, email: e.target.value } : prev))}
                    placeholder="name@example.com"
                  />
                </label>
                <label className="access-minimal-label">
                  <span>Status</span>
                  <select
                    className="pipelines-input"
                    value={userAccessEditor.status}
                    onChange={e => setUserAccessEditor(prev => (prev ? { ...prev, status: e.target.value } : prev))}
                    disabled={userAccessEditor.user.sub === 'admin'}
                  >
                    <option value="active">Active</option>
                    <option value="disabled">Disabled</option>
                  </select>
                </label>
              </div>
              <label className="access-minimal-label">
                <span>New password</span>
                <input
                  className="pipelines-input"
                  type="password"
                  value={userAccessEditor.password}
                  onChange={e => setUserAccessEditor(prev => (prev ? { ...prev, password: e.target.value } : prev))}
                  placeholder="Leave blank to keep current password"
                />
              </label>
              <div className="access-editor-section access-editor-section--plain">
                <div className="access-minimal-section__header">
                  <p className="text-sm font-medium text-[var(--text-primary)]">Access roles</p>
                  <span className="text-[11px] text-[var(--text-secondary)]">
                    {userRoleAssignmentsLocked ? 'Locked' : `${userAccessEditor.entries.length} assigned`}
                  </span>
                </div>
                <div className="space-y-2">
                  {userAccessEditor.entries.length === 0 && <p className="text-[12px] text-[var(--text-secondary)]">No roles assigned yet.</p>}
                  {userAccessEditor.entries.map((entry, idx) => {
                    const label = userRoleAssignmentsLocked ? 'Protected admin role assignment' : 'Remove assignment';
                    return (
                      <div key={`user-role-${idx}`} className="access-minimal-row justify-between">
                        <span className={`access-chip ${accessPresetToneClass(entry)}`}>{entry || 'Role'}</span>
                        <button
                          type="button"
                          className={`access-inline-btn access-inline-btn--danger access-role-remove ${userRoleAssignmentsLocked ? 'opacity-60 cursor-not-allowed' : ''}`}
                          onClick={() => removeUserAccessEntry(idx)}
                          title={label}
                          aria-label={label}
                          disabled={userRoleAssignmentsLocked}
                        >
                          <TrashIcon />
                        </button>
                      </div>
                    );
                  })}
                  <div className="access-editor-inline-add">
                    <select className="pipelines-input w-full" value={nextAccessRole} onChange={e => setNextAccessRole(e.target.value)} disabled={userRoleAssignmentsLocked}>
                      <option value="">{allRoleOptions.length === 0 ? 'No roles available' : 'Select a role'}</option>
                      {allRoleOptions.map(role => (
                        <option key={`access-role-${role}`} value={role}>
                          {role}
                        </option>
                      ))}
                    </select>
                    <button type="button" className="glass-button-subtle" onClick={addUserAccessEntry} disabled={userRoleAssignmentsLocked || !nextAccessRole || allRoleOptions.length === 0}>
                      Add
                    </button>
                  </div>
                </div>
              </div>
              <BasicAccessGrantEditor
                entries={basicGrantEntries}
                draft={basicGrantDraft}
                options={basicGrantOptions}
                error={basicGrantError}
                disabled={userRoleAssignmentsLocked}
                saving={basicGrantSaving}
                plain
                countLabel={userRoleAssignmentsLocked ? 'Locked' : undefined}
                showGrantedBy
                toneClassForRole={accessPresetToneClass}
                onDraftChange={setBasicGrantDraft}
                onAdd={() => handleStageBasicGrant()}
                onRemove={removeBasicGrantDraft}
              />
              <div className="access-editor-footer gap-2">
                {basicGrantDirty && (
                  <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={resetBasicGrantDrafts} disabled={userRoleAssignmentsLocked || basicGrantSaving || savingUserAccess}>
                    Reset basic roles
                  </button>
                )}
                <button type="submit" className="glass-button-primary" disabled={savingUserAccess || basicGrantSaving}>
                  {savingUserAccess || basicGrantSaving ? 'Saving…' : 'Save changes'}
                </button>
              </div>
            </form>
          </div>
        ) : showUserModal ? (
          createUserEditor
        ) : (
          <AccessEditorEmptyState sectionLabel="User details" hint="Select a user to edit access." />
        )}
      </aside>
    </div>
  );

  const serviceAccountsWorkspace = (
    <div className="access-workspace">
      <div className="space-y-4 access-workspace__list">
        {(serviceAccountsError || accessGrantsError) && (
          <div className="access-error-banner">
            {serviceAccountsError ? `Failed to load service accounts: ${serviceAccountsError}` : `Failed to load basic roles: ${accessGrantsError}`}
          </div>
        )}
        {serviceAccountsLoading || accessGrantsLoading ? (
          <div className="access-empty-card">
            <p className="font-medium text-[var(--text-primary)]">Loading service accounts…</p>
            <p className="text-sm text-[var(--text-secondary)]">Fetching integration identities, tokens, and role assignments.</p>
          </div>
        ) : serviceAccounts.length === 0 ? (
          <div className="access-empty-card">
            <p className="font-medium text-[var(--text-primary)]">No service accounts yet</p>
            <p className="text-sm text-[var(--text-secondary)]">Create a token-only account for integrations and automation.</p>
          </div>
        ) : filteredServiceAccounts.length === 0 ? (
          <div className="access-empty-card">
            <p className="font-medium text-[var(--text-primary)]">No service accounts match this search</p>
            <p className="text-sm text-[var(--text-secondary)]">Try a service account ID, contact email, role, or group path.</p>
          </div>
        ) : (
          <div className="access-entity-grid access-entity-grid--users">
            {filteredServiceAccounts.map(account => {
              const accountRoles = account.roles || [];
              const grants = basicServiceAccountGrantMap.get(account.sub) || [];
              const isSelected = serviceAccountEditor?.account.id === account.id;
              return (
                <article key={account.id} className={`access-card access-card--user ${isSelected ? 'access-card--selected' : ''}`}>
                  <div className="access-card__header">
                    <div className="min-w-0 flex items-center gap-3">
                      <div className="access-avatar">
                        <Server className="h-4 w-4" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          <p className="access-card__title">{account.sub}</p>
                          <span className={`access-status access-status--${statusKey(account.status)}`}>{account.status || 'unknown'}</span>
                        </div>
                        <p className="access-card__subtitle">{account.email || 'No contact email'}</p>
                        <p className="access-card__meta-line">
                          {account.last_used_at ? `Last token use ${formatAccessTimestamp(account.last_used_at)}` : 'No token activity yet'} · {formatAccessCount(account.token_count || 0, 'token')}
                        </p>
                      </div>
                    </div>
                    <div className="access-card__actions">
                      <button
                        type="button"
                        className="access-card-action"
                        title="Edit service account"
                        aria-label={`Edit ${account.sub || 'service account'}`}
                        onClick={() => openServiceAccountAccessModal(account)}
                      >
                        <EditIcon />
                      </button>
                      <button
                        type="button"
                        className="access-card-action access-card-action--danger"
                        title="Delete service account"
                        aria-label={`Delete ${account.sub || 'service account'}`}
                        onClick={() => confirmDeleteServiceAccount(account.id)}
                        disabled={serviceAccountsLoading}
                      >
                        <TrashIcon />
                      </button>
                    </div>
                  </div>
                  <div className="space-y-2">
                    <p className="access-card__label">Access roles</p>
                    <div className="flex flex-wrap gap-2">
                      {accountRoles.length > 0 ? (
                        accountRoles.map(role => (
                          <span key={`${account.id}-${role.role}`} className={`access-chip ${accessPresetToneClass(role.role)}`}>
                            {role.role}
                          </span>
                        ))
                      ) : (
                        <span className="text-sm text-[var(--text-secondary)]">No roles assigned yet</span>
                      )}
                    </div>
                  </div>
                  <div className="space-y-2">
                    <p className="access-card__label">Basic roles</p>
                    <div className="flex flex-wrap gap-2">
                      {grants.length > 0 ? (
                        grants.slice(0, 4).map(grant => (
                          <span key={grant.id} className={`access-chip ${accessPresetToneClass(grant.role)}`}>
                            {basicAccessGrantLabel(grant)}
                          </span>
                        ))
                      ) : (
                        <span className="text-sm text-[var(--text-secondary)]">No basic roles yet</span>
                      )}
                      {grants.length > 4 && <span className="access-chip access-chip--muted">+ {grants.length - 4} more</span>}
                    </div>
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </div>
      <aside className="access-editor-pane">
        {serviceAccountEditor ? (
          <div className="access-editor-surface access-editor-surface--minimal">
            <div className="access-editor-header">
              <div>
                <p className="access-editor-kicker">Edit service account</p>
                <h5 className="access-editor-title">{serviceAccountEditor.account.sub}</h5>
                <p className="access-editor-text">Manage token-only integration access and scoped basic roles.</p>
              </div>
              <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={() => setServiceAccountEditor(null)}>
                Close
              </button>
            </div>
            {serviceAccountTokenReveal}
            <form className="access-editor-form access-editor-form--compact" onSubmit={handleSaveServiceAccountAccess}>
              <div className="access-editor-grid">
                <label className="access-minimal-label">
                  <span>Contact email</span>
                  <input
                    className="pipelines-input"
                    type="email"
                    value={serviceAccountEditor.email}
                    onChange={e => setServiceAccountEditor(prev => (prev ? { ...prev, email: e.target.value } : prev))}
                    placeholder="platform@example.com"
                  />
                </label>
                <label className="access-minimal-label">
                  <span>Status</span>
                  <select
                    className="pipelines-input"
                    value={serviceAccountEditor.status}
                    onChange={e => setServiceAccountEditor(prev => (prev ? { ...prev, status: e.target.value } : prev))}
                  >
                    <option value="active">Active</option>
                    <option value="disabled">Disabled</option>
                  </select>
                </label>
              </div>
              <div className="access-editor-section access-editor-section--plain">
                <div className="access-minimal-section__header">
                  <p className="text-sm font-medium text-[var(--text-primary)]">Tokens</p>
                  <span className="text-[11px] text-[var(--text-secondary)]">{serviceAccountEditor.tokens.length} active</span>
                </div>
                <div className="access-editor-inline-add">
                  <input
                    className="pipelines-input flex-1"
                    value={serviceAccountEditor.tokenName}
                    onChange={e => setServiceAccountEditor(prev => (prev ? { ...prev, tokenName: e.target.value } : prev))}
                    placeholder="rotation"
                  />
                  <button type="button" className="glass-button-subtle" onClick={handleCreateServiceAccountToken} disabled={serviceAccountEditor.tokensLoading || !serviceAccountEditor.tokenName.trim()}>
                    Create token
                  </button>
                </div>
                {serviceAccountEditor.tokensError && <div className="access-error-banner">{serviceAccountEditor.tokensError}</div>}
                <div className="space-y-2">
                  {serviceAccountEditor.tokensLoading ? (
                    <p className="text-[12px] text-[var(--text-secondary)]">Loading tokens…</p>
                  ) : serviceAccountEditor.tokens.length === 0 ? (
                    <p className="text-[12px] text-[var(--text-secondary)]">No active tokens.</p>
                  ) : (
                    serviceAccountEditor.tokens.map(token => (
                      <div key={token.id} className="access-minimal-row access-minimal-row--stack">
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className="access-chip access-chip--muted">{token.name}</span>
                            <span className="access-chip access-chip--muted">••••{token.token_suffix}</span>
                          </div>
                          <p className="text-[11px] text-[var(--text-secondary)] mt-2">
                            Created {formatAccessTimestamp(token.created_at)}
                            {token.expires_at ? ` · Expires ${formatAccessTimestamp(token.expires_at)}` : ' · Never expires'}
                            {token.last_used_at ? ` · Last used ${formatAccessTimestamp(token.last_used_at)}` : ''}
                          </p>
                        </div>
                        <button type="button" className="access-inline-btn access-inline-btn--danger" onClick={() => handleRevokeServiceAccountToken(token.id)} disabled={serviceAccountEditor.tokensLoading}>
                          <TrashIcon />
                          <span>Revoke</span>
                        </button>
                      </div>
                    ))
                  )}
                </div>
              </div>
              <div className="access-editor-section access-editor-section--plain">
                <div className="access-minimal-section__header">
                  <p className="text-sm font-medium text-[var(--text-primary)]">Access roles</p>
                  <span className="text-[11px] text-[var(--text-secondary)]">{serviceAccountEditor.entries.length} assigned</span>
                </div>
                <div className="space-y-2">
                  {serviceAccountEditor.entries.length === 0 && <p className="text-[12px] text-[var(--text-secondary)]">No roles assigned yet.</p>}
                  {serviceAccountEditor.entries.map((entry, idx) => (
                    <div key={`service-account-role-${idx}`} className="access-minimal-row justify-between">
                      <span className={`access-chip ${accessPresetToneClass(entry)}`}>{entry || 'Role'}</span>
                      <button
                        type="button"
                        className="access-inline-btn access-inline-btn--danger access-role-remove"
                        onClick={() => removeServiceAccountAccessEntry(idx)}
                        title="Remove assignment"
                        aria-label="Remove assignment"
                      >
                        <TrashIcon />
                      </button>
                    </div>
                  ))}
                  <div className="access-editor-inline-add">
                    <select className="pipelines-input w-full" value={nextAccessRole} onChange={e => setNextAccessRole(e.target.value)}>
                      <option value="">{allRoleOptions.length === 0 ? 'No roles available' : 'Select a role'}</option>
                      {allRoleOptions.map(role => (
                        <option key={`service-access-role-${role}`} value={role}>
                          {role}
                        </option>
                      ))}
                    </select>
                    <button type="button" className="glass-button-subtle" onClick={addServiceAccountAccessEntry} disabled={!nextAccessRole || allRoleOptions.length === 0}>
                      Add
                    </button>
                  </div>
                </div>
              </div>
              <BasicAccessGrantEditor
                entries={basicGrantEntries}
                draft={basicGrantDraft}
                options={basicGrantOptions}
                error={basicGrantError}
                saving={basicGrantSaving}
                plain
                showGrantedBy
                toneClassForRole={accessPresetToneClass}
                onDraftChange={setBasicGrantDraft}
                onAdd={() => handleStageBasicGrant()}
                onRemove={removeBasicGrantDraft}
              />
              <div className="access-editor-footer gap-2">
                {basicGrantDirty && (
                  <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={resetBasicGrantDrafts} disabled={basicGrantSaving || savingServiceAccountAccess}>
                    Reset basic roles
                  </button>
                )}
                <button type="submit" className="glass-button-primary" disabled={savingServiceAccountAccess || basicGrantSaving}>
                  {savingServiceAccountAccess || basicGrantSaving ? 'Saving…' : 'Save changes'}
                </button>
              </div>
            </form>
          </div>
        ) : showServiceAccountModal ? (
          createServiceAccountEditor
        ) : (
          <AccessEditorEmptyState sectionLabel="Service account details" hint="Select a service account to edit access and tokens." />
        )}
      </aside>
    </div>
  );

  return (
    <div className="access-layout pb-24" data-access-build={ACCESS_UI_BUILD_ID}>
      <div className="access-shell">
        <div className="access-header">
          <div className="access-title-group">
            <h3 className="access-header__title">Access</h3>
          </div>
          <div className="access-header__actions">
            <div className="access-mode-switch" role="tablist" aria-label="Access mode">
              <button
                type="button"
                role="tab"
                aria-selected={accessMode === 'basic'}
                className={`access-mode-switch__option ${accessMode === 'basic' ? 'access-mode-switch__option--active' : ''}`}
                onClick={() => setAccessMode('basic')}
              >
                <span className="access-mode-switch__title">Basic</span>
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={accessMode === 'advanced'}
                className={`access-mode-switch__option ${accessMode === 'advanced' ? 'access-mode-switch__option--active' : ''}`}
                onClick={() => setAccessMode('advanced')}
              >
                <span className="access-mode-switch__title">Advanced</span>
              </button>
            </div>
            <button
              className="glass-button-ghost access-toolbar-btn"
              type="button"
              onClick={handleRefresh}
              disabled={loading || serviceAccountsLoading || accessGrantsLoading || policiesLoading}
            >
              <RefreshIcon />
              <span>Refresh</span>
            </button>
          </div>
        </div>

        {accessMode === 'advanced' && (
          <div className="access-nav">
            <div className="access-tabs">
              {tabItems.map(tab => (
                <button
                  key={tab.id}
                  type="button"
                  className={`access-tab ${activeSection === tab.id ? 'access-tab--active' : ''}`}
                  onClick={() => setActiveSection(tab.id)}
                >
                  <span className="access-tab__label">{tab.label}</span>
                  <span className="access-tab__badge">{tab.id === 'policies' ? policyCount : tab.count}</span>
                </button>
              ))}
            </div>
          </div>
        )}

        {accessMode === 'basic' ? (
          <div className="access-panel-card">
            <div className="access-section-header">
              <div className="space-y-1">
                <h4 className="access-section-title">People and basic roles</h4>
              </div>
              <div className="access-section-tools">
                {accessSearchControl}
                <button type="button" className="glass-button-primary access-section-action" onClick={openCreateUserEditor}>
                  <PlusIcon />
                  <span>Add user</span>
                </button>
              </div>
            </div>
            {usersWorkspace}
          </div>
        ) : (
          <div className="access-panel-card">
            <div className="access-section-header">
              <div className="space-y-1">
                <h4 className="access-section-title">{sectionContent.title}</h4>
              </div>
              <div className="access-section-tools">
                {accessSearchControl}
                {activeSection === 'users' && (
                  <button type="button" className="glass-button-primary access-section-action" onClick={openCreateUserEditor}>
                    <PlusIcon />
                    <span>Add user</span>
                  </button>
                )}
                {activeSection === 'service-accounts' && (
                  <button type="button" className="glass-button-primary access-section-action" onClick={openCreateServiceAccountEditor}>
                    <PlusIcon />
                    <span>Add service account</span>
                  </button>
                )}
                {activeSection === 'roles' && (
                  <button type="button" className="glass-button-primary access-section-action" onClick={openCreateRoleEditor}>
                    <PlusIcon />
                    <span>Add role</span>
                  </button>
                )}
                {activeSection === 'policies' && (
                  <button type="button" className="glass-button-primary access-section-action" onClick={openCreatePolicyEditor}>
                    <PlusIcon />
                    <span>Add policy</span>
                  </button>
                )}
              </div>
            </div>

          {activeSection === 'users' && usersWorkspace}

          {activeSection === 'service-accounts' && serviceAccountsWorkspace}

          {activeSection === 'roles' && (
            <div className="access-workspace">
              <div className="space-y-4 access-workspace__list">
                {(policiesError || error) && <div className="access-error-banner">Failed to load roles: {policiesError || error}</div>}
                {loading || policiesLoading ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">Loading roles…</p>
                    <p className="text-sm text-[var(--text-secondary)]">Collecting reusable bundles and their current assignees.</p>
                  </div>
                ) : roleDefinitions.length === 0 ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">No roles yet</p>
                    <p className="text-sm text-[var(--text-secondary)]">Create a role and attach policies that match the language your operators already use.</p>
                  </div>
                ) : filteredRoleDefinitions.length === 0 ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">No roles match this search</p>
                    <p className="text-sm text-[var(--text-secondary)]">Try a role name, policy label, or one of the assigned accounts.</p>
                  </div>
                ) : (
                  <div className="access-entity-grid access-entity-grid--roles">
                    {filteredRoleDefinitions.map(role => {
                      const assignedUsers = roleUserMap.get(role.id) || [];
                      const preset = accessPresetForRole(role.role);
                      const coverage = summarizeRoleCoverage(role.policies);
                      const protectedRole = isProtectedAccessRole(role.role);
                      const isSelected = roleEditor?.role === role.role;
                      return (
                        <article key={role.id} className={`access-card access-card--role ${isSelected ? 'access-card--selected' : ''}`}>
                          <div className="access-card__header">
                            <div className="space-y-2 min-w-0">
                              <div className="flex items-center gap-2 flex-wrap">
                                <p className="access-card__title">{role.role}</p>
                                {preset && <span className={`access-chip ${accessPresetToneClass(role.role)}`}>{preset.label}</span>}
                                {protectedRole && <span className="access-chip access-chip--muted">Protected</span>}
                              </div>
                              <p className="access-card__subtitle">
                                {preset?.description || 'Reusable role bundle for assigning multiple low-level AAA policies together.'}
                              </p>
                              <p className="access-card__meta-line">
                                {formatAccessCount(role.policies.length, 'policy', 'policies')} · {formatAccessCount(assignedUsers.length, 'assignee')}
                              </p>
                            </div>
                            <div className="access-card__actions">
                              {protectedRole ? (
                                <span className="access-chip access-chip--muted">Protected</span>
                              ) : (
                                <>
                                  <button
                                    type="button"
                                    className="access-card-action"
                                    title="Edit role"
                                    aria-label={`Edit ${role.role}`}
                                    onClick={() => openEditRoleEditor(role)}
                                  >
                                    <EditIcon />
                                  </button>
                                  <button
                                    type="button"
                                    className="access-card-action access-card-action--danger"
                                    title="Delete role"
                                    aria-label={`Delete ${role.role}`}
                                    onClick={() => confirmDeleteRoleDefinition(role)}
                                  >
                                    <TrashIcon />
                                  </button>
                                </>
                              )}
                            </div>
                          </div>
                          <div className="space-y-2">
                            <p className="access-card__label">Coverage</p>
                            <div className="flex flex-wrap gap-2">
                              {coverage.map(label => (
                                <span key={`${role.id}-coverage-${label}`} className="access-chip access-chip--muted">
                                  {label}
                                </span>
                              ))}
                              {coverage.length === 0 && <span className="text-sm text-[var(--text-secondary)]">No coverage yet</span>}
                            </div>
                          </div>
                          <div className="space-y-2">
                            <p className="access-card__label">Includes</p>
                            <div className="flex flex-wrap gap-2">
                              {role.policies.slice(0, 4).map(policy => (
                                <span key={policyKey(policy)} className="access-chip access-chip--muted">
                                  {policyLabel(policy)}
                                </span>
                              ))}
                              {role.policies.length > 4 && <span className="access-chip access-chip--muted">+ {role.policies.length - 4} more</span>}
                            </div>
                          </div>
                        </article>
                      );
                    })}
                  </div>
                )}
              </div>
              <aside className="access-editor-pane">
                {roleEditor ? (
                  <div className="access-editor-surface access-editor-surface--minimal">
                    <div className="access-editor-header">
                      <div>
                        <p className="access-editor-kicker">{roleEditor.mode === 'create' ? 'Create role' : 'Edit role'}</p>
                        <h5 className="access-editor-title">{roleEditor.mode === 'create' ? 'Role definition' : roleEditor.role}</h5>
                        <p className="access-editor-text">Assign reusable policies.</p>
                      </div>
                      <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={() => setRoleEditor(null)}>
                        Close
                      </button>
                    </div>
                    <form className="access-editor-form access-editor-form--compact" onSubmit={handleSaveRoleEditor}>
                      <label className="access-minimal-label">
                        <span>Role name</span>
                        <input
                          className="pipelines-input"
                          value={roleEditor.role}
                          onChange={e => setRoleEditor(prev => (prev ? { ...prev, role: e.target.value } : prev))}
                          placeholder="developer"
                          required
                          disabled={roleEditor.mode === 'edit'}
                        />
                      </label>
                      <div className="access-editor-section access-editor-section--plain">
                        <div className="access-minimal-section__header">
                          <p className="text-sm font-medium text-[var(--text-primary)]">Policies</p>
                          <span className="text-[11px] text-[var(--text-secondary)]">{formatAccessCount(roleEditor.policies.length, 'policy', 'policies')}</span>
                        </div>
                        <div className="space-y-2">
                          {roleEditor.policies.map((policy, idx) => (
                            <div key={`policy-${idx}`} className="access-minimal-row">
                              <div className="flex-1 space-y-1">
                                <p className="font-semibold truncate">{policyLabel(policy)}</p>
                                <p className="text-[11px] text-[var(--text-secondary)] truncate">{policy.obj || 'Select an object'}</p>
                                <p className="text-[11px] text-[var(--text-secondary)] truncate">{policy.act || 'Select an action'}</p>
                              </div>
                              <button
                                type="button"
                                className="access-inline-btn access-inline-btn--danger"
                                onClick={() => removeRolePolicyDraft(idx)}
                                title="Remove policy"
                              >
                                <TrashIcon />
                              </button>
                            </div>
                          ))}
                          <div className="access-editor-inline-add">
                            {availablePoliciesForRoleEditor.length > 0 ? (
                              <>
                                <select className="pipelines-input w-full" value={nextPolicyKey} onChange={e => setNextPolicyKey(e.target.value)}>
                                  <option value="" disabled>
                                    Add policy…
                                  </option>
                                  {availablePoliciesForRoleEditor.map(item => (
                                    <option key={item.key} value={item.key}>
                                      {item.label}
                                    </option>
                                  ))}
                                </select>
                                <button
                                  type="button"
                                  className="glass-button-subtle"
                                  onClick={() => {
                                    if (nextPolicyKey) {
                                      addExistingPolicyDraft(nextPolicyKey);
                                      setNextPolicyKey('');
                                    }
                                  }}
                                  disabled={!nextPolicyKey}
                                >
                                  Add
                                </button>
                              </>
                            ) : (
                              <p className="text-sm text-[var(--text-secondary)]">No reusable policies available</p>
                            )}
                          </div>
                        </div>
                      </div>
                      <div className="access-editor-footer access-editor-footer--inline">
                        <button type="submit" className="glass-button-primary" disabled={savingRoleEditor}>
                          {savingRoleEditor ? 'Saving…' : 'Save role'}
                        </button>
                      </div>
                    </form>
                  </div>
                ) : (
                  <AccessEditorEmptyState sectionLabel="Role details" hint="Select a role to edit policies." />
                )}
              </aside>
            </div>
          )}

          {activeSection === 'policies' && (
            <div className="access-workspace">
              <div className="space-y-4 access-workspace__list">
                {policiesError && <div className="access-error-banner">Failed to load policies: {policiesError}</div>}
                {policiesLoading ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">Loading policies…</p>
                    <p className="text-sm text-[var(--text-secondary)]">Fetching the low-level rules behind each role bundle.</p>
                  </div>
                ) : visiblePolicies.length === 0 ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">No policies yet</p>
                    <p className="text-sm text-[var(--text-secondary)]">Create a rule, then attach it to roles like viewer or developer.</p>
                  </div>
                ) : filteredPolicies.length === 0 ? (
                  <div className="access-empty-card">
                    <p className="font-medium text-[var(--text-primary)]">No policies match this search</p>
                    <p className="text-sm text-[var(--text-secondary)]">Search by role, resource selector, action, or policy label.</p>
                  </div>
                ) : (
                  <div className="access-policy-stack">
                    {filteredPolicies.map(policy => {
                      const protectedPolicy = isProtectedAccessRole(policy.role);
                      const parsedAction = parseAAAActionValue(policy.act);
                      const preset = accessPresetForRole(policy.role);
                      const isSelected =
                        policyEditor?.original.role === policy.role &&
                        policyEditor?.original.obj === policy.obj &&
                        policyEditor?.original.act === policy.act;
                      return (
                        <article key={`${policy.role}-${policy.obj}-${policy.act}`} className={`access-card access-card--policy ${isSelected ? 'access-card--selected' : ''}`}>
                          <div className="access-card__header">
                            <div className="space-y-2 min-w-0">
                              <div className="flex items-center gap-2 flex-wrap">
                                <p className="access-card__title">{policyLabel(policy)}</p>
                                <span className={`access-chip ${accessPresetToneClass(policy.role)}`}>{policy.role}</span>
                                {parsedAction.effect === 'deny' && <span className="access-chip access-chip--danger">Deny</span>}
                                {protectedPolicy && <span className="access-chip access-chip--muted">Protected</span>}
                              </div>
                              <p className="access-card__subtitle">
                                {preset ? `${preset.label} role` : 'Role'} can {formatAccessActionSummary(policy.act)} on {formatAccessResourceSummary(policy.obj)}.
                              </p>
                            </div>
                            <div className="access-card__actions">
                              {protectedPolicy ? (
                                <span className="access-chip access-chip--muted">Protected</span>
                              ) : (
                                <>
                                  <button
                                    type="button"
                                    className="access-card-action"
                                    title="Edit policy"
                                    aria-label={`Edit ${policyLabel(policy)}`}
                                    onClick={() => openPolicyEditModal(policy)}
                                  >
                                    <EditIcon />
                                  </button>
                                  <button
                                    type="button"
                                    className="access-card-action access-card-action--danger"
                                    title="Delete policy"
                                    aria-label={`Delete ${policyLabel(policy)}`}
                                    onClick={() => confirmDeletePolicy(policy)}
                                  >
                                    <TrashIcon />
                                  </button>
                                </>
                              )}
                            </div>
                          </div>
                          <div className="access-policy-preview access-policy-preview--minimal">
                            <span className="access-policy-chip access-policy-chip--path">{policy.obj}</span>
                            <span className="access-policy-arrow">-&gt;</span>
                            <span className="access-policy-chip access-policy-chip--act">{policy.act}</span>
                          </div>
                        </article>
                      );
                    })}
                  </div>
                )}
              </div>
              <aside className="access-editor-pane">
                {policyEditor ? (
                  <div className="access-editor-surface access-editor-surface--minimal">
                    <div className="access-editor-header">
                      <div>
                        <p className="access-editor-kicker">Edit policy</p>
                        <h5 className="access-editor-title">{policyEditor.name || policyLabel(policyEditor)}</h5>
                        <p className="access-editor-text">Update this reusable rule.</p>
                      </div>
                      <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={() => setPolicyEditor(null)}>
                        Close
                      </button>
                    </div>
                    <form className="access-editor-form access-editor-form--compact" onSubmit={handleSavePolicyEdit}>
                      <AccessPolicyRuleFields
                        policy={policyEditor}
                        onChange={next => setPolicyEditor(prev => (prev ? { ...prev, ...next } : prev))}
                        resourceCatalog={resourceCatalog}
                      />
                      <div className="access-editor-footer access-editor-footer--inline">
                        <button type="submit" className="glass-button-primary" disabled={savingPolicy}>
                          {savingPolicy ? 'Saving…' : 'Save changes'}
                        </button>
                      </div>
                    </form>
                  </div>
                ) : showPolicyModal ? (
                  <div className="access-editor-surface access-editor-surface--minimal">
                    <div className="access-editor-header">
                      <div>
                        <p className="access-editor-kicker">Create policy</p>
                        <h5 className="access-editor-title">Reusable AAA rule</h5>
                        <p className="access-editor-text">Define a reusable rule.</p>
                      </div>
                      <button
                        type="button"
                        className="access-inline-btn access-inline-btn--pill"
                        onClick={() => {
                          setAwaitingPolicyCreateReset(false);
                          setShowPolicyModal(false);
                        }}
                      >
                        Close
                      </button>
                    </div>
                    <form className="access-editor-form access-editor-form--compact" onSubmit={handleCreatePolicyInline}>
                      <AccessPolicyRuleFields policy={newPermission} onChange={onChangePermission} resourceCatalog={resourceCatalog} />
                      <div className="access-editor-footer access-editor-footer--inline">
                        <button type="submit" className="glass-button-primary" disabled={creatingPolicyInline}>
                          {creatingPolicyInline ? 'Adding…' : 'Add policy'}
                        </button>
                      </div>
                    </form>
                  </div>
                ) : (
                  <AccessEditorEmptyState sectionLabel="Policy details" hint="Select a policy to edit rules." />
                )}
              </aside>
            </div>
          )}
          </div>
        )}
      </div>

      {confirmDialog && (
        <AccessModal
          kicker="Confirm"
          title="Please confirm"
          subtitle="This action cannot be undone."
          onClose={() => setConfirmDialog(null)}
          icon={<TrashIcon />}
          variant="minimal"
        >
          <div className="space-y-4">
            <p className="text-sm text-[var(--text-primary)]">{confirmDialog.message}</p>
            <div className="flex items-center justify-end gap-2">
              <button data-dialog-initial-focus type="button" className="access-inline-btn" onClick={() => setConfirmDialog(null)} disabled={confirming}>
                Cancel
              </button>
              <button type="button" className="glass-button-danger" onClick={() => void handleConfirmDialog()} disabled={confirming}>
                {confirming ? 'Working…' : 'Delete'}
              </button>
            </div>
          </div>
        </AccessModal>
      )}
    </div>
  );
}

function PlusIcon() {
  return <Plus className="h-4 w-4" strokeWidth={2} aria-hidden="true" />;
}

function TrashIcon() {
  return <Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />;
}

function EditIcon() {
  return <Edit3 className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />;
}

function RefreshIcon() {
  return <RefreshCw className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />;
}

function SearchIcon() {
  return <Search className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />;
}

export default AccessPanel;
