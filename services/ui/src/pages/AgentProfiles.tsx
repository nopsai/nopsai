import AgentProfilesPanel from '../features/system/AgentProfilesPanel';

export default function AgentProfilesPage({ canManage }: { canManage: boolean }) {
  return (
    <div data-page="agent-profiles" className="active h-full flex flex-col">
      <AgentProfilesPanel canManage={canManage} />
    </div>
  );
}
