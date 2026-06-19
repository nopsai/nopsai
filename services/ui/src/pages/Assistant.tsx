import { AssistantPanel } from '../features/assistant/AssistantPanel.js';

export default function AssistantPage() {
  return (
    <div className="h-full min-h-[calc(100vh-4rem)] bg-[var(--bg-primary)]">
      <AssistantPanel variant="page" />
    </div>
  );
}
