(function (global) {
    // --- 1. Configuration & Constants ---
    const PIPELINE_DIRECTIVES = [
        { key: 'name', hint: 'Pipeline display name' },
        { key: 'version', hint: 'Pipeline schema version' },
        { key: 'description', hint: 'Human readable summary' },
        { key: 'container_image', hint: 'Default container image' },
        { key: 'working_directory', hint: 'Default working directory' },
        { key: 'variables', hint: 'Global variables' },
        { key: 'steps', hint: 'List pipeline steps' },
        { key: 'timeout', hint: 'Pipeline timeout' },
        { key: 'llm_output_sharing', hint: 'Share LLM outputs across steps' },
        { key: 'llm_content_sharing', hint: 'Share LLM prompts across steps' },
        { key: 'llm_content_ignore', hint: 'Paths excluded from LLM context' },
        { key: 'display_options', hint: 'UI rendering preferences' },
    ];
    const STEP_DIRECTIVES = [
        { key: 'name', hint: 'Step name' },
        { key: 'include', hint: 'Include reusable step' },
        { key: 'sync', hint: 'Run step synchronously' },
        { key: 'image', hint: 'Override container image' },
        { key: 'secrets', hint: 'Step secrets' },
        { key: 'volumes', hint: 'Step volumes' },
        { key: 'variables', hint: 'Step variables' },
        { key: 'tasks', hint: 'Nested task list' },
        { key: 'condition', hint: 'Conditional execution' },
        { key: 'goal', hint: 'LLM goal prompt' },
        { key: 'script', hint: 'Shell script body' },
        { key: 'depends_on', hint: 'Upstream steps' },
        { key: 'ignore_failure', hint: 'Ignore failures' },
        { key: 'llm_output_sharing', hint: 'Share step LLM output' },
    ];
    const TASK_DIRECTIVES = [
        { key: 'name', hint: 'Task name' },
        { key: 'goal', hint: 'Task goal prompt' },
        { key: 'script', hint: 'Task script body' },
        { key: 'depends_on', hint: 'Dependent tasks' },
        { key: 'ignore_failure', hint: 'Ignore task errors' },
        { key: 'llm_output_sharing', hint: 'Share task LLM output' },
    ];
    const DIRECTIVE_VALUE_METADATA = {
        llm_output_sharing: { values: ['true', 'false'], title: 'Boolean value' },
        llm_content_sharing: { values: ['true', 'false'], title: 'Boolean value' },
        ignore_failure: { values: ['true', 'false'], title: 'Boolean value' },
        sync: { values: ['true', 'false'], title: 'Boolean value' },
    };
    const LIST_KEYS_WITH_NAME_TEMPLATE = new Set(['steps', 'tasks']);
    const LIST_KEYS_SIMPLE = new Set(['secrets', 'volumes', 'depends_on', 'artifacts', 'variables', 'llm_content_ignore']);
    const ARRAY_KEYS = new Set(['steps', 'tasks', 'variables', 'secrets', 'volumes', 'depends_on', 'artifacts', 'llm_content_ignore']);

    // --- 2. State Management ---
    const state = {
        pipelines: [],
        pipelineSources: new Map(),
        pipelinesLoaded: false,
        scopes: [],
        scopesLoaded: false,
        overrides: [],
        selectedPipeline: '',
        scopeValue: '',
        currentYaml: '',
        originalYaml: '',
        isRunning: false,
        isLoadingYaml: false,
        lastFeedback: null,
        validationErrors: [],
        hasUnsavedChanges: false,
        isEditing: false,
        lastRouteHash: '#/lab',
        beforeUnloadHandler: null,
        suggestionPanelFloating: false,
        suggestionPanelOriginalParent: null,
        suggestionPanelOriginalNextSibling: null,
        suggestionPanelOverlayContainer: null,
        suggestionPanelPositionHandler: null,
        editorSuggestionItems: [],
        editorSuggestionIndex: -1,
        editorSuggestionAnimationFrame: null,
        lastEditorSelection: null,
        autocomplete: {
            secrets: [],
            variables: [],
            reusableSteps: [],
            fetchedAt: 0,
            isLoading: false,
        },
        editorSuggestionContext: null,
        // Legacy/Fallback
        variableSuggestions: [],
        variableSuggestionsLoadedAt: 0,
        variableSuggestionCache: new Map(),
        variableSuggestionPromise: null,
    };

    const OVERRIDE_KEY_PATTERN = /^[A-Za-z0-9_.-]+$/;
    const DEFAULT_PIPELINE_NAME = 'ad-hoc-pipeline';

    const DOM = {};
    let context = null;
    let initialized = false;
    let overrideSeq = 0;
    let textareaCaretMirror = null;

    function init(ctx) {
        if (initialized && context === ctx) return;
        context = ctx;
        mapDom();
        attachEvents();
        initialized = true;
    }

    function mapDom() {
        const ids = [
            'lab-pipeline-select', 'lab-refresh-pipelines', 'lab-open-pipeline',
            'lab-scope-input',
            'lab-yaml-editor', 'lab-yaml-highlight', 'lab-line-numbers', 'lab-yaml-stage',
            'lab-editor-wrapper', 'lab-overrides-list', 'lab-overrides-empty', 'lab-add-override',
            'lab-summary-pipeline', 'lab-summary-scope', 'lab-summary-overrides',
            'lab-run-btn', 'lab-run-feedback', 'lab-save-yaml',
            'lab-validation-status', 'lab-includes',
            'lab-suggestion-panel', 'lab-suggestion-list', 'lab-suggestion-empty',
            'lab-suggestion-title', 'lab-suggestion-subtitle', 'lab-suggestion-footnote'
        ];
        ids.forEach(id => { DOM[id] = document.getElementById(id); });
    }

    function attachEvents() {
        if (DOM['lab-pipeline-select']) DOM['lab-pipeline-select'].addEventListener('change', handlePipelineChange);
        if (DOM['lab-scope-input']) DOM['lab-scope-input'].addEventListener('change', handleScopeInput);
        if (DOM['lab-add-override']) DOM['lab-add-override'].addEventListener('click', () => addOverride());
        if (DOM['lab-refresh-pipelines']) DOM['lab-refresh-pipelines'].addEventListener('click', () => loadPipelines(true));
        if (DOM['lab-run-btn']) DOM['lab-run-btn'].addEventListener('click', handleRunClick);
        if (DOM['lab-save-yaml']) DOM['lab-save-yaml'].addEventListener('click', saveLabYaml);
        
        if (DOM['lab-yaml-editor']) {
            const editor = DOM['lab-yaml-editor'];
            editor.addEventListener('input', handleEditorInput);
            editor.addEventListener('scroll', () => {
                syncEditorScroll();
                updateInlineSuggestionPosition();
            });
            editor.addEventListener('keydown', handleEditorKeydown);
            editor.addEventListener('click', () => { updateSuggestionPanel(); });
            editor.addEventListener('keyup', (e) => {
                if (!['Shift', 'Control', 'Alt', 'Meta'].includes(e.key)) updateSuggestionPanel();
            });
        }
        if (DOM['lab-suggestion-panel']) {
            DOM['lab-suggestion-panel'].addEventListener('click', handleSuggestionClick);
        }
    }

    async function handleRoute() {
        if (!initialized) init(context);
        clearFeedback();
        renderOverrides();
        renderSummary();
        state.lastRouteHash = window.location.hash || state.lastRouteHash || '#/lab';
        
        await Promise.all([
            loadPipelines(), 
            loadScopes(),
            preloadAutocompleteMetadata(),
            ensureVariableSuggestionData().catch(() => [])
        ]);

        if (!state.currentYaml && state.selectedPipeline) {
            await loadPipelineYaml(state.selectedPipeline);
        } else if (!state.currentYaml) {
            startBlankPipeline();
        }

        enterEditMode();
        handleValidation();
        updateSuggestionPanel();
        updateFloatingSuggestionPanelPosition();
    }

    // --- 3. Data Loading ---

    async function preloadAutocompleteMetadata() {
        if (!context || typeof context.fetchData !== 'function') return;
        if (Date.now() - state.autocomplete.fetchedAt < 5 * 60 * 1000) return;

        state.autocomplete.isLoading = true;
        try {
            const [secrets, vars, steps] = await Promise.all([
                context.fetchData('/v1/secrets'),
                context.fetchData('/v1/variables'),
                context.fetchData('/v1/steps'),
            ]);
            
            const normalize = (list) => Array.isArray(list) ? list.map(i => typeof i === 'string' ? i.trim() : '').filter(Boolean) : [];
            
            state.autocomplete.secrets = normalize(secrets);
            state.autocomplete.variables = normalize(vars);
            state.autocomplete.reusableSteps = normalize(steps);
            state.autocomplete.fetchedAt = Date.now();
        } catch (error) {
            console.error('Failed to load autocomplete metadata:', error);
        } finally {
            state.autocomplete.isLoading = false;
            updateSuggestionPanel();
        }
    }

    async function fetchVariableScopeLabels() {
        const labels = new Set(['']);
        if (!context || typeof context.fetchData !== 'function') {
            return Array.from(labels);
        }
        try {
            const response = await context.fetchData('/v1/variables/scopes');
            if (Array.isArray(response)) {
                response.forEach(entry => {
                    const normalized = normalizeVariableScopeLabel(entry);
                    if (normalized !== null && normalized !== undefined) {
                        labels.add(normalized);
                    }
                });
            }
        } catch (error) {
            console.error('Failed to fetch variable scope list for suggestions:', error);
        }
        return Array.from(labels).sort((a, b) => {
            if (a === b) return 0;
            if (a === '') return -1;
            if (b === '') return 1;
            return a.localeCompare(b, undefined, { sensitivity: 'base' });
        });
    }

    function normalizeVariableScopeLabel(entry) {
        if (entry == null) return '';
        if (typeof entry === 'string') {
            return String(entry).trim().replace(/^\/+|\/+$/g, '');
        }
        if (typeof entry === 'object') {
            const value = entry.scope ?? entry.env ?? entry.name ?? entry.value ?? '';
            return String(value || '').trim().replace(/^\/+|\/+$/g, '');
        }
        return '';
    }

    async function fetchVariablesForLabel(label, force = false) {
        const normalized = typeof label === 'string' ? label : '';
        if (!force && state.variableSuggestionCache instanceof Map && state.variableSuggestionCache.has(normalized)) {
            return state.variableSuggestionCache.get(normalized);
        }
        if (!context || typeof context.fetchData !== 'function') {
            return [];
        }

        const baseUrl = normalized ? `/v1/variables?env=${encodeURIComponent(normalized)}` : '/v1/variables';
        const url = baseUrl.includes('?') ? `${baseUrl}&include_source=true` : `${baseUrl}?include_source=true`;
        try {
            const response = await context.fetchData(url);
            const entries = Array.isArray(response) ? response : [];
            const variables = entries.map(item => {
                if (typeof item === 'string') {
                    return item.trim();
                }
                if (item && typeof item === 'object' && item.name) {
                    return String(item.name).trim();
                }
                return '';
            }).filter(Boolean).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
            if (!(state.variableSuggestionCache instanceof Map)) {
                state.variableSuggestionCache = new Map();
            }
            state.variableSuggestionCache.set(normalized, variables);
            return variables;
        } catch (error) {
            console.error(`Failed to load variables for '/${normalized || ''}':`, error);
            return [];
        }
    }

    async function ensureVariableSuggestionData(force = false) {
        if (!context || typeof context.fetchData !== 'function') {
            return [];
        }
        if (!force && state.variableSuggestions.length) {
            return state.variableSuggestions;
        }
        if (state.variableSuggestionPromise) {
            return state.variableSuggestionPromise;
        }

        const promise = (async () => {
            const labels = await fetchVariableScopeLabels();
            const summaries = [];
            for (const label of labels) {
                const variables = await fetchVariablesForLabel(label, force);
                summaries.push({
                    key: `env:${label || ''}`,
                    label: label ? `/${label}` : '/ (default)',
                    count: variables.length,
                    preview: variables.slice(0, 5),
                });
            }
            state.variableSuggestions = summaries;
            state.variableSuggestionsLoadedAt = Date.now();
            return summaries;
        })();

        state.variableSuggestionPromise = promise;
        try {
            const result = await promise;
            updateSuggestionPanel();
            return result;
        } catch (error) {
            console.error('Failed to load variable suggestions for lab editor:', error);
            state.variableSuggestions = [];
            updateSuggestionPanel();
            throw error;
        } finally {
            state.variableSuggestionPromise = null;
        }
    }

    async function loadPipelines(force = false) {
        if (!context || typeof context.fetchData !== 'function') return;
        if (state.pipelinesLoaded && !force) {
            renderPipelineOptions();
            return;
        }
        try {
            state.pipelineSources = new Map();
            const response = await context.fetchData('/v1/pipelines?include_source=true');
            state.pipelines = parsePipelineList(response);
            state.pipelines.forEach(item => {
                if (item.source) state.pipelineSources.set(item.id, item.source);
            });
        } catch (error) {
            console.error('Failed to load pipelines:', error);
            state.pipelines = [];
        }
        state.pipelinesLoaded = true;
        renderPipelineOptions();
    }

    function parsePipelineList(response) {
        if (!Array.isArray(response)) return [];
        return response.map(item => {
            if (typeof item === 'string') return { id: item.trim(), source: '' };
            if (!item || typeof item !== 'object') return null;
            return { id: (item.id || item.identifier || item.pipeline || '').trim(), source: (item.source || '').trim() };
        }).filter(Boolean).sort((a, b) => a.id.localeCompare(b.id));
    }

    function renderPipelineOptions() {
        const select = DOM['lab-pipeline-select'];
        if (!select) return;
        select.innerHTML = '';
        select.appendChild(new Option(state.pipelines.length ? 'Select a pipeline' : 'No pipelines available', ''));
        state.pipelines.forEach(item => {
            const label = item.source ? `${item.id} (${item.source})` : item.id;
            const opt = new Option(label, item.id);
            if (item.id === state.selectedPipeline) opt.selected = true;
            select.appendChild(opt);
        });
        updatePipelineLink();
    }

    async function loadScopes() {
        if (!context || typeof context.fetchData !== 'function') return;
        if (state.scopesLoaded) { renderScopeOptions(); return; }
        const scopeSet = new Set();
        try {
            const [secrets, vars] = await Promise.all([
                context.fetchData('/v1/secrets/scopes'),
                context.fetchData('/v1/variables/scopes'),
            ]);
            [secrets, vars].forEach(list => {
                if (Array.isArray(list)) list.forEach(i => {
                    const val = (typeof i === 'string' ? i : i?.scope || '').trim();
                    if (val) scopeSet.add(val);
                });
            });
        } catch (e) { console.error(e); }
        state.scopes = Array.from(scopeSet).sort();
        state.scopesLoaded = true;
        renderScopeOptions();
    }

    function renderScopeOptions() {
        const select = DOM['lab-scope-input'];
        if (!select) return;
        const selected = state.scopeValue || '';
        select.innerHTML = '';
        select.appendChild(new Option('Default scope', '', selected === ''));
        state.scopes.forEach(s => {
            const opt = new Option(s, s, false, selected === s);
            select.appendChild(opt);
        });
        if (selected && !state.scopes.includes(selected)) {
            // Preserve any custom scope not returned by the API
            const customOpt = new Option(selected, selected, false, true);
            select.appendChild(customOpt);
        }
    }

    // --- 4. Editor Logic ---

    function renderEditor() {
        const textarea = DOM['lab-yaml-editor'];
        if (!textarea) return;
        textarea.value = state.currentYaml || '';
        refreshEditorUI();
        enterEditMode();
    }

    function handleEditorInput(event) {
        state.currentYaml = event.target.value || '';
        refreshEditorUI();
    }

    function refreshEditorUI() {
        updateDirtyState();
        handleValidation();
        renderSummary();
        updateSuggestionPanel();
        updateActionState();
        updateFloatingSuggestionPanelPosition();
    }

    function handleEditorKeydown(event) {
        const textarea = DOM['lab-yaml-editor'];
        if (!textarea) return;

        if (event.key === 'Tab') {
            if (state.editorSuggestionItems.length && state.editorSuggestionContext) {
                event.preventDefault();
                const idx = state.editorSuggestionIndex >= 0 ? state.editorSuggestionIndex : 0;
                const item = state.editorSuggestionItems[idx] || state.editorSuggestionItems[0];
                applyEditorSuggestion(item);
            } else {
                event.preventDefault();
                insertEditorIndent(event.target);
                handleValidation();
                updateLineNumbers();
                updateSuggestionPanel();
            }
        } else if (event.key === 'Enter') {
            handleEditorEnterKey(event);
        } else if (event.key === 'Escape') {
            hideEditorSuggestions();
        }
    }

    function syncEditorScroll() {
        const ta = DOM['lab-yaml-editor'];
        const hl = DOM['lab-yaml-highlight'];
        const ln = DOM['lab-line-numbers'];
        
        if (hl) { 
            hl.style.transform = `translate(${-ta.scrollLeft}px, ${-ta.scrollTop}px)`;
        }
        if (ln) ln.style.setProperty('--line-number-scroll', `${ta.scrollTop}px`);
        updateFloatingSuggestionPanelPosition();
    }

    function updateEditorHighlight() {
        const hl = DOM['lab-yaml-highlight'];
        const ta = DOM['lab-yaml-editor'];
        const stage = DOM['lab-yaml-stage'];
        if (!hl || !ta) return;
        
        const renderer = global.yaml && global.yaml.renderTokens;
        if (renderer) {
            hl.innerHTML = renderer(ta.value || '') || '&nbsp;';
            stage.classList.add('yaml-editor-stage--with-highlight');
        } else {
            hl.textContent = ta.value || '';
            stage.classList.remove('yaml-editor-stage--with-highlight');
        }
        syncEditorScroll();
    }

    function updateLineNumbers() {
        const ta = DOM['lab-yaml-editor'];
        const ln = DOM['lab-line-numbers'];
        if (!ta || !ln) return;
        
        const lines = ta.value.split('\n');
        const errorMap = new Map();
        (state.validationErrors || []).forEach(e => {
            if (typeof e.line === 'number') {
                const l = errorMap.get(e.line) || [];
                l.push(e.message);
                errorMap.set(e.line, l);
            }
        });

        ln.innerHTML = `<div class="line-number-track">${lines.map((_, i) => {
            const num = i + 1;
            const errs = errorMap.get(num);
            const cls = errs ? 'line-number line-number--error' : 'line-number';
            const title = errs ? ` title="${escapeAttribute(errs.join('\n'))}"` : '';
            return `<div class="${cls}" data-line-number="${num}"${title}>${num}</div>`;
        }).join('')}</div>`;
        syncEditorScroll();
    }

    function enterEditMode() {
        state.isEditing = true;
        state.lastRouteHash = window.location.hash || state.lastRouteHash || '#/lab';
        if (DOM['lab-validation-status']) DOM['lab-validation-status'].classList.remove('hidden');
        enableFloatingSuggestionPanel();
        bindBeforeUnload();
        if (DOM['lab-yaml-editor']) {
            DOM['lab-yaml-editor'].focus();
        }
        updateFloatingSuggestionPanelPosition();
    }

    function exitEditMode() {
        state.isEditing = false;
        disableFloatingSuggestionPanel();
        hideEditorSuggestions();
        unbindBeforeUnload();
    }

    function updateDirtyState() {
        const dirty = (state.currentYaml || '') !== (state.originalYaml || '');
        state.hasUnsavedChanges = dirty;
        if (dirty) {
            bindBeforeUnload();
        } else {
            unbindBeforeUnload();
        }
    }

    function bindBeforeUnload() {
        if (state.beforeUnloadHandler) return;
        const handler = (event) => {
            if (!state.isEditing || !state.hasUnsavedChanges) return;
            event.preventDefault();
            event.returnValue = '';
            return '';
        };
        state.beforeUnloadHandler = handler;
        window.addEventListener('beforeunload', handler);
    }

    function unbindBeforeUnload() {
        if (!state.beforeUnloadHandler) return;
        try {
            window.removeEventListener('beforeunload', state.beforeUnloadHandler);
        } catch {}
        state.beforeUnloadHandler = null;
    }

    function enableFloatingSuggestionPanel() {
        if (state.suggestionPanelFloating) return;
        const panel = DOM['lab-suggestion-panel'];
        if (!panel || !panel.parentNode) return;
        const parent = panel.parentNode;
        const nextSibling = panel.nextSibling;
        state.suggestionPanelOriginalParent = parent;
        state.suggestionPanelOriginalNextSibling = nextSibling;
        const container = DOM['lab-editor-wrapper'] || document.getElementById('page-content-wrapper') || document.body;
        const baseWidth = panel.offsetWidth || 260;
        try {
            parent.removeChild(panel);
        } catch {}
        if (container && container.classList) {
            container.classList.add('pipeline-suggestion-overlay-host');
        }
        container.appendChild(panel);
        panel.classList.add('pipeline-suggestion-overlay');
        panel.dataset.baseWidth = String(baseWidth);
        state.suggestionPanelOverlayContainer = container;
        state.suggestionPanelFloating = true;
        state.suggestionPanelPositionHandler = () => updateFloatingSuggestionPanelPosition();
        window.addEventListener('resize', state.suggestionPanelPositionHandler);
        if (container) {
            container.addEventListener('scroll', state.suggestionPanelPositionHandler);
        }
        updateFloatingSuggestionPanelPosition();
        startEditorSuggestionTracking();
    }

    function disableFloatingSuggestionPanel() {
        if (!state.suggestionPanelFloating) return;
        const panel = DOM['lab-suggestion-panel'];
        const container = state.suggestionPanelOverlayContainer;
        if (state.suggestionPanelPositionHandler) {
            window.removeEventListener('resize', state.suggestionPanelPositionHandler);
            if (container) {
                container.removeEventListener('scroll', state.suggestionPanelPositionHandler);
            }
        }
        if (panel) {
            panel.classList.remove('pipeline-suggestion-overlay');
            const originalParent = state.suggestionPanelOriginalParent;
            const reference = state.suggestionPanelOriginalNextSibling;
            if (originalParent) {
                if (reference && reference.parentNode === originalParent) {
                    originalParent.insertBefore(panel, reference);
                } else {
                    originalParent.appendChild(panel);
                }
            }
            panel.style.left = '';
            panel.style.top = '';
            panel.style.width = '';
            panel.style.minWidth = '';
            panel.style.maxHeight = '';
        }
        if (container && container.classList) {
            container.classList.remove('pipeline-suggestion-overlay-host');
        }
        state.suggestionPanelFloating = false;
        state.suggestionPanelOriginalParent = null;
        state.suggestionPanelOriginalNextSibling = null;
        state.suggestionPanelOverlayContainer = null;
        state.suggestionPanelPositionHandler = null;
        stopEditorSuggestionTracking(true);
    }

    function updateFloatingSuggestionPanelPosition() {
        if (!state.suggestionPanelFloating) return;
        const panel = DOM['lab-suggestion-panel'];
        const textarea = DOM['lab-yaml-editor'];
        const container = state.suggestionPanelOverlayContainer || DOM['lab-editor-wrapper'] || document.getElementById('page-content-wrapper') || document.body;
        if (!panel || panel.classList.contains('hidden') || !textarea || !container) {
            return;
        }

        const textareaRect = textarea.getBoundingClientRect();
        const containerRect = container.getBoundingClientRect();
        const padding = 24;
        const baseWidth = panel.dataset.baseWidth ? parseFloat(panel.dataset.baseWidth) : (panel.offsetWidth || 260);

        panel.style.width = 'auto';
        panel.style.minWidth = `${baseWidth}px`;
        const currentWidth = panel.offsetWidth;

        const containerWidth = container.clientWidth || (window.innerWidth ?? baseWidth + padding * 2);
        const targetLeft = textareaRect.right - containerRect.left + container.scrollLeft + padding;
        const maxLeft = container.scrollLeft + containerWidth - currentWidth - padding;
        const minLeft = container.scrollLeft + padding;
        const finalLeft = Math.max(minLeft, Math.min(targetLeft, maxLeft));

        const viewportTop = container.scrollTop + padding;
        const viewportBottom = container.scrollTop + (container.clientHeight || window.innerHeight || (panelHeight + padding * 2)) - padding;
        
        let finalTop = Math.max(viewportTop, textareaRect.top - containerRect.top + container.scrollTop);

        const validationBox = DOM['lab-validation-status'];
        if (validationBox && !validationBox.classList.contains('hidden')) {
            const vRect = validationBox.getBoundingClientRect();
            const valBottom = vRect.bottom - containerRect.top + container.scrollTop;
            const minTop = valBottom + 12;
            
            if (finalTop < minTop) {
                finalTop = minTop;
            }
        }

        let availableHeight = Math.max(150, viewportBottom - finalTop);

        panel.style.left = `${finalLeft}px`;
        panel.style.top = `${finalTop}px`;
        panel.style.maxHeight = `${availableHeight}px`;
    }

    // --- 5. Context Detection & Suggestions ---

    function updateSuggestionPanel() {
        const ta = DOM['lab-yaml-editor'];
        if (!ta) {
            hideEditorSuggestions();
            return;
        }
        if (!state.variableSuggestions.length && !state.variableSuggestionPromise) {
            ensureVariableSuggestionData().catch(() => {});
        }
        ensureEditorSuggestionOverlay();

        const text = ta.value || '';
        const selectionStart = Math.min(ta.selectionStart ?? 0, ta.selectionEnd ?? 0);
        const selectionEnd = Math.max(ta.selectionStart ?? 0, ta.selectionEnd ?? 0);
        state.lastEditorSelection = { start: selectionStart, end: selectionEnd };
        const contextInfo = detectSuggestionContext(text, selectionStart, selectionEnd);
        state.editorSuggestionContext = contextInfo;

        if (!contextInfo) {
            hideEditorSuggestions();
            renderFloatingSuggestions([], null);
            updateFloatingSuggestionPanelPosition();
            return;
        }

        const requiresMetadata = contextInfo.type === 'secrets'
            || contextInfo.type === 'variables'
            || contextInfo.type === 'include';

        if (requiresMetadata) {
            let poolSize = 0;
            if (contextInfo.type === 'secrets') {
                poolSize = state.autocomplete.secrets.length;
            } else if (contextInfo.type === 'variables') {
                poolSize = state.autocomplete.variables.length;
            } else if (contextInfo.type === 'include') {
                const stepsCount = state.autocomplete.reusableSteps.length;
                const pipelineCount = Array.isArray(state.pipelines) ? state.pipelines.length : 0;
                poolSize = stepsCount + pipelineCount;
            }
            if (!poolSize) {
                preloadAutocompleteMetadata().catch(() => {});
                renderEditorSuggestions({ title: contextInfo.title, loading: true });
                renderFloatingSuggestions([], contextInfo);
                updateFloatingSuggestionPanelPosition();
                return;
            }
        }

        const items = buildSuggestionItems(contextInfo, text);
        if (!items.length) {
            if (requiresMetadata && state.autocomplete.isLoading) {
                renderEditorSuggestions({ title: contextInfo.title, loading: true });
            } else {
                hideEditorSuggestions();
            }
            renderFloatingSuggestions([], contextInfo);
            updateFloatingSuggestionPanelPosition();
            return;
        }

        renderEditorSuggestions({ title: contextInfo.title, items });
        renderFloatingSuggestions(items, contextInfo);
        updateFloatingSuggestionPanelPosition();
    }

    function ensureEditorSuggestionOverlay() {
        if (DOM['lab-editor-autocomplete']) return;
        const overlay = document.createElement('div');
        overlay.id = 'lab-editor-autocomplete';
        overlay.className = 'pipeline-editor-autocomplete hidden';
        const ghost = document.createElement('span');
        ghost.id = 'lab-editor-autocomplete-ghost';
        ghost.className = 'pipeline-editor-autocomplete__ghost';
        overlay.appendChild(ghost);
        document.body.appendChild(overlay);
        DOM['lab-editor-autocomplete'] = overlay;
        DOM['lab-editor-autocomplete-ghost'] = ghost;
    }

    function renderEditorSuggestions(payload) {
        ensureEditorSuggestionOverlay();
        const overlay = DOM['lab-editor-autocomplete'];
        const ghostEl = DOM['lab-editor-autocomplete-ghost'];
        const textarea = DOM['lab-yaml-editor'];
        if (!overlay || !ghostEl || !textarea) return;

        if (payload.loading) {
            hideEditorSuggestions();
            return;
        }

        if (!payload.items || !payload.items.length) {
            hideEditorSuggestions();
            return;
        }

        state.editorSuggestionItems = payload.items.slice();
        state.editorSuggestionIndex = 0;
        const activeItem = state.editorSuggestionItems[0];
        const preview = buildInlineSuggestionPreview(activeItem, state.editorSuggestionContext);
        if (!preview) {
            hideEditorSuggestions();
            return;
        }

        ghostEl.textContent = preview;
        overlay.classList.remove('hidden');
        updateInlineSuggestionPosition();
        startEditorSuggestionTracking();
    }

    function hideEditorSuggestions() {
        state.editorSuggestionContext = null;
        state.editorSuggestionItems = [];
        state.editorSuggestionIndex = -1;
        const overlay = DOM['lab-editor-autocomplete'];
        const ghostEl = DOM['lab-editor-autocomplete-ghost'];
        if (overlay) {
            overlay.style.transform = '';
            overlay.classList.add('hidden');
        }
        if (ghostEl) {
            ghostEl.textContent = '';
        }
        stopEditorSuggestionTracking();
    }

    function buildInlineSuggestionPreview(item, contextInfo) {
        if (!item) return '';
        const prefix = (contextInfo && typeof contextInfo.prefix === 'string') ? contextInfo.prefix : '';
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

    function updateInlineSuggestionPosition() {
        if (!DOM['lab-yaml-editor'] || !DOM['lab-editor-autocomplete'] || !state.editorSuggestionContext) return;
        const textarea = DOM['lab-yaml-editor'];
        const overlay = DOM['lab-editor-autocomplete'];
        if (overlay.classList.contains('hidden')) return;

        const caret = calculateCaretOffset(textarea);
        if (!caret) return;

        const textareaRect = textarea.getBoundingClientRect();
        const docLeft = window.scrollX + textareaRect.left + caret.left;
        const docTop = window.scrollY + textareaRect.top + caret.top;

        overlay.style.transform = `translate3d(${docLeft}px, ${docTop}px, 0)`;
    }

    function startEditorSuggestionTracking() {
        if (state.editorSuggestionAnimationFrame != null) return;
        const step = () => {
            state.editorSuggestionAnimationFrame = window.requestAnimationFrame(() => {
                updateInlineSuggestionPosition();
                updateFloatingSuggestionPanelPosition();
                step();
            });
        };
        step();
    }

    function stopEditorSuggestionTracking(force = false) {
        if (!force && state.suggestionPanelFloating) {
            return;
        }
        if (state.editorSuggestionAnimationFrame != null) {
            window.cancelAnimationFrame(state.editorSuggestionAnimationFrame);
            state.editorSuggestionAnimationFrame = null;
        }
    }

    function ensureTextareaCaretMirror() {
        if (textareaCaretMirror && textareaCaretMirror.parentNode) {
            return textareaCaretMirror;
        }
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
        textareaCaretMirror = mirror;
        return mirror;
    }

    function calculateCaretOffset(textarea) {
        if (!textarea) return null;
        const selectionStart = textarea.selectionStart;
        if (typeof selectionStart !== 'number') return null;

        const mirror = ensureTextareaCaretMirror();
        const computed = window.getComputedStyle(textarea);
        const properties = [
            'boxSizing', 'width', 'height', 'overflowX', 'overflowY',
            'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
            'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
            'fontStyle', 'fontVariant', 'fontWeight', 'fontStretch', 'fontSize',
            'fontFamily', 'lineHeight', 'textAlign', 'textTransform', 'textIndent',
            'letterSpacing', 'wordSpacing', 'tabSize'
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

        const borderLeft = parseFloat(computed.borderLeftWidth) || 0;
        const borderTop = parseFloat(computed.borderTopWidth) || 0;
        const offsetLeft = marker.offsetLeft + borderLeft - (textarea.scrollLeft || 0);
        const offsetTop = marker.offsetTop + borderTop - (textarea.scrollTop || 0);

        mirror.textContent = '';

        return { left: offsetLeft, top: offsetTop };
    }

    function applyEditorSuggestion(item) {
        if (!state.editorSuggestionContext || !DOM['lab-yaml-editor'] || !item) return;
        const contextInfo = state.editorSuggestionContext;
        const textarea = DOM['lab-yaml-editor'];
        const textLength = textarea.value.length;
        const rangeStart = Math.max(0, Math.min(contextInfo.rangeStart ?? textarea.selectionStart, textLength));
        const rangeEnd = Math.max(rangeStart, Math.min(contextInfo.rangeEnd ?? textarea.selectionEnd, textLength));
        const before = textarea.value.slice(0, rangeStart);
        const after = textarea.value.slice(rangeEnd);
        let insertText = item.snippet ?? item.value;
        if (typeof insertText !== 'string') {
            insertText = String(insertText ?? '');
        }
        const prefixInsert = contextInfo.insertPrefix || '';
        let suffix = contextInfo.insertSuffix || '';
        if (item.overrideSuffix !== undefined) {
            suffix = item.overrideSuffix;
        } else if (suffix && after.trimStart().startsWith(':')) {
            suffix = '';
        }
        const finalText = prefixInsert + insertText + suffix;
        textarea.value = before + finalText + after;
        const caret = rangeStart + finalText.length;
        textarea.selectionStart = textarea.selectionEnd = caret;
        textarea.focus();
        hideEditorSuggestions();
        handleValidation();
        updateLineNumbers();
        updateSuggestionPanel();
    }

    function insertEditorIndent(target) {
        if (!target || typeof target.value !== 'string') return;
        const start = target.selectionStart ?? 0;
        const end = target.selectionEnd ?? start;
        const value = target.value;
        target.value = value.substring(0, start) + '  ' + value.substring(end);
        const caret = start + 2;
        target.selectionStart = target.selectionEnd = caret;
    }

    function handleEditorEnterKey(event) {
        const textarea = event.target;
        if (!textarea || typeof textarea.value !== 'string') {
            return;
        }

        const start = textarea.selectionStart ?? 0;
        const end = textarea.selectionEnd ?? start;
        if (start === null || end === null) {
            return;
        }

        event.preventDefault();

        const value = textarea.value;
        const lineInfo = getCurrentLineInfo(value, start);
        const before = value.slice(0, start);
        const after = value.slice(end);
        const indentMatch = lineInfo.line.match(/^\s*/);
        const currentIndent = indentMatch ? indentMatch[0] : '';
        const trimmed = lineInfo.line.trim();
        const parentBlock = findParentBlock(value.slice(0, lineInfo.start), ['steps', 'tasks'], lineInfo.indent);
        let newIndent = currentIndent;
        let listPrefix = '';

        if (/^-\s*name\s*:/i.test(trimmed)) {
            newIndent = ' '.repeat(lineInfo.indent + 2);
            listPrefix = '';
        } else if (trimmed.startsWith('-')) {
            newIndent = currentIndent;
            const parent = findParentBlock(value.slice(0, lineInfo.start), ['steps', 'tasks'], lineInfo.indent);
            if (parent && LIST_KEYS_WITH_NAME_TEMPLATE.has(parent)) {
                listPrefix = '- name: ';
            } else {
                listPrefix = '- ';
            }
        } else if (trimmed.endsWith(':')) {
            newIndent = currentIndent + '  ';
            const key = trimmed.slice(0, -1).trim();
            if (LIST_KEYS_WITH_NAME_TEMPLATE.has(key)) {
                listPrefix = '- name: ';
            } else if (LIST_KEYS_SIMPLE.has(key) && !parentBlock) {
                listPrefix = '- ';
            }
        } else {
            if (parentBlock && LIST_KEYS_WITH_NAME_TEMPLATE.has(parentBlock) && trimmed === '') {
                newIndent = ' '.repeat(lineInfo.indent);
                listPrefix = '- name: ';
            } else {
                newIndent = currentIndent;
            }
        }

        const insertion = `\n${newIndent}${listPrefix}`;
        textarea.value = before + insertion + after;
        const caret = before.length + insertion.length;
        textarea.selectionStart = textarea.selectionEnd = caret;
        handleValidation();
        updateLineNumbers();
        updateSuggestionPanel();
    }

    function detectSuggestionContext(text, caret, end) {
        const lineInfo = getCurrentLineInfo(text, caret);
        const beforeLine = text.slice(0, lineInfo.start);
        const lineStr = lineInfo.line;
        
        // 1. Reusable Steps
        const incMatch = lineStr.match(/include:\s*"?([^"]*)$/);
        if (incMatch) {
            return {
                type: 'include', title: 'Include Targets',
                prefix: incMatch[1],
                rangeStart: lineInfo.start + incMatch.index + incMatch[0].length - incMatch[1].length,
                rangeEnd: end, insertSuffix: ''
            };
        }

        // 2. Dependencies
        const depMatch = lineStr.match(/depends_on:\s*\[?([^\]]*)$/);
        if (depMatch || findParentBlock(beforeLine, ['depends_on'], lineInfo.indent)) {
            const word = lineStr.slice(0, lineInfo.column).split(/[\s,\[]+/).pop();
            return {
                type: 'depends_on', title: 'Dependencies',
                prefix: word,
                rangeStart: caret - word.length, rangeEnd: end, insertSuffix: ''
            };
        }

        // 3. Secrets / Variables
        const secList = detectListEntryContext(lineInfo, end, beforeLine, 'secrets');
        if (secList) return { ...secList, type: 'secrets', title: 'Secrets', insertSuffix: '' };

        const variableContext = detectVariableContext(lineInfo, end, beforeLine);
        if (variableContext) return variableContext;

        // 4. Directive Values
        const valContext = detectDirectiveValueContext(lineInfo, end);
        if (valContext) return valContext;

        // 5. Directives (Keys)
        const directiveCtx = detectDirectiveKeyContext(lineInfo, end, beforeLine, text);
        if (directiveCtx) return directiveCtx;

        return null;
    }

    function detectDirectiveValueContext(lineInfo, selectionEnd) {
        const rawLine = lineInfo.line;
        const colonIndex = rawLine.indexOf(':');
        if (colonIndex === -1 || lineInfo.column <= colonIndex) return null;

        const key = rawLine.slice(lineInfo.indent, colonIndex).trim();
        const valueOffsetLocal = colonIndex + 1 + (rawLine.slice(colonIndex + 1).match(/^\s*/) || [''])[0].length;
        const rangeStart = lineInfo.start + valueOffsetLocal;
        const currentValue = rawLine.slice(valueOffsetLocal, lineInfo.column).trim();

        const metadata = DIRECTIVE_VALUE_METADATA[key];
        if (metadata) {
            return {
                type: 'directive-value', title: metadata.title,
                key, prefix: currentValue,
                rangeStart, rangeEnd: Math.max(rangeStart, selectionEnd),
                insertSuffix: '',
            };
        }
        return null;
    }

    function detectVariableContext(lineInfo, selectionEnd, beforeLine) {
        const parent = findParentBlock(beforeLine, ['variables'], lineInfo.indent, ['steps', 'tasks']);
        if (parent !== 'variables') return null;

        const local = lineInfo.line.slice(lineInfo.indent);
        const trimmedLocal = local.trimStart();
        
        // List style: "- VAR"
        if (trimmedLocal.startsWith('-')) {
            const dashMatch = local.match(/^-\s*/);
            const dashSegment = dashMatch ? dashMatch[0] : '-';
            const valueStartLocal = lineInfo.indent + dashSegment.length;
            const relativeText = lineInfo.line.slice(valueStartLocal, lineInfo.column);
            const trimmedValue = relativeText.trim();
            const relativeOffset = trimmedValue ? relativeText.indexOf(trimmedValue) : 0;
            const rangeStart = lineInfo.start + valueStartLocal + relativeOffset;
            return {
                type: 'variables', title: 'Variables', prefix: trimmedValue,
                rangeStart, rangeEnd: Math.max(rangeStart, selectionEnd),
                insertSuffix: '', insertPrefix: dashSegment.endsWith(' ') ? '' : ' ',
            };
        }

        // Key style: "VAR:"
        const colonIndex = lineInfo.line.indexOf(':', lineInfo.indent);
        const hasColon = colonIndex !== -1;
        const valueEnd = hasColon ? Math.min(colonIndex, lineInfo.column) : lineInfo.column;
        const rawPrefix = lineInfo.line.slice(lineInfo.indent, valueEnd);
        const prefix = rawPrefix.trim();
        const computedRangeEnd = hasColon && colonIndex < selectionEnd ? lineInfo.start + colonIndex : selectionEnd;
        const safeRangeEnd = Math.max(lineInfo.start + lineInfo.indent, computedRangeEnd);
        return {
            type: 'variables', title: 'Variables', prefix,
            rangeStart: lineInfo.start + lineInfo.indent,
            rangeEnd: safeRangeEnd,
            insertSuffix: hasColon ? '' : ': ', insertPrefix: '',
        };
    }

    function detectListEntryContext(lineInfo, selectionEnd, beforeLine, keyName) {
        const trimmed = lineInfo.line.trimStart();
        if (!trimmed.startsWith('-')) return null;
        const parent = findParentBlock(beforeLine, [keyName], lineInfo.indent);
        if (parent !== keyName) return null;
        const dashMatch = lineInfo.line.match(/^(\s*-\s*)/);
        const valueStart = dashMatch ? dashMatch[0].length : lineInfo.indent;
        const rangeStart = lineInfo.start + valueStart;
        return {
            prefix: lineInfo.line.slice(valueStart, lineInfo.column).trim(),
            rangeStart, rangeEnd: Math.max(rangeStart, selectionEnd),
            insertSuffix: '', insertPrefix: dashMatch && /\s$/.test(dashMatch[0]) ? '' : ' ',
        };
    }

    function detectDirectiveKeyContext(lineInfo, selectionEnd, beforeLine, fullText) {
        const rawLine = lineInfo.line;
        if (!rawLine) return null;
        const trimmed = rawLine.trim();
        if (trimmed.startsWith('#')) return null;

        const colonIndex = rawLine.indexOf(':');
        if (colonIndex !== -1 && lineInfo.column > colonIndex) return null;

        let type = 'pipeline-key';
        let rangeStart;
        let rangeEnd;
        let prefix;
        let parent = findParentBlock(beforeLine, ['steps', 'tasks'], lineInfo.indent);

        if (trimmed.startsWith('-')) {
            const dashMatch = rawLine.match(/^(\s*-\s*)/);
            const dashSegment = dashMatch ? dashMatch[0] : '- ';
            const valueStartLocal = dashSegment.length;
            const valueSlice = rawLine.slice(valueStartLocal, Math.max(lineInfo.column, valueStartLocal));
            rangeStart = lineInfo.start + valueStartLocal;
            const endIndex = colonIndex !== -1 ? Math.min(colonIndex, lineInfo.column) : Math.max(lineInfo.column, valueStartLocal);
            prefix = valueSlice.slice(0, Math.max(0, endIndex - valueStartLocal)).trim();
            parent = findParentBlock(beforeLine, ['steps', 'tasks'], lineInfo.indent);
            if (parent === 'steps') type = 'step-key';
            else if (parent === 'tasks') type = 'task-key';
            else return null;
        } else {
            if (parent === 'steps') type = 'step-key';
            else if (parent === 'tasks') type = 'task-key';
            else if (lineInfo.indent !== 0) return null;

            rangeStart = lineInfo.start + lineInfo.indent;
            const endIndex = colonIndex !== -1 ? Math.min(colonIndex, lineInfo.column) : lineInfo.column;
            prefix = rawLine.slice(lineInfo.indent, endIndex).trim();
        }

        const colonBound = colonIndex !== -1 ? Math.min(colonIndex, lineInfo.column) : Math.max(lineInfo.column, rangeStart - lineInfo.start);
        rangeEnd = Math.max(rangeStart, lineInfo.start + colonBound);

        const title = type === 'pipeline-key' ? 'Pipeline Directives' : (type === 'step-key' ? 'Step Directives' : 'Task Directives');
        const existingKeys = collectExistingKeysForContext(fullText, lineInfo, type);

        return {
            type,
            title,
            prefix,
            rangeStart,
            rangeEnd,
            insertSuffix: ': ',
            existingKeys
        };
    }

    function collectExistingKeysForContext(text, lineInfo, type) {
        const keys = new Set();
        if (type === 'pipeline-key') return keys;

        const lines = text.split('\n');
        // Find current line index
        let currentLineIdx = 0;
        let count = 0;
        for (let i = 0; i < lines.length; i++) {
            if (count + lines[i].length >= lineInfo.start) {
                currentLineIdx = i;
                break;
            }
            count += lines[i].length + 1;
        }

        const targetIndent = lineInfo.indent;
        let start = currentLineIdx;
        
        // Scan UP for start of item (dash)
        while (start >= 0) {
            const line = lines[start];
            const indent = (line.match(/^\s*/) || [''])[0].length;
            if (indent < targetIndent) break; 
            if (indent === targetIndent && line.trim().startsWith('-')) break;
            start--;
        }
        if (start < 0) start = 0;

        // Scan DOWN to collect keys until indent drops or next item starts
        for (let i = start; i < lines.length; i++) {
            const line = lines[i];
            const trimmed = line.trim();
            if (!trimmed) continue;
            
            const indent = (line.match(/^\s*/) || [''])[0].length;
            if (i > start && indent <= targetIndent && trimmed.startsWith('-')) break; // Next item
            if (indent < targetIndent) break; // End of block

            const match = trimmed.match(/^(-\s+)?([a-zA-Z0-9_]+):/);
            if (match) keys.add(match[2]);
        }
        return keys;
    }

    function findParentBlock(beforeText, targetKeys, currentIndent, stopKeys = []) {
        const lines = beforeText.split('\n');
        for (let i = lines.length - 1; i >= 0; i--) {
            const line = lines[i];
            const trimmed = line.trim();
            if (!trimmed || trimmed.startsWith('#')) continue;
            
            const indent = (line.match(/^\s*/) || [''])[0].length;
            if (indent < currentIndent) {
                const colonIdx = trimmed.indexOf(':');
                if (colonIdx !== -1) {
                    const key = trimmed.slice(0, colonIdx).trim();
                    if (stopKeys.includes(key)) return null;
                    if (targetKeys.includes(key)) return key;
                    if (indent === 0) return null;
                } else if (indent === 0) {
                    return null;
                }
                currentIndent = indent;
            }
        }
        return null;
    }

    function buildSuggestionItems(ctx, text) {
        const prefix = ctx.prefix || '';
        let pool = [];

        if (ctx.type === 'depends_on') {
            pool = collectStepNames(text).map(n => ({ value: n, label: n }));
        } else if (ctx.type === 'secrets') {
            pool = state.autocomplete.secrets.map(s => ({ value: s, label: s }));
        } else if (ctx.type === 'variables') {
            pool = state.autocomplete.variables.map(v => ({ value: v, label: v }));
        } else if (ctx.type === 'include') {
            pool = [
                ...state.autocomplete.reusableSteps.map(s => ({ value: `step:${s}`, label: `step:${s}` })),
                ...state.pipelines.map(p => ({ value: `pipeline:${p.id}`, label: `pipeline:${p.id}` }))
            ];
        } else if (ctx.type === 'directive-value') {
             const metadata = DIRECTIVE_VALUE_METADATA[ctx.key];
             if (metadata && Array.isArray(metadata.values)) {
                 pool = metadata.values.map(v => ({ value: v, label: v }));
             }
        } else if (ctx.type === 'pipeline-key') {
            pool = PIPELINE_DIRECTIVES.map(d => ({ value: d.key, label: d.key, hint: d.hint }));
        } else if (ctx.type === 'step-key') {
            pool = STEP_DIRECTIVES.map(d => ({ value: d.key, label: d.key, hint: d.hint }));
        } else if (ctx.type === 'task-key') {
            pool = TASK_DIRECTIVES.map(d => ({ value: d.key, label: d.key, hint: d.hint }));
        }

        const existing = ctx.existingKeys || new Set();
        pool = pool.filter(i => !existing.has(i.value));

        if (!pool.length) return [];
        return pool.filter(i => i.value.toLowerCase().includes(prefix.toLowerCase())).slice(0, 15);
    }

    function renderFloatingSuggestions(contextItems = [], contextInfo = null) {
        const panel = DOM['lab-suggestion-panel'];
        const list = DOM['lab-suggestion-list'];
        const empty = DOM['lab-suggestion-empty'];
        if (!panel || !list || !empty) return;

        panel.classList.remove('hidden');
        setSuggestionPanelCopy(buildSuggestionCopy(contextInfo));

        const sections = [];

        const type = contextInfo?.type || null;

        // Context-matched block
        if (contextItems.length) {
            sections.push(renderSuggestionSection(contextInfo?.title || 'Suggestions', contextItems.length, contextItems.map(i => ({
                html: `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-val="${escapeAttribute(i.value)}">
                            <span>${escapeHtml(i.label)}</span>
                            ${i.hint ? `<span class="env-suggestion-hint">${escapeHtml(i.hint)}</span>` : ''}
                       </button>`
            }))));
        }

        // Additional pools only when in matching context
        if (type === 'variables' && state.variableSuggestions.length) {
            state.variableSuggestions.forEach(entry => {
                const pills = entry.preview.map(name => ({
                    html: `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-variable-suggestion="${escapeAttribute(name)}">${escapeHtml(name)}</button>`
                }));
                const remaining = entry.count - entry.preview.length;
                if (remaining > 0) pills.push({ html: `<span class="env-suggestion-pill env-suggestion-pill--more">+${remaining} more</span>` });
                sections.push(renderSuggestionSection(entry.label, entry.count, pills));
            });
        }

        if (type === 'secrets' && Array.isArray(state.autocomplete.secrets) && state.autocomplete.secrets.length) {
            const secretButtons = state.autocomplete.secrets.slice(0, 30).map(name => ({
                html: `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-secret-suggestion="${escapeAttribute(name)}"><span>${escapeHtml(name)}</span></button>`
            }));
            sections.push(renderSuggestionSection('Secrets', state.autocomplete.secrets.length, secretButtons));
        }

        if (type === 'include') {
            const reusableSteps = Array.isArray(state.autocomplete.reusableSteps) ? state.autocomplete.reusableSteps.filter(Boolean) : [];
            const pipelineIds = Array.isArray(state.pipelines) ? state.pipelines.map(p => p.id || p).filter(Boolean) : [];
            if (reusableSteps.length) {
                const buttons = reusableSteps.slice(0, 24).map(name => ({
                    html: `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-include-target="${escapeAttribute(`step:${name}`)}">${escapeHtml(`step:${name}`)}</button>`
                }));
                const remaining = reusableSteps.length - Math.min(reusableSteps.length, 24);
                if (remaining > 0) buttons.push({ html: `<span class="env-suggestion-pill env-suggestion-pill--more">+${remaining} more</span>` });
                sections.push(renderSuggestionSection('Reusable steps', reusableSteps.length, buttons));
            }
            if (pipelineIds.length) {
                const buttons = pipelineIds.slice(0, 24).map(id => ({
                    html: `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-include-target="${escapeAttribute(`pipeline:${id}`)}">${escapeHtml(`pipeline:${id}`)}</button>`
                }));
                const remaining = pipelineIds.length - Math.min(pipelineIds.length, 24);
                if (remaining > 0) buttons.push({ html: `<span class="env-suggestion-pill env-suggestion-pill--more">+${remaining} more</span>` });
                sections.push(renderSuggestionSection('Pipelines', pipelineIds.length, buttons));
            }
        }

        if (!sections.length) {
            empty.textContent = state.autocomplete.isLoading || state.variableSuggestionPromise ? 'Loading suggestions…' : 'No suggestions available yet.';
            empty.classList.remove('hidden');
            list.innerHTML = '';
        } else {
            empty.classList.add('hidden');
            list.innerHTML = sections.join('');
        }

        updateFloatingSuggestionPanelPosition();
    }

    function buildSuggestionCopy(contextInfo) {
        const type = contextInfo?.type;
        switch (type) {
            case 'variables':
                return { title: 'Variables', subtitle: 'Insert variables scoped to your env.', footnote: 'Tab to accept inline hint.' };
            case 'secrets':
                return { title: 'Secrets', subtitle: 'Available secret names.', footnote: 'Tab to accept inline hint.' };
            case 'include':
                return { title: 'Include targets', subtitle: 'Reusable steps and pipelines.', footnote: 'Click or Tab to insert.' };
            case 'pipeline-key':
                return { title: 'Pipeline directives', subtitle: 'Keys allowed at root level.', footnote: 'Tab to accept inline hint.' };
            case 'step-key':
                return { title: 'Step directives', subtitle: 'Keys allowed within steps.', footnote: 'Tab to accept inline hint.' };
            case 'task-key':
                return { title: 'Task directives', subtitle: 'Keys allowed within tasks.', footnote: 'Tab to accept inline hint.' };
            case 'depends_on':
                return { title: 'Dependencies', subtitle: 'Existing step/task names.', footnote: 'Tab to accept inline hint.' };
            case 'directive-value':
                return { title: 'Allowed values', subtitle: 'Insert permitted values.', footnote: 'Tab to accept inline hint.' };
            default:
                return { title: 'Suggestions', subtitle: 'Context-aware helpers.', footnote: 'Tab to accept inline hint.' };
        }
    }

    function renderSuggestionSection(label, count, items) {
        return `
            <article class="env-suggestion-item">
                <div class="env-suggestion-env">
                    <span class="env-suggestion-env-label">${escapeHtml(label)}</span>
                    <span class="env-suggestion-env-count">${escapeHtml(`${count} item${count === 1 ? '' : 's'}`)}</span>
                </div>
                <div class="env-suggestion-variables">
                    ${items.map(i => i.html).join('')}
                </div>
            </article>
        `;
    }

    function handleSuggestionClick(e) {
        const btn = e.target.closest('button[data-val], [data-variable-suggestion], [data-secret-suggestion], [data-include-target]');
        if (!btn) return;
        
        const val = btn.dataset.val || btn.dataset.variableSuggestion || btn.dataset.secretSuggestion || btn.dataset.includeTarget;
        const ta = DOM['lab-yaml-editor'];
        const ctx = state.editorSuggestionContext;

        if (ctx) {
            const pre = ta.value.slice(0, ctx.rangeStart);
            const post = ta.value.slice(ctx.rangeEnd);
            ta.value = pre + val + (ctx.insertSuffix || '') + post;
            const newPos = pre.length + val.length + (ctx.insertSuffix || '').length;
            ta.setSelectionRange(newPos, newPos);
        } else {
            document.execCommand('insertText', false, val);
        }
        
        ta.focus();
        refreshEditorUI();
    }

    // --- 7. Helpers ---

    function getCurrentLineInfo(text, pos) {
        const start = text.lastIndexOf('\n', pos - 1) + 1;
        let end = text.indexOf('\n', pos);
        if (end === -1) end = text.length;
        return { line: text.slice(start, end), start, end, column: pos - start, indent: (text.slice(start, end).match(/^\s*/) || [''])[0].length };
    }

    function collectStepNames(text) {
        const names = [];
        const regex = /^\s*-\s*name:\s*([a-zA-Z0-9_-]+)/gm;
        let m;
        while ((m = regex.exec(text)) !== null) names.push(m[1]);
        return names;
    }

    async function loadPipelineYaml(id) {
        state.isLoadingYaml = true;
        updateActionState();
        try {
            const res = await context.fetchData(`/v1/pipelines/${id.split('/').map(encodeURIComponent).join('/')}`);
            state.originalYaml = typeof res === 'string' ? res : '';
            state.currentYaml = state.originalYaml;
            renderEditor();
        } catch (e) { console.error(e); }
        finally { state.isLoadingYaml = false; updateActionState(); }
    }

    function handlePipelineChange(e) {
        const nextValue = e.target.value;
        if (state.hasUnsavedChanges && nextValue !== state.selectedPipeline) {
            notifyEditingLock();
            if (DOM['lab-pipeline-select']) {
                DOM['lab-pipeline-select'].value = state.selectedPipeline || '';
            }
            return;
        }
        state.selectedPipeline = nextValue;
        updatePipelineLink();
        if (state.selectedPipeline) loadPipelineYaml(state.selectedPipeline);
        else startBlankPipeline();
        renderSummary();
    }

    function startBlankPipeline() {
        if (state.hasUnsavedChanges) {
            notifyEditingLock();
            return;
        }
        state.selectedPipeline = '';
        state.originalYaml = `name: ${DEFAULT_PIPELINE_NAME}\nversion: latest\ndescription: Lab Pipeline\nsteps:\n  - name: hello\n    script: echo "Hello World"`;
        state.currentYaml = state.originalYaml;
        if (DOM['lab-pipeline-select']) DOM['lab-pipeline-select'].value = '';
        updatePipelineLink();
        renderEditor();
    }

    // --- 8. UI State & Overrides ---

    function saveLabYaml() {
        handleValidation();
        if (state.validationErrors.length > 0) {
            state.lastFeedback = { status: 'error', message: 'Fix validation errors before saving.' };
            renderFeedback();
            return;
        }
        state.originalYaml = state.currentYaml || '';
        updateDirtyState();
        state.lastFeedback = { status: 'success', message: 'YAML saved for this lab session (pipelines unchanged).' };
        renderFeedback();
        updateActionState();
    }

    function handleRunClick() {
        handleValidation();
        if (state.validationErrors.length > 0) {
            state.lastFeedback = { status: 'error', message: 'Fix validation errors first.' };
            renderFeedback();
            return;
        }
        const payload = {
            pipeline: state.selectedPipeline || parsePipelineName(state.currentYaml) || DEFAULT_PIPELINE_NAME,
            definition: state.currentYaml
        };
        const overrides = {};
        state.overrides.forEach(o => { if(o.key) overrides[o.key] = o.value; });
        if(Object.keys(overrides).length) payload.variables = overrides;
        
        if (state.scopeValue) payload.scope = state.scopeValue;

        state.isRunning = true;
        updateActionState();
        context.fetchData('/v1/run', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', ...(state.scopeValue ? {'X-Nopsai-Scope': state.scopeValue} : {}) },
            body: JSON.stringify(payload)
        }).then(res => {
            state.lastFeedback = { status: 'success', message: 'Run started!', runId: res.run_id || extractRunId(res) };
            renderFeedback();
        }).catch(err => {
            state.lastFeedback = { status: 'error', message: 'Run failed: ' + err.message };
            renderFeedback();
        }).finally(() => {
            state.isRunning = false;
            updateActionState();
        });
    }

    function extractRunId(res) {
        if(typeof res === 'string') {
            const m = res.match(/[0-9a-f]{8}-[0-9a-f]{4}/);
            return m ? m[0] : '';
        }
        return '';
    }

    function parsePipelineName(yaml) {
        const m = yaml.match(/^\s*name:\s*([^\s]+)/m);
        return m ? m[1] : '';
    }

    function addOverride() {
        state.overrides.push({ id: ++overrideSeq, key: '', value: '' });
        renderOverrides();
    }

    function renderOverrides() {
        const list = DOM['lab-overrides-list'];
        if (!list) return;
        list.innerHTML = '';
        state.overrides.forEach(o => {
            const div = document.createElement('div');
            const keyId = `lab-override-key-${o.id}`;
            const valueId = `lab-override-val-${o.id}`;
            div.className = 'lab-override-row';
            div.innerHTML = `
                <div class="lab-override-field">
                    <input id="${escapeAttribute(keyId)}" class="pipelines-input lab-override-input" placeholder="key" value="${escapeHtml(o.key)}" onchange="NopsAI.pages.lab.updateOv(${o.id}, 'key', this.value)">
                </div>
                <div class="lab-override-field">
                    <input id="${escapeAttribute(valueId)}" class="pipelines-input lab-override-input" placeholder="value" value="${escapeHtml(o.value)}" onchange="NopsAI.pages.lab.updateOv(${o.id}, 'value', this.value)">
                </div>
                <button type="button" class="lab-override-remove" onclick="NopsAI.pages.lab.removeOv(${o.id})" aria-label="Remove override">
                    <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                    </svg>
                </button>
            `;
            list.appendChild(div);
        });
        if (DOM['lab-overrides-empty']) DOM['lab-overrides-empty'].classList.toggle('hidden', state.overrides.length > 0);
        renderSummary();
    }

    function notifyEditingLock() {
        state.lastFeedback = { status: 'error', message: 'Save or reset your lab edits before leaving.' };
        renderFeedback();
    }

    function restoreEditingRoute(targetHash) {
        const desiredHash = normalizeLabHash(state.lastRouteHash || '#/lab');
        const currentHash = normalizeLabHash(typeof targetHash === 'string' ? targetHash : window.location.hash);
        if (currentHash === desiredHash) return;
        suppressNextRouteOnce();
        try {
            const url = new URL(window.location.href);
            url.hash = desiredHash.slice(1);
            history.replaceState(null, '', url.toString());
        } catch {
            window.location.hash = desiredHash;
        }
    }

    function suppressNextRouteOnce() {
        if (!context || !context.state) return;
        const appState = context.state;
        appState._suppressNextRoute = true;
        if (appState._suppressRouteTimeout) {
            try { clearTimeout(appState._suppressRouteTimeout); } catch {}
        }
        try {
            appState._suppressRouteTimeout = setTimeout(() => {
                appState._suppressNextRoute = false;
                appState._suppressRouteTimeout = null;
            }, 100);
        } catch {
            appState._suppressNextRoute = false;
        }
    }

    function normalizeLabHash(rawHash) {
        let hash = rawHash;
        if (!hash) hash = '#/lab';
        if (typeof hash !== 'string') {
            hash = String(hash || '');
        }
        hash = hash.trim();
        if (!hash.startsWith('#')) {
            hash = hash.startsWith('/') ? `#${hash.slice(1)}` : `#${hash}`;
        }
        hash = hash.replace(/^#\/+/, '#/');
        hash = hash.replace(/\/+$/g, '');
        return hash || '#/lab';
    }

    function preventNavigation(targetHash, previousHash) {
        if (!state.isEditing || !state.hasUnsavedChanges) return false;
        state.lastRouteHash = normalizeLabHash(previousHash || state.lastRouteHash || '#/lab');
        notifyEditingLock();
        restoreEditingRoute(targetHash);
        return true;
    }

    function onLeave() {
        exitEditMode();
        if (DOM['lab-suggestion-panel']) {
            DOM['lab-suggestion-panel'].classList.add('hidden');
        }
    }

    // Exposed helpers for inline event handlers
    global.pages = global.pages || {};
    global.pages.lab = {
        init, handleRoute, preventNavigation, onLeave,
        updateOv: (id, f, v) => { const o = state.overrides.find(x=>x.id===id); if(o) o[f]=v; renderSummary(); },
        removeOv: (id) => { state.overrides = state.overrides.filter(x=>x.id!==id); renderOverrides(); }
    };

    // Utils
    function escapeHtml(s) { return (s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/"/g,'&quot;'); }
    function escapeAttribute(s) { return escapeHtml(s); }
    function clearFeedback() { state.lastFeedback=null; renderFeedback(); }
    function renderFeedback() {
        const fb = DOM['lab-run-feedback'];
        if(!fb) return;
        if(!state.lastFeedback) { fb.classList.add('hidden'); return; }
        fb.classList.remove('hidden', 'text-red-500', 'text-green-500');
        fb.classList.add(state.lastFeedback.status==='error'?'text-red-500':'text-green-500');
        fb.innerHTML = state.lastFeedback.message + (state.lastFeedback.runId ? ` <a href="#/pipelineruns/main/${state.lastFeedback.runId}" class="underline">View</a>` : '');
    }
    function updatePipelineLink() {
        const a = DOM['lab-open-pipeline'];
        if(a) {
            a.classList.toggle('hidden', !state.selectedPipeline);
            a.href = `#/pipelines/${encodeURIComponent(state.selectedPipeline)}`;
        }
    }
    function handleScopeInput(e) { state.scopeValue = e.target.value; renderSummary(); }
    function copyYamlToClipboard() { navigator.clipboard.writeText(state.currentYaml); }
    function resetYamlToSource() { state.currentYaml = state.originalYaml; renderEditor(); }
    function renderSummary() {
        if(DOM['lab-summary-pipeline']) DOM['lab-summary-pipeline'].textContent = state.selectedPipeline || '(unsaved)';
        if(DOM['lab-summary-scope']) DOM['lab-summary-scope'].textContent = state.scopeValue || 'default';
        if(DOM['lab-summary-overrides']) DOM['lab-summary-overrides'].textContent = state.overrides.length;
        updateActionState();
    }
    function updateActionState() {
        const btn = DOM['lab-run-btn'];
        if(btn) btn.disabled = state.isRunning || state.validationErrors.length > 0;
    }
    function setSuggestionPanelCopy(t) {
        if(DOM['lab-suggestion-title']) DOM['lab-suggestion-title'].textContent = t.title;
        if(DOM['lab-suggestion-subtitle']) DOM['lab-suggestion-subtitle'].textContent = t.subtitle;
    }

    // --- Validation (Strict) ---
    const VALIDATION_EXAMPLES = [
        { pattern: /Unknown field '.*'/i, example: `name: demo-pipeline\nversion: latest\nsteps:\n  - name: build\n    script: echo "hello"` },
        { pattern: /At least one step is required/i, example: `steps:\n  - name: build\n    script: echo "hello"` },
        { pattern: /Duplicate step name/i, example: `steps:\n  - name: build\n    script: echo "first"\n  - name: build\n    script: echo "second"` },
        { pattern: /Duplicate task name/i, example: `steps:\n  - name: build\n    tasks:\n      - name: compile\n        script: make\n      - name: compile\n        script: make test` },
        { pattern: /has an empty 'goal'/i, example: `steps:\n  - name: summarize\n    goal: "Describe the changes for release notes"` },
        { pattern: /has an empty 'script'/i, example: `steps:\n  - name: build\n    script: |\n      npm run build` },
        { pattern: /empty 'include'/i, example: `steps:\n  - name: reuse\n    include: "step:path/to/reusable"` },
        { pattern: /must define either 'goal' or 'script'/i, example: `steps:\n  - name: lint\n    tasks:\n      - name: run-lint\n        script: |\n          npm run lint` }
    ];

    function buildValidationExample(message) {
        if (!message) return '';
        for (const entry of VALIDATION_EXAMPLES) {
            if (entry.pattern.test(message)) {
                return entry.example;
            }
        }
        return '';
    }

    function handleValidation() {
        const editor = DOM['lab-yaml-editor'];
        if (!editor) return;
        updateEditorHighlight();
        const yamlString = editor.value || '';
        const result = validatePipelineYaml(yamlString);
        applyValidationResult(result);
        renderLabIncludes(yamlString);
    }

    function applyValidationResult(result) {
        const errors = (result && Array.isArray(result.errors)) ? result.errors : [];
        state.validationErrors = errors;
        updateLineNumbers();

        const status = DOM['lab-validation-status'];
        if (!status) {
            updateActionState();
            return;
        }
        status.classList.remove('hidden');

        const baseClass = 'validation-box validation-box--inline';
        if (errors.length) {
            const items = errors.map(err => {
                const lineLabel = typeof err.line === 'number' ? `<span class="validation-box__line">Line ${err.line}</span>` : '';
                const message = `<div class="validation-box__message">${escapeHtml(err.message)}</div>`;
                const example = buildValidationExample(err.message);
                const exampleHtml = example ? `<pre class="validation-box__example"><code>${escapeHtml(example)}</code></pre>` : '';
                return `<div class="validation-box__item">${lineLabel}${message}${exampleHtml}</div>`;
            }).join('');
            status.innerHTML = `<div class="validation-box__header">Validation issues</div>${items}`;
            status.className = `${baseClass} validation-box--error`;
            if (DOM['lab-save-yaml']) DOM['lab-save-yaml'].disabled = true;
        } else {
            status.innerHTML = '<div class="validation-box__header">Valid</div>';
            status.className = `${baseClass} validation-box--success`;
            if (DOM['lab-save-yaml']) DOM['lab-save-yaml'].disabled = false;
        }
        updateActionState();
    }

    function renderLabIncludes(yamlString) {
        const container = DOM['lab-includes'];
        if (!container) return;
        const parsed = parseYamlSafely(yamlString);
        if (!parsed || !Array.isArray(parsed.steps) || parsed.steps.length === 0) {
            container.innerHTML = `<p>${escapeHtml(container.dataset.empty || 'No steps defined.')}</p>`;
            return;
        }
        const includes = new Set();
        parsed.steps.forEach(step => {
            if (step && typeof step.include === 'string' && step.include.trim()) {
                includes.add(step.include.trim());
            }
        });
        if (includes.size === 0) {
            container.innerHTML = '<p class="text-[var(--text-secondary)]">No included dependencies found.</p>';
            return;
        }

        const renderStepLogo = global.NopsAI && global.NopsAI.ui && global.NopsAI.ui.renderStepLogo;
        const buildBadge = (svgMarkup, variant = 'list', logoClass = '') => {
            if (typeof renderStepLogo === 'function') {
                return renderStepLogo(variant, logoClass, svgMarkup);
            }
            return svgMarkup;
        };

        const items = Array.from(includes).sort();
        container.innerHTML = `<ul class="triggers-pipeline-list">${items.map(item => {
            const isPipeline = item.startsWith('pipeline:');
            const isStep = item.startsWith('step:');
            let identifier = item;
            let href = '#';
            let icon = buildBadge('<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l4-4 4 4m0 6l-4 4-4-4" /></svg>', 'list', 'step-logo--system');
            let title = `Dependency: ${escapeHtml(item)}`;

            if (isPipeline) {
                identifier = item.substring('pipeline:'.length);
                href = `#/pipelines/${identifier.split('/').map(encodeURIComponent).join('/')}`;
                title = `Open pipeline ${escapeHtml(identifier)}`;
                icon = buildBadge('<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h10"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 17h10"/><circle cx="7" cy="7" r="1.5" fill="currentColor" stroke="none"/><circle cx="17" cy="7" r="1.5" fill="currentColor" stroke="none"/><circle cx="7" cy="17" r="1.5" fill="currentColor" stroke="none"/><circle cx="17" cy="17" r="1.5" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.5" fill="currentColor" stroke="none"/></svg>', 'list', 'step-logo--pipelines');
            } else if (isStep) {
                identifier = item.substring('step:'.length);
                href = `#/steps/${identifier.split('/').map(encodeURIComponent).join('/')}`;
                title = `Open step ${escapeHtml(identifier)}`;
                icon = buildBadge('<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 10l6-3 6 3-6 3-6-3z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 14l6 3 6-3"/></svg>', 'list', 'step-logo--steps');
            }

            return `
                <li class="triggers-pipeline-item">
                    <a href="${href}" class="triggers-pipeline-link" title="${title}">
                        <div class="flex items-center gap-2">
                            ${icon}
                            <span class="triggers-pipeline-name">${escapeHtml(item)}</span>
                        </div>
                    </a>
                </li>`;
        }).join('')}</ul>`;
    }

    function parseYamlSafely(text) {
        if (!text || !window.jsyaml) return null;
        try {
            return window.jsyaml.load(text);
        } catch {
            return null;
        }
    }

    function validatePipelineYaml(yamlString) {
        if (!window.jsyaml) return { errors: [{ message: 'YAML parser is unavailable.' }] };

        const pathIndex = buildYamlPathIndex(yamlString);

        const knownPipelineKeys = new Set(['name', 'version', 'description', 'container_image', 'display_options', 'working_directory', 'variables', 'steps', 'timeout', 'llm_content_sharing', 'llm_output_sharing', 'llm_content_ignore']);
        const knownStepKeys = new Set(['name', 'include', 'sync', 'image', 'secrets', 'volumes', 'variables', 'tasks', 'condition', 'goal', 'script', 'depends_on', 'ignore_failure', 'llm_output_sharing']);
        const knownTaskKeys = new Set(['name', 'goal', 'script', 'depends_on', 'ignore_failure', 'llm_output_sharing']);
        const knownDisplayOptionsKeys = new Set(['github_view']);

        const createError = (message, pathHints = []) => {
            let line = null;
            for (const hint of pathHints) {
                if (!hint) continue;
                if (hint.startsWith('line:')) {
                    const direct = Number(hint.slice(5));
                    if (!Number.isNaN(direct) && direct > 0) {
                        line = direct;
                        break;
                    }
                    continue;
                }
                const candidate = pathIndex.get(hint);
                if (typeof candidate === 'number') {
                    line = candidate;
                    break;
                }
            }
            return { message, line };
        };

        function findUnknownKeys(obj, knownKeys, path = '') {
            if (!obj || typeof obj !== 'object' || Array.isArray(obj)) {
                return [];
            }
            const unknown = [];
            for (const key in obj) {
                if (!knownKeys.has(key)) {
                    unknown.push({ path: path ? `${path}.${key}` : key, key });
                }
            }
            return unknown;
        }

        function checkAllKeys(pipeline) {
            let allUnknown = findUnknownKeys(pipeline, knownPipelineKeys);
            if (pipeline.display_options) {
                allUnknown = allUnknown.concat(findUnknownKeys(pipeline.display_options, knownDisplayOptionsKeys, 'display_options'));
            }

            if (Array.isArray(pipeline.steps)) {
                pipeline.steps.forEach((step, index) => {
                    const stepPath = `steps[${index}]`;
                    allUnknown = allUnknown.concat(findUnknownKeys(step, knownStepKeys, stepPath));
                    if (Array.isArray(step.tasks)) {
                        step.tasks.forEach((task, taskIndex) => {
                            const taskPath = `${stepPath}.tasks[${taskIndex}]`;
                            allUnknown = allUnknown.concat(findUnknownKeys(task, knownTaskKeys, taskPath));
                        });
                    }
                });
            }
            return allUnknown;
        }

        try {
            const pipeline = window.jsyaml.load(yamlString);
            if (!pipeline) return { errors: [createError('YAML is empty or invalid.', [''])] };
            if (typeof pipeline !== 'object') return { errors: [createError('YAML root must be an object.', [''])] };

            const unknownKeys = checkAllKeys(pipeline);
            if (unknownKeys.length > 0) {
                return { errors: unknownKeys.map(item => createError(`Validation Error: Unknown field '${item.key}'.`, [item.path])) };
            }
            if (!pipeline.name) return { errors: [createError("Validation Error: 'name' is a required field.", ['name'])] };
            const allowed = /^[a-zA-Z0-9_.-]+$/;
            if (!allowed.test(pipeline.name)) {
                return { errors: [createError(`Validation Error: Pipeline name '${pipeline.name}' contains invalid characters.`, ['name'])] };
            }
            if (pipeline.version && !allowed.test(pipeline.version)) {
                return { errors: [createError(`Validation Error: Pipeline version '${pipeline.version}' contains invalid characters.`, ['version'])] };
            }
            if (!Array.isArray(pipeline.steps) || pipeline.steps.length === 0) {
                return { errors: [createError("Validation Error: At least one step is required in 'steps'.", ['steps'])] };
            }
            const stepNames = new Set();
            const stepTaskMaps = new Map();
            for (let index = 0; index < pipeline.steps.length; index++) {
                const step = pipeline.steps[index];
                const stepPath = `steps[${index}]`;
                if (!step || typeof step !== 'object') {
                    return { errors: [createError('Validation Error: A step is not a valid object.', [stepPath])] };
                }
                if (!step.name) {
                    return { errors: [createError('Validation Error: All steps require a name.', [`${stepPath}.name`, stepPath])] };
                }
                if (stepNames.has(step.name)) {
                    return { errors: [createError(`Validation Error: Duplicate step name '${step.name}' found.`, [`${stepPath}.name`, stepPath])] };
                }
                stepNames.add(step.name);

                const hasIncludeKey = Object.prototype.hasOwnProperty.call(step, 'include');
                const includeValue = hasIncludeKey ? step.include : null;
                const includeValid = typeof includeValue === 'string' && includeValue.trim().length > 0;
                if (hasIncludeKey && !includeValid) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' has an empty 'include' value.`, [`${stepPath}.include`, stepPath])] };
                }
                const isInclude = includeValid;

                const hasTasksKey = Object.prototype.hasOwnProperty.call(step, 'tasks');
                if (hasTasksKey && !Array.isArray(step.tasks)) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' has 'tasks' but the value is not an array.`, [`${stepPath}.tasks`, stepPath])] };
                }
                const hasTasks = Array.isArray(step.tasks) && step.tasks.length > 0;
                if (hasTasksKey && !hasTasks) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' must define at least one task when using 'tasks'.`, [`${stepPath}.tasks`, stepPath])] };
                }

                const hasGoalKey = Object.prototype.hasOwnProperty.call(step, 'goal');
                const goalValue = hasGoalKey ? step.goal : null;
                const hasGoalContent = typeof goalValue === 'string' && goalValue.trim().length > 0;

                const hasScriptKey = Object.prototype.hasOwnProperty.call(step, 'script');
                const scriptValue = hasScriptKey ? step.script : null;
                const hasScriptContent = typeof scriptValue === 'string' && scriptValue.trim().length > 0;

                if (hasGoalKey && !hasGoalContent) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' has an empty 'goal'.`, [`${stepPath}.goal`, stepPath])] };
                }
                if (hasScriptKey && !hasScriptContent) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' has an empty 'script'.`, [`${stepPath}.script`, stepPath])] };
                }

                if (hasGoalKey && hasScriptKey) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' cannot define both 'goal' and 'script'.`, [`${stepPath}.goal`, `${stepPath}.script`, stepPath])] };
                }

                const hasLegacyContent = hasGoalContent || hasScriptContent;

                if (!isInclude && !hasTasks && !hasLegacyContent) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' must contain 'include', 'tasks', 'goal', or 'script'.`, [`${stepPath}.include`, `${stepPath}.tasks`, `${stepPath}.goal`, `${stepPath}.script`, stepPath])] };
                }
                if (isInclude && (hasTasksKey || hasGoalKey || hasScriptKey)) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' is an include step and cannot also contain tasks, goal, or script.`, [`${stepPath}.include`, `${stepPath}.tasks`, `${stepPath}.goal`, `${stepPath}.script`, stepPath])] };
                }
                if (hasTasks && (hasGoalKey || hasScriptKey)) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' mixes tasks with goal/script.`, [`${stepPath}.tasks`, `${stepPath}.goal`, `${stepPath}.script`, stepPath])] };
                }

                if (hasTasks) {
                    const taskNames = new Set();
                    for (let taskIndex = 0; taskIndex < step.tasks.length; taskIndex++) {
                        const task = step.tasks[taskIndex];
                        const taskPath = `${stepPath}.tasks[${taskIndex}]`;
                        if (!task || typeof task !== 'object' || !task.name) {
                            return { errors: [createError(`Validation Error: A task in step '${step.name}' is missing its name.`, [`${taskPath}.name`, taskPath])] };
                        }
                        if (taskNames.has(task.name)) {
                            return { errors: [createError(`Validation Error: Duplicate task name '${task.name}' in step '${step.name}'.`, [`${taskPath}.name`, taskPath])] };
                        }
                        taskNames.add(task.name);

                        const taskHasGoalKey = Object.prototype.hasOwnProperty.call(task, 'goal');
                        const taskGoalValue = taskHasGoalKey ? task.goal : null;
                        const taskHasGoalContent = typeof taskGoalValue === 'string' && taskGoalValue.trim().length > 0;

                        const taskHasScriptKey = Object.prototype.hasOwnProperty.call(task, 'script');
                        const taskScriptValue = taskHasScriptKey ? task.script : null;
                        const taskHasScriptContent = typeof taskScriptValue === 'string' && taskScriptValue.trim().length > 0;

                        if (taskHasGoalKey && taskHasScriptKey) {
                            return { errors: [createError(`Validation Error: Task '${task.name}' in step '${step.name}' cannot define both 'goal' and 'script'.`, [`${taskPath}.goal`, `${taskPath}.script`, taskPath])] };
                        }
                        if (taskHasGoalKey && !taskHasGoalContent) {
                            return { errors: [createError(`Validation Error: Task '${task.name}' in step '${step.name}' has an empty 'goal'.`, [`${taskPath}.goal`, taskPath])] };
                        }
                        if (taskHasScriptKey && !taskHasScriptContent) {
                            return { errors: [createError(`Validation Error: Task '${task.name}' in step '${step.name}' has an empty 'script'.`, [`${taskPath}.script`, taskPath])] };
                        }
                        if (!taskHasGoalContent && !taskHasScriptContent) {
                            return { errors: [createError(`Validation Error: Task '${task.name}' in step '${step.name}' must define either 'goal' or 'script'.`, [`${taskPath}.goal`, `${taskPath}.script`, taskPath])] };
                        }
                    }
                    for (let taskIndex = 0; taskIndex < step.tasks.length; taskIndex++) {
                        const task = step.tasks[taskIndex];
                        if (Array.isArray(task.depends_on)) {
                            for (const dep of task.depends_on) {
                                if (!taskNames.has(dep)) {
                                    const taskPath = `${stepPath}.tasks[${taskIndex}].depends_on`;
                                    return { errors: [createError(`Validation Error: Task '${task.name}' in step '${step.name}' depends on unknown task '${dep}'.`, [taskPath])] };
                                }
                            }
                        }
                    }
                    stepTaskMaps.set(step.name, taskNames);
                }
            }

            for (let index = 0; index < pipeline.steps.length; index++) {
                const step = pipeline.steps[index];
                if (Array.isArray(step.depends_on)) {
                    for (const dep of step.depends_on) {
                        if (!stepNames.has(dep)) {
                            return { errors: [createError(`Validation Error: Step '${step.name}' depends on unknown step '${dep}'.`, [`steps[${index}].depends_on`, `steps[${index}]`])] };
                        }
                    }
                }
            }

            return { errors: [] };
        } catch (error) {
            if (error && error.mark && typeof error.mark.line === 'number') {
                return { errors: [createError(`YAML Parsing Error: ${error.message}`, [`line:${error.mark.line + 1}`])] };
            }
            return { errors: [createError(`YAML Parsing Error: ${error.message}`)] };
        }
    }

    function buildYamlPathIndex(yamlString) {
        const index = new Map();
        if (typeof yamlString !== 'string' || !yamlString.length) {
            return index;
        }
        const lines = yamlString.split('\n');
        const stack = [];

        const pushContext = (indent, path, type) => {
            stack.push({ indent, path, type, nextIndex: 0 });
        };

        const popToIndent = indent => {
            while (stack.length && indent < stack[stack.length - 1].indent) {
                stack.pop();
            }
        };

        const setPathIndex = (path, lineNumber) => {
            if (path) {
                index.set(path, lineNumber);
            }
        };

        lines.forEach((line, idx) => {
            const lineNumber = idx + 1;
            const indentMatch = line.match(/^\s*/);
            const indent = indentMatch ? indentMatch[0].length : 0;
            const trimmed = line.trim();

            if (!trimmed || trimmed.startsWith('#')) {
                return;
            }

            popToIndent(indent);

            if (trimmed.startsWith('-')) {
                const parent = stack[stack.length - 1];
                if (!parent || parent.type !== 'array') {
                    return;
                }
                const itemIndex = parent.nextIndex++;
                const itemPath = `${parent.path}[${itemIndex}]`;
                setPathIndex(itemPath, lineNumber);

                const rest = trimmed.slice(1).trim();
                const keyMatch = rest.match(/^([A-Za-z0-9_]+)\s*:/);
                if (keyMatch) {
                    const key = keyMatch[1];
                    setPathIndex(`${itemPath}.${key}`, lineNumber);
                    const endsWithColon = rest.endsWith(':');
                    if (endsWithColon) {
                        const isArrayKey = ARRAY_KEYS.has(key);
                        pushContext(indent + 2, `${itemPath}.${key}`, isArrayKey ? 'array' : 'object');
                    }
                } else {
                    pushContext(indent + 2, itemPath, 'object');
                }
                return;
            }

            const keyMatch = trimmed.match(/^([A-Za-z0-9_]+)\s*:/);
            if (!keyMatch) {
                return;
            }
            const key = keyMatch[1];
            const parentPath = stack.length ? stack[stack.length - 1].path : '';
            const currentPath = parentPath ? `${parentPath}.${key}` : key;
            setPathIndex(currentPath, lineNumber);

            const endsWithColon = trimmed.endsWith(':');
            if (endsWithColon) {
                const isArrayKey = ARRAY_KEYS.has(key);
                pushContext(indent + 2, currentPath, isArrayKey ? 'array' : 'object');
            }
        });

        return index;
    }

})(window.NopsAI = window.NopsAI || {});
