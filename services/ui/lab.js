(function (global) {
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
        variableSuggestions: [],
        variableSuggestionCache: new Map(),
        variableSuggestionPromise: null,
        variableSuggestionLoadedAt: 0,
    };

    const OVERRIDE_KEY_PATTERN = /^[A-Za-z0-9_.-]+$/;
    const DEFAULT_PIPELINE_NAME = 'ad-hoc-pipeline';

    const DOM = {};
    let context = null;
    let initialized = false;
    let overrideSeq = 0;

    function init(ctx) {
        if (initialized && context === ctx) {
            return;
        }
        context = ctx;
        mapDom();
        attachEvents();
        initialized = true;
    }

    function mapDom() {
        const ids = [
            'lab-pipeline-select',
            'lab-refresh-pipelines',
            'lab-open-pipeline',
            'lab-scope-input',
            'lab-scope-options',
            'lab-yaml-editor',
            'lab-yaml-highlight',
            'lab-line-numbers',
            'lab-yaml-stage',
            'lab-editor-wrapper',
            'lab-overrides-list',
            'lab-overrides-empty',
            'lab-add-override',
            'lab-summary-pipeline',
            'lab-summary-scope',
            'lab-summary-overrides',
            'lab-run-btn',
            'lab-run-feedback',
            'lab-copy-yaml',
            'lab-reset-yaml',
            'lab-blank-pipeline',
            'lab-validation-status',
            'lab-suggestion-panel',
            'lab-suggestion-list',
            'lab-suggestion-empty',
            'lab-suggestion-title',
            'lab-suggestion-subtitle',
            'lab-suggestion-footnote',
        ];
        ids.forEach(id => {
            DOM[id] = document.getElementById(id);
        });

        if (DOM['lab-suggestion-panel']) {
            DOM['lab-suggestion-panel'].addEventListener('click', handleSuggestionClick);
        }
    }

    function attachEvents() {
        if (DOM['lab-pipeline-select']) {
            DOM['lab-pipeline-select'].addEventListener('change', handlePipelineChange);
        }
        if (DOM['lab-scope-input']) {
            DOM['lab-scope-input'].addEventListener('input', handleScopeInput);
        }
        if (DOM['lab-add-override']) {
            DOM['lab-add-override'].addEventListener('click', () => addOverride());
        }
        if (DOM['lab-refresh-pipelines']) {
            DOM['lab-refresh-pipelines'].addEventListener('click', () => loadPipelines(true));
        }
        if (DOM['lab-run-btn']) {
            DOM['lab-run-btn'].addEventListener('click', handleRunClick);
        }
        if (DOM['lab-copy-yaml']) {
            DOM['lab-copy-yaml'].addEventListener('click', copyYamlToClipboard);
        }
        if (DOM['lab-reset-yaml']) {
            DOM['lab-reset-yaml'].addEventListener('click', resetYamlToSource);
        }
        if (DOM['lab-blank-pipeline']) {
            DOM['lab-blank-pipeline'].addEventListener('click', startBlankPipeline);
        }
        if (DOM['lab-yaml-editor']) {
            DOM['lab-yaml-editor'].addEventListener('input', handleEditorInput);
            DOM['lab-yaml-editor'].addEventListener('scroll', syncEditorScroll);
            DOM['lab-yaml-editor'].addEventListener('keydown', handleEditorKeydown);
            DOM['lab-yaml-editor'].addEventListener('click', () => {
                updateSuggestionPanel();
                renderValidation();
            });
            DOM['lab-yaml-editor'].addEventListener('keyup', (event) => {
                if (['Shift', 'Control', 'Alt', 'Meta'].includes(event.key)) return;
                updateSuggestionPanel();
            });
        }
    }

    async function handleRoute() {
        if (!initialized) {
            init(context);
        }
        clearFeedback();
        renderOverrides();
        renderSummary();
        await Promise.all([loadPipelines(), loadScopes()]);
        if (!state.currentYaml && state.selectedPipeline) {
            await loadPipelineYaml(state.selectedPipeline);
        }
        renderValidation();
        updateSuggestionPanel();
    }

    async function loadPipelines(force = false) {
        if (!context || typeof context.fetchData !== 'function') return;
        if (state.pipelinesLoaded && !force) {
            renderPipelineOptions();
            return;
        }
        const previousSelection = state.selectedPipeline;
        state.pipelineSources = new Map();
        try {
            const response = await context.fetchData('/v1/pipelines?include_source=true');
            state.pipelines = parsePipelineList(response);
            state.pipelines.forEach(item => {
                if (item.source) {
                    state.pipelineSources.set(item.id, item.source);
                }
            });
        } catch (error) {
            console.error('Failed to load pipelines for Lab page:', error);
            state.pipelines = [];
        }
        state.pipelinesLoaded = true;
        if (!state.pipelines.find(p => p.id === previousSelection)) {
            state.selectedPipeline = '';
        }
        renderPipelineOptions();
    }

    function parsePipelineList(response) {
        if (!Array.isArray(response)) {
            return [];
        }
        const items = [];
        response.forEach(item => {
            if (typeof item === 'string') {
                const id = item.trim();
                if (id) {
                    items.push({ id, source: '' });
                }
                return;
            }
            if (!item || typeof item !== 'object') return;
            const id = String(item.id || item.identifier || item.pipeline || '').trim();
            if (!id) return;
            items.push({ id, source: String(item.source || '').trim() });
        });
        items.sort((a, b) => a.id.localeCompare(b.id));
        return items;
    }

    function renderPipelineOptions() {
        const select = DOM['lab-pipeline-select'];
        if (!select) return;
        select.innerHTML = '';

        const placeholder = document.createElement('option');
        placeholder.value = '';
        placeholder.textContent = state.pipelines.length ? 'Select a pipeline' : 'No pipelines available';
        select.appendChild(placeholder);

        state.pipelines.forEach(item => {
            const option = document.createElement('option');
            option.value = item.id;
            const sourceLabel = item.source ? ` (${formatSourceLabel(item.source)})` : '';
            option.textContent = `${item.id}${sourceLabel}`;
            if (item.id === state.selectedPipeline) {
                option.selected = true;
            }
            select.appendChild(option);
        });

        if (state.selectedPipeline && select.value !== state.selectedPipeline) {
            select.value = state.selectedPipeline;
        }

        updatePipelineLink();
        renderSummary();
        updateActionState();
    }

    function formatSourceLabel(source) {
        const normalized = String(source || '').toLowerCase();
        if (normalized.includes('git')) return 'Git';
        if (normalized.includes('draft')) return 'Draft';
        if (normalized.includes('override')) return 'Override';
        if (normalized.includes('local')) return 'Local';
        if (normalized.includes('db') || normalized.includes('database')) return 'Database';
        return source || '';
    }

    async function loadScopes(force = false) {
        if (!context || typeof context.fetchData !== 'function') return;
        if (state.scopesLoaded && !force) {
            renderScopeOptions();
            return;
        }
        const scopeSet = new Set();
        try {
            const [secretScopes, variableScopes] = await Promise.all([
                context.fetchData('/v1/secrets/scopes'),
                context.fetchData('/v1/variables/scopes'),
            ]);
            addScopesToSet(scopeSet, secretScopes);
            addScopesToSet(scopeSet, variableScopes);
        } catch (error) {
            console.error('Failed to load scopes for Lab page:', error);
        }
        state.scopes = Array.from(scopeSet).sort((a, b) => a.localeCompare(b));
        state.scopesLoaded = true;
        renderScopeOptions();
    }

    function addScopesToSet(set, entries) {
        if (!set || !entries) return;
        if (Array.isArray(entries)) {
            entries.forEach(entry => {
                const value = normalizeScopeEntry(entry);
                if (value) {
                    set.add(value);
                }
            });
        } else if (typeof entries === 'string') {
            const value = normalizeScopeEntry(entries);
            if (value) set.add(value);
        }
    }

    function normalizeScopeEntry(entry) {
        if (entry == null) return '';
        if (typeof entry === 'string') return entry.trim();
        if (typeof entry === 'object') {
            const value = entry.scope ?? entry.env ?? entry.name ?? entry.value ?? '';
            return typeof value === 'string' ? value.trim() : '';
        }
        return '';
    }

    function renderScopeOptions() {
        const datalist = DOM['lab-scope-options'];
        if (!datalist) return;
        datalist.innerHTML = '';
        state.scopes.forEach(scope => {
            const option = document.createElement('option');
            option.value = scope;
            datalist.appendChild(option);
        });
    }

    function handlePipelineChange(event) {
        state.selectedPipeline = event.target.value || '';
        clearFeedback();
        updatePipelineLink();
        if (state.selectedPipeline) {
            loadPipelineYaml(state.selectedPipeline);
        } else {
            state.originalYaml = '';
            state.currentYaml = '';
            renderEditor();
        }
        renderSummary();
        renderValidation();
        updateSuggestionPanel();
        updateActionState();
    }

    function handleScopeInput(event) {
        state.scopeValue = event.target.value || '';
        clearFeedback();
        renderSummary();
    }

    async function loadPipelineYaml(identifier) {
        if (!identifier || !context || typeof context.fetchData !== 'function') {
            return;
        }
        state.isLoadingYaml = true;
        updateActionState();
        try {
            const encoded = identifier.split('/').map(encodeURIComponent).join('/');
            const result = await context.fetchData(`/v1/pipelines/${encoded}`);
            if (typeof result === 'string') {
                state.originalYaml = result;
                state.currentYaml = result;
                renderEditor();
            } else {
                state.originalYaml = '';
                state.currentYaml = '';
                renderEditor();
            }
        } catch (error) {
            console.error('Failed to load pipeline YAML for Lab page:', error);
        } finally {
            state.isLoadingYaml = false;
            updateActionState();
        }
    }

    function addOverride(initial = {}) {
        state.overrides.push({
            id: `ov-${Date.now()}-${++overrideSeq}`,
            key: initial.key || '',
            value: initial.value || '',
        });
        clearFeedback();
        renderOverrides();
    }

    function updateOverride(id, updates) {
        const next = state.overrides.map(item => {
            if (item.id !== id) return item;
            return { ...item, ...updates };
        });
        state.overrides = next;
        clearFeedback();
        renderSummary();
    }

    function removeOverride(id) {
        state.overrides = state.overrides.filter(item => item.id !== id);
        clearFeedback();
        renderOverrides();
    }

    function renderOverrides() {
        const list = DOM['lab-overrides-list'];
        const empty = DOM['lab-overrides-empty'];
        if (!list || !empty) return;
        list.innerHTML = '';

        if (!state.overrides.length) {
            empty.classList.remove('hidden');
            renderSummary();
            return;
        }
        empty.classList.add('hidden');

        state.overrides.forEach(item => {
            const row = document.createElement('div');
            row.className = 'border border-[var(--border-primary)] bg-[var(--bg-tertiary)] rounded-lg p-3';
            row.dataset.id = item.id;

            const grid = document.createElement('div');
            grid.className = 'grid grid-cols-1 md:grid-cols-12 gap-3 items-center';

            const keyWrap = document.createElement('div');
            keyWrap.className = 'md:col-span-5';
            const keyInput = document.createElement('input');
            keyInput.type = 'text';
            keyInput.placeholder = 'KEY';
            keyInput.value = item.key || '';
            keyInput.className = 'w-full bg-[var(--bg-primary)] border border-[var(--border-input)] rounded-md py-2 px-3 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-[var(--border-accent)] focus:border-[var(--border-accent)]';
            keyInput.addEventListener('input', e => updateOverride(item.id, { key: e.target.value }));
            keyWrap.appendChild(keyInput);

            const valueWrap = document.createElement('div');
            valueWrap.className = 'md:col-span-6';
            const valueInput = document.createElement('input');
            valueInput.type = 'text';
            valueInput.placeholder = 'Value';
            valueInput.value = item.value || '';
            valueInput.className = 'w-full bg-[var(--bg-primary)] border border-[var(--border-input)] rounded-md py-2 px-3 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-[var(--border-accent)] focus:border-[var(--border-accent)]';
            valueInput.addEventListener('input', e => updateOverride(item.id, { value: e.target.value }));
            valueWrap.appendChild(valueInput);

            const removeWrap = document.createElement('div');
            removeWrap.className = 'md:col-span-1 flex justify-start md:justify-end';
            const removeBtn = document.createElement('button');
            removeBtn.type = 'button';
            removeBtn.className = 'glass-button-ghost';
            removeBtn.innerHTML = `<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg><span>Remove</span>`;
            removeBtn.addEventListener('click', () => removeOverride(item.id));
            removeWrap.appendChild(removeBtn);

            grid.appendChild(keyWrap);
            grid.appendChild(valueWrap);
            grid.appendChild(removeWrap);
            row.appendChild(grid);
            list.appendChild(row);
        });

        renderSummary();
    }

    function updatePipelineLink() {
        const link = DOM['lab-open-pipeline'];
        if (!link) return;
        if (!state.selectedPipeline) {
            link.classList.add('hidden');
            link.removeAttribute('href');
            return;
        }
        const encoded = state.selectedPipeline.split('/').map(encodeURIComponent).join('/');
        link.href = `#/pipelines/${encoded}`;
        link.classList.remove('hidden');
    }

    function getOverridesMap() {
        const overrides = {};
        state.overrides.forEach(item => {
            const key = (item.key || '').trim();
            if (!key) return;
            overrides[key] = item.value || '';
        });
        return overrides;
    }

    function findInvalidOverrideKeys(overridesMap) {
        return Object.keys(overridesMap).filter(key => !OVERRIDE_KEY_PATTERN.test(key));
    }

    function getPipelineNameForRun() {
        if (state.selectedPipeline) return state.selectedPipeline;
        const parsed = parsePipelineNameFromYaml(state.currentYaml);
        if (parsed) return parsed;
        return '';
    }

    function renderSummary() {
        const pipelineSummary = DOM['lab-summary-pipeline'];
        const scopeSummary = DOM['lab-summary-scope'];
        const overridesSummary = DOM['lab-summary-overrides'];
        const nameForRun = getPipelineNameForRun();
        const isEdited = state.originalYaml && state.currentYaml && state.currentYaml !== state.originalYaml;

        if (pipelineSummary) {
            const label = nameForRun || 'Not selected';
            pipelineSummary.textContent = isEdited ? `${label} (edited)` : label;
            pipelineSummary.title = label;
        }
        if (scopeSummary) {
            const scopeLabel = (state.scopeValue || '').trim();
            scopeSummary.textContent = scopeLabel || 'Default';
            scopeSummary.title = scopeLabel || 'Default';
        }
        if (overridesSummary) {
            overridesSummary.textContent = String(Object.keys(getOverridesMap()).length);
        }
        updateActionState();
    }

    function updateActionState() {
        const runBtn = DOM['lab-run-btn'];
        const nameForRun = getPipelineNameForRun();
        const disabled = state.isRunning || state.isLoadingYaml || !nameForRun || !state.currentYaml.trim() || (state.validationErrors && state.validationErrors.length > 0);
        if (runBtn) {
            runBtn.disabled = disabled;
            runBtn.classList.toggle('opacity-60', disabled);
            runBtn.classList.toggle('cursor-not-allowed', disabled);
        }
    }

    function clearFeedback() {
        state.lastFeedback = null;
        renderFeedback();
    }

    function renderFeedback() {
        const target = DOM['lab-run-feedback'];
        if (!target) return;
        if (!state.lastFeedback) {
            target.classList.add('hidden');
            target.textContent = '';
            return;
        }

        target.classList.remove('hidden');
        target.classList.remove('text-[var(--text-secondary)]', 'text-red-500', 'text-green-500');
        target.classList.add(state.lastFeedback.status === 'success' ? 'text-green-500' : 'text-red-500');
        target.innerHTML = '';

        const messageSpan = document.createElement('span');
        messageSpan.textContent = state.lastFeedback.message;
        target.appendChild(messageSpan);

        if (state.lastFeedback.runId) {
            const link = document.createElement('a');
            link.href = `#/pipelineruns/main/${state.lastFeedback.runId}`;
            link.textContent = 'View run';
            link.className = 'ml-2 text-[var(--border-accent)] hover:underline';
            target.appendChild(link);
        }
    }

    function extractRunId(text) {
        if (typeof text !== 'string') return '';
        const match = text.match(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i);
        return match ? match[0] : '';
    }

    async function handleRunClick() {
        if (!context || typeof context.fetchData !== 'function') {
            alert('API client unavailable.');
            return;
        }
        const nameForRun = getPipelineNameForRun() || parsePipelineNameFromYaml(state.currentYaml) || DEFAULT_PIPELINE_NAME;
        if (!state.currentYaml.trim()) {
            state.lastFeedback = { status: 'error', message: 'Pipeline YAML is required.' };
            renderFeedback();
            return;
        }

        setRunning(true);
        clearFeedback();
        renderValidation();
        if (state.validationErrors && state.validationErrors.length) {
            state.lastFeedback = { status: 'error', message: 'Resolve validation errors before running.' };
            renderFeedback();
            setRunning(false);
            return;
        }

        const scope = (state.scopeValue || '').trim();
        const overrides = getOverridesMap();
        const invalidKeys = findInvalidOverrideKeys(overrides);
        if (invalidKeys.length) {
            state.lastFeedback = { status: 'error', message: `Invalid variable name(s): ${invalidKeys.join(', ')}. Use letters, numbers, underscores, dots, or hyphens.` };
            renderFeedback();
            setRunning(false);
            return;
        }
        const payload = {
            pipeline: nameForRun,
            definition: state.currentYaml,
        };
        if (scope) payload.scope = scope;
        if (Object.keys(overrides).length) {
            payload.variables = overrides;
        }

        const headers = { 'Content-Type': 'application/json' };
        if (scope) {
            headers['X-Nopsai-Scope'] = scope;
        }

        try {
            const result = await context.fetchData('/v1/run', {
                method: 'POST',
                headers,
                body: JSON.stringify(payload),
            });
            if (!result) {
                state.lastFeedback = { status: 'error', message: 'Run request failed. Check alerts for details.' };
                renderFeedback();
                return;
            }
            const runId = extractRunId(result);
            state.lastFeedback = {
                status: 'success',
                message: runId ? `Run ${runId} started.` : String(result),
                runId: runId || '',
            };
            renderFeedback();
        } catch (error) {
            console.error('Failed to trigger run from Lab page:', error);
            state.lastFeedback = { status: 'error', message: 'Could not start run. See console for details.' };
            renderFeedback();
        } finally {
            setRunning(false);
        }
    }

    function setRunning(isRunning) {
        state.isRunning = isRunning;
        const runBtn = DOM['lab-run-btn'];
        if (runBtn) {
            const label = runBtn.querySelector('span');
            if (label) {
                label.textContent = isRunning ? 'Starting...' : 'Run with Scope';
            }
        }
        updateActionState();
    }

    function renderEditor() {
        const textarea = DOM['lab-yaml-editor'];
        if (!textarea) return;
        textarea.value = state.currentYaml || '';
        updateLineNumbers();
        updateEditorHighlight();
        renderSummary();
        renderValidation();
        updateSuggestionPanel();
    }

    function handleEditorInput(event) {
        state.currentYaml = event.target.value || '';
        updateLineNumbers();
        updateEditorHighlight();
        renderSummary();
        renderValidation();
        updateSuggestionPanel();
    }

    function handleEditorKeydown(event) {
        const textarea = DOM['lab-yaml-editor'];
        if (!textarea) return;
        if (event.key === 'Tab') {
            event.preventDefault();
            const { selectionStart, selectionEnd, value } = textarea;
            const insert = '  ';
            textarea.value = value.slice(0, selectionStart) + insert + value.slice(selectionEnd);
            const pos = selectionStart + insert.length;
            textarea.selectionStart = textarea.selectionEnd = pos;
            state.currentYaml = textarea.value;
            updateEditorHighlight();
            updateLineNumbers();
        }
    }

    function syncEditorScroll() {
        const textarea = DOM['lab-yaml-editor'];
        const highlight = DOM['lab-yaml-highlight'];
        const stage = DOM['lab-yaml-stage'];
        const lineNumbers = DOM['lab-line-numbers'];
        if (!textarea) return;
        if (highlight) {
            highlight.scrollTop = textarea.scrollTop;
            highlight.scrollLeft = textarea.scrollLeft;
        }
        if (stage) {
            stage.scrollTop = 0;
            stage.scrollLeft = 0;
        }
        if (lineNumbers) {
            lineNumbers.style.setProperty('--line-number-scroll', `${textarea.scrollTop || 0}px`);
        }
    }

    function updateEditorHighlight() {
        const highlight = DOM['lab-yaml-highlight'];
        const textarea = DOM['lab-yaml-editor'];
        const stage = DOM['lab-yaml-stage'];
        if (!highlight || !textarea) return;
        const renderer = global.yaml && typeof global.yaml.renderTokens === 'function'
            ? global.yaml.renderTokens
            : null;
        if (!renderer) {
            highlight.textContent = textarea.value || '';
            if (stage) stage.classList.remove('yaml-editor-stage--with-highlight');
            return;
        }
        highlight.innerHTML = renderer(textarea.value || '') || '&nbsp;';
        if (stage) stage.classList.add('yaml-editor-stage--with-highlight');
        syncEditorScroll();
    }

    function updateLineNumbers() {
        const textarea = DOM['lab-yaml-editor'];
        const lineNumbers = DOM['lab-line-numbers'];
        if (!textarea || !lineNumbers) return;
        const lines = textarea.value.split('\n');
        const errorMap = new Map();
        (state.validationErrors || []).forEach(err => {
            if (typeof err.line === 'number') {
                const list = errorMap.get(err.line) || [];
                list.push(err.message);
                errorMap.set(err.line, list);
            }
        });
        const numbersHtml = lines.map((_, idx) => {
            const lineNumber = idx + 1;
            const messages = errorMap.get(lineNumber);
            const classes = ['line-number'];
            if (messages && messages.length) classes.push('line-number--error');
            const titleAttr = messages && messages.length ? ` title="${escapeAttribute(messages.join('\n'))}"` : '';
            return `<div class="${classes.join(' ')}" data-line-number="${lineNumber}"${titleAttr}>${lineNumber}</div>`;
        }).join('');
        lineNumbers.innerHTML = `<div class="line-number-track">${numbersHtml}</div>`;
        syncEditorScroll();
    }

    function copyYamlToClipboard() {
        if (!navigator.clipboard || !DOM['lab-yaml-editor']) {
            alert('Clipboard not available in this browser.');
            return;
        }
        const text = DOM['lab-yaml-editor'].value || '';
        navigator.clipboard.writeText(text).then(() => {
            state.lastFeedback = { status: 'success', message: 'Pipeline YAML copied to clipboard.' };
            renderFeedback();
        }).catch(() => {
            state.lastFeedback = { status: 'error', message: 'Could not copy YAML to clipboard.' };
            renderFeedback();
        });
    }

    function resetYamlToSource() {
        if (!state.originalYaml) {
            state.currentYaml = '';
        } else {
            state.currentYaml = state.originalYaml;
        }
        renderEditor();
    }

    function startBlankPipeline() {
        state.selectedPipeline = '';
        state.originalYaml = buildBlankTemplate(DEFAULT_PIPELINE_NAME);
        state.currentYaml = state.originalYaml;
        if (DOM['lab-pipeline-select']) {
            DOM['lab-pipeline-select'].value = '';
        }
        updatePipelineLink();
        renderEditor();
        renderValidation();
        updateSuggestionPanel();
        renderSummary();
        renderValidation();
        updateSuggestionPanel();
        updateActionState();
    }

    function buildBlankTemplate(name) {
        const safeName = name && name.trim() ? name.trim() : DEFAULT_PIPELINE_NAME;
        return [
            `name: ${safeName}`,
            'version: latest',
            'description: Ad-hoc pipeline run from Lab',
            'variables: []',
            'steps:',
            '  - name: example',
            '    script: echo "Hello from Lab"',
            '',
        ].join('\n');
    }

    function renderValidation() {
        const validation = validatePipelineYaml(state.currentYaml || '');
        applyValidationResult(validation);
    }

    function applyValidationResult(result) {
        state.validationErrors = result && Array.isArray(result.errors) ? result.errors : [];
        updateLineNumbers();
        const status = DOM['lab-validation-status'];
        if (!status) return;
        if (!state.validationErrors.length) {
            status.textContent = 'No validation errors.';
            status.classList.remove('hidden', 'text-red-500');
            status.classList.add('text-[var(--text-secondary)]');
        } else {
            const first = state.validationErrors[0];
            const summary = `${state.validationErrors.length} validation error${state.validationErrors.length > 1 ? 's' : ''}: ${first.message}`;
            status.textContent = summary;
            status.classList.remove('hidden', 'text-[var(--text-secondary)]');
            status.classList.add('text-red-500');
        }
        updateActionState();
    }

    function buildYamlPathIndex(yamlString) {
        const index = new Map();
        if (typeof yamlString !== 'string' || !yamlString.length) {
            return index;
        }
        const lines = yamlString.split('\n');
        const stack = [];
        const ARRAY_KEYS = new Set(['steps', 'tasks', 'depends_on', 'variables', 'secrets', 'volumes']);

        const pushContext = (indent, path, type) => {
            stack.push({ indent, path, type, nextIndex: 0 });
        };

        const popToIndent = indent => {
            while (stack.length && indent < stack[stack.length - 1].indent) {
                stack.pop();
            }
        };

        lines.forEach((line, idx) => {
            const lineNumber = idx + 1;
            const indentMatch = line.match(/^\s*/);
            const indent = indentMatch ? indentMatch[0].length : 0;
            const trimmed = line.trim();
            popToIndent(indent);
            if (!trimmed || trimmed.startsWith('#')) {
                return;
            }

            if (trimmed.startsWith('-')) {
                const parent = stack[stack.length - 1];
                if (!parent || parent.type !== 'array') {
                    return;
                }
                const itemIndex = parent.nextIndex++;
                const itemPath = `${parent.path}[${itemIndex}]`;
                index.set(itemPath, lineNumber);

                const rest = trimmed.slice(1).trim();
                const keyMatch = rest.match(/^([A-Za-z0-9_]+)\s*:/);
                pushContext(indent + 2, itemPath, 'object');
                if (keyMatch) {
                    const key = keyMatch[1];
                    index.set(`${itemPath}.${key}`, lineNumber);
                    const endsWithColon = rest.endsWith(':');
                    if (endsWithColon) {
                        const isArrayKey = ARRAY_KEYS.has(key);
                        pushContext(indent + 2, `${itemPath}.${key}`, isArrayKey ? 'array' : 'object');
                    }
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
            index.set(currentPath, lineNumber);

            const endsWithColon = trimmed.endsWith(':');
            if (endsWithColon) {
                const isArrayKey = ARRAY_KEYS.has(key);
                pushContext(indent + 2, currentPath, isArrayKey ? 'array' : 'object');
            }
        });

        return index;
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

            return null;
        } catch (error) {
            if (error && error.mark && typeof error.mark.line === 'number') {
                return { errors: [createError(`YAML Parsing Error: ${error.message}`, [`line:${error.mark.line + 1}`])] };
            }
            return { errors: [createError(`YAML Parsing Error: ${error.message}`)] };
        }
    }

    function parsePipelineNameFromYaml(yamlString) {
        if (!yamlString) return '';
        if (window.jsyaml) {
            try {
                const parsed = window.jsyaml.load(yamlString);
                if (parsed && typeof parsed === 'object' && parsed.name) {
                    return String(parsed.name).trim();
                }
            } catch { }
        }
        const match = String(yamlString).match(/^\s*name:\s*([A-Za-z0-9_.-]+)/m);
        return match ? match[1].trim() : '';
    }

    function escapeHtml(value) {
        return String(value ?? '').replace(/[&<>'"`]/g, c => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;', '`': '&#96;'
        })[c]);
    }

    function escapeAttribute(value) {
        return escapeHtml(value).replace(/"/g, '&quot;');
    }

    function updateSuggestionPanel() {
        renderVariableSuggestions();
        ensureVariableSuggestionData().catch(() => { });
    }

    function renderVariableSuggestions() {
        const panel = DOM['lab-suggestion-panel'];
        const list = DOM['lab-suggestion-list'];
        const emptyState = DOM['lab-suggestion-empty'];
        if (!panel || !list || !emptyState) return;

        if (!state.currentYaml) {
            panel.classList.add('hidden');
            return;
        }

        panel.classList.remove('hidden');
        setSuggestionPanelCopy({
            title: 'Variables by scope',
            subtitle: 'Click to insert variables into your pipeline.',
            footnote: 'Values are not shown; use overrides below to inject values.',
        });

        if (state.variableSuggestionPromise && !state.variableSuggestions.length) {
            emptyState.textContent = 'Loading variables…';
            emptyState.classList.remove('hidden');
            list.innerHTML = '';
            return;
        }

        if (!state.variableSuggestions.length) {
            emptyState.textContent = 'No variables available yet.';
            emptyState.classList.remove('hidden');
            list.innerHTML = '';
            return;
        }

        emptyState.classList.add('hidden');
        list.innerHTML = state.variableSuggestions.map(entry => {
            const pills = entry.preview.map(name => {
                const valueAttr = escapeAttribute(name);
                return `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-lab-suggestion="${valueAttr}">${escapeHtml(name)}</button>`;
            });
            const remaining = entry.count - entry.preview.length;
            if (remaining > 0) {
                pills.push(`<span class="env-suggestion-pill env-suggestion-pill--more">+${remaining} more</span>`);
            }
            return `
                <div class="env-suggestion-item">
                    <div class="env-suggestion-meta">
                        <p class="env-suggestion-label">${escapeHtml(entry.label || '')}</p>
                        <p class="env-suggestion-subtext">${entry.count} variable${entry.count === 1 ? '' : 's'}</p>
                    </div>
                    <div class="env-suggestion-variables">${pills.join('')}</div>
                </div>
            `;
        }).join('');
    }

    function setSuggestionPanelCopy(copy = {}) {
        const titleEl = DOM['lab-suggestion-title'];
        const subtitleEl = DOM['lab-suggestion-subtitle'];
        const footnoteEl = DOM['lab-suggestion-footnote'];
        if (titleEl && copy.title) titleEl.textContent = copy.title;
        if (subtitleEl && copy.subtitle) subtitleEl.textContent = copy.subtitle;
        if (footnoteEl && copy.footnote) footnoteEl.textContent = copy.footnote;
    }

    function handleSuggestionClick(event) {
        const target = event.target.closest('[data-lab-suggestion]');
        if (!target) return;
        const value = target.getAttribute('data-lab-suggestion') || '';
        applyEditorSuggestion(value);
    }

    function applyEditorSuggestion(value) {
        const textarea = DOM['lab-yaml-editor'];
        if (!textarea) return;
        const { selectionStart, selectionEnd, value: current } = textarea;
        const insert = String(value || '');
        textarea.value = current.slice(0, selectionStart) + insert + current.slice(selectionEnd);
        const pos = selectionStart + insert.length;
        textarea.selectionStart = textarea.selectionEnd = pos;
        state.currentYaml = textarea.value;
        updateEditorHighlight();
        updateLineNumbers();
        renderSummary();
        renderValidation();
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
                    key: buildVariableScopeKey(label),
                    label: label ? `/${label}` : '/ (default)',
                    count: variables.length,
                    preview: variables.slice(0, 5),
                });
            }
            state.variableSuggestions = summaries;
            state.variableSuggestionLoadedAt = Date.now();
            return summaries;
        })();

        state.variableSuggestionPromise = promise;
        try {
            const result = await promise;
            renderVariableSuggestions();
            return result;
        } catch (error) {
            console.error('Failed to load variable suggestions for lab editor:', error);
            state.variableSuggestions = [];
            renderVariableSuggestions();
            throw error;
        } finally {
            state.variableSuggestionPromise = null;
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
        if (entry == null) {
            return '';
        }
        if (typeof entry === 'string') {
            return String(entry).trim().replace(/^\/+|\/+$/g, '');
        }
        if (typeof entry === 'object') {
            const value = entry.scope ?? entry.env ?? entry.name ?? entry.value ?? '';
            return String(value || '').trim().replace(/^\/+|\/+$/g, '');
        }
        return '';
    }

    function buildVariableScopeKey(label) {
        return `env:${label || ''}`;
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

    global.pages = global.pages || {};
    global.pages.lab = {
        init,
        handleRoute,
    };
})(window.NopsAI = window.NopsAI || {});
