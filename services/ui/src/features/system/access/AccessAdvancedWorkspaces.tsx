import type { FormEvent } from 'react';
import { Trash2 } from 'lucide-react';
import { AccessPoliciesCatalog, AccessRolesCatalog } from './AccessEntityCatalogs';
import { AccessEditorEmptyState } from './AccessModal';
import { AccessPolicyRuleFields } from './AccessPolicyRuleFields';
import { formatAccessCount } from './presentation';
import { policyLabel, type RoleDefinition, type RolePermission, type RolePolicyDraft } from './model';
import type { AccessResourceCatalog } from './resourceCatalog';
import type { PolicyEditorState, RoleEditorState } from './panelTypes';

type RoleAssignee = {
  user: string;
  userId: string;
  email: string;
  kind: string;
};

type AvailablePolicyOption = {
  key: string;
  obj: string;
  act: string;
  name?: string;
  label: string;
};

export type RolesWorkspaceProps = {
  roles: RoleDefinition[];
  filteredRoles: RoleDefinition[];
  roleUserMap: Map<string, RoleAssignee[]>;
  selectedRole?: string;
  loading: boolean;
  error: string | null;
  roleEditor: RoleEditorState | null;
  availablePolicies: AvailablePolicyOption[];
  nextPolicyKey: string;
  saving: boolean;
  onEdit: (role: RoleDefinition) => void;
  onDelete: (role: RoleDefinition) => void;
  onCloseEditor: () => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onChangeRoleName: (value: string) => void;
  onRemovePolicyDraft: (index: number) => void;
  onNextPolicyKeyChange: (value: string) => void;
  onAddPolicyDraft: (key: string) => void;
};

export function RolesWorkspace({
  roles,
  filteredRoles,
  roleUserMap,
  selectedRole,
  loading,
  error,
  roleEditor,
  availablePolicies,
  nextPolicyKey,
  saving,
  onEdit,
  onDelete,
  onCloseEditor,
  onSubmit,
  onChangeRoleName,
  onRemovePolicyDraft,
  onNextPolicyKeyChange,
  onAddPolicyDraft,
}: RolesWorkspaceProps) {
  return (
    <div className="access-workspace">
      <div className="space-y-4 access-workspace__list">
        <AccessRolesCatalog
          roles={roles}
          filteredRoles={filteredRoles}
          roleUserMap={roleUserMap}
          selectedRole={selectedRole}
          loading={loading}
          error={error}
          onEdit={onEdit}
          onDelete={onDelete}
        />
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
              <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={onCloseEditor}>
                Close
              </button>
            </div>
            <form className="access-editor-form access-editor-form--compact" onSubmit={onSubmit}>
              <label className="access-minimal-label">
                <span>Role name</span>
                <input
                  className="pipelines-input"
                  value={roleEditor.role}
                  onChange={event => onChangeRoleName(event.target.value)}
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
                  {roleEditor.policies.map((policy, index) => (
                    <RolePolicyDraftRow key={`policy-${index}`} policy={policy} index={index} onRemove={onRemovePolicyDraft} />
                  ))}
                  <div className="access-editor-inline-add">
                    {availablePolicies.length > 0 ? (
                      <>
                        <select className="pipelines-input w-full" value={nextPolicyKey} onChange={event => onNextPolicyKeyChange(event.target.value)}>
                          <option value="" disabled>
                            Add policy…
                          </option>
                          {availablePolicies.map(item => (
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
                              onAddPolicyDraft(nextPolicyKey);
                              onNextPolicyKeyChange('');
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
                <button type="submit" className="glass-button-primary" disabled={saving}>
                  {saving ? 'Saving…' : 'Save role'}
                </button>
              </div>
            </form>
          </div>
        ) : (
          <AccessEditorEmptyState sectionLabel="Role details" hint="Select a role to edit policies." />
        )}
      </aside>
    </div>
  );
}

export type PoliciesWorkspaceProps = {
  policies: RolePermission[];
  filteredPolicies: RolePermission[];
  selectedPolicy?: RolePermission;
  loading: boolean;
  error: string | null;
  policyEditor: PolicyEditorState | null;
  showPolicyModal: boolean;
  newPermission: { name: string; obj: string; act: string };
  resourceCatalog: AccessResourceCatalog;
  saving: boolean;
  creating: boolean;
  onEdit: (policy: RolePermission) => void;
  onDelete: (policy: RolePermission) => void;
  onCloseEditor: () => void;
  onCloseCreate: () => void;
  onSubmitEdit: (event: FormEvent<HTMLFormElement>) => void;
  onSubmitCreate: (event: FormEvent<HTMLFormElement>) => void;
  onChangeEditor: (next: Partial<PolicyEditorState>) => void;
  onChangeCreate: (next: { name: string; obj: string; act: string }) => void;
};

export function PoliciesWorkspace({
  policies,
  filteredPolicies,
  selectedPolicy,
  loading,
  error,
  policyEditor,
  showPolicyModal,
  newPermission,
  resourceCatalog,
  saving,
  creating,
  onEdit,
  onDelete,
  onCloseEditor,
  onCloseCreate,
  onSubmitEdit,
  onSubmitCreate,
  onChangeEditor,
  onChangeCreate,
}: PoliciesWorkspaceProps) {
  return (
    <div className="access-workspace">
      <div className="space-y-4 access-workspace__list">
        <AccessPoliciesCatalog
          policies={policies}
          filteredPolicies={filteredPolicies}
          selectedPolicy={selectedPolicy}
          loading={loading}
          error={error}
          onEdit={onEdit}
          onDelete={onDelete}
        />
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
              <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={onCloseEditor}>
                Close
              </button>
            </div>
            <form className="access-editor-form access-editor-form--compact" onSubmit={onSubmitEdit}>
              <AccessPolicyRuleFields policy={policyEditor} onChange={onChangeEditor} resourceCatalog={resourceCatalog} />
              <div className="access-editor-footer access-editor-footer--inline">
                <button type="submit" className="glass-button-primary" disabled={saving}>
                  {saving ? 'Saving…' : 'Save changes'}
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
              <button type="button" className="access-inline-btn access-inline-btn--pill" onClick={onCloseCreate}>
                Close
              </button>
            </div>
            <form className="access-editor-form access-editor-form--compact" onSubmit={onSubmitCreate}>
              <AccessPolicyRuleFields policy={newPermission} onChange={onChangeCreate} resourceCatalog={resourceCatalog} />
              <div className="access-editor-footer access-editor-footer--inline">
                <button type="submit" className="glass-button-primary" disabled={creating}>
                  {creating ? 'Adding…' : 'Add policy'}
                </button>
              </div>
            </form>
          </div>
        ) : (
          <AccessEditorEmptyState sectionLabel="Policy details" hint="Select a policy to edit rules." />
        )}
      </aside>
    </div>
  );
}

function RolePolicyDraftRow({
  policy,
  index,
  onRemove,
}: {
  policy: RolePolicyDraft;
  index: number;
  onRemove: (index: number) => void;
}) {
  return (
    <div className="access-minimal-row">
      <div className="flex-1 space-y-1">
        <p className="font-semibold truncate">{policyLabel(policy)}</p>
        <p className="text-[11px] text-[var(--text-secondary)] truncate">{policy.obj || 'Select an object'}</p>
        <p className="text-[11px] text-[var(--text-secondary)] truncate">{policy.act || 'Select an action'}</p>
      </div>
      <button
        type="button"
        className="access-inline-btn access-inline-btn--danger"
        onClick={() => onRemove(index)}
        title="Remove policy"
      >
        <TrashIcon />
      </button>
    </div>
  );
}

function TrashIcon() {
  return <Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />;
}
