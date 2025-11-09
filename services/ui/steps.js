(function (global) {
    const STEP_DIRECTIVES = [
        { key: 'name', hint: 'Step name' },
        { key: 'include', hint: 'Include reusable step' },
        { key: 'sync', hint: 'Run step synchronously' },
        { key: 'image', hint: 'Override container image' },
        { key: 'secrets', hint: 'Step secrets' },
        { key: 'volumes', hint: 'Mount volumes' },
        { key: 'environment', hint: 'Step environment variables' },
        { key: 'tasks', hint: 'Nested tasks list' },
        { key: 'condition', hint: 'Conditional execution' },
        { key: 'goal', hint: 'LLM goal prompt' },
        { key: 'script', hint: 'Shell script body' },
        { key: 'depends_on', hint: 'Upstream steps' },
        { key: 'ignore_failure', hint: 'Ignore failure' },
        { key: 'llm_output_sharing', hint: 'Share step LLM output' },
    ];

    const TASK_DIRECTIVES = [
        { key: 'name', hint: 'Task name' },
        { key: 'goal', hint: 'LLM goal prompt' },
        { key: 'script', hint: 'Shell script body' },
        { key: 'depends_on', hint: 'Dependent tasks' },
        { key: 'ignore_failure', hint: 'Ignore task errors' },
        { key: 'llm_output_sharing', hint: 'Share task LLM output' },
    ];

    const STEP_ALLOWED_KEYS = new Set([
        ...STEP_DIRECTIVES.map(def => def.key),
        'artifacts',
        'description',
    ]);
    const TASK_ALLOWED_KEYS = new Set(TASK_DIRECTIVES.map(def => def.key));

    const STEP_LIST_KEYS_WITH_NAME_TEMPLATE = new Set(['tasks']);
    const STEP_LIST_KEYS_SIMPLE = new Set(['secrets', 'volumes', 'depends_on', 'environment', 'artifacts']);

    const state = {
        steps: [],
        searchTerm: '',
        selectedId: null,
        cache: new Map(),
        drafts: new Set(),
        activeFolderKey: '',
        sidebarExpanded: new Set(),
        isEditing: false,
        currentYaml: '',
        pendingDelete: null,
        listPromise: null,
        stepSources: new Map(),
        stepMetadata: new Map(),
        cloneContext: null,
        usageCache: new Map(),
        usagePromises: new Map(),
        beforeUnloadHandler: null,
        environmentSuggestions: [],
        environmentSuggestionCache: new Map(),
        environmentSuggestionPromise: null,
        environmentSuggestionLoadedAt: 0,
        secretSuggestions: [],
        secretSuggestionPromise: null,
        secretSuggestionLoadedAt: 0,
        stepSuggestionMode: null,
        stepSuggestionContext: null,
        stepValidationErrors: [],
        stepSuggestionPanelFloating: false,
        stepSuggestionPanelOriginalParent: null,
        stepSuggestionPanelOriginalNextSibling: null,
        stepSuggestionPanelOverlayContainer: null,
        stepSuggestionPanelAnimationFrame: null,
    };

    const DOM = {};
    let context = null;

    const TOAST_TIMEOUT = 4000;
    const STEP_ENV_REFRESH_MS = 2 * 60 * 1000;

    function normalizeSourceValue(raw) {
        if (raw == null) return 'database';
        const value = String(raw).trim().toLowerCase();
        if (!value) return 'database';
        if (value.includes('git')) return 'git';
        if (value.includes('draft')) return 'draft';
        if (value.includes('local')) return 'local';
        if (value.includes('database') || value.includes('db')) return 'database';
        return value;
    }

    function enableStepSuggestionOverlay() {
        if (state.stepSuggestionPanelFloating) return;
        const panel = DOM['step-suggestion-panel'];
        if (!panel) return;
        const parent = panel.parentNode;
        if (!parent) return;
        const baseWidth = panel.offsetWidth || 260;
        const nextSibling = panel.nextSibling;
        state.stepSuggestionPanelOriginalParent = parent;
        state.stepSuggestionPanelOriginalNextSibling = nextSibling;
        parent.removeChild(panel);
        const container = document.getElementById('page-content-wrapper') || document.body;
        if (container && container.classList) {
            container.classList.add('step-suggestion-overlay-host');
        }
        container.appendChild(panel);
        panel.classList.add('pipeline-suggestion-overlay');
        panel.dataset.baseWidth = String(baseWidth);
        panel.style.left = '0px';
        panel.style.top = '0px';
        panel.style.transform = '';
        state.stepSuggestionPanelOverlayContainer = container;
        state.stepSuggestionPanelFloating = true;
        updateStepSuggestionOverlayPosition();
        startStepSuggestionOverlayTracking();
    }

    function disableStepSuggestionOverlay() {
        if (!state.stepSuggestionPanelFloating) return;
        const panel = DOM['step-suggestion-panel'];
        if (!panel) return;
        panel.classList.remove('pipeline-suggestion-overlay');
        const originalParent = state.stepSuggestionPanelOriginalParent;
        const referenceNode = state.stepSuggestionPanelOriginalNextSibling;
        if (originalParent) {
            if (referenceNode && referenceNode.parentNode === originalParent) {
                originalParent.insertBefore(panel, referenceNode);
            } else {
                originalParent.appendChild(panel);
            }
        }
        state.stepSuggestionPanelOriginalParent = null;
        state.stepSuggestionPanelOriginalNextSibling = null;
        if (state.stepSuggestionPanelOverlayContainer && state.stepSuggestionPanelOverlayContainer.classList) {
            state.stepSuggestionPanelOverlayContainer.classList.remove('step-suggestion-overlay-host');
        }
        state.stepSuggestionPanelOverlayContainer = null;
        state.stepSuggestionPanelFloating = false;
        panel.style.transform = '';
        panel.style.left = '';
        panel.style.top = '';
        panel.style.width = '';
        panel.style.maxHeight = '';
        stopStepSuggestionOverlayTracking();
    }

    function updateStepSuggestionOverlayPosition() {
        if (!state.stepSuggestionPanelFloating) return;
        const panel = DOM['step-suggestion-panel'];
        const textarea = DOM['step-yaml-editor'];
        const container = state.stepSuggestionPanelOverlayContainer || document.getElementById('page-content-wrapper') || document.body;
        if (!panel || panel.classList.contains('hidden') || !textarea || !container) {
            return;
        }
        const textareaRect = textarea.getBoundingClientRect();
        if (!textareaRect) return;
        const containerRect = container.getBoundingClientRect();
        const textareaRight = textareaRect.right - containerRect.left + container.scrollLeft;
        const padding = 24;
        const baseWidth = panel.dataset.baseWidth ? parseFloat(panel.dataset.baseWidth) : panel.offsetWidth || 260;
        const containerWidth = container.clientWidth || (window.innerWidth ?? baseWidth + padding * 2);
        const targetLeft = textareaRight + padding;
        const maxLeft = container.scrollLeft + containerWidth - baseWidth - padding;
        const minLeft = container.scrollLeft + padding;
        const finalLeft = Math.max(minLeft, Math.min(targetLeft, maxLeft));

        const panelHeight = panel.offsetHeight || 0;
        const viewportTop = container.scrollTop + padding;
        const viewportBottom = container.scrollTop + (container.clientHeight || window.innerHeight || (panelHeight + padding * 2)) - padding;
        const actions = DOM['step-edit-actions'];
        let anchorTop = textareaRect.top - containerRect.top + container.scrollTop;
        if (actions && !actions.classList.contains('hidden')) {
            const actionsRect = actions.getBoundingClientRect();
            anchorTop = Math.max(anchorTop, actionsRect.bottom - containerRect.top + container.scrollTop + 12);
        }
        const minTop = Math.max(viewportTop, anchorTop);
        let finalTop = anchorTop;
        if (finalTop + panelHeight > viewportBottom) {
            finalTop = Math.max(minTop, viewportBottom - panelHeight);
        }

        panel.style.transform = '';
        panel.style.left = `${finalLeft}px`;
        panel.style.top = `${finalTop}px`;
        panel.style.width = `${baseWidth}px`;
        panel.style.maxHeight = `${Math.max(240, viewportBottom - viewportTop)}px`;
    }

    function startStepSuggestionOverlayTracking() {
        if (state.stepSuggestionPanelAnimationFrame != null) return;
        const tick = () => {
            state.stepSuggestionPanelAnimationFrame = window.requestAnimationFrame(() => {
                updateStepSuggestionOverlayPosition();
                tick();
            });
        };
        tick();
    }

    function stopStepSuggestionOverlayTracking() {
        if (state.stepSuggestionPanelAnimationFrame == null) return;
        window.cancelAnimationFrame(state.stepSuggestionPanelAnimationFrame);
        state.stepSuggestionPanelAnimationFrame = null;
    }

    function formatSourceLabel(sourceKey) {
        switch (normalizeSourceValue(sourceKey)) {
            case 'git':
                return 'Git';
            case 'draft':
                return 'Draft';
            case 'local':
                return 'Local';
            default:
                return 'Database';
        }
    }

    function resolveStepSource(identifier) {
        if (!identifier) return 'database';
        if (state.drafts.has(identifier)) {
            return 'draft';
        }
        if (!(state.stepSources instanceof Map)) {
            state.stepSources = new Map();
        }
        return normalizeSourceValue(state.stepSources.get(identifier) || 'database');
    }

    function updateStepMetadata(identifier, updates = {}) {
        if (!identifier) return {};
        if (!(state.stepMetadata instanceof Map)) {
            state.stepMetadata = new Map();
        }
        const existing = state.stepMetadata.get(identifier) || {};
        const merged = { ...existing, ...updates };
        state.stepMetadata.set(identifier, merged);
        const cacheEntry = state.cache.get(identifier);
        if (cacheEntry) {
            cacheEntry.meta = { ...(cacheEntry.meta || {}), ...merged };
            state.cache.set(identifier, cacheEntry);
        }
        return merged;
    }

    function getStepMetadata(identifier) {
        if (!identifier) return null;
        const cacheEntry = state.cache.get(identifier);
        if (cacheEntry && cacheEntry.meta) {
            return cacheEntry.meta;
        }
        if (!(state.stepMetadata instanceof Map)) {
            state.stepMetadata = new Map();
        }
        return state.stepMetadata.get(identifier) || null;
    }

    function formatUpdatedLabel(value) {
        if (!value) return '—';
        const date = new Date(value);
        if (Number.isNaN(date.getTime())) return '—';
        return date.toLocaleString();
    }

    function buildIdentifierFromParts(path, name) {
        const trimmedName = (name || '').trim();
        if (!trimmedName) return '';
        const trimmedPath = (path || '').trim();
        return trimmedPath ? `${trimmedPath}/${trimmedName}` : trimmedName;
    }

    function init(ctx) {
        context = ctx;
        const ids = [
            'steps-search', 'steps-list-view', 'steps-detail-view', 'steps-list-container', 'steps-empty',
            'steps-new-btn', 'steps-back-btn', 'steps-new-modal', 'steps-new-form',
            'steps-new-close', 'steps-new-cancel', 'steps-new-path', 'steps-new-name', 'steps-delete-modal',
            'steps-delete-message', 'steps-delete-confirm', 'steps-delete-cancel', 'steps-delete-close',
            'steps-clone-modal', 'steps-clone-form', 'steps-clone-close', 'steps-clone-cancel',
            'steps-clone-path', 'steps-clone-name', 'steps-clone-subtitle',
            'step-detail-name', 'step-detail-description', 'step-detail-identifier', 'step-detail-path',
            'step-detail-source', 'step-detail-updated', 'step-view-actions', 'step-edit-actions', 'step-copy-btn', 'step-download-btn',
            'step-edit-btn', 'step-clone-btn', 'step-save-btn', 'step-cancel-btn', 'step-yaml-content',
            'step-editor-wrapper', 'step-editor-container', 'step-line-numbers', 'step-yaml-stage', 'step-yaml-highlight', 'step-yaml-editor',
            'step-validation-status', 'step-usage-content', 'step-usage-empty',
            'step-suggestion-panel', 'step-suggestion-list', 'step-suggestion-empty',
            'step-suggestion-title', 'step-suggestion-subtitle', 'step-suggestion-footnote'
        ];

        ids.forEach(id => {
            const el = document.getElementById(id);
            if (el) DOM[id] = el;
        });

        if (DOM['steps-search']) {
            DOM['steps-search'].addEventListener('input', handleSearch);
        }

        if (DOM['steps-list-container']) {
            DOM['steps-list-container'].addEventListener('click', handleListClick);
            DOM['steps-list-container'].addEventListener('keydown', handleListKeydown);
        }

        if (DOM['steps-new-btn']) {
            DOM['steps-new-btn'].addEventListener('click', () => {
                if (state.isEditing) {
                    notifyEditingLock();
                    return;
                }
                openNewStepModal();
            });
        }

        if (DOM['steps-back-btn']) {
            DOM['steps-back-btn'].addEventListener('click', () => {
                if (state.isEditing) {
                    notifyEditingLock();
                    return;
                }
                const folderHash = buildFolderHash(state.activeFolderKey);
                window.location.hash = folderHash;
            });
        }

        if (DOM['steps-new-close']) {
            DOM['steps-new-close'].addEventListener('click', closeNewStepModal);
        }
        if (DOM['steps-new-cancel']) {
            DOM['steps-new-cancel'].addEventListener('click', closeNewStepModal);
        }
        if (DOM['steps-new-form']) {
            DOM['steps-new-form'].addEventListener('submit', handleCreateStep);
        }

        if (DOM['steps-delete-close']) {
            DOM['steps-delete-close'].addEventListener('click', closeDeleteModal);
        }
        if (DOM['steps-delete-cancel']) {
            DOM['steps-delete-cancel'].addEventListener('click', closeDeleteModal);
        }
        if (DOM['steps-delete-confirm']) {
            DOM['steps-delete-confirm'].addEventListener('click', confirmDeleteStep);
        }

        if (DOM['step-edit-btn']) {
            DOM['step-edit-btn'].addEventListener('click', enterEditMode);
        }
        if (DOM['step-cancel-btn']) {
            DOM['step-cancel-btn'].addEventListener('click', cancelEditing);
        }
        if (DOM['step-save-btn']) {
            DOM['step-save-btn'].addEventListener('click', saveStepChanges);
        }
        if (DOM['step-copy-btn']) {
            DOM['step-copy-btn'].addEventListener('click', copyStepYaml);
        }
        if (DOM['step-download-btn']) {
            DOM['step-download-btn'].addEventListener('click', downloadStepYaml);
        }
        if (DOM['step-clone-btn']) {
            DOM['step-clone-btn'].addEventListener('click', () => {
                if (!state.selectedId) return;
                if (state.isEditing) {
                    notifyEditingLock();
                    return;
                }
                openCloneStepModal(state.selectedId).catch(() => {});
            });
        }

        if (DOM['step-suggestion-panel']) {
            DOM['step-suggestion-panel'].addEventListener('click', handleStepSuggestionClick);
        }

        if (DOM['step-yaml-editor']) {
            const editor = DOM['step-yaml-editor'];
            const handleCursorChange = () => updateStepEditorSuggestions();
            editor.addEventListener('input', () => {
                updateLineNumbers();
                updateValidationStatus();
                updateStepEditorSuggestions();
                updateStepEditorHighlight();
            });
            editor.addEventListener('scroll', syncLineNumbersScroll);
            editor.addEventListener('keydown', handleEditorKeydown);
            editor.addEventListener('click', handleCursorChange);
            editor.addEventListener('keyup', handleCursorChange);
            editor.addEventListener('select', handleCursorChange);
        }

        if (DOM['steps-clone-close']) {
            DOM['steps-clone-close'].addEventListener('click', closeCloneStepModal);
        }
        if (DOM['steps-clone-cancel']) {
            DOM['steps-clone-cancel'].addEventListener('click', closeCloneStepModal);
        }
        if (DOM['steps-clone-form']) {
            DOM['steps-clone-form'].addEventListener('submit', handleCloneStep);
        }

        exitEditMode({ resetEditor: true, suppressHash: true });
        renderStepsList();
    }

    async function handleRoute(hash) {
        const routeInfo = parseStepsHash(hash);
        await refreshSteps(false);

        if (!state.steps.length) {
            state.selectedId = null;
            showListView();
            renderStepsList();
            renderSidebarTree();
            return;
        }

        const requestedIdentifier = routeInfo.segments.join('/') || '';
        if (state.isEditing) {
            const sameStep = requestedIdentifier && requestedIdentifier === state.selectedId;
            const stayingInEdit = sameStep && routeInfo.mode === 'edit';
            if (!stayingInEdit) {
                if (preventEditingNavigation(hash)) {
                    return;
                }
            }
        }

        let targetId = null;
        let targetFolder = '';

        if (routeInfo.segments.length) {
            const candidate = routeInfo.segments.join('/');
            if (state.steps.includes(candidate)) {
                targetId = candidate;
                targetFolder = getFolderPath(candidate);
            } else if (state.steps.includes(decodeURIComponentPath(candidate))) {
                targetId = decodeURIComponentPath(candidate);
                targetFolder = getFolderPath(targetId);
            } else {
                targetFolder = candidate;
            }
        }

        if (targetFolder && !state.steps.some(step => step === targetFolder || step.startsWith(`${targetFolder}/`))) {
            targetFolder = '';
        }

        if (targetId) {
            state.selectedId = targetId;
            state.activeFolderKey = targetFolder;
            ensureSidebarExpansionForPath(state.activeFolderKey);
            ensureSidebarExpansionForPath(getFolderPath(targetId));
            renderStepsList();
            renderSidebarTree();
            await renderStepDetail(targetId);
            if (routeInfo.mode === 'edit') {
                enterEditMode();
            } else if (state.isEditing) {
                exitEditMode();
            } else {
                exitEditMode({ resetEditor: true, suppressHash: true });
            }
        } else {
            state.selectedId = null;
            state.activeFolderKey = routeInfo.mode === 'edit' ? '' : (targetFolder || '');
            showListView();
            renderStepsList();
            renderSidebarTree();
            exitEditMode({ suppressHash: true });
        }
    }

    function parseStepsHash(hash) {
        const cleaned = (hash || '').replace(/^#\/?/, '');
        const parts = cleaned.split('/').filter(Boolean);
        if (!parts.length || parts[0] !== 'steps') {
            return { segments: [], mode: null };
        }
        const rest = parts.slice(1).map(segment => {
            try {
                return decodeURIComponent(segment);
            } catch {
                return segment;
            }
        });
        let mode = null;
        if (rest.length && rest[rest.length - 1] === 'edit') {
            mode = 'edit';
            rest.pop();
        }
        return { segments: rest, mode };
    }

    async function refreshSteps(force = false) {
        if (!context || typeof context.fetchData !== 'function') {
            return;
        }

        if (state.listPromise) {
            await state.listPromise;
        }

        if (!force && state.steps.length) {
            return;
        }

        state.listPromise = context.fetchData('/v1/steps?include_source=true');

        let response = null;
        try {
            response = await state.listPromise;
        } finally {
            state.listPromise = null;
        }

        ingestStepListResponse(response);

        if (state.selectedId && !state.steps.includes(state.selectedId)) {
            state.selectedId = null;
        }

        if (state.activeFolderKey && !state.steps.some(step => step.startsWith(`${state.activeFolderKey}/`) || step === state.activeFolderKey)) {
            state.activeFolderKey = '';
        }

        renderStepsList();
        renderSidebarTree();
    }

    function ingestStepListResponse(response) {
        if (!(response instanceof Array)) {
            state.steps = [];
            state.stepSources = new Map();
            state.stepMetadata = new Map();
            return;
        }

        if (!response.length) {
            state.steps = [];
            state.stepSources = new Map();
            state.stepMetadata = new Map();
            return;
        }

        if (typeof response[0] === 'object' && response[0] !== null) {
            state.stepSources = new Map();
            state.stepMetadata = new Map();
            const identifiers = [];
            response.forEach(item => {
                const identifier = item.identifier || buildIdentifierFromParts(item.path, item.name);
                if (!identifier) return;
                identifiers.push(identifier);
                state.stepSources.set(identifier, normalizeSourceValue(item.source));
                const updatedAt = item.updated_at ? String(item.updated_at) : '';
                updateStepMetadata(identifier, {
                    name: item.name,
                    path: item.path || '',
                    updatedAt,
                    updatedLabel: updatedAt ? formatUpdatedLabel(updatedAt) : '—',
                });
            });
            identifiers.sort((a, b) => a.localeCompare(b));
            state.steps = identifiers;
            return;
        }

        const normalized = response
            .map(item => typeof item === 'string' ? item.trim() : '')
            .filter(Boolean);
        normalized.sort((a, b) => a.localeCompare(b));
        state.steps = normalized;
        state.stepSources = new Map();
        state.stepMetadata = new Map();
        normalized.forEach(identifier => {
            updateStepMetadata(identifier, {
                name: getStepName(identifier),
                path: getFolderPath(identifier),
                updatedLabel: state.drafts.has(identifier) ? 'Draft' : '—',
            });
        });
    }

    function renderStepsList() {
        if (!DOM['steps-list-container']) return;

        const search = state.searchTerm.trim().toLowerCase();
        const tree = buildStepsTree(search);
        const hasContent = treeHasContent(tree);
        const isSearching = !!search;

        if (!hasContent) {
            if (DOM['steps-empty']) DOM['steps-empty'].classList.remove('hidden');
            DOM['steps-list-container'].innerHTML = '';
            return;
        }

        if (DOM['steps-empty']) DOM['steps-empty'].classList.add('hidden');

        let activeNode = isSearching ? tree : resolveFolderNode(tree, state.activeFolderKey);
        if (!activeNode) activeNode = tree;

        const childNodes = activeNode.children ? Array.from(activeNode.children.values()) : [];
        childNodes.sort((a, b) => a.label.localeCompare(b.label, undefined, { sensitivity: 'base' }));
        const folderCards = childNodes.map(renderFolderCard);

        const stepEntries = activeNode.steps ? activeNode.steps.slice() : [];
        stepEntries.sort((a, b) => a.displayName.localeCompare(b.displayName, undefined, { sensitivity: 'base' }));
        const stepCards = stepEntries.map(renderStepCard);

        const foldersHtml = folderCards.length
            ? `<div class="pipelines-card-grid pipelines-card-grid--folders">${folderCards.join('')}</div>`
            : '';
        const stepsHtml = stepCards.length
            ? `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${stepCards.join('')}</div>`
            : '';

        const combined = `${stepsHtml}${foldersHtml}` || `<div class="pipeline-folder-empty-state">No steps to display.</div>`;
        DOM['steps-list-container'].innerHTML = combined;
    }

    function renderSidebarTree(container) {
        const target = container || document.getElementById('steps-sidebar-tree');
        if (!target) return;

        const tree = buildStepsTree('');
        ensureSidebarExpansionForPath(state.activeFolderKey);
        ensureSidebarExpansionForPath(getFolderPath(state.selectedId));

        const html = renderSidebarNode(tree, 0, state.activeFolderKey, state.selectedId);
        target.innerHTML = html || `<p class="px-2 text-sm text-[var(--text-secondary)]">No steps available.</p>`;

        if (!target.dataset.stepsSidebarBound) {
            target.addEventListener('click', handleSidebarTreeClick);
            target.dataset.stepsSidebarBound = 'true';
        }
    }

    function buildStepsTree(searchTerm) {
        const root = createTreeNode('', '');
        const lowerSearch = (searchTerm || '').trim().toLowerCase();

        state.steps.forEach(identifier => {
            const cacheEntry = state.cache.get(identifier);
            const fallbackMeta = (state.stepMetadata instanceof Map) ? state.stepMetadata.get(identifier) : null;
            const meta = cacheEntry && cacheEntry.meta ? cacheEntry.meta : fallbackMeta;
            const haystackParts = [identifier];
            if (meta) {
                if (meta.name) haystackParts.push(meta.name);
                if (meta.description) haystackParts.push(meta.description);
                if (meta.goal) haystackParts.push(meta.goal);
            }

            const matchesSearch = !lowerSearch || haystackParts.some(value => (value || '').toLowerCase().includes(lowerSearch));
            if (!matchesSearch) return;

            const segments = identifier.split('/').filter(Boolean);
            const name = segments.pop() || identifier;
            let node = root;
            let currentKey = '';
            segments.forEach(segment => {
                currentKey = currentKey ? `${currentKey}/${segment}` : segment;
                if (!node.children.has(segment)) {
                    node.children.set(segment, createTreeNode(currentKey, segment));
                }
                node = node.children.get(segment);
            });
            const entry = {
                id: identifier,
                displayName: (meta && meta.name) || name,
                meta: meta || null,
            };
            node.steps.push(entry);
        });

        return root;
    }

    function createTreeNode(key, label) {
        return {
            key: key || '',
            label: label || '',
            children: new Map(),
            steps: [],
        };
    }

    function treeHasContent(node) {
        if (!node) return false;
        if (node.steps.length) return true;
        if (!node.children) return false;
        for (const child of node.children.values()) {
            if (treeHasContent(child)) return true;
        }
        return false;
    }

    function resolveFolderNode(root, folderKey) {
        if (!folderKey) return root;
        const segments = folderKey.split('/').filter(Boolean);
        let node = root;
        for (const segment of segments) {
            if (!node.children || !node.children.has(segment)) {
                return root;
            }
            node = node.children.get(segment);
        }
        return node;
    }

    function renderFolderCard(node) {
        const keyAttr = escapeAttribute(node.key || '');
        const label = formatPathLabel(node.label || node.key || 'Folder');
        const totalSteps = countSteps(node);
        const subFolders = node.children ? node.children.size : 0;

        return `
            <article class="pipeline-folder-card border border-[var(--border-primary)]" data-folder-key="${keyAttr}" tabindex="0" role="button" aria-label="Open folder ${escapeHtml(label)}">
                <div class="pipeline-folder-card-header">
                    <span class="pipeline-folder-icon">
                        <svg class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <path d="M3 7h5l2 2h9a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V9a2 2 0 012-2z" />
                        </svg>
                    </span>
                    <h3 class="pipeline-folder-title">${escapeHtml(label)}</h3>
                    <div class="pipeline-folder-actions">
                        <span class="pipeline-folder-chevron" aria-hidden="true">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M9 5l7 7-7 7" />
                            </svg>
                        </span>
                    </div>
                </div>
                <div class="pipeline-folder-meta">
                    <div class="pipeline-folder-meta-row">
                        <span class="pipeline-folder-meta-label">Steps:</span>
                        <span class="pipeline-folder-meta-value">${totalSteps}</span>
                    </div>
                    <div class="pipeline-folder-meta-row">
                        <span class="pipeline-folder-meta-label">Sub folders:</span>
                        <span class="pipeline-folder-meta-value">${subFolders}</span>
                    </div>
                </div>
            </article>`;
    }

    function countSteps(node) {
        if (!node) return 0;
        let total = node.steps.length;
        if (node.children) {
            for (const child of node.children.values()) {
                total += countSteps(child);
            }
        }
        return total;
    }

    function renderStepCard(entry) {
        const idAttr = escapeAttribute(entry.id);
        const folderPath = formatPathLabel(getFolderPath(entry.id) || 'root');
        const meta = entry.meta || {};
        const description = meta.description || meta.goal || 'Reusable step definition.';
        const sourceKey = resolveStepSource(entry.id);
        const sourceLabel = formatSourceLabel(sourceKey);
        const isGitManaged = sourceKey === 'git';
        const isActive = state.selectedId === entry.id;

        const deleteButtonHtml = isGitManaged
            ? `<button class="pipelines-delete-button" type="button" title="Managed via Git" disabled aria-disabled="true">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                                <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                                <path d="M4 7h16" />
                            </svg>
                        </button>`
            : `<button class="pipelines-delete-button" type="button" data-delete-step="${idAttr}" title="Delete step">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                                <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                                <path d="M4 7h16" />
                            </svg>
                        </button>`;

        return `
            <article class="pipeline-card bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded-lg shadow-sm transition-all duration-200 p-3 flex flex-col ${isActive ? 'ring-2 ring-indigo-500 border-indigo-400' : ''}" data-step-id="${idAttr}" tabindex="0" role="button" aria-label="Open step ${escapeHtml(entry.displayName)}">
                <div class="pipeline-card-header flex items-start justify-between gap-3">
                    <div class="pipeline-card-text min-w-0">
                        <h3 class="pipeline-card-title">${escapeHtml(entry.displayName)}</h3>
                        <p class="pipeline-card-path">${escapeHtml(folderPath)}</p>
                    </div>
                    <div class="pipeline-card-actions">
                        ${deleteButtonHtml}
                    </div>
                </div>
                <p class="pipeline-card-description">${escapeHtml(description)}</p>
                <div class="pipeline-card-meta">
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">Source</span>
                        <span class="pipeline-card-meta-value">${escapeHtml(sourceLabel)}</span>
                    </div>
                </div>
            </article>`;
    }

    function renderSidebarNode(node, level, activeFolder, activeStep) {
        const childEntries = node.children ? Array.from(node.children.entries()) : [];
        const stepEntries = node.steps || [];
        if (!childEntries.length && !stepEntries.length && level !== 0) {
            return '';
        }

        childEntries.sort((a, b) => a[0].localeCompare(b[0], undefined, { sensitivity: 'base' }));
        stepEntries.sort((a, b) => a.displayName.localeCompare(b.displayName, undefined, { sensitivity: 'base' }));

        let html = `<ul class="${level > 0 ? 'pl-4' : ''} space-y-1">`;

        childEntries.forEach(([segment, child]) => {
            const folderPath = child.key || segment;
            const isExpanded = state.sidebarExpanded.has(folderPath) || !child.children.size;
            const isActiveFolder = activeFolder === folderPath;
            const childrenHtml = renderSidebarNode(child, level + 1, activeFolder, activeStep);
            html += `
                <li>
                    <div class="flex items-center justify-between p-1 text-[var(--text-primary)] rounded-md ${isActiveFolder ? 'bg-[var(--bg-tertiary)]' : ''} hover:bg-[var(--bg-tertiary)]">
                        <div class="flex items-center flex-grow min-w-0">
                            <button type="button" class="sidebar-toggle-btn flex items-center justify-center h-5 w-5 rounded mr-1 text-[var(--text-secondary)] hover:bg-[var(--bg-hover)]" data-step-toggle-folder="${escapeAttribute(folderPath)}" aria-expanded="${isExpanded ? 'true' : 'false'}" aria-label="${escapeAttribute((isExpanded ? 'Collapse' : 'Expand') + ' ' + segment)}">
                                <svg class="h-4 w-4 chevron ${isExpanded ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>
                            </button>
                            <button type="button" class="pipeline-sidebar-folder flex items-center gap-2 flex-grow text-left min-w-0 p-1 rounded hover:bg-[var(--bg-hover)]" data-step-open-folder="${escapeAttribute(folderPath)}">
                                <svg class="h-4 w-4 text-[var(--text-secondary)] flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>
                                <span class="truncate">${escapeHtml(segment)}</span>
                            </button>
                        </div>
                    </div>
                    <div class="${isExpanded ? '' : 'hidden'}" data-step-folder-children="${escapeAttribute(folderPath)}">
                        ${childrenHtml}
                    </div>
                </li>`;
        });

        stepEntries.forEach(entry => {
            const isActive = activeStep === entry.id;
            html += `
                <li>
                    <a href="${buildStepHash(entry.id)}" class="sidebar-link flex items-center p-2 text-[var(--text-primary)] rounded-md transition-colors duration-200 ${isActive ? 'active' : ''}" data-step-link="${escapeAttribute(entry.id)}">
                        <svg class="h-4 w-4 mr-2 text-[var(--text-secondary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h4l2 2h6a2 2 0 012 2v10a2 2 0 01-2 2H6a2 2 0 01-2-2V6zm3 4h10m-10 4h6"/></svg>
                        <span class="truncate">${escapeHtml(entry.displayName)}</span>
                    </a>
                </li>`;
        });

        html += '</ul>';
        return html;
    }

    async function renderStepDetail(identifier) {
        if (!identifier) return;

        let cacheEntry = state.cache.get(identifier);
        if (!cacheEntry || (!cacheEntry.yaml && !state.drafts.has(identifier))) {
            const url = `/v1/steps/${encodeIdentifier(identifier)}`;
            const yaml = await context.fetchData(url);
            if (yaml == null) {
                showToast('Unable to load step definition.', 'error');
                return;
            }
            cacheEntry = {
                yaml: typeof yaml === 'string' ? yaml : '',
                fetchedAt: Date.now(),
                meta: extractMetaFromYaml(yaml, identifier),
                isDraft: false,
            };
            state.cache.set(identifier, cacheEntry);
        }

        if (!cacheEntry) return;

        const mergedMeta = updateStepMetadata(identifier, cacheEntry.meta || extractMetaFromYaml(cacheEntry.yaml, identifier));
        cacheEntry.meta = mergedMeta;
        const meta = mergedMeta || {};
        const sourceKey = resolveStepSource(identifier);
        const sourceLabel = formatSourceLabel(sourceKey);
        const updatedLabel = cacheEntry.isDraft ? 'Draft' : (meta.updatedLabel || formatUpdatedLabel(meta.updatedAt));

        if (DOM['step-detail-name']) {
            DOM['step-detail-name'].textContent = meta.name || getStepName(identifier);
        }
        if (DOM['step-detail-description']) {
            DOM['step-detail-description'].textContent = meta.description || 'No description provided.';
        }
        if (DOM['step-detail-identifier']) {
            DOM['step-detail-identifier'].textContent = identifier;
        }
        if (DOM['step-detail-path']) {
            DOM['step-detail-path'].textContent = formatPathLabel(meta.path || getFolderPath(identifier));
        }
        if (DOM['step-detail-source']) {
            DOM['step-detail-source'].textContent = sourceLabel;
        }
        if (DOM['step-detail-updated']) {
            DOM['step-detail-updated'].textContent = updatedLabel;
        }

        state.currentYaml = cacheEntry.yaml || '';
        renderYamlView(state.currentYaml);
        const isGitSource = sourceKey === 'git';
        if (DOM['step-edit-btn']) {
            DOM['step-edit-btn'].classList.toggle('hidden', isGitSource);
        }
        if (DOM['step-clone-btn']) {
            DOM['step-clone-btn'].classList.toggle('hidden', !isGitSource);
            DOM['step-clone-btn'].dataset.stepId = identifier;
        }
        exitEditMode({ resetEditor: true, suppressHash: true });
        updateStepSuggestionPanelVisibility();
        showDetailView();
        renderStepUsage(identifier).catch(() => {});
    }

    function showListView() {
        if (DOM['steps-list-view']) DOM['steps-list-view'].classList.remove('hidden');
        if (DOM['steps-detail-view']) DOM['steps-detail-view'].classList.add('hidden');
        if (DOM['steps-back-btn']) DOM['steps-back-btn'].classList.add('hidden');
    }

    function showDetailView() {
        if (DOM['steps-list-view']) DOM['steps-list-view'].classList.add('hidden');
        if (DOM['steps-detail-view']) DOM['steps-detail-view'].classList.remove('hidden');
        if (DOM['steps-back-btn']) DOM['steps-back-btn'].classList.toggle('hidden', window.innerWidth >= 640);
    }

    function enterEditMode() {
        if (!state.selectedId || !DOM['step-yaml-editor']) return;
        if (resolveStepSource(state.selectedId) === 'git') {
            showToast('This step is managed via Git. Clone it to edit locally.', 'info');
            restoreViewRoute();
            return;
        }
        if (state.isEditing) return;
        state.isEditing = true;

        if (DOM['step-view-actions']) DOM['step-view-actions'].classList.add('hidden');
        if (DOM['step-edit-actions']) DOM['step-edit-actions'].classList.remove('hidden');
        if (DOM['step-editor-wrapper']) DOM['step-editor-wrapper'].classList.remove('hidden');
        if (DOM['step-editor-container']) DOM['step-editor-container'].classList.remove('hidden');
        if (DOM['step-yaml-content']) DOM['step-yaml-content'].classList.add('hidden');
        if (DOM['step-validation-status']) DOM['step-validation-status'].classList.remove('hidden');

        DOM['step-yaml-editor'].value = state.currentYaml;
        updateLineNumbers();
        updateValidationStatus();
        updateStepEditorHighlight();
        DOM['step-yaml-editor'].focus();
        DOM['step-yaml-editor'].setSelectionRange(state.currentYaml.length, state.currentYaml.length);
        updateStepEditorSuggestions();

        const editHash = buildStepHash(state.selectedId, { edit: true });
        if (window.location.hash !== editHash) {
            try {
                history.replaceState(null, '', editHash);
            } catch {
                window.location.hash = editHash;
            }
        }
        bindBeforeUnload();
        updateStepSuggestionPanelVisibility(true);
        ensureStepEnvironmentSuggestions().catch(err => console.error('Failed to load environment suggestions:', err));
        ensureStepSecretSuggestions().catch(err => console.error('Failed to load secrets suggestions:', err));
    }

    function exitEditMode(options = {}) {
        const { resetEditor = false, suppressHash = false } = options;
        const wasEditing = state.isEditing;
        state.isEditing = false;
        if (DOM['step-view-actions']) DOM['step-view-actions'].classList.remove('hidden');
        if (DOM['step-edit-actions']) DOM['step-edit-actions'].classList.add('hidden');
        if (DOM['step-editor-wrapper']) DOM['step-editor-wrapper'].classList.add('hidden');
        if (DOM['step-yaml-content']) DOM['step-yaml-content'].classList.remove('hidden');
        if (DOM['step-validation-status']) {
            DOM['step-validation-status'].classList.add('hidden');
            DOM['step-validation-status'].textContent = '';
        }
        if (resetEditor && DOM['step-yaml-editor']) {
            DOM['step-yaml-editor'].value = state.currentYaml;
            updateLineNumbers();
            updateStepEditorHighlight();
        }

        if (wasEditing && !suppressHash && state.selectedId) {
            const viewHash = buildStepHash(state.selectedId);
            if (window.location.hash !== viewHash) {
                try {
                    history.replaceState(null, '', viewHash);
                } catch {
                    window.location.hash = viewHash;
                }
            }
        }

        if (wasEditing) {
            unbindBeforeUnload();
        }
        state.stepSuggestionMode = null;
        state.stepSuggestionContext = null;
        state.stepValidationErrors = [];
        updateStepSuggestionPanelVisibility();
    }

    function bindBeforeUnload() {
        if (state.beforeUnloadHandler) return;
        const handler = (event) => {
            if (!state.isEditing) return;
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

    function notifyEditingLock() {
        showToast('Save or discard your changes before leaving edit mode.', 'error');
    }

    function restoreViewRoute() {
        if (!state.selectedId) return;
        const viewHash = buildStepHash(state.selectedId);
        if (window.location.hash === viewHash) return;
        try {
            history.replaceState(null, '', viewHash);
        } catch {
            window.location.hash = viewHash;
        }
    }

    function restoreEditingRoute(targetHash) {
        const desiredHash = state.selectedId ? buildStepHash(state.selectedId, { edit: true }) : '#/steps';
        const desiredNormalized = normalizeHashForCompare(desiredHash);
        const currentNormalized = normalizeHashForCompare(typeof targetHash === 'string' ? targetHash : window.location.hash);
        if (desiredNormalized === currentNormalized) return;
        suppressNextRouteOnce();
        try {
            const url = new URL(window.location.href);
            url.hash = desiredHash.slice(1);
            history.replaceState(null, '', url.toString());
        } catch {
            window.location.hash = desiredHash;
        }
    }

    function preventEditingNavigation(targetHash) {
        if (!state.isEditing) return false;
        notifyEditingLock();
        restoreEditingRoute(targetHash);
        return true;
    }

    function suppressNextRouteOnce() {
        if (!context || !context.state) return;
        const appState = context.state;
        appState._suppressNextRoute = true;
        if (appState._suppressRouteTimeout) {
            try {
                clearTimeout(appState._suppressRouteTimeout);
            } catch {}
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

    function normalizeHashForCompare(rawHash) {
        if (!rawHash) return '';
        const trimmed = rawHash.replace(/^#/, '');
        return trimmed.replace(/\/+$/, '');
    }

    function updateLineNumbers() {
        if (!DOM['step-yaml-editor']) return;
        if (DOM['step-line-numbers']) {
            const value = DOM['step-yaml-editor'].value || '';
            const lines = value.split('\n');
            const errorMap = new Map();
            (state.stepValidationErrors || []).forEach(err => {
                if (!err || typeof err.line !== 'number') return;
                const lineNumber = normalizeLineNumber(err.line);
                if (!lineNumber) return;
                if (!errorMap.has(lineNumber)) {
                    errorMap.set(lineNumber, []);
                }
                if (err.message) {
                    errorMap.get(lineNumber).push(err.message);
                }
            });
            DOM['step-line-numbers'].innerHTML = lines.map((_, idx) => {
                const lineNumber = idx + 1;
                const messages = errorMap.get(lineNumber);
                const classes = ['line-number'];
                if (messages && messages.length) {
                    classes.push('line-number--error');
                }
                const titleAttr = messages && messages.length
                    ? ` title="${escapeAttribute(messages.join('\n'))}"`
                    : '';
                return `<div class="${classes.join(' ')}" data-line-number="${lineNumber}"${titleAttr}>${lineNumber}</div>`;
            }).join('');
            DOM['step-line-numbers'].scrollTop = DOM['step-yaml-editor'].scrollTop;
        }
    }

    function syncLineNumbersScroll() {
        if (DOM['step-line-numbers'] && DOM['step-yaml-editor']) {
            DOM['step-line-numbers'].scrollTop = DOM['step-yaml-editor'].scrollTop;
        }
        syncStepHighlightScroll();
    }

    function updateStepEditorHighlight() {
        if (!DOM['step-yaml-highlight'] || !DOM['step-yaml-editor']) return;
        const renderer = global.yaml && typeof global.yaml.renderTokens === 'function'
            ? global.yaml.renderTokens
            : null;
        const stage = DOM['step-yaml-stage'];
        if (!renderer) {
            if (stage) stage.classList.remove('yaml-editor-stage--with-highlight');
            DOM['step-yaml-highlight'].textContent = DOM['step-yaml-editor'].value || '';
            return;
        }
        const html = renderer(DOM['step-yaml-editor'].value || '', escapeHtml);
        DOM['step-yaml-highlight'].innerHTML = html || '&nbsp;';
        if (stage) stage.classList.add('yaml-editor-stage--with-highlight');
        syncStepHighlightScroll();
    }

    function syncStepHighlightScroll() {
        if (!DOM['step-yaml-highlight'] || !DOM['step-yaml-editor']) return;
        const editor = DOM['step-yaml-editor'];
        const x = editor.scrollLeft || 0;
        const y = editor.scrollTop || 0;
        DOM['step-yaml-highlight'].style.transform = `translate(${-x}px, ${-y}px)`;
    }

    function handleEditorKeydown(event) {
        if (!DOM['step-yaml-editor']) return;
        if (event.key === 'Enter') {
            handleStepEditorEnterKey(event);
            return;
        }
        if (event.key === 'Tab') {
            event.preventDefault();
            const textarea = DOM['step-yaml-editor'];
            const start = textarea.selectionStart;
            const end = textarea.selectionEnd;
            const value = textarea.value;
            textarea.value = `${value.substring(0, start)}  ${value.substring(end)}`;
            textarea.selectionStart = textarea.selectionEnd = start + 2;
            updateLineNumbers();
            updateValidationStatus();
            updateStepEditorHighlight();
            updateStepEditorSuggestions();
        }
    }

    function handleStepEditorEnterKey(event) {
        const textarea = DOM['step-yaml-editor'];
        if (!textarea || typeof textarea.value !== 'string') {
            return;
        }

        const start = textarea.selectionStart ?? 0;
        const end = textarea.selectionEnd ?? start;
        event.preventDefault();

        const value = textarea.value;
        const lineInfo = getCurrentLineInfo(value, start);
        const before = value.slice(0, start);
        const after = value.slice(end);
        const indentMatch = lineInfo.line.match(/^\s*/);
        const currentIndent = indentMatch ? indentMatch[0] : '';
        const trimmed = lineInfo.line.trim();
        const listTargetKeys = [...STEP_LIST_KEYS_WITH_NAME_TEMPLATE, ...STEP_LIST_KEYS_SIMPLE];
        const listParent = findNearestParentKey(value.slice(0, lineInfo.start), listTargetKeys, lineInfo.indent);

        let newIndent = currentIndent;
        let listPrefix = '';

        if (/^-\s*name\s*:/i.test(trimmed)) {
            newIndent = ' '.repeat(lineInfo.indent + 2);
        } else if (trimmed.startsWith('-')) {
            newIndent = currentIndent;
            if (listParent && STEP_LIST_KEYS_WITH_NAME_TEMPLATE.has(listParent)) {
                listPrefix = '- name: ';
            } else {
                listPrefix = '- ';
            }
        } else if (trimmed.endsWith(':')) {
            newIndent = currentIndent + '  ';
            const key = trimmed.slice(0, -1).trim();
            if (STEP_LIST_KEYS_WITH_NAME_TEMPLATE.has(key)) {
                listPrefix = '- name: ';
            } else if (STEP_LIST_KEYS_SIMPLE.has(key)) {
                listPrefix = '- ';
            }
        } else if (!trimmed && listParent) {
            if (STEP_LIST_KEYS_WITH_NAME_TEMPLATE.has(listParent)) {
                newIndent = ' '.repeat(lineInfo.indent);
                listPrefix = '- name: ';
            } else if (STEP_LIST_KEYS_SIMPLE.has(listParent)) {
                newIndent = ' '.repeat(lineInfo.indent);
                listPrefix = '- ';
            }
        }

        const insertion = `\n${newIndent}${listPrefix}`;
        textarea.value = before + insertion + after;
        const caret = before.length + insertion.length;
        textarea.selectionStart = textarea.selectionEnd = caret;
        textarea.focus();
        updateLineNumbers();
        updateValidationStatus();
        updateStepEditorHighlight();
        updateStepEditorSuggestions();
    }

    async function saveStepChanges() {
        if (!state.selectedId || !DOM['step-yaml-editor']) return;
        const yaml = DOM['step-yaml-editor'].value;
        const validation = validateStepYaml(yaml);
        if (!validation.ok) {
            updateValidationStatus(validation);
            showToast(validation.message || 'Step YAML is invalid.', 'error');
            return;
        }

        const identifier = state.selectedId;
        const url = `/v1/steps/${encodeIdentifier(identifier)}`;

        await context.fetchData(url, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/x-yaml' },
            body: yaml,
        });

        if (context.fetchData.lastError) {
            showToast(context.fetchData.lastError.message || 'Failed to save step.', 'error');
            return;
        }

        const metaFromYaml = extractMetaFromYaml(yaml, identifier);
        const savedAt = new Date().toISOString();
        const mergedMeta = updateStepMetadata(identifier, {
            ...metaFromYaml,
            updatedAt: savedAt,
            updatedLabel: formatUpdatedLabel(savedAt),
        });
        state.cache.set(identifier, {
            yaml,
            fetchedAt: Date.now(),
            meta: mergedMeta,
            isDraft: false,
        });
        state.currentYaml = yaml;
        if (!(state.stepSources instanceof Map)) {
            state.stepSources = new Map();
        }
        state.stepSources.set(identifier, 'database');
        state.drafts.delete(identifier);
        if (state.usageCache instanceof Map) {
            state.usageCache.delete(identifier);
        }
        if (state.usagePromises instanceof Map) {
            state.usagePromises.delete(identifier);
        }
        if (!state.steps.includes(identifier)) {
            state.steps.push(identifier);
            state.steps.sort((a, b) => a.localeCompare(b));
        }
        renderStepsList();
        renderSidebarTree();
        renderYamlView(yaml);
        exitEditMode({ resetEditor: true });
        showToast('Step saved successfully.', 'success');
    }

    function cancelEditing() {
        if (!state.isEditing) return;
        if (state.drafts.has(state.selectedId)) {
            const confirmDiscard = window.confirm('Discard this draft step? Unsaved changes will be lost.');
            if (!confirmDiscard) return;
            removeDraft(state.selectedId);
            showToast('Draft step discarded.', 'info');
            const folderHash = buildFolderHash(state.activeFolderKey);
            window.location.hash = folderHash;
            return;
        }
        exitEditMode({ resetEditor: true });
    }

    function removeDraft(identifier) {
        exitEditMode({ resetEditor: true, suppressHash: true });
        state.drafts.delete(identifier);
        state.cache.delete(identifier);
        if (state.stepSources instanceof Map) {
            state.stepSources.delete(identifier);
        }
        if (state.stepMetadata instanceof Map) {
            state.stepMetadata.delete(identifier);
        }
        if (state.usageCache instanceof Map) {
            state.usageCache.delete(identifier);
        }
        if (state.usagePromises instanceof Map) {
            state.usagePromises.delete(identifier);
        }
        state.steps = state.steps.filter(step => step !== identifier);
        state.selectedId = null;
        renderStepsList();
        renderSidebarTree();
        showListView();
    }

    function renderYamlView(yamlString) {
        const target = DOM['step-yaml-content'];
        if (!target) return;
        const renderer = global.yaml && typeof global.yaml.renderLines === 'function'
            ? global.yaml.renderLines
            : null;
        target.innerHTML = renderer
            ? renderer(yamlString, escapeHtml)
            : buildPlainYamlLines(yamlString);
    }

    function buildPlainYamlLines(yamlString) {
        const lines = (yamlString || '').split('\n');
        return lines.map((line, idx) => `
            <div class="yaml-line">
                <span class="yaml-line-number">${idx + 1}</span>
                <span class="yaml-line-text">${escapeHtml(line)}</span>
            </div>`).join('');
    }

    async function renderStepUsage(identifier) {
        const container = DOM['step-usage-content'];
        const empty = DOM['step-usage-empty'];
        if (!container || !identifier) return;

        const sourceKey = resolveStepSource(identifier);
        if (sourceKey === 'draft' || !state.stepSources?.has(identifier)) {
            container.innerHTML = '';
            if (empty) {
                empty.classList.remove('hidden');
                empty.textContent = 'Save this step to view pipeline usage.';
                container.appendChild(empty);
            } else {
                container.innerHTML = '<p class="text-sm text-[var(--text-secondary)]">Save this step to view pipeline usage.</p>';
            }
            return;
        }

        if (!state.usageCache.has(identifier)) {
            container.innerHTML = '<p class="text-sm text-[var(--text-secondary)]">Loading usage…</p>';
        }

        let usage = [];
        try {
            usage = await fetchStepUsage(identifier);
        } catch (error) {
            console.error('Failed to load step usage', error);
            container.innerHTML = '<p class="text-sm text-red-500">Unable to load pipeline usage.</p>';
            return;
        }

        if (!usage || usage.length === 0) {
            container.innerHTML = '';
            if (empty) {
                empty.classList.remove('hidden');
                container.appendChild(empty);
            } else {
                container.innerHTML = '<p class="text-sm text-[var(--text-secondary)]">This step is not referenced by any pipeline.</p>';
            }
            return;
        }

        if (empty) {
            empty.classList.add('hidden');
        }

        const rows = usage.map(item => {
            const pipelineName = item.name || item.identifier || 'Pipeline';
            const pipelinePath = formatPathLabel(item.path || 'root');
            const sourceLabel = formatSourceLabel(item.source);
            const href = buildPipelineHashFromIdentifier(item.identifier);
            return `
                <a href="${href}" class="pipelines-run-row block" title="Open pipeline ${escapeAttribute(pipelineName)}">
                    <div class="triggers-run-row">
                        <div class="triggers-run-row__line triggers-run-row__line--primary">
                            <span class="triggers-run-row__pipeline">${escapeHtml(pipelineName)}</span>
                            <span class="triggers-run-row__time">${escapeHtml(sourceLabel)}</span>
                        </div>
                        <div class="triggers-run-row__line triggers-run-row__line--status">
                            <span class="triggers-run-row__status">${escapeHtml(pipelinePath)}</span>
                        </div>
                    </div>
                </a>`;
        }).join('');

        container.innerHTML = rows;
    }

    function updateStepSuggestionPanelVisibility(forceLoad = false) {
        const panel = DOM['step-suggestion-panel'];
        if (!panel) return;
        const mode = state.stepSuggestionMode;
        const shouldShow = state.isEditing && !!mode;
        if (!shouldShow) {
            panel.classList.add('hidden');
            disableStepSuggestionOverlay();
            renderStepSuggestionSections();
            return;
        }

        panel.classList.remove('hidden');
        enableStepSuggestionOverlay();

        const loads = [];
        if (mode === 'environment') {
            const needsEnvReload = forceLoad || !state.environmentSuggestions.length || (Date.now() - state.environmentSuggestionLoadedAt) > STEP_ENV_REFRESH_MS;
            if (needsEnvReload) {
                loads.push(ensureStepEnvironmentSuggestions(forceLoad));
            }
        } else if (mode === 'secrets') {
            const needsSecretReload = forceLoad || !state.secretSuggestions.length || (Date.now() - state.secretSuggestionLoadedAt) > STEP_ENV_REFRESH_MS;
            if (needsSecretReload) {
                loads.push(ensureStepSecretSuggestions(forceLoad));
            }
        }

        if (loads.length) {
            Promise.allSettled(loads).finally(() => {
                renderStepSuggestionSections();
            });
        } else {
            renderStepSuggestionSections();
        }
    }

    async function ensureStepEnvironmentSuggestions(force = false) {
        if (!context || typeof context.fetchData !== 'function') {
            return [];
        }
        if (!force && state.environmentSuggestions.length && (Date.now() - state.environmentSuggestionLoadedAt) < STEP_ENV_REFRESH_MS) {
            return state.environmentSuggestions;
        }
        if (!force && state.environmentSuggestionPromise) {
            return state.environmentSuggestionPromise;
        }

        const promise = (async () => {
            const labels = await fetchStepEnvironmentScopeLabels();
            const summaries = [];
            for (const label of labels) {
                const variables = await fetchStepEnvironmentVariablesForLabel(label, force);
                summaries.push({
                    key: buildEnvironmentScopeKey(label),
                    label: formatEnvironmentScopeLabel(label),
                    count: variables.length,
                    preview: variables.slice(0, 6),
                });
            }
            state.environmentSuggestions = summaries;
            state.environmentSuggestionLoadedAt = Date.now();
            renderStepSuggestionSections();
            return summaries;
        })();

        state.environmentSuggestionPromise = promise;
        try {
            return await promise;
        } finally {
            state.environmentSuggestionPromise = null;
        }
    }

    async function fetchStepEnvironmentScopeLabels() {
        const labels = new Set(['']);
        if (!context || typeof context.fetchData !== 'function') {
            return Array.from(labels);
        }
        try {
            const response = await context.fetchData('/v1/environments/scopes');
            if (Array.isArray(response)) {
                response.forEach(entry => {
                    const normalized = normalizeEnvironmentScopeLabel(entry);
                    if (normalized !== null && normalized !== undefined) {
                        labels.add(normalized);
                    }
                });
            }
        } catch (error) {
            console.error('Failed to fetch environment scope list for step editor:', error);
        }
        return Array.from(labels).sort((a, b) => {
            if (a === b) return 0;
            if (a === '') return -1;
            if (b === '') return 1;
            return a.localeCompare(b, undefined, { sensitivity: 'base' });
        });
    }

    function formatEnvironmentScopeLabel(label) {
        const trimmed = (label || '').trim();
        return trimmed ? `/${trimmed}` : '/ (default)';
    }

    function buildEnvironmentScopeKey(label) {
        const trimmed = (label || '').trim();
        return trimmed || '__default__';
    }

    function normalizeEnvironmentScopeLabel(entry) {
        if (entry == null) return '';
        if (typeof entry === 'string') {
            return entry.trim().replace(/^\/+|\/+$/g, '');
        }
        if (typeof entry === 'object') {
            const value = entry.environment ?? entry.env ?? entry.name ?? entry.value ?? '';
            return String(value || '').trim().replace(/^\/+|\/+$/g, '');
        }
        return '';
    }

    async function fetchStepEnvironmentVariablesForLabel(label, force = false) {
        const normalized = typeof label === 'string' ? label : '';
        if (!force && state.environmentSuggestionCache instanceof Map && state.environmentSuggestionCache.has(normalized)) {
            return state.environmentSuggestionCache.get(normalized);
        }
        if (!context || typeof context.fetchData !== 'function') {
            return [];
        }

        const baseUrl = normalized ? `/v1/environments?env=${encodeURIComponent(normalized)}` : '/v1/environments';
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
            if (!(state.environmentSuggestionCache instanceof Map)) {
                state.environmentSuggestionCache = new Map();
            }
            state.environmentSuggestionCache.set(normalized, variables);
            return variables;
        } catch (error) {
            console.error(`Failed to load environment variables for '/${normalized || ''}'`, error);
            return [];
        }
    }

    async function ensureStepSecretSuggestions(force = false) {
        if (!context || typeof context.fetchData !== 'function') {
            return [];
        }
        if (!force && state.secretSuggestions.length && (Date.now() - state.secretSuggestionLoadedAt) < STEP_ENV_REFRESH_MS) {
            return state.secretSuggestions;
        }
        if (!force && state.secretSuggestionPromise) {
            return state.secretSuggestionPromise;
        }

        const promise = (async () => {
            const response = await context.fetchData('/v1/secrets');
            const names = Array.isArray(response)
                ? response.map(normalizeSecretName).filter(Boolean).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }))
                : [];
            state.secretSuggestions = names;
            state.secretSuggestionLoadedAt = Date.now();
            renderStepSuggestionSections();
            return names;
        })();

        state.secretSuggestionPromise = promise;
        try {
            return await promise;
        } finally {
            state.secretSuggestionPromise = null;
        }
    }

    function normalizeSecretName(entry) {
        if (entry == null) return '';
        if (typeof entry === 'string') {
            return entry.trim();
        }
        if (typeof entry === 'object' && entry.name) {
            return String(entry.name).trim();
        }
        return '';
    }

    function renderStepSuggestionSections() {
        const list = DOM['step-suggestion-list'];
        const emptyState = DOM['step-suggestion-empty'];
        if (!list || !emptyState) return;

        const mode = state.stepSuggestionMode;
        const contextInfo = state.stepSuggestionContext || null;
        const parsedStep = getActiveStepYamlObject();
        if (!state.isEditing || !mode) {
            list.innerHTML = '';
            emptyState.textContent = 'Enter edit mode to view suggestions.';
            emptyState.classList.remove('hidden');
            updateStepSuggestionOverlayPosition();
            return;
        }

        if (mode === 'environment') {
            setStepSuggestionPanelCopy({
                title: 'Environment variables',
                subtitle: 'Select a variable name to insert it.',
                footnote: 'Variables resolve based on the selected scope when the step runs.',
            });
            const envSections = buildEnvironmentSuggestionSections(state.environmentSuggestions);
            if (!envSections.length) {
                list.innerHTML = '';
                emptyState.textContent = state.environmentSuggestionPromise ? 'Loading environment variables…' : 'No environment variables available yet.';
                emptyState.classList.remove('hidden');
                updateStepSuggestionOverlayPosition();
                return;
            }
            emptyState.classList.add('hidden');
            list.innerHTML = envSections.join('');
            updateStepSuggestionOverlayPosition();
            return;
        }

        if (mode === 'secrets') {
            setStepSuggestionPanelCopy({
                title: 'Secrets',
                subtitle: 'Insert the secret name into the step definition.',
                footnote: 'Secrets are resolved based on environment scope.',
            });
            const secretSection = buildStepSecretSection(state.secretSuggestions);
            if (!secretSection) {
                list.innerHTML = '';
                emptyState.textContent = state.secretSuggestionPromise ? 'Loading secrets…' : 'No secrets available yet.';
                emptyState.classList.remove('hidden');
                updateStepSuggestionOverlayPosition();
                return;
            }
            emptyState.classList.add('hidden');
            list.innerHTML = secretSection;
            updateStepSuggestionOverlayPosition();
            return;
        }

        const isTaskMode = mode === 'directive-task';
        setStepSuggestionPanelCopy({
            title: isTaskMode ? 'Task directives' : 'Step directives',
            subtitle: 'Available keys for this section.',
            footnote: 'Click a directive to insert it at the cursor position.',
        });
        const definitions = isTaskMode
            ? filterTaskDirectives(TASK_DIRECTIVES, parsedStep, contextInfo)
            : filterStepDirectives(STEP_DIRECTIVES, parsedStep);
        const directiveSection = buildStepDirectiveSection(isTaskMode ? 'Task directives' : 'Step directives', definitions);
        if (!directiveSection) {
            list.innerHTML = '';
            emptyState.textContent = 'No directive suggestions available here.';
            emptyState.classList.remove('hidden');
            updateStepSuggestionOverlayPosition();
            return;
        }
        emptyState.classList.add('hidden');
        list.innerHTML = directiveSection;
        updateStepSuggestionOverlayPosition();
    }

    function setStepSuggestionPanelCopy(copy = {}) {
        if (DOM['step-suggestion-title']) {
            DOM['step-suggestion-title'].textContent = copy.title || 'Step helpers';
        }
        if (DOM['step-suggestion-subtitle']) {
            DOM['step-suggestion-subtitle'].textContent = copy.subtitle || '';
        }
        if (DOM['step-suggestion-footnote']) {
            DOM['step-suggestion-footnote'].textContent = copy.footnote || '';
        }
    }

    function buildStepDirectiveSection(title, directives) {
        if (!Array.isArray(directives) || !directives.length) return '';
        const buttons = directives.map(def => {
            const hint = def.hint ? `<span class="env-suggestion-hint">${escapeHtml(def.hint)}</span>` : '';
            return `
                <button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-step-directive="${escapeAttribute(def.key)}">
                    <span class="font-medium">${escapeHtml(def.key)}</span>
                    ${hint}
                </button>`;
        }).join('');

        return `
            <article class="env-suggestion-item">
                <div class="env-suggestion-env">
                    <span class="env-suggestion-env-label">${escapeHtml(title)}</span>
                </div>
                <div class="env-suggestion-variables">${buttons}</div>
            </article>`;
    }

    function buildStepSecretSection(secrets) {
        secrets = Array.isArray(secrets) ? secrets : [];
        if (!secrets.length) {
            if (state.secretSuggestionPromise) {
                return `<article class="env-suggestion-item"><div class="env-suggestion-env"><span class="env-suggestion-env-label">Secrets</span></div><div class="env-suggestion-variables"><span class="env-suggestion-pill env-suggestion-pill--more">Loading secrets…</span></div></article>`;
            }
            return '';
        }
        const buttons = secrets.slice(0, 12).map(name => `
            <button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-step-secret="${escapeAttribute(name)}">
                ${escapeHtml(name)}
            </button>`).join('');
        const remaining = Math.max(secrets.length - 12, 0);
        const more = remaining > 0 ? `<span class="env-suggestion-pill env-suggestion-pill--more">+${remaining} more</span>` : '';

        return `
            <article class="env-suggestion-item">
                <div class="env-suggestion-env">
                    <span class="env-suggestion-env-label">Secrets</span>
                    <span class="env-suggestion-env-count">${escapeHtml(`${secrets.length} total`)}</span>
                </div>
                <div class="env-suggestion-variables">${buttons}${more}</div>
            </article>`;
    }

    function buildEnvironmentSuggestionSections(summaries = []) {
        if (!summaries.length) {
            if (state.environmentSuggestionPromise) {
                return [`<article class="env-suggestion-item"><div class="env-suggestion-env"><span class="env-suggestion-env-label">Environment variables</span></div><div class="env-suggestion-variables"><span class="env-suggestion-pill env-suggestion-pill--more">Loading variables…</span></div></article>`];
            }
            return [];
        }

        return summaries.map(summary => {
            const preview = summary.preview || [];
            const pills = preview.length
                ? preview.map(name => `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-step-suggestion="${escapeAttribute(name)}">${escapeHtml(name)}</button>`).join('')
                : '<span class="env-suggestion-pill env-suggestion-pill--more">No variables</span>';
            const remaining = Math.max(summary.count - preview.length, 0);
            const moreBadge = remaining > 0
                ? `<span class="env-suggestion-pill env-suggestion-pill--more">+${remaining} more</span>`
                : '';

            return `
                <article class="env-suggestion-item">
                    <div class="env-suggestion-env">
                        <span class="env-suggestion-env-label">${escapeHtml(summary.label)}</span>
                        <span class="env-suggestion-env-count">${escapeHtml(`${summary.count} ${summary.count === 1 ? 'variable' : 'variables'}`)}</span>
                    </div>
                    <div class="env-suggestion-variables">${pills}${moreBadge}</div>
                </article>`;
        });
    }

    function getActiveStepYamlObject() {
        if (!state.isEditing || !DOM['step-yaml-editor']) {
            return null;
        }
        if (!window.jsyaml || typeof window.jsyaml.load !== 'function') {
            return null;
        }
        const yaml = DOM['step-yaml-editor'].value || '';
        if (!yaml.trim()) {
            return null;
        }
        try {
            const parsed = window.jsyaml.load(yaml);
            return parsed && typeof parsed === 'object' ? parsed : null;
        } catch {
            return null;
        }
    }

    function hasMeaningfulValue(entity, key) {
        if (!entity || typeof entity !== 'object') return false;
        return Object.prototype.hasOwnProperty.call(entity, key);
    }

    function filterStepDirectives(definitions, parsedStep) {
        if (!Array.isArray(definitions) || !definitions.length) return [];
        if (!parsedStep || typeof parsedStep !== 'object') {
            return definitions;
        }

        const hasInclude = hasMeaningfulValue(parsedStep, 'include');
        const hasTasks = hasMeaningfulValue(parsedStep, 'tasks');
        const hasGoal = hasMeaningfulValue(parsedStep, 'goal');
        const hasScript = hasMeaningfulValue(parsedStep, 'script');

        return definitions.filter(def => {
            const key = def?.key;
            if (!key) return false;
            if (key === 'include') {
                return !hasInclude && !hasTasks && !hasGoal && !hasScript;
            }
            if (key === 'tasks') {
                return !hasTasks && !hasInclude && !hasGoal && !hasScript;
            }
            if (key === 'goal') {
                return !hasGoal && !hasInclude && !hasTasks && !hasScript;
            }
            if (key === 'script') {
                return !hasScript && !hasInclude && !hasTasks && !hasGoal;
            }
            return !hasMeaningfulValue(parsedStep, key);
        });
    }

    function filterTaskDirectives(definitions, parsedStep, context = {}) {
        if (!Array.isArray(definitions) || !definitions.length) return [];
        if (!parsedStep || typeof parsedStep !== 'object') {
            return definitions;
        }
        const tasks = Array.isArray(parsedStep.tasks) ? parsedStep.tasks : [];
        const requestedIndex = typeof context.taskIndex === 'number'
            ? context.taskIndex
            : (tasks.length ? 0 : -1);
        const currentTask = (requestedIndex >= 0 && requestedIndex < tasks.length)
            ? (tasks[requestedIndex] && typeof tasks[requestedIndex] === 'object' ? tasks[requestedIndex] : null)
            : null;

        return definitions.filter(def => {
            const key = def?.key;
            if (!key) return false;
            if (key === 'goal') {
                return !hasMeaningfulValue(currentTask, 'goal') && !hasMeaningfulValue(currentTask, 'script');
            }
            if (key === 'script') {
                return !hasMeaningfulValue(currentTask, 'goal') && !hasMeaningfulValue(currentTask, 'script');
            }
            return !hasMeaningfulValue(currentTask, key);
        });
    }

    function handleStepSuggestionClick(event) {
        const envButton = event.target.closest('[data-step-suggestion]');
        if (envButton) {
            const value = envButton.getAttribute('data-step-suggestion') || '';
            if (!value) return;
            event.preventDefault();
            insertStepSuggestionValue(value);
            return;
        }

        const secretButton = event.target.closest('[data-step-secret]');
        if (secretButton) {
            const secretName = secretButton.getAttribute('data-step-secret') || '';
            if (!secretName) return;
            event.preventDefault();
            insertStepSecretValue(secretName);
            return;
        }

        const directiveButton = event.target.closest('[data-step-directive]');
        if (directiveButton) {
            const directiveKey = directiveButton.getAttribute('data-step-directive') || '';
            if (!directiveKey) return;
            event.preventDefault();
            insertStepDirectiveValue(directiveKey);
        }
    }

    function insertStepSuggestionValue(value) {
        insertValueIntoStepEditor(value);
    }

    function insertStepSecretValue(value) {
        insertValueIntoStepEditor(value);
    }

    function insertStepDirectiveValue(key) {
        const snippet = key.endsWith(':') ? `${key} ` : `${key}: `;
        insertValueIntoStepEditor(snippet);
    }

    function insertValueIntoStepEditor(value) {
        if (!state.isEditing || !DOM['step-yaml-editor']) {
            showToast('Enter edit mode to insert suggestions.', 'info');
            return;
        }
        const textarea = DOM['step-yaml-editor'];
        const start = textarea.selectionStart ?? textarea.value.length;
        const end = textarea.selectionEnd ?? start;
        const before = textarea.value.slice(0, start);
        const after = textarea.value.slice(end);
        textarea.value = `${before}${value}${after}`;
        const cursor = start + value.length;
        textarea.selectionStart = textarea.selectionEnd = cursor;
        textarea.focus();
        updateLineNumbers();
        updateValidationStatus();
        updateStepEditorHighlight();
    }

    function updateStepEditorSuggestions() {
        if (!DOM['step-yaml-editor']) return;
        if (!state.isEditing) {
            if (state.stepSuggestionMode !== null) {
                state.stepSuggestionMode = null;
                state.stepSuggestionContext = null;
                updateStepSuggestionPanelVisibility();
            }
            return;
        }

        const textarea = DOM['step-yaml-editor'];
        const selectionStart = textarea.selectionStart ?? 0;
        const selectionEnd = textarea.selectionEnd ?? selectionStart;
        const cursor = Math.min(selectionStart, selectionEnd);
        const contextInfo = detectStepEditorContext(textarea.value || '', cursor);
        const nextMode = contextInfo?.mode || 'directive';

        if (state.stepSuggestionMode !== nextMode) {
            state.stepSuggestionMode = nextMode;
            state.stepSuggestionContext = contextInfo;
            updateStepSuggestionPanelVisibility();
        } else {
            state.stepSuggestionContext = contextInfo;
            renderStepSuggestionSections();
        }
    }

    function detectStepEditorContext(text, cursorIndex) {
        if (typeof text !== 'string') {
            return { mode: 'directive', key: 'step' };
        }
        const lineInfo = getCurrentLineInfo(text, cursorIndex);
        if (!lineInfo) {
            return { mode: 'directive', key: 'step' };
        }
        const beforeText = text.slice(0, lineInfo.start);
        const trimmedLine = lineInfo.line.trim();

        if (trimmedLine.startsWith('environment:') || findNearestParentKey(beforeText, ['environment'], lineInfo.indent) === 'environment') {
            return { mode: 'environment', key: 'environment' };
        }

        if (trimmedLine.startsWith('secrets:') || findNearestParentKey(beforeText, ['secrets'], lineInfo.indent) === 'secrets') {
            return { mode: 'secrets', key: 'secrets' };
        }

        const taskParent = findNearestParentKey(beforeText, ['tasks'], lineInfo.indent, { withMeta: true });
        if (taskParent && taskParent.key === 'tasks') {
            const taskIndex = resolveTaskIndex(beforeText, taskParent, lineInfo);
            return { mode: 'directive-task', key: 'tasks', taskIndex };
        }

        return { mode: 'directive', key: 'step' };
    }

    function findNearestParentKey(beforeText, targetKeys, currentIndent, options = {}) {
        if (!beforeText) return options.withMeta ? null : null;
        const lines = beforeText.split('\n');
        for (let i = lines.length - 1; i >= 0; i -= 1) {
            const rawLine = lines[i];
            if (!rawLine) continue;
            const trimmed = rawLine.trim();
            if (!trimmed || trimmed.startsWith('#') || trimmed.startsWith('-')) {
                continue;
            }
            if (!trimmed.endsWith(':')) {
                continue;
            }
            const indentMatch = rawLine.match(/^\s*/);
            const indent = indentMatch ? indentMatch[0].length : 0;
            if (indent >= currentIndent) {
                continue;
            }
            const key = trimmed.slice(0, trimmed.indexOf(':')).trim();
            if (!targetKeys || targetKeys.includes(key)) {
                if (options.withMeta) {
                    return { key, indent };
                }
                return key;
            }
            return options.withMeta ? null : null;
        }
        return options.withMeta ? null : null;
    }

    function resolveTaskIndex(beforeText, taskParentInfo, currentLineInfo) {
        if (!taskParentInfo) {
            return 0;
        }
        const parentIndent = typeof taskParentInfo.indent === 'number' ? taskParentInfo.indent : 0;
        const entryIndent = parentIndent + 2;
        const lines = beforeText ? beforeText.split('\n') : [];
        let inTasksBlock = false;
        let currentIndex = -1;

        for (let i = 0; i < lines.length; i += 1) {
            const line = lines[i];
            if (!line) continue;
            const indent = line.match(/^\s*/)?.[0].length ?? 0;
            const trimmed = line.trim();
            if (!trimmed) continue;

            if (!inTasksBlock) {
                if (indent === parentIndent && trimmed.startsWith('tasks:')) {
                    inTasksBlock = true;
                }
                continue;
            }

            if (indent <= parentIndent && trimmed.endsWith(':')) {
                if (!trimmed.startsWith('tasks:')) {
                    break;
                }
                continue;
            }

            if (indent === entryIndent && trimmed.startsWith('-')) {
                currentIndex += 1;
            }
        }

        if (currentLineInfo && currentLineInfo.indent === entryIndent) {
            const currentTrimmed = (currentLineInfo.line || '').trim();
            if (currentTrimmed.startsWith('-')) {
                currentIndex += 1;
            }
        }

        return currentIndex >= 0 ? currentIndex : 0;
    }

    function getCurrentLineInfo(text, index) {
        const safeIndex = Math.max(0, Math.min(index, text.length));
        const lineStart = text.lastIndexOf('\n', safeIndex - 1);
        const start = lineStart === -1 ? 0 : lineStart + 1;
        const lineEnd = text.indexOf('\n', safeIndex);
        const end = lineEnd === -1 ? text.length : lineEnd;
        const line = text.slice(start, end);
        const indentMatch = line.match(/^\s*/);
        return {
            line,
            start,
            end,
            column: safeIndex - start,
            indent: indentMatch ? indentMatch[0].length : 0,
        };
    }

    function normalizeLineNumber(value) {
        if (typeof value !== 'number' || Number.isNaN(value)) {
            return null;
        }
        const normalized = Math.max(1, Math.floor(value));
        return Number.isFinite(normalized) ? normalized : null;
    }

    function findLineNumberByRegex(yamlString, regex) {
        if (!yamlString || !regex) return null;
        const lines = String(yamlString).split('\n');
        for (let i = 0; i < lines.length; i += 1) {
            if (regex.test(lines[i])) {
                return normalizeLineNumber(i + 1);
            }
        }
        return null;
    }

    function findLineNumberForKey(yamlString, key) {
        if (!yamlString || !key) return null;
        const pattern = new RegExp(`^\\s*(?:-\\s*)?${escapeRegExp(key)}\\s*:`, 'i');
        return findLineNumberByRegex(yamlString, pattern);
    }

    function findLineNumberForTaskName(yamlString, taskName) {
        if (!yamlString || !taskName) return null;
        const pattern = new RegExp(`^\\s*(?:-\\s*)?name:\\s*${escapeRegExp(taskName)}\\b`, 'i');
        return findLineNumberByRegex(yamlString, pattern);
    }

    async function fetchStepUsage(identifier) {
        if (!identifier || !context || typeof context.fetchData !== 'function') {
            return [];
        }
        if (!(state.usageCache instanceof Map)) {
            state.usageCache = new Map();
        }
        if (!(state.usagePromises instanceof Map)) {
            state.usagePromises = new Map();
        }
        if (state.usageCache.has(identifier)) {
            return state.usageCache.get(identifier);
        }
        if (state.usagePromises.has(identifier)) {
            return state.usagePromises.get(identifier);
        }
        const request = (async () => {
            const url = `/v1/steps/${encodeIdentifier(identifier)}/usage`;
            try {
                const response = await context.fetchData(url);
                if (context.fetchData.lastStatus === 404) {
                    return [];
                }
                return Array.isArray(response) ? response : [];
            } catch (error) {
                if (context.fetchData.lastStatus === 404) {
                    return [];
                }
                throw error;
            }
        })();
        state.usagePromises.set(identifier, request);
        try {
            const result = await request;
            state.usageCache.set(identifier, result);
            return result;
        } finally {
            state.usagePromises.delete(identifier);
        }
    }

    function validateStepYaml(yaml) {
        if (!yaml || !yaml.trim()) {
            return { ok: false, message: 'Step definition cannot be empty.', line: 1 };
        }
        if (!window.jsyaml) {
            return { ok: true };
        }
        try {
            const parsed = window.jsyaml.load(yaml);
            if (!parsed || typeof parsed !== 'object') {
                return { ok: false, message: 'Step YAML must define an object.', line: 1 };
            }

            const lineForKey = (key, fallback = null) => findLineNumberForKey(yaml, key) ?? fallback;
            const lineForTask = (taskName, fallback = null) => findLineNumberForTaskName(yaml, taskName) ?? fallback;
            const lineForMode = () => lineForKey('include') ?? lineForKey('tasks') ?? lineForKey('goal') ?? lineForKey('script') ?? 1;
            const tasksLine = lineForKey('tasks') ?? 1;

            const invalidStepKey = Object.keys(parsed).find(key => !STEP_ALLOWED_KEYS.has(key));
            if (invalidStepKey) {
                return {
                    ok: false,
                    message: `Unknown step directive '${invalidStepKey}'. Remove or replace it with a supported key.`,
                    line: lineForKey(invalidStepKey) ?? 1,
                };
            }

            const name = typeof parsed.name === 'string' ? parsed.name.trim() : '';
            if (!name) {
                return { ok: false, message: "Step YAML must include a 'name' field.", line: lineForKey('name') ?? 1 };
            }
            if (!/^[A-Za-z0-9_.-]+$/.test(name)) {
                return { ok: false, message: "Step name can only contain letters, numbers, dots, underscores, and hyphens.", line: lineForKey('name') ?? 1 };
            }

            const includeValue = typeof parsed.include === 'string' ? parsed.include.trim() : '';
            const tasks = Array.isArray(parsed.tasks) ? parsed.tasks : [];
            const goalValue = typeof parsed.goal === 'string' ? parsed.goal.trim() : '';
            const scriptValue = typeof parsed.script === 'string' ? parsed.script.trim() : '';

            const modes = [
                includeValue !== '',
                tasks.length > 0,
                goalValue !== '',
                scriptValue !== '',
            ].filter(Boolean);

            if (!modes.length) {
                return { ok: false, message: "Step must define one of 'include', 'tasks', 'goal', or 'script'.", line: lineForMode() };
            }
            if (modes.length > 1) {
                return { ok: false, message: "Step may only define one of 'include', 'tasks', 'goal', or 'script'.", line: lineForMode() };
            }

            if (includeValue) {
                if (!includeValue.startsWith('step:')) {
                    return { ok: false, message: "Include steps must reference a reusable step using the 'step:' prefix.", line: lineForKey('include') ?? 1 };
                }
                return { ok: true };
            }

            if (tasks.length) {
                const taskNames = new Map();
                for (let index = 0; index < tasks.length; index += 1) {
                    const taskObj = tasks[index] && typeof tasks[index] === 'object' ? tasks[index] : {};
                    const taskName = typeof taskObj.name === 'string' ? taskObj.name.trim() : '';
                    if (!taskName) {
                        return { ok: false, message: `Task #${index + 1} is missing the required 'name' field.`, line: tasksLine };
                    }
                    const nameKey = taskName.toLowerCase();
                    if (taskNames.has(nameKey)) {
                        return { ok: false, message: `Duplicate task name '${taskName}' found. Task names must be unique within a step.`, line: lineForTask(taskName, tasksLine) };
                    }
                    taskNames.set(nameKey, true);

                    const taskGoal = typeof taskObj.goal === 'string' ? taskObj.goal.trim() : '';
                    const taskScript = typeof taskObj.script === 'string' ? taskObj.script.trim() : '';
                    if (taskGoal && taskScript) {
                        return { ok: false, message: `Task '${taskName}' cannot define both 'goal' and 'script'.`, line: lineForTask(taskName, tasksLine) };
                    }
                    if (!taskGoal && !taskScript) {
                        return { ok: false, message: `Task '${taskName}' must define either 'goal' or 'script'.`, line: lineForTask(taskName, tasksLine) };
                    }

                    const invalidTaskKey = Object.keys(taskObj).find(key => !TASK_ALLOWED_KEYS.has(key));
                    if (invalidTaskKey) {
                        const fallbackLine = lineForTask(taskName, tasksLine);
                        return {
                            ok: false,
                            message: `Task '${taskName}' contains unknown directive '${invalidTaskKey}'.`,
                            line: lineForKey(invalidTaskKey, fallbackLine) ?? fallbackLine ?? tasksLine,
                        };
                    }
                }

                for (const task of tasks) {
                    const deps = Array.isArray(task?.depends_on) ? task.depends_on : [];
                    for (const dep of deps) {
                        const key = typeof dep === 'string' ? dep.trim().toLowerCase() : '';
                        if (!key) {
                            return { ok: false, message: 'Task dependency names must be non-empty strings.', line: lineForKey('depends_on', lineForTask(task?.name, tasksLine)) };
                        }
                        if (!taskNames.has(key)) {
                            return { ok: false, message: `Task '${task?.name || 'unknown'}' depends on undefined task '${dep}'.`, line: lineForTask(task?.name, tasksLine) };
                        }
                    }
                }

                return { ok: true };
            }

            if (goalValue && scriptValue) {
                return { ok: false, message: "A step may not define both 'goal' and 'script'.", line: lineForKey('goal', lineForKey('script') ?? 1) };
            }
            if (!goalValue && !scriptValue) {
                return { ok: false, message: "Step must include executable content such as 'goal' or 'script'.", line: lineForKey('goal', lineForKey('script') ?? 1) };
            }

            return { ok: true };
        } catch (error) {
            const parsedLine = typeof error?.mark?.line === 'number' ? error.mark.line + 1 : null;
            return {
                ok: false,
                message: error.message || 'Unable to parse YAML.',
                line: normalizeLineNumber(parsedLine),
            };
        }
    }

    function updateValidationStatus(result) {
        if (!DOM['step-validation-status'] || !DOM['step-yaml-editor']) return;
        if (!state.isEditing) {
            DOM['step-validation-status'].classList.add('hidden');
            DOM['step-validation-status'].textContent = '';
            state.stepValidationErrors = [];
            updateLineNumbers();
            return;
        }

        let validationInfo = null;
        if (result && typeof result === 'object') {
            validationInfo = result;
        } else if (typeof result === 'string') {
            validationInfo = { ok: false, message: result };
        } else {
            validationInfo = validateStepYaml(DOM['step-yaml-editor'].value);
        }

        const isError = validationInfo && validationInfo.ok === false;
        const message = validationInfo?.message || (isError ? 'Step YAML is invalid.' : 'Step definition parsed successfully.');
        const lineNumber = normalizeLineNumber(validationInfo?.line);

        if (isError) {
            const lineMarkup = lineNumber ? `<span class="validation-box__line">Line ${lineNumber}</span>` : '';
            DOM['step-validation-status'].innerHTML = `
                <div class="validation-box__header">Validation issue</div>
                <div class="validation-box__item">
                    ${lineMarkup}
                    <div class="validation-box__message">${escapeHtml(message)}</div>
                </div>`;
            DOM['step-validation-status'].className = 'validation-box validation-box--error';
            state.stepValidationErrors = lineNumber ? [{ line: lineNumber, message }] : [];
        } else {
            DOM['step-validation-status'].innerHTML = '<div class="validation-box__header">All good</div><div class="validation-box__message">Step definition parsed successfully.</div>';
            DOM['step-validation-status'].className = 'validation-box validation-box--success';
            state.stepValidationErrors = [];
        }

        DOM['step-validation-status'].classList.remove('hidden');
        updateLineNumbers();
    }

    function openNewStepModal() {
        if (!DOM['steps-new-modal']) return;
        DOM['steps-new-modal'].classList.remove('hidden');
        requestAnimationFrame(() => DOM['steps-new-modal'].classList.add('show'));
        if (DOM['steps-new-form']) DOM['steps-new-form'].reset();
        if (DOM['steps-new-path']) DOM['steps-new-path'].focus();
    }

    function closeNewStepModal() {
        if (!DOM['steps-new-modal']) return;
        DOM['steps-new-modal'].classList.remove('show');
        setTimeout(() => {
            DOM['steps-new-modal'].classList.add('hidden');
        }, 200);
    }

    function buildDefaultStepYaml(name) {
        return `name: ${name}\ngoal: |\n  Describe what ${name} should accomplish.\nscript: |\n  echo "Implement ${name}"\n`;
    }

    function handleCreateStep(event) {
        event.preventDefault();
        if (!DOM['steps-new-path'] || !DOM['steps-new-name']) return;
        const pathRaw = DOM['steps-new-path'].value.trim().replace(/^\/+|\/+$/g, '');
        const nameRaw = DOM['steps-new-name'].value.trim();

        if (!nameRaw) {
            showToast('Step name is required.', 'error');
            return;
        }
        if (!/^[A-Za-z0-9_.-]+$/.test(nameRaw)) {
            showToast('Step name can only contain letters, numbers, dots, underscores, and hyphens.', 'error');
            return;
        }

        const identifier = pathRaw ? `${pathRaw}/${nameRaw}` : nameRaw;
        if (state.steps.includes(identifier)) {
            showToast('A step with that identifier already exists.', 'error');
            return;
        }

        const yaml = buildDefaultStepYaml(nameRaw);
        state.steps.push(identifier);
        state.steps.sort((a, b) => a.localeCompare(b));
        state.cache.set(identifier, {
            yaml,
            fetchedAt: Date.now(),
            meta: {
                name: nameRaw,
                description: 'Describe what this step does.',
                path: pathRaw,
                goal: '',
                updatedLabel: 'Draft',
            },
            isDraft: true,
        });
        state.drafts.add(identifier);
        if (!(state.stepSources instanceof Map)) {
            state.stepSources = new Map();
        }
        state.stepSources.set(identifier, 'draft');
        updateStepMetadata(identifier, {
            name: nameRaw,
            path: pathRaw,
            updatedLabel: 'Draft',
            updatedAt: '',
        });
        state.selectedId = identifier;
        state.activeFolderKey = pathRaw;
        ensureSidebarExpansionForPath(state.activeFolderKey);
        renderStepsList();
        renderSidebarTree();
        closeNewStepModal();
        window.location.hash = buildStepHash(identifier, { edit: true });
        showToast('Draft step created. Update the YAML and save when ready.', 'info');
    }

    function generateCloneName(baseName) {
        const sanitized = (baseName || 'step').trim().replace(/[^A-Za-z0-9_.-]/g, '-').replace(/^-+|-+$/g, '') || 'step';
        let candidate = `${sanitized}-copy`;
        let counter = 2;
        while (state.steps.includes(candidate)) {
            candidate = `${sanitized}-copy-${counter}`;
            counter += 1;
        }
        return candidate;
    }

    function cloneStepYamlWithName(originalYaml, newName) {
        let updated = originalYaml;
        if (window.jsyaml && typeof window.jsyaml.load === 'function' && typeof window.jsyaml.dump === 'function') {
            try {
                const parsed = window.jsyaml.load(originalYaml);
                if (parsed && typeof parsed === 'object') {
                    parsed.name = newName;
                    updated = window.jsyaml.dump(parsed, { lineWidth: 120, noRefs: true });
                }
            } catch {
                // fall through to regex replacement
            }
        }
        if (!/^\s*name\s*:/m.test(updated)) {
            return `name: ${newName}\n${updated}`;
        }
        return updated.replace(/(^\s*name\s*:\s*).*$/m, `$1${newName}`);
    }

    async function openCloneStepModal(identifier) {
        if (!DOM['steps-clone-modal']) return;
        if (state.isEditing) {
            notifyEditingLock();
            return;
        }
        let cacheEntry = state.cache.get(identifier);
        if (!cacheEntry || !cacheEntry.yaml) {
            await renderStepDetail(identifier);
            cacheEntry = state.cache.get(identifier);
        }
        if (!cacheEntry || !cacheEntry.yaml) {
            showToast('Unable to load step definition for cloning.', 'error');
            return;
        }

        const meta = cacheEntry.meta || getStepMetadata(identifier) || {};
        state.cloneContext = {
            sourceId: identifier,
            yaml: cacheEntry.yaml,
            meta,
        };

        if (DOM['steps-clone-path']) {
            DOM['steps-clone-path'].value = meta.path || '';
        }
        if (DOM['steps-clone-name']) {
            DOM['steps-clone-name'].value = generateCloneName(meta.name || getStepName(identifier));
        }
        if (DOM['steps-clone-subtitle']) {
            DOM['steps-clone-subtitle'].textContent = `Cloning from “${meta.name || identifier}”. Provide a new path and name.`;
        }

        DOM['steps-clone-modal'].classList.remove('hidden');
        requestAnimationFrame(() => DOM['steps-clone-modal'].classList.add('show'));
    }

    function closeCloneStepModal() {
        if (!DOM['steps-clone-modal']) return;
        state.cloneContext = null;
        DOM['steps-clone-modal'].classList.remove('show');
        setTimeout(() => {
            DOM['steps-clone-modal'].classList.add('hidden');
            if (DOM['steps-clone-form']) {
                DOM['steps-clone-form'].reset();
            }
        }, 200);
    }

    function validateStepIdentifier(path, name) {
        if (!name) {
            showToast('Step name is required.', 'error');
            return false;
        }
        if (!/^[A-Za-z0-9_.-]+$/.test(name)) {
            showToast('Step name can only contain letters, numbers, dots, underscores, and hyphens.', 'error');
            return false;
        }
        return true;
    }

    async function handleCloneStep(event) {
        event.preventDefault();
        if (!state.cloneContext) {
            closeCloneStepModal();
            return;
        }
        const pathValue = (DOM['steps-clone-path']?.value || '').trim().replace(/^\/+|\/+$/g, '');
        const nameValue = (DOM['steps-clone-name']?.value || '').trim();
        if (!validateStepIdentifier(pathValue, nameValue)) {
            return;
        }

        const newIdentifier = pathValue ? `${pathValue}/${nameValue}` : nameValue;
        if (state.steps.includes(newIdentifier)) {
            showToast('A step with that identifier already exists.', 'error');
            return;
        }

        const clonedYaml = cloneStepYamlWithName(state.cloneContext.yaml || '', nameValue);
        state.steps.push(newIdentifier);
        state.steps.sort((a, b) => a.localeCompare(b));
        state.cache.set(newIdentifier, {
            yaml: clonedYaml,
            fetchedAt: Date.now(),
            meta: {
                name: nameValue,
                description: state.cloneContext.meta?.description || 'Describe what this step does.',
                path: pathValue,
                goal: state.cloneContext.meta?.goal || '',
                updatedLabel: 'Draft',
            },
            isDraft: true,
        });
        if (!(state.stepSources instanceof Map)) {
            state.stepSources = new Map();
        }
        state.stepSources.set(newIdentifier, 'draft');
        updateStepMetadata(newIdentifier, {
            name: nameValue,
            path: pathValue,
            updatedLabel: 'Draft',
        });
        state.drafts.add(newIdentifier);
        closeCloneStepModal();
        showToast('Draft cloned. Update the YAML and save when ready.', 'info');
        window.location.hash = buildStepHash(newIdentifier, { edit: true });
    }

    function openDeleteModal(identifier) {
        if (!DOM['steps-delete-modal'] || !identifier) return;
        if (state.isEditing) {
            notifyEditingLock();
            return;
        }
        if (resolveStepSource(identifier) === 'git') {
            showToast('This step is managed via Git and must be removed from the config repository.', 'info');
            return;
        }
        state.pendingDelete = identifier;
        if (DOM['steps-delete-message']) {
            DOM['steps-delete-message'].textContent = `Are you sure you want to delete “${identifier}”?`;
        }
        DOM['steps-delete-modal'].classList.remove('hidden');
        requestAnimationFrame(() => DOM['steps-delete-modal'].classList.add('show'));
    }

    function closeDeleteModal() {
        if (!DOM['steps-delete-modal']) return;
        DOM['steps-delete-modal'].classList.remove('show');
        setTimeout(() => {
            DOM['steps-delete-modal'].classList.add('hidden');
        }, 200);
        state.pendingDelete = null;
    }

    async function confirmDeleteStep() {
        const identifier = state.pendingDelete;
        closeDeleteModal();
        if (!identifier) return;

        if (resolveStepSource(identifier) === 'git') {
            showToast('This step is managed via Git and must be removed from the config repository.', 'info');
            return;
        }

        if (state.drafts.has(identifier)) {
            removeDraft(identifier);
            showToast('Draft step removed.', 'success');
            return;
        }

        const url = `/v1/steps/${encodeIdentifier(identifier)}`;
        await context.deleteData(url);

        if (context.fetchData.lastError) {
            showToast(context.fetchData.lastError.message || 'Failed to delete step.', 'error');
            return;
        }

        state.cache.delete(identifier);
        if (state.stepSources instanceof Map) {
            state.stepSources.delete(identifier);
        }
        if (state.stepMetadata instanceof Map) {
            state.stepMetadata.delete(identifier);
        }
        state.steps = state.steps.filter(step => step !== identifier);
        if (state.selectedId === identifier) {
            state.selectedId = null;
        }
        renderStepsList();
        renderSidebarTree();
        showListView();
        showToast('Step deleted.', 'success');
        const folderHash = buildFolderHash(state.activeFolderKey);
        window.location.hash = folderHash;
    }

    function handleSearch(event) {
        state.searchTerm = event.target.value || '';
        renderStepsList();
    }

    function handleListClick(event) {
        const deleteButton = event.target.closest('[data-delete-step]');
        if (deleteButton) {
            event.stopPropagation();
            if (state.isEditing) {
                notifyEditingLock();
                return;
            }
            const identifier = deleteButton.dataset.deleteStep;
            if (identifier) openDeleteModal(identifier);
            return;
        }

        const folderCard = event.target.closest('[data-folder-key]');
        if (folderCard) {
            if (state.isEditing) {
                notifyEditingLock();
                return;
            }
            const folderKey = folderCard.dataset.folderKey || '';
            state.activeFolderKey = folderKey;
            ensureSidebarExpansionForPath(folderKey);
            window.location.hash = buildFolderHash(folderKey);
            return;
        }

        const stepCard = event.target.closest('[data-step-id]');
        if (stepCard) {
            const identifier = stepCard.dataset.stepId;
            if (identifier) {
                if (state.isEditing) {
                    if (identifier !== state.selectedId) {
                        notifyEditingLock();
                        return;
                    }
                    return;
                }
                window.location.hash = buildStepHash(identifier);
            }
        }
    }

    function handleListKeydown(event) {
        if (event.key !== 'Enter' && event.key !== ' ') return;
        const folderCard = event.target.closest('[data-folder-key]');
        if (folderCard) {
            event.preventDefault();
            if (state.isEditing) {
                notifyEditingLock();
                return;
            }
            const folderKey = folderCard.dataset.folderKey || '';
            state.activeFolderKey = folderKey;
            ensureSidebarExpansionForPath(folderKey);
            window.location.hash = buildFolderHash(folderKey);
            return;
        }

        const stepCard = event.target.closest('[data-step-id]');
        if (stepCard) {
            event.preventDefault();
            const identifier = stepCard.dataset.stepId;
            if (identifier) {
                if (state.isEditing) {
                    if (identifier !== state.selectedId) {
                        notifyEditingLock();
                        return;
                    }
                    return;
                }
                window.location.hash = buildStepHash(identifier);
            }
        }
    }

    function handleSidebarTreeClick(event) {
        const toggleBtn = event.target.closest('[data-step-toggle-folder]');
        if (toggleBtn) {
            event.preventDefault();
            if (state.isEditing) {
                notifyEditingLock();
                return;
            }
            const folderPath = toggleBtn.dataset.stepToggleFolder || '';
            if (state.sidebarExpanded.has(folderPath)) {
                state.sidebarExpanded.delete(folderPath);
            } else {
                state.sidebarExpanded.add(folderPath);
            }
            renderSidebarTree();
            return;
        }

        const openFolderBtn = event.target.closest('[data-step-open-folder]');
        if (openFolderBtn) {
            event.preventDefault();
            if (state.isEditing) {
                notifyEditingLock();
                return;
            }
            const folderKey = openFolderBtn.dataset.stepOpenFolder || '';
            state.activeFolderKey = folderKey;
            ensureSidebarExpansionForPath(folderKey);
            window.location.hash = buildFolderHash(folderKey);
            return;
        }

        const stepLink = event.target.closest('[data-step-link]');
        if (stepLink) {
            if (state.isEditing) {
                const identifier = stepLink.dataset.stepLink || '';
                if (identifier !== state.selectedId) {
                    notifyEditingLock();
                    event.preventDefault();
                    return;
                }
                return;
            }
            state.activeFolderKey = getFolderPath(stepLink.dataset.stepLink || '');
        }
    }

    function copyStepYaml() {
        if (!navigator.clipboard) {
            showToast('Clipboard API not available.', 'error');
            return;
        }
        const yaml = state.isEditing && DOM['step-yaml-editor'] ? DOM['step-yaml-editor'].value : state.currentYaml;
        navigator.clipboard.writeText(yaml || '').then(() => {
            showToast('Step YAML copied to clipboard.', 'success');
        }).catch(() => {
            showToast('Failed to copy YAML.', 'error');
        });
    }

    function downloadStepYaml() {
        const yaml = state.isEditing && DOM['step-yaml-editor'] ? DOM['step-yaml-editor'].value : state.currentYaml;
        if (!yaml) {
            showToast('No YAML content to download.', 'error');
            return;
        }
        const blob = new Blob([yaml], { type: 'text/yaml' });
        const url = URL.createObjectURL(blob);
        const anchor = document.createElement('a');
        anchor.href = url;
        const safeName = (state.selectedId || 'step').replace(/\/+|\\+/g, '-');
        anchor.download = `${safeName}.yaml`;
        document.body.appendChild(anchor);
        anchor.click();
        document.body.removeChild(anchor);
        URL.revokeObjectURL(url);
    }

    function extractMetaFromYaml(yaml, identifier) {
        const meta = {
            name: getStepName(identifier),
            description: '',
            path: getFolderPath(identifier),
            goal: '',
            updatedLabel: '—',
        };
        if (!window.jsyaml) return meta;
        try {
            const parsed = window.jsyaml.load(yaml);
            if (!parsed || typeof parsed !== 'object') return meta;
            if (parsed.name) meta.name = String(parsed.name);
            if (parsed.goal) {
                const goalText = String(parsed.goal);
                meta.goal = goalText;
                meta.description = goalText.split('\n').map(line => line.trim()).filter(Boolean)[0] || '';
            }
            if (!meta.description && parsed.description) {
                meta.description = String(parsed.description);
            }
        } catch {
            // ignore parse errors
        }
        if (!meta.description) {
            meta.description = 'Reusable step definition.';
        }
        return meta;
    }

    function buildPipelineHashFromIdentifier(identifier) {
        if (!identifier) return '#/pipelines';
        const segments = identifier.split('/').filter(Boolean).map(encodeURIComponent);
        return segments.length ? `#/pipelines/${segments.join('/')}` : '#/pipelines';
    }

    function ensureSidebarExpansionForPath(path) {
        if (!path) return;
        const segments = path.split('/').filter(Boolean);
        let current = '';
        segments.forEach(segment => {
            current = current ? `${current}/${segment}` : segment;
            state.sidebarExpanded.add(current);
        });
    }

    function buildStepHash(identifier, options = {}) {
        const segments = (identifier || '').split('/').filter(Boolean).map(encodeURIComponent);
        let hash = '#/steps';
        if (segments.length) {
            hash += `/${segments.join('/')}`;
        }
        if (options.edit) {
            hash += '/edit';
        }
        return hash;
    }

    function buildFolderHash(folderKey) {
        if (!folderKey) return '#/steps';
        const segments = folderKey.split('/').filter(Boolean).map(encodeURIComponent);
        return `#/steps/${segments.join('/')}`;
    }

    function encodeIdentifier(identifier) {
        return (identifier || '').split('/').map(encodeURIComponent).join('/');
    }

    function getFolderPath(identifier) {
        if (!identifier) return '';
        const segments = identifier.split('/').filter(Boolean);
        segments.pop();
        return segments.join('/');
    }

    function getStepName(identifier) {
        if (!identifier) return '';
        const segments = identifier.split('/').filter(Boolean);
        return segments.pop() || identifier;
    }

    function formatPathLabel(path) {
        if (!path) return 'root';
        return path;
    }

    function decodeURIComponentPath(value) {
        try {
            return decodeURIComponent(value);
        } catch {
            return value;
        }
    }

    function escapeRegExp(value) {
        return String(value || '').replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }

    function escapeHtml(value) {
        return String(value || '')
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    function escapeAttribute(value) {
        return escapeHtml(value).replace(/"/g, '&quot;');
    }

    function showToast(message, variant = 'info') {
        const container = document.getElementById('toast-container');
        if (!container) {
            alert(message);
            return;
        }
        const toast = document.createElement('div');
        toast.className = `pipelines-toast pipelines-toast--${variant}`;
        toast.innerHTML = `<div class="pipelines-toast__content">${escapeHtml(message)}</div>`;
        container.appendChild(toast);
        requestAnimationFrame(() => toast.classList.add('show'));
        setTimeout(() => {
            toast.classList.remove('show');
            setTimeout(() => toast.remove(), 200);
        }, TOAST_TIMEOUT);
    }

    global.pages = global.pages || {};
    global.pages.steps = {
        init,
        handleRoute,
        renderSidebarTree,
    };
})(window.NopsAI = window.NopsAI || {});
