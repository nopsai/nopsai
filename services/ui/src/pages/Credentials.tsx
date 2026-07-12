import CredentialsPanel from '../features/system/CredentialsPanel';

export default function CredentialsPage({ canManage, canManageAccess }: { canManage: boolean; canManageAccess: boolean }) {
  return (
    <div data-page="credentials" className="active p-6 space-y-6">
      <CredentialsPanel canManage={canManage} canManageAccess={canManageAccess} />
    </div>
  );
}
