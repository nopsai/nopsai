import { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate, type NavigateFunction } from 'react-router-dom';
import { fetchAssistantConfig } from '../features/assistant/api.js';
import { AssistantPanel } from '../features/assistant/AssistantPanel.js';
import { buildAssistantPageContext, type AssistantPageContext } from '../features/assistant/pageContext.js';
import { ObjectIcon } from './ObjectIcon.js';

export function AssistantDock() {
  const location = useLocation();
  const navigate = useNavigate();
  const isAssistantPage = location.pathname === '/assistant' || location.pathname.startsWith('/assistant/');
  const pageContext = useMemo(
    () => buildAssistantPageContext(location.pathname, location.search),
    [location.pathname, location.search]
  );

  // Unmounting the dock on the full-page assistant route resets its local state
  // without scheduling a second render from an effect.
  if (isAssistantPage) return null;

  return <AssistantDockContent navigate={navigate} pageContext={pageContext} />;
}

function AssistantDockContent({ navigate, pageContext }: { navigate: NavigateFunction; pageContext: AssistantPageContext }) {
  const [open, setOpen] = useState(false);
  const [enabled, setEnabled] = useState(false);

  useEffect(() => {
    let active = true;
    fetchAssistantConfig()
      .then(config => {
        if (active) setEnabled(config.enabled);
      })
      .catch(() => {
        if (active) setEnabled(false);
      });
    return () => {
      active = false;
    };
  }, []);

  if (!enabled) return null;

  return (
    <>
      <button
        type="button"
        className="fixed bottom-5 right-5 z-[90] inline-flex h-12 w-12 items-center justify-center rounded-md bg-[var(--border-accent)] text-white shadow-lg transition hover:brightness-95 focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)]"
        onClick={() => setOpen(true)}
        aria-label="Open Nopsai AI Assistant"
      >
        <ObjectIcon type="assistant" className="h-5 w-5" />
      </button>

      {open && (
        <div className="fixed inset-0 z-[95]">
          <button
            type="button"
            className="absolute inset-0 h-full w-full cursor-default bg-black/20"
            aria-label="Close assistant overlay"
            onClick={() => setOpen(false)}
          />
          <aside className="absolute right-0 top-0 h-full w-full max-w-[420px] border-l border-[var(--border-primary)] bg-[var(--bg-primary)] shadow-xl">
            <AssistantPanel
              variant="dock"
              startFresh
              pageContext={pageContext}
              onClose={() => setOpen(false)}
              onExpand={() => {
                setOpen(false);
                navigate('/assistant', { state: { assistantPageContext: pageContext, assistantStartFresh: true } });
              }}
            />
          </aside>
        </div>
      )}
    </>
  );
}

export default AssistantDock;
