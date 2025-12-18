import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type UIEvent } from 'react';
import { createPortal } from 'react-dom';
import { NavLink } from 'react-router-dom';
import yaml from 'js-yaml';
import { buildApiUrl } from '../lib/api';
import {
  DEFAULT_PIPELINE_NAME,
  applyEnterIndent,
  buildSuggestionItems,
  buildValidationExample,
  detectSuggestionContext,
  suggestionCopyForContext,
  validateOverrideKey,
  validatePipelineYamlStrict,
  type LabSuggestionContext,
  type LabSuggestionItem,
} from '../lib/lab';
import { renderYamlHighlight } from '../lib/yamlRenderer';

type PipelineListItem = { id: string; source?: string };

type OverrideRow = { id: number; key: string; value: string };

type LabFeedback = { tone: 'success' | 'error' | 'info'; message: string; runId?: string } | null;

type LabSessionState = {
  version: 1;
  selectedPipelineId: string;
  yaml: string;
  originalYaml: string;
  scope: string;
  overrides: Array<{ key: string; value: string }>;
};

const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;
const LAB_SESSION_STORAGE_KEY = 'nopsai.lab.session.v1';

function buildBlankYaml(name = DEFAULT_PIPELINE_NAME) {
  return [
    `name: ${name}`,
    'version: latest',
    'description: Lab pipeline (ad-hoc)',
    'steps:',
    '  - name: hello',
    '    script: echo "Hello from Lab"',
    '',
  ].join('\n');
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

function encodeId(id: string) {
  return id.split('/').map(encodeURIComponent).join('/');
}

function parsePipelineName(yamlText: string): string {
  const match = yamlText.match(/^\s*name:\s*([^\s]+)/m);
  return match ? match[1] : '';
}

function extractRunId(payload: unknown): string {
  if (!payload) return '';
  if (typeof payload === 'string') {
    const fullMatch = payload.match(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i);
    if (fullMatch) return fullMatch[0];
    const shortMatch = payload.match(/[0-9a-f]{8}-[0-9a-f]{4}/i);
    return shortMatch ? shortMatch[0] : '';
  }
  if (typeof payload === 'object') {
    const record = payload as Record<string, unknown>;
    const runId = record.run_id ?? record.id ?? '';
    return typeof runId === 'string' ? runId : '';
  }
  return '';
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
  const initialSession = useMemo<LabSessionState | null>(() => {
    if (typeof window === 'undefined') return null;
    try {
      const raw = sessionStorage.getItem(LAB_SESSION_STORAGE_KEY);
      if (!raw) return null;
      const parsed = JSON.parse(raw) as Partial<LabSessionState>;
      if (!parsed || parsed.version !== 1) return null;
      if (typeof parsed.yaml !== 'string') return null;
      return {
        version: 1,
        selectedPipelineId: typeof parsed.selectedPipelineId === 'string' ? parsed.selectedPipelineId : '',
        yaml: parsed.yaml,
        originalYaml: typeof parsed.originalYaml === 'string' ? parsed.originalYaml : parsed.yaml,
        scope: typeof parsed.scope === 'string' ? parsed.scope : '',
        overrides: Array.isArray(parsed.overrides)
          ? parsed.overrides.map(row => ({ key: typeof row?.key === 'string' ? row.key : '', value: typeof row?.value === 'string' ? row.value : '' }))
          : [],
      };
    } catch {
      return null;
    }
  }, []);

  const [pipelines, setPipelines] = useState<PipelineListItem[]>([]);
  const [pipelinesLoading, setPipelinesLoading] = useState(false);
  const [pipelinesError, setPipelinesError] = useState<string | null>(null);

  const [scopes, setScopes] = useState<string[]>([]);
  const [scopeValue, setScopeValue] = useState(initialSession?.scope ?? '');

  const [autocompleteMeta, setAutocompleteMeta] = useState<{
    secrets: string[];
    variables: string[];
    reusableSteps: string[];
    fetchedAt: number;
    loading: boolean;
  }>({ secrets: [], variables: [], reusableSteps: [], fetchedAt: 0, loading: false });
  const autocompleteFetchRef = useRef<{ fetchedAt: number; loadingPromise: Promise<void> | null }>({ fetchedAt: 0, loadingPromise: null });

  const [selectedPipelineId, setSelectedPipelineId] = useState(initialSession?.selectedPipelineId ?? '');
  const [yamlText, setYamlText] = useState(initialSession?.yaml ?? buildBlankYaml());
  const [originalYaml, setOriginalYaml] = useState(initialSession?.originalYaml ?? (initialSession?.yaml ?? buildBlankYaml()));

  const overrideIdRef = useRef(0);
  const [overrides, setOverrides] = useState<OverrideRow[]>(() => {
    const seed = initialSession?.overrides ?? [];
    return seed.map((row, idx) => ({ id: idx + 1, key: row.key, value: row.value }));
  });

  const [feedback, setFeedback] = useState<LabFeedback>(null);
  const [runPending, setRunPending] = useState(false);
  const [yamlLoading, setYamlLoading] = useState(false);

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

  const hasUnsavedChanges = useMemo(() => (yamlText || '') !== (originalYaml || ''), [yamlText, originalYaml]);

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
        const [secretsResp, varsResp, stepsResp] = await Promise.all([
          fetch(buildApiUrl('/v1/secrets')).then(r => (r.ok ? r.json() : [])),
          fetch(buildApiUrl('/v1/variables')).then(r => (r.ok ? r.json() : [])),
          fetch(buildApiUrl('/v1/steps')).then(r => (r.ok ? r.json() : [])),
        ]);

        setAutocompleteMeta({
          secrets: normalizeList(secretsResp),
          variables: normalizeList(varsResp),
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
  }, []);

  const loadPipelines = useCallback(async () => {
    setPipelinesLoading(true);
    setPipelinesError(null);
    try {
      const response = await fetch(buildApiUrl('/v1/pipelines?include_source=true'));
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
      const response = await fetch(buildApiUrl('/v1/variables/scopes'));
      if (!response.ok) return;
      const payload = await response.json();
      const list = Array.isArray(payload) ? payload : [];
      const normalized = list.map(item => (typeof item === 'string' ? item.trim() : '')).filter(Boolean);
      normalized.sort((a: string, b: string) => a.localeCompare(b));
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
    overrideIdRef.current = Math.max(overrideIdRef.current, overrides.reduce((max, row) => Math.max(max, row.id), 0));
  }, [overrides]);

  useEffect(() => {
    if (suggestionContext?.type === 'secrets' && autocompleteMeta.secrets.length === 0 && !autocompleteMeta.loading) {
      void loadAutocomplete();
    }
    if (suggestionContext?.type === 'variables' && autocompleteMeta.variables.length === 0 && !autocompleteMeta.loading) {
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
    [suggestionContext, yamlText]
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
  }, [yamlText]);

  const handleAutoIndentEnter = useCallback(() => {
    const textarea = editorRef.current;
    if (!textarea) return;
    const start = textarea.selectionStart ?? 0;
    const end = textarea.selectionEnd ?? start;
    const { nextValue, nextCursor } = applyEnterIndent(yamlText, start, end);
    setYamlText(nextValue);
    pendingSelectionRef.current = nextCursor;
  }, [yamlText]);

  const handleSaveSession = useCallback(() => {
    if (validation.errors.length > 0) {
      setFeedback({ tone: 'error', message: 'Fix validation errors before saving.' });
      return;
    }
    setOriginalYaml(yamlText);
    try {
      const payload: LabSessionState = {
        version: 1,
        selectedPipelineId,
        yaml: yamlText,
        originalYaml: yamlText,
        scope: scopeValue,
        overrides: overrides.map(row => ({ key: row.key, value: row.value })),
      };
      sessionStorage.setItem(LAB_SESSION_STORAGE_KEY, JSON.stringify(payload));
    } catch (error) {
      console.warn('Unable to save Lab session state', error);
    }
    setFeedback({ tone: 'success', message: 'YAML saved for this lab session (pipelines unchanged).' });
  }, [validation.errors.length, yamlText, selectedPipelineId, scopeValue, overrides]);

  const handleRun = useCallback(async () => {
    if (validation.errors.length > 0) {
      setFeedback({ tone: 'error', message: 'Fix validation errors first.' });
      return;
    }

    const overridesObject: Record<string, string> = {};
    for (const row of overrides) {
      const key = row.key.trim();
      if (!key) continue;
      if (!validateOverrideKey(key)) {
        setFeedback({ tone: 'error', message: `Invalid override key '${key}'. Use only letters, numbers, underscore, dot, hyphen.` });
        return;
      }
      overridesObject[key] = row.value;
    }

    const parsedName = parsePipelineName(yamlText);
    const pipeline = selectedPipelineId || parsedName || DEFAULT_PIPELINE_NAME;

    const payload: Record<string, unknown> = {
      pipeline,
      definition: yamlText,
    };
    if (Object.keys(overridesObject).length > 0) payload.variables = overridesObject;
    if (scopeValue) payload.scope = scopeValue;

    setRunPending(true);
    setFeedback(null);
    try {
      const headers: Record<string, string> = { 'Content-Type': 'application/json' };
      if (scopeValue) headers['X-Nopsai-Scope'] = scopeValue;

      const response = await fetch(buildApiUrl('/v1/run'), {
        method: 'POST',
        headers,
        body: JSON.stringify(payload),
      });

      const contentType = response.headers.get('content-type') || '';
      const body = contentType.includes('application/json') ? await response.json() : await response.text();

      if (!response.ok) {
        const message = typeof body === 'string' ? body : JSON.stringify(body);
        throw new Error(message || `Run failed (${response.status})`);
      }

      const runId = extractRunId(body);
      setFeedback({ tone: 'success', message: 'Run started!', runId: runId || undefined });
    } catch (error) {
      setFeedback({ tone: 'error', message: `Run failed: ${error instanceof Error ? error.message : 'Unknown error'}` });
    } finally {
      setRunPending(false);
    }
  }, [validation.errors.length, overrides, yamlText, selectedPipelineId, scopeValue]);

  const handlePipelineChange = useCallback(
    async (nextId: string) => {
      if (nextId === selectedPipelineId) return;
      if (hasUnsavedChanges && !window.confirm('Discard your current Lab edits?')) {
        return;
      }

      setFeedback(null);
      setYamlLoading(true);
      setSelectedPipelineId(nextId);
      try {
        if (!nextId) {
          const blank = buildBlankYaml();
          setYamlText(blank);
          setOriginalYaml(blank);
          return;
        }

        const response = await fetch(buildApiUrl(`/v1/pipelines/${encodeId(nextId)}`));
        if (!response.ok) {
          const text = await response.text();
          throw new Error(text || `Failed to load pipeline YAML (${response.status})`);
        }
        const rawYaml = await response.text();
        setYamlText(rawYaml);
        setOriginalYaml(rawYaml);
      } catch (error) {
        console.error('Failed to load pipeline YAML', error);
        setFeedback({ tone: 'error', message: error instanceof Error ? error.message : 'Unable to load pipeline YAML' });
      } finally {
        setYamlLoading(false);
      }
    },
    [selectedPipelineId, hasUnsavedChanges]
  );

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

  const handleAddOverride = useCallback(() => {
    overrideIdRef.current += 1;
    setOverrides(prev => [...prev, { id: overrideIdRef.current, key: '', value: '' }]);
  }, []);

  const handleOverrideChange = useCallback((id: number, field: 'key' | 'value', value: string) => {
    setOverrides(prev => prev.map(row => (row.id === id ? { ...row, [field]: value } : row)));
  }, []);

  const handleRemoveOverride = useCallback((id: number) => {
    setOverrides(prev => prev.filter(row => row.id !== id));
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
          <div className="glass-card p-6 space-y-5 rounded-2xl shadow-lg ring-1 ring-[var(--border-primary)]/70 bg-gradient-to-br from-[var(--bg-secondary)] to-[var(--bg-tertiary)]">
            <div className="grid grid-cols-1 md:grid-cols-[1fr_1fr_auto] gap-4 items-end">
              <div>
                <label htmlFor="lab-pipeline-select" className="block text-sm font-medium text-[var(--text-secondary)]">
                  Pipeline
                </label>
                <select
                  id="lab-pipeline-select"
                  className="mt-1 block w-full pipelines-input py-2 px-3 text-sm"
                  aria-label="Pipeline selection"
                  value={selectedPipelineId}
                  disabled={pipelinesLoading || yamlLoading}
                  onChange={event => void handlePipelineChange(event.target.value)}
                >
                  <option value="">Select a pipeline</option>
                  {pipelines.map(item => (
                    <option key={item.id} value={item.id}>
                      {item.id}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label htmlFor="lab-scope-input" className="block text-sm font-medium text-[var(--text-secondary)]">
                  Target scope
                </label>
                <select
                  id="lab-scope-input"
                  className="mt-1 block w-full pipelines-input py-2 px-3 text-sm"
                  aria-label="Target scope selection"
                  value={scopeValue}
                  onChange={event => setScopeValue(event.target.value)}
                >
                  <option value="">Default scope</option>
                  {scopes.map(scope => (
                    <option key={scope} value={scope}>
                      {scope}
                    </option>
                  ))}
                </select>
              </div>

              <div className="flex flex-wrap items-center gap-2 justify-start md:justify-end">
                <button
                  id="lab-run-btn"
                  type="button"
                  className="glass-button-primary"
                  onClick={() => void handleRun()}
                  disabled={runPending || yamlLoading || validation.errors.length > 0}
                >
                  <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M5 12h4l1-5 4 10 1-5h4" />
                  </svg>
                  <span>{runPending ? 'Running…' : 'Run'}</span>
                </button>
              </div>
            </div>

            <div
              id="lab-run-feedback"
              className={`text-sm ${feedback ? '' : 'hidden'} ${
                feedback?.tone === 'error'
                  ? 'text-red-500'
                  : feedback?.tone === 'success'
                    ? 'text-green-500'
                    : 'text-[var(--text-secondary)]'
              }`}
            >
              {feedback && (
                <>
                  <span>{feedback.message}</span>
                  {feedback.runId && (
                    <>
                      {' '}
                      <NavLink className="underline" to="/pipelineruns/main">
                        View
                      </NavLink>
                    </>
                  )}
                </>
              )}
            </div>
          </div>

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
                    onClick={handleSaveSession}
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
                  <button id="lab-add-override" type="button" className="glass-button-primary" onClick={handleAddOverride}>
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
                            onChange={event => handleOverrideChange(row.id, 'key', event.target.value)}
                          />
                        </div>
                        <div className="lab-override-field">
                          <input
                            className="pipelines-input lab-override-input"
                            placeholder="value"
                            value={row.value}
                            onChange={event => handleOverrideChange(row.id, 'value', event.target.value)}
                          />
                        </div>
                        <button
                          type="button"
                          className="lab-override-remove"
                          onClick={() => handleRemoveOverride(row.id)}
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
            className="env-suggestion-panel pipeline-suggestion-panel pipeline-suggestion-overlay"
            aria-live="polite"
            data-base-width="260"
          >
            <div className="env-suggestion-heading">
              <div>
                <h3 id="lab-suggestion-title" className="env-suggestion-title">
                  {suggestionContext?.title || suggestionCopy.title}
                </h3>
              </div>
              <p id="lab-suggestion-subtitle" className="env-suggestion-subtitle">
                {suggestionCopy.subtitle}
              </p>
            </div>

            <div className="env-suggestion-body">
              <p id="lab-suggestion-empty" className={`env-suggestion-empty ${suggestionItems.length ? 'hidden' : ''}`}>
                {autocompleteMeta.loading ? 'Loading suggestions…' : 'No suggestions available yet.'}
              </p>
              <div id="lab-suggestion-list" className={`env-suggestion-list ${suggestionItems.length ? '' : 'hidden'}`}>
                {suggestionItems.length > 0 && (
                  <article className="env-suggestion-item">
                    <div className="env-suggestion-env">
                      <span className="env-suggestion-env-label">{suggestionContext?.title || suggestionCopy.title}</span>
                      <span className="env-suggestion-env-count">{suggestionItems.length} items</span>
                    </div>
                    <div className="env-suggestion-variables">
                      {suggestionItems.map(item => (
                        <button
                          key={`${item.value}`}
                          type="button"
                          className="env-suggestion-pill env-suggestion-pill--action"
                          onClick={() => applySuggestion(item)}
                        >
                          <span>{item.label ?? item.value}</span>
                          {item.hint && <span className="env-suggestion-hint">{item.hint}</span>}
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
