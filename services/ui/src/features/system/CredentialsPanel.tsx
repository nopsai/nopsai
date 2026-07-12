import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useAIResourceTeamPaths } from './useAIResourceTeamPaths';
import { CredentialCatalog, type CredentialScopeTab } from './credentials/CredentialCatalog';
import { CredentialCreateForm } from './credentials/CredentialCreateForm';
import { CredentialDashboard } from './credentials/CredentialDashboard';
import { CredentialDetail } from './credentials/CredentialDetail';
import {
  credentialCatalogGroups,
  credentialNamespaces,
  credentialSummary,
  filterCredentials,
  isTeamCredentialReference,
  normalizeCredentialTeamPath,
  parseCredentialReference,
  type CredentialRecord,
} from './credentials/model';
import { useCredentials } from './credentials/useCredentials';
import './credentials/credentials.css';

type CredentialsController = ReturnType<typeof useCredentials>;

type CredentialsPanelBodyProps = {
  canManage: boolean;
  controller: CredentialsController;
  isNopsAIAdmin: boolean;
  linkedCredentialRef: string;
  teamPaths: string[];
  teamPathsLoading: boolean;
  onCloseCredentialDetails: () => void;
  onSelectCredential: (credential: CredentialRecord) => void;
  onStartCreate: () => void;
};

function CredentialsPanel({ canManage, isNopsAIAdmin = false }: { canManage: boolean; isNopsAIAdmin?: boolean }) {
  const controller = useCredentials({ canManage });
  const { teamPaths, teamPathsLoading } = useAIResourceTeamPaths();
  const [searchParams, setSearchParams] = useSearchParams();
  const linkedCredentialRef = (searchParams.get('credential') || '').trim();
  const suppressedLinkedCredentialRef = useRef('');
  const {
    closeDetails,
    creating,
    credentials,
    loading,
    selectCredential: selectControllerCredential,
    selected,
    startCreate: startControllerCreate,
  } = controller;

  useEffect(() => {
    if (!linkedCredentialRef) {
      suppressedLinkedCredentialRef.current = '';
      return;
    }
    if (linkedCredentialRef === suppressedLinkedCredentialRef.current || loading || creating) return;
    const match = credentials.find(credential =>
      credential.reference === linkedCredentialRef &&
      (isNopsAIAdmin || isTeamCredentialReference(credential.reference))
    );
    if (match && selected?.id !== match.id) void selectControllerCredential(match);
  }, [
    credentials,
    creating,
    isNopsAIAdmin,
    linkedCredentialRef,
    loading,
    selectControllerCredential,
    selected?.id,
  ]);

  const selectCredential = (credential: CredentialRecord) => {
    suppressedLinkedCredentialRef.current = '';
    const next = new URLSearchParams(searchParams);
    next.set('credential', credential.reference);
    setSearchParams(next, { replace: true });
    void selectControllerCredential(credential);
  };

  const closeCredentialDetails = () => {
    suppressedLinkedCredentialRef.current = linkedCredentialRef;
    closeDetails();
    const next = new URLSearchParams(searchParams);
    next.delete('credential');
    setSearchParams(next, { replace: true });
  };

  const startCreate = () => {
    suppressedLinkedCredentialRef.current = linkedCredentialRef;
    startControllerCreate();
    if (!isNopsAIAdmin) {
      const defaultTeamPath = normalizeCredentialTeamPath(teamPaths[0] || '');
      if (defaultTeamPath) {
        controller.setForm(current => ({ ...current, namespace: 'team', team_path: defaultTeamPath }));
      }
    }
    const next = new URLSearchParams(searchParams);
    next.delete('credential');
    setSearchParams(next, { replace: true });
  };

  return (
    <CredentialsPanelBody
      canManage={canManage}
      controller={controller}
      isNopsAIAdmin={isNopsAIAdmin}
      linkedCredentialRef={linkedCredentialRef}
      teamPaths={teamPaths}
      teamPathsLoading={teamPathsLoading}
      onCloseCredentialDetails={closeCredentialDetails}
      onSelectCredential={selectCredential}
      onStartCreate={startCreate}
    />
  );
}

function CredentialsPanelBody({
  canManage,
  controller,
  isNopsAIAdmin,
  linkedCredentialRef,
  teamPaths,
  teamPathsLoading,
  onCloseCredentialDetails,
  onSelectCredential,
  onStartCreate,
}: CredentialsPanelBodyProps) {
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState('all');
  const [scopeOverride, setScopeOverride] = useState<string | null>(null);
  const [grouped, setGrouped] = useState(true);
  const visibleCredentials = useMemo(
    () => isNopsAIAdmin
      ? controller.credentials
      : controller.credentials.filter(credential => isTeamCredentialReference(credential.reference)),
    [controller.credentials, isNopsAIAdmin]
  );
  const visibleSelected = useMemo(
    () => controller.selected && visibleCredentials.some(credential => credential.id === controller.selected?.id)
      ? controller.selected
      : null,
    [controller.selected, visibleCredentials]
  );
  const linkedCredential = useMemo(
    () => visibleCredentials.find(credential => credential.reference === linkedCredentialRef),
    [linkedCredentialRef, visibleCredentials]
  );
  const requestedScope = scopeOverride ?? (linkedCredential ? parseCredentialReference(linkedCredential.reference).namespace : 'all');
  const scope = !isNopsAIAdmin && !['all', 'team'].includes(requestedScope) ? 'team' : requestedScope;
  const canCreateCredentials = canManage && (isNopsAIAdmin || teamPathsLoading || teamPaths.length > 0);
  const summary = useMemo(() => credentialSummary(visibleCredentials), [visibleCredentials]);
  const namespaces = useMemo(() => credentialNamespaces(visibleCredentials), [visibleCredentials]);
  const filteredCredentials = useMemo(
    () => filterCredentials(visibleCredentials, query, status, scope),
    [query, scope, status, visibleCredentials]
  );
  const groups = useMemo(
    () => credentialCatalogGroups(filteredCredentials, teamPaths),
    [filteredCredentials, teamPaths]
  );
  const scopeTabs = useMemo(
    () => buildCredentialScopeTabs(visibleCredentials, isNopsAIAdmin),
    [isNopsAIAdmin, visibleCredentials]
  );

  return (
    <div id="system-credentials-section" className="credential-registry">
      <div className="credential-registry__shell">
        <CredentialDashboard
          canManage={canManage}
          canCreate={canCreateCredentials}
          loading={controller.loading}
          saving={controller.saving}
          scopeDescription={
            isNopsAIAdmin
              ? 'Manage encrypted, versioned credentials across teams and global resources.'
              : 'Manage encrypted, versioned credentials for authorized teams.'
          }
          summary={summary}
          onReload={() => void controller.loadCredentials()}
          onStartCreate={onStartCreate}
        />

        {controller.error && (
          <div className="credential-registry__error">{controller.error}</div>
        )}

        <CredentialCatalog
          groups={groups}
          isNopsAIAdmin={isNopsAIAdmin}
          namespaces={namespaces}
          scopeTabs={scopeTabs}
          selectedID={visibleSelected?.id}
          query={query}
          status={status}
          scope={scope}
          grouped={grouped}
          loading={controller.loading}
          teamPaths={teamPaths}
          onQueryChange={setQuery}
          onStatusChange={setStatus}
          onScopeChange={value => setScopeOverride(value)}
          onGroupedChange={setGrouped}
          onSelect={onSelectCredential}
        />
      </div>

      {visibleSelected && !controller.creating && (
        <CredentialDetail
          credential={visibleSelected}
          canManage={canManage}
          saving={controller.saving}
          teamPaths={teamPaths}
          rotationValue={controller.rotationValue}
          onRotationValueChange={controller.setRotationValue}
          onSubmitRotation={controller.submitRotation}
          onActivateVersion={version => void controller.activateVersion(version)}
          onDeleteVersion={version => void controller.deleteVersion(version)}
          onEnable={() => void controller.enableSelected()}
          onDisable={() => void controller.disableSelected()}
          onDelete={() => void controller.deleteSelected()}
          onClose={onCloseCredentialDetails}
        />
      )}

      {controller.creating && (
        <CredentialCreateForm
          allowSystemScope={isNopsAIAdmin}
          form={controller.form}
          saving={controller.saving}
          setForm={controller.setForm}
          teamPaths={teamPaths}
          teamPathsLoading={teamPathsLoading}
          onClose={() => controller.setCreating(false)}
          onSubmit={controller.submitCreate}
        />
      )}
    </div>
  );
}

function buildCredentialScopeTabs(credentials: CredentialRecord[], includeShared: boolean): CredentialScopeTab[] {
  const teamCount = credentials.filter(credential => parseCredentialReference(credential.reference).namespace === 'team').length;
  const sharedCount = credentials.length - teamCount;
  const tabs = [
    { value: 'all', label: 'All', count: credentials.length },
    { value: 'team', label: 'Teams', count: teamCount },
  ];
  if (includeShared) tabs.push({ value: 'shared', label: 'Shared', count: sharedCount });
  return tabs;
}

export default CredentialsPanel;
