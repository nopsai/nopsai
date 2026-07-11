import LLMProfilesPanel from '../features/system/LLMProfilesPanel';

export default function LLMProfilesPage({ canManage }: { canManage: boolean }) {
  return (
    <div data-page="llm-profiles" className="active p-6 space-y-6">
      <LLMProfilesPanel canManage={canManage} />
    </div>
  );
}
