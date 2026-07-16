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

export function useAssistantController({ autoload = true, startFresh = false }: { autoload?: boolean; startFresh?: boolean } = {}) {
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
  const [retrying, setRetrying] = useState(false);
  const [deletingConversationID, setDeletingConversationID] = useState('');
  const [copiedMessageID, setCopiedMessageID] = useState('');
  const [conversationCopied, setConversationCopied] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const activeConversationIDRef = useRef('');
  const copyResetRef = useRef<number | undefined>(undefined);
  const profileOptions = useMemo(() => selectableAssistantProfileNames(profiles), [profiles]);

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
        fetchAssistantLLMProfiles(),
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
  }, [activateConversation, startFresh]);

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
  }, [activateConversation, config?.default_docs_version, config?.enabled, selectedProfile]);

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
    if (!conversationID || sending) return;
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
  }, [activateConversation, activeConversation?.id, conversations, profileOptions, sending]);

  const submitMessage = useCallback(async () => {
    const content = draft.trim();
    if (!content || sending || config?.enabled !== true) return;
    setDraft('');
    setSending(true);
    setError(null);
    let messageConversationID = activeConversation?.id || '';
    try {
      let conversation = activeConversation;
      if (!conversation) {
        const createdConversation = await createAssistantConversation({
          selected_llm_profile: selectedProfile,
          docs_version: config?.default_docs_version || 'auto',
        });
        conversation = createdConversation;
        activateConversation(conversation);
        setConversations(current => [
          createdConversation,
          ...current.filter(item => item.id !== createdConversation.id),
        ]);
      }
      messageConversationID = conversation.id;
      setPendingMessage(buildPendingAssistantMessage(conversation.id, content));
      setSendingConversationID(conversation.id);
      const payload = await sendAssistantMessage({
        conversation_id: conversation.id,
        content,
        selected_llm_profile: selectedProfile,
      });
      setPendingMessage(current => (current?.conversation_id === conversation.id ? null : current));
      if (activeConversationIDRef.current === conversation.id) {
        activateConversation(payload.conversation);
      }
      setConversations(current => [
        payload.conversation,
        ...current.filter(item => item.id !== payload.conversation.id),
      ]);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to send message');
      setDraft(current => current || content);
      setPendingMessage(current => (current?.conversation_id === messageConversationID ? null : current));
    } finally {
      setSending(false);
      setSendingConversationID(current => (current === messageConversationID ? '' : current));
    }
  }, [activateConversation, activeConversation, config?.default_docs_version, config?.enabled, draft, selectedProfile, sending]);

  const retryMessage = useCallback(async (message?: AssistantMessage | null) => {
    if (sending || retrying || config?.enabled !== true || !activeConversation) return;
    const sourceMessage = message?.role === 'user' ? message : assistantLastUserMessage(activeConversation.messages);
    if (!sourceMessage) return;
    setPendingMessage(buildPendingAssistantMessage(activeConversation.id, sourceMessage.content));
    setSending(true);
    setSendingConversationID(activeConversation.id);
    setRetrying(true);
    setError(null);
    try {
      const payload = await sendAssistantMessage({
        conversation_id: activeConversation.id,
        content: sourceMessage.content,
        selected_llm_profile: selectedProfile,
      });
      setPendingMessage(current => (current?.conversation_id === activeConversation.id ? null : current));
      if (activeConversationIDRef.current === activeConversation.id) {
        activateConversation(payload.conversation);
      }
      setConversations(current => [
        payload.conversation,
        ...current.filter(item => item.id !== payload.conversation.id),
      ]);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to retry message');
      setPendingMessage(current => (current?.conversation_id === activeConversation.id ? null : current));
    } finally {
      setRetrying(false);
      setSending(false);
      setSendingConversationID(current => (current === activeConversation.id ? '' : current));
    }
  }, [activateConversation, activeConversation, config?.enabled, retrying, selectedProfile, sending]);

  const retryLastUserMessage = useCallback(async () => {
    await retryMessage();
  }, [retryMessage]);

  const activeMessages = useMemo(() => {
    const messages = activeConversation?.messages || [];
    if (!activeConversation || pendingMessage?.conversation_id !== activeConversation.id) {
      return messages;
    }
    return [...messages, pendingMessage];
  }, [activeConversation, activeConversation?.messages, pendingMessage]);
  const activeConversationSending = Boolean(activeConversation?.id && sendingConversationID === activeConversation.id);
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
    activeConversationSending,
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
