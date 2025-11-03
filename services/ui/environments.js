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
            'environment-variable-triggers', 'environment-create-btn', 'environment-sidebar-tree',
            'environment-edit-modal', 'environment-edit-form', 'environment-edit-name', 'environment-edit-value',
            'environment-edit-scope', 'environment-edit-submit', 'environment-delete-modal', 'environment-delete-message',
            'environment-confirm-delete-btn'
        ];

        ids.forEach(id => {
            DOM[id] = document.getElementById(id);
        });
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
        if (DOM['environment-edit-form']) {
            DOM['environment-edit-form'].addEventListener('submit', handleSubmitVariable);
        }
        const cancelEditBtn = DOM['environment-edit-modal']?.querySelector('[data-cancel]');
        if (cancelEditBtn) {
            cancelEditBtn.addEventListener('click', () => closeModal('environment-edit-modal'));
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
            const folderCards = childNodes.map(renderEnvironmentFolderCard).filter(Boolean);
            if (folderCards.length) {
                html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${folderCards.join('')}</div>`;
            }

            const scopes = Array.isArray(node?.scopes) ? node.scopes.slice() : [];
            scopes.sort((a, b) => (a.env || '').localeCompare(b.env || '', undefined, { sensitivity: 'base' }));
            const scopeCards = scopes.map(renderEnvironmentCard);
            if (scopeCards.length) {
                html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${scopeCards.join('')}</div>`;
            }

            if (!folderCards.length && !scopeCards.length) {
                showEmpty = true;
            }
        }

        container.innerHTML = html;
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
                    <h3 class="pipeline-folder-title" title="${escapeAttribute(displayPath)}">${escapeHtml(label)}</h3>
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
                            <h3 class="pipeline-card-title" title="${escapeAttribute(title)}">${escapeHtml(title)}</h3>
                            <p class="pipeline-card-path" title="${escapeAttribute(parentPath)}">${escapeHtml(parentPath)}</p>
                        </div>
                    </div>
                    <div class="pipeline-card-actions">
                        <button class="pipelines-delete-button" type="button" data-environment-delete="${escapeAttribute(scope.key)}" title="Delete environment scope">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                                <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                                <path d="M4 7h16" />
                            </svg>
                        </button>
                    </div>
                </div>
                <div class="pipeline-card-meta">
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">Variables</span>
                        <span class="pipeline-card-meta-value" title="${variableCount} variables">${variableCount}</span>
                    </div>
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">Triggers</span>
                        <span class="pipeline-card-meta-value" title="${triggerCount} triggers">${triggerCount}</span>
                    </div>
                </div>
            </article>`;
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

    function renderSidebarScopeEntry(scope) {
        if (!scope) return '';
        const isActive = scope.key === state.selectedScopeKey;
        const { title, fullPath } = describeEnvironmentScope(scope);
        const displayLabel = scope.label || title || 'Default';
        const displayPath = fullPath || '/';

        return `<li>
            <button type="button" class="w-full flex items-center gap-2 text-left px-3 py-1.5 rounded-md text-sm transition ${isActive ? 'bg-[var(--bg-hover)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'}" data-environment-sidebar-scope="${escapeAttribute(scope.key)}" title="${escapeAttribute(displayPath)}" aria-label="Open environment ${escapeAttribute(displayPath)}">
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

        const url = scope.env ? `/v1/environments?env=${encodeURIComponent(scope.env)}` : '/v1/environments';
        scope.fetching = true;
        scope.fetchPromise = (async () => {
            try {
                const result = await context.fetchData(url);
                const names = Array.isArray(result) ? result.map(item => String(item || '').trim()).filter(Boolean) : [];
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
    }

    function showDetailView() {
        if (DOM['environment-list-view']) DOM['environment-list-view'].classList.add('hidden');
        if (DOM['environment-detail-view']) DOM['environment-detail-view'].classList.remove('hidden');
        if (DOM['environment-back-btn']) DOM['environment-back-btn'].classList.remove('hidden');
        if (DOM['environment-search-container']) DOM['environment-search-container'].classList.add('hidden');
        if (DOM['environment-create-btn']) DOM['environment-create-btn'].classList.add('hidden');
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

        if (!scope.variables.length) {
            if (DOM['environment-variable-empty']) DOM['environment-variable-empty'].classList.remove('hidden');
            container.innerHTML = '';
            return;
        }

        if (DOM['environment-variable-empty']) DOM['environment-variable-empty'].classList.add('hidden');

        const grouped = groupVariablesByRepository(scope.variables);
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
                        openDeleteModal(scopeRef, variableName);
                        break;
                    default:
                        break;
                }
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

    function renderVariableSection(title, items, options = {}) {
        if (!Array.isArray(items) || !items.length) return '';
        const scopeKey = options.scopeKey || '';
        const groups = items.map(item => {
            const isExpanded = state.expandedVariable === item.full;
            const isActive = item.full === state.selectedVariable;
            return `
                <div class="env-variable-item${isActive ? ' env-variable-item--active' : ''}${isExpanded ? ' env-variable-item--expanded' : ''}" data-env-variable-item data-env-variable-full="${escapeAttribute(item.full)}" data-env-variable-scope="${escapeAttribute(scopeKey)}">
                    <button type="button" class="env-variable-btn${isActive ? ' env-variable-btn--active' : ''}" data-environment-variable="${escapeAttribute(item.full)}">
                        <span class="truncate">${escapeHtml(item.display)}</span>
                    </button>
                    <div class="env-variable-inline-actions">
                        <button type="button" class="env-inline-icon" data-env-variable-show>
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-7 11-7 11 7 11 7-4 7-11 7-11-7-11-7z"/><circle cx="12" cy="12" r="3"/></svg>
                        </button>
                        <button type="button" class="env-inline-icon" data-env-variable-action="edit">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 113 3L7 19l-4 1 1-4 12.5-12.5z"/></svg>
                        </button>
                        <button type="button" class="env-inline-icon env-inline-icon--danger" data-env-variable-action="delete">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6l1-3h4l1 3"/></svg>
                        </button>
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
            try {
                const result = await context.fetchData(url);
                if (result && typeof result === 'object' && result.value != null) {
                    return String(result.value);
                }
                if (typeof result === 'string') {
                    return result;
                }
                return '';
            } catch (error) {
                console.error('Failed to retrieve environment value:', error);
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

        if (DOM['environment-edit-name']) {
            DOM['environment-edit-name'].value = mode === 'update' ? options.name || '' : '';
            DOM['environment-edit-name'].readOnly = mode === 'update';
        }
        if (DOM['environment-edit-value']) {
            DOM['environment-edit-value'].value = '';
        }
        if (DOM['environment-edit-form']) {
            DOM['environment-edit-form'].dataset.mode = mode;
            DOM['environment-edit-form'].dataset.scopeKey = scope.key;
            DOM['environment-edit-form'].dataset.variableName = options.name || '';
        }
        if (DOM['environment-edit-submit']) {
            DOM['environment-edit-submit'].textContent = mode === 'update' ? 'Save Value' : 'Create Variable';
        }

        openModal('environment-edit-modal');
    }

    async function handleSubmitVariable(event) {
        event.preventDefault();
        if (!context || typeof context.fetchData !== 'function') return;

        const form = event.currentTarget;
        const mode = form.dataset.mode || 'create';
        const scopeKey = form.dataset.scopeKey || DEFAULT_SCOPE_KEY;
        const variableName = form.dataset.variableName || DOM['environment-edit-name']?.value.trim();
        const value = DOM['environment-edit-value']?.value || '';
        if (!variableName) {
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

        const urlBase = `/v1/environments/${encodeURIComponent(variableName)}`;
        const url = scope.env ? `${urlBase}?env=${encodeURIComponent(scope.env)}` : urlBase;

        try {
            await context.fetchData(url, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ value }),
            });
            showToast(mode === 'update' ? 'Variable value updated.' : 'Variable created.', 'success');
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

    function openDeleteModal(scope, name) {
        if (DOM['environment-delete-message']) {
            DOM['environment-delete-message'].textContent = `Remove “${name}” from ${scope.env ? `/${scope.env}` : '/'} scope?`;
        }
        if (DOM['environment-confirm-delete-btn']) {
            DOM['environment-confirm-delete-btn'].dataset.scopeKey = scope.key;
            DOM['environment-confirm-delete-btn'].dataset.variableName = name;
        }
        openModal('environment-delete-modal');
    }

    async function handleConfirmDelete() {
        if (!context || typeof context.deleteData !== 'function') return;
        const button = DOM['environment-confirm-delete-btn'];
        if (!button) return;
        const scopeKey = button.dataset.scopeKey;
        const name = button.dataset.variableName;
        const scope = state.scopeMap.get(scopeKey);
        if (!scope || !name) return;

        const urlBase = `/v1/environments/${encodeURIComponent(name)}`;
        const url = scope.env ? `${urlBase}?env=${encodeURIComponent(scope.env)}` : urlBase;

        try {
            await context.deleteData(url);
            showToast('Variable removed.', 'success');
            closeModal('environment-delete-modal');
            await ensureScopeVariablesLoaded(scope, true);
            renderScopeDetail(scope);
            renderScopeCollection();
            renderSidebarTree();
        } catch (error) {
            console.error('Failed to delete environment variable:', error);
            showToast('Failed to delete variable.', 'error');
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
