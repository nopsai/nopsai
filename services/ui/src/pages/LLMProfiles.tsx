import LLMProfilesPanel from '../features/system/LLMProfilesPanel';

export default function LLMProfilesPage({ canManage }: { canManage: boolean }) {
  return (
    <div data-page="llm-profiles" className="active h-full flex flex-col">
      <LLMProfilesPanel canManage={canManage} />
    </div>
  );
}
