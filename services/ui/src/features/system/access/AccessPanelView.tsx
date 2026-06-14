import { Plus, RefreshCw, Search, X } from "lucide-react";
import { ACCESS_UI_BUILD_ID, ROOT_ACCESS_SCOPE } from "./model";
import { AccessConfirmationDialog } from "./AccessConfirmationDialog";
import {
  CreateServiceAccountEditor,
  CreateUserEditor,
} from "./AccessCreateEditors";
import { PoliciesWorkspace, RolesWorkspace } from "./AccessAdvancedWorkspaces";
import {
  ServiceAccountsWorkspace,
  UsersWorkspace,
} from "./AccessIdentityWorkspaces";
import { IdentityProvidersWorkspace } from "./IdentityProvidersWorkspace";
import { accessPresetToneClass } from "./presentation";
import type { AccessPanelController } from "./useAccessPanelController";

export function AccessPanelView({
  controller,
}: {
  controller: AccessPanelController;
}) {
  const {
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
  } = controller;

  const createUserEditor = (
    <CreateUserEditor
      newUser={newUser}
      creating={creatingUserInline}
      allRoleOptions={allRoleOptions}
      nextRole={nextUserRole}
      basicGrantEntries={basicGrantEntries}
      basicGrantDraft={basicGrantDraft}
      basicGrantOptions={basicGrantOptions}
      basicGrantError={basicGrantError}
      toneClassForRole={accessPresetToneClass}
      onChangeUser={onChangeUser}
      onSubmit={handleCreateUserInline}
      onClose={() => {
        setAwaitingUserCreateReset(false);
        setShowUserModal(false);
        setBasicGrantError(null);
        setBasicGrantDraft({ role: "", scope: ROOT_ACCESS_SCOPE });
        setBasicGrantEntries([]);
      }}
      onUpdateRoleEntry={updateNewUserRoleEntry}
      onRemoveRoleEntry={removeNewUserRoleEntry}
      onNextRoleChange={setNextUserRole}
      onAppendRole={appendUserRoleFromPicker}
      onBasicGrantDraftChange={setBasicGrantDraft}
      onAddBasicGrant={() => handleStageBasicGrant()}
      onRemoveBasicGrant={removeBasicGrantDraft}
    />
  );

  const createServiceAccountEditor = (
    <CreateServiceAccountEditor
      newServiceAccount={newServiceAccount}
      createdToken={createdServiceAccountToken}
      copyTokenLabel={copyServiceAccountTokenLabel}
      creating={creatingServiceAccountInline}
      allRoleOptions={allRoleOptions}
      nextRole={nextUserRole}
      basicGrantEntries={basicGrantEntries}
      basicGrantDraft={basicGrantDraft}
      basicGrantOptions={basicGrantOptions}
      basicGrantError={basicGrantError}
      toneClassForRole={accessPresetToneClass}
      onChangeServiceAccount={onChangeServiceAccount}
      onSubmit={handleCreateServiceAccountInline}
      onClose={() => {
        setAwaitingServiceAccountCreateReset(false);
        setShowServiceAccountModal(false);
        setCreatedServiceAccountToken(null);
        setBasicGrantError(null);
        setBasicGrantDraft({ role: "", scope: ROOT_ACCESS_SCOPE });
        setBasicGrantEntries([]);
      }}
      onCopyToken={copyCreatedServiceAccountToken}
      onUpdateRoleEntry={updateNewServiceAccountRoleEntry}
      onRemoveRoleEntry={removeNewServiceAccountRoleEntry}
      onNextRoleChange={setNextUserRole}
      onAppendRole={appendServiceAccountRoleFromPicker}
      onBasicGrantDraftChange={setBasicGrantDraft}
      onAddBasicGrant={() => handleStageBasicGrant()}
      onRemoveBasicGrant={removeBasicGrantDraft}
    />
  );

  const accessSearchPlaceholder =
    accessMode === "basic"
      ? "Search by username, email, role, or group"
      : sectionContent.searchPlaceholder;
  const accessSearchControl = (
    <div
      className={`pipelines-search-shell access-search-shell ${searchOpen ? "open" : ""}`}
    >
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
        onChange={(event) => {
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
            setSearchTerm("");
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
    <UsersWorkspace
      users={users}
      filteredUsers={filteredUsers}
      grantMap={basicUserGrantMap}
      selectedUserID={userAccessEditor?.user.id}
      loading={loading}
      error={error}
      grantsLoading={accessGrantsLoading}
      grantsError={accessGrantsError}
      userAccessEditor={userAccessEditor}
      showUserModal={showUserModal}
      createUserEditor={createUserEditor}
      allRoleOptions={allRoleOptions}
      nextAccessRole={nextAccessRole}
      userRoleAssignmentsLocked={userRoleAssignmentsLocked}
      userRoleAssignmentsLockLabel={userRoleAssignmentsLockLabel}
      savingUserAccess={savingUserAccess}
      entries={basicGrantEntries}
      draft={basicGrantDraft}
      options={basicGrantOptions}
      basicGrantError={basicGrantError}
      basicGrantSaving={basicGrantSaving}
      basicGrantDirty={basicGrantDirty}
      toneClassForRole={accessPresetToneClass}
      onEdit={openUserAccessModal}
      onDelete={confirmDeleteUser}
      onCloseEditor={() => setUserAccessEditor(null)}
      onSubmit={handleSaveUserAccess}
      onChangeEmail={(email) =>
        setUserAccessEditor((prev) => (prev ? { ...prev, email } : prev))
      }
      onChangeStatus={(status) =>
        setUserAccessEditor((prev) => (prev ? { ...prev, status } : prev))
      }
      onChangePassword={(password) =>
        setUserAccessEditor((prev) => (prev ? { ...prev, password } : prev))
      }
      onNextAccessRoleChange={setNextAccessRole}
      onAddAccessEntry={addUserAccessEntry}
      onRemoveAccessEntry={removeUserAccessEntry}
      onDraftChange={setBasicGrantDraft}
      onAdd={() => handleStageBasicGrant()}
      onRemove={removeBasicGrantDraft}
      onReset={resetBasicGrantDrafts}
    />
  );

  const serviceAccountsWorkspace = (
    <ServiceAccountsWorkspace
      accounts={serviceAccounts}
      filteredAccounts={filteredServiceAccounts}
      grantMap={basicServiceAccountGrantMap}
      selectedAccountID={serviceAccountEditor?.account.id}
      loading={serviceAccountsLoading}
      error={serviceAccountsError}
      grantsLoading={accessGrantsLoading}
      grantsError={accessGrantsError}
      serviceAccountEditor={serviceAccountEditor}
      showServiceAccountModal={showServiceAccountModal}
      createServiceAccountEditor={createServiceAccountEditor}
      allRoleOptions={allRoleOptions}
      nextAccessRole={nextAccessRole}
      createdToken={createdServiceAccountToken}
      copyTokenLabel={copyServiceAccountTokenLabel}
      savingServiceAccountAccess={savingServiceAccountAccess}
      entries={basicGrantEntries}
      draft={basicGrantDraft}
      options={basicGrantOptions}
      basicGrantError={basicGrantError}
      basicGrantSaving={basicGrantSaving}
      basicGrantDirty={basicGrantDirty}
      toneClassForRole={accessPresetToneClass}
      onEdit={openServiceAccountAccessModal}
      onDelete={confirmDeleteServiceAccount}
      onCloseEditor={() => setServiceAccountEditor(null)}
      onSubmit={handleSaveServiceAccountAccess}
      onChangeEmail={(email) =>
        setServiceAccountEditor((prev) => (prev ? { ...prev, email } : prev))
      }
      onChangeStatus={(status) =>
        setServiceAccountEditor((prev) => (prev ? { ...prev, status } : prev))
      }
      onChangeTokenName={(tokenName) =>
        setServiceAccountEditor((prev) =>
          prev ? { ...prev, tokenName } : prev,
        )
      }
      onCreateToken={handleCreateServiceAccountToken}
      onRevokeToken={handleRevokeServiceAccountToken}
      onCopyToken={copyCreatedServiceAccountToken}
      onNextAccessRoleChange={setNextAccessRole}
      onAddAccessEntry={addServiceAccountAccessEntry}
      onRemoveAccessEntry={removeServiceAccountAccessEntry}
      onDraftChange={setBasicGrantDraft}
      onAdd={() => handleStageBasicGrant()}
      onRemove={removeBasicGrantDraft}
      onReset={resetBasicGrantDrafts}
    />
  );

  return (
    <div className="access-layout pb-24" data-access-build={ACCESS_UI_BUILD_ID}>
      <div className="access-shell">
        <div className="access-header">
          <div className="access-title-group">
            <h3 className="access-header__title">Access</h3>
          </div>
          <div className="access-header__actions">
            <div
              className="access-mode-switch"
              role="tablist"
              aria-label="Access mode"
            >
              <button
                type="button"
                role="tab"
                aria-selected={accessMode === "basic"}
                className={`access-mode-switch__option ${accessMode === "basic" ? "access-mode-switch__option--active" : ""}`}
                onClick={() => setAccessMode("basic")}
              >
                <span className="access-mode-switch__title">Basic</span>
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={accessMode === "advanced"}
                className={`access-mode-switch__option ${accessMode === "advanced" ? "access-mode-switch__option--active" : ""}`}
                onClick={() => setAccessMode("advanced")}
              >
                <span className="access-mode-switch__title">Advanced</span>
              </button>
            </div>
            <button
              className="glass-button-ghost access-toolbar-btn"
              type="button"
              onClick={handleRefresh}
              disabled={
                loading ||
                serviceAccountsLoading ||
                accessGrantsLoading ||
                policiesLoading
              }
            >
              <RefreshIcon />
              <span>Refresh</span>
            </button>
          </div>
        </div>

        {accessMode === "advanced" && (
          <div className="access-nav">
            <div className="access-tabs">
              {tabItems.map((tab) => (
                <button
                  key={tab.id}
                  type="button"
                  className={`access-tab ${activeSection === tab.id ? "access-tab--active" : ""}`}
                  onClick={() => setActiveSection(tab.id)}
                >
                  <span className="access-tab__label">{tab.label}</span>
                  <span className="access-tab__badge">
                    {tab.id === "policies" ? policyCount : tab.count}
                  </span>
                </button>
              ))}
            </div>
          </div>
        )}

        {accessMode === "basic" ? (
          <div className="access-panel-card">
            <div className="access-section-header">
              <div className="space-y-1">
                <h4 className="access-section-title">People and basic roles</h4>
              </div>
              <div className="access-section-tools">
                {accessSearchControl}
                <button
                  type="button"
                  className="glass-button-primary access-section-action"
                  onClick={openCreateUserEditor}
                >
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
                {activeSection === "users" && (
                  <button
                    type="button"
                    className="glass-button-primary access-section-action"
                    onClick={openCreateUserEditor}
                  >
                    <PlusIcon />
                    <span>Add user</span>
                  </button>
                )}
                {activeSection === "service-accounts" && (
                  <button
                    type="button"
                    className="glass-button-primary access-section-action"
                    onClick={openCreateServiceAccountEditor}
                  >
                    <PlusIcon />
                    <span>Add service account</span>
                  </button>
                )}
                {activeSection === "roles" && (
                  <button
                    type="button"
                    className="glass-button-primary access-section-action"
                    onClick={openCreateRoleEditor}
                  >
                    <PlusIcon />
                    <span>Add role</span>
                  </button>
                )}
                {activeSection === "identity-providers" && (
                  <button
                    type="button"
                    className="glass-button-primary access-section-action"
                    onClick={openCreateIdentityProvider}
                  >
                    <PlusIcon />
                    <span>Add provider</span>
                  </button>
                )}
                {activeSection === "policies" && (
                  <button
                    type="button"
                    className="glass-button-primary access-section-action"
                    onClick={openCreatePolicyEditor}
                  >
                    <PlusIcon />
                    <span>Add policy</span>
                  </button>
                )}
              </div>
            </div>

            {activeSection === "users" && usersWorkspace}

            {activeSection === "service-accounts" && serviceAccountsWorkspace}

            {activeSection === "roles" && (
              <RolesWorkspace
                roles={roleDefinitions}
                filteredRoles={filteredRoleDefinitions}
                roleUserMap={roleUserMap}
                selectedRole={roleEditor?.role}
                loading={loading || policiesLoading}
                error={policiesError || error}
                roleEditor={roleEditor}
                availablePolicies={availablePoliciesForRoleEditor}
                nextPolicyKey={nextPolicyKey}
                saving={savingRoleEditor}
                onEdit={openEditRoleEditor}
                onDelete={confirmDeleteRoleDefinition}
                onCloseEditor={() => setRoleEditor(null)}
                onSubmit={handleSaveRoleEditor}
                onChangeRoleName={(role) =>
                  setRoleEditor((prev) => (prev ? { ...prev, role } : prev))
                }
                onRemovePolicyDraft={removeRolePolicyDraft}
                onNextPolicyKeyChange={setNextPolicyKey}
                onAddPolicyDraft={addExistingPolicyDraft}
              />
            )}

            {activeSection === "identity-providers" && (
              <IdentityProvidersWorkspace
                providers={identityProviders}
                filteredProviders={filteredIdentityProviders}
                settings={identityProviderSettingsDraft}
                domainMappingDraft={identityProviderDomainMappingDraft}
                form={identityProviderForm}
                selectedProvider={selectedIdentityProvider}
                loading={identityProvidersLoading}
                error={identityProvidersError}
                savingSettings={savingIdentityProviderSettings}
                savingProvider={savingIdentityProvider}
                onSettingsChange={setIdentityProviderSettingsDraft}
                onDomainMappingChange={setIdentityProviderDomainMappingDraft}
                onFormChange={setIdentityProviderForm}
                onEdit={openEditIdentityProvider}
                onCreate={openCreateIdentityProvider}
                onDelete={confirmDeleteIdentityProvider}
                onSubmitSettings={handleSaveIdentityProviderSettings}
                onSubmitProvider={handleSaveIdentityProvider}
              />
            )}

            {activeSection === "policies" && (
              <PoliciesWorkspace
                policies={visiblePolicies}
                filteredPolicies={filteredPolicies}
                selectedPolicy={policyEditor?.original}
                loading={policiesLoading}
                error={policiesError}
                policyEditor={policyEditor}
                showPolicyModal={showPolicyModal}
                newPermission={newPermission}
                resourceCatalog={resourceCatalog}
                saving={savingPolicy}
                creating={creatingPolicyInline}
                onEdit={openPolicyEditModal}
                onDelete={confirmDeletePolicy}
                onCloseEditor={() => setPolicyEditor(null)}
                onCloseCreate={() => {
                  setAwaitingPolicyCreateReset(false);
                  setShowPolicyModal(false);
                }}
                onSubmitEdit={handleSavePolicyEdit}
                onSubmitCreate={handleCreatePolicyInline}
                onChangeEditor={(next) =>
                  setPolicyEditor((prev) =>
                    prev ? { ...prev, ...next } : prev,
                  )
                }
                onChangeCreate={onChangePermission}
              />
            )}
          </div>
        )}
      </div>

      {confirmDialog && (
        <AccessConfirmationDialog
          message={confirmDialog.message}
          pending={confirming}
          onCancel={() => setConfirmDialog(null)}
          onConfirm={handleConfirmDialog}
        />
      )}
    </div>
  );
}

function PlusIcon() {
  return <Plus className="h-4 w-4" strokeWidth={2} aria-hidden="true" />;
}

function RefreshIcon() {
  return <RefreshCw className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />;
}

function SearchIcon() {
  return <Search className="h-4 w-4" strokeWidth={1.8} aria-hidden="true" />;
}
