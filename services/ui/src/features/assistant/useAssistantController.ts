import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  createAssistantConversation,
  fetchAssistantConversation,
  fetchAssistantConversations,
  fetchAssistantLLMProfiles,
  sendAssistantMessage,
} from './api.js';
import type { AssistantConversation, AssistantLLMProfile } from './model.js';

export function useAssistantController({ autoload = true }: { autoload?: boolean } = {}) {
  const [conversations, setConversations] = useState<AssistantConversation[]>([]);
  const [activeConversation, setActiveConversation] = useState<AssistantConversation | null>(null);
  const [profiles, setProfiles] = useState<AssistantLLMProfile[]>([]);
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
    setSending(true);
    setError(null);
    try {
      const conversation = await createAssistantConversation({
        selected_llm_profile: selectedProfile,
        docs_version: 'auto',
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
  }, [selectedProfile]);

  const submitMessage = useCallback(async () => {
    const content = draft.trim();
    if (!content || sending) return;
    setSending(true);
    setError(null);
    try {
      let conversation = activeConversation;
      if (!conversation) {
        conversation = await createAssistantConversation({
          selected_llm_profile: selectedProfile,
          docs_version: 'auto',
        });
      }
      const payload = await sendAssistantMessage({
        conversation_id: conversation.id,
        content,
        selected_llm_profile: selectedProfile,
      });
      setDraft('');
      setActiveConversation(payload.conversation);
      setConversations(current => [
        payload.conversation,
        ...current.filter(item => item.id !== payload.conversation.id),
      ]);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to send message');
    } finally {
      setSending(false);
    }
  }, [activeConversation, draft, selectedProfile, sending]);

  const activeMessages = activeConversation?.messages || [];

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
    load,
    selectConversation,
    startConversation,
    submitMessage,
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
