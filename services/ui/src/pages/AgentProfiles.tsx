import AgentProfilesPanel from '../features/system/AgentProfilesPanel';

export default function AgentProfilesPage({ canManage }: { canManage: boolean }) {
  return (
    <div data-page="agent-profiles" className="active p-6 space-y-6">
      <AgentProfilesPanel canManage={canManage} />
    </div>
  );
}
