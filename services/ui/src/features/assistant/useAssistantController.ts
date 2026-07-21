import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { copyTextToClipboard } from '../../lib/clipboard.js';
import {
  deleteAssistantConversation,
  fetchAssistantConfig,
  createAssistantConversation,
  fetchAssistantConversation,
  fetchAssistantConversations,
  fetchAssistantLLMProfiles,
  sendAssistantMessage,
} from './api.js';
import {
  assistantConversationClipboardText,
  assistantLastUserMessage,
  assistantMessageClipboardText,
  emptyAssistantMessageUsage,
  type AssistantConfig,
  type AssistantConversation,
  type AssistantLLMProfile,
  type AssistantMessage,
} from './model.js';
import {
  assistantPageContextIsEmpty,
  assistantPageContextScope,
  normalizeAssistantPageContext,
  type AssistantPageContext,
} from './pageContext.js';

export function useAssistantController({
  autoload = true,
  startFresh = false,
  pageContext = null,
}: {
  autoload?: boolean;
  startFresh?: boolean;
  pageContext?: Partial<AssistantPageContext> | null;
} = {}) {
  const [config, setConfig] = useState<AssistantConfig | null>(null);
  const [conversations, setConversations] = useState<AssistantConversation[]>([]);
  const [activeConversation, setActiveConversation] = useState<AssistantConversation | null>(null);
  const [profiles, setProfiles] = useState<AssistantLLMProfile[]>([]);
  const [pendingMessage, setPendingMessage] = useState<AssistantMessage | null>(null);
  const [selectedProfile, setSelectedProfile] = useState('');
  const [draft, setDraft] = useState('');
  const [loading, setLoading] = useState(autoload);
  const [sending, setSending] = useState(false);
  const [sendingConversationID, setSendingConversationID] = useState('');
  const [sendingStartedAt, setSendingStartedAt] = useState(0);
  const [retrying, setRetrying] = useState(false);
  const [deletingConversationID, setDeletingConversationID] = useState('');
  const [copiedMessageID, setCopiedMessageID] = useState('');
  const [conversationCopied, setConversationCopied] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const activeConversationIDRef = useRef('');
  const copyResetRef = useRef<number | undefined>(undefined);
  const profileOptions = useMemo(() => selectableAssistantProfileNames(profiles), [profiles]);
  const normalizedPageContext = useMemo(() => normalizeAssistantPageContext(pageContext), [pageContext]);
  const pageContextScope = useMemo(() => assistantPageContextScope(normalizedPageContext), [normalizedPageContext]);
  const sendPageContext = useMemo(
    () => assistantPageContextIsEmpty(normalizedPageContext) ? null : normalizedPageContext,
    [normalizedPageContext]
  );

  const activateConversation = useCallback((conversation: AssistantConversation | null) => {
    activeConversationIDRef.current = conversation?.id || '';
    setActiveConversation(conversation);
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const assistantConfig = await fetchAssistantConfig();
      setConfig(assistantConfig);
      if (!assistantConfig.enabled) {
        setConversations([]);
        activateConversation(null);
        setProfiles([]);
        setPendingMessage(null);
        setSelectedProfile('');
        return;
      }
      const [conversationPayload, profilePayload] = await Promise.all([
        fetchAssistantConversations(),
        fetchAssistantLLMProfiles(pageContextScope),
      ]);
      const selectable = selectableAssistantProfileNames(profilePayload.profiles);
      setConversations(conversationPayload.conversations);
      setProfiles(profilePayload.profiles);
      setSelectedProfile(current => selectAssistantProfile(current, profilePayload.default_profile, selectable));
      if (startFresh) {
        setPendingMessage(null);
        activateConversation(null);
      } else if (conversationPayload.conversations[0]) {
        const conversation = await fetchAssistantConversation(conversationPayload.conversations[0].id);
        activateConversation(conversation);
        if (conversation.selected_llm_profile && selectable.includes(conversation.selected_llm_profile)) {
          setSelectedProfile(conversation.selected_llm_profile);
        }
      } else {
        activateConversation(null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load assistant');
    } finally {
      setLoading(false);
    }
  }, [activateConversation, pageContextScope, startFresh]);

  useEffect(() => {
    if (!autoload) return;
    void load();
  }, [autoload, load]);

  useEffect(() => () => {
    if (copyResetRef.current) window.clearTimeout(copyResetRef.current);
  }, []);

  const selectConversation = useCallback(async (conversationID: string) => {
    setLoading(true);
    setError(null);
    try {
      const conversation = await fetchAssistantConversation(conversationID);
      activateConversation(conversation);
      if (conversation.selected_llm_profile && profileOptions.includes(conversation.selected_llm_profile)) {
        setSelectedProfile(conversation.selected_llm_profile);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load conversation');
    } finally {
      setLoading(false);
    }
  }, [activateConversation, profileOptions]);

  const startConversation = useCallback(async () => {
    if (config?.enabled !== true) return null;
    setSending(true);
    setError(null);
    try {
      const conversation = await createAssistantConversation({
        selected_llm_profile: selectedProfile,
        docs_version: config?.default_docs_version || 'auto',
        scope: pageContextScope,
      });
      activateConversation(conversation);
      setConversations(current => [conversation, ...current.filter(item => item.id !== conversation.id)]);
      return conversation;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to create conversation');
      return null;
    } finally {
      setSending(false);
    }
  }, [activateConversation, config?.default_docs_version, config?.enabled, pageContextScope, selectedProfile]);

  const finishCopyFeedback = useCallback((messageID = '') => {
    if (copyResetRef.current) window.clearTimeout(copyResetRef.current);
    copyResetRef.current = window.setTimeout(() => {
      setCopiedMessageID(current => (messageID && current === messageID ? '' : current));
      setConversationCopied(false);
    }, 1600);
  }, []);

  const copyMessage = useCallback(async (message: AssistantMessage) => {
    const text = assistantMessageClipboardText(message);
    if (!text) return;
    setError(null);
    try {
      await copyTextToClipboard(text);
      setConversationCopied(false);
      setCopiedMessageID(message.id);
      finishCopyFeedback(message.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to copy message');
    }
  }, [finishCopyFeedback]);

  const copyConversation = useCallback(async () => {
    const text = assistantConversationClipboardText(activeConversation);
    if (!text) return;
    setError(null);
    try {
      await copyTextToClipboard(text);
      setCopiedMessageID('');
      setConversationCopied(true);
      finishCopyFeedback();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to copy conversation');
    }
  }, [activeConversation, finishCopyFeedback]);

  const deleteConversation = useCallback(async (conversationID = activeConversation?.id || '') => {
    if (!conversationID || conversationID === sendingConversationID) return;
    const remaining = conversations.filter(conversation => conversation.id !== conversationID);
    setDeletingConversationID(conversationID);
    setError(null);
    try {
      await deleteAssistantConversation(conversationID);
      setConversations(remaining);
      setPendingMessage(current => (current?.conversation_id === conversationID ? null : current));
      setSendingConversationID(current => (current === conversationID ? '' : current));
      if (activeConversation?.id === conversationID) {
        if (remaining[0]) {
          const nextConversation = await fetchAssistantConversation(remaining[0].id);
          activateConversation(nextConversation);
          if (nextConversation.selected_llm_profile && profileOptions.includes(nextConversation.selected_llm_profile)) {
            setSelectedProfile(nextConversation.selected_llm_profile);
          }
        } else {
          activateConversation(null);
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete conversation');
    } finally {
      setDeletingConversationID('');
    }
  }, [activateConversation, activeConversation?.id, conversations, profileOptions, sendingConversationID]);

  const submitMessage = useCallback(async () => {
    const content = draft.trim();
    if (!content || sending || config?.enabled !== true) return;
    setDraft('');
    setSending(true);
    setError(null);
    let messageConversationID = activeConversation?.id || '';
    let turnStartedAt = Date.now();
    try {
      let conversation = activeConversation;
      if (!conversation) {
        const createdConversation = await createAssistantConversation({
          selected_llm_profile: selectedProfile,
          docs_version: config?.default_docs_version || 'auto',
          scope: pageContextScope,
        });
        conversation = createdConversation;
        activateConversation(conversation);
        setConversations(current => [
          createdConversation,
          ...current.filter(item => item.id !== createdConversation.id),
        ]);
      }
      messageConversationID = conversation.id;
      turnStartedAt = Date.now();
      setPendingMessage(buildPendingAssistantMessage(conversation.id, content));
      setSendingConversationID(conversation.id);
      setSendingStartedAt(turnStartedAt);
      const sendInput: Parameters<typeof sendAssistantMessage>[0] = {
        conversation_id: conversation.id,
        content,
        selected_llm_profile: selectedProfile,
      };
      if (sendPageContext) sendInput.page_context = sendPageContext;
      const payload = await sendAssistantMessage(sendInput);
      setPendingMessage(current => (current?.conversation_id === conversation.id ? null : current));
      if (activeConversationIDRef.current === conversation.id) {
        activateConversation(payload.conversation);
      }
      setConversations(current => [
        payload.conversation,
        ...current.filter(item => item.id !== payload.conversation.id),
      ]);
    } catch (err) {
      const recoveredConversation = assistantSendErrorMayHavePersisted(err)
        ? await recoverAssistantConversationAfterSendError(messageConversationID, content, turnStartedAt)
        : null;
      if (recoveredConversation) {
        setPendingMessage(current => (current?.conversation_id === messageConversationID ? null : current));
        if (activeConversationIDRef.current === messageConversationID) {
          activateConversation(recoveredConversation);
        }
        setConversations(current => [
          recoveredConversation,
          ...current.filter(item => item.id !== recoveredConversation.id),
        ]);
        return;
      }
      setError(err instanceof Error ? err.message : 'Unable to send message');
      setDraft(current => current || content);
      setPendingMessage(current => (current?.conversation_id === messageConversationID ? null : current));
    } finally {
      setSending(false);
      setSendingConversationID(current => (current === messageConversationID ? '' : current));
      setSendingStartedAt(0);
    }
  }, [activateConversation, activeConversation, config?.default_docs_version, config?.enabled, draft, pageContextScope, selectedProfile, sendPageContext, sending]);

  const retryMessage = useCallback(async (message?: AssistantMessage | null) => {
    if (sending || retrying || config?.enabled !== true || !activeConversation) return;
    const sourceMessage = message?.role === 'user' ? message : assistantLastUserMessage(activeConversation.messages);
    if (!sourceMessage) return;
    setPendingMessage(buildPendingAssistantMessage(activeConversation.id, sourceMessage.content));
    setSending(true);
    setSendingConversationID(activeConversation.id);
    const turnStartedAt = Date.now();
    setSendingStartedAt(turnStartedAt);
    setRetrying(true);
    setError(null);
    try {
      const sendInput: Parameters<typeof sendAssistantMessage>[0] = {
        conversation_id: activeConversation.id,
        content: sourceMessage.content,
        selected_llm_profile: selectedProfile,
      };
      if (sendPageContext) sendInput.page_context = sendPageContext;
      const payload = await sendAssistantMessage(sendInput);
      setPendingMessage(current => (current?.conversation_id === activeConversation.id ? null : current));
      if (activeConversationIDRef.current === activeConversation.id) {
        activateConversation(payload.conversation);
      }
      setConversations(current => [
        payload.conversation,
        ...current.filter(item => item.id !== payload.conversation.id),
      ]);
    } catch (err) {
      const recoveredConversation = assistantSendErrorMayHavePersisted(err)
        ? await recoverAssistantConversationAfterSendError(activeConversation.id, sourceMessage.content, turnStartedAt)
        : null;
      if (recoveredConversation) {
        setPendingMessage(current => (current?.conversation_id === activeConversation.id ? null : current));
        if (activeConversationIDRef.current === activeConversation.id) {
          activateConversation(recoveredConversation);
        }
        setConversations(current => [
          recoveredConversation,
          ...current.filter(item => item.id !== recoveredConversation.id),
        ]);
        return;
      }
      setError(err instanceof Error ? err.message : 'Unable to retry message');
      setPendingMessage(current => (current?.conversation_id === activeConversation.id ? null : current));
    } finally {
      setRetrying(false);
      setSending(false);
      setSendingConversationID(current => (current === activeConversation.id ? '' : current));
      setSendingStartedAt(0);
    }
  }, [activateConversation, activeConversation, config?.enabled, retrying, selectedProfile, sendPageContext, sending]);

  const retryLastUserMessage = useCallback(async () => {
    await retryMessage();
  }, [retryMessage]);

  const activeMessages = useMemo(() => {
    const messages = activeConversation?.messages || [];
    if (!activeConversation || pendingMessage?.conversation_id !== activeConversation.id) {
      return messages;
    }
    if (messages.some(message => assistantMessageMatchesPending(message, pendingMessage))) {
      return messages;
    }
    return [...messages, pendingMessage];
  }, [activeConversation, pendingMessage]);
  const activeConversationSending = Boolean(activeConversation?.id && sendingConversationID === activeConversation.id);
  const activeConversationSendingStartedAt = activeConversationSending ? sendingStartedAt : 0;
  const canRetry = Boolean(activeConversation && assistantLastUserMessage(activeConversation.messages));

  return {
    conversations,
    activeConversation,
    activeMessages,
    profiles,
    profileOptions,
    selectedProfile,
    setSelectedProfile,
    draft,
    setDraft,
    loading,
    sending,
    sendingConversationID,
    activeConversationSending,
    activeConversationSendingStartedAt,
    retrying,
    deletingConversationID,
    copiedMessageID,
    conversationCopied,
    error,
    config,
    enabled: config?.enabled === true,
    canRetry,
    load,
    selectConversation,
    startConversation,
    deleteConversation,
    retryMessage,
    retryLastUserMessage,
    copyMessage,
    copyConversation,
    submitMessage,
  };
}

const assistantSendRecoveryAttempts = 12;
const assistantSendRecoveryDelayMS = 2500;
const assistantSendRecoveryClockSkewMS = 30000;

function assistantSendErrorMayHavePersisted(err: unknown): boolean {
  const message = err instanceof Error ? err.message : String(err);
  return /\b(502|503|504|gateway|timeout|timed out|network|failed to fetch|load failed|proxy|abort|aborted|connection|econnreset)\b/i.test(message);
}

async function recoverAssistantConversationAfterSendError(
  conversationID: string,
  content: string,
  turnStartedAt: number,
): Promise<AssistantConversation | null> {
  if (!conversationID || !content.trim()) return null;
  for (let attempt = 0; attempt < assistantSendRecoveryAttempts; attempt += 1) {
    if (attempt > 0) await delay(assistantSendRecoveryDelayMS);
    try {
      const conversation = await fetchAssistantConversation(conversationID);
      if (assistantConversationHasCompletedTurn(conversation, content, turnStartedAt)) {
        return conversation;
      }
    } catch {
      // Keep polling briefly; the original POST may have timed out while the turn is still being saved.
    }
  }
  return null;
}

function assistantConversationHasCompletedTurn(
  conversation: AssistantConversation,
  content: string,
  turnStartedAt: number,
): boolean {
  const normalizedContent = content.trim();
  for (let index = conversation.messages.length - 1; index >= 0; index -= 1) {
    const message = conversation.messages[index];
    if (message.role !== 'user' || message.content.trim() !== normalizedContent) continue;
    if (!assistantMessageIsRecoverableNew(message, turnStartedAt)) continue;
    return conversation.messages.slice(index + 1).some(reply => (
      reply.role === 'assistant'
      && reply.content.trim().length > 0
      && assistantMessageIsRecoverableNew(reply, turnStartedAt)
    ));
  }
  return false;
}

function assistantMessageIsRecoverableNew(message: AssistantMessage, turnStartedAt: number): boolean {
  const messageTime = Date.parse(message.created_at);
  if (!Number.isFinite(messageTime)) return true;
  return messageTime >= turnStartedAt - assistantSendRecoveryClockSkewMS;
}

function delay(ms: number): Promise<void> {
  return new Promise(resolve => {
    window.setTimeout(resolve, ms);
  });
}

function buildPendingAssistantMessage(conversationID: string, content: string): AssistantMessage {
  return {
    id: `pending-${Date.now()}`,
    conversation_id: conversationID,
    role: 'user',
    content,
    tool_calls: [],
    usage: emptyAssistantMessageUsage,
    created_at: new Date().toISOString(),
  };
}

function assistantMessageMatchesPending(message: AssistantMessage, pending: AssistantMessage): boolean {
  if (message.id === pending.id) return true;
  if (message.conversation_id !== pending.conversation_id || message.role !== pending.role) return false;
  if (message.content.trim() !== pending.content.trim()) return false;
  const messageTime = Date.parse(message.created_at);
  const pendingTime = Date.parse(pending.created_at);
  if (!Number.isFinite(messageTime) || !Number.isFinite(pendingTime)) return true;
  return messageTime >= pendingTime - 30_000;
}

function selectableAssistantProfileNames(profiles: AssistantLLMProfile[]) {
  return profiles
    .filter(profile => profile.status === 'valid' && profile.allowed_in_scope !== false)
    .map(profile => profile.name);
}

function selectAssistantProfile(current: string, defaultProfile: string, selectable: string[]) {
  if (current && selectable.includes(current)) return current;
  if (defaultProfile && selectable.includes(defaultProfile)) return defaultProfile;
  return selectable[0] || '';
}
