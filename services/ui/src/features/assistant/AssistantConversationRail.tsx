import { Loader2, Plus, Trash2 } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon.js';
import { assistantConversationTitle, groupAssistantConversations } from './conversationGroups.js';
import type { AssistantConversation } from './model.js';

export function AssistantConversationRail({
  conversations,
  activeConversation,
  enabled,
  loading,
  sending,
  sendingConversationID,
  deletingConversationID,
  onStartConversation,
  onSelectConversation,
  onDeleteConversation,
}: {
  conversations: AssistantConversation[];
  activeConversation: AssistantConversation | null;
  enabled: boolean;
  loading: boolean;
  sending: boolean;
  sendingConversationID: string;
  deletingConversationID: string;
  onStartConversation: () => void;
  onSelectConversation: (conversationID: string) => void;
  onDeleteConversation: (conversationID: string) => void;
}) {
  const groups = groupAssistantConversations(conversations);
  return (
    <aside
      className="hidden w-[280px] shrink-0 flex-col border-r border-[var(--border-primary)] bg-[var(--bg-secondary)] sm:flex"
      aria-label="Assistant conversations"
    >
      <div className="flex flex-col gap-4 p-4">
        <div className="flex items-center gap-2 px-1 text-sm font-semibold text-[var(--text-primary)]">
          <ObjectIcon type="assistant" className="h-4 w-4" />
          <span>NopsAI</span>
        </div>
        <button
          type="button"
          className="group inline-flex w-full items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm font-medium text-[var(--text-primary)] shadow-sm transition hover:border-[var(--border-secondary)] hover:bg-[var(--bg-tertiary)] disabled:cursor-not-allowed disabled:opacity-60"
          onClick={onStartConversation}
          disabled={!enabled || loading || sending}
        >
          <Plus className="h-4 w-4 text-[var(--text-secondary)] transition-colors group-hover:text-[var(--text-primary)]" aria-hidden="true" />
          New conversation
        </button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
        {conversations.length === 0 && (
          <p className="rounded-lg border border-dashed border-[var(--border-primary)] px-3 py-2 text-sm text-[var(--text-secondary)]">
            No conversations yet.
          </p>
        )}
        {groups.map(group => (
          <section key={group.label}>
            <h3 className="mb-2 mt-4 px-2 text-[11px] font-semibold uppercase tracking-wider text-[var(--text-secondary)]">
              {group.label}
            </h3>
            <ul className="flex flex-col gap-1">
              {group.conversations.map(conversation => (
                <AssistantConversationRow
                  key={conversation.id}
                  conversation={conversation}
                  active={activeConversation?.id === conversation.id}
                  running={sendingConversationID === conversation.id}
                  deleting={deletingConversationID === conversation.id}
                  onSelect={() => onSelectConversation(conversation.id)}
                  onDelete={() => onDeleteConversation(conversation.id)}
                />
              ))}
            </ul>
          </section>
        ))}
      </div>

    </aside>
  );
}

function AssistantConversationRow({
  conversation,
  active,
  running,
  deleting,
  onSelect,
  onDelete,
}: {
  conversation: AssistantConversation;
  active: boolean;
  running: boolean;
  deleting: boolean;
  onSelect: () => void;
  onDelete: () => void;
}) {
  const title = assistantConversationTitle(conversation);
  return (
    <li>
      <div
        className={`group flex items-center justify-between gap-1 rounded-lg px-3 py-2 transition-colors ${active ? 'bg-[var(--bg-active)]' : 'hover:bg-[var(--bg-tertiary)]'}`}
      >
        <button
          type="button"
          className={`min-w-0 flex-1 truncate text-left text-sm transition-colors ${active ? 'font-medium text-[var(--text-primary)]' : 'text-[var(--text-secondary)] group-hover:text-[var(--text-primary)]'}`}
          onClick={onSelect}
          title={title}
        >
          {title}
        </button>
        <button
          type="button"
          className={`inline-flex h-6 w-6 shrink-0 items-center justify-center rounded transition ${active ? '' : 'opacity-0 focus-visible:opacity-100 group-hover:opacity-100'} text-[var(--text-secondary)] hover:bg-rose-500/10 hover:text-rose-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-rose-400`}
          onClick={onDelete}
          disabled={running || deleting}
          aria-label={`Delete conversation ${title}`}
          title={running ? 'Assistant turn in progress' : 'Delete'}
        >
          {deleting
            ? <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
            : <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />}
        </button>
      </div>
    </li>
  );
}
