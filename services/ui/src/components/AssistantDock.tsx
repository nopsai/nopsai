import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AssistantPanel } from '../features/assistant/AssistantPanel.js';
import { ObjectIcon } from './ObjectIcon.js';

export function AssistantDock() {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();

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
          <aside className="absolute right-0 top-0 h-full w-full max-w-[460px] border-l border-[var(--border-primary)] bg-[var(--bg-primary)] shadow-xl">
            <AssistantPanel
              variant="dock"
              onClose={() => setOpen(false)}
              onExpand={() => {
                setOpen(false);
                navigate('/assistant');
              }}
            />
          </aside>
        </div>
      )}
    </>
  );
}

export default AssistantDock;
