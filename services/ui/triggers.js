(function (global) {
    const state = {
        triggers: [],
        filteredSlugs: [],
        triggerCache: new Map(),
        triggerTree: null,
        selectedSlug: null,
        activeFolderKey: '',
        isEditing: false,
        currentYaml: '',
        searchTerm: '',
        runsCache: { fetchedAt: 0, runs: [] },
        pendingDeleteSlug: null,
        triggerSources: new Map(),
        sidebarExpanded: new Set(),
        pipelineSourceIndex: null,
        pipelineMetaCache: new Map(),
        pipelineMetaPromises: new Map(),
        pipelineSourceIndexPromise: null,
        _prefetching: new Set(),
        editorSuggestionContext: null,
        editorSuggestionItems: [],
        editorSuggestionIndex: -1,
        editorCharWidth: null,
        currentTriggerSummary: null,
    };

    const DOM = {};
    let context = null;
    let initialized = false;
    let measurementCanvas = null;

    const RUNS_CACHE_TTL = 60 * 1000;
    const MAX_RUNS = 5;
    const VALID_TRIGGER_EVENTS = new Map([
        ['push', 'push'],
        ['pull_request', 'pull_request'],
        ['pull-request', 'pull_request'],
    ]);
    const TRIGGER_EVENT_OPTIONS = Array.from(new Set(VALID_TRIGGER_EVENTS.values()));
    const TRIGGER_ROOT_DEFINITIONS = [
        {
            key: 'triggers',
            hint: 'List of trigger rules',
            snippet: 'triggers:\n  - on: \n    branches:\n      - \n    pipelines:\n      - ',
        },
    ];
    const TRIGGER_FIELD_DEFINITIONS = [
        { key: 'on', hint: 'Event type to run pipelines for', kind: 'scalar' },
        { key: 'branches', hint: 'Only run for these branches', kind: 'list' },
        { key: 'skip_branches', hint: 'Branches to exclude', kind: 'list' },
        { key: 'tags', hint: 'Tags that should trigger runs', kind: 'list' },
        { key: 'pipelines', hint: 'Pipelines to execute', kind: 'list' },
        { key: 'environment', hint: 'Environment label for this trigger', kind: 'scalar' },
    ];
    const TRIGGER_LIST_FIELDS = new Set(['branches', 'skip_branches', 'tags', 'pipelines']);
    const DEFAULT_PIPELINE_PATH = 'pipelines/sample-pipeline.yaml';

    function sanitizePipelineFileName(value) {
        const trimmed = String(value || '').trim();
        const fallback = 'sample-pipeline';
        if (!trimmed) return fallback;
        const sanitized = trimmed.replace(/[^A-Za-z0-9_.-]+/g, '-').replace(/^-+|-+$/g, '');
        return sanitized || fallback;
    }

    function deriveDefaultPipelinePath(repo) {
        if (!repo) {
            return DEFAULT_PIPELINE_PATH;
        }
        const parts = String(repo).split('/').filter(Boolean);
        const candidate = parts[parts.length - 1] || '';
        const fileName = sanitizePipelineFileName(candidate);
        return `pipelines/${fileName}.yaml`;
    }

    function buildNewTriggerYaml(pipelinePath = DEFAULT_PIPELINE_PATH) {
        const path = pipelinePath || DEFAULT_PIPELINE_PATH;
        return `triggers:\n  - on: push\n    branches:\n      - main\n    pipelines:\n      - ${path}\n`;
    }

    function parseTriggerOverrideList(items) {
        const slugs = [];
        const sourceMap = new Map();
        if (!Array.isArray(items)) {
            return { slugs, sourceMap };
        }
        items.forEach(item => {
            if (item == null) return;
            let slug = '';
            let source = '';
            if (typeof item === 'string') {
                slug = item;
                source = 'database';
            } else if (typeof item === 'object') {
                slug = item.repository_name || item.name || item.slug || item.repo || item.id || '';
                source = item.source || '';
            }
            slug = String(slug || '').trim();
            if (!slug) return;
            slugs.push(slug);
            if (source) {
                sourceMap.set(slug, normalizePipelineSourceKey(source));
            }
        });
        return { slugs, sourceMap };
    }

    function updateNewTriggerBlueprint() {
        const repoInput = DOM['triggers-new-repo'];
        const yamlInput = DOM['triggers-new-yaml'];
        const repo = repoInput ? repoInput.value.trim() : '';
        const pipelinePath = deriveDefaultPipelinePath(repo);
        if (yamlInput) {
            yamlInput.value = buildNewTriggerYaml(pipelinePath);
        }
    }

    function openCreateTriggerModal(options = {}) {
        if (DOM['triggers-new-form']) {
            DOM['triggers-new-form'].reset();
        }

        const repoValue = typeof options.repository === 'string' ? options.repository : '';
        if (DOM['triggers-new-repo']) {
            DOM['triggers-new-repo'].value = repoValue;
        }

        updateNewTriggerBlueprint();
        openModal('triggers-new-modal');

        if (DOM['triggers-new-repo']) {
            DOM['triggers-new-repo'].focus();
            const input = DOM['triggers-new-repo'];
            if (repoValue && typeof input.setSelectionRange === 'function') {
                const length = input.value.length;
                input.setSelectionRange(length, length);
            }
        }
    }

    function getActiveTriggerFolderPrefix() {
        const key = normalizeFolderKey(state.activeFolderKey || '');
        return key ? `${key}/` : '';
    }

    function formatTriggerFolderLabel(label) {
        const str = String(label || '').trim();
        if (!str) return 'Folder';
        return str.replace(/[-_]+/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
    }

    function getActiveTriggerSidebarFolder() {
        const explicit = (state.activeFolderKey || '').trim();
        if (explicit) return explicit;
        return getFolderKeyForSlug(state.selectedSlug || '');
    }

    function ensureTriggerSidebarExpansionForPath(path) {
        if (!path) return;
        if (!(state.sidebarExpanded instanceof Set)) {
            state.sidebarExpanded = new Set();
        }
        const segments = String(path).split('/').filter(Boolean);
        let current = '';
        segments.forEach(segment => {
            current = current ? `${current}/${segment}` : segment;
            state.sidebarExpanded.add(current);
        });
    }

    function shouldExpandTriggerFolder(path, activeFolder, activeTriggerFolder) {
        if (!path) return true;
        if (!(state.sidebarExpanded instanceof Set)) {
            state.sidebarExpanded = new Set();
        }
        if (state.sidebarExpanded.has(path)) {
            return true;
        }
        const hasActiveFolder = activeFolder && (activeFolder === path || activeFolder.startsWith(`${path}/`));
        if (hasActiveFolder) return true;
        const hasActiveTrigger = activeTriggerFolder && (activeTriggerFolder === path || activeTriggerFolder.startsWith(`${path}/`));
        return hasActiveTrigger;
    }

    function renderTriggerSidebarTreeNodes(node, level, activeFolder, activeTrigger) {
        const childEntries = node && node.children instanceof Map ? Array.from(node.children.entries()) : [];
        const triggerEntries = Array.isArray(node?.triggers) ? node.triggers.slice() : [];

        if (!childEntries.length && !triggerEntries.length) {
            return '';
        }

        childEntries.sort((a, b) => (a[0] || '').localeCompare(b[0] || '', undefined, { sensitivity: 'base' }));
        triggerEntries.sort((a, b) => (a.name || a.slug || '').localeCompare(b.name || b.slug || '', undefined, { sensitivity: 'base' }));

        let html = `<ul class="${level > 0 ? 'pl-4' : ''} space-y-1">`;
        const activeTriggerFolder = getFolderKeyForSlug(activeTrigger || '');

        childEntries.forEach(([segment, childNode]) => {
            const folderPath = (childNode && childNode.key) || segment || '';
            const isExpanded = shouldExpandTriggerFolder(folderPath, activeFolder, activeTriggerFolder);
            if (isExpanded) ensureTriggerSidebarExpansionForPath(folderPath);
            const isActiveFolder = !!folderPath && folderPath === activeFolder;
            const folderLabel = formatTriggerFolderLabel(childNode?.label || segment || 'Folder');
            const childrenHtml = renderTriggerSidebarTreeNodes(childNode, level + 1, activeFolder, activeTrigger);

            html += `
                <li data-trigger-folder-node="${escapeAttribute(folderPath)}">
                    <div class="flex items-center justify-between p-1 text-[var(--text-primary)] rounded-md pipeline-sidebar-folder-row ${isActiveFolder ? 'bg-[var(--bg-tertiary)]' : ''} hover:bg-[var(--bg-tertiary)]">
                        <div class="flex items-center flex-grow min-w-0">
                            <button type="button" class="sidebar-toggle-btn flex items-center justify-center h-5 w-5 rounded mr-1 text-[var(--text-secondary)] hover:bg-[var(--bg-hover)]" data-trigger-toggle-folder="${escapeAttribute(folderPath)}" aria-expanded="${isExpanded ? 'true' : 'false'}" aria-label="${escapeAttribute((isExpanded ? 'Collapse' : 'Expand') + ' ' + folderLabel)}">
                                <svg class="h-4 w-4 chevron ${isExpanded ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>
                            </button>
                            <button type="button" class="pipeline-sidebar-folder flex items-center gap-2 flex-grow text-left min-w-0 p-1 rounded hover:bg-[var(--bg-hover)]" data-trigger-open-folder="${escapeAttribute(folderPath)}">
                                <svg class="h-4 w-4 text-[var(--text-secondary)] flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>
                                <span class="truncate">${escapeHtml(folderLabel)}</span>
                            </button>
                        </div>
                    </div>
                    <div class="pipeline-sidebar-children ${isExpanded ? '' : 'hidden'}" data-trigger-folder-children="${escapeAttribute(folderPath)}">
                        ${childrenHtml}
                    </div>
                </li>`;
        });

        triggerEntries.forEach(entry => {
            const slug = entry.slug;
            if (!slug) return;
            const triggerName = entry.name || slug.split('/').pop() || slug;
            const isActive = state.selectedSlug === slug;
            const triggerHref = buildTriggerHash(slug);
            html += `
                <li data-trigger-sidebar-item="${escapeAttribute(slug)}">
                    <a href="${triggerHref}" class="sidebar-link flex items-center p-2 text-[var(--text-primary)] rounded-md transition-colors duration-200 ${isActive ? 'active' : ''}" data-trigger-sidebar-slug="${escapeAttribute(slug)}">
                        <svg class="h-4 w-4 mr-2 text-[var(--text-secondary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 3L6 14h6l-2 7 9-13h-6z"/></svg>
                        <span class="truncate">${escapeHtml(triggerName)}</span>
                    </a>
                </li>`;
        });

        html += '</ul>';
        return html;
    }

    function renderTriggerSidebarTree(container) {
        if (!container) return;
        if (!state.triggerTree) {
            state.triggerTree = buildTriggerTree(state.triggers || []);
        }
        if (!(state.sidebarExpanded instanceof Set)) {
            state.sidebarExpanded = new Set();
        }

        const activeFolder = getActiveTriggerSidebarFolder();
        const activeTrigger = state.selectedSlug || '';
        ensureTriggerSidebarExpansionForPath(activeFolder);
        ensureTriggerSidebarExpansionForPath(getFolderKeyForSlug(activeTrigger));

        const treeHtml = renderTriggerSidebarTreeNodes(state.triggerTree, 0, activeFolder, activeTrigger);
        container.innerHTML = treeHtml || `<p class="px-2 text-sm text-[var(--text-secondary)]">No triggers available.</p>`;

        if (!container.dataset.triggerSidebarBound) {
            container.addEventListener('click', handleTriggerSidebarClick);
            container.dataset.triggerSidebarBound = 'true';
        }

        updateSidebarHighlight();
    }

    function handleTriggerSidebarClick(event) {
        const toggleBtn = event.target.closest('[data-trigger-toggle-folder]');
        if (toggleBtn) {
            event.preventDefault();
            event.stopPropagation();
            const folderPath = toggleBtn.dataset.triggerToggleFolder || '';
            if (!(state.sidebarExpanded instanceof Set)) {
                state.sidebarExpanded = new Set();
            }
            if (state.sidebarExpanded.has(folderPath)) {
                state.sidebarExpanded.delete(folderPath);
            } else if (folderPath) {
                state.sidebarExpanded.add(folderPath);
            }
            const container = document.getElementById('triggers-sidebar-tree');
            if (container) {
                renderTriggerSidebarTree(container);
            }
            return;
        }

        const folderBtn = event.target.closest('[data-trigger-open-folder]');
        if (folderBtn) {
            event.preventDefault();
            event.stopPropagation();
            const folderPath = folderBtn.dataset.triggerOpenFolder || '';
            if (state.isEditing) {
                const proceed = confirm('Discard unsaved changes?');
                if (!proceed) {
                    return;
                }
                exitEditMode(true, { updateHash: false });
            }
            const hash = buildTriggerFolderHash(folderPath);
            if (window.location.hash !== hash) {
                window.location.hash = hash;
            } else {
                handleRoute(hash);
            }
            return;
        }

        const triggerLink = event.target.closest('[data-trigger-sidebar-slug]');
        if (triggerLink) {
            const slug = triggerLink.dataset.triggerSidebarSlug || '';
            if (!slug) return;
            if (state.isEditing && slug !== state.selectedSlug) {
                const proceed = confirm('Discard unsaved changes?');
                if (!proceed) {
                    event.preventDefault();
                    event.stopPropagation();
                    return;
                }
                exitEditMode(true, { updateHash: false });
            }
            event.preventDefault();
            event.stopPropagation();
            navigateToSlug(slug);
        }
    }

    function normalizeTriggerEventKey(value) {
        return String(value ?? '').trim().toLowerCase().replace(/\s+/g, '_');
    }

    function canonicalizeTriggerEvent(value) {
        const key = normalizeTriggerEventKey(value);
        return VALID_TRIGGER_EVENTS.get(key) || null;
    }

    function init(ctx = {}) {
        if (initialized) return;
        context = ctx;
        cacheDom();
        bindEvents();
        updateNewTriggerBlueprint();
        loadTriggers(true).catch(err => console.error('Failed to load triggers:', err));
        initialized = true;
    }

    function cacheDom() {
        const ids = [
            'triggers-search-container', 'triggers-search', 'triggers-clear-search', 'triggers-new-btn',
            'triggers-list', 'triggers-list-empty', 'triggers-detail', 'triggers-detail-name',
            'triggers-detail-source', 'triggers-detail-meta', 'triggers-meta-chips', 'triggers-yaml-content',
            'triggers-editor-wrapper', 'triggers-editor-container', 'triggers-line-numbers', 'triggers-yaml-stage', 'triggers-yaml-highlight', 'triggers-yaml-editor',
            'triggers-validation-status', 'triggers-edit-btn', 'triggers-copy-btn', 'triggers-download-btn',
            'triggers-clone-btn', 'triggers-view-actions', 'triggers-edit-actions',
            'triggers-save-btn', 'triggers-cancel-btn', 'triggers-pipelines-empty', 'triggers-pipelines-list',
            'triggers-runs-empty', 'triggers-runs-list',
            'triggers-new-modal', 'triggers-new-form', 'triggers-new-cancel', 'triggers-new-close',
            'triggers-new-repo', 'triggers-new-yaml',
            'triggers-delete-modal', 'triggers-delete-message', 'triggers-delete-cancel', 'triggers-delete-confirm', 'triggers-delete-close',
            'triggers-clone-modal', 'triggers-clone-form', 'triggers-clone-cancel', 'triggers-clone-close', 'triggers-clone-repo', 'triggers-clone-subtitle',
            'triggers-list-view', 'triggers-detail-view', 'triggers-back-btn'
        ];

        ids.forEach(id => {
            DOM[id] = document.getElementById(id);
        });
    }

    function rebuildTriggerSources(slugs, sourceOverrides) {
        const previous = state.triggerSources instanceof Map ? state.triggerSources : new Map();
        const overrides = sourceOverrides instanceof Map ? sourceOverrides : new Map();
        const map = new Map();
        if (Array.isArray(slugs)) {
            slugs.forEach(slug => {
                const normalizedSlug = String(slug || '').trim();
                if (!normalizedSlug) return;
                let source = overrides.get(normalizedSlug) || previous.get(normalizedSlug) || 'database';
                source = normalizePipelineSourceKey(source) || 'database';
                map.set(normalizedSlug, source);
            });
        }
        state.triggerSources = map;
    }

    function getTriggerSourceKey(slug) {
        if (!slug) return '';
        if (!(state.triggerSources instanceof Map)) return '';
        const raw = state.triggerSources.get(slug);
        if (!raw) return '';
        return String(raw).trim().toLowerCase();
    }

    function getTriggerSourceLabel(key) {
        switch (String(key || '').trim().toLowerCase()) {
            case 'git':
                return 'Git';
            case 'draft':
                return 'Draft';
            case 'local':
                return 'Local';
            case 'database':
            default:
                return 'Database';
        }
    }

    function resolveTriggerSource(slug) {
        const key = getTriggerSourceKey(slug);
        if (key) {
            return getTriggerSourceLabel(key);
        }
        if (Array.isArray(state.triggers) && state.triggers.includes(slug)) {
            return getTriggerSourceLabel('database');
        }
        return getTriggerSourceLabel('git');
    }

    function isTriggerGitManaged(slug) {
        return getTriggerSourceKey(slug) === 'git';
    }

    function normalizePipelineSourceKey(value) {
        if (value == null) return '';
        const key = String(value).trim().toLowerCase();
        if (!key) return '';
        if (key.includes('git') || key.includes('config repository') || key === 'repository') return 'git';
        if (key.includes('draft')) return 'draft';
        if (key.includes('local') || key.includes('repo file') || key.includes('repository file')) return 'local';
        if (key.includes('database') || key === 'db') return 'database';
        return key;
    }

    async function ensurePipelineSourceIndex() {
        if (state.pipelineSourceIndex instanceof Map) {
            return state.pipelineSourceIndex;
        }
        if (state.pipelineSourceIndexPromise) {
            return state.pipelineSourceIndexPromise;
        }
        const promise = (async () => {
            const map = new Map();
            try {
                if (!context || typeof context.fetchData !== 'function') {
                    state.pipelineSourceIndex = map;
                    return map;
                }
                const response = await context.fetchData('/v1/pipelines?include_source=true');
                if (Array.isArray(response)) {
                    response.forEach(item => {
                        if (!item || typeof item !== 'object') return;
                        const id = normalizePipelineIdentifier(item.id || item.ID || item.identifier || item.pipeline || '');
                        if (!id) return;
                        const key = normalizePipelineSourceKey(item.source);
                        if (key) {
                            map.set(id, key);
                        }
                    });
                }
                state.pipelineSourceIndex = map;
                return map;
            } catch (error) {
                console.error('Failed to build pipeline source index:', error);
                state.pipelineSourceIndex = map;
                return map;
            } finally {
                state.pipelineSourceIndexPromise = null;
            }
        })();
        state.pipelineSourceIndexPromise = promise;
        return promise;
    }

    function extractPipelineVersion(yamlText) {
        if (!yamlText || !window.jsyaml) return 'latest';
        try {
            const parsed = window.jsyaml.load(yamlText);
            if (parsed && typeof parsed === 'object' && parsed.version) {
                const version = String(parsed.version).trim();
                return version || 'latest';
            }
        } catch (error) {
            console.warn('Failed to parse pipeline YAML for version:', error);
        }
        return 'latest';
    }

    async function ensurePipelineMeta(identifier) {
        const id = normalizePipelineIdentifier(identifier);
        if (!id) {
            return { version: 'latest', source: getTriggerSourceLabel('database'), sourceKey: 'database' };
        }

        if (state.pipelineMetaCache.has(id)) {
            return state.pipelineMetaCache.get(id);
        }

        if (state.pipelineMetaPromises.has(id)) {
            return state.pipelineMetaPromises.get(id);
        }

        const promise = (async () => {
            await ensurePipelineSourceIndex();
            let sourceKey = '';
            if (state.pipelineSourceIndex instanceof Map) {
                sourceKey = state.pipelineSourceIndex.get(id) || '';
            }
            if (!sourceKey) {
                sourceKey = 'local';
                if (state.pipelineSourceIndex instanceof Map && !state.pipelineSourceIndex.has(id)) {
                    state.pipelineSourceIndex.set(id, sourceKey);
                }
            }
            const normalizedSourceKey = normalizePipelineSourceKey(sourceKey) || 'database';
            const sourceLabel = getTriggerSourceLabel(normalizedSourceKey) || getTriggerSourceLabel('database');

            let version = 'latest';
            if (normalizedSourceKey === 'database' && context && typeof context.fetchData === 'function') {
                const encodedId = id.split('/').map(encodeURIComponent).join('/');
                const yaml = await context.fetchData(`/v1/pipelines/${encodedId}`);
                if (typeof yaml === 'string') {
                    version = extractPipelineVersion(yaml);
                }
            }

            const meta = { version, source: sourceLabel, sourceKey: normalizedSourceKey };
            state.pipelineMetaCache.set(id, meta);
            state.pipelineMetaPromises.delete(id);
            return meta;
        })();

        state.pipelineMetaPromises.set(id, promise);
        return promise;
    }

    function bindEvents() {
        if (DOM['triggers-list']) {
            DOM['triggers-list'].addEventListener('click', handleListClick);
            DOM['triggers-list'].addEventListener('keydown', handleListKeydown);
        }

        if (DOM['triggers-back-btn']) {
            DOM['triggers-back-btn'].addEventListener('click', () => {
                if (state.isEditing) {
                    const proceed = confirm('Discard unsaved changes?');
                    if (!proceed) return;
                    exitEditMode(true, { updateHash: false });
                }
                renderDetailEmpty();
                const hash = buildTriggerFolderHash(state.activeFolderKey || '');
                try {
                    history.replaceState(null, '', hash);
                } catch {
                    window.location.hash = hash;
                }
            });
        }

        if (DOM['triggers-search']) {
            DOM['triggers-search'].addEventListener('input', handleSearchInput);
        }

        if (DOM['triggers-clear-search']) {
            DOM['triggers-clear-search'].addEventListener('click', () => {
                if (DOM['triggers-search']) {
                    DOM['triggers-search'].value = '';
                }
                state.searchTerm = '';
                applySearch();
            });
        }

        if (DOM['triggers-new-btn']) {
            DOM['triggers-new-btn'].addEventListener('click', () => {
                const prefix = getActiveTriggerFolderPrefix();
                openCreateTriggerModal({ repository: prefix });
            });
        }

        if (DOM['triggers-new-repo']) {
            DOM['triggers-new-repo'].addEventListener('input', updateNewTriggerBlueprint);
        }

        if (DOM['triggers-new-cancel']) {
            DOM['triggers-new-cancel'].addEventListener('click', () => closeModal('triggers-new-modal'));
        }

        if (DOM['triggers-new-close']) {
            DOM['triggers-new-close'].addEventListener('click', () => closeModal('triggers-new-modal'));
        }

        if (DOM['triggers-new-form']) {
            DOM['triggers-new-form'].addEventListener('submit', handleCreateTriggerSubmit);
        }

        if (DOM['triggers-delete-cancel']) {
            DOM['triggers-delete-cancel'].addEventListener('click', () => closeModal('triggers-delete-modal'));
        }

        if (DOM['triggers-delete-close']) {
            DOM['triggers-delete-close'].addEventListener('click', () => closeModal('triggers-delete-modal'));
        }

        if (DOM['triggers-delete-confirm']) {
            DOM['triggers-delete-confirm'].addEventListener('click', handleDeleteConfirm);
        }

        if (DOM['triggers-copy-btn']) {
            DOM['triggers-copy-btn'].addEventListener('click', copyTriggerYaml);
        }

        if (DOM['triggers-download-btn']) {
            DOM['triggers-download-btn'].addEventListener('click', downloadTriggerYaml);
        }

        if (DOM['triggers-clone-btn']) {
            DOM['triggers-clone-btn'].addEventListener('click', handleCloneButtonClick);
        }

        if (DOM['triggers-clone-cancel']) {
            DOM['triggers-clone-cancel'].addEventListener('click', () => closeModal('triggers-clone-modal'));
        }

        if (DOM['triggers-clone-close']) {
            DOM['triggers-clone-close'].addEventListener('click', () => closeModal('triggers-clone-modal'));
        }

        if (DOM['triggers-clone-form']) {
            DOM['triggers-clone-form'].addEventListener('submit', handleCloneSubmit);
        }

        if (DOM['triggers-edit-btn']) {
            DOM['triggers-edit-btn'].addEventListener('click', () => enterEditMode());
        }

        if (DOM['triggers-cancel-btn']) {
            DOM['triggers-cancel-btn'].addEventListener('click', () => exitEditMode(true));
        }

        if (DOM['triggers-save-btn']) {
            DOM['triggers-save-btn'].addEventListener('click', handleSaveTrigger);
        }

        if (DOM['triggers-yaml-editor']) {
            const editor = DOM['triggers-yaml-editor'];
            editor.addEventListener('input', handleEditorInput);
            editor.addEventListener('scroll', () => {
                syncEditorScroll();
                updateTriggerInlineSuggestionPosition();
            });
            editor.addEventListener('click', () => {
                updateTriggerEditorSuggestions();
            });
            editor.addEventListener('keyup', (event) => {
                if (event.key === 'Shift' || event.key === 'Control' || event.key === 'Alt' || event.key === 'Meta') {
                    return;
                }
                updateTriggerEditorSuggestions();
            });
            editor.addEventListener('keydown', handleTriggerEditorKeydown);
        }

        document.addEventListener('keydown', (event) => {
            if (event.key === 'Escape') {
                closeModal('triggers-new-modal');
                closeModal('triggers-delete-modal');
                closeModal('triggers-clone-modal');
            }
        });

        updateTriggerNewButtonVisibility();
    }

    async function loadTriggers(force = false) {
        if (!context || typeof context.fetchData !== 'function') return;

        if (!force && Array.isArray(state.triggers) && state.triggers.length) {
            if (!state.triggerTree) {
                state.triggerTree = buildTriggerTree(state.triggers);
            }
            ensureActiveFolderKey();
            applySearch();
            renderTriggerCollection();
            refreshSidebarListFromState();
            return;
        }

        try {
            const response = await context.fetchData('/v1/overrides?include_source=true');
            if (Array.isArray(response)) {
                const { slugs, sourceMap } = parseTriggerOverrideList(response);
                state.triggers = slugs
                    .map(slug => String(slug || '').trim())
                    .filter(Boolean)
                    .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
                state.triggerTree = buildTriggerTree(state.triggers);
                rebuildTriggerSources(state.triggers, sourceMap);
                ensureActiveFolderKey();
                if (!state.triggers.includes(state.selectedSlug)) {
                    state.selectedSlug = null;
                    renderDetailEmpty();
                }
            } else {
                state.triggers = [];
                state.triggerTree = buildTriggerTree(state.triggers);
                rebuildTriggerSources(state.triggers);
                state.selectedSlug = null;
                renderDetailEmpty();
            }
            applySearch();
            renderTriggerCollection();
            refreshSidebarListFromState();
        } catch (error) {
            console.error('Unable to fetch triggers:', error);
            showToast('Failed to load triggers.', 'error');
            if (!state.triggerTree) {
                state.triggerTree = buildTriggerTree(state.triggers || []);
            }
            ensureActiveFolderKey();
            renderTriggerCollection();
            refreshSidebarListFromState();
        }
    }

    async function ensureTriggerLoaded(slug, options = {}) {
        if (state.triggerCache.has(slug)) {
            return state.triggerCache.get(slug);
        }

        try {
            const [owner, name] = splitSlug(slug);
            const yaml = await context.fetchData(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`);
            if (typeof yaml !== 'string') {
                throw new Error('Invalid response when loading trigger definition');
            }
            const parsed = parseTriggerYaml(yaml);
            const summary = buildTriggerSummary(parsed);
            const info = { yaml, manifest: parsed, summary, fetchedAt: Date.now() };
            state.triggerCache.set(slug, info);
            return info;
        } catch (error) {
            console.error(`Failed to load trigger ${slug}:`, error);
            if (!options.silent) {
                showToast(`Failed to load trigger ${slug}.`, 'error');
            }
            return null;
        }
    }

    function applySearch() {
        if (!Array.isArray(state.triggers)) {
            state.filteredSlugs = [];
        } else {
            const term = (state.searchTerm || '').trim().toLowerCase();
            if (!term) {
                state.filteredSlugs = state.triggers.slice();
            } else {
                state.filteredSlugs = state.triggers.filter(slug => slug.toLowerCase().includes(term));
            }
        }
        renderTriggerCollection();
        toggleSearchClearButton();
    }

    function toggleSearchClearButton() {
        if (!DOM['triggers-clear-search']) return;
        const hasTerm = !!(state.searchTerm && state.searchTerm.trim());
        DOM['triggers-clear-search'].classList.toggle('hidden', !hasTerm);
    }

    function renderTriggerCollection() {
        const container = DOM['triggers-list'];
        if (!container) return;

        const searchTerm = (state.searchTerm || '').trim();
        const isSearching = !!searchTerm;
        const hasSelection = !!state.selectedSlug;

        if (isSearching) {
            showDetailView(false);
        } else if (hasSelection) {
            showDetailView(true);
        } else {
            showDetailView(false);
        }

        let html = '';
        let showEmpty = false;

        if (isSearching) {
            const results = state.filteredSlugs || [];
            if (results.length) {
                html += renderSearchSummary(results.length, searchTerm);
                html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${results.map(renderTriggerCard).join('')}</div>`;
            } else {
                showEmpty = true;
            }
        } else {
            if (!state.triggerTree) {
                state.triggerTree = buildTriggerTree(state.triggers || []);
            }
            ensureActiveFolderKey();
            const tree = state.triggerTree;
            const activeNode = getFolderNodeByKey(tree, state.activeFolderKey) || tree;

            if (activeNode?.children && activeNode.children.size) {
                const folders = Array.from(activeNode.children.values())
                    .sort((a, b) => a.label.localeCompare(b.label, undefined, { sensitivity: 'base' }))
                    .map(renderFolderCard);
                if (folders.length) {
                    html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${folders.join('')}</div>`;
                }
            }

            const triggerEntries = Array.isArray(activeNode?.triggers) ? activeNode.triggers.slice() : [];
            if (triggerEntries.length) {
                triggerEntries.sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));
            }
            const triggerCards = triggerEntries.map(entry => renderTriggerCard(entry.slug));
            if (triggerCards.length) {
                html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${triggerCards.join('')}</div>`;
            }

            const hasFolders = !!(activeNode?.children && activeNode.children.size);
            const hasRealTriggers = triggerEntries.length > 0;
            if (!hasFolders && !hasRealTriggers) {
                showEmpty = true;
            }
        }

        container.innerHTML = html;
        if (DOM['triggers-list-empty']) {
            if (showEmpty) {
                if (isSearching && searchTerm) {
                    DOM['triggers-list-empty'].innerHTML = `<p class="text-sm">No triggers matched "${escapeHtml(searchTerm)}".</p>`;
                } else if (state.activeFolderKey) {
                    DOM['triggers-list-empty'].innerHTML = '<p class="text-sm">No triggers in this folder yet.</p>';
                } else {
                    DOM['triggers-list-empty'].innerHTML = '<p class="text-sm">No triggers found. Sync your configuration repository to import triggers.</p>';
                }
            }
            DOM['triggers-list-empty'].classList.toggle('hidden', !showEmpty);
        }
        highlightActiveListItem();
        updateTriggerNewButtonVisibility({ isSearching });
    }

    function showDetailView(show) {
        if (DOM['triggers-list-view']) {
            DOM['triggers-list-view'].classList.toggle('hidden', show);
        }
        if (DOM['triggers-detail-view']) {
            DOM['triggers-detail-view'].classList.toggle('hidden', !show);
        }
        if (DOM['triggers-back-btn']) {
            DOM['triggers-back-btn'].classList.toggle('hidden', !show);
        }
        if (DOM['triggers-search-container']) {
            DOM['triggers-search-container'].classList.toggle('hidden', show);
        }
        updateTriggerNewButtonVisibility({ showDetail: show });
    }

    function isTriggerDetailVisible() {
        if (DOM['triggers-detail-view']) {
            return !DOM['triggers-detail-view'].classList.contains('hidden');
        }
        return !!state.selectedSlug;
    }

    function updateTriggerNewButtonVisibility(options = {}) {
        const button = DOM['triggers-new-btn'];
        if (!button) return;
        const isSearching = Object.prototype.hasOwnProperty.call(options, 'isSearching')
            ? !!options.isSearching
            : !!(state.searchTerm || '').trim();
        const showDetail = Object.prototype.hasOwnProperty.call(options, 'showDetail')
            ? !!options.showDetail
            : isTriggerDetailVisible();
        button.classList.toggle('hidden', isSearching || showDetail);
    }

    function renderSearchSummary(count, term) {
        const safeTerm = escapeHtml(term);
        return `<div class="triggers-search-summary">Showing ${count} result${count === 1 ? '' : 's'} for "${safeTerm}"</div>`;
    }

    function buildBreadcrumbs(key) {
        const crumbs = [{ label: 'All triggers', key: '' }];
        if (!key) return crumbs;
        const segments = key.split('/').filter(Boolean);
        let path = '';
        segments.forEach(segment => {
            path = path ? `${path}/${segment}` : segment;
            crumbs.push({ label: segment, key: path });
        });
        return crumbs;
    }

    function getParentFolderKey(key) {
        if (!key) return '';
        const segments = key.split('/').filter(Boolean);
        segments.pop();
        return segments.join('/');
    }

    function renderFolderCard(node) {
        const totalTriggers = countTriggersRecursive(node);
        const childCount = node.children ? node.children.size : 0;
        const label = node.label || node.key || 'Folder';
        const keyAttr = escapeAttribute(node.key || '');
        const labelDisplay = escapeHtml(label);

        return `
            <article class="pipeline-folder-card border border-[var(--border-primary)]" data-trigger-folder="${keyAttr}" tabindex="0" role="button" aria-label="Open folder ${escapeAttribute(label)}">
                <div class="pipeline-folder-card-header">
                    <span class="pipeline-folder-icon">
                        <svg class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7h5l2 2h9a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V9a2 2 0 012-2z"/></svg>
                    </span>
                    <h3 class="pipeline-folder-title">${labelDisplay}</h3>
                    <div class="pipeline-folder-actions">
                        <span class="pipeline-folder-chevron" aria-hidden="true"><svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 5l7 7-7 7"/></svg></span>
                    </div>
                </div>
                <div class="pipeline-folder-meta">
                    <div class="pipeline-folder-meta-row"><span class="pipeline-folder-meta-label">Triggers:</span><span class="pipeline-folder-meta-value">${totalTriggers}</span></div>
                    <div class="pipeline-folder-meta-row"><span class="pipeline-folder-meta-label">Sub folders:</span><span class="pipeline-folder-meta-value">${childCount}</span></div>
                </div>
            </article>`;
    }

    function renderTriggerCard(slug) {
        const info = state.triggerCache.get(slug);
        if (!info) {
            prefetchTriggerSummary(slug);
        }
        const sourceLabel = resolveTriggerSource(slug);
        const isGitManaged = isTriggerGitManaged(slug);
        const parts = slug.split('/').filter(Boolean);
        const name = parts.pop() || slug;
        const owner = parts.join('/') || 'root';
        const isActive = slug === state.selectedSlug;
        const deleteTitle = isGitManaged
            ? 'This trigger is managed via Git. Clone it to customize.'
            : 'Delete trigger';
        const deleteButton = isGitManaged
            ? `<button class="pipelines-delete-button" type="button" disabled aria-disabled="true" title="${escapeAttribute(deleteTitle)}">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                                <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                                <path d="M4 7h16" />
                            </svg>
                        </button>`
            : `<button class="pipelines-delete-button" type="button" data-trigger-delete="${escapeAttribute(slug)}" title="${escapeAttribute(deleteTitle)}">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                                <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                                <path d="M4 7h16" />
                            </svg>
                        </button>`;

        return `
            <article class="pipeline-card triggers-card bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded-lg shadow-sm transition-all duration-200 p-3 flex flex-col${isActive ? ' triggers-card--active' : ''}" data-trigger-slug="${escapeAttribute(slug)}" tabindex="0" role="button" aria-label="Open trigger ${escapeAttribute(slug)}">
                <div class="pipeline-card-header flex items-start justify-between gap-3">
                    <div class="pipeline-card-info">
                        <span class="triggers-card-icon" aria-hidden="true">
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M13 3L6 14h6l-2 7 9-13h-6z" />
                            </svg>
                        </span>
                        <div class="pipeline-card-text min-w-0">
                            <h3 class="pipeline-card-title">${escapeHtml(name)}</h3>
                            <p class="pipeline-card-path">${escapeHtml(owner)}</p>
                        </div>
                    </div>
                    <div class="pipeline-card-actions">
                        ${deleteButton}
                    </div>
                </div>
                <div class="pipeline-card-meta">
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">Source</span>
                        <span class="pipeline-card-meta-value">${escapeHtml(sourceLabel)}</span>
                    </div>
                </div>
            </article>`;
    }

    function highlightActiveListItem() {
        if (!DOM['triggers-list']) return;
        DOM['triggers-list'].querySelectorAll('[data-trigger-slug]').forEach(card => {
            const slug = card.getAttribute('data-trigger-slug');
            card.classList.toggle('triggers-card--active', slug === state.selectedSlug);
        });
        updateSidebarHighlight();
    }

    function handleListClick(event) {
        const deleteButton = event.target.closest('[data-trigger-delete]');
        if (deleteButton) {
            const slug = deleteButton.getAttribute('data-trigger-delete') || '';
            if (slug) {
                promptTriggerDelete(slug);
            }
            event.preventDefault();
            event.stopPropagation();
            return;
        }

        const folderNav = event.target.closest('[data-trigger-folder-nav]');
        if (folderNav) {
            if (state.isEditing) {
                const proceed = confirm('Discard unsaved changes?');
                if (!proceed) return;
            }
            const key = folderNav.getAttribute('data-trigger-folder-nav') || '';
            renderDetailEmpty();
            const hash = buildTriggerFolderHash(key);
            if (window.location.hash !== hash) {
                window.location.hash = hash;
            } else {
                handleRoute(hash);
            }
            event.preventDefault();
            event.stopPropagation();
            return;
        }

        const folderCard = event.target.closest('[data-trigger-folder]');
        if (folderCard) {
            if (state.isEditing) {
                const proceed = confirm('Discard unsaved changes?');
                if (!proceed) return;
            }
            const key = folderCard.getAttribute('data-trigger-folder') || '';
            renderDetailEmpty();
            const hash = buildTriggerFolderHash(key);
            if (window.location.hash !== hash) {
                window.location.hash = hash;
            } else {
                handleRoute(hash);
            }
            event.preventDefault();
            event.stopPropagation();
            return;
        }

        const triggerCard = event.target.closest('[data-trigger-slug]');
        if (!triggerCard) return;

        const slug = triggerCard.getAttribute('data-trigger-slug');
        if (!slug) return;

        if (state.isEditing && slug !== state.selectedSlug) {
            const proceed = confirm('Discard unsaved changes?');
            if (!proceed) {
                event.preventDefault();
                event.stopPropagation();
                return;
            }
            exitEditMode(true, { updateHash: false });
        }

        navigateToSlug(slug);
        event.preventDefault();
        event.stopPropagation();
    }

    function handleListKeydown(event) {
        if (event.defaultPrevented) return;
        if (event.key !== 'Enter' && event.key !== ' ' && event.key !== 'Spacebar') return;

        const targetNav = event.target.closest('[data-trigger-folder-nav]');
        if (targetNav && targetNav === document.activeElement) {
            event.preventDefault();
            if (state.isEditing) {
                const proceed = confirm('Discard unsaved changes?');
                if (!proceed) return;
            }
            const key = targetNav.getAttribute('data-trigger-folder-nav') || '';
            renderDetailEmpty();
            const hash = buildTriggerFolderHash(key);
            if (window.location.hash !== hash) {
                window.location.hash = hash;
            } else {
                handleRoute(hash);
            }
            focusFirstTriggerCard();
            return;
        }

        const targetFolder = event.target.closest('[data-trigger-folder]');
        if (targetFolder && targetFolder === document.activeElement) {
            event.preventDefault();
            if (state.isEditing) {
                const proceed = confirm('Discard unsaved changes?');
                if (!proceed) return;
                exitEditMode(true, { updateHash: false });
            }
            const key = targetFolder.getAttribute('data-trigger-folder') || '';
            renderDetailEmpty();
            const hash = buildTriggerFolderHash(key);
            if (window.location.hash !== hash) {
                window.location.hash = hash;
            } else {
                handleRoute(hash);
            }
            focusFirstTriggerCard();
            return;
        }

        const triggerCard = event.target.closest('[data-trigger-slug]');
        if (triggerCard && triggerCard === document.activeElement) {
            event.preventDefault();
            const slug = triggerCard.getAttribute('data-trigger-slug');
            if (!slug) return;
            if (state.isEditing && slug !== state.selectedSlug) {
                const proceed = confirm('Discard unsaved changes?');
                if (!proceed) return;
                exitEditMode(true, { updateHash: false });
            }
            navigateToSlug(slug);
        }
    }

    function focusFirstTriggerCard() {
        const list = DOM['triggers-list'];
        if (!list) return;
        const first = list.querySelector('[data-trigger-folder], [data-trigger-slug]');
        if (first && typeof first.focus === 'function') {
            first.focus();
        }
    }

    function setActiveFolder(key, options = {}) {
        const normalized = normalizeFolderKey(key);
        if (!state.triggerTree) {
            state.triggerTree = buildTriggerTree(state.triggers || []);
        }
        const node = getFolderNodeByKey(state.triggerTree, normalized);
        state.activeFolderKey = node ? normalized : '';
        ensureTriggerSidebarExpansionForPath(state.activeFolderKey);
        if (options.render !== false) {
            renderTriggerCollection();
        }
        refreshSidebarListFromState();
        if (options.updateHash !== false) {
            const hash = buildTriggerFolderHash(state.activeFolderKey);
            if (window.location.hash !== hash) {
                window.location.hash = hash;
            }
        }
    }

    function ensureActiveFolderKey() {
        if (!state.triggerTree) {
            state.activeFolderKey = '';
            return;
        }
        state.activeFolderKey = normalizeFolderKey(state.activeFolderKey);
        if (!state.activeFolderKey) return;
        if (!getFolderNodeByKey(state.triggerTree, state.activeFolderKey)) {
            state.activeFolderKey = '';
        }
    }

    function getFolderNodeByKey(tree, key) {
        if (!tree) return null;
        if (!key) return tree;
        const segments = key.split('/').filter(Boolean);
        let node = tree;
        for (const segment of segments) {
            if (!node.children || !node.children.has(segment)) {
                return null;
            }
            node = node.children.get(segment);
        }
        return node;
    }

    function normalizeFolderKey(key) {
        if (!key) return '';
        return key
            .split('/')
            .filter(Boolean)
            .join('/');
    }

    function countTriggersRecursive(node) {
        if (!node) return 0;
        const own = Array.isArray(node.triggers) ? node.triggers.length : 0;
        if (!node.children || !node.children.size) return own;
        let total = own;
        node.children.forEach(child => {
            total += countTriggersRecursive(child);
        });
        return total;
    }

    function buildTriggerTree(slugs) {
        const root = { key: '', label: '', children: new Map(), triggers: [] };
        if (!Array.isArray(slugs)) {
            return root;
        }

        slugs.forEach(slug => {
            const parts = String(slug || '').split('/').filter(Boolean);
            if (!parts.length) return;
            const triggerName = parts.pop();
            let node = root;
            let path = '';
            parts.forEach(part => {
                path = path ? `${path}/${part}` : part;
                if (!node.children) {
                    node.children = new Map();
                }
                if (!node.children.has(part)) {
                    node.children.set(part, { key: path, label: part, children: new Map(), triggers: [] });
                }
                node = node.children.get(part);
            });
            if (!node.triggers) {
                node.triggers = [];
            }
            node.triggers.push({ slug, name: triggerName });
        });

        return root;
    }

    function getFolderKeyForSlug(slug) {
        const parts = String(slug || '').split('/').filter(Boolean);
        parts.pop();
        return parts.join('/');
    }

    function prefetchTriggerSummary(slug) {
        if (!state || !context) return;
        if (state.triggerCache.has(slug)) return;
        if (!(state._prefetching instanceof Set)) {
            state._prefetching = new Set();
        }
        if (state._prefetching.has(slug)) return;
        state._prefetching.add(slug);
        ensureTriggerLoaded(slug, { silent: true })
            .then(info => {
                state._prefetching.delete(slug);
                if (info) {
                    renderTriggerCollection();
                }
            })
            .catch(() => {
                state._prefetching.delete(slug);
            });
    }

    function navigateToSlug(slug, mode = 'view') {
        const targetHash = buildTriggerHash(slug, mode);
        if (window.location.hash !== targetHash) {
            window.location.hash = targetHash;
        } else {
            handleRoute(window.location.hash);
        }
    }

    function handleSearchInput(event) {
        const value = event.target.value || '';
        state.searchTerm = value;
        applySearch();
    }

    function renderDetailEmpty() {
        if (state.isEditing) {
            exitEditMode(true, { silent: true, updateHash: false });
        }
        state.isEditing = false;
        state.selectedSlug = null;
        state.currentYaml = '';
        showDetailView(false);
        if (DOM['triggers-detail']) DOM['triggers-detail'].classList.add('hidden');
        if (DOM['triggers-view-actions']) DOM['triggers-view-actions'].classList.remove('hidden');
        if (DOM['triggers-edit-actions']) DOM['triggers-edit-actions'].classList.add('hidden');
        if (DOM['triggers-editor-container']) DOM['triggers-editor-container'].classList.add('hidden');
        if (DOM['triggers-yaml-content']) DOM['triggers-yaml-content'].classList.remove('hidden');
        if (DOM['triggers-detail-name']) DOM['triggers-detail-name'].textContent = '';
        if (DOM['triggers-detail-source']) DOM['triggers-detail-source'].textContent = '';
        if (DOM['triggers-detail-meta']) DOM['triggers-detail-meta'].innerHTML = '';
        if (DOM['triggers-meta-chips']) DOM['triggers-meta-chips'].innerHTML = '';
        if (DOM['triggers-pipelines-list']) DOM['triggers-pipelines-list'].innerHTML = '';
        if (DOM['triggers-pipelines-empty']) DOM['triggers-pipelines-empty'].classList.add('hidden');
        if (DOM['triggers-runs-list']) DOM['triggers-runs-list'].innerHTML = '';
        if (DOM['triggers-runs-empty']) DOM['triggers-runs-empty'].classList.add('hidden');
        updateTriggerActionButtons(null);
        highlightActiveListItem();
        refreshSidebarListFromState();
    }

    async function renderTriggerDetail(slug, { enterEditor = false } = {}) {
        if (!slug) {
            renderDetailEmpty();
            return;
        }

        state.selectedSlug = slug;
        const folderKey = getFolderKeyForSlug(slug);
        setActiveFolder(folderKey, { render: true, updateHash: false });
        if (state.isEditing) {
            exitEditMode(true, { silent: true, updateHash: false });
        }
        state.isEditing = false;
        showDetailView(true);
        if (DOM['triggers-view-actions']) DOM['triggers-view-actions'].classList.remove('hidden');
        if (DOM['triggers-edit-actions']) DOM['triggers-edit-actions'].classList.add('hidden');
        if (DOM['triggers-editor-container']) DOM['triggers-editor-container'].classList.add('hidden');
        if (DOM['triggers-yaml-content']) DOM['triggers-yaml-content'].classList.remove('hidden');

        const info = await ensureTriggerLoaded(slug);
        if (!info) {
            renderDetailEmpty();
            return;
        }

        state.currentYaml = info.yaml;
        if (DOM['triggers-detail']) DOM['triggers-detail'].classList.remove('hidden');

        if (DOM['triggers-detail-name']) {
            DOM['triggers-detail-name'].textContent = slug;
        }

        const sourceLabel = resolveTriggerSource(slug);
        if (DOM['triggers-detail-source']) {
            DOM['triggers-detail-source'].textContent = sourceLabel;
            DOM['triggers-detail-source'].setAttribute('title', sourceLabel);
        }

        updateTriggerActionButtons(slug, sourceLabel);
        renderMetaChips(info.summary);
        renderTriggerMeta(info.summary, slug, sourceLabel);
        renderYamlView(info.yaml);
        await renderPipelinesList(info.summary);
        await renderRecentRuns(slug, info.summary);

        if (enterEditor) {
            enterEditMode();
        }
        highlightActiveListItem();
    }

    function renderMetaChips(summary) {
        if (!DOM['triggers-meta-chips']) return;
        DOM['triggers-meta-chips'].innerHTML = '';
        DOM['triggers-meta-chips'].classList.add('hidden');
    }

    function renderTriggerMeta(summary, slug, sourceLabel = '') {
        if (!DOM['triggers-detail-meta']) return;
        const repo = slug || 'N/A';
        const triggerCount = summary?.triggerCount ?? 0;
        
        const environments = Array.isArray(summary?.environments) ? summary.environments : [];
        
        const eventsLabel = summary?.events && summary.events.length
            ? summary.events.slice(0, 3).join(', ') + (summary.events.length > 3 ? '…' : '')
            : 'All events';
        const fullEventsLabel = summary?.events && summary.events.length
            ? summary.events.join(', ')
            : eventsLabel;
        const source = sourceLabel || resolveTriggerSource(slug);

        const items = [
            { label: 'Repository', value: repo },
            { label: 'Source', value: source },
            { label: 'Rules', value: triggerCount },
            { label: 'Events', value: eventsLabel, title: fullEventsLabel },
        ];

        const encodeEnvSegment = (envLabel) => {
            const label = String(envLabel || '').trim();
            return label ? encodeURIComponent(label) : 'default';
        };

        let envHtml = '';
        if (environments.length > 0) {
            envHtml = environments.map(env => {
                const label = env === '' ? 'Default Scope' : `/${env}`;
                const href = `#/environment/${encodeEnvSegment(env)}`;
                // Use the 'pipelines-tag' class to make it look like a clickable chip
                return `<a href="${href}" class="pipelines-tag font-semibold transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-accent)]" style="text-decoration: none;">${escapeHtml(label)}</a>`;
            }).join('');
        } else {
            const href = `#/environment/default`;
            envHtml = `<a href="${href}" class="pipelines-tag font-semibold transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-accent)]" style="text-decoration: none;">Default Scope</a>`;
        }

        let html = `<dl class="triggers-detail-grid">`;

        html += items.map(({ label, value }) => {
            const safeValue = escapeHtml(String(value ?? ''));
            return `
                <dt class="triggers-detail-label">${escapeHtml(label)}:</dt>
                <dd class="triggers-detail-value">${safeValue}</dd>
            `;
        }).join('');

        // Add the Environments row, allowing the chips to wrap
        html += `
            <dt class="triggers-detail-label" style="align-self: flex-start; margin-top: 4px;">Environments:</dt>
            <dd class="triggers-detail-value flex flex-wrap gap-1.5" style="white-space: normal;">
                ${envHtml}
            </dd>
        `;

        html += `</dl>`;
        DOM['triggers-detail-meta'].innerHTML = html;
    }

    function updateTriggerActionButtons(slug, sourceLabel) {
        const label = (sourceLabel || (slug ? resolveTriggerSource(slug) : '') || '').trim().toLowerCase();
        const isGit = label === 'git';
        if (DOM['triggers-edit-btn']) {
            DOM['triggers-edit-btn'].classList.toggle('hidden', !!isGit);
        }
        if (DOM['triggers-clone-btn']) {
            DOM['triggers-clone-btn'].classList.toggle('hidden', !isGit);
            DOM['triggers-clone-btn'].dataset.triggerSlug = slug || '';
        }
    }

    function renderYamlView(yaml) {
        const target = DOM['triggers-yaml-content'];
        if (target) {
            const renderer = global.yaml && typeof global.yaml.renderLines === 'function'
                ? global.yaml.renderLines
                : null;
            target.innerHTML = renderer
                ? renderer(yaml, escapeHtml)
                : buildPlainYamlLines(yaml);
        }
        if (DOM['triggers-editor-container']) {
            DOM['triggers-editor-container'].classList.add('hidden');
        }
        if (DOM['triggers-yaml-content']) {
            DOM['triggers-yaml-content'].classList.remove('hidden');
        }
        if (DOM['triggers-validation-status']) {
            DOM['triggers-validation-status'].classList.add('hidden');
            DOM['triggers-validation-status'].textContent = '';
        }
    }

    function buildPlainYamlLines(yamlString) {
        const lines = (yamlString || '').split('\n');
        return lines.map((line, idx) => `
            <div class="yaml-line">
                <span class="yaml-line-number">${idx + 1}</span>
                <span class="yaml-line-text">${escapeHtml(line)}</span>
            </div>`).join('');
    }

    async function renderPipelinesList(summary) {
        if (!DOM['triggers-pipelines-list'] || !DOM['triggers-pipelines-empty']) return;
        const seen = new Set();
        const items = (summary?.pipelines || []).filter(item => {
            const identifier = item?.identifier;
            if (!identifier || seen.has(identifier)) return false;
            seen.add(identifier);
            return true;
        });
        if (!items.length) {
            DOM['triggers-pipelines-empty'].classList.remove('hidden');
            DOM['triggers-pipelines-list'].innerHTML = '';
            return;
        }

        DOM['triggers-pipelines-empty'].classList.add('hidden');
        const enriched = await Promise.all(items.map(async item => {
            const meta = await ensurePipelineMeta(item.identifier);
            return { ...item, meta };
        }));

        const html = `<ul class="triggers-pipeline-list">${enriched.map(item => {
            const versionValue = escapeHtml(item.meta?.version || 'latest');
            const sourceKey = String(item.meta?.sourceKey || '').toLowerCase();
            const sourceValue = escapeHtml(item.meta?.source || getTriggerSourceLabel('database'));
            const pathDisplay = item.pathLabel === 'root' ? '/' : `/${item.pathLabel}`;
            const commonDetails = `
                <span class="triggers-pipeline-name">${escapeHtml(item.display)}</span>
                <dl class="triggers-detail-grid triggers-pipeline-details">
                    <dt class="triggers-detail-label">Path:</dt>
                    <dd class="triggers-detail-value">${escapeHtml(pathDisplay)}</dd>
                    <dt class="triggers-detail-label">Version:</dt>
                    <dd class="triggers-detail-value">${versionValue}</dd>
                    <dt class="triggers-detail-label">Source:</dt>
                    <dd class="triggers-detail-value">${sourceValue}</dd>
                </dl>`;
            if (sourceKey === 'local') {
                return `
                    <li class="triggers-pipeline-item triggers-pipeline-item--local" title="Local pipeline defined directly in repository">
                        <div class="triggers-pipeline-link triggers-pipeline-link--static triggers-pipeline-link--local">
                            ${commonDetails}
                        </div>
                    </li>`;
            }
            const hash = buildPipelineHash(item.identifier);
            return `
                <li class="triggers-pipeline-item">
                    <a href="${hash}" class="triggers-pipeline-link" title="Open ${escapeAttribute(item.display)}">
                        ${commonDetails}
                    </a>
                </li>`;
        }).join('')}</ul>`;

        const listEl = DOM['triggers-pipelines-list'];
        listEl.innerHTML = html;
        listEl.classList.toggle('triggers-pipelines-scroll', enriched.length > 5);
    }

    async function renderRecentRuns(slug, summary) {
        if (!DOM['triggers-runs-list'] || !DOM['triggers-runs-empty']) return;

        const pipelines = new Set((summary?.pipelines || []).map(item => item.identifier));
        const runs = await getRecentRunsForSlug(slug, pipelines);

        if (!runs.length) {
            DOM['triggers-runs-empty'].classList.remove('hidden');
            DOM['triggers-runs-list'].innerHTML = '';
            return;
        }

        DOM['triggers-runs-empty'].classList.add('hidden');
        const shouldScroll = runs.length > MAX_RUNS;
        const listHtml = `<ul class="triggers-runs-list${shouldScroll ? ' triggers-runs-scroll' : ''}">${runs.map(run => `<li class="triggers-runs-item">${renderRunRow(run)}</li>`).join('')}</ul>`;
        DOM['triggers-runs-list'].innerHTML = listHtml;

        const listElement = DOM['triggers-runs-list'].querySelector('.triggers-runs-list');
        if (!listElement) return;

        if (shouldScroll) {
            listElement.style.removeProperty('max-height');
            adjustTriggersRunsScrollHeight(listElement);
        } else {
            listElement.style.removeProperty('max-height');
        }
    }

    function adjustTriggersRunsScrollHeight(listEl, attempt = 0) {
        requestAnimationFrame(() => {
            const items = Array.from(listEl.children).slice(0, MAX_RUNS);
            if (!items.length) {
                listEl.style.removeProperty('max-height');
                return;
            }

            const totalItemsHeight = items.reduce((sum, item) => sum + item.offsetHeight, 0);
            const computedStyles = getComputedStyle(listEl);
            const rowGap = parseFloat(computedStyles.rowGap) || 0;
            const totalGapHeight = rowGap * Math.max(items.length - 1, 0);
            const maxHeight = Math.ceil(totalItemsHeight + totalGapHeight);

            if (maxHeight <= 0) {
                if (attempt < 5) {
                    setTimeout(() => adjustTriggersRunsScrollHeight(listEl, attempt + 1), 60);
                } else {
                    listEl.style.removeProperty('max-height');
                }
                return;
            }

            listEl.style.maxHeight = `${maxHeight}px`;
        });
    }

    function renderRunRow(run) {
        const runId = run.run_id || run.runId || '';
        const startedAt = run.started_at || run.startedAt;
        const branch = formatBranch(run.git_ref || run.gitRef);
        const status = (run.status || 'unknown').toLowerCase();
        const statusLabel = status.toUpperCase();
        const timeAgo = formatRelativeTime(startedAt);
        const pipelineName = run.pipeline_name || run.pipelineName || 'pipeline';
        const triggerEventId = run.trigger_event_id || run.triggerEventId || '';
        const shortRunId = runId ? String(runId).slice(0, 8) : 'unknown';
        const shortTriggerId = triggerEventId ? String(triggerEventId).slice(0, 8) : 'unknown';
        const encodedRunId = runId ? encodeURIComponent(runId) : '';
        const runUrl = runId ? `#/pipelineruns/recent/${encodedRunId}` : '#/pipelineruns/recent';

        return `
            <a href="${runUrl}" class="pipelines-run-row block" title="Open run ${escapeAttribute(runId || '')}">
                <div class="triggers-run-row">
                    <div class="triggers-run-row__line triggers-run-row__line--primary">
                        <span class="triggers-run-row__pipeline" title="Pipeline: ${escapeAttribute(pipelineName)}">${escapeHtml(pipelineName)}</span>
                        <span class="triggers-run-row__time">${escapeHtml(timeAgo)}</span>
                    </div>
                    <div class="triggers-run-row__line triggers-run-row__line--status">
                        <span class="triggers-run-row__status">${escapeHtml(statusLabel)}</span>
                    </div>
                    <dl class="triggers-detail-grid triggers-run-details">
                        <dt class="triggers-detail-label">Branch:</dt>
                        <dd class="triggers-detail-value" title="${escapeAttribute(branch)}">${escapeHtml(branch)}</dd>
                        <dt class="triggers-detail-label">Run ID:</dt>
                        <dd class="triggers-detail-value" title="${escapeAttribute(runId || shortRunId)}">${escapeHtml(shortRunId)}</dd>
                        <dt class="triggers-detail-label">Trigger ID:</dt>
                        <dd class="triggers-detail-value" title="${escapeAttribute(triggerEventId || shortTriggerId)}">${escapeHtml(shortTriggerId)}</dd>
                    </dl>
                </div>
            </a>
        `;
    }

    async function getRecentRunsForSlug(slug, pipelinesSet) {
        const now = Date.now();
        if (!state.runsCache.runs.length || (now - state.runsCache.fetchedAt) > RUNS_CACHE_TTL) {
            try {
                const runs = await context.fetchData('/v1/runs');
                if (Array.isArray(runs)) {
                    state.runsCache = { runs, fetchedAt: Date.now() };
                } else {
                    state.runsCache = { runs: [], fetchedAt: Date.now() };
                }
            } catch (error) {
                console.error('Failed to load runs:', error);
                showToast('Failed to load recent runs.', 'error');
                state.runsCache = { runs: [], fetchedAt: Date.now() };
            }
        }

        const [owner, name] = splitSlug(slug);
        const normalizedOwner = owner.toLowerCase();
        const normalizedName = name.toLowerCase();
        const pipelines = Array.from(pipelinesSet || []);

        return (state.runsCache.runs || []).filter(run => {
            const runOwner = (run.git_repo_owner || '').toLowerCase();
            const runName = (run.git_repo_name || '').toLowerCase();
            if (runOwner !== normalizedOwner || runName !== normalizedName) return false;
            if (!pipelines.length) return true;
            const runIdentifier = buildPipelineIdentifierFromRun(run);
            return pipelines.includes(runIdentifier);
        }).sort((a, b) => new Date(b.started_at || b.startedAt || 0) - new Date(a.started_at || a.startedAt || 0));
    }

    function copyTriggerYaml() {
        if (!state.selectedSlug) return;
        const entry = state.triggerCache.get(state.selectedSlug);
        const yamlText = (entry && entry.yaml) || state.currentYaml || '';
        if (!yamlText) return;
        navigator.clipboard.writeText(yamlText).then(() => {
            showToast('Trigger YAML copied to clipboard.', 'success');
        }).catch(() => {
            showToast('Failed to copy trigger YAML.', 'error');
        });
    }

    function downloadTriggerYaml() {
        if (!state.selectedSlug) return;
        const entry = state.triggerCache.get(state.selectedSlug);
        const yamlText = (entry && entry.yaml) || state.currentYaml || '';
        const blob = new Blob([yamlText], { type: 'text/yaml' });
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        const safeName = state.selectedSlug.replace(/\//g, '_') || 'trigger';
        link.href = url;
        link.download = `${safeName}.yaml`;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        URL.revokeObjectURL(url);
    }

    function buildPipelineIdentifierFromRun(run) {
        const name = run.pipeline_name || run.pipelineName || '';
        const path = run.pipeline_path || run.pipelinePath || '';
        const identifier = path ? `${path}/${name}` : name;
        return normalizePipelineIdentifier(identifier);
    }

    function enterEditMode() {
        if (!state.selectedSlug) return;
        const info = state.triggerCache.get(state.selectedSlug);
        if (!info) return;

        state.isEditing = true;
        state.currentYaml = info.yaml;
        state.currentTriggerSummary = info.summary || null;
        state.editorCharWidth = null;

        if (DOM['triggers-yaml-content']) {
            DOM['triggers-yaml-content'].classList.add('hidden');
        }
        if (DOM['triggers-editor-wrapper']) {
            DOM['triggers-editor-wrapper'].classList.remove('hidden');
        }
        if (DOM['triggers-editor-container']) {
            DOM['triggers-editor-container'].classList.remove('hidden');
        }
        if (DOM['triggers-view-actions']) {
            DOM['triggers-view-actions'].classList.add('hidden');
        }
        if (DOM['triggers-edit-actions']) {
            DOM['triggers-edit-actions'].classList.remove('hidden');
        }
        if (DOM['triggers-yaml-editor']) {
            DOM['triggers-yaml-editor'].value = info.yaml;
            updateLineNumbers(info.yaml);
            updateTriggerEditorHighlight();
            validateCurrentYaml();
            ensureTriggerEditorSuggestionOverlay();
            updateTriggerEditorSuggestions();
            updateTriggerInlineSuggestionPosition();
            DOM['triggers-yaml-editor'].focus();
        }

        const expectedHash = buildTriggerHash(state.selectedSlug, 'edit');
        if (window.location.hash !== expectedHash) {
            try {
                history.replaceState(null, '', expectedHash);
            } catch {
                window.location.hash = expectedHash;
            }
        }
    }

    function exitEditMode(discardChanges = false, { silent = false, updateHash } = {}) {
        if (!state.isEditing) return;
        state.isEditing = false;
        hideTriggerEditorSuggestions();
        state.editorCharWidth = null;
        state.currentTriggerSummary = null;
        if (DOM['triggers-editor-container']) {
            DOM['triggers-editor-container'].classList.add('hidden');
        }
        if (DOM['triggers-editor-wrapper']) {
            DOM['triggers-editor-wrapper'].classList.add('hidden');
        }
        if (DOM['triggers-yaml-content']) {
            DOM['triggers-yaml-content'].classList.remove('hidden');
        }
        if (DOM['triggers-view-actions']) {
            DOM['triggers-view-actions'].classList.remove('hidden');
        }
        if (DOM['triggers-edit-actions']) {
            DOM['triggers-edit-actions'].classList.add('hidden');
        }
        if (DOM['triggers-validation-status']) {
            DOM['triggers-validation-status'].className = 'hidden text-xs text-[var(--text-secondary)]';
            DOM['triggers-validation-status'].textContent = '';
        }
        if (DOM['triggers-save-btn']) {
            DOM['triggers-save-btn'].disabled = false;
        }

        if (!silent && discardChanges && state.selectedSlug) {
            const cache = state.triggerCache.get(state.selectedSlug);
            if (cache) {
                renderYamlView(cache.yaml);
            }
        }

        updateTriggerActionButtons(state.selectedSlug);
        const shouldUpdateHash = updateHash !== undefined ? updateHash : !silent;
        if (shouldUpdateHash && state.selectedSlug) {
            const expectedHash = buildTriggerHash(state.selectedSlug, 'view');
            if (window.location.hash !== expectedHash) {
                try {
                    history.replaceState(null, '', expectedHash);
                } catch {
                    window.location.hash = expectedHash;
                }
            }
        }
    }

    function handleEditorInput(event) {
        const value = event.target.value;
        updateLineNumbers(value);
        updateTriggerEditorHighlight();
        validateCurrentYaml();
        updateTriggerEditorSuggestions();
        updateTriggerInlineSuggestionPosition();
    }

    function syncEditorScroll() {
        if (DOM['triggers-line-numbers'] && DOM['triggers-yaml-editor']) {
            DOM['triggers-line-numbers'].scrollTop = DOM['triggers-yaml-editor'].scrollTop;
        }
        syncTriggerHighlightScroll();
        updateTriggerInlineSuggestionPosition();
    }

    function updateLineNumbers(text) {
        if (!DOM['triggers-line-numbers']) return;
        const lines = String(text ?? DOM['triggers-yaml-editor']?.value ?? '').split('\n');
        const html = lines.map((_, idx) => {
            const lineNumber = idx + 1;
            return `<div class="line-number" data-line-number="${lineNumber}">${lineNumber}</div>`;
        }).join('');
        DOM['triggers-line-numbers'].innerHTML = html || '<div class="line-number" data-line-number="1">1</div>';
    }

    function updateTriggerEditorHighlight() {
        if (!DOM['triggers-yaml-highlight'] || !DOM['triggers-yaml-editor']) return;
        const renderer = global.yaml && typeof global.yaml.renderTokens === 'function'
            ? global.yaml.renderTokens
            : null;
        const stage = DOM['triggers-yaml-stage'];
        if (!renderer) {
            if (stage) stage.classList.remove('yaml-editor-stage--with-highlight');
            DOM['triggers-yaml-highlight'].textContent = DOM['triggers-yaml-editor'].value || '';
            return;
        }
        DOM['triggers-yaml-highlight'].innerHTML = renderer(DOM['triggers-yaml-editor'].value || '', escapeHtml) || '&nbsp;';
        if (stage) stage.classList.add('yaml-editor-stage--with-highlight');
        syncTriggerHighlightScroll();
    }

    function syncTriggerHighlightScroll() {
        if (!DOM['triggers-yaml-highlight'] || !DOM['triggers-yaml-editor']) return;
        const editor = DOM['triggers-yaml-editor'];
        const x = editor.scrollLeft || 0;
        const y = editor.scrollTop || 0;
        DOM['triggers-yaml-highlight'].style.transform = `translate(${-x}px, ${-y}px)`;
    }

    function validateCurrentYaml() {
        if (!DOM['triggers-yaml-editor'] || !DOM['triggers-validation-status']) return false;
        const value = DOM['triggers-yaml-editor'].value || '';
        try {
            const parsed = parseTriggerYaml(value);
            const summary = buildTriggerSummary(parsed);
            state.currentTriggerSummary = summary;
            const message = `${summary.triggerCount} trigger${summary.triggerCount === 1 ? '' : 's'} · ${summary.pipelineCount} pipeline${summary.pipelineCount === 1 ? '' : 's'}`;
            DOM['triggers-validation-status'].className = 'validation-box validation-box--success';
            DOM['triggers-validation-status'].innerHTML = `
                <div class="validation-box__header">All good</div>
                <div class="validation-box__message">${escapeHtml(message)}</div>
            `.trim();
            if (DOM['triggers-save-btn']) {
                DOM['triggers-save-btn'].disabled = false;
            }
            return true;
        } catch (error) {
            state.currentTriggerSummary = null;
            const errorMessage = error && error.message ? String(error.message) : 'Unknown validation error';
            DOM['triggers-validation-status'].className = 'validation-box validation-box--error';
            DOM['triggers-validation-status'].innerHTML = `
                <div class="validation-box__header">Validation failed</div>
                <div class="validation-box__message">${escapeHtml(errorMessage)}</div>
            `.trim();
            if (DOM['triggers-save-btn']) {
                DOM['triggers-save-btn'].disabled = true;
            }
            return false;
        }
    }

    function handleTriggerEditorKeydown(event) {
        if (!DOM['triggers-yaml-editor'] || !state.isEditing) return;
        if (event.key === 'Tab') {
            if (state.editorSuggestionItems.length && state.editorSuggestionContext) {
                event.preventDefault();
                const index = state.editorSuggestionIndex >= 0 ? state.editorSuggestionIndex : 0;
                const item = state.editorSuggestionItems[index] || state.editorSuggestionItems[0];
                applyTriggerEditorSuggestion(item);
            } else {
                event.preventDefault();
                insertTriggerEditorIndent(event.target);
                updateLineNumbers(event.target.value);
                validateCurrentYaml();
                updateTriggerEditorSuggestions();
                updateTriggerInlineSuggestionPosition();
            }
        } else if (event.key === 'Enter') {
            handleTriggerEditorEnterKey(event);
        } else if (event.key === 'Escape') {
            hideTriggerEditorSuggestions();
        } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            if (!state.editorSuggestionItems.length) return;
            event.preventDefault();
            if (!DOM['triggers-editor-autocomplete-ghost']) return;
            const direction = event.key === 'ArrowDown' ? 1 : -1;
            const total = state.editorSuggestionItems.length;
            let nextIndex = state.editorSuggestionIndex + direction;
            if (nextIndex < 0) nextIndex = total - 1;
            if (nextIndex >= total) nextIndex = 0;
            state.editorSuggestionIndex = nextIndex;
            const preview = buildTriggerInlineSuggestionPreview(state.editorSuggestionItems[nextIndex], state.editorSuggestionContext);
            DOM['triggers-editor-autocomplete-ghost'].textContent = preview || '';
        }
    }

    function insertTriggerEditorIndent(target) {
        if (!target || typeof target.value !== 'string') return;
        const start = target.selectionStart ?? 0;
        const end = target.selectionEnd ?? start;
        const value = target.value;
        target.value = value.substring(0, start) + '  ' + value.substring(end);
        const caret = start + 2;
        target.selectionStart = target.selectionEnd = caret;
        updateTriggerEditorHighlight();
    }

    function handleTriggerEditorEnterKey(event) {
        const textarea = event.target;
        if (!textarea || typeof textarea.value !== 'string') return;
        const start = textarea.selectionStart ?? 0;
        const end = textarea.selectionEnd ?? start;
        event.preventDefault();
        const value = textarea.value;
        const lineInfo = getCurrentLineInfo(value, start);
        const line = lineInfo ? lineInfo.line : '';
        const indentMatch = line.match(/^\s*/);
        const currentIndent = indentMatch ? indentMatch[0] : '';
        const trimmed = line.trim();
        let newIndent = currentIndent;

        if (trimmed.startsWith('-')) {
            const endsWithColon = trimmed.endsWith(':');
            const hasColon = trimmed.includes(':');
            if (endsWithColon) {
                newIndent = currentIndent + '  ';
            } else if (hasColon) {
                newIndent = ' '.repeat(lineInfo.indent + 2);
            } else {
                const parentIndent = Math.max(0, lineInfo.indent - 2);
                newIndent = ' '.repeat(parentIndent);
            }
        } else if (trimmed.endsWith(':')) {
            newIndent = currentIndent + '  ';
        }

        const insertion = `\n${newIndent}`;
        textarea.value = value.slice(0, start) + insertion + value.slice(end);
        const caret = start + insertion.length;
        textarea.selectionStart = textarea.selectionEnd = caret;
        textarea.focus();
        updateLineNumbers(textarea.value);
        updateTriggerEditorHighlight();
        validateCurrentYaml();
        updateTriggerEditorSuggestions();
        updateTriggerInlineSuggestionPosition();
    }

    function ensureTriggerEditorSuggestionOverlay() {
        if (DOM['triggers-editor-autocomplete'] || !DOM['triggers-editor-container']) return;
        const overlay = document.createElement('div');
        overlay.id = 'triggers-editor-autocomplete';
        overlay.className = 'pipeline-editor-autocomplete hidden';
        const ghost = document.createElement('span');
        ghost.className = 'pipeline-editor-autocomplete__ghost';
        overlay.appendChild(ghost);
        DOM['triggers-editor-container'].appendChild(overlay);
        DOM['triggers-editor-autocomplete'] = overlay;
        DOM['triggers-editor-autocomplete-ghost'] = ghost;
    }

    function hideTriggerEditorSuggestions() {
        state.editorSuggestionContext = null;
        state.editorSuggestionItems = [];
        state.editorSuggestionIndex = -1;
        const overlay = DOM['triggers-editor-autocomplete'];
        if (overlay) {
            overlay.classList.add('hidden');
        }
        if (DOM['triggers-editor-autocomplete-ghost']) {
            DOM['triggers-editor-autocomplete-ghost'].textContent = '';
        }
    }

    function updateTriggerEditorSuggestions() {
        if (!state.isEditing || !DOM['triggers-yaml-editor']) {
            hideTriggerEditorSuggestions();
            return;
        }

        ensureTriggerEditorSuggestionOverlay();

        const textarea = DOM['triggers-yaml-editor'];
        const text = textarea.value || '';
        const selectionStart = Math.min(textarea.selectionStart ?? 0, textarea.selectionEnd ?? 0);
        const selectionEnd = Math.max(textarea.selectionStart ?? 0, textarea.selectionEnd ?? 0);
        const contextInfo = detectTriggerSuggestionContext(text, selectionStart, selectionEnd);

        if (!contextInfo) {
            hideTriggerEditorSuggestions();
            return;
        }

        if (contextInfo.type === 'pipeline-value') {
            if (!(state.pipelineSourceIndex instanceof Map) || !state.pipelineSourceIndex.size) {
                ensurePipelineSourceIndex().then(() => {
                    updateTriggerEditorSuggestions();
                }).catch(() => {
                    hideTriggerEditorSuggestions();
                });
                return;
            }
        }

        const items = buildTriggerSuggestionItems(contextInfo, text);
        if (!items.length) {
            hideTriggerEditorSuggestions();
            return;
        }

        state.editorSuggestionContext = contextInfo;
        renderTriggerEditorSuggestions({ items });
    }

    function renderTriggerEditorSuggestions(payload) {
        ensureTriggerEditorSuggestionOverlay();
        const overlay = DOM['triggers-editor-autocomplete'];
        const ghostEl = DOM['triggers-editor-autocomplete-ghost'];
        const textarea = DOM['triggers-yaml-editor'];
        if (!overlay || !ghostEl || !textarea) return;

        if (!payload || !payload.items || !payload.items.length) {
            hideTriggerEditorSuggestions();
            return;
        }

        state.editorSuggestionItems = payload.items.slice();
        state.editorSuggestionIndex = 0;
        const activeItem = state.editorSuggestionItems[0];
        const preview = buildTriggerInlineSuggestionPreview(activeItem, state.editorSuggestionContext);
        if (!preview) {
            hideTriggerEditorSuggestions();
            return;
        }

        ghostEl.textContent = preview;
        overlay.classList.remove('hidden');
        updateTriggerInlineSuggestionPosition();
    }

    function buildTriggerInlineSuggestionPreview(item, contextInfo) {
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
        }

        if (prefix) {
            return '';
        }

        return firstLine;
    }

    function updateTriggerInlineSuggestionPosition() {
        if (!DOM['triggers-yaml-editor'] || !DOM['triggers-editor-autocomplete'] || !state.editorSuggestionContext) return;
        const textarea = DOM['triggers-yaml-editor'];
        const overlay = DOM['triggers-editor-autocomplete'];
        if (overlay.classList.contains('hidden')) return;

        const caret = calculateTriggerCaretOffset(textarea);
        if (!caret) return;

        const textareaRect = textarea.getBoundingClientRect();
        const containerRect = DOM['triggers-editor-container'] ? DOM['triggers-editor-container'].getBoundingClientRect() : textareaRect;
        const left = textareaRect.left - containerRect.left + caret.left;
        const top = textareaRect.top - containerRect.top + caret.top;

        overlay.style.transform = `translate(${Math.max(0, left)}px, ${Math.max(0, top)}px)`;
    }

    function calculateTriggerCaretOffset(textarea) {
        if (!textarea) return null;
        const selectionStart = textarea.selectionStart;
        if (typeof selectionStart !== 'number') return null;

        const textBeforeCaret = textarea.value.slice(0, selectionStart);
        const lines = textBeforeCaret.split('\n');
        const currentLine = lines[lines.length - 1] || '';
        const column = currentLine.length;
        const style = window.getComputedStyle(textarea);
        const lineHeight = getTriggerTextareaLineHeight(style);
        const charWidth = getTriggerTextareaCharWidth(textarea, style);
        const paddingLeft = parseFloat(style.paddingLeft) || 0;
        const paddingTop = parseFloat(style.paddingTop) || 0;
        const scrollLeft = textarea.scrollLeft || 0;
        const scrollTop = textarea.scrollTop || 0;

        return {
            left: paddingLeft + column * charWidth - scrollLeft,
            top: paddingTop + (lines.length - 1) * lineHeight - scrollTop,
        };
    }

    function getTriggerTextareaLineHeight(style) {
        const raw = style.lineHeight;
        const parsed = parseFloat(raw);
        if (!Number.isNaN(parsed) && parsed > 0) {
            return parsed;
        }
        const fontSize = parseFloat(style.fontSize) || 16;
        return fontSize * 1.5;
    }

    function getTriggerTextareaCharWidth(textarea, style) {
        if (state.editorCharWidth) {
            return state.editorCharWidth;
        }
        if (!measurementCanvas) {
            measurementCanvas = document.createElement('canvas');
        }
        const context2d = measurementCanvas.getContext('2d');
        if (context2d) {
            const fontWeight = style.fontWeight || '400';
            const fontSize = style.fontSize || '14px';
            const fontFamily = style.fontFamily || 'monospace';
            context2d.font = `${fontWeight} ${fontSize} ${fontFamily}`.trim();
            const metrics = context2d.measureText('M');
            if (metrics && metrics.width) {
                state.editorCharWidth = metrics.width;
                return metrics.width;
            }
        }
        const fallbackSize = parseFloat(style.fontSize) || 12;
        state.editorCharWidth = fallbackSize * 0.6;
        return state.editorCharWidth;
    }

    function applyTriggerEditorSuggestion(item) {
        if (!state.editorSuggestionContext || !DOM['triggers-yaml-editor'] || !item) return;
        const contextInfo = state.editorSuggestionContext;
        const textarea = DOM['triggers-yaml-editor'];
        const textLength = textarea.value.length;
        const rangeStart = Math.max(0, Math.min(contextInfo.rangeStart ?? textarea.selectionStart, textLength));
        const rangeEnd = Math.max(rangeStart, Math.min(contextInfo.rangeEnd ?? textarea.selectionEnd, textLength));
        const before = textarea.value.slice(0, rangeStart);
        const after = textarea.value.slice(rangeEnd);
        let insertText = item.snippet ?? item.value ?? '';
        if (typeof insertText !== 'string') {
            insertText = String(insertText ?? '');
        }
        const prefixInsert = contextInfo.insertPrefix || '';
        let suffixInsert = contextInfo.insertSuffix || '';
        if (item.overrideSuffix !== undefined) {
            suffixInsert = item.overrideSuffix;
        }
        const finalText = prefixInsert + insertText + suffixInsert;
        textarea.value = before + finalText + after;
        const caret = rangeStart + finalText.length;
        textarea.selectionStart = textarea.selectionEnd = caret;
        textarea.focus();
        hideTriggerEditorSuggestions();
        updateLineNumbers(textarea.value);
        updateTriggerEditorHighlight();
        validateCurrentYaml();
        updateTriggerEditorSuggestions();
        updateTriggerInlineSuggestionPosition();
    }

    function detectTriggerSuggestionContext(text, selectionStart, selectionEnd) {
        if (typeof text !== 'string') return null;
        const lineInfo = getCurrentLineInfo(text, selectionStart);
        if (!lineInfo) return null;
        const trimmedLine = lineInfo.line.trim();
        if (trimmedLine.startsWith('#')) {
            return null;
        }
        const beforeLine = text.slice(0, lineInfo.start);
        const parent = findParentBlock(beforeLine, ['triggers', 'branches', 'skip_branches', 'tags', 'pipelines'], lineInfo.indent);
        const insideTriggers = parent === 'triggers' || (parent === null && trimmedLine.startsWith('-'));

        const inlineContext = detectInlineKeyValueContext(lineInfo, selectionEnd);
        if (inlineContext && insideTriggers) {
            if (inlineContext.key === 'on') {
                return { ...inlineContext, type: 'event-value', title: 'Trigger events' };
            }
            if (inlineContext.key === 'environment') {
                return { ...inlineContext, type: 'environment-value', title: 'Environment names' };
            }
            if (inlineContext.key === 'pipelines') {
                return { ...inlineContext, type: 'pipeline-value', title: 'Pipelines' };
            }
            if (inlineContext.key === 'branches') {
                return { ...inlineContext, type: 'branch-value', title: 'Branches' };
            }
            if (inlineContext.key === 'skip_branches') {
                return { ...inlineContext, type: 'skip-branch-value', title: 'Skip branches' };
            }
            if (inlineContext.key === 'tags') {
                return { ...inlineContext, type: 'tag-value', title: 'Tags' };
            }
        }

        const listContext = detectTriggerListEntryContext(lineInfo, selectionEnd, beforeLine);
        if (listContext) {
            return listContext;
        }

        const lines = text.split('\n');
        const lineIndex = getLineIndexForOffset(text, lineInfo.start);
        const entryInfo = collectCurrentTriggerEntryInfo(lines, lineIndex);
        const existingKeys = collectExistingKeysForTriggerEntry(lines, entryInfo);
        const keyContext = detectTriggerKeyContext(lineInfo, selectionEnd, entryInfo, insideTriggers, existingKeys);
        if (keyContext) {
            return keyContext;
        }

        const rootContext = detectTriggerRootKeyContext(lineInfo, selectionEnd);
        if (rootContext) {
            return rootContext;
        }

        return null;
    }

    function detectInlineKeyValueContext(lineInfo, selectionEnd) {
        const colonIndex = lineInfo.line.indexOf(':');
        if (colonIndex === -1 || lineInfo.column <= colonIndex) {
            return null;
        }
        const keySegment = lineInfo.line.slice(lineInfo.indent, colonIndex);
        let key = keySegment.trim();
        if (key.startsWith('-')) {
            key = key.replace(/^-+\s*/, '').trim();
        }
        if (!key) return null;

        const afterColon = lineInfo.line.slice(colonIndex + 1, lineInfo.column);
        const whitespaceMatch = afterColon.match(/^\s*/);
        const whitespace = whitespaceMatch ? whitespaceMatch[0] : '';
        const valueSegment = afterColon.slice(whitespace.length);
        const trimmedValue = valueSegment.trim();
        const offsetWithinValue = trimmedValue ? valueSegment.indexOf(trimmedValue) : valueSegment.length;
        const rangeStart = lineInfo.start + colonIndex + 1 + whitespace.length + offsetWithinValue;
        const safeEnd = Math.max(rangeStart, selectionEnd);

        return {
            key,
            prefix: trimmedValue,
            rangeStart,
            rangeEnd: safeEnd,
            insertPrefix: whitespace ? '' : ' ',
            insertSuffix: '',
        };
    }

    function detectTriggerListEntryContext(lineInfo, selectionEnd, beforeLine) {
        const trimmed = lineInfo.line.trimStart();
        if (!trimmed.startsWith('-')) return null;
        const parent = findParentBlock(beforeLine, ['branches', 'skip_branches', 'tags', 'pipelines'], lineInfo.indent);
        if (!parent) return null;
        if (!TRIGGER_LIST_FIELDS.has(parent)) return null;
        const dashMatch = lineInfo.line.match(/^(\s*-\s*)/);
        const valueStart = dashMatch ? dashMatch[0].length : lineInfo.indent;
        const rangeStart = lineInfo.start + valueStart;
        const safeEnd = Math.max(rangeStart, selectionEnd);
        const baseContext = {
            prefix: lineInfo.line.slice(valueStart, lineInfo.column).trim(),
            rangeStart,
            rangeEnd: safeEnd,
            insertSuffix: '',
            insertPrefix: dashMatch && /\s$/.test(dashMatch[0]) ? '' : ' ',
        };
        if (parent === 'pipelines') {
            return { ...baseContext, type: 'pipeline-value', title: 'Pipelines' };
        }
        if (parent === 'tags') {
            return { ...baseContext, type: 'tag-value', title: 'Tags' };
        }
        if (parent === 'skip_branches') {
            return { ...baseContext, type: 'skip-branch-value', title: 'Skip branches' };
        }
        return { ...baseContext, type: 'branch-value', title: 'Branches' };
    }

    function detectTriggerKeyContext(lineInfo, selectionEnd, entryInfo, insideTriggers, existingKeys) {
        if (!insideTriggers) return null;
        const segment = lineInfo.line.slice(lineInfo.indent, lineInfo.column);
        if (!segment || segment.includes(':')) return null;
        const match = segment.match(/^-?\s*([A-Za-z_][A-Za-z0-9_]*)?$/);
        if (!match) return null;
        const prefix = match[1] || '';
        const prefixLocalIndex = segment.lastIndexOf(prefix);
        const dashMatch = segment.match(/^(-\s*)/);
        const dashLength = dashMatch ? dashMatch[0].length : 0;
        const baseIndent = entryInfo && typeof entryInfo.indent === 'number'
            ? entryInfo.indent
            : lineInfo.indent;
        let keyIndent = baseIndent + 2;
        if (dashMatch) {
            keyIndent = lineInfo.indent + dashLength;
        }
        const prefixOffset = Math.max(prefixLocalIndex, 0);
        const baseRangeIndent = Math.min(lineInfo.indent, keyIndent);
        const rangeStart = lineInfo.start + baseRangeIndent + prefixOffset;
        const needsSpaceAfterDash = dashMatch ? !dashMatch[0].endsWith(' ') : false;
        const requiredPrefixSpaces = Math.max(0, keyIndent - lineInfo.indent - (dashMatch ? dashLength : 0));

        return {
            type: 'trigger-key',
            title: 'Trigger fields',
            prefix,
            rangeStart,
            rangeEnd: Math.max(rangeStart, selectionEnd),
            insertPrefix: dashMatch
                ? (needsSpaceAfterDash ? ' ' : '')
                : ' '.repeat(requiredPrefixSpaces),
            insertSuffix: '',
            keyIndent,
            existingKeys,
            entryInfo,
        };
    }

    function detectTriggerRootKeyContext(lineInfo, selectionEnd) {
        if (lineInfo.indent !== 0) return null;
        if (lineInfo.line.includes(':')) return null;
        const segment = lineInfo.line.slice(0, lineInfo.column);
        const match = segment.match(/([A-Za-z_][A-Za-z0-9_]*)$/);
        const prefix = match ? match[1] : segment.trim();
        if (prefix == null) return null;
        const prefixStart = match ? lineInfo.column - prefix.length : lineInfo.column;
        const rangeStart = lineInfo.start + prefixStart;
        return {
            type: 'root-key',
            title: 'Manifest keys',
            prefix,
            rangeStart,
            rangeEnd: Math.max(rangeStart, selectionEnd),
            insertPrefix: '',
            insertSuffix: '',
            keyIndent: 0,
        };
    }

    function buildTriggerSuggestionItems(contextInfo, text) {
        const prefix = contextInfo.prefix || '';
        switch (contextInfo.type) {
            case 'root-key':
                return filterSuggestionPool(buildTriggerRootSuggestionItems(contextInfo), prefix, 4);
            case 'trigger-key':
                return filterSuggestionPool(buildTriggerKeySuggestionItems(contextInfo), prefix, 8);
            case 'event-value': {
                const events = new Set(TRIGGER_EVENT_OPTIONS);
                gatherTriggerSummaries().forEach(summary => {
                    (summary.events || []).forEach(value => {
                        const canonical = canonicalizeTriggerEvent(value);
                        if (canonical) {
                            events.add(canonical);
                        }
                    });
                });
                const items = Array.from(events)
                    .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }))
                    .map(value => ({ value, label: value }));
                return filterSuggestionPool(items, prefix, 8);
            }
            case 'environment-value': {
                const environments = collectKnownEnvironmentValues();
                const items = environments.map(value => ({ value, label: value }));
                return filterSuggestionPool(items, prefix, 8);
            }
            case 'pipeline-value': {
                const pipelines = collectKnownPipelineValues();
                const items = pipelines.map(value => ({ value, label: value }));
                return filterSuggestionPool(items, prefix, 8);
            }
            case 'branch-value': {
                const branches = collectKnownBranchValues();
                const items = branches.map(value => ({ value, label: value }));
                return filterSuggestionPool(items, prefix, 8);
            }
            case 'skip-branch-value': {
                const branches = collectKnownSkipBranchValues();
                const items = branches.map(value => ({ value, label: value }));
                return filterSuggestionPool(items, prefix, 8);
            }
            case 'tag-value': {
                const tags = collectKnownTagValues();
                const items = tags.map(value => ({ value, label: value }));
                return filterSuggestionPool(items, prefix, 8);
            }
            default:
                return [];
        }
    }

    function buildTriggerRootSuggestionItems(contextInfo) {
        return TRIGGER_ROOT_DEFINITIONS.map(def => ({
            value: def.key,
            label: def.key,
            snippet: def.snippet || `${def.key}: `,
            hint: def.hint || '',
        }));
    }

    function buildTriggerKeySuggestionItems(contextInfo) {
        const existingKeys = contextInfo.existingKeys instanceof Set ? contextInfo.existingKeys : new Set(contextInfo.existingKeys || []);
        const keyIndent = Math.max(0, contextInfo.keyIndent || 0);
        const childIndent = ' '.repeat(keyIndent + 2);
        return TRIGGER_FIELD_DEFINITIONS.map(def => {
            const keyLower = def.key.toLowerCase();
            const prefixLower = (contextInfo.prefix || '').toLowerCase();
            if (existingKeys.has(def.key) && !(prefixLower && keyLower.startsWith(prefixLower))) {
                return null;
            }
            let snippet;
            if (def.kind === 'list') {
                snippet = `${def.key}:\n${childIndent}- `;
            } else {
                snippet = `${def.key}: `;
            }
            return {
                value: def.key,
                label: def.key,
                snippet,
                hint: def.hint || '',
            };
        }).filter(Boolean);
    }

    function filterSuggestionPool(pool, prefix, limit = 8) {
        if (!Array.isArray(pool) || !pool.length) return [];
        const seen = new Set();
        const normalized = [];
        pool.forEach(item => {
            if (!item || !item.value) return;
            if (seen.has(item.value)) return;
            seen.add(item.value);
            normalized.push(item);
        });
        const lowerPrefix = (prefix || '').toLowerCase();
        if (!lowerPrefix) {
            return normalized.slice(0, limit);
        }
        const starts = [];
        const contains = [];
        normalized.forEach(item => {
            const key = (item.matchValue || item.label || item.value || '').toLowerCase();
            if (!key) return;
            if (key.startsWith(lowerPrefix)) {
                starts.push(item);
            } else if (key.includes(lowerPrefix)) {
                contains.push(item);
            }
        });
        return [...starts, ...contains].slice(0, limit);
    }

    function findParentKeyForLine(lines, startIndex, currentIndent) {
        for (let i = startIndex - 1; i >= 0; i--) {
            const candidate = lines[i];
            if (candidate === undefined) continue;
            const trimmed = candidate.trim();
            if (!trimmed || trimmed.startsWith('#')) continue;
            const indent = candidate.match(/^\s*/)[0].length;
            if (indent >= currentIndent) continue;
            let normalized = trimmed;
            if (normalized.startsWith('-')) {
                normalized = normalized.slice(1).trim();
            }
            if (!normalized.endsWith(':')) continue;
            const colonIndex = normalized.indexOf(':');
            if (colonIndex === -1) continue;
            const key = normalized.slice(0, colonIndex).trim();
            if (key) return key;
        }
        return null;
    }

    function collectCurrentTriggerEntryInfo(lines, lineIndex) {
        for (let i = lineIndex; i >= 0; i--) {
            const raw = lines[i];
            if (raw === undefined) continue;
            const trimmed = raw.trim();
            if (!trimmed || trimmed.startsWith('#')) continue;
            const indent = raw.match(/^\s*/)[0].length;
            if (trimmed.startsWith('-')) {
                const parentKey = findParentKeyForLine(lines, i, indent);
                if (parentKey === 'triggers') {
                    return { startIndex: i, indent, parentKey };
                }
                continue;
            }
            if (indent === 0 && trimmed.endsWith(':') && trimmed.slice(0, trimmed.indexOf(':')).trim() === 'triggers') {
                break;
            }
        }
        return null;
    }

    function collectExistingKeysForTriggerEntry(lines, entryInfo) {
        const keys = new Set();
        if (!entryInfo || entryInfo.startIndex == null || entryInfo.parentKey !== 'triggers') {
            return keys;
        }
        const baseIndent = entryInfo.indent;
        const firstLine = lines[entryInfo.startIndex] || '';
        const firstTrimmed = firstLine.trim();
        if (firstTrimmed.startsWith('-')) {
            const afterDash = firstTrimmed.slice(1).trim();
            const colonIndex = afterDash.indexOf(':');
            if (colonIndex !== -1) {
                const key = afterDash.slice(0, colonIndex).trim();
                if (key) keys.add(key);
            }
        }
        for (let i = entryInfo.startIndex + 1; i < lines.length; i++) {
            const raw = lines[i];
            if (raw === undefined) break;
            const trimmed = raw.trim();
            if (!trimmed || trimmed.startsWith('#')) continue;
            const indent = raw.match(/^\s*/)[0].length;
            if (indent < baseIndent) break;
            if (indent === baseIndent && trimmed.startsWith('-')) break;
            const normalized = trimmed.startsWith('- ') ? trimmed.slice(2) : trimmed;
            const colonIndex = normalized.indexOf(':');
            if (colonIndex !== -1) {
                const key = normalized.slice(0, colonIndex).trim();
                if (key) keys.add(key);
            }
        }
        return keys;
    }

    function collectKnownPipelineValues() {
        if (!(state.pipelineSourceIndex instanceof Map) || !state.pipelineSourceIndex.size) {
            return [];
        }

        const available = new Set();
        state.pipelineSourceIndex.forEach((_, key) => {
            const normalized = normalizePipelineIdentifier(key);
            if (normalized) {
                available.add(normalized);
            }
        });

        if (!available.size) {
            return [];
        }

        const suggestions = new Set(available);
        gatherTriggerSummaries().forEach(summary => {
            (summary.pipelines || []).forEach(item => {
                const candidate = typeof item === 'string' ? item : item && item.identifier;
                const normalized = normalizePipelineIdentifier(candidate || '');
                if (normalized && available.has(normalized)) {
                    suggestions.add(normalized);
                }
            });
        });

        return Array.from(suggestions).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
    }

    function collectKnownBranchValues() {
        const set = new Set();
        gatherTriggerSummaries().forEach(summary => {
            (summary.branches || []).forEach(value => set.add(String(value)));
        });
        return Array.from(set).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
    }

    function collectKnownSkipBranchValues() {
        const set = new Set();
        gatherTriggerSummaries().forEach(summary => {
            (summary.skipBranches || []).forEach(value => set.add(String(value)));
        });
        return Array.from(set).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
    }

    function collectKnownTagValues() {
        const set = new Set();
        gatherTriggerSummaries().forEach(summary => {
            (summary.tags || []).forEach(value => set.add(String(value)));
        });
        return Array.from(set).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
    }

    function collectKnownEnvironmentValues() {
        const set = new Set();
        gatherTriggerSummaries().forEach(summary => {
            (summary.environments || []).forEach(value => set.add(String(value)));
        });
        return Array.from(set).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
    }

    function gatherTriggerSummaries() {
        const summaries = [];
        if (state.currentTriggerSummary) {
            summaries.push(state.currentTriggerSummary);
        }
        if (state.triggerCache instanceof Map) {
            state.triggerCache.forEach(info => {
                if (info && info.summary) {
                    summaries.push(info.summary);
                }
            });
        }
        return summaries;
    }

    function getLineIndexForOffset(text, offset) {
        if (!text || offset <= 0) return 0;
        let count = 0;
        for (let i = 0; i < offset && i < text.length; i++) {
            if (text[i] === '\n') count++;
        }
        return count;
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

    function findParentBlock(beforeText, targetKeys, currentIndent) {
        if (!beforeText) return null;
        const lines = beforeText.split('\n');
        for (let i = lines.length - 1; i >= 0; i--) {
            const rawLine = lines[i];
            if (rawLine === undefined) continue;
            const trimmed = rawLine.trim();
            if (!trimmed || trimmed.startsWith('#')) {
                continue;
            }
            const indent = rawLine.match(/^\s*/)[0].length;
            if (indent >= currentIndent) {
                continue;
            }
            let normalized = trimmed;
            if (normalized.startsWith('-')) {
                normalized = normalized.slice(1).trim();
            }
            if (!normalized.endsWith(':')) {
                continue;
            }
            const key = normalized.slice(0, normalized.indexOf(':')).trim();
            if (targetKeys.includes(key)) {
                return key;
            }
        }
        return null;
    }

    async function handleSaveTrigger() {
        if (!state.selectedSlug || !DOM['triggers-yaml-editor']) return;
        const updatedYaml = DOM['triggers-yaml-editor'].value;

        const isValid = validateCurrentYaml();
        if (!isValid) {
            showToast('Please fix validation errors before saving.', 'error');
            return;
        }

        if (updatedYaml === state.currentYaml) {
            showToast('No changes to save.', 'info');
            exitEditMode(true, { silent: true });
            return;
        }

        const [owner, name] = splitSlug(state.selectedSlug);

        try {
            await context.fetchData(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/x-yaml' },
                body: updatedYaml,
            });

            const parsed = parseTriggerYaml(updatedYaml);
            const summary = buildTriggerSummary(parsed);
            const cacheEntry = { yaml: updatedYaml, manifest: parsed, summary, fetchedAt: Date.now() };
            state.triggerCache.set(state.selectedSlug, cacheEntry);
            renderYamlView(updatedYaml);
            renderMetaChips(summary);
            await renderPipelinesList(summary);
            await renderRecentRuns(state.selectedSlug, summary);
            exitEditMode(false, { updateHash: true });
            renderTriggerMeta(summary, state.selectedSlug);
            renderTriggerCollection();
            showToast('Trigger saved successfully.', 'success');
        } catch (error) {
            console.error('Failed to save trigger:', error);
            showToast('Failed to save trigger.', 'error');
        }
    }

    function promptTriggerDelete(slug) {
        const target = slug || state.selectedSlug;
        if (!target) return;
        if (isTriggerGitManaged(target)) {
            showToast('This trigger is managed via Git. Clone it to customize instead of deleting.', 'info');
            return;
        }
        state.pendingDeleteSlug = target;
        if (DOM['triggers-delete-message']) {
            DOM['triggers-delete-message'].innerHTML = `Are you sure you want to delete the trigger <strong>${escapeHtml(target)}</strong>?`;
        }
        openModal('triggers-delete-modal');
    }

    async function handleDeleteConfirm() {
        if (!state.pendingDeleteSlug) return;
        const slug = state.pendingDeleteSlug;
        if (isTriggerGitManaged(slug)) {
            showToast('This trigger is managed via Git and cannot be deleted here.', 'error');
            closeModal('triggers-delete-modal');
            state.pendingDeleteSlug = null;
            return;
        }
        const [owner, name] = splitSlug(slug);
        try {
            await context.deleteData(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`);
            closeModal('triggers-delete-modal');
            state.triggerCache.delete(slug);
            await loadTriggers(true);
            showToast(`Deleted trigger ${slug}.`, 'success');
            if (state.selectedSlug === slug) {
                state.selectedSlug = null;
                renderDetailEmpty();
            }
        } catch (error) {
            console.error('Failed to delete trigger:', error);
            showToast('Failed to delete trigger.', 'error');
        } finally {
            state.pendingDeleteSlug = null;
        }
    }

    function handleCloneButtonClick() {
        if (!state.selectedSlug || !DOM['triggers-clone-subtitle']) return;
        DOM['triggers-clone-subtitle'].textContent = `Cloning ${state.selectedSlug}`;
        if (DOM['triggers-clone-form']) DOM['triggers-clone-form'].reset();
        openModal('triggers-clone-modal');
    }

    async function handleCloneSubmit(event) {
        event.preventDefault();
        if (!state.selectedSlug) return;
        const form = event.currentTarget;
        const input = DOM['triggers-clone-repo'];
        if (!input) return;
        const newSlug = (input.value || '').trim();
        if (!newSlug || !newSlug.includes('/')) {
            showToast('Please provide a repository in owner/name format.', 'error');
            return;
        }

        const cache = await ensureTriggerLoaded(state.selectedSlug);
        if (!cache) return;

        try {
            const [owner, name] = splitSlug(newSlug);
            await context.fetchData(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/x-yaml' },
                body: cache.yaml,
            });

            closeModal('triggers-clone-modal');
            await loadTriggers(true);
            showToast(`Cloned trigger to ${newSlug}.`, 'success');
            navigateToSlug(newSlug);
        } catch (error) {
            console.error('Failed to clone trigger:', error);
            showToast('Failed to clone trigger.', 'error');
        }
    }

    async function handleCreateTriggerSubmit(event) {
        event.preventDefault();
        const repoInput = DOM['triggers-new-repo'];
        if (!repoInput) return;

        const repo = (repoInput.value || '').trim();
        if (!repo) {
            showToast('Repository is required.', 'error');
            return;
        }

        let owner;
        let name;
        try {
            [owner, name] = splitSlug(repo);
        } catch (error) {
            showToast(error.message, 'error');
            return;
        }

        const pipelinePath = deriveDefaultPipelinePath(repo);
        const yaml = buildNewTriggerYaml(pipelinePath);

        try {
            parseTriggerYaml(yaml);
        } catch (error) {
            console.error('Generated trigger template failed validation:', error);
            showToast('Failed to prepare trigger template.', 'error');
            return;
        }

        try {
            await context.fetchData(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/x-yaml' },
                body: yaml,
            });

            closeModal('triggers-new-modal');
            repoInput.value = '';
            if (DOM['triggers-new-form']) {
                DOM['triggers-new-form'].reset();
            }
            updateNewTriggerBlueprint();
            showToast(`Created trigger ${repo}.`, 'success');
            await loadTriggers(true);
            navigateToSlug(repo, 'edit');
        } catch (error) {
            console.error('Failed to create trigger:', error);
            showToast('Failed to create trigger.', 'error');
        }
    }

    function openModal(id) {
        const el = DOM[id];
        if (!el) return;
        if (id === 'triggers-new-modal') {
            updateNewTriggerBlueprint();
        }
        el.classList.remove('hidden');
        requestAnimationFrame(() => {
            el.classList.add('show');
            el.classList.add('opacity-100');
        });
    }

    function closeModal(id) {
        const el = DOM[id];
        if (!el) return;
        el.classList.remove('opacity-100');
        el.classList.remove('show');
        setTimeout(() => el.classList.add('hidden'), 300);
        if (id === 'triggers-new-modal') {
            updateNewTriggerBlueprint();
        }
    }

    async function handleRoute(hash) {
        await loadTriggers();
        const info = parseTriggerHash(hash || window.location.hash || '#/triggers');
        if (info.path !== 'triggers') {
            renderDetailEmpty();
            return;
        }

        if (info.slug) {
            if (state.isEditing && info.slug !== state.selectedSlug) {
                const proceed = confirm('Discard unsaved changes?');
                if (!proceed) {
                    navigateToSlug(state.selectedSlug || info.slug, state.isEditing ? 'edit' : 'view');
                    return;
                }
                exitEditMode(true);
            }
            await renderTriggerDetail(info.slug, { enterEditor: info.mode === 'edit' });
        } else {
            renderDetailEmpty();
            setActiveFolder(info.folderKey || '', { render: true, updateHash: false });
        }
    }

    function parseTriggerHash(hash) {
        const raw = String(hash || '').replace(/^#/, '');
        const parts = raw.split('/').filter(Boolean);
        const path = parts[0] || 'triggers';
        const segments = parts.slice(1);
        let mode = 'view';
        if (segments.length && segments[segments.length - 1] === 'edit') {
            mode = 'edit';
            segments.pop();
        }
        const decodedSegments = segments.map(segment => {
            try {
                return decodeURIComponent(segment);
            } catch {
                return segment;
            }
        });
        const candidate = decodedSegments.join('/');
        const slug = (state.triggers || []).includes(candidate) ? candidate : null;
        const folderKey = slug ? getFolderKeyForSlug(slug) : decodedSegments.join('/');
        return {
            path,
            folderKey: normalizeFolderKey(folderKey),
            slug,
            mode,
        };
    }

    function splitSlug(slug) {
        const trimmed = String(slug || '').trim();
        const sep = trimmed.indexOf('/');
        if (sep === -1) throw new Error('Slug must be in owner/name format');
        const owner = trimmed.slice(0, sep).trim();
        const name = trimmed.slice(sep + 1).trim();
        if (!owner || !name) {
            throw new Error('Slug must be in owner/name format');
        }
        return [owner, name];
    }

    function parseTriggerYaml(text) {
        if (!window.jsyaml) {
            throw new Error('YAML parser unavailable');
        }
        const manifest = window.jsyaml.load(text);
        if (!manifest || typeof manifest !== 'object') {
            throw new Error('Manifest must be a YAML object');
        }
        if (!Array.isArray(manifest.triggers) || manifest.triggers.length === 0) {
            throw new Error('Manifest must contain a non-empty "triggers" array');
        }
        return manifest;
    }

    function buildTriggerSummary(manifest) {
        const triggers = Array.isArray(manifest?.triggers) ? manifest.triggers : [];
        const pipelineIdentifiers = [];
        const events = new Set();
        const branches = new Set();
        const skipBranches = new Set();
        const tags = new Set();
        const environments = new Set();

        triggers.forEach(trigger => {
            if (trigger?.on) {
                const canonicalEvent = canonicalizeTriggerEvent(trigger.on);
                if (canonicalEvent) {
                    events.add(canonicalEvent);
                } else {
                    events.add(String(trigger.on));
                }
            }
            (trigger?.branches || []).forEach(branch => branches.add(String(branch)));
            (trigger?.skip_branches || trigger?.skipBranches || []).forEach(branch => skipBranches.add(String(branch)));
            (trigger?.tags || []).forEach(tag => tags.add(String(tag)));
            if (trigger?.environment) environments.add(String(trigger.environment));

            const pipelines = Array.isArray(trigger?.pipelines) ? trigger.pipelines : [];
            pipelines.forEach(entry => {
                let raw = '';
                if (typeof entry === 'string') {
                    raw = entry;
                } else if (entry && typeof entry === 'object' && entry.path) {
                    raw = entry.path;
                }
                raw = String(raw || '').trim();
                if (!raw) return;
                const identifier = normalizePipelineIdentifier(raw);
                const { pathLabel, display } = describePipeline(identifier);
                pipelineIdentifiers.push({ identifier, pathLabel, display });
            });
        });

        return {
            triggerCount: triggers.length,
            pipelineCount: pipelineIdentifiers.length,
            pipelines: pipelineIdentifiers,
            events: Array.from(events).sort(),
            branches: Array.from(branches).sort(),
            skipBranches: Array.from(skipBranches).sort(),
            tags: Array.from(tags).sort(),
            environments: Array.from(environments).sort(),
        };
    }

    function describePipeline(identifier) {
        const segments = identifier.split('/').filter(Boolean);
        const name = segments.pop() || identifier;
        const path = segments.join('/');
        const display = name;
        const pathLabel = path || 'root';
        return { identifier, display, pathLabel };
    }

    function normalizePipelineIdentifier(value) {
        let str = String(value || '').trim();
        str = str.replace(/^\.nopsai\//i, '');
        str = str.replace(/^pipelines\//i, '');
        str = str.replace(/\.ya?ml$/i, '');
        str = str.replace(/\/+/g, '/');
        str = str.replace(/^\//, '');
        return str;
    }

    function buildPipelineHash(identifier, isEdit = false) {
        if (!identifier) return '#/pipelines';
        const segments = (identifier || '').split('/').filter(Boolean).map(encodeURIComponent);
        let hash = `#/pipelines/${segments.join('/')}`;
        if (isEdit) hash += '/edit';
        return hash;
    }

    function buildTriggerFolderHash(folderKey) {
        if (!folderKey) return '#/triggers';
        const segments = String(folderKey || '').split('/').filter(Boolean).map(encodeURIComponent);
        return `#/triggers/${segments.join('/')}`;
    }

    function buildTriggerHash(slug, mode = 'view') {
        if (!slug) return '#/triggers';
        const segments = String(slug || '').split('/').filter(Boolean).map(encodeURIComponent);
        let hash = `#/triggers/${segments.join('/')}`;
        if (mode === 'edit') hash += '/edit';
        return hash;
    }

    async function renderSidebarList(container) {
        if (!container) return;
        await loadTriggers();
        if (!state.triggerTree) {
            state.triggerTree = buildTriggerTree(state.triggers || []);
        }
        renderTriggerSidebarTree(container);
    }

    function updateSidebarHighlight() {
        const container = document.getElementById('triggers-sidebar-tree');
        if (!container) return;
        container.querySelectorAll('[data-trigger-sidebar-slug]').forEach(link => {
            const slug = link.getAttribute('data-trigger-sidebar-slug');
            if (slug === state.selectedSlug) {
                link.classList.add('active');
            } else {
                link.classList.remove('active');
            }
        });
    }

    function refreshSidebarListFromState() {
        const container = document.getElementById('triggers-sidebar-tree');
        if (container) {
            renderTriggerSidebarTree(container);
        }
    }

    function showToast(message, variant = 'info') {
        const container = document.getElementById('toast-container');
        if (!container) {
            console[variant === 'error' ? 'error' : 'log'](message);
            return;
        }
        const toast = document.createElement('div');
        toast.className = `triggers-toast triggers-toast--${variant}`;
        toast.innerHTML = `<div class="triggers-toast__content">${escapeHtml(message)}</div>`;
        container.appendChild(toast);
        requestAnimationFrame(() => toast.classList.add('show'));
        setTimeout(() => {
            toast.classList.remove('show');
            setTimeout(() => toast.remove(), 200);
        }, 3200);
    }

    function formatRelativeTime(value) {
        const date = value instanceof Date ? value : new Date(value);
        if (Number.isNaN(date.getTime())) return 'recently';
        const delta = (Date.now() - date.getTime()) / 1000;
        if (delta < 60) return 'just now';
        if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
        if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`;
        return `${Math.floor(delta / 86400)}d ago`;
    }

    function formatBranch(ref) {
        if (!ref) return 'manual';
        return ref.replace(/^refs\/heads\//, '');
    }

    function escapeHtml(value) {
        return String(value ?? '').replace(/[&<>'"`]/g, c => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;', '`': '&#96;'
        })[c]);
    }

    function escapeAttribute(value) {
        return escapeHtml(value).replace(/"/g, '&quot;');
    }

    global.pages = global.pages || {};
    global.pages.triggers = {
        init,
        handleRoute,
        refresh: () => loadTriggers(true),
        renderSidebarForRoute: async () => {
            const container = document.getElementById('triggers-sidebar-tree');
            await renderSidebarList(container);
        },
    };
})(window.NopsAI = window.NopsAI || {});
