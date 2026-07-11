import { Plus, RefreshCw } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { CredentialCatalog } from './credentials/CredentialCatalog';
import { CredentialCreateForm } from './credentials/CredentialCreateForm';
import { CredentialDetail } from './credentials/CredentialDetail';
import {
  credentialNamespaces,
  credentialSummary,
  filterCredentials,
  teamCredentials,
  parseCredentialReference,
  type CredentialRecord,
} from './credentials/model';
import { useCredentials } from './credentials/useCredentials';

type CredentialsController = ReturnType<typeof useCredentials>;

type CredentialsPanelBodyProps = {
  canManage: boolean;
  controller: CredentialsController;
  linkedCredentialRef: string;
  onCloseCredentialDetails: () => void;
  onSelectCredential: (credential: CredentialRecord) => void;
  onStartCreate: () => void;
};

function CredentialsPanel({ canManage }: { canManage: boolean }) {
  const controller = useCredentials({ canManage });
  const [searchParams, setSearchParams] = useSearchParams();
  const linkedCredentialRef = (searchParams.get('credential') || '').trim();

  useEffect(() => {
    if (!linkedCredentialRef || controller.loading || controller.creating) return;
    const match = controller.credentials.find(credential => credential.reference === linkedCredentialRef);
    if (match && controller.selected?.id !== match.id) void controller.selectCredential(match);
  }, [controller.credentials, controller.creating, controller.loading, controller.selected?.id, controller.selectCredential, linkedCredentialRef]);

  const selectCredential = (credential: CredentialRecord) => {
    const next = new URLSearchParams(searchParams);
    next.set('credential', credential.reference);
    setSearchParams(next, { replace: true });
    void controller.selectCredential(credential);
  };

  const closeCredentialDetails = () => {
    const next = new URLSearchParams(searchParams);
    next.delete('credential');
    setSearchParams(next, { replace: true });
    controller.closeDetails();
  };

  const startCreate = () => {
    const next = new URLSearchParams(searchParams);
    next.delete('credential');
    setSearchParams(next, { replace: true });
    controller.startCreate();
  };

  // A new linked credential gets a fresh filter view. This models URL state as
  // render input instead of copying it into state from an effect.
  return (
    <CredentialsPanelBody
      key={linkedCredentialRef}
      canManage={canManage}
      controller={controller}
      linkedCredentialRef={linkedCredentialRef}
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
  onCloseCredentialDetails,
  onSelectCredential,
  onStartCreate,
}: CredentialsPanelBodyProps) {
  const [query, setQuery] = useState(() => linkedCredentialRef);
  const [status, setStatus] = useState('all');
  const [namespaceOverride, setNamespaceOverride] = useState<string | null>(null);
  const linkedCredential = useMemo(
    () => controller.credentials.find(credential => credential.reference === linkedCredentialRef),
    [controller.credentials, linkedCredentialRef]
  );
  const namespace = namespaceOverride ?? (linkedCredential ? parseCredentialReference(linkedCredential.reference).namespace : 'all');
  const summary = useMemo(() => credentialSummary(controller.credentials), [controller.credentials]);
  const namespaces = useMemo(() => credentialNamespaces(controller.credentials), [controller.credentials]);
  const teams = useMemo(
    () => teamCredentials(filterCredentials(controller.credentials, query, status, namespace)),
    [controller.credentials, namespace, query, status]
  );
  const showSidePanel = controller.creating || Boolean(controller.selected);

  return (
    <div id="system-credentials-section" className="space-y-6 pb-24">
      <section className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 className="text-lg font-semibold text-[var(--text-primary)]">Credential registry</h2>
            <p className="text-sm text-[var(--text-secondary)]">
              Manage encrypted, versioned credentials by integration. Values remain write-only.
            </p>
          </div>
          <div className="flex items-center gap-2">
            {!canManage && <span className="runner-pill runner-pill--muted">Read-only</span>}
            <button
              type="button"
              className="glass-button-ghost"
              onClick={() => void controller.loadCredentials()}
              disabled={controller.loading || controller.saving}
            >
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
              Reload
            </button>
            {canManage && (
              <button type="button" className="glass-button-primary" onClick={onStartCreate} disabled={controller.saving}>
                <Plus className="h-4 w-4" aria-hidden="true" />
                New credential
              </button>
            )}
          </div>
        </div>

        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          {[
            ['Total', summary.total],
            ['Active', summary.active],
            ['Disabled', summary.disabled],
            ['Pending value', summary.pending],
          ].map(([label, value]) => (
            <div key={label} className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-3">
              <p className="text-xs text-[var(--text-secondary)]">{label}</p>
              <p className="text-xl font-semibold text-[var(--text-primary)]">{value}</p>
            </div>
          ))}
        </div>

        {controller.error && (
          <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-500">{controller.error}</div>
        )}
      </section>

      <div className={`grid items-start gap-6 ${showSidePanel ? 'xl:grid-cols-[minmax(0,1.25fr)_minmax(390px,0.75fr)]' : ''}`}>
        <CredentialCatalog
          teams={teams}
          namespaces={namespaces}
          selectedID={controller.selected?.id}
          query={query}
          status={status}
          namespace={namespace}
          loading={controller.loading}
          onQueryChange={setQuery}
          onStatusChange={setStatus}
          onNamespaceChange={setNamespaceOverride}
          onSelect={onSelectCredential}
        />

        {controller.creating && (
          <CredentialCreateForm
            form={controller.form}
            saving={controller.saving}
            setForm={controller.setForm}
            onClose={() => controller.setCreating(false)}
            onSubmit={controller.submitCreate}
          />
        )}

        {controller.selected && !controller.creating && (
          <CredentialDetail
            credential={controller.selected}
            canManage={canManage}
            saving={controller.saving}
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
      </div>
    </div>
  );
}

export default CredentialsPanel;
