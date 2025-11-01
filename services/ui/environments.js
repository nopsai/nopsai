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
        pipelineEnvIndexReady: false,
        pipelineEnvIndexPromise: null,
        triggersByEnv: new Map(),
        pipelinesByEnv: new Map(),
        scopeLoadPromise: null,
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
            'environment-detail-title', 'environment-detail-chip', 'environment-variable-list', 'environment-variable-empty',
            'environment-variable-title', 'environment-variable-subtitle', 'environment-variable-pipelines',
            'environment-variable-triggers', 'environment-variable-actions', 'environment-create-btn', 'environment-sidebar-tree', 'environment-scope-stats',
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
        } else if (!force && state.scopes.length) {
            return;
        }

        state.scopeLoadPromise = (async () => {
            await ensureScopesLoaded();
            await loadTriggerSummaries();
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

            html += renderEnvironmentBreadcrumbs(activeFolder);

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
                    DOM['environment-list-empty'].innerHTML = `<p class="text-sm">No environment scopes matched "${escapeHtml(searchTerm)}".</p>`;
                } else if (state.activeFolderKey) {
                    DOM['environment-list-empty'].innerHTML = '<p class="text-sm">No environment scopes in this folder.</p>';
                } else {
                    DOM['environment-list-empty'].innerHTML = '<p class="text-sm">No environment scopes found. Sync your configuration repository to import them.</p>';
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
            <article class="pipeline-folder-card" data-environment-folder="${keyAttr}" tabindex="0" role="button" aria-label="Open folder ${ariaLabel}">
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
                    <div class="pipeline-folder-meta-row"><span class="pipeline-folder-meta-label">Scopes:</span><span class="pipeline-folder-meta-value">${totalScopes}</span></div>
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
        const pipelineCount = Array.isArray(scope.pipelines) ? scope.pipelines.length : 0;

        return `
            <article class="pipeline-card triggers-card bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded-lg transition-all duration-200 p-3 flex flex-col${isActive ? ' triggers-card--active' : ''}" data-environment-scope="${escapeAttribute(scope.key)}" tabindex="0" role="button" aria-label="Open environment ${escapeAttribute(fullPath)}">
                <div class="pipeline-card-header flex items-start justify-between gap-3">
                    <div class="pipeline-card-info">
                        <span class="triggers-card-icon" aria-hidden="true">
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M12 2a10 10 0 100 20 10 10 0 000-20z" />
                                <path d="M2 12h20" />
                                <path d="M12 2c3.5 4 3.5 16 0 20" />
                                <path d="M12 2c-3.5 4-3.5 16 0 20" />
                            </svg>
                        </span>
                        <div class="pipeline-card-text min-w-0">
                            <h3 class="pipeline-card-title" title="${escapeAttribute(fullPath)}">${escapeHtml(title)}</h3>
                            <p class="pipeline-card-path" title="${escapeAttribute(parentPath)}">${escapeHtml(parentPath)}</p>
                        </div>
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
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">Pipelines</span>
                        <span class="pipeline-card-meta-value">${pipelineCount}</span>
                    </div>
                </div>
            </article>`;
    }

    function renderEnvironmentSearchSummary(count, term) {
        const safeTerm = escapeHtml(term);
        return `<div class="triggers-search-summary">Showing ${count} result${count === 1 ? '' : 's'} for "${safeTerm}"</div>`;
    }

    function buildEnvironmentBreadcrumbs(folderKey) {
        const crumbs = [{ label: 'All environments', key: '' }];
        const normalized = normalizeEnvironmentLabel(folderKey || '');
        if (!normalized) {
            return crumbs;
        }
        const segments = normalized.split('/').filter(Boolean);
        let path = '';
        segments.forEach(segment => {
            path = path ? `${path}/${segment}` : segment;
            crumbs.push({ label: segment, key: path });
        });
        return crumbs;
    }

    function renderEnvironmentBreadcrumbs(folderKey) {
        const crumbs = buildEnvironmentBreadcrumbs(folderKey);
        if (!crumbs.length) return '';
        const items = crumbs.map((crumb, index) => {
            const isCurrent = index === crumbs.length - 1;
            if (isCurrent) {
                return `<span class="pipeline-folder-crumb pipeline-folder-crumb--current">${escapeHtml(crumb.label)}</span>`;
            }
            return `<button type="button" class="pipeline-folder-crumb" data-environment-nav="${escapeAttribute(crumb.key)}">${escapeHtml(crumb.label)}</button><span class="pipeline-folder-crumb-separator" aria-hidden="true">/</span>`;
        }).join('');
        return `<nav class="pipeline-folder-breadcrumb" aria-label="Environment folders">${items}</nav>`;
    }

    function highlightActiveEnvironmentCard() {
        const list = DOM['environment-list'];
        if (!list) return;
        const isListVisible = !DOM['environment-list-view'] || !DOM['environment-list-view'].classList.contains('hidden');
        list.querySelectorAll('[data-environment-scope]').forEach(card => {
            const key = card.getAttribute('data-environment-scope');
            const shouldHighlight = isListVisible && key === state.selectedScopeKey;
            card.classList.toggle('triggers-card--active', shouldHighlight);
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
        clearVariableDetail();
        if (options.showList !== false) {
            showListView();
        }
        highlightActiveEnvironmentCard();
    }

    function handleEnvironmentListClick(event) {
        const breadcrumb = event.target.closest('[data-environment-nav]');
        if (breadcrumb) {
            event.preventDefault();
            const key = breadcrumb.getAttribute('data-environment-nav') || '';
            resetEnvironmentSelection({ showList: true });
            navigateToFolder(key);
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

        const breadcrumb = event.target.closest('[data-environment-nav]');
        if (breadcrumb && breadcrumb === document.activeElement) {
            event.preventDefault();
            const key = breadcrumb.getAttribute('data-environment-nav') || '';
            resetEnvironmentSelection({ showList: true });
            navigateToFolder(key);
            focusFirstEnvironmentCard();
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
        const container = DOM['environment-sidebar-tree'];
        if (!container) return;

        const tree = state.scopeTree || buildEnvironmentTree(state.scopes);
        const rootScopes = Array.isArray(tree?.scopes) ? tree.scopes.slice() : [];
        const childNodes = tree?.children instanceof Map ? Array.from(tree.children.values()) : [];

        rootScopes.sort((a, b) => (a.env || '').localeCompare(b.env || '', undefined, { sensitivity: 'base' }));
        childNodes.sort((a, b) => (a.key || '').localeCompare(b.key || '', undefined, { sensitivity: 'base' }));

        let html = '<ul class="space-y-1">';

        rootScopes.forEach(scope => {
            html += renderSidebarScopeEntry(scope);
        });

        childNodes.forEach(child => {
            const hasNestedFolders = child.children instanceof Map && child.children.size > 0;
            const childScopes = Array.isArray(child.scopes) ? child.scopes.slice() : [];

            if (!hasNestedFolders && childScopes.length) {
                childScopes.sort((a, b) => (a.env || '').localeCompare(b.env || '', undefined, { sensitivity: 'base' }));
                childScopes.forEach(scope => {
                    html += renderSidebarScopeEntry(scope);
                });
                return;
            }

            html += renderSidebarFolderNode(child, 0);
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
        const folderLabel = formatEnvironmentFolderLabel(folderPath);
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

        scopeEntries.forEach(scope => {
            html += renderSidebarScopeEntry(scope);
        });

        childEntries.forEach(child => {
            const hasNestedFolders = child.children instanceof Map && child.children.size > 0;
            const childScopes = Array.isArray(child.scopes) ? child.scopes.slice() : [];

            if (!hasNestedFolders && childScopes.length) {
                childScopes.sort((a, b) => (a.env || '').localeCompare(b.env || '', undefined, { sensitivity: 'base' }));
                childScopes.forEach(scope => {
                    html += renderSidebarScopeEntry(scope);
                });
                return;
            }

            html += renderSidebarFolderNode(child, level);
        });

        html += '</ul>';
        return html;
    }

    function renderSidebarScopeEntry(scope) {
        const isActive = scope.key === state.selectedScopeKey;
        const pathLabel = scope?.env ? `/${scope.env}` : '/';
        return `<li>
            <button type="button" class="w-full text-left px-3 py-1.5 rounded-md text-sm transition ${isActive ? 'bg-[var(--bg-hover)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'}" data-environment-sidebar-scope="${escapeAttribute(scope.key)}">
                <span class="block truncate">${escapeHtml(pathLabel)}</span>
            </button>
        </li>`;
    }

    function formatEnvironmentFolderLabel(label) {
        const str = String(label || '').trim();
        if (!str) return '/';
        return `/${str.replace(/^\/+/, '')}`;
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
            renderSidebarTree();
            showListView();
            return;
        }

        state.selectedScopeKey = scope.key;
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
        if (DOM['environment-detail-chip']) {
            DOM['environment-detail-chip'].textContent = scope.env ? `/${scope.env}` : '/';
        }

        renderVariableList(scope);
        renderScopeStats(scope);

        if (scope.variables.length) {
            const preferredVariable = scope.variables.includes(state.selectedVariable) ? state.selectedVariable : scope.variables[0];
            selectVariable(preferredVariable, { silent: true });
        } else {
            clearVariableDetail();
        }
    }

    function renderScopeStats(scope) {
        if (!DOM['environment-scope-stats']) return;
        const variablesCount = scope.variables.length;
        const triggersCount = scope.triggerCount || 0;
        DOM['environment-scope-stats'].innerHTML = `
            <div>
                <dt class="text-[var(--text-secondary)] text-xs uppercase tracking-wide">Variables</dt>
                <dd class="text-base font-semibold text-[var(--text-primary)]">${variablesCount}</dd>
            </div>
            <div>
                <dt class="text-[var(--text-secondary)] text-xs uppercase tracking-wide">Triggers</dt>
                <dd class="text-base font-semibold text-[var(--text-primary)]">${triggersCount}</dd>
            </div>`;
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

        container.innerHTML = scope.variables.map(name => {
            const isActive = name === state.selectedVariable;
            return `<button type="button" class="w-full text-left px-3 py-2 rounded-md text-sm transition ${isActive ? 'bg-[var(--bg-hover)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'}" data-environment-variable="${escapeAttribute(name)}">
                <span class="block truncate">${escapeHtml(name)}</span>
            </button>`;
        }).join('');

        container.querySelectorAll('[data-environment-variable]').forEach(button => {
            button.addEventListener('click', () => {
                const name = button.getAttribute('data-environment-variable');
                selectVariable(name);
            });
        });
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
        container.querySelectorAll('[data-environment-variable]').forEach(button => {
            const isActive = button.getAttribute('data-environment-variable') === name;
            button.classList.toggle('bg-[var(--bg-hover)]', isActive);
            button.classList.toggle('text-[var(--text-primary)]', isActive);
        });
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
                    const meta = buildPipelineMeta(identifier, details);
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
        if (!Array.isArray(response)) return [];
        const identifiers = [];
        response.forEach(item => {
            if (!item) return;
            if (typeof item === 'string') {
                identifiers.push(normalizePipelineIdentifier(item));
            } else if (typeof item === 'object') {
                const identifier = item.id || item.identifier || item.pipeline || '';
                if (identifier) identifiers.push(normalizePipelineIdentifier(identifier));
            }
        });
        return Array.from(new Set(identifiers.filter(Boolean))).sort((a, b) => a.localeCompare(b));
    }

    function extractEnvironmentVariables(details) {
        if (!details || typeof details !== 'object') return [];
        if (Array.isArray(details.environment)) {
            return details.environment.map(value => String(value || '').trim()).filter(Boolean);
        }
        return [];
    }

    function buildPipelineMeta(identifier, details) {
        const segments = identifier.split('/');
        const name = details?.name || segments[segments.length - 1] || identifier;
        const description = details?.description || '';
        return { identifier, name, description };
    }

    function renderVariableDetail(scope, name) {
        if (!DOM['environment-variable-title']) return;
        DOM['environment-variable-title'].textContent = name;
        if (DOM['environment-variable-subtitle']) {
            DOM['environment-variable-subtitle'].textContent = scope.env ? `Environment scope: /${scope.env}` : 'Environment scope: /';
        }

        if (DOM['environment-variable-actions']) {
            DOM['environment-variable-actions'].classList.remove('hidden');
            const editBtn = DOM['environment-variable-actions'].querySelector('[data-environment-edit]');
            const deleteBtn = DOM['environment-variable-actions'].querySelector('[data-environment-delete]');
            if (editBtn) {
                editBtn.onclick = () => openEditModal('update', { scopeKey: scope.key, name });
            }
            if (deleteBtn) {
                deleteBtn.onclick = () => openDeleteModal(scope, name);
            }
        }

        const pipelineEntries = Array.from(state.pipelineEnvIndex.get(name) || []);
        if (DOM['environment-variable-pipelines']) {
            DOM['environment-variable-pipelines'].innerHTML = pipelineEntries.length
                ? pipelineEntries.map(identifier => renderPipelineDetail(identifier)).join('')
                : '<p class="text-sm text-[var(--text-secondary)]">No pipelines declare this variable.</p>';
        }

        const relatedTriggers = scope.triggers;
        if (DOM['environment-variable-triggers']) {
            DOM['environment-variable-triggers'].innerHTML = relatedTriggers.length
                ? relatedTriggers.map(trigger => renderTriggerDetail(trigger)).join('')
                : '<p class="text-sm text-[var(--text-secondary)]">No triggers reference this scope.</p>';
        }
    }

    function renderPipelineDetail(identifier) {
        const meta = state.pipelineMetadata.get(identifier);
        const title = meta?.name || identifier;
        const description = meta?.description ? `<p class="text-xs text-[var(--text-secondary)] truncate">${escapeHtml(meta.description)}</p>` : '';
        const href = `#/pipelines/${identifier.split('/').map(encodeURIComponent).join('/')}`;
        return `<a href="${escapeAttribute(href)}" class="block rounded-md border border-[var(--border-primary)] p-3 hover:border-[var(--border-accent)] hover:bg-[var(--bg-tertiary)] transition">
            <div class="text-sm font-medium text-[var(--text-primary)] truncate">${escapeHtml(title)}</div>
            ${description}
        </a>`;
    }

    function renderTriggerDetail(trigger) {
        const href = `#/triggers/${trigger.slug.split('/').map(encodeURIComponent).join('/')}`;
        const branches = Array.isArray(trigger.branches) && trigger.branches.length ? trigger.branches.join(', ') : 'All branches';
        return `<a href="${escapeAttribute(href)}" class="block rounded-md border border-[var(--border-primary)] p-3 hover:border-[var(--border-accent)] hover:bg-[var(--bg-tertiary)] transition">
            <div class="text-sm font-medium text-[var(--text-primary)] truncate">${escapeHtml(trigger.slug)}</div>
            <p class="text-xs text-[var(--text-secondary)]">Event: ${escapeHtml(trigger.event || 'custom')}</p>
            <p class="text-xs text-[var(--text-secondary)] truncate">Branches: ${escapeHtml(branches)}</p>
        </a>`;
    }

    function clearVariableDetail() {
        state.selectedVariable = null;
        if (DOM['environment-variable-title']) DOM['environment-variable-title'].textContent = '';
        if (DOM['environment-variable-subtitle']) DOM['environment-variable-subtitle'].textContent = 'Select a variable to see details.';
        if (DOM['environment-variable-pipelines']) DOM['environment-variable-pipelines'].innerHTML = '';
        if (DOM['environment-variable-triggers']) DOM['environment-variable-triggers'].innerHTML = '';
        if (DOM['environment-variable-actions']) DOM['environment-variable-actions'].classList.add('hidden');
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
