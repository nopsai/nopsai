import type {
  EditableAccessGrant,
  RolePermission,
  RolePolicyDraft,
  ServiceAccountSummary,
  ServiceAccountToken,
  UserSummary,
} from './model';

export type AccessMode = 'basic' | 'advanced';
export type AccessSection = 'users' | 'service-accounts' | 'roles' | 'policies';

export type BasicGrantDraft = {
  role: string;
  scope: string;
};

export type NewUserFormState = {
  sub: string;
  email: string;
  password: string;
  roles: string[];
};

export type NewServiceAccountFormState = {
  sub: string;
  email: string;
  tokenName: string;
  roles: string[];
};

export type UserAccessEditorState = {
  user: UserSummary;
  entries: string[];
  original: string[];
  email: string;
  status: string;
  password: string;
};

export type ServiceAccountEditorState = {
  account: ServiceAccountSummary;
  entries: string[];
  original: string[];
  email: string;
  status: string;
  tokenName: string;
  tokens: ServiceAccountToken[];
  tokensLoading: boolean;
  tokensError: string | null;
};

export type RoleEditorState = {
  mode: 'create' | 'edit';
  role: string;
  policies: RolePolicyDraft[];
  original?: RolePermission[];
};

export type PolicyEditorState = {
  original: RolePermission;
  role: string;
  name: string;
  obj: string;
  act: string;
};

export type BasicGrantEditorState = {
  entries: EditableAccessGrant[];
  draft: BasicGrantDraft;
  error: string | null;
};
