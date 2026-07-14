import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { useLocation } from "react-router-dom";
import {
  DEFAULT_ADMIN_ROLE,
  POLICY_TEMPLATE_ROLE,
  ROOT_ACCESS_SCOPE,
  accessGrantMatchesServiceAccount,
  accessGrantMatchesUser,
  accessGrantSortLabel,
  assignmentKey,
  basicAccessGrantLabel,
  emptyIdentityProviderForm,
  editableAccessGrantFromRecord,
  identityProviderFormFromRecord,
  isBasicAccessGrant,
  isDefaultAdminUser,
  isExternallyManagedUser,
  isProtectedAccessRole,
  isUserRoleManagementLocked,
  normalizeEditableBasicGrants,
  policyKey,
  policyLabel,
  userDisplayName,
} from "./model";
import {
  areBasicGrantEntriesDirty,
  buildBasicGrantChangeSet,
  stageBasicGrant,
} from "./basicGrantModel";
import {
  formatAccessActionSummary,
  formatAccessResourceSummary,
} from "./policyRuleModel";
import type { AccessResourceCatalog } from "./resourceCatalog";
import type {
  AccessGrantRecord,
  BasicGrantInput,
  EditableAccessGrant,
  IdentityProviderFormState,
  IdentityProviderRecord,
  IdentityProviderSettings,
  RoleDefinition,
  RolePermission,
  RolePolicyDraft,
  ServiceAccountSummary,
  ServiceAccountToken,
  UserSummary,
} from "./model";
import {
  ACCESS_SECTION_CONTENT,
  accessPresetForRole,
  matchesAccessSearch,
} from "./presentation";
import { copyTextToClipboard } from "../../../lib/clipboard";
import type {
  AccessMode,
  AccessSection,
  PolicyEditorState,
  RoleEditorState,
  ServiceAccountEditorState,
  UserAccessEditorState,
} from "./panelTypes";

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
  identityProviders: IdentityProviderRecord[];
  identityProviderSettings: IdentityProviderSettings;
  identityProviderDomainMappings: Record<string, string>;
  identityProvidersLoading: boolean;
  identityProvidersError: string | null;
  policies: RolePermission[];
  policiesLoading: boolean;
  policiesError: string | null;
  resourceCatalog: AccessResourceCatalog;
  newUser: { sub: string; email: string; password: string; roles: string[] };
  newServiceAccount: {
    sub: string;
    email: string;
    tokenName: string;
    roles: string[];
  };
  policyTemplates: RolePermission[];
  onChangeUser: (next: {
    sub: string;
    email: string;
    password: string;
    roles: string[];
  }) => void;
  onCreateUser: (
    e: FormEvent<HTMLFormElement>,
    options?: { basicGrants?: BasicGrantInput[] },
  ) => Promise<boolean>;
  onChangeServiceAccount: (next: {
    sub: string;
    email: string;
    tokenName: string;
    roles: string[];
  }) => void;
  onCreateServiceAccount: (
    e: FormEvent<HTMLFormElement>,
    options?: { basicGrants?: BasicGrantInput[] },
  ) => Promise<ServiceAccountToken | null>;
  onReloadUsers: () => void;
  onReloadServiceAccounts: () => void;
  onCreatePermission: (e: FormEvent<HTMLFormElement>) => Promise<void>;
  newPermission: { name: string; obj: string; act: string };
  onChangePermission: (next: {
    name: string;
    obj: string;
    act: string;
  }) => void;
  onDeleteUser: (userId: string) => Promise<void>;
  onDeleteServiceAccount: (serviceAccountId: string) => Promise<void>;
  onDeletePolicy: (policy: RolePermission) => Promise<void>;
  onDeleteRoleDefinition: (role: RoleDefinition) => Promise<void>;
  onSaveRoleDefinition: (input: {
    role: string;
    policies: RolePolicyDraft[];
    original?: RolePermission[];
  }) => Promise<void>;
  onEditPolicy: (
    current: RolePermission,
    next: { role: string; name: string; obj: string; act: string },
  ) => Promise<void>;
  onUpdateUserRoles: (
    userId: string,
    nextRoles: string[],
    previousRoles: string[],
  ) => Promise<void>;
  onUpdateServiceAccountRoles: (
    serviceAccountId: string,
    nextRoles: string[],
    previousRoles: string[],
  ) => Promise<void>;
  onCreateAccessGrant: (input: {
    userID: string;
    role: string;
    resourceType: string;
    resourceID: string;
    inherit?: boolean;
  }) => Promise<void>;
  onCreateServiceAccountAccessGrant: (input: {
    serviceAccountSub: string;
    role: string;
    resourceType: string;
    resourceID: string;
    inherit?: boolean;
  }) => Promise<void>;
  onDeleteAccessGrant: (grantID: string) => Promise<void>;
  onReloadAccessGrants: () => void;
  onReloadIdentityProviders: () => void;
  onSaveIdentityProviderSettings: (
    settings: IdentityProviderSettings,
    mappings: Record<string, string>,
  ) => Promise<void>;
  onSaveIdentityProvider: (form: IdentityProviderFormState) => Promise<void>;
  onDeleteIdentityProvider: (providerID: string) => Promise<void>;
  onReloadPolicies: () => void;
  onUpdateUser: (
    userId: string,
    input: { email?: string; status?: string; password?: string },
  ) => Promise<void>;
  onUpdateServiceAccount: (
    serviceAccountId: string,
    input: { email?: string; status?: string },
  ) => Promise<void>;
  onLoadServiceAccountTokens: (
    serviceAccountId: string,
  ) => Promise<ServiceAccountToken[]>;
  onCreateServiceAccountToken: (
    serviceAccountId: string,
    name: string,
  ) => Promise<ServiceAccountToken>;
  onRevokeServiceAccountToken: (
    serviceAccountId: string,
    tokenId: string,
  ) => Promise<void>;
};

function accessResourceSearchFromQuery(search: string) {
  const params = new URLSearchParams(search);
  const resourceType = (
    params.get("resource_type") ||
    params.get("resourceType") ||
    ""
  ).trim();
  const resourceID = (
    params.get("resource_id") ||
    params.get("resourceID") ||
    ""
  ).trim().replace(/^\/+|\/+$/g, "");
  if (!resourceType || !resourceID) return "";
  return resourceID;
}

export function useAccessPanelController({
  users,
  loading,
  error,
  serviceAccounts,
  serviceAccountsLoading,
  serviceAccountsError,
  accessGrants,
  accessGrantsLoading,
  accessGrantsError,
  identityProviders,
  identityProviderSettings,
  identityProviderDomainMappings,
  identityProvidersLoading,
  identityProvidersError,
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
  onReloadIdentityProviders,
  onSaveIdentityProviderSettings,
  onSaveIdentityProvider,
  onDeleteIdentityProvider,
  onReloadPolicies,
  onUpdateUser,
  onUpdateServiceAccount,
  onLoadServiceAccountTokens,
  onCreateServiceAccountToken,
  onRevokeServiceAccountToken,
}: AccessPanelProps) {
  const location = useLocation();
  const resourceSearchQuery = useMemo(
    () => accessResourceSearchFromQuery(location.search),
    [location.search],
  );
  const [accessMode, setAccessMode] = useState<AccessMode>("basic");
  const [activeSection, setActiveSection] = useState<AccessSection>("users");
  const [showUserModal, setShowUserModal] = useState(false);
  const [showServiceAccountModal, setShowServiceAccountModal] = useState(false);
  const [showPolicyModal, setShowPolicyModal] = useState(false);
  const [roleEditor, setRoleEditor] = useState<RoleEditorState | null>(null);
  const [policyEditor, setPolicyEditor] = useState<PolicyEditorState | null>(
    null,
  );
  const [confirmDialog, setConfirmDialog] = useState<{
    message: string;
    onConfirm: () => Promise<void> | void;
  } | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [userAccessEditor, setUserAccessEditor] =
    useState<UserAccessEditorState | null>(null);
  const [serviceAccountEditor, setServiceAccountEditor] =
    useState<ServiceAccountEditorState | null>(null);
  const [createdServiceAccountToken, setCreatedServiceAccountToken] =
    useState<ServiceAccountToken | null>(null);
  const [copyServiceAccountTokenLabel, setCopyServiceAccountTokenLabel] =
    useState("Copy");
  const [savingRoleEditor, setSavingRoleEditor] = useState(false);
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [savingUserAccess, setSavingUserAccess] = useState(false);
  const [savingServiceAccountAccess, setSavingServiceAccountAccess] =
    useState(false);
  const [creatingUserInline, setCreatingUserInline] = useState(false);
  const [creatingServiceAccountInline, setCreatingServiceAccountInline] =
    useState(false);
  const [creatingPolicyInline, setCreatingPolicyInline] = useState(false);
  const [awaitingUserCreateReset, setAwaitingUserCreateReset] = useState(false);
  const [
    awaitingServiceAccountCreateReset,
    setAwaitingServiceAccountCreateReset,
  ] = useState(false);
  const [awaitingPolicyCreateReset, setAwaitingPolicyCreateReset] =
    useState(false);
  const [basicGrantDraft, setBasicGrantDraft] = useState({
    role: "",
    scope: ROOT_ACCESS_SCOPE,
  });
  const [basicGrantEntries, setBasicGrantEntries] = useState<
    EditableAccessGrant[]
  >([]);
  const [basicGrantSaving, setBasicGrantSaving] = useState(false);
  const [basicGrantError, setBasicGrantError] = useState<string | null>(null);
  const [selectedIdentityProviderID, setSelectedIdentityProviderID] =
    useState("");
  const [identityProviderForm, setIdentityProviderForm] =
    useState<IdentityProviderFormState>(emptyIdentityProviderForm);
  const [identityProviderSettingsDraft, setIdentityProviderSettingsDraft] =
    useState<IdentityProviderSettings>(identityProviderSettings);
  const [
    identityProviderDomainMappingDraft,
    setIdentityProviderDomainMappingDraft,
  ] = useState("");
  const [savingIdentityProvider, setSavingIdentityProvider] = useState(false);
  const [savingIdentityProviderSettings, setSavingIdentityProviderSettings] =
    useState(false);
  const policyOptions = useMemo(() => {
    const seen = new Set<string>();
    return policyTemplates
      .filter((policy) => {
        const key = `${policy.obj}::${policy.act}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      })
      .map((policy) => ({
        key: `${policy.obj}::${policy.act}`,
        obj: policy.obj,
        act: policy.act,
        name: policy.name,
        label: policyLabel(policy),
      }))
      .sort((a, b) => a.label.localeCompare(b.label));
  }, [policyTemplates]);

  const roleAssignments = useMemo(
    () =>
      [
        ...users.flatMap((user) =>
          (user.roles || []).map((role) => ({
            id: `${user.id}-${role.role}`,
            role: role.role,
            user: userDisplayName(user),
            userId: user.id,
            email: user.email,
            status: user.status,
            kind: "user",
          })),
        ),
        ...serviceAccounts.flatMap((account) =>
          (account.roles || []).map((role) => ({
            id: `${account.id}-${role.role}`,
            role: role.role,
            user: account.sub,
            userId: account.id,
            email: account.email,
            status: account.status,
            kind: "service-account",
          })),
        ),
      ].sort(
        (a, b) => a.role.localeCompare(b.role) || a.user.localeCompare(b.user),
      ),
    [serviceAccounts, users],
  );

  const roleDefinitions = useMemo(() => {
    const map = new Map<string, RoleDefinition>();
    policies.forEach((policy) => {
      if (!map.has(policy.role)) {
        map.set(policy.role, {
          id: policy.role,
          role: policy.role,
          policies: [],
        });
      }
      map.get(policy.role)?.policies.push(policy);
    });
    return Array.from(map.values()).sort((a, b) =>
      a.role.localeCompare(b.role),
    );
  }, [policies]);

  const allRoleOptions = useMemo(() => {
    const set = new Set<string>();
    roleDefinitions.forEach((role) => set.add(role.role));
    set.add(DEFAULT_ADMIN_ROLE);
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [roleDefinitions]);

  const roleUserMap = useMemo(() => {
    const map = new Map<
      string,
      { user: string; userId: string; email: string; kind: string }[]
    >();
    roleAssignments.forEach((item) => {
      const existing = map.get(item.role) || [];
      existing.push({
        user: item.user,
        userId: item.userId,
        email: item.email,
        kind: item.kind,
      });
      map.set(item.role, existing);
    });
    return map;
  }, [roleAssignments]);

  const tabItems = [
    { id: "users", label: "Users", count: users.length },
    {
      id: "service-accounts",
      label: "Service accounts",
      count: serviceAccounts.length,
    },
    { id: "roles", label: "Roles", count: roleDefinitions.length },
    {
      id: "identity-providers",
      label: "Identity Providers",
      count: identityProviders.length,
    },
    { id: "policies", label: "Policies", count: policyTemplates.length },
  ] as const;

  const openCreateRoleEditor = useCallback(() => {
    setNextPolicyKey("");
    setRoleEditor({
      mode: "create",
      role: "",
      policies: [],
    });
  }, []);

  const openCreateUserEditor = useCallback(() => {
    setNextUserRole("");
    setAwaitingUserCreateReset(false);
    setUserAccessEditor(null);
    setServiceAccountEditor(null);
    setBasicGrantError(null);
    setBasicGrantDraft({ role: "", scope: ROOT_ACCESS_SCOPE });
    setBasicGrantEntries([]);
    setShowServiceAccountModal(false);
    setShowUserModal(true);
  }, []);

  const openCreateServiceAccountEditor = useCallback(() => {
    setNextUserRole("");
    setAwaitingServiceAccountCreateReset(false);
    setUserAccessEditor(null);
    setServiceAccountEditor(null);
    setCreatedServiceAccountToken(null);
    setBasicGrantError(null);
    setBasicGrantDraft({ role: "", scope: ROOT_ACCESS_SCOPE });
    setBasicGrantEntries([]);
    setShowUserModal(false);
    setShowServiceAccountModal(true);
  }, []);

  const openCreatePolicyEditor = useCallback(() => {
    setAwaitingPolicyCreateReset(false);
    setPolicyEditor(null);
    setShowPolicyModal(true);
  }, []);

  const openCreateIdentityProvider = useCallback(() => {
    setSelectedIdentityProviderID("");
    setIdentityProviderForm(emptyIdentityProviderForm());
  }, []);

  const openEditIdentityProvider = useCallback(
    (provider: IdentityProviderRecord) => {
      setSelectedIdentityProviderID(provider.id);
      setIdentityProviderForm(identityProviderFormFromRecord(provider));
    },
    [],
  );

  const openEditRoleEditor = (role: RoleDefinition) => {
    if (isProtectedAccessRole(role.role)) return;
    setShowPolicyModal(false);
    setRoleEditor({
      mode: "edit",
      role: role.role,
      policies: role.policies.map((p) => ({
        name: p.name || policyLabel(p),
        obj: p.obj,
        act: p.act,
      })),
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
    setBasicGrantDraft({ role: "", scope: ROOT_ACCESS_SCOPE });
    setBasicGrantEntries(
      (basicUserGrantMap.get(user.id) || []).map(editableAccessGrantFromRecord),
    );
    const entries = (user.roles || []).map((role) => role.role);
    const nextEntries = entries.length > 0 ? entries : [];
    setUserAccessEditor({
      user,
      entries: nextEntries,
      original: entries,
      email: user.email || "",
      status: user.status || "active",
      password: "",
    });
  };

  const openServiceAccountAccessModal = (account: ServiceAccountSummary) => {
    setAwaitingServiceAccountCreateReset(false);
    setShowServiceAccountModal(false);
    setShowUserModal(false);
    setUserAccessEditor(null);
    setCreatedServiceAccountToken(null);
    setBasicGrantError(null);
    setBasicGrantDraft({ role: "", scope: ROOT_ACCESS_SCOPE });
    setBasicGrantEntries(
      (basicServiceAccountGrantMap.get(account.sub) || []).map(
        editableAccessGrantFromRecord,
      ),
    );
    const entries = (account.roles || []).map((role) => role.role);
    setServiceAccountEditor({
      account,
      entries,
      original: entries,
      email: account.email || "",
      status: account.status || "active",
      tokenName: "rotation",
      tokens: [],
      tokensLoading: true,
      tokensError: null,
    });
    void onLoadServiceAccountTokens(account.id)
      .then((tokens) => {
        setServiceAccountEditor((prev) =>
          prev?.account.id === account.id
            ? { ...prev, tokens, tokensLoading: false, tokensError: null }
            : prev,
        );
      })
      .catch((error) => {
        setServiceAccountEditor((prev) =>
          prev?.account.id === account.id
            ? {
                ...prev,
                tokensLoading: false,
                tokensError:
                  error instanceof Error
                    ? error.message
                    : "Unable to load tokens",
              }
            : prev,
        );
      });
  };

  const removeRolePolicyDraft = (index: number) => {
    setRoleEditor((prev) => {
      if (!prev) return prev;
      const nextPolicies = prev.policies.filter((_, i) => i !== index);
      return {
        ...prev,
        policies: nextPolicies,
      };
    });
  };

  const addExistingPolicyDraft = (key: string) => {
    setRoleEditor((prev) => {
      if (!prev) return prev;
      const match = policyOptions.find((p) => p.key === key);
      if (!match) return prev;
      const already = prev.policies.some(
        (p) => p.obj === match.obj && p.act === match.act,
      );
      if (already) return prev;
      return {
        ...prev,
        policies: [
          ...prev.policies,
          { name: match.name || match.label, obj: match.obj, act: match.act },
        ],
      };
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
      console.error("Failed to save role", error);
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
      console.error("Failed to update policy", error);
    } finally {
      setSavingPolicy(false);
    }
  };

  const handleSaveUserAccess = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!userAccessEditor) return;
    const deduped = userAccessEditor.entries
      .map((role) => role.trim())
      .filter(Boolean)
      .filter((role, index, roles) => roles.indexOf(role) === index);
    setSavingUserAccess(true);
    try {
      const updatePayload: {
        email?: string;
        status?: string;
        password?: string;
      } = {};
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
      const rolesLocked = isUserRoleManagementLocked(userAccessEditor.user);
      if (!rolesLocked) {
        await onUpdateUserRoles(
          userAccessEditor.user.id,
          deduped,
          userAccessEditor.original,
        );
      }
      if (!rolesLocked && basicGrantDirty) {
        await saveBasicGrantsForUser(userAccessEditor.user);
      }
      setUserAccessEditor(null);
    } catch (error) {
      console.error("Failed to update user access", error);
    } finally {
      setSavingUserAccess(false);
    }
  };

  const handleSaveServiceAccountAccess = async (
    e: FormEvent<HTMLFormElement>,
  ) => {
    e.preventDefault();
    if (!serviceAccountEditor) return;
    const deduped = serviceAccountEditor.entries
      .map((role) => role.trim())
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
      if (
        statusTrimmed &&
        statusTrimmed !== serviceAccountEditor.account.status
      ) {
        updatePayload.status = statusTrimmed;
      }
      if (Object.keys(updatePayload).length > 0) {
        await onUpdateServiceAccount(
          serviceAccountEditor.account.id,
          updatePayload,
        );
      }
      await onUpdateServiceAccountRoles(
        serviceAccountEditor.account.id,
        deduped,
        serviceAccountEditor.original,
      );
      if (basicGrantDirty) {
        await saveBasicGrantsForServiceAccount(serviceAccountEditor.account);
      }
      setServiceAccountEditor(null);
    } catch (error) {
      console.error("Failed to update service account access", error);
    } finally {
      setSavingServiceAccountAccess(false);
    }
  };

  const addUserAccessEntry = () => {
    setUserAccessEditor((prev) => {
      if (!prev) return prev;
      if (isUserRoleManagementLocked(prev.user)) return prev;
      const roleName = nextAccessRole.trim();
      if (!roleName) return prev;
      if (
        prev.entries.some(
          (entry) => assignmentKey(entry) === assignmentKey(roleName),
        )
      )
        return prev;
      return { ...prev, entries: [...prev.entries, roleName] };
    });
    setNextAccessRole("");
  };

  const removeUserAccessEntry = (index: number) => {
    setUserAccessEditor((prev) => {
      if (!prev) return prev;
      if (isUserRoleManagementLocked(prev.user)) return prev;
      const next = prev.entries.filter((_, i) => i !== index);
      return { ...prev, entries: next };
    });
  };

  const addServiceAccountAccessEntry = () => {
    setServiceAccountEditor((prev) => {
      if (!prev) return prev;
      const roleName = nextAccessRole.trim();
      if (!roleName) return prev;
      if (
        prev.entries.some(
          (entry) => assignmentKey(entry) === assignmentKey(roleName),
        )
      )
        return prev;
      return { ...prev, entries: [...prev.entries, roleName] };
    });
    setNextAccessRole("");
  };

  const removeServiceAccountAccessEntry = (index: number) => {
    setServiceAccountEditor((prev) => {
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
    const existing = (newUser.roles || []).some(
      (entry) => entry.trim().toLowerCase() === roleName.toLowerCase(),
    );
    if (existing) {
      setNextUserRole("");
      return;
    }
    onChangeUser({ ...newUser, roles: [...(newUser.roles || []), roleName] });
    setNextUserRole("");
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
    const existing = (newServiceAccount.roles || []).some(
      (entry) => entry.trim().toLowerCase() === roleName.toLowerCase(),
    );
    if (existing) {
      setNextUserRole("");
      return;
    }
    onChangeServiceAccount({
      ...newServiceAccount,
      roles: [...(newServiceAccount.roles || []), roleName],
    });
    setNextUserRole("");
  };

  const visiblePolicies = useMemo(() => {
    const combined = [
      ...policyTemplates,
      ...policies.filter(
        (policy) =>
          policy.role !== POLICY_TEMPLATE_ROLE &&
          !isProtectedAccessRole(policy.role),
      ),
    ];
    const seen = new Set<string>();
    return combined
      .filter((policy) => {
        const key = policyKey(policy);
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      })
      .sort(
        (a, b) =>
          a.role.localeCompare(b.role) ||
          policyLabel(a).localeCompare(policyLabel(b)),
      );
  }, [policies, policyTemplates]);
  const policyCount = visiblePolicies.length;
  const [nextPolicyKey, setNextPolicyKey] = useState("");
  const [nextUserRole, setNextUserRole] = useState("");
  const [nextAccessRole, setNextAccessRole] = useState("");
  const [searchTerm, setSearchTerm] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const searchQuery = searchTerm.trim().toLowerCase();
  const basicAccessGrants = useMemo(
    () => accessGrants.filter(isBasicAccessGrant),
    [accessGrants],
  );
  const basicGrantOptions = useMemo(
    () => [
      { value: ROOT_ACCESS_SCOPE, label: "Global" },
      ...resourceCatalog.teamOptions,
    ],
    [resourceCatalog.teamOptions],
  );
  const basicUserGrantMap = useMemo(() => {
    const map = new Map<string, AccessGrantRecord[]>();
    basicAccessGrants.forEach((grant) => {
      const user = users.find((entry) => accessGrantMatchesUser(grant, entry));
      const key = user?.id || grant.subjectID;
      const entries = map.get(key) || [];
      entries.push(grant);
      map.set(key, entries);
    });
    map.forEach((entries) => {
      entries.sort(
        (a, b) =>
          accessGrantSortLabel(a).localeCompare(
            accessGrantSortLabel(b),
            undefined,
            { sensitivity: "base" },
          ) || a.role.localeCompare(b.role, undefined, { sensitivity: "base" }),
      );
    });
    return map;
  }, [basicAccessGrants, users]);

  const basicServiceAccountGrantMap = useMemo(() => {
    const map = new Map<string, AccessGrantRecord[]>();
    basicAccessGrants.forEach((grant) => {
      const account = serviceAccounts.find((entry) =>
        accessGrantMatchesServiceAccount(grant, entry),
      );
      const key = account?.sub || grant.subjectID;
      if (!key) return;
      const entries = map.get(key) || [];
      entries.push(grant);
      map.set(key, entries);
    });
    map.forEach((entries) => {
      entries.sort(
        (a, b) =>
          accessGrantSortLabel(a).localeCompare(
            accessGrantSortLabel(b),
            undefined,
            { sensitivity: "base" },
          ) || a.role.localeCompare(b.role, undefined, { sensitivity: "base" }),
      );
    });
    return map;
  }, [basicAccessGrants, serviceAccounts]);

  const sectionContent = ACCESS_SECTION_CONTENT[activeSection];
  const filteredUsers = useMemo(() => {
    if (!searchQuery) return users;
    return users.filter((user) => {
      const grants = basicUserGrantMap.get(user.id) || [];
      return matchesAccessSearch(
        searchQuery,
        userDisplayName(user),
        user.sub,
        user.email,
        user.status,
        user.external_provider_name,
        user.external_subject,
        (user.external_teams || []).join(" "),
        (user.external_auth_teams || [])
          .map((team) => `${team.name} ${team.id}`)
          .join(" "),
        (user.roles || []).map((role) => role.role).join(" "),
        grants.map((grant) => basicAccessGrantLabel(grant)).join(" "),
      );
    });
  }, [basicUserGrantMap, searchQuery, users]);

  const filteredServiceAccounts = useMemo(() => {
    if (!searchQuery) return serviceAccounts;
    return serviceAccounts.filter((account) => {
      const grants = basicServiceAccountGrantMap.get(account.sub) || [];
      return matchesAccessSearch(
        searchQuery,
        account.sub,
        account.email,
        account.status,
        String(account.token_count || 0),
        (account.roles || []).map((role) => role.role).join(" "),
        grants.map((grant) => basicAccessGrantLabel(grant)).join(" "),
      );
    });
  }, [basicServiceAccountGrantMap, searchQuery, serviceAccounts]);

  const filteredRoleDefinitions = useMemo(() => {
    if (!searchQuery) return roleDefinitions;
    return roleDefinitions.filter((role) => {
      const assignedUsers = roleUserMap.get(role.id) || [];
      const preset = accessPresetForRole(role.role);
      return matchesAccessSearch(
        searchQuery,
        role.role,
        preset?.label,
        preset?.description,
        role.policies
          .map((policy) => `${policyLabel(policy)} ${policy.obj} ${policy.act}`)
          .join(" "),
        assignedUsers.map((item) => `${item.user} ${item.email}`).join(" "),
      );
    });
  }, [roleDefinitions, roleUserMap, searchQuery]);

  const filteredIdentityProviders = useMemo(() => {
    if (!searchQuery) return identityProviders;
    return identityProviders.filter((provider) =>
      matchesAccessSearch(
        searchQuery,
        provider.id,
        provider.display_name,
        provider.type,
        provider.issuer,
        provider.client_id,
        (provider.allowed_email_domains || []).join(" "),
        Object.entries(provider.role_mapping || {})
          .map(([team, role]) => `${team} ${role}`)
          .join(" "),
        Object.entries(provider.basic_role_mapping || {})
          .map(([team, grant]) => `${team} ${grant.role} ${grant.resource || `${grant.resource_type || ""}:${grant.resource_id || ""}`}`)
          .join(" "),
      ),
    );
  }, [identityProviders, searchQuery]);

  const filteredPolicies = useMemo(() => {
    if (!searchQuery) return visiblePolicies;
    return visiblePolicies.filter((policy) =>
      matchesAccessSearch(
        searchQuery,
        policy.role,
        policy.name,
        policy.obj,
        policy.act,
        policyLabel(policy),
        formatAccessResourceSummary(policy.obj),
        formatAccessActionSummary(policy.act),
      ),
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
    (newServiceAccount.tokenName.trim() === "" ||
      newServiceAccount.tokenName.trim() === "default") &&
    (newServiceAccount.roles || []).length === 0 &&
    basicGrantEntries.length === 0 &&
    !createdServiceAccountToken;
  const isNewPolicyPristine =
    !newPermission.name.trim() &&
    newPermission.obj.trim() === "pipeline:*" &&
    newPermission.act.trim() === "pipeline.read";
  const selectedBasicUserID = userAccessEditor?.user.id || "";
  const selectedBasicUser = userAccessEditor?.user ?? null;
  const selectedBasicServiceAccountSub =
    serviceAccountEditor?.account.sub || "";
  const selectedBasicServiceAccount = serviceAccountEditor?.account ?? null;
  const selectedIdentityProvider = useMemo(
    () =>
      identityProviders.find(
        (provider) => provider.id === selectedIdentityProviderID,
      ) || null,
    [identityProviders, selectedIdentityProviderID],
  );
  const userRoleAssignmentsLocked = isUserRoleManagementLocked(
    userAccessEditor?.user,
  );
  const userRoleAssignmentsLockLabel = isExternallyManagedUser(
    userAccessEditor?.user,
  )
    ? "Managed by identity provider"
    : "Protected admin role assignment";
  const selectedBasicUserGrants = useMemo(
    () =>
      selectedBasicUserID
        ? basicUserGrantMap.get(selectedBasicUserID) || []
        : [],
    [basicUserGrantMap, selectedBasicUserID],
  );
  const selectedBasicServiceAccountGrants = useMemo(
    () =>
      selectedBasicServiceAccountSub
        ? basicServiceAccountGrantMap.get(selectedBasicServiceAccountSub) || []
        : [],
    [basicServiceAccountGrantMap, selectedBasicServiceAccountSub],
  );
  const selectedBasicGrants = selectedBasicServiceAccount
    ? selectedBasicServiceAccountGrants
    : selectedBasicUserGrants;
  const basicGrantDirty = useMemo(
    () => areBasicGrantEntriesDirty(selectedBasicGrants, basicGrantEntries),
    [basicGrantEntries, selectedBasicGrants],
  );

  useEffect(() => {
    setNextAccessRole("");
  }, [userAccessEditor]);

  useEffect(() => {
    setIdentityProviderSettingsDraft(identityProviderSettings);
  }, [identityProviderSettings]);

  useEffect(() => {
    setIdentityProviderDomainMappingDraft(
      Object.entries(identityProviderDomainMappings)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([domain, providerID]) => `${domain}: ${providerID}`)
        .join("\n"),
    );
  }, [identityProviderDomainMappings]);

  useEffect(() => {
    setBasicGrantEntries(
      selectedBasicUserGrants.map(editableAccessGrantFromRecord),
    );
  }, [selectedBasicUserGrants]);

  useEffect(() => {
    if (!resourceSearchQuery) {
      setSearchTerm("");
      setSearchOpen(false);
    }
    setBasicGrantError(null);
    setUserAccessEditor(null);
    setBasicGrantEntries([]);
  }, [accessMode, resourceSearchQuery]);

  useEffect(() => {
    if (!resourceSearchQuery) return;
    setAccessMode("basic");
    setSearchTerm(resourceSearchQuery);
    setSearchOpen(true);
    setBasicGrantError(null);
    setUserAccessEditor(null);
    setServiceAccountEditor(null);
  }, [resourceSearchQuery]);

  useEffect(() => {
    setBasicGrantError(null);
    setBasicGrantDraft({ role: "", scope: ROOT_ACCESS_SCOPE });
  }, [userAccessEditor?.user.id]);

  useEffect(() => {
    if (!resourceSearchQuery) {
      setSearchTerm("");
      setSearchOpen(false);
    }
    setShowUserModal(false);
    setShowPolicyModal(false);
    setRoleEditor(null);
    setPolicyEditor(null);
    setUserAccessEditor(null);
    setAwaitingUserCreateReset(false);
    setAwaitingPolicyCreateReset(false);
    if (activeSection === "identity-providers") {
      setIdentityProviderForm(emptyIdentityProviderForm());
      setSelectedIdentityProviderID("");
    }
  }, [activeSection, resourceSearchQuery]);

  useEffect(() => {
    if (!showUserModal || !awaitingUserCreateReset || !isNewUserPristine)
      return;
    setShowUserModal(false);
    setAwaitingUserCreateReset(false);
  }, [awaitingUserCreateReset, isNewUserPristine, showUserModal]);

  useEffect(() => {
    if (
      !showServiceAccountModal ||
      !awaitingServiceAccountCreateReset ||
      !isNewServiceAccountPristine
    )
      return;
    setShowServiceAccountModal(false);
    setAwaitingServiceAccountCreateReset(false);
  }, [
    awaitingServiceAccountCreateReset,
    isNewServiceAccountPristine,
    showServiceAccountModal,
  ]);

  useEffect(() => {
    if (!showPolicyModal || !awaitingPolicyCreateReset || !isNewPolicyPristine)
      return;
    setShowPolicyModal(false);
    setAwaitingPolicyCreateReset(false);
  }, [awaitingPolicyCreateReset, isNewPolicyPristine, showPolicyModal]);

  const handleCreateUserInline = async (e: FormEvent<HTMLFormElement>) => {
    setCreatingUserInline(true);
    setAwaitingUserCreateReset(true);
    try {
      const created = await onCreateUser(e, {
        basicGrants: normalizeEditableBasicGrants(basicGrantEntries),
      });
      if (created) {
        setBasicGrantError(null);
        setBasicGrantDraft({ role: "", scope: ROOT_ACCESS_SCOPE });
        setBasicGrantEntries([]);
      }
    } finally {
      setCreatingUserInline(false);
    }
  };

  const handleCreateServiceAccountInline = async (
    e: FormEvent<HTMLFormElement>,
  ) => {
    setCreatingServiceAccountInline(true);
    setAwaitingServiceAccountCreateReset(true);
    try {
      const token = await onCreateServiceAccount(e, {
        basicGrants: normalizeEditableBasicGrants(basicGrantEntries),
      });
      if (token) {
        setCreatedServiceAccountToken(token);
        setCopyServiceAccountTokenLabel("Copy");
        setBasicGrantError(null);
        setBasicGrantDraft({ role: "", scope: ROOT_ACCESS_SCOPE });
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

  const openConfirmDialog = (
    message: string,
    onConfirm: () => Promise<void> | void,
  ) => {
    setConfirmDialog({ message, onConfirm });
  };

  const confirmDeleteUser = (userId: string) => {
    openConfirmDialog("Delete this user? This cannot be undone.", () =>
      onDeleteUser(userId),
    );
  };

  const confirmDeleteServiceAccount = (serviceAccountId: string) => {
    openConfirmDialog(
      "Delete this service account and revoke its tokens? This cannot be undone.",
      () => onDeleteServiceAccount(serviceAccountId),
    );
  };

  const confirmDeleteRoleDefinition = (role: RoleDefinition) => {
    if (isProtectedAccessRole(role.role)) return;
    openConfirmDialog(
      "Delete this role and its policies? This cannot be undone.",
      () => onDeleteRoleDefinition(role),
    );
  };

  const confirmDeletePolicy = (policy: RolePermission) => {
    if (isProtectedAccessRole(policy.role)) return;
    openConfirmDialog("Delete this policy? This cannot be undone.", () =>
      onDeletePolicy(policy),
    );
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
    if (accessMode === "basic") {
      void onReloadUsers();
      void onReloadAccessGrants();
      return;
    }
    if (activeSection === "users") {
      void onReloadUsers();
      void onReloadAccessGrants();
      return;
    }
    if (activeSection === "service-accounts") {
      void onReloadServiceAccounts();
      void onReloadAccessGrants();
      return;
    }
    if (activeSection === "policies") {
      void onReloadPolicies();
      return;
    }
    if (activeSection === "identity-providers") {
      void onReloadIdentityProviders();
      return;
    }
    void onReloadUsers();
    void onReloadServiceAccounts();
    if (activeSection === "roles") {
      void onReloadPolicies();
    }
  }, [
    accessMode,
    activeSection,
    onReloadAccessGrants,
    onReloadIdentityProviders,
    onReloadPolicies,
    onReloadServiceAccounts,
    onReloadUsers,
  ]);

  const handleStageBasicGrant = (e?: FormEvent<HTMLFormElement>) => {
    e?.preventDefault();
    const creatingUser = showUserModal && !userAccessEditor;
    const creatingServiceAccount =
      showServiceAccountModal && !serviceAccountEditor;
    if (
      !selectedBasicUser &&
      !selectedBasicServiceAccount &&
      !creatingUser &&
      !creatingServiceAccount
    ) {
      setBasicGrantError("Select an account first.");
      return;
    }
    if (selectedBasicUser && isDefaultAdminUser(selectedBasicUser)) {
      setBasicGrantError("Default admin role assignments are locked.");
      return;
    }
    if (selectedBasicUser && isExternallyManagedUser(selectedBasicUser)) {
      setBasicGrantError(
        "Role assignments for this user are managed by the identity provider.",
      );
      return;
    }
    if (creatingUser && newUser.sub.trim().toLowerCase() === "admin") {
      setBasicGrantError("Default admin role assignments are locked.");
      return;
    }

    const result = stageBasicGrant(
      basicGrantEntries,
      basicGrantDraft,
      `draft-${Date.now()}-${Math.random().toString(36).slice(2)}`,
    );
    setBasicGrantError(result.error);
    if (result.error) return;
    setBasicGrantEntries(result.entries);
    setBasicGrantDraft((prev) => ({ ...prev, role: "" }));
  };

  const removeBasicGrantDraft = (localID: string) => {
    if (selectedBasicUser && isUserRoleManagementLocked(selectedBasicUser))
      return;
    setBasicGrantEntries((prev) =>
      prev.filter((grant) => grant.localID !== localID),
    );
    setBasicGrantError(null);
  };

  const resetBasicGrantDrafts = () => {
    if (selectedBasicUser && isUserRoleManagementLocked(selectedBasicUser))
      return;
    setBasicGrantEntries(
      selectedBasicGrants.map(editableAccessGrantFromRecord),
    );
    setBasicGrantError(null);
  };

  const saveBasicGrantsForUser = async (user: UserSummary) => {
    if (!selectedBasicUser) {
      setBasicGrantError("Select a user first.");
      return;
    }
    if (isDefaultAdminUser(user)) {
      setBasicGrantError("Default admin role assignments are locked.");
      return;
    }
    if (isExternallyManagedUser(user)) {
      setBasicGrantError(
        "Role assignments for this user are managed by the identity provider.",
      );
      return;
    }
    if (!basicGrantDirty) return;
    const { grantsToDelete, grantsToAdd } = buildBasicGrantChangeSet(
      selectedBasicUserGrants,
      basicGrantEntries,
    );

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
      setBasicGrantError(
        error instanceof Error ? error.message : "Failed to save basic roles",
      );
      throw error;
    } finally {
      setBasicGrantSaving(false);
    }
  };

  const saveBasicGrantsForServiceAccount = async (
    account: ServiceAccountSummary,
  ) => {
    if (!selectedBasicServiceAccount) {
      setBasicGrantError("Select a service account first.");
      return;
    }
    if (!basicGrantDirty) return;
    const { grantsToDelete, grantsToAdd } = buildBasicGrantChangeSet(
      selectedBasicServiceAccountGrants,
      basicGrantEntries,
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
      setBasicGrantError(
        error instanceof Error ? error.message : "Failed to save basic roles",
      );
      throw error;
    } finally {
      setBasicGrantSaving(false);
    }
  };

  const handleCreateServiceAccountToken = async () => {
    if (!serviceAccountEditor) return;
    const name = serviceAccountEditor.tokenName.trim();
    if (!name) {
      setServiceAccountEditor((prev) =>
        prev ? { ...prev, tokensError: "Token name is required." } : prev,
      );
      return;
    }
    setServiceAccountEditor((prev) =>
      prev ? { ...prev, tokensLoading: true, tokensError: null } : prev,
    );
    try {
      const token = await onCreateServiceAccountToken(
        serviceAccountEditor.account.id,
        name,
      );
      const tokens = await onLoadServiceAccountTokens(
        serviceAccountEditor.account.id,
      );
      setCreatedServiceAccountToken(token);
      setCopyServiceAccountTokenLabel("Copy");
      setServiceAccountEditor((prev) =>
        prev?.account.id === serviceAccountEditor.account.id
          ? {
              ...prev,
              tokenName: "rotation",
              tokens,
              tokensLoading: false,
              tokensError: null,
            }
          : prev,
      );
    } catch (error) {
      setServiceAccountEditor((prev) =>
        prev
          ? {
              ...prev,
              tokensLoading: false,
              tokensError:
                error instanceof Error
                  ? error.message
                  : "Failed to create token",
            }
          : prev,
      );
    }
  };

  const handleRevokeServiceAccountToken = async (tokenID: string) => {
    if (!serviceAccountEditor) return;
    setServiceAccountEditor((prev) =>
      prev ? { ...prev, tokensLoading: true, tokensError: null } : prev,
    );
    try {
      await onRevokeServiceAccountToken(
        serviceAccountEditor.account.id,
        tokenID,
      );
      const tokens = await onLoadServiceAccountTokens(
        serviceAccountEditor.account.id,
      );
      setServiceAccountEditor((prev) =>
        prev?.account.id === serviceAccountEditor.account.id
          ? { ...prev, tokens, tokensLoading: false, tokensError: null }
          : prev,
      );
    } catch (error) {
      setServiceAccountEditor((prev) =>
        prev
          ? {
              ...prev,
              tokensLoading: false,
              tokensError:
                error instanceof Error
                  ? error.message
                  : "Failed to revoke token",
            }
          : prev,
      );
    }
  };

  const copyCreatedServiceAccountToken = async () => {
    const token = createdServiceAccountToken?.token;
    if (!token) return;
    try {
      await copyTextToClipboard(token);
      setCopyServiceAccountTokenLabel("Copied");
    } catch {
      setCopyServiceAccountTokenLabel("Copy failed");
    }
    window.setTimeout(() => setCopyServiceAccountTokenLabel("Copy"), 1800);
  };

  const handleSaveIdentityProviderSettings = async (
    event: FormEvent<HTMLFormElement>,
  ) => {
    event.preventDefault();
    setSavingIdentityProviderSettings(true);
    try {
      await onSaveIdentityProviderSettings(
        identityProviderSettingsDraft,
        parseIdentityDomainMappingDraft(identityProviderDomainMappingDraft),
      );
    } finally {
      setSavingIdentityProviderSettings(false);
    }
  };

  const handleSaveIdentityProvider = async (
    event: FormEvent<HTMLFormElement>,
  ) => {
    event.preventDefault();
    setSavingIdentityProvider(true);
    try {
      await onSaveIdentityProvider(identityProviderForm);
      setIdentityProviderForm(emptyIdentityProviderForm());
      setSelectedIdentityProviderID("");
    } finally {
      setSavingIdentityProvider(false);
    }
  };

  const confirmDeleteIdentityProvider = (providerID: string) => {
    openConfirmDialog(
      "Delete this identity provider? Existing external identity links will be removed.",
      () => onDeleteIdentityProvider(providerID),
    );
  };

  const availablePoliciesForRoleEditor = roleEditor ? policyOptions : [];

  return {
    users,
    loading,
    error,
    serviceAccounts,
    serviceAccountsLoading,
    serviceAccountsError,
    accessGrantsLoading,
    accessGrantsError,
    identityProviders,
    filteredIdentityProviders,
    identityProviderSettingsDraft,
    setIdentityProviderSettingsDraft,
    identityProviderDomainMappingDraft,
    setIdentityProviderDomainMappingDraft,
    identityProviderForm,
    setIdentityProviderForm,
    selectedIdentityProvider,
    selectedIdentityProviderID,
    identityProvidersLoading,
    identityProvidersError,
    savingIdentityProvider,
    savingIdentityProviderSettings,
    policiesLoading,
    policiesError,
    resourceCatalog,
    newUser,
    newServiceAccount,
    newPermission,
    onChangeUser,
    onChangeServiceAccount,
    onChangePermission,
    accessMode,
    setAccessMode,
    activeSection,
    setActiveSection,
    showUserModal,
    setShowUserModal,
    setAwaitingUserCreateReset,
    showServiceAccountModal,
    setShowServiceAccountModal,
    setAwaitingServiceAccountCreateReset,
    showPolicyModal,
    setShowPolicyModal,
    setAwaitingPolicyCreateReset,
    roleEditor,
    setRoleEditor,
    policyEditor,
    setPolicyEditor,
    confirmDialog,
    setConfirmDialog,
    confirming,
    userAccessEditor,
    setUserAccessEditor,
    serviceAccountEditor,
    setServiceAccountEditor,
    createdServiceAccountToken,
    setCreatedServiceAccountToken,
    copyServiceAccountTokenLabel,
    creatingUserInline,
    creatingServiceAccountInline,
    creatingPolicyInline,
    savingRoleEditor,
    savingPolicy,
    savingUserAccess,
    savingServiceAccountAccess,
    basicGrantDraft,
    setBasicGrantDraft,
    basicGrantEntries,
    setBasicGrantEntries,
    basicGrantSaving,
    basicGrantError,
    setBasicGrantError,
    roleDefinitions,
    allRoleOptions,
    roleUserMap,
    tabItems,
    policyCount,
    filteredUsers,
    filteredServiceAccounts,
    filteredRoleDefinitions,
    visiblePolicies,
    filteredPolicies,
    sectionContent,
    userRoleAssignmentsLocked,
    userRoleAssignmentsLockLabel,
    basicGrantOptions,
    basicUserGrantMap,
    basicServiceAccountGrantMap,
    basicGrantDirty,
    availablePoliciesForRoleEditor,
    nextPolicyKey,
    setNextPolicyKey,
    nextUserRole,
    setNextUserRole,
    nextAccessRole,
    setNextAccessRole,
    searchTerm,
    setSearchTerm,
    searchOpen,
    setSearchOpen,
    searchInputRef,
    openCreateRoleEditor,
    openCreateUserEditor,
    openCreateServiceAccountEditor,
    openCreatePolicyEditor,
    openCreateIdentityProvider,
    openEditIdentityProvider,
    openEditRoleEditor,
    openPolicyEditModal,
    openUserAccessModal,
    openServiceAccountAccessModal,
    removeRolePolicyDraft,
    addExistingPolicyDraft,
    handleSaveRoleEditor,
    handleSavePolicyEdit,
    handleSaveUserAccess,
    handleSaveServiceAccountAccess,
    addUserAccessEntry,
    removeUserAccessEntry,
    addServiceAccountAccessEntry,
    removeServiceAccountAccessEntry,
    updateNewUserRoleEntry,
    removeNewUserRoleEntry,
    appendUserRoleFromPicker,
    updateNewServiceAccountRoleEntry,
    removeNewServiceAccountRoleEntry,
    appendServiceAccountRoleFromPicker,
    handleCreateUserInline,
    handleCreateServiceAccountInline,
    handleCreatePolicyInline,
    confirmDeleteUser,
    confirmDeleteServiceAccount,
    confirmDeleteRoleDefinition,
    confirmDeletePolicy,
    confirmDeleteIdentityProvider,
    handleConfirmDialog,
    handleRefresh,
    handleSaveIdentityProviderSettings,
    handleSaveIdentityProvider,
    handleStageBasicGrant,
    removeBasicGrantDraft,
    resetBasicGrantDrafts,
    handleCreateServiceAccountToken,
    handleRevokeServiceAccountToken,
    copyCreatedServiceAccountToken,
  };
}

function parseIdentityDomainMappingDraft(
  value: string,
): Record<string, string> {
  const entries: Array<[string, string]> = [];
  value.split("\n").forEach((line) => {
    const trimmed = line.trim();
    if (!trimmed) return;
    const separator = trimmed.includes(":") ? ":" : "=";
    const index = trimmed.indexOf(separator);
    if (index <= 0) return;
    const domain = trimmed
      .slice(0, index)
      .trim()
      .replace(/^@/, "")
      .toLowerCase();
    const providerID = trimmed
      .slice(index + 1)
      .trim()
      .toLowerCase();
    if (domain && providerID) entries.push([domain, providerID]);
  });
  return Object.fromEntries(entries);
}

export type AccessPanelController = ReturnType<typeof useAccessPanelController>;
