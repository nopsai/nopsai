import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowLeft, KeyRound, Plus, Search, X } from 'lucide-react';
import { useLocation, useNavigate } from 'react-router-dom';
import { WorkflowToastRegion, type WorkflowToast } from '../components/WorkflowToastRegion';
import { ScopeCollectionList } from '../features/scopes/ScopeCollectionList';
import { ScopeDetailView } from '../features/scopes/ScopeDetailView';
import { ScopeWorkflowModals } from '../features/scopes/ScopeWorkflowModals';
import {
  fetchScopeCatalogs,
  fetchScopedItems,
  fetchScopeUsageCatalogs,
  fetchScopeUsagePipelineYaml,
  fetchScopeUsageTriggerYaml,
  fetchVariableValue as fetchVariableValueRequest,
  scopedResourcePath,
} from '../features/scopes/api';
import { useScopeModalMutations } from '../features/scopes/useScopeModalMutations';
import { useScopePermissions } from '../features/scopes/useScopePermissions';
import {
  buildScopeTree,
  asScopeRecord,
  buildScopePipelineMeta,
  canonicalizeTriggerEvent,
  createInitialScopeData,
  decodeScopeFromRoute,
  encodeScopeForRoute,
  extractPipelineSecrets,
  extractScopeVariables,
  extractTriggerPipelines,
  getScopeTreeNode,
  normalizeItemListPayload,
  normalizeScopePipelineList,
  normalizeScopeLabel,
  normalizeTriggerOverrideSlugs,
  parentScopeTeam,
  parseScopedIdentity,
  parseScopeYamlSafe,
  runWithConcurrencyLimit,
  type ScopeData,
  type ScopeEntry,
  type ScopePipelineMeta,
  type ScopeTriggerDescriptor,
} from '../features/scopes/model';
import { TEAM_ROUTE_SEGMENT, decodeTeamRouteSegments, teamScopedRoute } from '../lib/teamRoutes';

function ScopesPage({
  canDeleteScopes = false,
}: {
  canDeleteScopes?: boolean;
}) {
  const navigate = useNavigate();
  const location = useLocation();

  const [scopes, setScopes] = useState<ScopeEntry[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);

  const [scopeDataByScope, setScopeDataByScope] = useState<Record<string, ScopeData>>({});
  const scopeDataRef = useRef<Record<string, ScopeData>>({});
  const scopeVariablesPromiseRef = useRef<Map<string, Promise<void>>>(new Map());
  const scopeSecretsPromiseRef = useRef<Map<string, Promise<void>>>(new Map());

  const [activeTeam, setActiveTeam] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  const [selectedScope, setSelectedScope] = useState<string | null>(null);
  const selectedScopeRef = useRef<string | null>(null);
  const preloadScopesRef = useRef<Set<string>>(new Set());
  const [selectedVariable, setSelectedVariable] = useState<string | null>(null);
  const [selectedSecret, setSelectedSecret] = useState<string | null>(null);
  const selectVariable = useCallback((name: string | null) => {
    setSelectedVariable(name);
    if (name) {
      setSelectedSecret(null);
    }
  }, []);
  const selectSecret = useCallback((name: string | null) => {
    setSelectedSecret(name);
    if (name) {
      setSelectedVariable(null);
    }
  }, []);

  const [expandedVariableKey, setExpandedVariableKey] = useState<string | null>(null);
  const [variableValueLoadingKey, setVariableValueLoadingKey] = useState<string | null>(null);
  const [variableValues, setVariableValues] = useState<Record<string, string>>({});
  const variableValuesRef = useRef<Record<string, string>>({});
  const variableValuePromiseRef = useRef<Map<string, Promise<string>>>(new Map());

  const [toasts, setToasts] = useState<WorkflowToast[]>([]);

  const [pipelineVariableIndex, setPipelineVariableIndex] = useState<Map<string, Set<string>>>(new Map());
  const [pipelineSecretIndex, setPipelineSecretIndex] = useState<Map<string, Set<string>>>(new Map());
  const [pipelineMetadata, setPipelineMetadata] = useState<Map<string, ScopePipelineMeta>>(new Map());
  const [triggersByScope, setTriggersByScope] = useState<Map<string, ScopeTriggerDescriptor[]>>(new Map());
  const [usageLoading, setUsageLoading] = useState(false);
  const [usageError, setUsageError] = useState<string | null>(null);
  const usageReadyRef = useRef(false);

  const addToast = useCallback((message: string, tone: WorkflowToast['tone'] = 'info') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts(prev => prev.filter(toast => toast.id !== id));
    }, 3200);
  }, []);

  useEffect(() => {
    scopeDataRef.current = scopeDataByScope;
  }, [scopeDataByScope]);

  useEffect(() => {
    variableValuesRef.current = variableValues;
  }, [variableValues]);

  const loadScopes = useCallback(async () => {
    setListLoading(true);
    setListError(null);
    try {
      const { secrets: secretJson, variables: variableJson } = await fetchScopeCatalogs();

      const secretCounts = new Map<string, number>();
      if (Array.isArray(secretJson)) {
        secretJson.forEach((entry: unknown) => {
          if (!entry || typeof entry !== 'object') return;
          const record = entry as Record<string, unknown>;
          const scopeLabel = normalizeScopeLabel(record.scope);
          const count = typeof record.secret_count === 'number' ? record.secret_count : 0;
          secretCounts.set(scopeLabel, count);
        });
      }

      const scopeSet = new Set<string>();
      scopeSet.add('');
      if (Array.isArray(variableJson)) {
        variableJson.forEach((entry: unknown) => {
          if (typeof entry === 'string') {
            scopeSet.add(normalizeScopeLabel(entry));
            return;
          }
          if (!entry || typeof entry !== 'object') return;
          const record = entry as Record<string, unknown>;
          scopeSet.add(normalizeScopeLabel(record.scope ?? record.name ?? record.value));
        });
      }
      secretCounts.forEach((_, scopeLabel) => scopeSet.add(scopeLabel));

      const nextScopes: ScopeEntry[] = Array.from(scopeSet)
        .map(scopeLabel => {
          const normalized = normalizeScopeLabel(scopeLabel);
          const parts = normalized.split('/').filter(Boolean);
          const label = normalized ? parts[parts.length - 1] : 'Default Scope';
          const description = normalized ? `Scope “/${normalized}”` : 'Fallback scope shared across all pipelines';
          return {
            scope: normalized,
            label,
            teamPath: normalized,
            description,
            secretCountHint: secretCounts.get(normalized) || 0,
          };
        })
        .sort((a, b) => {
          const teamCompare = (a.teamPath || '').localeCompare(b.teamPath || '', undefined, { sensitivity: 'base' });
          if (teamCompare !== 0) return teamCompare;
          return (a.label || '').localeCompare(b.label || '', undefined, { sensitivity: 'base' });
        });

      setScopes(nextScopes);
    } catch (error) {
      console.error('Failed to load scopes', error);
      setListError(error instanceof Error ? error.message : 'Unable to load scopes');
      setScopes([]);
    } finally {
      setListLoading(false);
    }
  }, []);

  const ensureScopeVariables = useCallback(async (scopeLabel: string, force = false) => {
    const scope = normalizeScopeLabel(scopeLabel);
    const existing = scopeDataRef.current[scope];
    if (!force && existing?.variablesLoaded) return;

    if (scopeVariablesPromiseRef.current.has(scope)) {
      await scopeVariablesPromiseRef.current.get(scope);
      if (!force) return;
    }

    setScopeDataByScope(prev => {
      const current = prev[scope] || createInitialScopeData();
      if (!force && current.variablesLoaded) return prev;
      return { ...prev, [scope]: { ...current, variablesLoading: true, error: undefined } };
    });

    const promise = (async () => {
      try {
        const payload = await fetchScopedItems('variable', scope);
        const { names, meta } = normalizeItemListPayload(payload);

        setScopeDataByScope(prev => {
          const current = prev[scope] || createInitialScopeData();
          return {
            ...prev,
            [scope]: {
              ...current,
              variables: names,
              variableMeta: meta,
              variablesLoaded: true,
              variablesLoading: false,
            },
          };
        });
      } catch (error) {
        console.error('Failed to load scope variables', { scope, error });
        setScopeDataByScope(prev => {
          const current = prev[scope] || createInitialScopeData();
          return {
            ...prev,
            [scope]: {
              ...current,
              variables: current.variablesLoaded ? current.variables : [],
              variableMeta: current.variablesLoaded ? current.variableMeta : {},
              variablesLoaded: current.variablesLoaded,
              variablesLoading: false,
              error: error instanceof Error ? error.message : 'Unable to load variables',
            },
          };
        });
      } finally {
        scopeVariablesPromiseRef.current.delete(scope);
      }
    })();

    scopeVariablesPromiseRef.current.set(scope, promise);
    await promise;
  }, []);

  const ensureScopeSecrets = useCallback(async (scopeLabel: string, force = false) => {
    const scope = normalizeScopeLabel(scopeLabel);
    const existing = scopeDataRef.current[scope];
    if (!force && existing?.secretsLoaded) return;

    if (scopeSecretsPromiseRef.current.has(scope)) {
      await scopeSecretsPromiseRef.current.get(scope);
      if (!force) return;
    }

    setScopeDataByScope(prev => {
      const current = prev[scope] || createInitialScopeData();
      if (!force && current.secretsLoaded) return prev;
      return { ...prev, [scope]: { ...current, secretsLoading: true, error: undefined } };
    });

    const promise = (async () => {
      try {
        const payload = await fetchScopedItems('secret', scope);
        const { names, meta } = normalizeItemListPayload(payload);

        setScopeDataByScope(prev => {
          const current = prev[scope] || createInitialScopeData();
          return {
            ...prev,
            [scope]: {
              ...current,
              secrets: names,
              secretMeta: meta,
              secretsLoaded: true,
              secretsLoading: false,
            },
          };
        });
      } catch (error) {
        console.error('Failed to load scope secrets', { scope, error });
        setScopeDataByScope(prev => {
          const current = prev[scope] || createInitialScopeData();
          return {
            ...prev,
            [scope]: {
              ...current,
              secrets: current.secretsLoaded ? current.secrets : [],
              secretMeta: current.secretsLoaded ? current.secretMeta : {},
              secretsLoaded: current.secretsLoaded,
              secretsLoading: false,
              error: error instanceof Error ? error.message : 'Unable to load secrets',
            },
          };
        });
      } finally {
        scopeSecretsPromiseRef.current.delete(scope);
      }
    })();

    scopeSecretsPromiseRef.current.set(scope, promise);
    await promise;
  }, []);

  const buildUsageIndexes = useCallback(async () => {
    if (usageReadyRef.current) return;
    setUsageLoading(true);
    setUsageError(null);
    try {
      const { pipelines: pipelinesPayload, triggers: trigPayload } = await fetchScopeUsageCatalogs();
      const { identifiers, seeds } = normalizeScopePipelineList(pipelinesPayload);

      const variableIndex = new Map<string, Set<string>>();
      const secretIndex = new Map<string, Set<string>>();
      const metaMap = new Map<string, ScopePipelineMeta>();

      const pipelineTasks = identifiers.map(identifier => {
        return async () => {
          try {
            const rawYaml = await fetchScopeUsagePipelineYaml(identifier);
            if (!rawYaml) return;
            const manifest = parseScopeYamlSafe(rawYaml);
            const seed = seeds.get(identifier);
            metaMap.set(identifier, buildScopePipelineMeta(identifier, manifest, seed));

            extractScopeVariables(manifest).forEach(variable => {
              if (!variable) return;
              const set = variableIndex.get(variable) || new Set<string>();
              set.add(identifier);
              variableIndex.set(variable, set);
            });

            extractPipelineSecrets(manifest).forEach(secret => {
              if (!secret) return;
              const set = secretIndex.get(secret) || new Set<string>();
              set.add(identifier);
              secretIndex.set(secret, set);
            });
          } catch (error) {
            console.warn('Failed to process pipeline for usage', identifier, error);
          }
        };
      });

      await runWithConcurrencyLimit(pipelineTasks, 4);
      setPipelineVariableIndex(variableIndex);
      setPipelineSecretIndex(secretIndex);
      setPipelineMetadata(metaMap);

      const slugs = normalizeTriggerOverrideSlugs(trigPayload);
      const trigMap = new Map<string, ScopeTriggerDescriptor[]>();

      const triggerTasks = slugs.map(slug => {
        return async () => {
          const [owner, name] = slug.split('/');
          if (!owner || !name) return;
          try {
            const rawYaml = await fetchScopeUsageTriggerYaml(slug);
            if (!rawYaml) return;
            const manifest = parseScopeYamlSafe(rawYaml);
            const triggers = Array.isArray(manifest?.triggers) ? manifest.triggers : [];
            triggers.forEach((triggerValue: unknown) => {
              const trigger = asScopeRecord(triggerValue);
              if (!trigger) return;
              const scope = normalizeScopeLabel(trigger?.scope || '');
              const entry: ScopeTriggerDescriptor = {
                slug,
                scope,
                pipelines: extractTriggerPipelines(trigger?.pipelines),
                event: canonicalizeTriggerEvent(trigger?.on),
                branches: Array.isArray(trigger?.branches) ? trigger.branches.map((b: unknown) => String(b || '').trim()).filter(Boolean) : [],
                tags: Array.isArray(trigger?.tags) ? trigger.tags.map((t: unknown) => String(t || '').trim()).filter(Boolean) : [],
              };
              const list = trigMap.get(scope) || [];
              list.push(entry);
              trigMap.set(scope, list);
            });
          } catch (error) {
            console.warn('Failed to process trigger override', slug, error);
          }
        };
      });

      await runWithConcurrencyLimit(triggerTasks, 4);
      trigMap.forEach(list => {
        list.sort((a, b) => a.slug.localeCompare(b.slug, undefined, { sensitivity: 'base' }));
      });
      setTriggersByScope(trigMap);

      usageReadyRef.current = true;
    } catch (error) {
      console.error('Impact analysis failed', error);
      setUsageError(error instanceof Error ? error.message : 'Unable to build impact analysis.');
    } finally {
      setUsageLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadScopes();
  }, [loadScopes]);

  useEffect(() => {
    if (!scopes.length) return;
    const tasks: Array<() => Promise<void>> = [];
    scopes.forEach(scope => {
      const label = normalizeScopeLabel(scope.scope);
      if (preloadScopesRef.current.has(label)) return;
      preloadScopesRef.current.add(label);
      tasks.push(async () => {
        await ensureScopeVariables(label);
        await ensureScopeSecrets(label);
      });
    });
    if (tasks.length) {
      void runWithConcurrencyLimit(tasks, 4);
    }
  }, [ensureScopeSecrets, ensureScopeVariables, scopes]);

  useEffect(() => {
    const segments = location.pathname.split('/').filter(Boolean);
    if (segments[0] !== 'scopes') return;
    const isTeamRoute = segments[1] === TEAM_ROUTE_SEGMENT;
    if (isTeamRoute) {
      selectedScopeRef.current = null;
      setSelectedScope(null);
    } else if (segments.length > 1) {
      const scopeLabel = normalizeScopeLabel(decodeScopeFromRoute(segments.slice(1)));
      if (scopeLabel !== selectedScopeRef.current) {
        selectedScopeRef.current = scopeLabel;
        setSelectedScope(scopeLabel);
      }
    } else if (selectedScopeRef.current !== null) {
      selectedScopeRef.current = null;
      setSelectedScope(null);
    }

    const params = new URLSearchParams(location.search);
    const routeTeam = isTeamRoute ? decodeTeamRouteSegments(segments.slice(2)) : '';
    const team = routeTeam || params.get('team') || '';
    setActiveTeam(team);
    if (!isTeamRoute && segments.length === 1 && params.get('team')) {
      navigate(teamScopedRoute('/scopes', team), { replace: true });
    }
  }, [location.pathname, location.search, navigate]);

  useEffect(() => {
    if (listLoading || listError) return;
    if (selectedScope == null) return;
    const normalized = normalizeScopeLabel(selectedScope);
    if (!scopes.some(scope => scope.scope === normalized)) {
      selectedScopeRef.current = null;
      setSelectedScope(null);
      navigate('/scopes', { replace: true });
    }
  }, [listError, listLoading, navigate, scopes, selectedScope]);

  useEffect(() => {
    if (selectedScope == null) {
      selectVariable(null);
      selectSecret(null);
      setExpandedVariableKey(null);
      return;
    }
    selectVariable(null);
    selectSecret(null);
    setExpandedVariableKey(null);
    void ensureScopeVariables(selectedScope);
    void ensureScopeSecrets(selectedScope);
  }, [ensureScopeSecrets, ensureScopeVariables, selectSecret, selectVariable, selectedScope]);

  useEffect(() => {
    if (selectedScope == null) return;
    const data = scopeDataByScope[selectedScope];
    if (!data) return;

    if (data.variablesLoaded && selectedVariable && !data.variables.includes(selectedVariable)) {
      selectVariable(null);
    }

    if (data.secretsLoaded && selectedSecret && !data.secrets.includes(selectedSecret)) {
      selectSecret(null);
    }
  }, [scopeDataByScope, selectSecret, selectVariable, selectedScope, selectedSecret, selectedVariable]);

  useEffect(() => {
    if (selectedScope == null) return;
    if (usageReadyRef.current) return;
    void buildUsageIndexes();
  }, [buildUsageIndexes, selectedScope]);

  const {
    canCreateScopeHere,
    canWriteVariablesInSelectedScope,
    canWriteSecretsInSelectedScope,
  } = useScopePermissions(activeTeam, selectedScope);

  const scopesByLabel = useMemo(() => {
    const map = new Map<string, ScopeEntry>();
    scopes.forEach(scope => map.set(scope.scope, scope));
    return map;
  }, [scopes]);

  const knownRepositories = useMemo(() => {
    const repos = new Set<string>();
    Object.values(scopeDataByScope).forEach(data => {
      data.variables.forEach(name => {
        const identity = parseScopedIdentity(name);
        if (identity.repoSlug) repos.add(identity.repoSlug);
      });
      data.secrets.forEach(name => {
        const identity = parseScopedIdentity(name);
        if (identity.repoSlug) repos.add(identity.repoSlug);
      });
    });
    return Array.from(repos).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
  }, [scopeDataByScope]);

  const variableSuggestionEntries = useMemo(() => {
    const entries: Array<{ scope: string; label: string; count: number; preview: string[] }> = [];
    Object.entries(scopeDataByScope).forEach(([scope, data]) => {
      if (!data.variablesLoaded || data.variables.length === 0) return;
      entries.push({
        scope,
        label: scope ? `/${scope}` : '/ (default)',
        count: data.variables.length,
        preview: data.variables.slice(0, 5),
      });
    });
    entries.sort((a, b) => a.label.localeCompare(b.label, undefined, { sensitivity: 'base' }));
    return entries;
  }, [scopeDataByScope]);

  const secretSuggestionEntries = useMemo(() => {
    const entries: Array<{ scope: string; label: string; count: number; preview: string[] }> = [];
    Object.entries(scopeDataByScope).forEach(([scope, data]) => {
      if (!data.secretsLoaded || data.secrets.length === 0) return;
      entries.push({
        scope,
        label: scope ? `/${scope}` : '/ (default)',
        count: data.secrets.length,
        preview: data.secrets.slice(0, 5),
      });
    });
    entries.sort((a, b) => a.label.localeCompare(b.label, undefined, { sensitivity: 'base' }));
    return entries;
  }, [scopeDataByScope]);

  const scopeTree = useMemo(() => buildScopeTree(scopes, []), [scopes]);

  const filteredScopes = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    if (!term) return scopes;
    return scopes.filter(scope => {
      if (scope.scope.toLowerCase().includes(term)) return true;
      if (scope.label.toLowerCase().includes(term)) return true;
      if (scope.description.toLowerCase().includes(term)) return true;
      return false;
    });
  }, [scopes, searchTerm]);

  const activeTeamNode = useMemo(() => {
    const node = getScopeTreeNode(scopeTree, activeTeam);
    return node || scopeTree;
  }, [activeTeam, scopeTree]);

  const openTeam = (path: string) => {
    const cleaned = normalizeScopeLabel(path);
    setActiveTeam(cleaned);
    selectedScopeRef.current = null;
    setSelectedScope(null);
    navigate(teamScopedRoute('/scopes', cleaned));
  };

  const handleSelectScope = (scopeLabel: string) => {
    const normalized = normalizeScopeLabel(scopeLabel);
    selectedScopeRef.current = normalized;
    setSelectedScope(normalized);
    navigate(`/scopes/${encodeScopeForRoute(normalized)}`);
  };

  const handleBackToList = () => {
    if (selectedScope != null) {
      navigate(teamScopedRoute('/scopes', parentScopeTeam(selectedScope)));
      return;
    }
    navigate('/scopes');
  };

  const {
    chooseSecretSuggestion,
    chooseVariableSuggestion,
    closeDeleteModal,
    closeGitOpsEncryptModal,
    closeScopeModal,
    closeSecretModal,
    closeVariableModal,
    confirmDelete,
    copyGitOpsEncryptedValue,
    deleteModal,
    encryptGitOpsSecretValue,
    gitOpsEncryptModal,
    openDeleteModal,
    openGitOpsEncryptModal,
    openNewScopeModal,
    openSecretCloneModal,
    openSecretCreateModal,
    openSecretUpdateModal,
    openVariableCloneModal,
    openVariableCreateModal,
    openVariableUpdateModal,
    scopeModal,
    secretModal,
    submitScopeModal,
    submitSecretModal,
    submitVariableModal,
    updateGitOpsEncryptValue,
    updateScopeName,
    updateSecretModal,
    updateVariableModal,
    variableModal,
  } = useScopeModalMutations({
    activeTeam,
    scopesByLabel,
    scopeDataByScope,
    canCreateScopeHere,
    canWriteVariablesInSelectedScope,
    canWriteSecretsInSelectedScope,
    canDeleteScopes,
    selectedVariable,
    selectedSecret,
    addToast,
    loadScopes,
    ensureScopeVariables,
    ensureScopeSecrets,
    selectVariable,
    selectSecret,
    clearExpandedVariable: () => setExpandedVariableKey(null),
    onScopeCreated: scope => navigate(`/scopes/${encodeScopeForRoute(scope)}`),
  });

  const fetchVariableValue = useCallback(async (scopeLabel: string, fullName: string): Promise<string> => {
    const scope = normalizeScopeLabel(scopeLabel);
    const identity = parseScopedIdentity(fullName);
    if (!identity.name) return '';

    const cacheKey = `${identity.fullName}@@${scope}`;
    if (Object.prototype.hasOwnProperty.call(variableValuesRef.current, cacheKey)) {
      return variableValuesRef.current[cacheKey] ?? '';
    }
    if (variableValuePromiseRef.current.has(cacheKey)) {
      return (await variableValuePromiseRef.current.get(cacheKey)) ?? '';
    }

    const promise = (async () => {
      try {
        return await fetchVariableValueRequest(scopedResourcePath('variable', scope, identity.name, identity.repoSlug));
      } finally {
        variableValuePromiseRef.current.delete(cacheKey);
      }
    })();

    variableValuePromiseRef.current.set(cacheKey, promise);
    const value = await promise;
    setVariableValues(prev => ({ ...prev, [cacheKey]: value }));
    return value;
  }, []);

  const toggleVariableValue = async (scopeLabel: string, fullName: string) => {
    const scope = normalizeScopeLabel(scopeLabel);
    const identity = parseScopedIdentity(fullName);
    const cacheKey = `${identity.fullName}@@${scope}`;
    if (expandedVariableKey === cacheKey) {
      setExpandedVariableKey(null);
      return;
    }

    if (!Object.prototype.hasOwnProperty.call(variableValuesRef.current, cacheKey)) {
      try {
        setVariableValueLoadingKey(cacheKey);
        await fetchVariableValue(scope, identity.fullName);
      } catch (error) {
        console.error('Failed to fetch variable value', error);
        addToast(error instanceof Error ? error.message : 'Failed to load variable value.', 'error');
      } finally {
        setVariableValueLoadingKey(null);
      }
    }

    setExpandedVariableKey(cacheKey);
  };

  return (
    <div data-page="scopes" className="active h-full flex flex-col">
      {selectedScope === null && (
        <div className="px-6 pt-6 pb-4">
          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              className="glass-button-ghost"
              aria-label="Back"
              onClick={() => openTeam(parentScopeTeam(activeTeam))}
              disabled={!activeTeam}
            >
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
            </button>

            <div className={`pipelines-search-shell ${searchOpen ? 'open' : ''}`}>
              <button
                type="button"
                className="pipelines-search-toggle"
                aria-label="Search scopes"
                onClick={() => {
                  setSearchOpen(true);
                  requestAnimationFrame(() => searchInputRef.current?.focus());
                }}
              >
                <Search className="h-4 w-4" aria-hidden="true" />
              </button>
              <input
                ref={searchInputRef}
                id="scopes-search"
                type="text"
                placeholder="Search scopes"
                className="pipelines-search-input"
                value={searchTerm}
                onChange={event => {
                  setSearchTerm(event.target.value);
                  if (event.target.value && !searchOpen) setSearchOpen(true);
                }}
                onBlur={() => {
                  if (!searchTerm.trim()) setSearchOpen(false);
                }}
              />
              {(searchTerm || searchOpen) && (
                <button
                  type="button"
                  className="pipelines-search-clear"
                  onClick={() => {
                    setSearchTerm('');
                    setSearchOpen(false);
                    searchInputRef.current?.blur();
                  }}
                  aria-label="Clear search"
                >
                  <X className="h-4 w-4" aria-hidden="true" />
                </button>
              )}
            </div>

            <button
              type="button"
              className="pipelines-icon-only"
              aria-label="Encrypt secret for GitOps"
              title="Encrypt secret for GitOps"
              onClick={openGitOpsEncryptModal}
            >
              <KeyRound className="h-4 w-4" aria-hidden="true" />
            </button>

            {!searchTerm.trim() && canCreateScopeHere && (
              <button
                id="scopes-new-btn"
                type="button"
                className="pipelines-icon-only"
                aria-label="Create new scope"
                title="New Scope"
                onClick={openNewScopeModal}
              >
                <Plus className="h-4 w-4" aria-hidden="true" />
              </button>
            )}
          </div>
        </div>
      )}

      <div className="flex-1 overflow-auto px-6 pb-8 triggers-content">
        {selectedScope === null ? (
          <ScopeCollectionList
            listLoading={listLoading}
            listError={listError}
            searchTerm={searchTerm}
            activeTeamNode={activeTeamNode}
            filteredScopes={filteredScopes}
            scopesByLabel={scopesByLabel}
            scopeDataByScope={scopeDataByScope}
            canCreateScopeHere={canCreateScopeHere}
            onOpenTeam={openTeam}
            onSelectScope={handleSelectScope}
          />
        ) : (
          <ScopeDetailView
            selectedScope={selectedScope}
            scopeDataByScope={scopeDataByScope}
            selectedVariable={selectedVariable}
            selectedSecret={selectedSecret}
            expandedVariableKey={expandedVariableKey}
            variableValueLoadingKey={variableValueLoadingKey}
            variableValues={variableValues}
            pipelineVariableIndex={pipelineVariableIndex}
            pipelineSecretIndex={pipelineSecretIndex}
            pipelineMetadata={pipelineMetadata}
            triggersByScope={triggersByScope}
            usageLoading={usageLoading}
            usageError={usageError}
            canWriteVariablesInSelectedScope={canWriteVariablesInSelectedScope}
            canWriteSecretsInSelectedScope={canWriteSecretsInSelectedScope}
            canDeleteScopes={canDeleteScopes}
            onSelectVariable={selectVariable}
            onSelectSecret={selectSecret}
            onToggleVariableValue={toggleVariableValue}
            onCreateVariable={openVariableCreateModal}
            onUpdateVariable={openVariableUpdateModal}
            onCloneVariable={openVariableCloneModal}
            onCreateSecret={openSecretCreateModal}
            onUpdateSecret={openSecretUpdateModal}
            onCloneSecret={openSecretCloneModal}
            onDeleteValue={openDeleteModal}
            onOpenGitOpsEncrypt={openGitOpsEncryptModal}
            onBack={handleBackToList}
          />
        )}
      </div>

      <ScopeWorkflowModals
        scopeModal={scopeModal}
        variableModal={variableModal}
        secretModal={secretModal}
        gitOpsEncryptModal={gitOpsEncryptModal}
        deleteModal={deleteModal}
        canDeleteScopes={canDeleteScopes}
        knownRepositories={knownRepositories}
        variableSuggestionEntries={variableSuggestionEntries}
        secretSuggestionEntries={secretSuggestionEntries}
        onCloseScope={closeScopeModal}
        onUpdateScopeName={updateScopeName}
        onSubmitScope={() => void submitScopeModal()}
        onCloseVariable={closeVariableModal}
        onUpdateVariable={updateVariableModal}
        onChooseVariableSuggestion={chooseVariableSuggestion}
        onSubmitVariable={() => void submitVariableModal()}
        onCloseSecret={closeSecretModal}
        onUpdateSecret={updateSecretModal}
        onChooseSecretSuggestion={chooseSecretSuggestion}
        onSubmitSecret={() => void submitSecretModal()}
        onCloseGitOpsEncrypt={closeGitOpsEncryptModal}
        onUpdateGitOpsEncryptValue={updateGitOpsEncryptValue}
        onEncryptGitOpsSecret={() => void encryptGitOpsSecretValue()}
        onCopyGitOpsEncryptedValue={() => void copyGitOpsEncryptedValue()}
        onCloseDelete={closeDeleteModal}
        onConfirmDelete={() => void confirmDelete()}
      />

      <WorkflowToastRegion toasts={toasts} />
    </div>
  );
}

export default ScopesPage;
