import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { KNOWLEDGE_CONTEXTS_CHANGED_EVENT } from './constants';
import {
  buildKnowledgeContextTree,
  buildPipelineTree,
  buildScopeTree,
  buildStepTree,
  normalizeScopeLabel,
  splitIdentifier,
} from './resourceTrees';
import { apiClient } from '../lib/api';
import { PIPELINE_DRAFTS_CHANGED_EVENT, getPipelineDraftStorageKey, loadPipelineDrafts } from '../lib/pipelineDrafts';
import { STEP_DRAFTS_CHANGED_EVENT, getStepDraftStorageKey, loadStepDrafts } from '../lib/stepDrafts';

type UseResourceTreesOptions = {
  canViewKnowledge: boolean;
  canWritePipelines: boolean;
  canWriteSteps: boolean;
  draftScope: string;
  isAuthenticated: boolean;
  pathname: string;
};

const asRecord = (value: unknown): Record<string, unknown> | null => {
  if (!value || typeof value !== 'object') return null;
  return value as Record<string, unknown>;
};

const toggleOpenSet = (prev: Set<string>, id: string) => {
  const next = new Set(prev);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  return next;
};

export function useResourceTrees({
  canViewKnowledge,
  canWritePipelines,
  canWriteSteps,
  draftScope,
  isAuthenticated,
  pathname,
}: UseResourceTreesOptions) {
  const [pipelines, setPipelines] = useState<string[]>([]);
  const serverPipelinesRef = useRef<string[]>([]);
  const [pipelineTreeOpen, setPipelineTreeOpen] = useState<Set<string>>(new Set());

  const [steps, setSteps] = useState<string[]>([]);
  const serverStepsRef = useRef<string[]>([]);
  const [stepTreeOpen, setStepTreeOpen] = useState<Set<string>>(new Set());

  const [scopes, setScopes] = useState<string[]>([]);
  const [scopeTreeOpen, setScopeTreeOpen] = useState<Set<string>>(new Set());

  const [knowledgeContexts, setKnowledgeContexts] = useState<string[]>([]);
  const [knowledgeContextTreeOpen, setKnowledgeContextTreeOpen] = useState<Set<string>>(new Set());

  const onToggleKnowledgeContextNode = useCallback((id: string) => {
    setKnowledgeContextTreeOpen(prev => toggleOpenSet(prev, id));
  }, []);

  const onTogglePipelineNode = useCallback((id: string) => {
    setPipelineTreeOpen(prev => toggleOpenSet(prev, id));
  }, []);

  const onToggleScopeNode = useCallback((id: string) => {
    setScopeTreeOpen(prev => toggleOpenSet(prev, id));
  }, []);

  const onToggleStepNode = useCallback((id: string) => {
    setStepTreeOpen(prev => toggleOpenSet(prev, id));
  }, []);

  useEffect(() => {
    if (!isAuthenticated) return;
    const load = async () => {
      try {
        const [secretResp, variableResp] = await Promise.all([
          apiClient.fetch('/v1/secrets/scopes'),
          apiClient.fetch('/v1/variables/scopes'),
        ]);
        const secretJson = secretResp.ok ? await secretResp.json() : [];
        const variableJson = variableResp.ok ? await variableResp.json() : [];
        const scopeSet = new Set<string>();
        scopeSet.add('');
        if (Array.isArray(secretJson)) {
          secretJson.forEach((entry: unknown) => {
            const record = asRecord(entry);
            const scopeLabel = typeof record?.scope === 'string' ? record.scope : '';
            scopeSet.add(normalizeScopeLabel(scopeLabel));
          });
        }
        if (Array.isArray(variableJson)) {
          variableJson.forEach((entry: unknown) => {
            if (typeof entry === 'string') {
              scopeSet.add(normalizeScopeLabel(entry));
              return;
            }
            const record = asRecord(entry);
            const scopeLabel = typeof record?.scope === 'string'
              ? record.scope
              : typeof record?.name === 'string'
                ? record.name
                : '';
            scopeSet.add(normalizeScopeLabel(scopeLabel));
          });
        }
        const list = Array.from(scopeSet).map(normalizeScopeLabel).sort((a, b) => a.localeCompare(b));
        setScopes(list);
      } catch (error) {
        console.warn('Failed to load scopes for sidebar', error);
      }
    };
    if (pathname.startsWith('/scopes')) {
      void load();
    }
  }, [isAuthenticated, pathname]);

  useEffect(() => {
    if (!isAuthenticated) return;
    const load = async () => {
      try {
        const response = await apiClient.fetch('/v1/pipelines');
        if (!response.ok) return;
        const payload = await response.json();
        const ids = Array.isArray(payload)
          ? payload
              .map((item: unknown) => {
                if (typeof item === 'string') return item;
                const record = asRecord(item);
                if (!record) return '';
                if (typeof record.id === 'string') return record.id;
                if (typeof record.identifier === 'string') return record.identifier;
                return '';
              })
              .filter(Boolean)
          : [];
        ids.sort((a, b) => a.localeCompare(b));
        serverPipelinesRef.current = ids;
        const draftIds = canWritePipelines && draftScope ? loadPipelineDrafts(draftScope).map(draft => draft.id) : [];
        const merged = Array.from(new Set([...ids, ...draftIds])).sort((a, b) => a.localeCompare(b));
        setPipelines(merged);
      } catch (error) {
        console.warn('Failed to load pipelines for sidebar', error);
      }
    };
    if (pathname.startsWith('/pipelines')) {
      void load();
    }
  }, [canWritePipelines, draftScope, isAuthenticated, pathname]);

  useEffect(() => {
    if (!isAuthenticated) return;
    const load = async () => {
      try {
        const response = await apiClient.fetch('/v1/steps');
        if (!response.ok) return;
        const payload = await response.json();
        const ids = Array.isArray(payload)
          ? payload.map((item: unknown) => (typeof item === 'string' ? item.trim() : '')).filter(Boolean)
          : [];
        ids.sort((a, b) => a.localeCompare(b));
        serverStepsRef.current = ids;
        const draftIds = canWriteSteps && draftScope ? loadStepDrafts(draftScope).map(draft => draft.id) : [];
        const merged = Array.from(new Set([...ids, ...draftIds])).sort((a, b) => a.localeCompare(b));
        setSteps(merged);
      } catch (error) {
        console.warn('Failed to load steps for sidebar', error);
      }
    };
    if (pathname.startsWith('/steps')) {
      void load();
    }
  }, [canWriteSteps, draftScope, isAuthenticated, pathname]);

  useEffect(() => {
    if (!isAuthenticated || !canViewKnowledge) {
      const handle = window.setTimeout(() => setKnowledgeContexts([]), 0);
      return () => window.clearTimeout(handle);
    }
    const load = async () => {
      try {
        const response = await apiClient.fetch('/v1/knowledge-contexts');
        if (!response.ok) return;
        const payload = await response.json();
        const ids = Array.isArray(payload)
          ? payload
              .map((item: unknown) => {
                if (typeof item === 'string') return item.trim();
                const record = asRecord(item);
                return typeof record?.id === 'string' ? record.id.trim() : '';
              })
              .filter(Boolean)
          : [];
        ids.sort((a, b) => a.localeCompare(b));
        setKnowledgeContexts(ids);
      } catch (error) {
        console.warn('Failed to load knowledge contexts for sidebar', error);
      }
    };
    const handleKnowledgeContextsChanged = () => {
      if (pathname.startsWith('/knowledge-context')) {
        void load();
      }
    };
    window.addEventListener(KNOWLEDGE_CONTEXTS_CHANGED_EVENT, handleKnowledgeContextsChanged);
    if (pathname.startsWith('/knowledge-context')) {
      void load();
    }
    return () => {
      window.removeEventListener(KNOWLEDGE_CONTEXTS_CHANGED_EVENT, handleKnowledgeContextsChanged);
    };
  }, [canViewKnowledge, isAuthenticated, pathname]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    if (!canWritePipelines || !draftScope) return;
    const storageKey = getPipelineDraftStorageKey(draftScope);
    const handleDraftsChanged = () => {
      if (!pathname.startsWith('/pipelines')) return;
      const draftIds = loadPipelineDrafts(draftScope).map(draft => draft.id);
      const merged = Array.from(new Set([...serverPipelinesRef.current, ...draftIds])).sort((a, b) => a.localeCompare(b));
      setPipelines(merged);
    };
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== storageKey) return;
      handleDraftsChanged();
    };
    window.addEventListener(PIPELINE_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
    window.addEventListener('storage', handleStorage);
    return () => {
      window.removeEventListener(PIPELINE_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
      window.removeEventListener('storage', handleStorage);
    };
  }, [canWritePipelines, draftScope, pathname]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    if (!canWriteSteps || !draftScope) return;
    const storageKey = getStepDraftStorageKey(draftScope);
    const handleDraftsChanged = () => {
      if (!pathname.startsWith('/steps')) return;
      const draftIds = loadStepDrafts(draftScope).map(draft => draft.id);
      const merged = Array.from(new Set([...serverStepsRef.current, ...draftIds])).sort((a, b) => a.localeCompare(b));
      setSteps(merged);
    };
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== storageKey) return;
      handleDraftsChanged();
    };
    window.addEventListener(STEP_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
    window.addEventListener('storage', handleStorage);
    return () => {
      window.removeEventListener(STEP_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
      window.removeEventListener('storage', handleStorage);
    };
  }, [canWriteSteps, draftScope, pathname]);

  const knowledgeContextTree = useMemo(
    () => buildKnowledgeContextTree(knowledgeContexts, []),
    [knowledgeContexts]
  );
  const pipelineTree = useMemo(() => buildPipelineTree(pipelines, []), [pipelines]);
  const scopeTree = useMemo(() => buildScopeTree(scopes, []), [scopes]);
  const stepTree = useMemo(() => buildStepTree(steps, []), [steps]);

  return {
    knowledgeContextTree,
    knowledgeContextTreeOpen,
    onToggleKnowledgeContextNode,
    pipelineTree,
    pipelineTreeOpen,
    onTogglePipelineNode,
    scopeTree,
    scopeTreeOpen,
    onToggleScopeNode,
    splitIdentifier,
    stepTree,
    stepTreeOpen,
    onToggleStepNode,
  };
}
