(function (global) {
    const state = {
        scopes: [],
        scopeMap: new Map(),
        scopeTree: null,
        filteredScopes: [],
        searchTerm: '',
        isSearching: false,
        selectedScopeKey: null,
        selectedVariable: null,
        activeFolderKey: '',
        sidebarExpanded: new Set(),
        pipelineEnvIndex: new Map(),
        envSources: new Map(),
        pendingEnvironmentParent: '',
        pipelineMetadata: new Map(),
        pipelineMetaSeeds: new Map(),
        pipelineEnvIndexReady: false,
        pipelineEnvIndexPromise: null,
        triggersByEnv: new Map(),
        pipelinesByEnv: new Map(),
        scopeLoadPromise: null,
        envValues: new Map(),
        envValuePromises: new Map(),
        expandedVariable: null,
    };

    const DOM = {};
    let context = null;
    let initialized = false;

    const DEFAULT_SCOPE_KEY = buildScopeKey('');
    const SAMPLE_ENVIRONMENT_VARIABLE = 'sample_variable';
    const SAMPLE_ENVIRONMENT_VALUE = 'Replace with your %ENV% environment value.';

    function init(ctx) {
        if (initialized && context === ctx) {
            return;
        }
        context = ctx;
        mapDomReferences();
        attachEventListeners();
        initialized = true;
        preloadData().catch(error => console.error('Failed to preload environment scopes:', error));
    }

    function mapDomReferences() {
        const ids = [
            'environment-search', 'environment-search-container', 'environment-clear-search', 'environment-list', 'environment-list-empty',
            'environment-list-view', 'environment-detail-view', 'environment-detail', 'environment-back-btn',
            'environment-detail-title', 'environment-variable-list', 'environment-variable-empty',
            'environment-variable-subtitle', 'environment-variable-pipelines',
        'environment-variable-triggers', 'environment-create-btn', 'environment-create-environment', 'environment-sidebar-tree',
        'environment-edit-modal', 'environment-edit-form', 'environment-edit-name', 'environment-edit-repo', 'environment-edit-value',
        'environment-edit-scope', 'environment-edit-submit', 'environment-delete-modal', 'environment-delete-message',
        'environment-confirm-delete-btn', 'environment-variable-detail-label', 'environment-variable-detail-source',
        'environment-variable-detail-updated', 'environment-variable-detail-created',
        'environment-new-modal', 'environment-new-form', 'environment-new-name', 'environment-new-parent', 'environment-new-cancel', 'environment-new-close',
        'environment-suggestion-panel', 'environment-suggestion-list', 'environment-suggestion-empty'
    ];

        ids.forEach(id => {
            DOM[id] = document.getElementById(id);
        });

        DOM['environment-repo-options'] = document.getElementById('environment-repo-options');
        DOM['environment-create-inline'] = document.getElementById('environment-create-inline');
    }

    function attachEventListeners() {
        if (DOM['environment-search']) {
            DOM['environment-search'].addEventListener('input', handleSearchInput);
        }
        if (DOM['environment-clear-search']) {
            DOM['environment-clear-search'].addEventListener('click', clearSearch);
        }
        if (DOM['environment-back-btn']) {
            DOM['environment-back-btn'].addEventListener('click', () => {
                resetEnvironmentSelection({ showList: true });
                window.location.hash = '#/environment';
            });
        }
        if (DOM['environment-create-btn']) {
            DOM['environment-create-btn'].addEventListener('click', () => openEditModal('create'));
        }
        if (DOM['environment-create-environment']) {
            DOM['environment-create-environment'].addEventListener('click', () => {
                const scopeKey = determineCreateEnvironmentScopeKey();
                openNewEnvironmentModal(scopeKey);
            });
        }
        if (DOM['environment-create-inline']) {
            DOM['environment-create-inline'].addEventListener('click', (event) => {
                event.preventDefault();
                const scopeKey = state.selectedScopeKey
                    || (state.activeFolderKey ? buildScopeKey(state.activeFolderKey) : DEFAULT_SCOPE_KEY);
                openEditModal('create', { scopeKey: scopeKey });
            });
        }
        if (DOM['environment-suggestion-panel']) {
            DOM['environment-suggestion-panel'].addEventListener('click', handleEnvironmentSuggestionClick);
        }
        if (DOM['environment-new-cancel']) {
            DOM['environment-new-cancel'].addEventListener('click', hideNewEnvironmentModal);
        }
        if (DOM['environment-new-close']) {
            DOM['environment-new-close'].addEventListener('click', hideNewEnvironmentModal);
        }
        if (DOM['environment-edit-form']) {
            DOM['environment-edit-form'].addEventListener('submit', handleSubmitVariable);
        }
        if (DOM['environment-new-form']) {
            DOM['environment-new-form'].addEventListener('submit', handleCreateEnvironment);
        }
        const cancelEditBtns = DOM['environment-edit-modal']?.querySelectorAll('[data-cancel]');
        if (cancelEditBtns) {
            cancelEditBtns.forEach(btn => {
                btn.addEventListener('click', () => closeModal('environment-edit-modal'));
            });
        }
        if (DOM['environment-confirm-delete-btn']) {
            DOM['environment-confirm-delete-btn'].addEventListener('click', handleConfirmDelete);
        }
        const cancelDeleteBtn = DOM['environment-delete-modal']?.querySelector('[data-cancel]');
        if (cancelDeleteBtn) {
            cancelDeleteBtn.addEventListener('click', () => closeModal('environment-delete-modal'));
        }
        if (DOM['environment-list']) {
            DOM['environment-list'].addEventListener('click', handleEnvironmentListClick);
            DOM['environment-list'].addEventListener('keydown', handleEnvironmentListKeydown);
        }
        document.addEventListener('keydown', event => {
            if (event.key === 'Escape') {
                closeModal('environment-edit-modal');
                closeModal('environment-delete-modal');
                hideNewEnvironmentModal();
            }
        });
    }

    async function preloadData(force = false) {
        if (state.scopeLoadPromise) {
            await state.scopeLoadPromise;
            if (!force) {
                return;
            }
        } else if (!force && state.scopes.length && state.scopes.every(s => s.fetched)) {
            return;
        }

        state.scopeLoadPromise = (async () => {
            await ensureScopesLoaded();
            await loadTriggerSummaries();
            
            const variableLoadPromises = [];
            if (state.scopeMap instanceof Map) {
                state.scopeMap.forEach(scope => {
                    variableLoadPromises.push(ensureScopeVariablesLoaded(scope, force));
                });
            }
            await Promise.all(variableLoadPromises);

            filterScopes();
            renderScopeCollection();
            renderSidebarTree();
        })();

        try {
            await state.scopeLoadPromise;
        } finally {
            state.scopeLoadPromise = null;
        }
    }

    async function ensureScopesLoaded() {
        const previousMap = state.scopeMap instanceof Map ? state.scopeMap : new Map();
        const previousExpanded = state.sidebarExpanded instanceof Set ? new Set(state.sidebarExpanded) : new Set(['']);
        const labels = new Set();

        previousMap.forEach(scope => {
            labels.add(normalizeEnvironmentLabel(scope?.env || ''));
        });

        const fetchedScopes = await fetchEnvironmentScopes();
        fetchedScopes.forEach(entry => {
            const label = normalizeScopeEntry(entry);
            if (label !== null) {
                labels.add(label);
            }
        });

        if (!labels.size || !labels.has('')) {
            labels.add('');
        }

        const sortedLabels = Array.from(labels).sort(compareScopeLabels);

        const nextMap = new Map();
        const nextList = [];

        sortedLabels.forEach(envLabel => {
            const key = buildScopeKey(envLabel);
            const existing = previousMap.get(key);
            const scope = existing || createScopeRecord(envLabel);

            scope.env = envLabel;
            scope.label = envLabel ? envLabel : 'Default Scope';
            scope.description = envLabel ? `Variables scoped to “/${envLabel}”` : 'Fallback variables shared across all environments';
            scope.triggers = [];
            scope.triggerCount = 0;
            scope.pipelineSet = new Set();
            scope.pipelines = [];
            scope.fetchPromise = null;
            scope.variables = Array.isArray(scope.variables) ? scope.variables : [];
            scope.fetched = !!scope.fetched;
            scope.variableMeta = scope.variableMeta instanceof Map ? scope.variableMeta : new Map();

            nextMap.set(key, scope);
            nextList.push(scope);
        });

        state.scopeMap = nextMap;
        state.scopes = nextList;
        state.filteredScopes = nextList.slice();
        state.scopeTree = buildEnvironmentTree(nextList);
        state.sidebarExpanded = previousExpanded.size ? previousExpanded : new Set(['']);
        state.sidebarExpanded.add('');
        state.triggersByEnv = new Map();
        state.pipelinesByEnv = new Map();
        ensureActiveFolderKey();
    }

    async function fetchEnvironmentScopes() {
        if (!context || typeof context.fetchData !== 'function') return [];
        try {
            const response = await context.fetchData('/v1/environments/scopes');
            if (Array.isArray(response)) {
                return response;
            }
        } catch (error) {
            console.error('Failed to retrieve environment scopes list:', error);
        }
        return [];
    }

    function normalizeScopeEntry(entry) {
        if (entry == null) {
            return '';
        }
        if (typeof entry === 'string') {
            return normalizeEnvironmentLabel(entry);
        }
        if (typeof entry === 'object') {
            const value = entry.environment ?? entry.env ?? entry.name ?? entry.value ?? '';
            return normalizeEnvironmentLabel(value);
        }
        return '';
    }

    function compareScopeLabels(a, b) {
        if (a === b) return 0;
        if (a === '') return -1;
        if (b === '') return 1;
        return a.localeCompare(b, undefined, { sensitivity: 'base' });
    }

    function buildEnvironmentTree(scopes) {
        const root = { key: '', label: '', children: new Map(), scopes: [] };
        if (!Array.isArray(scopes) || !scopes.length) {
            return root;
        }

        scopes.forEach(scope => {
            const envLabel = normalizeEnvironmentLabel(scope?.env || '');
            if (!envLabel) {
                root.scopes.push(scope);
                return;
            }
            const parts = envLabel.split('/').filter(Boolean);
            let node = root;
            let path = '';
            parts.forEach(part => {
                path = path ? `${path}/${part}` : part;
                if (!node.children) {
                    node.children = new Map();
                }
                if (!node.children.has(part)) {
                    node.children.set(part, { key: path, label: part, children: new Map(), scopes: [] });
                }
                node = node.children.get(part);
            });
            node.scopes.push(scope);
        });

        return root;
    }

    function createScopeRecord(envLabel) {
        const label = envLabel ? envLabel : 'Default Scope';
        const description = envLabel ? `Variables scoped to “/${envLabel}”` : 'Fallback variables shared across all environments';
        return {
            key: buildScopeKey(envLabel),
            env: envLabel,
            label,
            description,
            variables: [],
            variableMeta: new Map(),
            fetched: false,
            fetching: false,
            fetchPromise: null,
            triggers: [],
            triggerCount: 0,
            pipelineSet: new Set(),
            pipelines: [],
        };
    }

    function buildScopeKey(envLabel) {
        return `env:${envLabel || ''}`;
    }

    function encodeScopeSegment(envLabel) {
        return envLabel ? encodeURIComponent(envLabel) : 'default';
    }

    function decodeEnvironmentSegment(segment, index, folderMode) {
        if (!segment) {
            return '';
        }
        let value;
        try {
            value = decodeURIComponent(segment);
        } catch {
            value = segment;
        }
        if (!folderMode && index === 0 && value === 'default') {
            return '';
        }
        return value;
    }

    async function loadTriggerSummaries() {
        if (!context || typeof context.fetchData !== 'function') return;

        if (!(state.scopeMap instanceof Map)) {
            state.scopeMap = new Map();
        }

        state.triggersByEnv = new Map();
        state.pipelinesByEnv = new Map();

        state.scopeMap.forEach(scope => {
            scope.triggers = [];
            scope.triggerCount = 0;
            scope.pipelineSet = new Set();
            scope.pipelines = [];
        });

        try {
            const response = await context.fetchData('/v1/overrides?include_source=true');
            const slugs = normalizeOverrideSlugs(response);
            if (!slugs.length) {
                state.scopeTree = buildEnvironmentTree(state.scopes);
                return;
            }

            for (const slug of slugs) {
                const [owner, name] = splitSlug(slug);
                if (!owner || !name) continue;
                const yaml = await context.fetchData(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`);
                if (typeof yaml !== 'string') continue;
                const manifest = parseYaml(yaml);
                if (!manifest || !Array.isArray(manifest.triggers)) continue;
                manifest.triggers.forEach(trigger => {
                    registerTriggerForScope({ slug, trigger, owner, name });
                });
            }

            state.scopeMap.forEach(scope => {
                scope.pipelines = Array.from(scope.pipelineSet).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
                scope.triggerCount = scope.triggers.length;
            });
            state.scopeTree = buildEnvironmentTree(state.scopes);
            ensureActiveFolderKey();
        } catch (error) {
            console.error('Failed to build trigger summaries for environments:', error);
        }
    }

    function normalizeOverrideSlugs(response) {
        if (!Array.isArray(response)) return [];
        const slugs = [];
        response.forEach(item => {
            if (!item) return;
            if (typeof item === 'string') {
                const slug = item.trim();
                if (slug) slugs.push(slug);
            } else if (typeof item === 'object') {
                const slug = item.slug || item.name || item.repository_name || item.repo || '';
                if (slug && typeof slug === 'string') {
                    slugs.push(slug.trim());
                }
            }
        });
        return Array.from(new Set(slugs.filter(Boolean))).sort((a, b) => a.localeCompare(b));
    }

    function splitSlug(slug) {
        const parts = String(slug || '').split('/').filter(Boolean);
        return [parts[0] || '', parts[1] || ''];
    }

    function parseYaml(text) {
        if (!text || !window.jsyaml || typeof window.jsyaml.load !== 'function') return null;
        try {
            return window.jsyaml.load(text);
        } catch (error) {
            console.error('Failed to parse YAML for environment trigger summary:', error);
            return null;
        }
    }

    function normalizeEnvironmentSourceKey(value) {
        if (value == null) return 'database';
        const key = String(value).trim().toLowerCase();
        if (!key) return 'database';
        if (key.includes('git')) return 'git';
        if (key.includes('draft')) return 'draft';
        if (key.includes('local')) return 'local';
        return 'database';
    }

    function formatEnvironmentSourceLabel(key) {
        switch (normalizeEnvironmentSourceKey(key)) {
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

    function getEnvironmentMetadata(name, scopeRef) {
        if (!name) return null;
        let scope = scopeRef;
        if (typeof scope === 'string') {
            scope = state.scopeMap?.get(scope);
        }
        if (scope && scope.variableMeta instanceof Map && scope.variableMeta.has(name)) {
            return scope.variableMeta.get(name);
        }
        const envKey = makeEnvValueCacheKey(name, scope?.env || '');
        if (state.envSources instanceof Map && state.envSources.has(envKey)) {
            return state.envSources.get(envKey);
        }
        return null;
    }

    function getEnvironmentSourceForVariable(name, scopeRef) {
        const meta = getEnvironmentMetadata(name, scopeRef);
        return meta?.source || 'database';
    }

    function isEnvironmentSourceEditable(key) {
        return normalizeEnvironmentSourceKey(key) !== 'git';
    }

    function scopeHasGitManagedVariables(scope) {
        if (!scope) return false;
        const variables = Array.isArray(scope.variables) ? scope.variables : [];
        return variables.some(name => normalizeEnvironmentSourceKey(getEnvironmentSourceForVariable(name, scope)) === 'git');
    }

    function formatRelativeTime(value) {
        const date = value instanceof Date ? value : new Date(value);
        const timestamp = date.getTime();
        if (Number.isNaN(timestamp)) return '';
        const delta = (Date.now() - timestamp) / 1000;
        if (delta < 60) return 'just now';
        if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
        if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`;
        return `${Math.floor(delta / 86400)}d ago`;
    }

    function formatDisplayTimestamp(value) {
        if (!value) return '';
        const date = new Date(value);
        if (Number.isNaN(date.getTime())) return value;
        const relative = formatRelativeTime(date);
        return relative ? `${date.toLocaleString()} (${relative})` : date.toLocaleString();
    }

    function registerTriggerForScope({ slug, trigger, owner, name }) {
        const envLabel = normalizeEnvironmentLabel(trigger?.environment);
        const scopeKey = buildScopeKey(envLabel);
        let scope = state.scopeMap.get(scopeKey);
        if (!scope) {
            scope = createScopeRecord(envLabel);
            state.scopeMap.set(scopeKey, scope);
            state.scopes.push(scope);
        }

        if (!(scope.pipelineSet instanceof Set)) {
            scope.pipelineSet = new Set();
        }
        if (!Array.isArray(scope.triggers)) {
            scope.triggers = [];
        }

        const repoSlug = `${owner}/${name}`;
        const pipelines = extractPipelineIdentifiers(trigger?.pipelines);
        pipelines.forEach(identifier => scope.pipelineSet.add(identifier));

        const triggerDescriptor = {
            slug: repoSlug,
            environment: envLabel,
            pipelines,
            event: canonicalizeEvent(trigger?.on),
            branches: Array.isArray(trigger?.branches) ? trigger.branches : [],
            tags: Array.isArray(trigger?.tags) ? trigger.tags : [],
            source: (trigger?.source || '').toString(),
        };

        scope.triggers.push(triggerDescriptor);
        scope.triggerCount = scope.triggers.length;
        scope.pipelines = Array.from(scope.pipelineSet).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));

        const envTriggers = state.triggersByEnv.get(envLabel) || [];
        envTriggers.push(triggerDescriptor);
        state.triggersByEnv.set(envLabel, envTriggers);

        const envPipelines = state.pipelinesByEnv.get(envLabel) || new Set();
        pipelines.forEach(identifier => envPipelines.add(identifier));
        state.pipelinesByEnv.set(envLabel, envPipelines);
    }

    function normalizeEnvironmentLabel(value) {
        if (value == null) return '';
        return String(value).trim();
    }

    function extractPipelineIdentifiers(entries) {
        if (!Array.isArray(entries)) return [];
        const identifiers = new Set();
        entries.forEach(entry => {
            let raw = '';
            if (typeof entry === 'string') {
                raw = entry;
            } else if (entry && typeof entry === 'object') {
                raw = entry.path || entry.pipeline || entry.name || '';
            }
            raw = String(raw || '').trim();
            if (!raw) return;
            identifiers.add(normalizePipelineIdentifier(raw));
        });
        return Array.from(identifiers);
    }

    function canonicalizeEvent(value) {
        if (!value) return 'custom';
        const normalized = String(value).trim().toLowerCase();
        if (normalized === 'pull-request') return 'pull_request';
        return normalized;
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

    function handleSearchInput(event) {
        state.searchTerm = (event.target.value || '').trim();
        filterScopes();
        renderScopeCollection();
    }

    function clearSearch() {
        if (DOM['environment-search']) {
            DOM['environment-search'].value = '';
        }
        state.searchTerm = '';
        filterScopes();
        renderScopeCollection();
    }

    function filterScopes() {
        const term = state.searchTerm.toLowerCase();
        if (!term) {
            state.isSearching = false;
            state.filteredScopes = state.scopes.slice();
            toggleClearButton(false);
            return;
        }
        state.isSearching = true;
        state.filteredScopes = state.scopes.filter(scope => {
            if (scope.label.toLowerCase().includes(term)) return true;
            if (scope.description.toLowerCase().includes(term)) return true;
            if (scope.env && scope.env.toLowerCase().includes(term)) return true;
            return false;
        });
        toggleClearButton(true);
    }

    function toggleClearButton(show) {
        if (!DOM['environment-clear-search']) return;
        DOM['environment-clear-search'].classList.toggle('hidden', !show);
    }

    function renderScopeCollection() {
        const container = DOM['environment-list'];
        if (!container) return;

        const searchTerm = (state.searchTerm || '').trim();
        const isSearching = state.isSearching && !!searchTerm;
        const hasSelection = !!state.selectedScopeKey;

        if (isSearching) {
            showListView();
        } else if (hasSelection) {
            showDetailView();
        } else {
            showListView();
        }

        let html = '';
        let showEmpty = false;

        if (isSearching) {
            const results = state.filteredScopes || [];
            if (results.length) {
                html += renderEnvironmentSearchSummary(results.length, searchTerm);
                const cards = results
                    .slice()
                    .sort((a, b) => (a.env || '').localeCompare(b.env || '', undefined, { sensitivity: 'base' }))
                    .map(renderEnvironmentCard);
                if (cards.length) {
                    html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${cards.join('')}</div>`;
                }
            } else {
                showEmpty = true;
            }
        } else {
            ensureActiveFolderKey();
            const activeFolder = normalizeEnvironmentLabel(state.activeFolderKey || '');
            const tree = state.scopeTree || buildEnvironmentTree(state.scopes);
            const node = getEnvironmentTreeNode(activeFolder) || tree;

            const childNodes = node?.children instanceof Map ? Array.from(node.children.values()) : [];
            childNodes.sort((a, b) => (a.key || '').localeCompare(b.key || '', undefined, { sensitivity: 'base' }));
            const scopes = Array.isArray(node?.scopes) ? node.scopes.slice() : [];
            scopes.sort((a, b) => (a.env || '').localeCompare(b.env || '', undefined, { sensitivity: 'base' }));
            const folderCards = childNodes.map(renderEnvironmentFolderCard).filter(Boolean);
            if (folderCards.length) {
                html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${folderCards.join('')}</div>`;
            }
            const scopeCards = scopes.map(renderEnvironmentCard);
            if (scopeCards.length) {
                html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${scopeCards.join('')}</div>`;
            }

            if (!folderCards.length && !scopeCards.length) {
                showEmpty = true;
            }
        }

        container.innerHTML = html;
        updateCreateEnvironmentButton({ isSearching });
        if (DOM['environment-list-empty']) {
            if (showEmpty) {
                if (isSearching && searchTerm) {
                    DOM['environment-list-empty'].innerHTML = `<p class="text-sm">No envs matched "${escapeHtml(searchTerm)}".</p>`;
                } else if (state.activeFolderKey) {
                    DOM['environment-list-empty'].innerHTML = '<p class="text-sm">No envs in this folder.</p>';
                } else {
                    DOM['environment-list-empty'].innerHTML = '<p class="text-sm">No envs found. Sync your configuration repository to import them.</p>';
                }
            }
            DOM['environment-list-empty'].classList.toggle('hidden', !showEmpty);
        }

        highlightActiveEnvironmentCard();
    }

    function updateCreateEnvironmentButton(options = {}) {
        const { isSearching = false } = options;
        const button = DOM['environment-create-environment'];
        if (!button) return;
        const parentLabel = getCreateEnvironmentParentLabel();
        const scopeKey = buildScopeKey(parentLabel);
        button.dataset.scopeKey = scopeKey;
        const targetPath = parentLabel ? `/${parentLabel}` : '/';
        button.setAttribute('title', `Create environment under ${targetPath}`);
        const shouldHide = !!isSearching || !!state.selectedScopeKey;
        button.classList.toggle('hidden', shouldHide);
    }

    function determineCreateEnvironmentScopeKey() {
        const button = DOM['environment-create-environment'];
        if (button?.dataset?.scopeKey) {
            return button.dataset.scopeKey;
        }
        return buildScopeKey(getCreateEnvironmentParentLabel());
    }

    function getCreateEnvironmentParentLabel() {
        return normalizeEnvironmentLabel(state.activeFolderKey || '');
    }

    function ensureActiveFolderKey() {
        const current = normalizeEnvironmentLabel(state.activeFolderKey || '');
        if (!getEnvironmentTreeNode(current)) {
            state.activeFolderKey = '';
        }
    }

    function getEnvironmentTreeNode(path) {
        const normalized = normalizeEnvironmentLabel(path || '');
        const tree = state.scopeTree || buildEnvironmentTree(state.scopes);
        if (!normalized) {
            return tree;
        }
        const segments = normalized.split('/').filter(Boolean);
        let node = tree;
        for (const segment of segments) {
            if (!(node?.children instanceof Map) || !node.children.has(segment)) {
                return null;
            }
            node = node.children.get(segment);
        }
        return node;
    }

    function countScopesRecursive(node) {
        if (!node) return 0;
        let total = Array.isArray(node.scopes) ? node.scopes.length : 0;
        if (node.children instanceof Map) {
            node.children.forEach(child => {
                total += countScopesRecursive(child);
            });
        }
        return total;
    }

    function renderEnvironmentFolderCard(node) {
        if (!node) return '';
        const totalScopes = countScopesRecursive(node);
        const childCount = node.children instanceof Map ? node.children.size : 0;
        const displayPath = node.key ? `/${node.key}` : '/';
        const label = node.label || (node.key ? node.key.split('/').pop() : '/');
        const keyAttr = escapeAttribute(node.key || '');
        const ariaLabel = escapeAttribute(displayPath);

        return `
            <article class="pipeline-folder-card border border-[var(--border-primary)]" data-environment-folder="${keyAttr}" tabindex="0" role="button" aria-label="Open folder ${ariaLabel}">
                <div class="pipeline-folder-card-header">
                    <span class="pipeline-folder-icon">
                        <svg class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7h5l2 2h9a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V9a2 2 0 012-2z"/></svg>
                    </span>
                    <h3 class="pipeline-folder-title">${escapeHtml(label)}</h3>
                    <div class="pipeline-folder-actions">
                        <span class="pipeline-folder-chevron" aria-hidden="true"><svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 5l7 7-7 7"/></svg></span>
                    </div>
                </div>
                <div class="pipeline-folder-meta">
                    <div class="pipeline-folder-meta-row"><span class="pipeline-folder-meta-label">Envs:</span><span class="pipeline-folder-meta-value">${totalScopes}</span></div>
                    <div class="pipeline-folder-meta-row"><span class="pipeline-folder-meta-label">Sub folders:</span><span class="pipeline-folder-meta-value">${childCount}</span></div>
                </div>
            </article>`;
    }

    function describeEnvironmentScope(scope) {
        const envLabel = normalizeEnvironmentLabel(scope?.env || '');
        const segments = envLabel.split('/').filter(Boolean);
        const title = segments.length ? segments[segments.length - 1] : 'default';
        const parentPath = segments.length > 1 ? `/${segments.slice(0, -1).join('/')}` : '/';
        const fullPath = envLabel ? `/${envLabel}` : '/';
        return { title, parentPath, fullPath };
    }

    function renderEnvironmentCard(scope) {
        if (!scope) return '';
        const isActive = scope.key === state.selectedScopeKey;
        const { title, parentPath, fullPath } = describeEnvironmentScope(scope);
        const variableCount = Array.isArray(scope.variables) ? scope.variables.length : 0;
        const triggerCount = scope.triggerCount || 0;
        const deletionAllowed = !scopeHasGitManagedVariables(scope);
        const deleteTitle = deletionAllowed
            ? 'Delete environment scope'
            : 'Environment is managed via Git. Delete is disabled.';
        const deleteButton = deletionAllowed
            ? `<button class="pipelines-delete-button" type="button" data-environment-delete="${escapeAttribute(scope.key)}" title="${escapeAttribute(deleteTitle)}">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                                <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                                <path d="M4 7h16" />
                            </svg>
                        </button>`
            : `<button class="pipelines-delete-button" type="button" disabled aria-disabled="true" title="${escapeAttribute(deleteTitle)}">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                                <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                                <path d="M4 7h16" />
                            </svg>
                        </button>`;

        return `
            <article class="pipeline-card triggers-card bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded-lg shadow-sm transition-all duration-200 p-3 flex flex-col${isActive ? ' triggers-card--active' : ''}" data-environment-scope="${escapeAttribute(scope.key)}" tabindex="0" role="button" aria-label="Open environment ${escapeAttribute(fullPath)}">
                <div class="pipeline-card-header flex items-start justify-between gap-3">
                    <div class="pipeline-card-info">
                        <span class="triggers-card-icon" aria-hidden="true">
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M12 3a9 9 0 100 18 9 9 0 000-18z"/>
                                <path d="M3.6 9h16.8M3.6 15h16.8M12 3a15 15 0 010 18M12 3a15 15 0 000 18"/>
                            </svg>
                        </span>
                        <div class="pipeline-card-text min-w-0">
                            <h3 class="pipeline-card-title">${escapeHtml(title)}</h3>
                            <p class="pipeline-card-path">${escapeHtml(parentPath)}</p>
                        </div>
                    </div>
                    <div class="pipeline-card-actions">
                        ${deleteButton}
                    </div>
                </div>
                <div class="pipeline-card-meta">
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">Variables</span>
                        <span class="pipeline-card-meta-value">${variableCount}</span>
                    </div>
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">Triggers</span>
                        <span class="pipeline-card-meta-value">${triggerCount}</span>
                    </div>
                </div>
            </article>`;
    }

    function scopeKeyToLabel(scopeKey) {
        if (!scopeKey) return '';
        if (scopeKey.startsWith('env:')) {
            return scopeKey.slice(4);
        }
        return scopeKey;
    }

    function sanitizeEnvironmentSegments(raw) {
        if (!raw) return [];
        return String(raw)
            .split('/')
            .map(part => part.trim().replace(/[^A-Za-z0-9_.-]+/g, '-').replace(/^-+|-+$/g, ''))
            .filter(Boolean);
    }

    function renderEnvironmentSearchSummary(count, term) {
        const safeTerm = escapeHtml(term);
        return `<div class="triggers-search-summary">Showing ${count} result${count === 1 ? '' : 's'} for "${safeTerm}"</div>`;
    }

    function highlightActiveEnvironmentCard() {
        const list = DOM['environment-list'];
        if (!list) return;
        const isListVisible = !DOM['environment-list-view'] || !DOM['environment-list-view'].classList.contains('hidden');
        list.querySelectorAll('[data-environment-scope]').forEach(card => {
            const key = card.getAttribute('data-environment-scope');
            const shouldHighlight = isListVisible && key === state.selectedScopeKey;
            card.classList.toggle('env-scope-card--active', shouldHighlight);
        });
    }

    function buildEnvironmentFolderHash(folderKey) {
        const normalized = normalizeEnvironmentLabel(folderKey || '');
        if (!normalized) {
            return '#/environment';
        }
        const segments = normalized.split('/').filter(Boolean).map(encodeURIComponent);
        return `#/environment/folder/${segments.join('/')}`;
    }

    function navigateToFolder(folderKey) {
        const hash = buildEnvironmentFolderHash(folderKey);
        if (window.location.hash !== hash) {
            window.location.hash = hash;
        } else {
            handleRoute(hash);
        }
    }

function resetEnvironmentSelection(options = {}) {
        if (state.selectedScopeKey) {
            state.selectedScopeKey = null;
        }
        state.selectedVariable = null;
        state.expandedVariable = null;
        clearVariableDetail();
        if (options.showList !== false) {
            showListView();
        }
        highlightActiveEnvironmentCard();
        updateVariableItemStates();
    }

    function handleEnvironmentListClick(event) {
        const deleteButton = event.target.closest('[data-environment-delete]');
        if (deleteButton) {
            event.preventDefault();
            const scopeKey = deleteButton.getAttribute('data-environment-delete') || '';
            openDeleteEnvironmentModal(scopeKey);
            event.stopPropagation();
            return;
        }

        const folderCard = event.target.closest('[data-environment-folder]');
        if (folderCard) {
            event.preventDefault();
            const key = folderCard.getAttribute('data-environment-folder') || '';
            resetEnvironmentSelection({ showList: true });
            navigateToFolder(key);
            event.stopPropagation();
            return;
        }

        const scopeCard = event.target.closest('[data-environment-scope]');
        if (scopeCard) {
            event.preventDefault();
            const key = scopeCard.getAttribute('data-environment-scope');
            if (key) {
                navigateToScope(key);
            }
            event.stopPropagation();
            return;
        }
    }

    function handleEnvironmentListKeydown(event) {
        if (event.defaultPrevented) return;
        if (event.key !== 'Enter' && event.key !== ' ' && event.key !== 'Spacebar') return;

        const deleteButton = event.target.closest('[data-environment-delete]');
        if (deleteButton && deleteButton === document.activeElement) {
            event.preventDefault();
            const scopeKey = deleteButton.getAttribute('data-environment-delete') || '';
            openDeleteEnvironmentModal(scopeKey);
            event.stopPropagation();
            return;
        }

        const folderCard = event.target.closest('[data-environment-folder]');
        if (folderCard && folderCard === document.activeElement) {
            event.preventDefault();
            const key = folderCard.getAttribute('data-environment-folder') || '';
            resetEnvironmentSelection({ showList: true });
            navigateToFolder(key);
            focusFirstEnvironmentCard();
            event.stopPropagation();
            return;
        }

        const scopeCard = event.target.closest('[data-environment-scope]');
        if (scopeCard && scopeCard === document.activeElement) {
            event.preventDefault();
            const key = scopeCard.getAttribute('data-environment-scope');
            if (key) {
                navigateToScope(key);
            }
            event.stopPropagation();
        }
    }

    function focusFirstEnvironmentCard() {
        const list = DOM['environment-list'];
        if (!list) return;
        const first = list.querySelector('[data-environment-folder], [data-environment-scope]');
        if (first && typeof first.focus === 'function') {
            first.focus();
        }
    }

    function renderSidebarTree() {
        const container = document.getElementById('environment-sidebar-tree');
        if (!container) return;

        const tree = state.scopeTree || buildEnvironmentTree(state.scopes);
        const rootScopes = Array.isArray(tree?.scopes) ? tree.scopes.slice() : [];
        const childNodes = tree?.children instanceof Map ? Array.from(tree.children.values()) : [];

        rootScopes.sort((a, b) => (a.env || '').localeCompare(b.env || '', undefined, { sensitivity: 'base' }));
        childNodes.sort((a, b) => (a.key || '').localeCompare(b.key || '', undefined, { sensitivity: 'base' }));

        let html = '<ul class="space-y-1">';

        childNodes.forEach(child => {
            html += renderSidebarFolderNode(child, 0);
        });

        rootScopes.forEach(scope => {
            html += renderSidebarScopeEntry(scope);
        });

        html += '</ul>';
        container.innerHTML = html;

        container.querySelectorAll('[data-environment-toggle-folder]').forEach(button => {
            button.addEventListener('click', event => {
                event.stopPropagation();
                const path = button.getAttribute('data-environment-toggle-folder');
                toggleSidebarFolder(path);
            });
        });

        container.querySelectorAll('[data-environment-open-folder]').forEach(button => {
            button.addEventListener('click', event => {
                event.preventDefault();
                const path = button.getAttribute('data-environment-open-folder') || '';
                resetEnvironmentSelection({ showList: true });
                navigateToFolder(path);
                event.stopPropagation();
            });
        });

        container.querySelectorAll('[data-environment-sidebar-scope]').forEach(button => {
            button.addEventListener('click', event => {
                event.preventDefault();
                const key = button.getAttribute('data-environment-sidebar-scope');
                if (key) {
                    navigateToScope(key);
                }
                event.stopPropagation();
            });
        });

        renderEnvironmentSuggestions();
    }

    function renderSidebarFolderNode(node, level) {
        if (!node) return '';
        const folderPath = node.key || '';
        const folderLabel = formatEnvironmentFolderLabel(node.label);
        const isExpanded = shouldExpandFolder(folderPath);
        if (isExpanded) {
            ensureSidebarExpansionForPath(folderPath);
        }
        const isActiveFolder = folderPath && state.activeFolderKey === folderPath;

        const scopes = Array.isArray(node.scopes) ? node.scopes.slice() : [];
        scopes.sort((a, b) => (a.env || '').localeCompare(b.env || '', undefined, { sensitivity: 'base' }));

        const children = node.children instanceof Map ? Array.from(node.children.values()) : [];
        children.sort((a, b) => (a.key || '').localeCompare(b.key || '', undefined, { sensitivity: 'base' }));

        let innerHtml = '';
        if (children.length || scopes.length) {
            innerHtml += `<div class="environment-sidebar-children ${isExpanded ? '' : 'hidden'}" data-environment-folder-children="${escapeAttribute(folderPath)}">`;
            innerHtml += renderSidebarTreeNodes(node, level + 1);
            innerHtml += '</div>';
        }

        return `
            <li data-environment-folder-node="${escapeAttribute(folderPath)}">
                <div class="flex items-center justify-between p-1 text-[var(--text-primary)] rounded-md pipeline-sidebar-folder-row ${isActiveFolder ? 'bg-[var(--bg-tertiary)]' : ''} hover:bg-[var(--bg-tertiary)]">
                    <div class="flex items-center flex-grow min-w-0">
                        <button type="button" class="sidebar-toggle-btn flex items-center justify-center h-5 w-5 rounded mr-1 text-[var(--text-secondary)] hover:bg-[var(--bg-hover)]" data-environment-toggle-folder="${escapeAttribute(folderPath)}" aria-expanded="${isExpanded ? 'true' : 'false'}" aria-label="${escapeAttribute((isExpanded ? 'Collapse' : 'Expand') + ' ' + folderLabel)}">
                            <svg class="h-4 w-4 chevron ${isExpanded ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>
                        </button>
                        <button type="button" class="pipeline-sidebar-folder flex items-center gap-2 flex-grow text-left min-w-0 p-1 rounded hover:bg-[var(--bg-hover)]" data-environment-open-folder="${escapeAttribute(folderPath)}">
                            <svg class="h-4 w-4 text-[var(--text-secondary)] flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>
                            <span class="truncate">${escapeHtml(folderLabel)}</span>
                        </button>
                    </div>
                </div>
                ${innerHtml}
            </li>
        `;
    }

    function renderSidebarTreeNodes(node, level) {
        const childEntries = node.children instanceof Map ? Array.from(node.children.values()) : [];
        const scopeEntries = Array.isArray(node.scopes) ? node.scopes.slice() : [];

        if (!childEntries.length && !scopeEntries.length) {
            return '';
        }

        childEntries.sort((a, b) => (a.key || '').localeCompare(b.key || '', undefined, { sensitivity: 'base' }));
        scopeEntries.sort((a, b) => (a.env || '').localeCompare(b.env || '', undefined, { sensitivity: 'base' }));

        let html = `<ul class="${level > 0 ? 'pl-4' : ''} space-y-1">`;

        childEntries.forEach(child => {
            html += renderSidebarFolderNode(child, level);
        });

        scopeEntries.forEach(scope => {
            html += renderSidebarScopeEntry(scope);
        });

        html += '</ul>';
        return html;
    }

    function renderEnvironmentSuggestions(activeScopeKey = state.selectedScopeKey || DEFAULT_SCOPE_KEY) {
        const panel = DOM['environment-suggestion-panel'];
        const list = DOM['environment-suggestion-list'];
        const emptyState = DOM['environment-suggestion-empty'];
        if (!panel || !list || !emptyState) {
            return;
        }

        const suggestions = [];
        if (state.scopeMap instanceof Map) {
            state.scopeMap.forEach(scope => {
                const variables = Array.isArray(scope.variables) ? scope.variables.filter(Boolean) : [];
                if (!variables.length) return;
                suggestions.push({
                    key: scope.key,
                    label: scope.env ? `/${scope.env}` : '/ (default)',
                    count: variables.length,
                    preview: variables.slice(0, 5),
                });
            });
        }

        if (!suggestions.length) {
            list.innerHTML = '';
            emptyState.classList.remove('hidden');
            return;
        }

        emptyState.classList.add('hidden');
        const normalizedActive = activeScopeKey || DEFAULT_SCOPE_KEY;

        suggestions.sort((a, b) => (a.label || '').localeCompare(b.label || '', undefined, { sensitivity: 'base' }));

        list.innerHTML = suggestions.map(entry => {
            const pills = entry.preview.map(name => {
                const valueAttr = escapeAttribute(name);
                const scopeAttr = escapeAttribute(entry.key);
                return `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-env-suggestion-value="${valueAttr}" data-env-suggestion-scope="${scopeAttr}">${escapeHtml(name)}</button>`;
            });
            const remaining = entry.count - entry.preview.length;
            if (remaining > 0) {
                pills.push(`<span class="env-suggestion-pill env-suggestion-pill--more">+${remaining} more</span>`);
            }
            const countLabel = `${entry.count} ${entry.count === 1 ? 'variable' : 'variables'}`;
            const activeClass = entry.key === normalizedActive ? ' env-suggestion-item--active' : '';
            return `
                <article class="env-suggestion-item${activeClass}">
                    <div class="env-suggestion-env">
                        <span class="env-suggestion-env-label">${escapeHtml(entry.label)}</span>
                        <span class="env-suggestion-env-count">${escapeHtml(countLabel)}</span>
                    </div>
                    <div class="env-suggestion-variables">${pills.join('')}</div>
                </article>
            `;
        }).join('');
    }

    function handleEnvironmentSuggestionClick(event) {
        const button = event.target.closest('[data-env-suggestion-value]');
        if (!button) return;
        if (!DOM['environment-edit-modal'] || DOM['environment-edit-modal'].classList.contains('hidden')) {
            return;
        }
        const variableValue = button.getAttribute('data-env-suggestion-value') || '';
        if (!variableValue) return;
        event.preventDefault();
        const identity = parseVariableIdentity(variableValue);
        const nameInput = DOM['environment-edit-name'];
        if (nameInput && !nameInput.readOnly) {
            nameInput.value = identity.name || variableValue;
            try {
                const caret = nameInput.value.length;
                nameInput.setSelectionRange(caret, caret);
            } catch {}
        }
        const repoInput = DOM['environment-edit-repo'];
        if (repoInput && !repoInput.disabled) {
            const repoSlug = identity.repoOwner && identity.repoName ? `${identity.repoOwner}/${identity.repoName}` : '';
            repoInput.value = repoSlug;
        }
        nameInput?.focus();
    }

    function renderSidebarScopeEntry(scope) {
        if (!scope) return '';
        const isActive = scope.key === state.selectedScopeKey;
        const { title, fullPath } = describeEnvironmentScope(scope);
        const displayLabel = scope.label || title || 'Default';
        const displayPath = fullPath || '/';

        return `<li>
            <button type="button" class="w-full flex items-center gap-2 text-left px-3 py-1.5 rounded-md text-sm transition ${isActive ? 'bg-[var(--bg-hover)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'}" data-environment-sidebar-scope="${escapeAttribute(scope.key)}" aria-label="Open environment ${escapeAttribute(displayPath)}">
                <svg class="h-4 w-4 flex-shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M12 3a9 9 0 100 18 9 9 0 000-18z" />
                    <path d="M3.6 9h16.8M3.6 15h16.8M12 3a15 15 0 010 18M12 3a15 15 0 000 18" />
                </svg>
                <span class="truncate">${escapeHtml(displayLabel)}</span>
            </button>
        </li>`;
    }

    function formatEnvironmentFolderLabel(label) {
        const str = String(label || '').trim();
        if (!str) return 'Folder';
        return str.replace(/[-_]+/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
    }

    function ensureSidebarExpansionForPath(path) {
        if (!(state.sidebarExpanded instanceof Set)) {
            state.sidebarExpanded = new Set(['']);
        }
        if (!path) {
            state.sidebarExpanded.add('');
            return;
        }
        const segments = String(path).split('/').filter(Boolean);
        let current = '';
        segments.forEach(segment => {
            current = current ? `${current}/${segment}` : segment;
            state.sidebarExpanded.add(current);
        });
    }

    function getSelectedEnvironmentLabel() {
        if (!state.selectedScopeKey) return '';
        const scope = state.scopeMap instanceof Map ? state.scopeMap.get(state.selectedScopeKey) : null;
        return scope?.env || '';
    }

    function getEnvironmentFolderKey(envLabel) {
        const normalized = normalizeEnvironmentLabel(envLabel || '');
        if (!normalized) return '';
        const segments = normalized.split('/').filter(Boolean);
        if (segments.length <= 1) return '';
        segments.pop();
        return segments.join('/');
    }

    function shouldExpandFolder(path) {
        const normalized = normalizeEnvironmentLabel(path || '');
        if (!normalized) return true;
        if (!(state.sidebarExpanded instanceof Set)) {
            state.sidebarExpanded = new Set(['']);
        }
        if (state.sidebarExpanded.has(normalized)) {
            return true;
        }
        const active = state.activeFolderKey || '';
        if (active && (active === normalized || active.startsWith(`${normalized}/`))) {
            return true;
        }
        const selectedEnv = getSelectedEnvironmentLabel();
        if (selectedEnv && (selectedEnv === normalized || selectedEnv.startsWith(`${normalized}/`))) {
            return true;
        }
        return false;
    }

    function toggleSidebarFolder(path) {
        const normalized = normalizeEnvironmentLabel(path || '');
        if (!normalized) return;
        if (!(state.sidebarExpanded instanceof Set)) {
            state.sidebarExpanded = new Set(['']);
        }
        if (state.sidebarExpanded.has(normalized)) {
            state.sidebarExpanded.delete(normalized);
        } else {
            state.sidebarExpanded.add(normalized);
        }
        renderSidebarTree();
    }

    function setActiveFolder(path, options = {}) {
        const normalized = normalizeEnvironmentLabel(path || '');
        const changed = state.activeFolderKey !== normalized;
        state.activeFolderKey = normalized;
        if (options.ensure !== false) {
            ensureSidebarExpansionForPath(normalized);
        }
        renderSidebarTree();
        if (changed || options.force || options.refreshList) {
            renderScopeCollection();
        }
    }

    function navigateToScope(scopeKey, variableName = null) {
        const scope = state.scopeMap.get(scopeKey) || state.scopeMap.get(DEFAULT_SCOPE_KEY);
        if (!scope) {
            window.location.hash = '#/environment';
            return;
        }
        const envSegment = encodeScopeSegment(scope?.env || '');
        const variableSegment = variableName ? `/variables/${encodeURIComponent(variableName)}` : '';
        window.location.hash = `#/environment/${envSegment}${variableSegment}`;
    }

    async function selectScope(scopeKey, options = {}) {
        const scope = state.scopeMap.get(scopeKey);
        if (!scope) {
            state.selectedScopeKey = null;
            state.activeFolderKey = '';
            state.expandedVariable = null;
            renderSidebarTree();
            showListView();
            return;
        }

        state.selectedScopeKey = scope.key;
        state.expandedVariable = null;
        const envPath = normalizeEnvironmentLabel(scope.env || '');
        const folderKey = getEnvironmentFolderKey(envPath);
        setActiveFolder(folderKey, { ensure: true, refreshList: false });
        showDetailView();
        await ensureScopeVariablesLoaded(scope, options.forceReload);
        renderScopeDetail(scope);
        highlightActiveEnvironmentCard();

        if (!options.silent && !options.skipHash) {
            navigateToScope(scope.key);
        }
    }

    async function ensureScopeVariablesLoaded(scope, force = false) {
        if (scope.fetchPromise) {
            await scope.fetchPromise;
            return scope.variables;
        }
        if (!force && scope.fetched) {
            return scope.variables;
        }
        if (!context || typeof context.fetchData !== 'function') return [];

        const baseUrl = scope.env ? `/v1/environments?env=${encodeURIComponent(scope.env)}` : '/v1/environments';
        const url = baseUrl.includes('?') ? `${baseUrl}&include_source=true` : `${baseUrl}?include_source=true`;
        scope.fetching = true;
        scope.fetchPromise = (async () => {
            try {
                const result = await context.fetchData(url);
                const entries = Array.isArray(result) ? result : [];
                const normalizedEntries = entries.map(item => {
                    if (typeof item === 'string') {
                        return { name: String(item || '').trim(), source: 'database', createdAt: '', updatedAt: '' };
                    }
                    if (item && typeof item === 'object') {
                        return {
                            name: String(item.name || '').trim(),
                            source: normalizeEnvironmentSourceKey(item.source || 'database'),
                            createdAt: item.created_at || item.createdAt || '',
                            updatedAt: item.updated_at || item.updatedAt || '',
                        };
                    }
                    return { name: '', source: 'database', createdAt: '', updatedAt: '' };
                }).filter(entry => entry.name);

                const globalMeta = state.envSources instanceof Map ? state.envSources : new Map();
                state.envSources = globalMeta;
                if (!(scope.variableMeta instanceof Map)) {
                    scope.variableMeta = new Map();
                }
                if (scope.variableMeta instanceof Map) {
                    scope.variableMeta.forEach((_, varName) => {
                        globalMeta.delete(makeEnvValueCacheKey(varName, scope.env || ''));
                    });
                    scope.variableMeta.clear();
                }

                const names = normalizedEntries.map(entry => {
                    const meta = {
                        source: entry.source,
                        createdAt: entry.createdAt,
                        updatedAt: entry.updatedAt,
                    };
                    scope.variableMeta.set(entry.name, meta);
                    globalMeta.set(makeEnvValueCacheKey(entry.name, scope.env || ''), meta);
                    return entry.name;
                });
                names.sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
                scope.variables = names;
                scope.fetched = true;
                return names;
            } catch (error) {
                console.error('Failed to load environment variables for scope', scope.env, error);
                scope.variables = [];
                return [];
            } finally {
                scope.fetching = false;
                scope.fetchPromise = null;
            }
        })();

        const names = await scope.fetchPromise;
        renderScopeCollection();
        return names;
    }

    function showListView() {
        if (DOM['environment-list-view']) DOM['environment-list-view'].classList.remove('hidden');
        if (DOM['environment-detail-view']) DOM['environment-detail-view'].classList.add('hidden');
        if (DOM['environment-back-btn']) DOM['environment-back-btn'].classList.add('hidden');
        if (DOM['environment-search-container']) DOM['environment-search-container'].classList.remove('hidden');
        if (DOM['environment-create-btn']) DOM['environment-create-btn'].classList.remove('hidden');
        updateCreateEnvironmentButton({ isSearching: state.isSearching && !!(state.searchTerm || '').trim() });
    }

    function showDetailView() {
        if (DOM['environment-list-view']) DOM['environment-list-view'].classList.add('hidden');
        if (DOM['environment-detail-view']) DOM['environment-detail-view'].classList.remove('hidden');
        if (DOM['environment-back-btn']) DOM['environment-back-btn'].classList.remove('hidden');
        if (DOM['environment-search-container']) DOM['environment-search-container'].classList.add('hidden');
        if (DOM['environment-create-btn']) DOM['environment-create-btn'].classList.add('hidden');
        updateCreateEnvironmentButton({ isSearching: false });
    }

    function renderScopeDetail(scope) {
        if (!DOM['environment-detail']) return;
        DOM['environment-detail'].classList.remove('hidden');

        if (DOM['environment-detail-title']) {
            DOM['environment-detail-title'].textContent = scope.env ? `/${scope.env}` : '/';
        }

        renderVariableList(scope);

        if (scope.variables.length) {
            const preferredVariable = scope.variables.includes(state.selectedVariable) ? state.selectedVariable : scope.variables[0];
            selectVariable(preferredVariable, { silent: true });
        } else {
            clearVariableDetail();
        }
    }

    function renderVariableList(scope) {
        const container = DOM['environment-variable-list'];
        if (!container) return;

        const hasVariables = Array.isArray(scope.variables) && scope.variables.length > 0;
        if (DOM['environment-variable-empty']) {
            DOM['environment-variable-empty'].classList.toggle('hidden', hasVariables);
        }

        const grouped = groupVariablesByRepository(hasVariables ? scope.variables : []);
        const sections = [];

        if (grouped.global.length) {
            sections.push(renderVariableSection('', grouped.global, { scopeKey: scope.key }));
        }

        grouped.repositories.forEach(entry => {
            sections.push(renderVariableSection(entry.repo, entry.variables, { scopeKey: scope.key }));
        });

        container.innerHTML = sections.join('');

        container.querySelectorAll('[data-environment-variable]').forEach(button => {
            button.addEventListener('click', () => {
                const name = button.getAttribute('data-environment-variable');
                selectVariable(name);
            });
        });

        container.querySelectorAll('[data-env-variable-show]').forEach(button => {
            button.addEventListener('click', async event => {
                event.preventDefault();
                event.stopPropagation();
                const item = button.closest('[data-env-variable-item]');
                if (!item) return;
                const variableName = item.getAttribute('data-env-variable-full');
                const scopeKey = item.getAttribute('data-env-variable-scope') || scope.key;
                if (state.selectedVariable !== variableName) {
                    await selectVariable(variableName, { silent: true, skipHash: true });
                }
                await toggleVariableValue(scopeKey, variableName, item, button);
            });
        });

        container.querySelectorAll('[data-env-variable-action]').forEach(button => {
            button.addEventListener('click', async event => {
                event.preventDefault();
                event.stopPropagation();
                const action = button.getAttribute('data-env-variable-action');
                const item = button.closest('[data-env-variable-item]');
                if (!item) return;
                const variableName = item.getAttribute('data-env-variable-full');
                const scopeKey = item.getAttribute('data-env-variable-scope') || scope.key;
                if (!variableName) return;
                const scopeRef = state.scopeMap.get(scopeKey) || state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY);
                if (!scopeRef) return;

                if (state.selectedVariable !== variableName) {
                    await selectVariable(variableName, { silent: true, skipHash: true });
                }

                switch (action) {
                    case 'edit':
                        openEditModal('update', { scopeKey: scopeRef.key, name: variableName });
                        break;
                    case 'delete':
                        openDeleteVariableModal(scopeRef, variableName);
                        break;
                    default:
                        break;
                }
            });
        });

        container.querySelectorAll('[data-env-variable-clone]').forEach(button => {
            button.addEventListener('click', async event => {
                event.preventDefault();
                event.stopPropagation();
                const item = button.closest('[data-env-variable-item]');
                if (!item) return;
                const variableName = item.getAttribute('data-env-variable-full');
                const scopeKey = item.getAttribute('data-env-variable-scope') || scope.key;
                if (!variableName) return;
                await cloneEnvironmentVariable(scopeKey, variableName);
            });
        });

        highlightActiveVariable(state.selectedVariable);
        updateVariableItemStates();
        updateVariableItemStates();
    }

    function groupVariablesByRepository(variables) {
        const global = [];
        const repoMap = new Map();

        variables.forEach(name => {
            const trimmed = String(name || '').trim();
            if (!trimmed) return;
            const parts = trimmed.split('/');
            if (parts.length === 3) {
                const repo = `${parts[0]}/${parts[1]}`;
                const varName = parts[2];
                if (!repoMap.has(repo)) {
                    repoMap.set(repo, []);
                }
                repoMap.get(repo).push({ full: trimmed, display: varName });
            } else {
                global.push({ full: trimmed, display: trimmed });
            }
        });

        global.sort((a, b) => a.display.localeCompare(b.display, undefined, { sensitivity: 'base' }));

        const repositories = Array.from(repoMap.entries())
            .map(([repo, vars]) => ({
                repo,
                variables: vars.sort((a, b) => a.display.localeCompare(b.display, undefined, { sensitivity: 'base' })),
            }))
            .sort((a, b) => a.repo.localeCompare(b.repo, undefined, { sensitivity: 'base' }));

        return { global, repositories };
    }

    function normalizeRepositorySlug(raw) {
        if (typeof raw !== 'string') return '';
        const trimmed = raw.trim().replace(/^\/+|\/+$/g, '');
        if (!trimmed) return '';
        const segments = trimmed.split('/').filter(Boolean);
        if (segments.length !== 2) return '';
        const [owner, repo] = segments;
        if (!owner || !repo || /\s/.test(owner) || /\s/.test(repo)) return '';
        return `${owner}/${repo}`;
    }

    function collectKnownRepositorySlugs(extraSlug = '') {
        const repos = new Set();

        if (state.scopeMap instanceof Map) {
            state.scopeMap.forEach(scope => {
                (scope.variables || []).forEach(variable => {
                    const identity = parseVariableIdentity(variable);
                    if (identity.repoOwner && identity.repoName) {
                        repos.add(`${identity.repoOwner}/${identity.repoName}`);
                    }
                });
                (scope.triggers || []).forEach(trigger => {
                    if (!trigger) return;
                    const [owner, repo] = splitSlug(trigger.slug || '');
                    if (owner && repo) {
                        repos.add(`${owner}/${repo}`);
                    }
                });
            });
        }

        if (extraSlug) {
            const normalized = normalizeRepositorySlug(extraSlug);
            if (normalized) repos.add(normalized);
        }

        return Array.from(repos).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
    }

    function populateEnvironmentRepoSuggestions(preselect = '') {
        const datalist = DOM['environment-repo-options'];
        if (!datalist) return;
        const entries = collectKnownRepositorySlugs(preselect);
        datalist.innerHTML = entries.map(slug => `<option value="${escapeAttribute(slug)}"></option>`).join('');
    }

    function renderVariableSection(title, items, options = {}) {
        if (!Array.isArray(items) || !items.length) return '';
        const scopeKey = options.scopeKey || '';
        const scopeRef = state.scopeMap.get(scopeKey) || state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY);

        const groups = items.map(item => {
            const isExpanded = state.expandedVariable === item.full;
            const isActive = item.full === state.selectedVariable;
            const sourceKey = getEnvironmentSourceForVariable(item.full, scopeRef);
            const sourceLabel = formatEnvironmentSourceLabel(sourceKey);
            const isEditable = isEnvironmentSourceEditable(sourceKey);
            const identity = parseVariableIdentity(item.full);

            return `
                <div class="env-variable-item${isActive ? ' env-variable-item--active' : ''}${isExpanded ? ' env-variable-item--expanded' : ''}" data-env-variable-item data-env-variable-full="${escapeAttribute(item.full)}" data-env-variable-scope="${escapeAttribute(scopeKey)}">
                    <div class="env-variable-info">
                        <button type="button" class="env-variable-btn${isActive ? ' env-variable-btn--active' : ''}" data-environment-variable="${escapeAttribute(item.full)}">
                            <span class="truncate">${escapeHtml(item.display)}</span>
                        </button>
                    </div>
                    <div class="env-variable-inline-actions">
                        <button type="button" class="env-inline-icon" data-env-variable-show title="Show value">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-7 11-7 11 7 11 7-4 7-11 7-11-7-11-7z"/><circle cx="12" cy="12" r="3"/></svg>
                        </button>
                        ${isEditable ? `
                        <button type="button" class="env-inline-icon" data-env-variable-action="edit" title="Edit variable">
                            <svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.5L13.196 5.232z" />
                            </svg>
                        </button>
                        <button type="button" class="env-inline-icon env-inline-icon--danger" data-env-variable-action="delete" title="Delete variable">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6l1-3h4l1 3"/></svg>
                        </button>
                        ` : `
                        <button type="button" class="env-inline-icon" data-env-variable-clone="${escapeAttribute(item.full)}" title="Clone">
                            <svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2">
                                <path d="M16 7h-1V4a1 1 0 00-1-1H9a1 1 0 00-1 1v3H7a1 1 0 00-1 1v12a1 1 0 001 1h9a1 1 0 001-1V8a1 1 0 00-1-1zM9 4h5v3H9V4zm2.5 12a2.5 2.5 0 110-5 2.5 2.5 0 010 5z" />
                            </svg>
                        </button>
                        `}
                    </div>
                    <div class="env-variable-value" data-env-variable-value></div>
                </div>`;
        }).join('');

        const heading = title ? `<h4>${escapeHtml(title)}</h4>` : '';

        return `<section class="env-variable-section">
            ${heading}
            <div class="env-variable-buttons">${groups}</div>
        </section>`;
    }

    async function selectVariable(name, options = {}) {
        if (!name) {
            clearVariableDetail();
            return;
        }

        const scope = state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY);
        if (!scope) return;
        if (!scope.variables.includes(name)) {
            if (!options.silent) {
                showToast('Variable not found in selected scope.', 'error');
            }
            return;
        }

        state.selectedVariable = name;
        highlightActiveVariable(name);
        await ensurePipelineEnvIndex();
        renderVariableDetail(scope, name);

        if (!options.silent && !options.skipHash) {
            navigateToScope(scope.key, name);
        }
    }

    function highlightActiveVariable(name) {
        const container = DOM['environment-variable-list'];
        if (!container) return;
        container.querySelectorAll('[data-env-variable-item]').forEach(item => {
            const button = item.querySelector('[data-environment-variable]');
            const value = button ? button.getAttribute('data-environment-variable') : null;
            const isActive = value === name && !!name;
            item.classList.toggle('env-variable-item--active', isActive);
            if (button) {
                button.classList.toggle('env-variable-btn--active', isActive);
            }
        });
        updateVariableItemStates();
    }
    function updateVariableItemStates() {
        const container = DOM['environment-variable-list'];
        if (!container) return;
        container.querySelectorAll('[data-env-variable-item]').forEach(item => {
            const variableFull = item.getAttribute('data-env-variable-full');
            const scopeKey = item.getAttribute('data-env-variable-scope') || '';
            const scopeRef = state.scopeMap.get(scopeKey) || state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY) || null;
            const envLabel = scopeRef?.env || '';
            const cacheKey = makeEnvValueCacheKey(variableFull, envLabel);
            const value = state.envValues.get(cacheKey) || '';
            const isExpanded = state.expandedVariable === variableFull;
            const showButton = item.querySelector('[data-env-variable-show]');
            const valueContainer = item.querySelector('[data-env-variable-value]');

            item.classList.toggle('env-variable-item--expanded', isExpanded);
            if (showButton) {
                showButton.setAttribute('aria-label', isExpanded ? 'Hide value' : 'Show value');
                const isLoading = showButton.dataset.loading === 'true';
                showButton.classList.toggle('loading', isLoading);
                showButton.classList.toggle('env-inline-icon--active', isExpanded);
                showButton.disabled = isLoading;
            }
            if (valueContainer) {
                const displayValue = value ? value : '(empty)';
                if (isExpanded) {
                    valueContainer.textContent = displayValue;
                    valueContainer.setAttribute('title', displayValue);
                } else {
                    valueContainer.textContent = '';
                    valueContainer.removeAttribute('title');
                }
            }
        });
    }

    async function toggleVariableValue(scopeKey, variableName, item, button) {
        const scopeRef = state.scopeMap.get(scopeKey) || state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY) || null;
        if (!scopeRef) return;

        if (state.expandedVariable === variableName) {
            state.expandedVariable = null;
            updateVariableItemStates();
            return;
        }

        const cacheKey = makeEnvValueCacheKey(variableName, scopeRef.env || '');
        let value = state.envValues.get(cacheKey);
        if (value == null) {
            try {
                button.dataset.loading = 'true';
                button.classList.add('loading');
                button.disabled = true;
                value = await fetchVariableValue(scopeRef, variableName);
                state.envValues.set(cacheKey, value ?? '');
            } catch (error) {
                console.error('Failed to fetch environment value:', error);
                if (typeof showToast === 'function') {
                    showToast('Failed to load variable value.', 'error');
                }
            } finally {
                button.disabled = false;
                button.dataset.loading = 'false';
                button.classList.remove('loading');
            }
        }

        state.expandedVariable = variableName;
        updateVariableItemStates();
    }

    async function fetchVariableValue(scope, variableName) {
        if (!context || typeof context.fetchData !== 'function') return '';
        const identity = parseVariableIdentity(variableName);
        const envLabel = scope?.env || '';
        const cacheKey = makeEnvValueCacheKey(variableName, envLabel);

        if (state.envValues.has(cacheKey)) {
            return state.envValues.get(cacheKey);
        }
        if (state.envValuePromises.has(cacheKey)) {
            return state.envValuePromises.get(cacheKey);
        }

        let url = '';
        if (identity.repoOwner && identity.repoName) {
            url = `/v1/repositories/${encodeURIComponent(identity.repoOwner)}/${encodeURIComponent(identity.repoName)}/environments/${encodeURIComponent(identity.name)}`;
        } else {
            url = `/v1/environments/${encodeURIComponent(identity.name)}`;
        }
        if (envLabel) {
            url += `?env=${encodeURIComponent(envLabel)}`;
        }

        const promise = (async () => {
            const { status, data, error } = await fetchEnvironmentResource(url);
            try {
                if (status === 404) {
                    return '';
                }
                if (error) {
                    throw error;
                }
                if (data && typeof data === 'object' && data.value != null) {
                    return String(data.value);
                }
                if (typeof data === 'string') {
                    return data;
                }
                return '';
            } finally {
                state.envValuePromises.delete(cacheKey);
            }
        })();

        state.envValuePromises.set(cacheKey, promise);
        const value = await promise;
        state.envValues.set(cacheKey, value);
        return value;
    }

    function parseVariableIdentity(variableName) {
        const parts = String(variableName || '').split('/').filter(Boolean);
        if (parts.length === 3) {
            return { repoOwner: parts[0], repoName: parts[1], name: parts[2] };
        }
        return { repoOwner: null, repoName: null, name: variableName };
    }

    function makeEnvValueCacheKey(variableName, envLabel) {
        return `${envLabel || ''}::${variableName}`;
    }




    async function fetchEnvironmentResource(url, options = {}) {
        const base = (context && typeof context.apiBaseUrl === 'string') ? context.apiBaseUrl : '';
        const target = `${base || ''}${url}`;
        try {
            const response = await fetch(target, options);
            if (response.status === 404) {
                return { status: 404, data: null };
            }
            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(errorText || `HTTP ${response.status}`);
            }
            if (response.status === 204) {
                return { status: 204, data: null };
            }
            const contentType = (response.headers.get('content-type') || '').toLowerCase();
            const data = contentType.includes('application/json') ? await response.json() : await response.text();
            return { status: response.status, data };
        } catch (error) {
            console.error('Environment fetch failed', { url: target, error });
            return { status: 0, data: null, error };
        }
    }




    async function ensurePipelineEnvIndex() {
        if (state.pipelineEnvIndexReady) return;
        if (state.pipelineEnvIndexPromise) {
            await state.pipelineEnvIndexPromise;
            return;
        }

        state.pipelineEnvIndexPromise = (async () => {
            if (!context || typeof context.fetchData !== 'function') return;
            try {
                const response = await context.fetchData('/v1/pipelines?include_source=true');
                const identifiers = normalizePipelineList(response);
                for (const identifier of identifiers) {
                    const yaml = await context.fetchData(`/v1/pipelines/${identifier.split('/').map(encodeURIComponent).join('/')}`);
                    if (typeof yaml !== 'string') continue;
                    const details = parseYaml(yaml);
                    const vars = extractEnvironmentVariables(details);
                    const seed = state.pipelineMetaSeeds instanceof Map ? state.pipelineMetaSeeds.get(identifier) : null;
                    const meta = buildPipelineMeta(identifier, details, seed);
                    state.pipelineMetadata.set(identifier, meta);
                    vars.forEach(variable => {
                        const key = variable.trim();
                        if (!key) return;
                        const entries = state.pipelineEnvIndex.get(key) || new Set();
                        entries.add(identifier);
                        state.pipelineEnvIndex.set(key, entries);
                    });
                }
                state.pipelineEnvIndexReady = true;
            } catch (error) {
                console.error('Failed to build pipeline environment index:', error);
            }
        })();

        try {
            await state.pipelineEnvIndexPromise;
        } finally {
            state.pipelineEnvIndexPromise = null;
        }
    }

    function normalizePipelineList(response) {
        const seeds = new Map();
        if (!Array.isArray(response)) {
            state.pipelineMetaSeeds = seeds;
            return [];
        }

        const identifiers = [];
        response.forEach(item => {
            if (!item) return;
            let identifier = '';
            if (typeof item === 'string') {
                const rawValue = item.trim();
                identifier = normalizePipelineIdentifier(rawValue);
                if (identifier && !seeds.has(identifier)) {
                    seeds.set(identifier, {
                        path: rawValue.replace(/^\/+/, ''),
                        version: '',
                        source: '',
                    });
                }
            } else if (typeof item === 'object') {
                const rawIdentifier = item.id || item.identifier || item.pipeline || '';
                identifier = normalizePipelineIdentifier(rawIdentifier);
                if (identifier) {
                    const rawPath = typeof item.path === 'string' && item.path.trim()
                        ? item.path.trim()
                        : (typeof item.file === 'string' && item.file.trim() ? item.file.trim() : '');
                    const versionValue = item.version != null ? String(item.version).trim() : '';
                    const sourceValue = item.source != null ? String(item.source).trim() : '';
                    seeds.set(identifier, {
                        path: rawPath.replace(/^\/+/, ''),
                        version: versionValue,
                        source: sourceValue,
                    });
                }
            }

            if (identifier) {
                identifiers.push(identifier);
            }
        });

        state.pipelineMetaSeeds = seeds;
        return Array.from(new Set(identifiers.filter(Boolean))).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
    }

    function extractEnvironmentVariables(details) {
        if (!details || typeof details !== 'object') return [];
        if (Array.isArray(details.environment)) {
            return details.environment.map(value => String(value || '').trim()).filter(Boolean);
        }
        return [];
    }

    function buildPipelineMeta(identifier, details, seed = {}) {
        const normalizedId = String(identifier || '').trim();
        const fallback = parsePipelineIdentifier(normalizedId);
        const name = details?.name || fallback.name || normalizedId;
        const description = details?.description || '';
        const seedPath = typeof seed?.path === 'string' ? seed.path.trim() : '';
        const normalizedPath = seedPath ? seedPath.replace(/^\/+/, '') : fallback.path;
        const detailVersion = details?.version != null ? String(details.version).trim() : '';
        const seedVersion = typeof seed?.version === 'string' ? seed.version.trim() : '';
        const version = detailVersion || seedVersion || 'latest';
        const source = formatPipelineSource(details?.source || seed?.source);

        return {
            identifier: normalizedId,
            name,
            description,
            path: normalizedPath,
            version,
            source,
        };
    }

    function parsePipelineIdentifier(identifier) {
        const trimmed = String(identifier || '').trim().replace(/^\/+|\/+$/g, '');
        if (!trimmed) {
            return { path: '', name: '' };
        }
        const parts = trimmed.split('/');
        const name = parts.pop() || '';
        const path = parts.join('/');
        return { path, name };
    }

    function formatPipelineSource(value) {
        const raw = String(value || '').trim();
        const normalized = raw.toLowerCase();
        switch (normalized) {
            case 'git':
                return 'Git';
            case 'draft':
                return 'Draft';
            case 'local':
                return 'Local';
            case 'database':
                return 'Database';
            case 'config repository':
            case '':
                return 'Config Repository';
            default:
                return raw || 'Config Repository';
        }
    }

    function renderVariableDetail(scope, name) {
        updateVariableDetailMeta(name, scope);
        const pipelineEntries = Array.from(state.pipelineEnvIndex.get(name) || []);
        renderRelatedCollection('environment-variable-pipelines', pipelineEntries.map(renderPipelineDetail), 'No pipelines declare this variable.');

        const relatedTriggers = scope.triggers;
        renderRelatedCollection('environment-variable-triggers', relatedTriggers.map(renderTriggerDetail), 'No triggers reference this scope.');
    }

    function renderPipelineDetail(identifier) {
        const meta = state.pipelineMetadata.get(identifier);
        const title = meta?.name || identifier;
        const description = meta?.description ? escapeHtml(meta.description) : '';
        const pathDisplay = formatPipelinePath(meta?.path);
        const versionDisplay = meta?.version ? String(meta.version) : 'latest';
        const sourceDisplay = meta?.source || 'Config Repository';
        const href = `#/pipelines/${identifier.split('/').map(encodeURIComponent).join('/')}`;
        const descriptionBlock = description ? `<p class="env-related-card__description">${description}</p>` : '';
        const metaRows = [
            { label: 'version', value: versionDisplay },
            { label: 'source', value: sourceDisplay },
        ];
        const metaBlock = metaRows.map(row => `
            <div class="env-related-card__meta-row">
                <span class="env-related-card__meta-label">${escapeHtml(row.label)}:</span>
                <span class="env-related-card__meta-value">${escapeHtml(row.value)}</span>
            </div>
        `).join('');
        return `<a href="${escapeAttribute(href)}" class="env-related-card">
            <div>
                <h5 class="env-related-card__title">${escapeHtml(title)}</h5>
                <p class="env-related-card__path">${escapeHtml(pathDisplay)}</p>
            </div>
            ${descriptionBlock}
            <div class="env-related-card__meta">${metaBlock}</div>
        </a>`;
    }

    function formatPipelinePath(path) {
        if (!path) return '/';
        const normalized = String(path).replace(/^\/+/, '');
        return `/${normalized}`;
    }

    function renderTriggerDetail(trigger) {
        const href = `#/triggers/${trigger.slug.split('/').map(encodeURIComponent).join('/')}`;
        const branches = Array.isArray(trigger.branches) && trigger.branches.length ? trigger.branches.join(', ') : 'All branches';
        const pipelineCount = Array.isArray(trigger.pipelines) ? trigger.pipelines.length : 0;
        const pipelineSummary = pipelineCount ? `${pipelineCount} pipeline${pipelineCount === 1 ? '' : 's'}` : 'No pipelines linked';
        const scopeLabel = trigger.environment ? `/${trigger.environment}` : '/';
        const metaRows = [
            { label: 'event', value: trigger.event || 'custom' },
            { label: 'branches', value: branches },
            { label: 'pipelines', value: pipelineSummary },
        ];
        const metaBlock = metaRows.map(row => `
            <div class="env-related-card__meta-row">
                <span class="env-related-card__meta-label">${escapeHtml(row.label)}:</span>
                <span class="env-related-card__meta-value">${escapeHtml(row.value)}</span>
            </div>
        `).join('');
        return `<a href="${escapeAttribute(href)}" class="env-related-card">
            <div>
                <h5 class="env-related-card__title">${escapeHtml(trigger.slug)}</h5>
                <p class="env-related-card__path">${escapeHtml(scopeLabel)}</p>
            </div>
            <div class="env-related-card__meta">${metaBlock}</div>
        </a>`;
    }

    function renderRelatedCollection(elementId, items, emptyText) {
        const container = document.getElementById(elementId);
        if (!container) return;
        if (!container.classList.contains('env-related-list')) {
            container.classList.add('env-related-list');
        }
        container.dataset.empty = emptyText;
        if (items && items.length) {
            container.innerHTML = items.join('');
        } else {
            container.innerHTML = '';
        }
    }

    function clearVariableDetail() {
        state.selectedVariable = null;
        if (DOM['environment-variable-subtitle']) DOM['environment-variable-subtitle'].textContent = 'Select a variable to inspect details.';
        if (DOM['environment-variable-pipelines']) {
            DOM['environment-variable-pipelines'].innerHTML = '';
            DOM['environment-variable-pipelines'].dataset.empty = 'No pipelines declare this variable.';
        }
        if (DOM['environment-variable-triggers']) {
            DOM['environment-variable-triggers'].innerHTML = '';
            DOM['environment-variable-triggers'].dataset.empty = 'No triggers reference this scope.';
        }
        if (DOM['environment-variable-detail-label']) {
            DOM['environment-variable-detail-label'].textContent = 'Select a variable to inspect details.';
        }
        ['environment-variable-detail-source', 'environment-variable-detail-updated', 'environment-variable-detail-created'].forEach(id => {
            const container = DOM[id];
            if (!container) return;
            const valueEl = container.querySelector('.env-variable-detail-meta-value');
            if (valueEl) {
                valueEl.textContent = '';
                valueEl.className = valueEl.className
                    .split(' ')
                    .filter(cls => !cls.startsWith('env-variable-source-pill'))
                    .join(' ')
                    .trim() || 'env-variable-detail-meta-value';
            }
            container.classList.add('hidden');
        });
    }

    function updateVariableDetailMeta(name, scope) {
        const labelEl = DOM['environment-variable-detail-label'];
        const sourceEl = DOM['environment-variable-detail-source'];
        const updatedEl = DOM['environment-variable-detail-updated'];
        const createdEl = DOM['environment-variable-detail-created'];
        if (!labelEl || !sourceEl || !updatedEl || !createdEl) return;
        const sourceValueEl = sourceEl.querySelector('.env-variable-detail-meta-value');
        const updatedValueEl = updatedEl.querySelector('.env-variable-detail-meta-value');
        const createdValueEl = createdEl.querySelector('.env-variable-detail-meta-value');

        if (!name) {
            labelEl.textContent = 'Select a variable to inspect details.';
            [
                [sourceEl, sourceValueEl],
                [updatedEl, updatedValueEl],
                [createdEl, createdValueEl],
            ].forEach(([container, valueEl]) => {
                if (valueEl) {
                    valueEl.textContent = '';
                    valueEl.className = valueEl.className
                        .split(' ')
                        .filter(cls => !cls.startsWith('env-variable-source-pill'))
                        .join(' ')
                        .trim() || 'env-variable-detail-meta-value';
                }
                container.classList.add('hidden');
            });
            return;
        }

        const meta = getEnvironmentMetadata(name, scope);
        labelEl.textContent = name;

        if (sourceValueEl && meta?.source) {
            const sourceKey = meta.source;
            sourceValueEl.textContent = formatEnvironmentSourceLabel(sourceKey);
            sourceValueEl.className = `env-variable-detail-meta-value env-variable-source-pill env-variable-source-pill--${sourceKey}`;
            sourceEl.classList.remove('hidden');
        } else if (sourceValueEl) {
            sourceValueEl.textContent = '';
            sourceValueEl.className = 'env-variable-detail-meta-value';
            sourceEl.classList.add('hidden');
        }

        if (updatedValueEl && meta?.updatedAt) {
            updatedValueEl.textContent = formatDisplayTimestamp(meta.updatedAt);
            updatedEl.classList.remove('hidden');
        } else if (updatedValueEl) {
            updatedValueEl.textContent = '';
            updatedEl.classList.add('hidden');
        }

        if (createdValueEl && meta?.createdAt) {
            createdValueEl.textContent = formatDisplayTimestamp(meta.createdAt);
            createdEl.classList.remove('hidden');
        } else if (createdValueEl) {
            createdValueEl.textContent = '';
            createdEl.classList.add('hidden');
        }
    }

    async function cloneEnvironmentVariable(scopeKey, variableName) {
        const scope = state.scopeMap.get(scopeKey) || state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY);
        if (!scope) return;
        const identity = parseVariableIdentity(variableName);
        const repoSlug = identity.repoOwner && identity.repoName ? `${identity.repoOwner}/${identity.repoName}` : '';
        const baseName = identity.name || variableName;
        const suggestion = suggestEnvironmentCloneName(scope, repoSlug, baseName);
        let seedValue = '';
        try {
            seedValue = await fetchVariableValue(scope, variableName);
        } catch (error) {
            console.warn('Unable to prefill cloned environment value', error);
        }
        openEditModal('create', {
            scopeKey: scope.key,
            repository: repoSlug,
            nameSuggestion: suggestion,
            valuePreset: seedValue,
        });
    }

    function openNewEnvironmentModal(baseScopeKey = '') {
        const parentLabel = scopeKeyToLabel(baseScopeKey) || normalizeEnvironmentLabel(state.activeFolderKey || '');
        state.pendingEnvironmentParent = parentLabel;
        if (DOM['environment-new-parent']) {
            DOM['environment-new-parent'].textContent = parentLabel ? `Parent: /${parentLabel}` : 'Parent: /';
        }
        if (DOM['environment-new-name']) {
            DOM['environment-new-name'].value = '';
            setTimeout(() => DOM['environment-new-name']?.focus(), 0);
        }
        openModal('environment-new-modal');
    }

    function hideNewEnvironmentModal() {
        state.pendingEnvironmentParent = '';
        if (DOM['environment-new-form']) {
            DOM['environment-new-form'].reset();
        }
        closeModal('environment-new-modal');
    }

    async function handleCreateEnvironment(event) {
        event.preventDefault();
        if (!context || typeof context.fetchData !== 'function') return;

        const parentLabel = state.pendingEnvironmentParent || '';
        const inputEl = DOM['environment-new-name'];
        const rawInput = inputEl ? inputEl.value : '';
        const segments = sanitizeEnvironmentSegments(rawInput);
        if (!segments.length) {
            showToast('Environment name is required.', 'error');
            return;
        }

        const parentSegments = sanitizeEnvironmentSegments(parentLabel);
        const combinedSegments = parentSegments.concat(segments);
        const envLabel = combinedSegments.join('/');
        const normalizedLabel = normalizeEnvironmentLabel(envLabel);

        if (!normalizedLabel) {
            showToast('Environment name is required.', 'error');
            return;
        }

        const newScopeKey = buildScopeKey(normalizedLabel);
        if (state.scopeMap.has(newScopeKey)) {
            showToast(`Environment '/${normalizedLabel}' already exists.`, 'error');
            return;
        }

        const urlBase = `/v1/environments/${encodeURIComponent(SAMPLE_ENVIRONMENT_VARIABLE)}`;
        const sampleValue = SAMPLE_ENVIRONMENT_VALUE.replace('%ENV%', normalizedLabel || 'default');
        const url = normalizedLabel
            ? `${urlBase}?env=${encodeURIComponent(normalizedLabel)}`
            : urlBase;

        try {
            await context.fetchData(url, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ value: sampleValue }),
            });
            showToast(`Environment '/${normalizedLabel}' created.`, 'success');
        } catch (error) {
            console.error('Failed to create environment', error);
            showToast('Failed to create environment.', 'error');
            return;
        }

        hideNewEnvironmentModal();

        await preloadData(true);

        state.activeFolderKey = parentLabel;
        ensureSidebarExpansionForPath(parentLabel);

        if (state.scopeMap.has(newScopeKey)) {
            await selectScope(newScopeKey, { forceReload: true, skipHash: false });
            selectVariable(SAMPLE_ENVIRONMENT_VARIABLE, { silent: true, skipHash: true });
        } else {
            renderScopeCollection();
        }
    }

    function suggestEnvironmentCloneName(scope, repoSlug, baseName) {
        const sanitizedBase = String(baseName || 'variable')
            .trim()
            .replace(/[^A-Za-z0-9_.-]+/g, '-')
            .replace(/^-+|-+$/g, '') || 'variable';
        const normalizedRepo = repoSlug || '';
        const existing = new Set((scope.variables || []).map(name => name.toLowerCase()));
        const buildFull = candidate => (normalizedRepo ? `${normalizedRepo}/${candidate}` : candidate).toLowerCase();
        let candidate = `${sanitizedBase}_copy`;
        let counter = 2;
        while (existing.has(buildFull(candidate))) {
            candidate = `${sanitizedBase}_copy_${counter++}`;
        }
        return candidate;
    }

    function openEditModal(mode, options = {}) {
        const scope = state.scopeMap.get(options.scopeKey || state.selectedScopeKey || DEFAULT_SCOPE_KEY);
        if (!scope) return;

        const header = DOM['environment-edit-modal']?.querySelector('h2');
        if (header) {
            header.textContent = mode === 'update' ? 'Update Variable' : 'Create Variable';
        }

        if (DOM['environment-edit-scope']) {
            DOM['environment-edit-scope'].textContent = scope.env ? `Scope: /${scope.env}` : 'Scope: /';
        }

        const identity = parseVariableIdentity(options.name || '');
        const existingRepoSlug = identity.repoOwner && identity.repoName ? `${identity.repoOwner}/${identity.repoName}` : '';
        const requestedRepoSlug = normalizeRepositorySlug(options.repository || options.repoSlug || '') || existingRepoSlug;
        populateEnvironmentRepoSuggestions(requestedRepoSlug);

        if (DOM['environment-edit-name']) {
            let baseName = '';
            if (mode === 'update') {
                baseName = identity.repoOwner && identity.repoName ? identity.name : (options.name || '');
            } else {
                baseName = typeof options.nameSuggestion === 'string' ? options.nameSuggestion : '';
            }
            DOM['environment-edit-name'].value = baseName;
            DOM['environment-edit-name'].readOnly = mode === 'update';
        }
        if (DOM['environment-edit-repo']) {
            const input = DOM['environment-edit-repo'];
            if (mode === 'update') {
                input.value = existingRepoSlug;
                input.disabled = true;
                input.setAttribute('aria-disabled', 'true');
            } else {
                input.disabled = false;
                input.removeAttribute('aria-disabled');
                input.value = requestedRepoSlug;
            }
        }
        if (DOM['environment-edit-value']) {
            const preset = mode === 'create' && typeof options.valuePreset === 'string'
                ? options.valuePreset
                : '';
            DOM['environment-edit-value'].value = preset;
        }
        if (DOM['environment-edit-form']) {
            DOM['environment-edit-form'].dataset.mode = mode;
            DOM['environment-edit-form'].dataset.scopeKey = scope.key;
            DOM['environment-edit-form'].dataset.variableName = options.name || '';
        }
        if (DOM['environment-edit-submit']) {
            DOM['environment-edit-submit'].textContent = mode === 'update' ? 'Save Value' : 'Create Variable';
        }

        renderEnvironmentSuggestions(scope.key);
        openModal('environment-edit-modal');
    }

    async function handleSubmitVariable(event) {
        event.preventDefault();
        if (!context || typeof context.fetchData !== 'function') return;

        const form = event.currentTarget;
        const mode = form.dataset.mode || 'create';
        const scopeKey = form.dataset.scopeKey || DEFAULT_SCOPE_KEY;
        const nameInputValue = DOM['environment-edit-name']?.value.trim() || '';
        const repoInput = DOM['environment-edit-repo'];
        const value = DOM['environment-edit-value']?.value || '';

        let repoSlug = '';
        if (repoInput && !repoInput.disabled) {
            repoSlug = repoInput.value.trim();
            if (repoSlug) {
                repoSlug = normalizeRepositorySlug(repoSlug);
                if (!repoSlug) {
                    showToast('Repository must use the “owner/repository” format.', 'error');
                    return;
                }
            }
        }

        let variableName = form.dataset.variableName || '';
        let targetRepoOwner = '';
        let targetRepoName = '';
        let targetVarName = '';

        if (mode === 'update') {
            const identity = parseVariableIdentity(variableName || nameInputValue);
            targetVarName = identity.name || nameInputValue;
            targetRepoOwner = identity.repoOwner || '';
            targetRepoName = identity.repoName || '';
            if (targetRepoOwner && targetRepoName) {
                repoSlug = `${targetRepoOwner}/${targetRepoName}`;
            }
            variableName = identity.repoOwner && identity.repoName
                ? `${identity.repoOwner}/${identity.repoName}/${targetVarName}`
                : targetVarName;
        } else {
            if (!nameInputValue) {
                showToast('Variable name is required.', 'error');
                return;
            }
            if (repoSlug && nameInputValue.includes('/')) {
                showToast('Variable name should not include “/” when a repository is selected.', 'error');
                return;
            }
            targetVarName = nameInputValue;
            if (repoSlug) {
                [targetRepoOwner, targetRepoName] = repoSlug.split('/');
            }
            variableName = repoSlug ? `${repoSlug}/${nameInputValue}` : nameInputValue;
        }

        if (!variableName || !targetVarName) {
            showToast('Variable name is required.', 'error');
            return;
        }
        const scope = state.scopeMap.get(scopeKey);
        if (!scope) {
            showToast('Unknown scope selected.', 'error');
            return;
        }
        if (!value && mode === 'create') {
            showToast('Provide a value for the new variable.', 'error');
            return;
        }

        let urlBase = '';
        if (targetRepoOwner && targetRepoName) {
            urlBase = `/v1/repositories/${encodeURIComponent(targetRepoOwner)}/${encodeURIComponent(targetRepoName)}/environments/${encodeURIComponent(targetVarName)}`;
        } else {
            urlBase = `/v1/environments/${encodeURIComponent(targetVarName)}`;
        }
        const url = scope.env ? `${urlBase}?env=${encodeURIComponent(scope.env)}` : urlBase;

        try {
            await context.fetchData(url, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ value }),
            });
            showToast(mode === 'update' ? 'Variable value updated.' : 'Variable created.', 'success');
            const cacheKey = makeEnvValueCacheKey(variableName, scope.env || '');
            state.envValues.delete(cacheKey);
            if (state.envValuePromises instanceof Map) {
                state.envValuePromises.delete(cacheKey);
            }
            closeModal('environment-edit-modal');
            await ensureScopeVariablesLoaded(scope, true);
            renderScopeDetail(scope);
            selectVariable(variableName, { silent: true, skipHash: true });
            renderScopeCollection();
            renderSidebarTree();
        } catch (error) {
            console.error('Failed to save environment variable:', error);
            showToast('Failed to save environment variable.', 'error');
        }
    }

    function openDeleteVariableModal(scope, name) {
        const identity = parseVariableIdentity(name || '');
        const displayName = identity.repoOwner && identity.repoName
            ? `${identity.repoOwner}/${identity.repoName}/${identity.name}`
            : identity.name || name;
        if (DOM['environment-delete-message']) {
            DOM['environment-delete-message'].textContent = `Remove “${displayName}” from ${scope.env ? `/${scope.env}` : '/'} scope?`;
        }
        if (DOM['environment-confirm-delete-btn']) {
            const btn = DOM['environment-confirm-delete-btn'];
            btn.dataset.scopeKey = scope.key;
            btn.dataset.variableName = name;
            btn.dataset.deleteMode = 'variable';
        }
        openModal('environment-delete-modal');
    }

    function openDeleteEnvironmentModal(scopeKey) {
        const scope = state.scopeMap.get(scopeKey);
        if (!scope) return;
        if (scopeHasGitManagedVariables(scope)) {
            showToast('This environment is managed via Git and cannot be deleted here. Clone it in your repository instead.', 'info');
            return;
        }
        const label = scope.env ? `/${scope.env}` : '/';
        if (DOM['environment-delete-message']) {
            DOM['environment-delete-message'].textContent = `Delete environment “${label}”? All variables within this scope will be removed.`;
        }
        if (DOM['environment-confirm-delete-btn']) {
            const btn = DOM['environment-confirm-delete-btn'];
            btn.dataset.scopeKey = scope.key;
            btn.dataset.variableName = '';
            btn.dataset.deleteMode = 'environment';
        }
        openModal('environment-delete-modal');
    }

    async function deleteEnvironmentVariable(scope, name) {
        if (!scope || !name) return false;

        const identity = parseVariableIdentity(name);
        const repoOwner = identity.repoOwner || '';
        const repoName = identity.repoName || '';
        const baseName = identity.name || name;

        let urlBase = '';
        if (repoOwner && repoName) {
            urlBase = `/v1/repositories/${encodeURIComponent(repoOwner)}/${encodeURIComponent(repoName)}/environments/${encodeURIComponent(baseName)}`;
        } else {
            urlBase = `/v1/environments/${encodeURIComponent(baseName)}`;
        }
        const url = scope.env ? `${urlBase}?env=${encodeURIComponent(scope.env)}` : urlBase;

        try {
            await context.deleteData(url);
            const cacheKey = makeEnvValueCacheKey(name, scope.env || '');
            state.envValues.delete(cacheKey);
            if (state.envValuePromises instanceof Map) {
                state.envValuePromises.delete(cacheKey);
            }
            return true;
        } catch (error) {
            console.error('Failed to delete environment variable:', error);
            showToast('Failed to delete variable.', 'error');
            return false;
        }
    }

    async function deleteEnvironmentScope(scope) {
        if (!scope) return;

        const variables = Array.isArray(scope.variables) ? scope.variables.slice() : [];
        for (const name of variables) {
            await deleteEnvironmentVariable(scope, name);
        }

        state.scopeMap.delete(scope.key);
        state.scopes = Array.isArray(state.scopes) ? state.scopes.filter(item => item.key !== scope.key) : [];

        const envLabel = normalizeEnvironmentLabel(scope.env || '');
        const parentSegments = sanitizeEnvironmentSegments(envLabel);
        parentSegments.pop();
        const parentLabel = parentSegments.join('/');
        state.activeFolderKey = parentLabel;

        if (state.selectedScopeKey === scope.key) {
            state.selectedScopeKey = null;
            clearVariableDetail();
            showListView();
        }

        const targetHash = parentLabel ? buildEnvironmentFolderHash(parentLabel) : '#/environment';
        try {
            history.replaceState(null, '', targetHash);
        } catch {
            window.location.hash = targetHash;
        }

        await preloadData(true);

        renderScopeCollection();
        renderSidebarTree();
        showToast(`Environment '/${envLabel || ''}' deleted.`, 'success');
    }

    async function handleConfirmDelete() {
        const button = DOM['environment-confirm-delete-btn'];
        if (!button) return;
        const scopeKey = button.dataset.scopeKey;
        const scope = state.scopeMap.get(scopeKey);
        if (!scope) {
            closeModal('environment-delete-modal');
            return;
        }

        const mode = button.dataset.deleteMode || 'variable';

        if (mode === 'environment') {
            if (scopeHasGitManagedVariables(scope)) {
                showToast('This environment is managed via Git and cannot be deleted here.', 'error');
                closeModal('environment-delete-modal');
                return;
            }
            closeModal('environment-delete-modal');
            await deleteEnvironmentScope(scope);
            return;
        }

        const name = button.dataset.variableName;
        if (!name) {
            closeModal('environment-delete-modal');
            return;
        }

        const success = await deleteEnvironmentVariable(scope, name);
        if (success) {
            closeModal('environment-delete-modal');
            await ensureScopeVariablesLoaded(scope, true);
            renderScopeDetail(scope);
            selectVariable(name, { silent: true, skipHash: true });
            renderScopeCollection();
            renderSidebarTree();
            showToast('Variable removed.', 'success');
        }
    }

    function openModal(id) {
        const modal = document.getElementById(id);
        if (!modal) return;
        modal.classList.remove('hidden');
        requestAnimationFrame(() => modal.classList.add('show'));
    }

    function closeModal(id) {
        const modal = document.getElementById(id);
        if (!modal || modal.classList.contains('hidden')) return;
        modal.classList.remove('show');
        setTimeout(() => modal.classList.add('hidden'), 200);
    }

    async function handleRoute(hash) {
        const info = parseEnvironmentHash(hash);
        await preloadData();

        const normalizedPath = normalizeEnvironmentLabel(info.envPath || '');
        const scopeKey = buildScopeKey(normalizedPath);
        const isRootPath = !info.folderMode && !info.isDefaultScope && normalizedPath === '';

        if (!info.folderMode && normalizedPath && !state.scopeMap.has(scopeKey)) {
            const scope = createScopeRecord(normalizedPath);
            state.scopeMap.set(scope.key, scope);
            state.scopes.push(scope);
            state.filteredScopes = state.scopes.slice();
            state.scopeTree = buildEnvironmentTree(state.scopes);
        }

        const scopeExists = !info.folderMode && !isRootPath && state.scopeMap.has(scopeKey);

        if (scopeExists) {
            await selectScope(scopeKey, { silent: true, skipHash: true });
            if (info.variableName) {
                await selectVariable(info.variableName, { silent: true, skipHash: true });
            }
            return;
        }

        resetEnvironmentSelection({ showList: true });
        setActiveFolder(normalizedPath, { ensure: true, refreshList: true, force: true });
        highlightActiveEnvironmentCard();
    }

    function parseEnvironmentHash(hash) {
        const raw = String(hash || '').replace(/^#/, '');
        const parts = raw.split('/').filter(Boolean);
        const path = parts[0] || 'environment';
        let segments = parts.slice(1);
        let folderMode = false;
        if (segments[0] === 'folder') {
            folderMode = true;
            segments = segments.slice(1);
        }

        const isDefaultScope = !folderMode && segments.length > 0 && segments[0] === 'default';
        const decodedSegments = segments.map((segment, index) => decodeEnvironmentSegment(segment, index, folderMode));
        let variableName = null;
        let envSegments = decodedSegments.slice();
        if (!folderMode && envSegments.length >= 2) {
            const tailFirst = envSegments[envSegments.length - 2];
            if (tailFirst === 'variables') {
                variableName = envSegments[envSegments.length - 1];
                envSegments = envSegments.slice(0, -2);
            }
        }

        const envPath = envSegments.filter(Boolean).join('/');

        return {
            path,
            envPath,
            variableName,
            folderMode,
            isDefaultScope,
        };
    }

    function renderSidebarForRoute() {
        renderSidebarTree();
    }

    function refresh(force = false) {
        preloadData(force).catch(error => console.error('Failed to refresh environment data:', error));
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
        }, 3200);
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
    global.pages.environment = {
        init,
        handleRoute,
        refresh,
        renderSidebarForRoute,
    };
})(window.NopsAI = window.NopsAI || {});
