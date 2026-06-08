import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type UIEvent } from 'react';
import { createPortal } from 'react-dom';
import { useLocation, useNavigate } from 'react-router-dom';
import yaml from 'js-yaml';
import { apiClient } from '../lib/api';
import { LabRunControls } from '../features/lab/LabRunControls';
import { useLabRunAuthorization } from '../features/lab/useLabRunAuthorization';
import { useLabRunMutation } from '../features/lab/useLabRunMutation';
import { useLabSession } from '../features/lab/useLabSession';
import {
  applyEnterIndent,
  buildSuggestionItems,
  buildValidationExample,
  detectSuggestionContext,
  suggestionCopyForContext,
  validatePipelineYamlStrict,
  type LabSuggestionContext,
  type LabSuggestionItem,
} from '../lib/lab';
import { renderYamlHighlight } from '../lib/yamlRenderer';

type PipelineListItem = { id: string; source?: string };

const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;

function normalizeScopeLabel(value: unknown): string {
  if (value == null) return '';
  const normalized = String(value)
    .trim()
    .replace(/^\/+|\/+$/g, '');
  return normalized.toLowerCase() === 'default' ? '' : normalized;
}

function normalizeList(payload: unknown): string[] {
  if (!Array.isArray(payload)) return [];
  return payload
    .map(item => {
      if (typeof item === 'string') return item.trim();
      if (item && typeof item === 'object' && 'name' in item) {
        const name = (item as Record<string, unknown>).name;
        if (typeof name === 'string') return name.trim();
      }
      return '';
    })
    .filter(Boolean);
}

function normalizeVariableSuggestionList(payload: unknown): string[] {
  const names = normalizeList(payload);
  const set = new Set<string>();
  names.forEach(name => {
    const parts = name.split('/').filter(Boolean);
    if (parts.length === 3) {
      set.add(parts[2]);
    } else {
      set.add(name);
    }
  });
  return Array.from(set).sort((a, b) => a.localeCompare(b));
}

function normalizeLLMProfileSuggestionList(payload: unknown): string[] {
  const record = payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : null;
  const profiles = record && Array.isArray(record.profiles) ? record.profiles : [];
  return profiles
    .map(profile => {
      if (typeof profile === 'string') return profile.trim();
      if (!profile || typeof profile !== 'object') return '';
      const record = profile as Record<string, unknown>;
      if (record.allowed_in_scope === false) return '';
      return typeof record.name === 'string' ? record.name.trim() : '';
    })
    .filter(Boolean);
}

function normalizeMCPProfileSuggestionList(payload: unknown): string[] {
  const record = payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : null;
  const profiles = record && Array.isArray(record.profiles) ? record.profiles : [];
  return profiles
    .map(profile => {
      if (typeof profile === 'string') return profile.trim();
      if (!profile || typeof profile !== 'object') return '';
      const record = profile as Record<string, unknown>;
      if (record.enabled === false) return '';
      return typeof record.name === 'string' ? record.name.trim() : '';
    })
    .filter(Boolean);
}

function buildInlineSuggestionPreview(item: LabSuggestionItem, contextInfo: LabSuggestionContext): string {
  const prefix = typeof contextInfo.prefix === 'string' ? contextInfo.prefix : '';
  const snippetSource = item.value || item.snippet || '';
  if (!snippetSource) return '';
  const firstLine = String(snippetSource).split('\n')[0];
  if (!firstLine) return '';

  if (prefix) {
    const lowerPrefix = prefix.toLowerCase();
    const lowerLine = firstLine.toLowerCase();
    if (lowerLine.startsWith(lowerPrefix)) {
      return firstLine.slice(prefix.length);
    }
    return '';
  }

  return firstLine;
}

function LabPage() {
  const location = useLocation();
  const navigate = useNavigate();

  const [pipelines, setPipelines] = useState<PipelineListItem[]>([]);
  const [pipelinesLoading, setPipelinesLoading] = useState(false);
  const [pipelinesError, setPipelinesError] = useState<string | null>(null);

  const [scopes, setScopes] = useState<string[]>([]);

  const [autocompleteMeta, setAutocompleteMeta] = useState<{
    secrets: string[];
    variables: string[];
    llmProfiles: string[];
    mcpProfiles: string[];
    reusableSteps: string[];
    fetchedAt: number;
    loading: boolean;
  }>({ secrets: [], variables: [], llmProfiles: [], mcpProfiles: [], reusableSteps: [], fetchedAt: 0, loading: false });
  const autocompleteFetchRef = useRef<{ fetchedAt: number; loadingPromise: Promise<void> | null }>({ fetchedAt: 0, loadingPromise: null });
  const pipelineHandoffRef = useRef('');
  const {
    addOverride,
    changePipeline,
    feedback,
    overrides,
    removeOverride,
    saveSession,
    scopeValue,
    selectedPipelineId,
    setFeedback,
    setScopeValue,
    setYamlText,
    updateOverride,
    yamlLoading,
    yamlText,
  } = useLabSession();
  const editorRef = useRef<HTMLTextAreaElement | null>(null);
  const highlightContentRef = useRef<HTMLPreElement | null>(null);
  const lineNumbersRef = useRef<HTMLDivElement | null>(null);

  const suggestionPanelRef = useRef<HTMLDivElement | null>(null);
  const inlineOverlayRef = useRef<HTMLDivElement | null>(null);
  const caretMirrorRef = useRef<HTMLDivElement | null>(null);
  const rafRef = useRef<number | null>(null);
  const pendingSelectionRef = useRef<number | null>(null);
  const [overlayHost, setOverlayHost] = useState<HTMLElement | null>(null);
  const [portalBody, setPortalBody] = useState<HTMLElement | null>(null);
  const overlayHostNodeRef = useRef<HTMLElement | null>(null);
  const [editorFocused, setEditorFocused] = useState(false);

  const validation = useMemo(() => validatePipelineYamlStrict(yamlText), [yamlText]);

  const errorMap = useMemo(() => {
    const map = new Map<number, string[]>();
    validation.errors.forEach(err => {
      if (typeof err.line !== 'number') return;
      const existing = map.get(err.line) ?? [];
      existing.push(err.message);
      map.set(err.line, existing);
    });
    return map;
  }, [validation.errors]);

  const validationErrorLines = useMemo(() => new Set(Array.from(errorMap.keys())), [errorMap]);

  const editorLines = useMemo(() => (yamlText || '').split('\n'), [yamlText]);

  const [editorSelection, setEditorSelection] = useState<{ start: number; end: number }>({ start: 0, end: 0 });

  const pipelineIds = useMemo(() => pipelines.map(item => item.id).filter(Boolean), [pipelines]);
  const runValidation = useLabRunAuthorization(
    selectedPipelineId,
    yamlText,
    scopeValue,
    validation.errors.length
  );
  const runValidationBlocked = runValidation.blocked;
  const { run: handleRun, runPending } = useLabRunMutation({
    accessBlocked: runValidationBlocked,
    accessLoading: runValidation.loading,
    overrides,
    scopeValue,
    selectedPipelineId,
    setFeedback,
    validationErrorCount: validation.errors.length,
    yamlText,
  });

  useEffect(() => {
    if (scopeValue && !scopes.includes(scopeValue)) {
      setScopeValue('');
    }
  }, [scopeValue, scopes, setScopeValue]);

  const scopeOptions = useMemo(() => {
    const list = scopes
      .slice()
      .map(normalizeScopeLabel)
      .filter(scope => scope !== '')
      .sort((a, b) => a.localeCompare(b));
    return list;
  }, [scopes]);

  const includedDependencies = useMemo(() => {
    try {
      const parsed = yaml.load(yamlText) as unknown;
      if (!parsed || typeof parsed !== 'object') {
        return { status: 'invalid', items: [] as string[] };
      }
      const record = parsed as Record<string, unknown>;
      const stepsRaw = record.steps;
      const steps = Array.isArray(stepsRaw) ? stepsRaw : [];
      if (steps.length === 0) return { status: 'no-steps', items: [] as string[] };

      const includes = new Set<string>();
      steps.forEach(step => {
        if (!step || typeof step !== 'object') return;
        const include = (step as Record<string, unknown>).include;
        if (typeof include === 'string' && include.trim()) {
          includes.add(include.trim());
        }
      });

      return { status: 'ok', items: Array.from(includes).sort((a, b) => a.localeCompare(b)) };
    } catch {
      return { status: 'parse-error', items: [] as string[] };
    }
  }, [yamlText]);

  const suggestionContext = useMemo(() => {
    if (!editorFocused) return null;
    return detectSuggestionContext(yamlText, editorSelection.start, editorSelection.end);
  }, [yamlText, editorSelection.start, editorSelection.end, editorFocused]);

  const suggestionItems = useMemo(() => {
    if (!suggestionContext) return [];
    return buildSuggestionItems(suggestionContext, yamlText, {
      secrets: autocompleteMeta.secrets,
      variables: autocompleteMeta.variables,
      llmProfiles: autocompleteMeta.llmProfiles,
      mcpProfiles: autocompleteMeta.mcpProfiles,
      reusableSteps: autocompleteMeta.reusableSteps,
      pipelineIds,
    });
  }, [suggestionContext, yamlText, autocompleteMeta, pipelineIds]);

  const inlineSuggestion = useMemo(() => {
    if (!suggestionContext) return '';
    if (suggestionItems.length === 0) return '';
    return buildInlineSuggestionPreview(suggestionItems[0], suggestionContext);
  }, [suggestionContext, suggestionItems]);

  const suggestionCopy = useMemo(() => suggestionCopyForContext(suggestionContext), [suggestionContext]);

  const syncEditorOverlays = useCallback((textarea: HTMLTextAreaElement | null) => {
    if (!textarea) return;
    const x = textarea.scrollLeft || 0;
    const y = textarea.scrollTop || 0;
    if (highlightContentRef.current) {
      highlightContentRef.current.style.transform = `translate(${-x}px, ${-y}px)`;
    }
    if (lineNumbersRef.current) {
      lineNumbersRef.current.style.setProperty('--line-number-scroll', `${y}px`);
    }
  }, []);

  const handleEditorScroll = useCallback(
    (event: UIEvent<HTMLTextAreaElement>) => {
      syncEditorOverlays(event.currentTarget);
    },
    [syncEditorOverlays]
  );

  const loadAutocomplete = useCallback(async (force?: boolean) => {
    const now = Date.now();
    if (!force && autocompleteFetchRef.current.fetchedAt && now - autocompleteFetchRef.current.fetchedAt < AUTOCOMPLETE_REFRESH_INTERVAL) {
      return;
    }
    if (autocompleteFetchRef.current.loadingPromise) {
      await autocompleteFetchRef.current.loadingPromise;
      return;
    }

    setAutocompleteMeta(prev => ({ ...prev, loading: true }));
    try {
      const promise = (async () => {
        const scopeParam = scopeValue ? `?scope=${encodeURIComponent(scopeValue)}` : '';
        const [secretsResp, varsResp, stepsResp, llmProfilesResp, mcpProfilesResp] = await Promise.all([
          apiClient.fetch(`/v1/secrets${scopeParam}`).then(r => (r.ok ? r.json() : [])),
          apiClient.fetch(`/v1/variables${scopeParam}`).then(r => (r.ok ? r.json() : [])),
          apiClient.fetch('/v1/steps').then(r => (r.ok ? r.json() : [])),
          apiClient.fetch(`/v1/system/llm-profiles${scopeParam}`).then(r => (r.ok ? r.json() : null)),
          apiClient.fetch('/v1/system/mcp/profiles').then(r => (r.ok ? r.json() : null)),
        ]);

        setAutocompleteMeta({
          secrets: normalizeList(secretsResp),
          variables: normalizeVariableSuggestionList(varsResp),
          llmProfiles: normalizeLLMProfileSuggestionList(llmProfilesResp),
          mcpProfiles: normalizeMCPProfileSuggestionList(mcpProfilesResp),
          reusableSteps: normalizeList(stepsResp),
          fetchedAt: Date.now(),
          loading: false,
        });
        autocompleteFetchRef.current.fetchedAt = Date.now();
      })();

      autocompleteFetchRef.current.loadingPromise = promise;
      await promise;
    } catch (error) {
      console.warn('Failed to load Lab autocomplete metadata', error);
      setAutocompleteMeta(prev => ({ ...prev, loading: false }));
    } finally {
      autocompleteFetchRef.current.loadingPromise = null;
    }
  }, [scopeValue]);

  useEffect(() => {
    // refresh autocomplete when scope changes so suggestions match the target scope
    autocompleteFetchRef.current.fetchedAt = 0;
    void loadAutocomplete(true);
  }, [loadAutocomplete, scopeValue]);

  const loadPipelines = useCallback(async () => {
    setPipelinesLoading(true);
    setPipelinesError(null);
    try {
      const response = await apiClient.fetch('/v1/pipelines?include_source=true');
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Failed to load pipelines (${response.status})`);
      }
      const payload = await response.json();
      const normalized: PipelineListItem[] = Array.isArray(payload)
        ? payload
            .map((item: unknown): PipelineListItem | null => {
              if (typeof item === 'string') return { id: item };
              if (item && typeof item === 'object') {
                const record = item as Record<string, unknown>;
                const id = typeof record.id === 'string' ? record.id : typeof record.identifier === 'string' ? record.identifier : '';
                const source = typeof record.source === 'string' ? record.source : undefined;
                return id ? { id, source } : null;
              }
              return null;
            })
            .filter((item: PipelineListItem | null): item is PipelineListItem => Boolean(item))
        : [];
      normalized.sort((a, b) => a.id.localeCompare(b.id));
      setPipelines(normalized);
    } catch (error) {
      console.error('Failed to load Lab pipelines', error);
      setPipelinesError(error instanceof Error ? error.message : 'Unable to load pipelines');
      setPipelines([]);
    } finally {
      setPipelinesLoading(false);
    }
  }, []);

  const loadScopes = useCallback(async () => {
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
        secretJson.forEach(entry => {
          if (!entry || typeof entry !== 'object') return;
          const record = entry as Record<string, unknown>;
          const label = normalizeScopeLabel(record.scope);
          scopeSet.add(label);
        });
      }

      if (Array.isArray(variableJson)) {
        variableJson.forEach(entry => {
          if (typeof entry === 'string') {
            scopeSet.add(normalizeScopeLabel(entry));
            return;
          }
          if (!entry || typeof entry !== 'object') return;
          const record = entry as Record<string, unknown>;
          const label = normalizeScopeLabel(record.scope ?? record.name ?? record.value);
          scopeSet.add(label);
        });
      }

      const normalized = Array.from(scopeSet)
        .map(normalizeScopeLabel)
        .filter(scope => scope !== null)
        .sort((a, b) => a.localeCompare(b));
      setScopes(normalized);
    } catch (error) {
      console.warn('Failed to load scopes', error);
      setScopes([]);
    }
  }, []);

  const ensureCaretMirror = useCallback(() => {
    if (caretMirrorRef.current) return caretMirrorRef.current;
    const mirror = document.createElement('div');
    mirror.className = 'textarea-caret-mirror';
    mirror.style.position = 'absolute';
    mirror.style.visibility = 'hidden';
    mirror.style.whiteSpace = 'pre-wrap';
    mirror.style.wordWrap = 'break-word';
    mirror.style.pointerEvents = 'none';
    mirror.style.top = '0';
    mirror.style.left = '-9999px';
    mirror.style.transform = 'translateX(0)';
    document.body.appendChild(mirror);
    caretMirrorRef.current = mirror;
    return mirror;
  }, []);

  const calculateCaretOffset = useCallback(
    (textarea: HTMLTextAreaElement | null) => {
      if (!textarea) return null;
      const selectionStart = textarea.selectionStart;
      if (typeof selectionStart !== 'number') return null;

      const mirror = ensureCaretMirror();
      const computed = window.getComputedStyle(textarea);
      type CaretMirrorStyleKey =
        | 'boxSizing'
        | 'width'
        | 'height'
        | 'overflowX'
        | 'overflowY'
        | 'borderTopWidth'
        | 'borderRightWidth'
        | 'borderBottomWidth'
        | 'borderLeftWidth'
        | 'paddingTop'
        | 'paddingRight'
        | 'paddingBottom'
        | 'paddingLeft'
        | 'fontStyle'
        | 'fontVariant'
        | 'fontWeight'
        | 'fontStretch'
        | 'fontSize'
        | 'fontFamily'
        | 'lineHeight'
        | 'textAlign'
        | 'textTransform'
        | 'textIndent'
        | 'letterSpacing'
        | 'wordSpacing'
        | 'tabSize';

      const properties: CaretMirrorStyleKey[] = [
        'boxSizing',
        'width',
        'height',
        'overflowX',
        'overflowY',
        'borderTopWidth',
        'borderRightWidth',
        'borderBottomWidth',
        'borderLeftWidth',
        'paddingTop',
        'paddingRight',
        'paddingBottom',
        'paddingLeft',
        'fontStyle',
        'fontVariant',
        'fontWeight',
        'fontStretch',
        'fontSize',
        'fontFamily',
        'lineHeight',
        'textAlign',
        'textTransform',
        'textIndent',
        'letterSpacing',
        'wordSpacing',
        'tabSize',
      ];

      properties.forEach(prop => {
        mirror.style[prop] = computed[prop];
      });

      mirror.style.whiteSpace = textarea.getAttribute('wrap') === 'off' ? 'pre' : 'pre-wrap';
      mirror.style.wordWrap = textarea.getAttribute('wrap') === 'off' ? 'normal' : 'break-word';
      mirror.style.overflow = 'hidden';
      mirror.textContent = textarea.value.slice(0, selectionStart);

      const marker = document.createElement('span');
      marker.textContent = textarea.value.slice(selectionStart, selectionStart + 1) || '\u200b';
      mirror.appendChild(marker);

      const borderLeft = Number.parseFloat(computed.borderLeftWidth) || 0;
      const borderTop = Number.parseFloat(computed.borderTopWidth) || 0;
      const offsetLeft = marker.offsetLeft + borderLeft - (textarea.scrollLeft || 0);
      const offsetTop = marker.offsetTop + borderTop - (textarea.scrollTop || 0);

      mirror.textContent = '';

      return { left: offsetLeft, top: offsetTop };
    },
    [ensureCaretMirror]
  );

  const updateInlineSuggestionPosition = useCallback(() => {
    const overlay = inlineOverlayRef.current;
    const textarea = editorRef.current;
    if (!overlay || !textarea) return;
    if (!inlineSuggestion) return;

    const caret = calculateCaretOffset(textarea);
    if (!caret) return;

    const textareaRect = textarea.getBoundingClientRect();
    const docLeft = window.scrollX + textareaRect.left + caret.left;
    const docTop = window.scrollY + textareaRect.top + caret.top;

    overlay.style.transform = `translate3d(${docLeft}px, ${docTop}px, 0)`;
  }, [calculateCaretOffset, inlineSuggestion]);

  const updateFloatingSuggestionPanelPosition = useCallback(() => {
    const panel = suggestionPanelRef.current;
    const textarea = editorRef.current;
    const container = overlayHost;
    if (!panel || !textarea || !container) return;
    if (panel.classList.contains('hidden')) return;

    const textareaRect = textarea.getBoundingClientRect();
    const containerRect = container.getBoundingClientRect();
    const padding = 24;

    panel.style.width = 'auto';
    panel.style.minWidth = panel.dataset.baseWidth ? `${panel.dataset.baseWidth}px` : '260px';
    const currentWidth = panel.offsetWidth || 260;

    const containerWidth = container.clientWidth || window.innerWidth;
    const targetLeft = textareaRect.right - containerRect.left + container.scrollLeft + padding;
    const maxLeft = container.scrollLeft + containerWidth - currentWidth - padding;
    const minLeft = container.scrollLeft + padding;
    const finalLeft = Math.max(minLeft, Math.min(targetLeft, maxLeft));

    const viewportTop = container.scrollTop + padding;
    const viewportBottom = container.scrollTop + (container.clientHeight || window.innerHeight) - padding;
    let finalTop = Math.max(viewportTop, textareaRect.top - containerRect.top + container.scrollTop);

    const validationBox = document.getElementById('lab-validation-status');
    if (validationBox && !validationBox.classList.contains('hidden')) {
      const vRect = validationBox.getBoundingClientRect();
      const valBottom = vRect.bottom - containerRect.top + container.scrollTop;
      const minTopAfterValidation = valBottom + 12;
      if (finalTop < minTopAfterValidation) {
        finalTop = minTopAfterValidation;
      }
    }

    const availableHeight = Math.max(150, viewportBottom - finalTop);
    panel.style.left = `${finalLeft}px`;
    panel.style.top = `${finalTop}px`;
    panel.style.maxHeight = `${availableHeight}px`;
  }, [overlayHost]);

  const startOverlayTracking = useCallback(() => {
    if (rafRef.current != null) return;
    const step = () => {
      rafRef.current = window.requestAnimationFrame(() => {
        updateInlineSuggestionPosition();
        updateFloatingSuggestionPanelPosition();
        step();
      });
    };
    step();
  }, [updateInlineSuggestionPosition, updateFloatingSuggestionPanelPosition]);

  const stopOverlayTracking = useCallback(() => {
    if (rafRef.current == null) return;
    window.cancelAnimationFrame(rafRef.current);
    rafRef.current = null;
  }, []);

  useEffect(() => {
    setPortalBody(document.body);
    return () => {
      if (overlayHostNodeRef.current) {
        overlayHostNodeRef.current.classList.remove('pipeline-suggestion-overlay-host');
      }
    };
  }, []);

  useEffect(() => {
    if (suggestionContext?.type === 'secrets' && autocompleteMeta.secrets.length === 0 && !autocompleteMeta.loading) {
      void loadAutocomplete();
    }
    if (suggestionContext?.type === 'variables' && autocompleteMeta.variables.length === 0 && !autocompleteMeta.loading) {
      void loadAutocomplete();
    }
    if (suggestionContext?.type === 'llm_profile' && autocompleteMeta.llmProfiles.length === 0 && !autocompleteMeta.loading) {
      void loadAutocomplete();
    }
    if (suggestionContext?.type === 'mcp_profile' && autocompleteMeta.mcpProfiles.length === 0 && !autocompleteMeta.loading) {
      void loadAutocomplete();
    }
    if (suggestionContext?.type === 'include' && autocompleteMeta.reusableSteps.length === 0 && !autocompleteMeta.loading) {
      void loadAutocomplete();
    }
  }, [suggestionContext, autocompleteMeta, loadAutocomplete]);

  useEffect(() => {
    void Promise.all([loadPipelines(), loadScopes(), loadAutocomplete()]);
  }, [loadPipelines, loadScopes, loadAutocomplete]);

  useLayoutEffect(() => {
    requestAnimationFrame(() => {
      syncEditorOverlays(editorRef.current);
      updateInlineSuggestionPosition();
      updateFloatingSuggestionPanelPosition();
    });
  }, [yamlText, syncEditorOverlays, updateInlineSuggestionPosition, updateFloatingSuggestionPanelPosition]);

  useEffect(() => {
    if (!editorFocused) {
      stopOverlayTracking();
      return;
    }
    startOverlayTracking();
    return stopOverlayTracking;
  }, [editorFocused, startOverlayTracking, stopOverlayTracking]);

  useEffect(() => {
    return () => {
      if (caretMirrorRef.current) {
        caretMirrorRef.current.remove();
        caretMirrorRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    const nextCursor = pendingSelectionRef.current;
    if (nextCursor == null) return;
    pendingSelectionRef.current = null;
    requestAnimationFrame(() => {
      const textarea = editorRef.current;
      if (!textarea) return;
      textarea.focus();
      textarea.selectionStart = nextCursor;
      textarea.selectionEnd = nextCursor;
      setEditorSelection({ start: nextCursor, end: nextCursor });
      syncEditorOverlays(textarea);
    });
  }, [yamlText, syncEditorOverlays]);

  useEffect(() => {
    if (!overlayHost) return;
    const onScroll = () => updateFloatingSuggestionPanelPosition();
    const onResize = () => updateFloatingSuggestionPanelPosition();
    overlayHost.addEventListener('scroll', onScroll);
    window.addEventListener('resize', onResize);
    return () => {
      overlayHost.removeEventListener('scroll', onScroll);
      window.removeEventListener('resize', onResize);
    };
  }, [overlayHost, updateFloatingSuggestionPanelPosition]);

  useEffect(() => {
    requestAnimationFrame(() => updateFloatingSuggestionPanelPosition());
  }, [suggestionContext, suggestionItems.length, autocompleteMeta.loading, overlayHost, updateFloatingSuggestionPanelPosition]);

  const updateSelectionFromTextarea = useCallback((textarea: HTMLTextAreaElement) => {
    const rawStart = textarea.selectionStart ?? 0;
    const rawEnd = textarea.selectionEnd ?? rawStart;
    const start = Math.min(rawStart, rawEnd);
    const end = Math.max(rawStart, rawEnd);
    setEditorSelection({ start, end });
  }, []);

  const applySuggestion = useCallback(
    (item: LabSuggestionItem) => {
      if (!suggestionContext) return;

      const textarea = editorRef.current;
      const contextInfo = suggestionContext;
      const textLength = yamlText.length;
      const rangeStart = Math.max(0, Math.min(contextInfo.rangeStart ?? 0, textLength));
      const rangeEnd = Math.max(rangeStart, Math.min(contextInfo.rangeEnd ?? rangeStart, textLength));

      const before = yamlText.slice(0, rangeStart);
      const after = yamlText.slice(rangeEnd);
      const insertText = typeof item.snippet === 'string' ? item.snippet : item.value;
      const prefixInsert = contextInfo.insertPrefix || '';
      let suffix = contextInfo.insertSuffix || '';
      if (item.overrideSuffix !== undefined) {
        suffix = item.overrideSuffix;
      } else if (suffix && after.trimStart().startsWith(':')) {
        suffix = '';
      }
      const finalText = `${prefixInsert}${insertText}${suffix}`;

      setYamlText(before + finalText + after);
      const nextCursor = rangeStart + finalText.length;
      pendingSelectionRef.current = nextCursor;
      setFeedback(null);

      if (textarea) {
        textarea.focus();
      }
    },
    [setFeedback, setYamlText, suggestionContext, yamlText]
  );

  const handleIndentTab = useCallback(() => {
    const textarea = editorRef.current;
    if (!textarea) return;
    const start = textarea.selectionStart ?? 0;
    const end = textarea.selectionEnd ?? start;
    const before = yamlText.slice(0, start);
    const after = yamlText.slice(end);
    const nextValue = `${before}  ${after}`;
    setYamlText(nextValue);
    pendingSelectionRef.current = start + 2;
  }, [setYamlText, yamlText]);

  const handleAutoIndentEnter = useCallback(() => {
    const textarea = editorRef.current;
    if (!textarea) return;
    const start = textarea.selectionStart ?? 0;
    const end = textarea.selectionEnd ?? start;
    const { nextValue, nextCursor } = applyEnterIndent(yamlText, start, end);
    setYamlText(nextValue);
    pendingSelectionRef.current = nextCursor;
  }, [setYamlText, yamlText]);

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const requestedPipelineId = (params.get('pipeline') || '').trim().replace(/^\/+|\/+$/g, '');
    if (!requestedPipelineId) {
      pipelineHandoffRef.current = '';
      return;
    }
    if (pipelineHandoffRef.current === requestedPipelineId) return;
    pipelineHandoffRef.current = requestedPipelineId;

    void (async () => {
      await changePipeline(requestedPipelineId, { reload: true, resetOverrides: true });
      navigate('/lab', { replace: true });
    })();
  }, [changePipeline, location.search, navigate]);

  const handleOverlayHostRef = useCallback((node: HTMLDivElement | null) => {
    if (overlayHostNodeRef.current && overlayHostNodeRef.current !== node) {
      overlayHostNodeRef.current.classList.remove('pipeline-suggestion-overlay-host');
    }
    overlayHostNodeRef.current = node;
    if (node) {
      node.classList.add('pipeline-suggestion-overlay-host');
      setOverlayHost(node);
    } else {
      setOverlayHost(null);
    }
  }, []);

  return (
    <div data-page="lab" className="active h-full flex flex-col">
      <div className="px-6 pt-6 pb-4">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
          <div>
            <h1 className="text-3xl font-bold text-[var(--text-primary)]">Scoped pipeline runs</h1>
            <p className="text-sm text-[var(--text-secondary)] max-w-3xl">
              Pick a stored pipeline, lock in a scope, and launch it with temporary variables to validate behavior quickly.
            </p>
            {pipelinesError && <p className="text-sm text-red-500 mt-2">Failed to load pipelines: {pipelinesError}</p>}
          </div>
          <div className="flex items-center gap-2">
            <button
              id="lab-refresh-pipelines"
              type="button"
              className="glass-button-ghost"
              onClick={() => void Promise.all([loadPipelines(), loadAutocomplete(true), loadScopes()])}
              disabled={pipelinesLoading}
            >
              <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth="2"
                  d="M4 4v6h6M20 20v-6h-6M5 19a9 9 0 0113-13l1 .75M19 5v4h-4"
                />
              </svg>
              <span>{pipelinesLoading ? 'Refreshing…' : 'Refresh pipelines'}</span>
            </button>
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-auto px-6 pb-8">
        <div className="space-y-5">
          <LabRunControls
            pipelines={pipelines}
            pipelinesLoading={pipelinesLoading}
            yamlLoading={yamlLoading}
            selectedPipelineId={selectedPipelineId}
            scopeOptions={scopeOptions}
            scopeValue={scopeValue}
            runPending={runPending}
            validationErrorCount={validation.errors.length}
            accessLoading={runValidation.loading}
            accessError={runValidation.error}
            accessBlocked={runValidationBlocked}
            accessChecks={runValidation.checks}
            feedback={feedback}
            onPipelineChange={pipelineId => changePipeline(pipelineId)}
            onScopeChange={setScopeValue}
            onRun={() => void handleRun()}
          />

          <div className="grid grid-cols-1 lg:grid-cols-4 gap-5">
            <div className="glass-card p-6 space-y-5 rounded-2xl shadow-lg ring-1 ring-[var(--border-primary)]/70 lg:col-span-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-3">
                  <div>
                    <h3 className="text-lg font-semibold text-[var(--text-primary)]">Pipeline definition</h3>
                    <p className="text-sm text-[var(--text-secondary)]">Edit and validate before you launch.</p>
                  </div>
                </div>
                <div className="flex items-center flex-wrap gap-2">
                  <button
                    id="lab-save-yaml"
                    type="button"
                    className="glass-button-primary"
                    title="Save this YAML for the current lab session (pipelines stay unchanged)."
                    onClick={() => saveSession(validation.errors.length)}
                    disabled={validation.errors.length > 0}
                  >
                    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M5 13l4 4L19 7" />
                    </svg>
                    <span>Save for Lab</span>
                  </button>
                </div>
              </div>

              <div className="space-y-4">
                <div id="lab-editor-wrapper" ref={handleOverlayHostRef} className="pipeline-editor-wrapper lab-editor-wrapper">
                  <div id="lab-editor-container" className="editor-container">
                    <div id="lab-line-numbers" ref={lineNumbersRef}>
                      <div className="line-number-track">
                        {editorLines.map((_, idx) => {
                          const lineNum = idx + 1;
                          const errors = errorMap.get(lineNum);
                          return (
                            <div
                              key={`ln-${idx}`}
                              className={`line-number ${validationErrorLines.has(lineNum) ? 'line-number--error' : ''}`}
                              title={errors ? errors.join('\n') : undefined}
                            >
                              {lineNum}
                            </div>
                          );
                        })}
                      </div>
                    </div>

                    <div id="lab-yaml-stage" className="yaml-editor-stage yaml-editor-stage--with-highlight">
                      <div className="yaml-editor-highlight" aria-hidden="true">
                        <pre id="lab-yaml-highlight" ref={highlightContentRef} className="yaml-editor-highlight__content">
                          {renderYamlHighlight(yamlText)}
                        </pre>
                      </div>
                      <textarea
                        ref={editorRef}
                        id="lab-yaml-editor"
                        spellCheck={false}
                        value={yamlText}
                        onFocus={event => {
                          setEditorFocused(true);
                          updateSelectionFromTextarea(event.currentTarget);
                        }}
                        onBlur={() => setEditorFocused(false)}
                        onChange={event => {
                          setYamlText(event.target.value);
                          updateSelectionFromTextarea(event.target);
                        }}
                        onScroll={handleEditorScroll}
                        onSelect={event => updateSelectionFromTextarea(event.currentTarget)}
                        onKeyUp={event => updateSelectionFromTextarea(event.currentTarget)}
                        onClick={event => updateSelectionFromTextarea(event.currentTarget)}
                        onKeyDown={event => {
                          if (event.key === 'Tab') {
                            event.preventDefault();
                            if (suggestionContext && suggestionItems.length > 0 && inlineSuggestion) {
                              applySuggestion(suggestionItems[0]);
                              return;
                            }
                            handleIndentTab();
                            return;
                          }

                          if (event.key === 'Enter' && !event.shiftKey && !event.ctrlKey) {
                            event.preventDefault();
                            handleAutoIndentEnter();
                          }
                        }}
                      ></textarea>
                    </div>
                  </div>

                  <div className="lab-side-panel">
                    <div
                      id="lab-validation-status"
                      className={`validation-box validation-box--inline ${validation.errors.length ? 'validation-box--error' : 'validation-box--success'}`}
                    >
                      {validation.errors.length ? (
                        <>
                          <div className="validation-box__header">Validation issues</div>
                          {validation.errors.slice(0, 5).map((err, idx) => {
                            const example = buildValidationExample(err.message);
                            return (
                              <div key={`val-${idx}`} className="validation-box__item">
                                {typeof err.line === 'number' && <span className="validation-box__line">Line {err.line}</span>}
                                <div className="validation-box__message">{err.message}</div>
                                {example && (
                                  <pre className="validation-box__example">
                                    <code>{example}</code>
                                  </pre>
                                )}
                              </div>
                            );
                          })}
                          {validation.errors.length > 5 && (
                            <div className="validation-box__item">
                              <div className="validation-box__message">+ {validation.errors.length - 5} more…</div>
                            </div>
                          )}
                        </>
                      ) : (
                        <div className="validation-box__header">Valid</div>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div className="space-y-5">
              <div className="glass-card p-4 space-y-2 rounded-2xl shadow-lg ring-1 ring-[var(--border-primary)]/70">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-sm font-semibold text-[var(--text-primary)]">Included dependencies</p>
                </div>
                <div
                  id="lab-includes"
                  className="text-sm text-[var(--text-secondary)] space-y-2"
                  data-empty="No steps defined yet."
                >
                  {includedDependencies.status === 'no-steps' ? (
                    <p>No steps defined yet.</p>
                  ) : includedDependencies.status === 'parse-error' || includedDependencies.status === 'invalid' ? (
                    <p>Unable to parse pipeline YAML.</p>
                  ) : includedDependencies.items.length === 0 ? (
                    <p>No included dependencies found.</p>
                  ) : (
                    <ul className="triggers-pipeline-list">
                      {includedDependencies.items.map(item => (
                        <li key={item} className="triggers-pipeline-item">
                          <span className="triggers-pipeline-name">{item}</span>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </div>

              <div className="glass-card p-5 space-y-4 rounded-2xl ring-1 ring-[var(--border-primary)]/70">
                <div className="flex items-start justify-between gap-3">
                  <div className="space-y-1">
                    <h3 className="text-lg font-semibold text-[var(--text-primary)] leading-tight" style={{ paddingTop: 10 }}>
                      Variable overrides
                    </h3>
                  </div>
                  <button id="lab-add-override" type="button" className="glass-button-primary" onClick={addOverride}>
                    <span className="lab-override-add__icon" aria-hidden="true">
                      +
                    </span>
                  </button>
                </div>

                <div className="lab-overrides-panel">
                  <div id="lab-overrides-empty" className={`lab-overrides-empty ${overrides.length > 0 ? 'hidden' : ''}`}>
                    No overrides yet. Leave it blank to inherit scope defaults.
                  </div>
                  <div id="lab-overrides-list" className="lab-overrides-list">
                    {overrides.map(row => (
                      <div key={row.id} className="lab-override-row">
                        <div className="lab-override-field">
                          <input
                            className="pipelines-input lab-override-input"
                            placeholder="key"
                            value={row.key}
                            onChange={event => updateOverride(row.id, 'key', event.target.value)}
                          />
                        </div>
                        <div className="lab-override-field">
                          <input
                            className="pipelines-input lab-override-input"
                            placeholder="value"
                            value={row.value}
                            onChange={event => updateOverride(row.id, 'value', event.target.value)}
                          />
                        </div>
                        <button
                          type="button"
                          className="lab-override-remove"
                          onClick={() => removeOverride(row.id)}
                          aria-label="Remove override"
                        >
                          <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
                          </svg>
                        </button>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {portalBody &&
        createPortal(
          <div
            id="lab-editor-autocomplete"
            ref={inlineOverlayRef}
            className={`pipeline-editor-autocomplete ${inlineSuggestion ? '' : 'hidden'}`}
            aria-hidden="true"
          >
            <span id="lab-editor-autocomplete-ghost" className="pipeline-editor-autocomplete__ghost">
              {inlineSuggestion}
            </span>
          </div>,
          portalBody
        )}

      {overlayHost &&
        createPortal(
          <section
            id="lab-suggestion-panel"
            ref={suggestionPanelRef}
            className="scope-suggestion-panel pipeline-suggestion-panel pipeline-suggestion-overlay"
            aria-live="polite"
            data-base-width="260"
          >
            <div className="scope-suggestion-heading">
              <div>
                <h3 id="lab-suggestion-title" className="scope-suggestion-title">
                  {suggestionContext?.title || suggestionCopy.title}
                </h3>
              </div>
              <p id="lab-suggestion-subtitle" className="scope-suggestion-subtitle">
                {suggestionCopy.subtitle}
              </p>
            </div>

            <div className="scope-suggestion-body">
              <p id="lab-suggestion-empty" className={`scope-suggestion-empty ${suggestionItems.length ? 'hidden' : ''}`}>
                {autocompleteMeta.loading ? 'Loading suggestions…' : 'No suggestions available yet.'}
              </p>
              <div id="lab-suggestion-list" className={`scope-suggestion-list ${suggestionItems.length ? '' : 'hidden'}`}>
                {suggestionItems.length > 0 && (
                  <article className="scope-suggestion-item">
                    <div className="scope-suggestion-scope">
                      <span className="scope-suggestion-scope-label">{suggestionContext?.title || suggestionCopy.title}</span>
                      <span className="scope-suggestion-scope-count">{suggestionItems.length} items</span>
                    </div>
                    <div className="scope-suggestion-variables">
                      {suggestionItems.map(item => (
                        <button
                          key={`${item.value}`}
                          type="button"
                          className="scope-suggestion-pill scope-suggestion-pill--action"
                          onClick={() => applySuggestion(item)}
                        >
                          <span>{item.label ?? item.value}</span>
                          {item.hint && <span className="scope-suggestion-hint">{item.hint}</span>}
                        </button>
                      ))}
                    </div>
                  </article>
                )}
              </div>
            </div>
          </section>,
          overlayHost
        )}
    </div>
  );
}

export default LabPage;
