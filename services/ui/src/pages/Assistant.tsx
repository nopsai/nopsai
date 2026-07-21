import { useMemo } from 'react';
import { useLocation } from 'react-router-dom';
import { AssistantPanel } from '../features/assistant/AssistantPanel.js';
import { normalizeAssistantPageContext, type AssistantPageContext } from '../features/assistant/pageContext.js';

export default function AssistantPage() {
  const location = useLocation();
  const pageContext = useMemo(
    () => normalizeAssistantPageContext(readAssistantPageContextState(location.state)),
    [location.state]
  );
  const startFresh = readAssistantStartFreshState(location.state);
  return (
    <div className="h-full min-h-[calc(100vh-4rem)] bg-[var(--bg-primary)]">
      <AssistantPanel variant="page" startFresh={startFresh} pageContext={pageContext} />
    </div>
  );
}

function readAssistantPageContextState(state: unknown): Partial<AssistantPageContext> | null {
  if (!state || typeof state !== 'object') return null;
  const value = (state as { assistantPageContext?: unknown }).assistantPageContext;
  return value && typeof value === 'object' ? value as Partial<AssistantPageContext> : null;
}

function readAssistantStartFreshState(state: unknown): boolean {
  return Boolean(state && typeof state === 'object' && (state as { assistantStartFresh?: unknown }).assistantStartFresh === true);
}
