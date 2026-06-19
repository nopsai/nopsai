import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  fetchAssistantConfig,
  createAssistantConversation,
  fetchAssistantConversation,
  fetchAssistantConversations,
  fetchAssistantLLMProfiles,
  sendAssistantMessage,
} from './api.js';
import type { AssistantConfig, AssistantConversation, AssistantLLMProfile, AssistantMessage } from './model.js';

export function useAssistantController({ autoload = true }: { autoload?: boolean } = {}) {
  const [config, setConfig] = useState<AssistantConfig | null>(null);
  const [conversations, setConversations] = useState<AssistantConversation[]>([]);
  const [activeConversation, setActiveConversation] = useState<AssistantConversation | null>(null);
  const [profiles, setProfiles] = useState<AssistantLLMProfile[]>([]);
  const [pendingMessage, setPendingMessage] = useState<AssistantMessage | null>(null);
  const [selectedProfile, setSelectedProfile] = useState('');
  const [draft, setDraft] = useState('');
  const [loading, setLoading] = useState(autoload);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const profileOptions = useMemo(() => selectableAssistantProfileNames(profiles), [profiles]);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const assistantConfig = await fetchAssistantConfig();
      setConfig(assistantConfig);
      if (!assistantConfig.enabled) {
        setConversations([]);
        setActiveConversation(null);
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
      if (conversationPayload.conversations[0]) {
        const conversation = await fetchAssistantConversation(conversationPayload.conversations[0].id);
        setActiveConversation(conversation);
        if (conversation.selected_llm_profile && selectable.includes(conversation.selected_llm_profile)) {
          setSelectedProfile(conversation.selected_llm_profile);
        }
      } else {
        setActiveConversation(null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load assistant');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!autoload) return;
    void load();
  }, [autoload, load]);

  const selectConversation = useCallback(async (conversationID: string) => {
    setLoading(true);
    setError(null);
    try {
      const conversation = await fetchAssistantConversation(conversationID);
      setPendingMessage(null);
      setActiveConversation(conversation);
      if (conversation.selected_llm_profile && profileOptions.includes(conversation.selected_llm_profile)) {
        setSelectedProfile(conversation.selected_llm_profile);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load conversation');
    } finally {
      setLoading(false);
    }
  }, [profileOptions]);

  const startConversation = useCallback(async () => {
    if (config?.enabled !== true) return null;
    setSending(true);
    setError(null);
    try {
      const conversation = await createAssistantConversation({
        selected_llm_profile: selectedProfile,
        docs_version: config?.default_docs_version || 'auto',
      });
      setActiveConversation(conversation);
      setConversations(current => [conversation, ...current.filter(item => item.id !== conversation.id)]);
      return conversation;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to create conversation');
      return null;
    } finally {
      setSending(false);
    }
  }, [config?.default_docs_version, config?.enabled, selectedProfile]);

  const submitMessage = useCallback(async () => {
    const content = draft.trim();
    if (!content || sending || config?.enabled !== true) return;
    setDraft('');
    setPendingMessage(buildPendingAssistantMessage(activeConversation?.id || '', content));
    setSending(true);
    setError(null);
    try {
      let conversation = activeConversation;
      if (!conversation) {
        const createdConversation = await createAssistantConversation({
          selected_llm_profile: selectedProfile,
          docs_version: config?.default_docs_version || 'auto',
        });
        conversation = createdConversation;
        setActiveConversation(conversation);
        setConversations(current => [
          createdConversation,
          ...current.filter(item => item.id !== createdConversation.id),
        ]);
      }
      const payload = await sendAssistantMessage({
        conversation_id: conversation.id,
        content,
        selected_llm_profile: selectedProfile,
      });
      setPendingMessage(null);
      setActiveConversation(payload.conversation);
      setConversations(current => [
        payload.conversation,
        ...current.filter(item => item.id !== payload.conversation.id),
      ]);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to send message');
      setDraft(current => current || content);
      setPendingMessage(null);
    } finally {
      setSending(false);
    }
  }, [activeConversation, config?.default_docs_version, config?.enabled, draft, selectedProfile, sending]);

  const activeMessages = useMemo(() => {
    const messages = activeConversation?.messages || [];
    return pendingMessage ? [...messages, pendingMessage] : messages;
  }, [activeConversation?.messages, pendingMessage]);

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
    error,
    config,
    enabled: config?.enabled === true,
    load,
    selectConversation,
    startConversation,
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
