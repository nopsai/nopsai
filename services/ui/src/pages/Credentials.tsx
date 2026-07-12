import CredentialsPanel from '../features/system/CredentialsPanel';

export default function CredentialsPage({
  canManage,
  isNopsAIAdmin,
}: {
  canManage: boolean;
  isNopsAIAdmin: boolean;
}) {
  return (
    <div data-page="credentials" className="active min-h-full">
      <CredentialsPanel canManage={canManage} isNopsAIAdmin={isNopsAIAdmin} />
    </div>
  );
}
