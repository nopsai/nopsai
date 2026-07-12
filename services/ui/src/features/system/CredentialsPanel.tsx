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
  parseCredentialReference,
  recentlyUpdatedCredentials,
  type CredentialRecord,
} from './credentials/model';
import { useCredentials } from './credentials/useCredentials';

type CredentialsController = ReturnType<typeof useCredentials>;

type CredentialsPanelBodyProps = {
  canManage: boolean;
  controller: CredentialsController;
  linkedCredentialRef: string;
  teamPaths: string[];
  teamPathsLoading: boolean;
  onCloseCredentialDetails: () => void;
  onSelectCredential: (credential: CredentialRecord) => void;
  onStartCreate: () => void;
};

function CredentialsPanel({ canManage }: { canManage: boolean }) {
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
    const match = credentials.find(credential => credential.reference === linkedCredentialRef);
    if (match && selected?.id !== match.id) void selectControllerCredential(match);
  }, [
    credentials,
    creating,
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
    const next = new URLSearchParams(searchParams);
    next.delete('credential');
    setSearchParams(next, { replace: true });
  };

  return (
    <CredentialsPanelBody
      canManage={canManage}
      controller={controller}
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
  const [compact, setCompact] = useState(false);
  const [activeGroupKey, setActiveGroupKey] = useState<string | null>(null);
  const linkedCredential = useMemo(
    () => controller.credentials.find(credential => credential.reference === linkedCredentialRef),
    [controller.credentials, linkedCredentialRef]
  );
  const scope = scopeOverride ?? (linkedCredential ? parseCredentialReference(linkedCredential.reference).namespace : 'all');
  const summary = useMemo(() => credentialSummary(controller.credentials), [controller.credentials]);
  const namespaces = useMemo(() => credentialNamespaces(controller.credentials), [controller.credentials]);
  const filteredCredentials = useMemo(
    () => filterCredentials(controller.credentials, query, status, scope),
    [controller.credentials, query, scope, status]
  );
  const groups = useMemo(
    () => credentialCatalogGroups(filteredCredentials, teamPaths),
    [filteredCredentials, teamPaths]
  );
  const selectedGroupKey = activeGroupKey && groups.some(group => group.key === activeGroupKey) ? activeGroupKey : null;
  const recentCredentials = useMemo(
    () => recentlyUpdatedCredentials(controller.credentials, 5),
    [controller.credentials]
  );
  const scopeTabs = useMemo(
    () => buildCredentialScopeTabs(controller.credentials, namespaces),
    [controller.credentials, namespaces]
  );

  return (
    <div id="system-credentials-section" className="space-y-6 pb-24">
      <CredentialDashboard
        canManage={canManage}
        loading={controller.loading}
        saving={controller.saving}
        summary={summary}
        onReload={() => void controller.loadCredentials()}
        onStartCreate={onStartCreate}
      />

      {controller.error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/5 px-4 py-3 text-sm text-red-500">{controller.error}</div>
      )}

      <CredentialCatalog
        groups={groups}
        namespaces={namespaces}
        recentCredentials={recentCredentials}
        scopeTabs={scopeTabs}
        selectedID={controller.selected?.id}
        query={query}
        status={status}
        scope={scope}
        compact={compact}
        activeGroupKey={selectedGroupKey}
        loading={controller.loading}
        teamPaths={teamPaths}
        onQueryChange={setQuery}
        onStatusChange={setStatus}
        onScopeChange={value => {
          setScopeOverride(value);
          setActiveGroupKey(null);
        }}
        onCompactChange={setCompact}
        onGroupChange={setActiveGroupKey}
        onSelect={onSelectCredential}
      />

      {controller.selected && !controller.creating && (
        <CredentialDetail
          credential={controller.selected}
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

function buildCredentialScopeTabs(credentials: CredentialRecord[], namespaces: string[]): CredentialScopeTab[] {
  const teamCount = credentials.filter(credential => parseCredentialReference(credential.reference).namespace === 'team').length;
  const countForNamespace = (namespace: string) =>
    credentials.filter(credential => parseCredentialReference(credential.reference).namespace === namespace).length;
  const tabs: CredentialScopeTab[] = [
    { value: 'all', label: 'All', count: credentials.length },
  ];
  if (teamCount > 0) tabs.push({ value: 'team', label: 'Teams', count: teamCount });
  namespaces
    .filter(namespace => namespace !== 'team')
    .forEach(namespace => {
      tabs.push({ value: namespace, label: namespace, count: countForNamespace(namespace) });
    });
  return tabs;
}

export default CredentialsPanel;
