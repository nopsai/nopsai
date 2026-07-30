import type { FormEvent } from 'react';
import { Trash2 } from 'lucide-react';
import { AccessPoliciesCatalog, AccessRolesCatalog } from './AccessEntityCatalogs';
import { AccessEditorDrawer } from './AccessEditorDrawer';
import { AccessPolicyRuleFields } from './AccessPolicyRuleFields';
import {
  AccessFormCard,
  AccessReviewRow,
  AccessReviewStat,
  AccessSectionedEditor,
  type AccessEditorSection,
} from './AccessSectionedEditor';
import { formatAccessCount } from './presentation';
import { policyLabel, type RoleDefinition, type RolePermission, type RolePolicyDraft } from './model';
import { parseAAAActionValue } from './policyRuleModel';
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

function titleCase(value: string) {
  return (value || 'unknown')
    .replace(/[-_]/g, ' ')
    .replace(/\b\w/g, letter => letter.toUpperCase());
}

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
  const editorContent = roleEditor
    ? (() => {
        const selectedDefinition = roles.find(role => role.role === roleEditor.role);
        const assignees = selectedDefinition ? roleUserMap.get(selectedDefinition.id) || [] : [];
        const roleTitle = roleEditor.role || 'Role definition';
        const sections: AccessEditorSection[] = [
          {
            id: 'details',
            label: 'Details',
            description: 'Name and type',
            children: (
              <AccessFormCard
                title="Role identity"
                description="A stable role name makes assignment reviews easier."
                badge={roleEditor.mode === 'edit' ? 'Existing role' : 'New role'}
              >
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
                <dl className="access-review-list">
                  <AccessReviewRow
                    label="Role type"
                    value={roleEditor.mode === 'edit' ? 'Custom reusable role' : 'Custom role'}
                  />
                  <AccessReviewRow
                    label="Current assignees"
                    value={formatAccessCount(assignees.length, 'assignee')}
                  />
                </dl>
              </AccessFormCard>
            ),
          },
          {
            id: 'permissions',
            label: 'Permissions',
            description: 'AAA policies',
            children: (
              <>
                <AccessFormCard
                  title="Add policy"
                  description="Attach one reusable low-level policy at a time."
                  badge={`${availablePolicies.length} available`}
                >
                  <div className="access-editor-inline-add">
                    {availablePolicies.length > 0 ? (
                      <>
                        <select
                          className="pipelines-input w-full"
                          value={nextPolicyKey}
                          onChange={event => onNextPolicyKeyChange(event.target.value)}
                        >
                          <option value="" disabled>
                            Add policy...
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
                </AccessFormCard>
                <AccessFormCard
                  title="Included policies"
                  description="Review the low-level actions bundled by this role."
                  badge={formatAccessCount(roleEditor.policies.length, 'policy', 'policies')}
                >
                  <div className="access-permission-list">
                    {roleEditor.policies.length ? (
                      roleEditor.policies.map((policy, index) => (
                        <RolePolicyDraftRow key={`policy-${index}`} policy={policy} index={index} onRemove={onRemovePolicyDraft} />
                      ))
                    ) : (
                      <p className="access-empty-card">No policies added.</p>
                    )}
                  </div>
                </AccessFormCard>
              </>
            ),
          },
          {
            id: 'review',
            label: 'Review',
            description: 'Save impact',
            children: (
              <>
                <div className="access-review-grid">
                  <AccessReviewStat label="Policies" value={roleEditor.policies.length} />
                  <AccessReviewStat label="Assignees" value={assignees.length} />
                  <AccessReviewStat label="Mode" value={titleCase(roleEditor.mode)} />
                </div>
                <AccessFormCard
                  title="Role summary"
                  description="Reusable access bundle."
                  badge={roleEditor.mode === 'edit' ? 'Update' : 'Create'}
                >
                  <dl className="access-review-list">
                    <AccessReviewRow label="Name" value={roleTitle} />
                    <AccessReviewRow label="Type" value="Custom" />
                    <AccessReviewRow
                      label="Policies"
                      value={formatAccessCount(roleEditor.policies.length, 'policy', 'policies')}
                    />
                  </dl>
                </AccessFormCard>
              </>
            ),
          },
        ];

        return (
          <AccessSectionedEditor
            modeLabel={roleEditor.mode === 'create' ? 'Create' : 'Edit'}
            entityLabel="Role"
            title={roleTitle}
            subtitle="Assign reusable policies and review the effective access bundle."
            icon="R"
            sections={sections}
            resetKey={`role-${roleEditor.mode}-${roleTitle}`}
            saveLabel={roleEditor.mode === 'create' ? 'Create role' : 'Save role'}
            savingLabel="Saving..."
            saving={saving}
            deleteLabel="Delete role"
            onClose={onCloseEditor}
            onDelete={
              selectedDefinition
                ? () => {
                    onCloseEditor();
                    onDelete(selectedDefinition);
                  }
                : undefined
            }
            onSubmit={onSubmit}
          />
        );
      })()
    : null;

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
      <AccessEditorDrawer open={Boolean(editorContent)} label="Role editor" onClose={onCloseEditor}>
        {editorContent}
      </AccessEditorDrawer>
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
  const editorContent = policyEditor
    ? (() => {
        const parsedAction = parseAAAActionValue(policyEditor.act);
        const effectLabel = titleCase(parsedAction.effect);
        const sections: AccessEditorSection[] = [
          {
            id: 'rule',
            label: 'Rule',
            description: 'Resource and action',
            children: (
              <>
                <AccessFormCard
                  title="Rule definition"
                  description="The label, resource selector, effect, and action evaluated by the access engine."
                  badge={effectLabel}
                >
                  <AccessPolicyRuleFields policy={policyEditor} onChange={onChangeEditor} resourceCatalog={resourceCatalog} />
                </AccessFormCard>
                <AccessFormCard
                  title="Assignment"
                  description="Policies are consumed through reusable roles rather than assigned directly."
                  badge={policyEditor.role}
                >
                  <dl className="access-review-list">
                    <AccessReviewRow label="Reusable role" value={policyEditor.role} />
                    <AccessReviewRow label="Original label" value={policyLabel(policyEditor.original)} />
                  </dl>
                </AccessFormCard>
              </>
            ),
          },
          {
            id: 'review',
            label: 'Review',
            description: 'Save impact',
            children: (
              <>
                <AccessFormCard
                  title="Effective rule"
                  description="The expression that will be evaluated by the access engine."
                  badge={effectLabel}
                >
                  <div className="access-permission-row">
                    <div>
                      <div className="access-permission-expression">
                        {policyEditor.obj} - {parsedAction.action || policyEditor.act}
                      </div>
                      <div className="access-permission-meta">
                        {effectLabel} through {policyEditor.role}
                      </div>
                    </div>
                    <span className={`access-chip ${parsedAction.effect === 'deny' ? 'access-chip--danger' : 'access-chip--success'}`}>
                      {effectLabel}
                    </span>
                  </div>
                </AccessFormCard>
                <AccessFormCard
                  title="Summary"
                  description="Assignment and selector state."
                  badge="Update"
                >
                  <dl className="access-review-list">
                    <AccessReviewRow label="Policy" value={policyEditor.name || policyLabel(policyEditor)} />
                    <AccessReviewRow label="Resource" value={policyEditor.obj || 'Not selected'} />
                    <AccessReviewRow label="Action" value={parsedAction.action || 'Not selected'} />
                    <AccessReviewRow label="Role" value={policyEditor.role} />
                  </dl>
                </AccessFormCard>
              </>
            ),
          },
        ];

        return (
          <AccessSectionedEditor
            modeLabel="Edit"
            entityLabel="Policy"
            title={policyEditor.name || policyLabel(policyEditor)}
            subtitle="Update this reusable AAA rule."
            icon="P"
            sections={sections}
            resetKey={`policy-${policyEditor.original.role}-${policyEditor.original.obj}-${policyEditor.original.act}`}
            saveLabel="Save changes"
            savingLabel="Saving..."
            saving={saving}
            deleteLabel="Delete policy"
            onClose={onCloseEditor}
            onDelete={() => {
              onCloseEditor();
              onDelete(policyEditor.original);
            }}
            onSubmit={onSubmitEdit}
          />
        );
      })()
    : showPolicyModal
      ? (() => {
          const parsedAction = parseAAAActionValue(newPermission.act);
          const effectLabel = titleCase(parsedAction.effect);
          const sections: AccessEditorSection[] = [
            {
              id: 'rule',
              label: 'Rule',
              description: 'Resource and action',
              children: (
                <AccessFormCard
                  title="Rule definition"
                  description="The label, resource selector, effect, and action evaluated by the access engine."
                  badge={effectLabel}
                >
                  <AccessPolicyRuleFields policy={newPermission} onChange={onChangeCreate} resourceCatalog={resourceCatalog} />
                </AccessFormCard>
              ),
            },
            {
              id: 'review',
              label: 'Review',
              description: 'Create impact',
              children: (
                <AccessFormCard
                  title="Effective rule"
                  description="The rule that will be made available for role assignment."
                  badge="Template"
                >
                  <dl className="access-review-list">
                    <AccessReviewRow label="Policy" value={newPermission.name || 'Not entered'} />
                    <AccessReviewRow label="Resource" value={newPermission.obj || 'Not selected'} />
                    <AccessReviewRow label="Action" value={parsedAction.action || 'Not selected'} />
                    <AccessReviewRow label="Effect" value={effectLabel} />
                  </dl>
                </AccessFormCard>
              ),
            },
          ];

          return (
            <AccessSectionedEditor
              modeLabel="Create"
              entityLabel="Policy"
              title="Reusable AAA rule"
              subtitle="Define a reusable rule for the access role catalog."
              icon="P"
              sections={sections}
              resetKey="policy-create"
              saveLabel="Add policy"
              savingLabel="Adding..."
              saving={creating}
              onClose={onCloseCreate}
              onSubmit={onSubmitCreate}
            />
          );
        })()
      : null;
  const closeDrawer = policyEditor ? onCloseEditor : onCloseCreate;

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
      <AccessEditorDrawer open={Boolean(editorContent)} label="Policy editor" onClose={closeDrawer}>
        {editorContent}
      </AccessEditorDrawer>
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
  const parsedAction = parseAAAActionValue(policy.act);
  return (
    <div className="access-permission-row">
      <div className="min-w-0">
        <p className="access-permission-title">{policyLabel(policy)}</p>
        <p className="access-permission-expression">
          {policy.obj || 'Select a resource'} - {parsedAction.action || policy.act || 'Select an action'}
        </p>
        <p className="access-permission-meta">{titleCase(parsedAction.effect)} through this role</p>
      </div>
      <button
        type="button"
        className="access-permission-remove"
        onClick={() => onRemove(index)}
        title="Remove policy"
        aria-label={`Remove ${policyLabel(policy)}`}
      >
        <TrashIcon />
      </button>
    </div>
  );
}

function TrashIcon() {
  return <Trash2 className="h-4 w-4" strokeWidth={1.9} aria-hidden="true" />;
}
