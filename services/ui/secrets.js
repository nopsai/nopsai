(function (global) {
    const state = {
        scopes: [],
        scopeMap: new Map(),
        scopeTree: null,
        filteredScopes: [],
        searchTerm: '',
        isSearching: false,
        selectedScopeKey: null,
        selectedSecret: null,
        selectedVariable: null,
        selectedScopeCategory: 'secrets',
        activeFolderKey: '',
        sidebarExpanded: new Set(),
        pipelineSecretIndex: new Map(),
        secretSources: new Map(),
        pipelineVariableIndex: new Map(),
        variableSources: new Map(),
        variableValues: new Map(),
        variableValuePromises: new Map(),
        pipelineMetadata: new Map(),
        pipelineMetaSeeds: new Map(),
        pipelineSecretIndexReady: false,
        pipelineSecretIndexPromise: null,
        pipelineVariableIndexReady: false,
        pipelineVariableIndexPromise: null,
        expandedVariable: null,
        pendingScopeParent: '',
        triggersByScope: new Map(),
        pipelinesByScope: new Map(),
        scopeLoadPromise: null,
    };

    const DOM = {};
    let context = null;
    let initialized = false;

    const DEFAULT_SCOPE_KEY = buildScopeKey('', '');
    const SAMPLE_SCOPE_VARIABLE = 'sample_variable';
    const SAMPLE_SCOPE_VALUE = 'Replace with your %SCOPE% scope value.';
    const SAMPLE_SCOPE_SECRET = 'sample_secret';
    const SAMPLE_SCOPE_SECRET_VALUE = 'Replace with your %SCOPE% secret value.';

    function init(ctx) {
        if (initialized && context === ctx) {
            return;
        }
        context = ctx;
        mapDomReferences();
        setSelectedScopeCategory(state.selectedScopeCategory || 'secrets');
        attachEventListeners();
        initialized = true;
        preloadData().catch(error => console.error('Failed to preload secret scopes:', error));
    }

    function mapDomReferences() {
        const ids = [
            'secret-search', 'secret-search-container', 'secret-clear-search', 'secret-list', 'secret-list-empty',
            'secret-list-view', 'secret-detail-view', 'secret-detail', 'secret-back-btn',
            'secret-detail-title', 'secret-variable-list', 'secret-variable-empty',
            'secret-variable-pipelines',
            'secret-variable-triggers', 'scope-create-btn',
            'secret-edit-modal', 'secret-edit-form', 'secret-edit-name', 'secret-edit-repo', 'secret-edit-value',
            'secret-edit-scope', 'secret-edit-submit', 'secret-delete-modal', 'secret-delete-message',
            'secret-confirm-delete-btn', 'secret-variable-detail-label', 'secret-variable-detail-source',
            'secret-variable-detail-updated', 'secret-variable-detail-created',
            'secret-suggestion-panel', 'secret-suggestion-list', 'secret-suggestion-empty',
            'variable-list', 'variable-empty',
            'variable-pipelines', 'variable-triggers',
            'variable-edit-modal', 'variable-edit-form', 'variable-edit-name', 'variable-edit-repo', 'variable-edit-value',
            'variable-edit-scope', 'variable-edit-submit', 'variable-delete-modal', 'variable-delete-message',
            'variable-confirm-delete-btn', 'variable-detail-label', 'variable-detail-source',
            'variable-detail-updated', 'variable-detail-created',
            'scope-new-modal', 'scope-new-form', 'scope-new-name', 'scope-new-parent', 'scope-new-cancel', 'scope-new-close',
            'variable-suggestion-panel', 'variable-suggestion-list', 'variable-suggestion-empty'
        ];

        ids.forEach(id => {
            DOM[id] = document.getElementById(id);
        });

        DOM['secret-repo-options'] = document.getElementById('secret-repo-options');
        DOM['secret-create-inline'] = document.getElementById('secret-create-inline');
        DOM['variable-repo-options'] = document.getElementById('variable-repo-options');
        DOM['variable-create-inline'] = document.getElementById('variable-create-inline');
    }

    function attachEventListeners() {
        if (DOM['secret-search']) {
            DOM['secret-search'].addEventListener('input', handleSearchInput);
        }
        if (DOM['secret-clear-search']) {
            DOM['secret-clear-search'].addEventListener('click', clearSearch);
        }
        if (DOM['secret-back-btn']) {
            DOM['secret-back-btn'].addEventListener('click', () => {
                resetSecretSelection({ showList: true });
                window.location.hash = '#/scopes';
            });
        }
        if (DOM['scope-create-btn']) {
            DOM['scope-create-btn'].addEventListener('click', () => {
                const scopeKey = state.selectedScopeKey || DEFAULT_SCOPE_KEY;
                const scope = state.scopeMap instanceof Map ? state.scopeMap.get(scopeKey) : null;
                const parentPath = getScopeParentFolderPath(scope) || state.activeFolderKey || '';
                openNewScopeModal(parentPath);
            });
        }
        if (DOM['secret-create-inline']) {
            DOM['secret-create-inline'].addEventListener('click', (event) => {
                event.preventDefault();
                const scopeKey = state.selectedScopeKey || DEFAULT_SCOPE_KEY;
                openEditModal('create', { scopeKey });
            });
        }
        if (DOM['secret-suggestion-panel']) {
            DOM['secret-suggestion-panel'].addEventListener('click', handleSecretSuggestionClick);
        }
        if (DOM['secret-edit-form']) {
            DOM['secret-edit-form'].addEventListener('submit', handleSubmitSecret);
        }
        const cancelEditBtn = DOM['secret-edit-modal']?.querySelector('[data-cancel]');
        if (cancelEditBtn) {
            cancelEditBtn.addEventListener('click', () => closeModal('secret-edit-modal'));
        }
        if (DOM['secret-confirm-delete-btn']) {
            DOM['secret-confirm-delete-btn'].addEventListener('click', handleConfirmDelete);
        }
        const cancelDeleteBtn = DOM['secret-delete-modal']?.querySelector('[data-cancel]');
        if (cancelDeleteBtn) {
            cancelDeleteBtn.addEventListener('click', () => closeModal('secret-delete-modal'));
        }
        if (DOM['secret-list']) {
            DOM['secret-list'].addEventListener('click', handleSecretListClick);
            DOM['secret-list'].addEventListener('keydown', handleSecretListKeydown);
        }
        if (DOM['variable-create-inline']) {
            DOM['variable-create-inline'].addEventListener('click', event => {
                event.preventDefault();
                const scopeKey = state.selectedScopeKey || DEFAULT_SCOPE_KEY;
                openVariableEditModal('create', { scopeKey });
            });
        }
        if (DOM['variable-edit-form']) {
            DOM['variable-edit-form'].addEventListener('submit', handleSubmitVariable);
        }
        const cancelVariableEditBtn = DOM['variable-edit-modal']?.querySelector('[data-cancel]');
        if (cancelVariableEditBtn) {
            cancelVariableEditBtn.addEventListener('click', () => closeModal('variable-edit-modal'));
        }
        if (DOM['variable-confirm-delete-btn']) {
            DOM['variable-confirm-delete-btn'].addEventListener('click', handleConfirmVariableDelete);
        }
        const cancelVariableDeleteBtn = DOM['variable-delete-modal']?.querySelector('[data-cancel]');
        if (cancelVariableDeleteBtn) {
            cancelVariableDeleteBtn.addEventListener('click', () => closeModal('variable-delete-modal'));
        }
        if (DOM['scope-new-form']) {
            DOM['scope-new-form'].addEventListener('submit', handleCreateScope);
        }
        if (DOM['scope-new-cancel']) {
            DOM['scope-new-cancel'].addEventListener('click', hideNewScopeModal);
        }
        if (DOM['scope-new-close']) {
            DOM['scope-new-close'].addEventListener('click', hideNewScopeModal);
        }
        if (DOM['variable-suggestion-panel']) {
            DOM['variable-suggestion-panel'].addEventListener('click', handleVariableSuggestionClick);
        }
        document.addEventListener('keydown', event => {
            if (event.key === 'Escape') {
                closeModal('secret-edit-modal');
                closeModal('secret-delete-modal');
                closeModal('variable-edit-modal');
                closeModal('variable-delete-modal');
                hideNewScopeModal();
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
            
            const scopeLoadPromises = [];
            if (state.scopeMap instanceof Map) {
                state.scopeMap.forEach(scope => {
                    scopeLoadPromises.push(ensureScopeSecretsLoaded(scope, force));
                    scopeLoadPromises.push(ensureScopeVariablesLoaded(scope, force));
                });
            }
            await Promise.all(scopeLoadPromises);

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
        const secretScopes = await fetchSecretScopes();
        const variableScopes = await fetchEnvironmentScopeLabels();

        const scopeMeta = new Map();
        const ensureMeta = (label) => {
            const normalized = normalizeScopeLabel(label || '');
            if (!scopeMeta.has(normalized)) {
                scopeMeta.set(normalized, { secretCountHint: 0, hasSecrets: false, hasVariables: false });
            }
            return scopeMeta.get(normalized);
        };

        secretScopes.forEach(entry => {
            const envLabel = normalizeScopeLabel(entry?.scope ?? '');
            const meta = ensureMeta(envLabel);
            if (typeof entry?.secret_count === 'number') {
                meta.secretCountHint = entry.secret_count;
            }
            meta.hasSecrets = true;
        });

        variableScopes.forEach(label => {
            const meta = ensureMeta(label);
            meta.hasVariables = true;
        });

        if (!scopeMeta.has('')) {
            scopeMeta.set('', { secretCountHint: 0, hasSecrets: false, hasVariables: false });
        }

        const nextMap = new Map();
        const nextList = [];

        scopeMeta.forEach((meta, envLabel) => {
            const key = buildScopeKey(envLabel);
            const existing = previousMap.get(key);
            const scope = existing || createScopeRecord(envLabel);

            scope.scopeName = envLabel;
            scope.folderPath = buildSecretFolderPath(envLabel);
            scope.label = envLabel ? envLabel.split('/').pop() : 'Default Scope';
            scope.description = envLabel ? `Scope “/${envLabel}”` : 'Fallback scope shared across all pipelines';
            scope.triggers = [];
            scope.triggerCount = 0;
            scope.pipelineSet = new Set();
            scope.pipelines = [];
            scope.fetchPromise = null;
            scope.secrets = Array.isArray(scope.secrets) ? scope.secrets : [];
            scope.fetched = !!scope.fetched && scope.secrets.length > 0;
            scope.secretMeta = scope.secretMeta instanceof Map ? scope.secretMeta : new Map();
            scope.secretCountHint = typeof meta.secretCountHint === 'number' ? meta.secretCountHint : scope.secretCountHint || 0;
            scope.hasSecretSources = meta.hasSecrets || scope.hasSecretSources || scope.secretCountHint > 0;
            scope.hasVariableSources = meta.hasVariables || scope.hasVariableSources || (scope.variableCountHint > 0);
            scope.isVirtual = false;

            nextMap.set(key, scope);
            nextList.push(scope);
        });

        nextList.sort((a, b) => {
            const folderCompare = (a.folderPath || '').localeCompare(b.folderPath || '', undefined, { sensitivity: 'base' });
            if (folderCompare !== 0) return folderCompare;
            return (a.label || '').localeCompare(b.label || '', undefined, { sensitivity: 'base' });
        });

        state.scopeMap = nextMap;
        state.scopes = nextList;
        state.filteredScopes = nextList.slice();
        state.scopeTree = buildSecretTree(nextList);
        state.sidebarExpanded = previousExpanded.size ? previousExpanded : new Set(['']);
        state.sidebarExpanded.add('');
        state.triggersByScope = new Map();
        state.pipelinesByScope = new Map();

        if (!state.scopeMap.has(DEFAULT_SCOPE_KEY)) {
            const defaultScope = createScopeRecord('');
            defaultScope.isVirtual = true;
            state.scopeMap.set(DEFAULT_SCOPE_KEY, defaultScope);
        }

        ensureActiveFolderKey();
    }

    async function fetchSecretScopes() {
        if (!context || typeof context.fetchData !== 'function') return [];
        try {
            const response = await context.fetchData('/v1/secrets/scopes');
            if (Array.isArray(response)) {
                return response;
            }
        } catch (error) {
            console.error('Failed to retrieve secret scopes list:', error);
        }
        return [];
    }

    async function fetchEnvironmentScopeLabels() {
        if (!context || typeof context.fetchData !== 'function') return [];
        try {
            const response = await context.fetchData('/v1/variables/scopes');
            if (Array.isArray(response)) {
                return response
                    .map(entry => normalizeEnvironmentScopeEntry(entry))
                    .filter(label => label !== null);
            }
        } catch (error) {
            console.error('Failed to retrieve scope list:', error);
        }
        return [];
    }

    function normalizeEnvironmentScopeEntry(entry) {
        if (entry == null) {
            return '';
        }
        if (typeof entry === 'string') {
            return normalizeScopeLabel(entry);
        }
        if (typeof entry === 'object') {
            const value = entry.scope ?? entry.env ?? entry.name ?? entry.value ?? '';
            return normalizeScopeLabel(value);
        }
        return '';
    }

    function buildSecretTree(scopes) {
        const root = { key: '', label: '', children: new Map(), scopes: [] };
        if (!Array.isArray(scopes) || !scopes.length) {
            return root;
        }

        scopes.forEach(scope => {
            const folderPath = normalizeScopeLabel(scope?.folderPath || '');
            if (!folderPath) {
                root.scopes.push(scope);
                return;
            }
            const parts = folderPath.split('/').filter(Boolean);
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

    function syncScopeVisibility(scope) {
        if (!scope) {
            return;
        }
        const hasSecrets = (Array.isArray(scope.secrets) && scope.secrets.length > 0)
            || (typeof scope.secretCountHint === 'number' && scope.secretCountHint > 0)
            || scope.hasSecretSources;
        const hasVariables = (Array.isArray(scope.variables) && scope.variables.length > 0)
            || (typeof scope.variableCountHint === 'number' && scope.variableCountHint > 0)
            || scope.hasVariableSources;
        const shouldKeep = hasSecrets || hasVariables || scope.scopeName === '';
        const index = Array.isArray(state.scopes)
            ? state.scopes.findIndex(entry => entry.key === scope.key)
            : -1;

        if (shouldKeep) {
            scope.isVirtual = false;
            if (index === -1) {
                state.scopes.push(scope);
            }
        } else if (index !== -1) {
            state.scopes.splice(index, 1);
        }

        state.scopes.sort((a, b) => {
            const folderCompare = (a.folderPath || '').localeCompare(b.folderPath || '', undefined, { sensitivity: 'base' });
            if (folderCompare !== 0) return folderCompare;
            return (a.label || '').localeCompare(b.label || '', undefined, { sensitivity: 'base' });
        });

        state.filteredScopes = state.scopes.slice();
        state.scopeTree = buildSecretTree(state.scopes);
    }

    function createScopeRecord(scopeLabel) {
        const normalizedEnv = normalizeScopeLabel(scopeLabel || '');
        const folderPath = buildSecretFolderPath(normalizedEnv);
        const label = normalizedEnv ? normalizedEnv.split('/').pop() : 'Default Scope';
        const description = normalizedEnv ? `Scope “/${normalizedEnv}”` : 'Fallback scope shared across all pipelines';
        return {
            key: buildScopeKey(normalizedEnv),
            scopeName: normalizedEnv,
            label,
            folderPath,
            description,
            variables: [],
            variableMeta: new Map(),
            variablesFetched: false,
            variableFetchPromise: null,
            variableCountHint: 0,
            hasVariableSources: false,
            secrets: [],
            secretMeta: new Map(),
            fetched: false,
            fetching: false,
            fetchPromise: null,
            hasSecretSources: false,
            triggers: [],
            triggerCount: 0,
            pipelineSet: new Set(),
            pipelines: [],
            secretCountHint: 0,
            isVirtual: false,
        };
    }

    function buildScopeKey(scopeLabel) {
        const envPart = normalizeScopeLabel(scopeLabel || '');
        return `env:${envPart}`;
    }

    function buildSecretFolderPath(scopeLabel) {
        const envSegments = normalizeScopeLabel(scopeLabel || '')
            .split('/')
            .filter(Boolean);
        return envSegments.join('/');
    }

    function encodeScopeSegment(scopeLabel) {
        const normalized = normalizeScopeLabel(scopeLabel || '');
        if (!normalized) {
            return 'default';
        }
        return normalized
            .split('/')
            .filter(Boolean)
            .map(encodeURIComponent)
            .join('/');
    }

    function decodeSecretSegment(segment, index, folderMode) {
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

        state.triggersByScope = new Map();
        state.pipelinesByScope = new Map();

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
                state.scopeTree = buildSecretTree(state.scopes);
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
                    registerTriggerForSecretScope({ slug, trigger, owner, name });
                });
            }

            state.scopeMap.forEach(scope => {
                scope.pipelines = Array.from(scope.pipelineSet).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
                scope.triggerCount = scope.triggers.length;
            });
            state.scopeTree = buildSecretTree(state.scopes);
            ensureActiveFolderKey();
        } catch (error) {
            console.error('Failed to build trigger summaries for secrets:', error);
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
            console.error('Failed to parse YAML for secret trigger summary:', error);
            return null;
        }
    }

    function normalizeSecretSourceKey(value) {
        if (value == null) return 'database';
        const key = String(value).trim().toLowerCase();
        if (!key) return 'database';
        if (key.includes('git')) return 'git';
        if (key.includes('draft')) return 'draft';
        if (key.includes('local')) return 'local';
        return 'database';
    }

    function formatSecretSourceLabel(key) {
        switch (normalizeSecretSourceKey(key)) {
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

    function getSecretMetadata(name, scopeRef) {
        if (!name) return null;
        let scope = scopeRef;
        if (typeof scope === 'string') {
            scope = state.scopeMap?.get(scope);
        }
        if (scope && scope.secretMeta instanceof Map && scope.secretMeta.has(name)) {
            return scope.secretMeta.get(name);
        }
        const envKey = makeSecretValueCacheKey(name, scope?.scopeName || '');
        if (state.secretSources instanceof Map && state.secretSources.has(envKey)) {
            return state.secretSources.get(envKey);
        }
        return null;
    }

    function getSecretSourceForSecret(name, scopeRef) {
        const meta = getSecretMetadata(name, scopeRef);
        return meta?.source || 'database';
    }

    function isSecretSourceEditable(key) {
        return normalizeSecretSourceKey(key) !== 'git';
    }

    function scopeHasGitManagedSecrets(scope) {
        if (!scope) return false;
        const secrets = Array.isArray(scope.secrets) ? scope.secrets : [];
        return secrets.some(name => normalizeSecretSourceKey(getSecretSourceForSecret(name, scope)) === 'git');
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

    function registerTriggerForSecretScope({ slug, trigger, owner, name }) {
        const scopeValue = trigger?.scope;
        const scopeLabel = normalizeScopeLabel(scopeValue);
        const scopeKey = buildScopeKey(scopeLabel);
        const scope = state.scopeMap.get(scopeKey);
        if (!scope) {
            return;
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
            scope: scopeLabel,
            pipelines,
            event: canonicalizeEvent(trigger?.on),
            branches: Array.isArray(trigger?.branches) ? trigger.branches : [],
            tags: Array.isArray(trigger?.tags) ? trigger.tags : [],
            source: (trigger?.source || '').toString(),
        };

        scope.triggers.push(triggerDescriptor);
        scope.triggerCount = scope.triggers.length;
        scope.pipelines = Array.from(scope.pipelineSet).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));

        const scopeTriggers = state.triggersByScope.get(scopeLabel) || [];
        scopeTriggers.push(triggerDescriptor);
        state.triggersByScope.set(scopeLabel, scopeTriggers);

        const scopePipelines = state.pipelinesByScope.get(scopeLabel) || new Set();
        pipelines.forEach(identifier => scopePipelines.add(identifier));
        state.pipelinesByScope.set(scopeLabel, scopePipelines);
    }

    function normalizeScopeLabel(value) {
        if (value == null) return '';
        return String(value)
            .trim()
            .replace(/^\/+|\/+$/g, '');
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
        if (DOM['secret-search']) {
            DOM['secret-search'].value = '';
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
            if (scope.scopeName && scope.scopeName.toLowerCase().includes(term)) return true;
            if (scope.folderPath && scope.folderPath.toLowerCase().includes(term)) return true;
            return false;
        });
        toggleClearButton(true);
    }

    function toggleClearButton(show) {
        if (!DOM['secret-clear-search']) return;
        DOM['secret-clear-search'].classList.toggle('hidden', !show);
    }

    function renderScopeCollection() {
        const container = DOM['secret-list'];
        if (!container) return;

        const searchTerm = (state.searchTerm || '').trim();
        const isSearching = state.isSearching && !!searchTerm;
        const hasSelection = !!state.selectedScopeKey;

        if (isSearching) {
            showSecretListView();
        } else if (hasSelection) {
            showSecretDetailView();
        } else {
            showSecretListView();
        }

        let html = '';
        let showEmpty = false;

        if (isSearching) {
            const results = state.filteredScopes || [];
            if (results.length) {
                html += renderSecretSearchSummary(results.length, searchTerm);
                const cards = results
                    .slice()
                    .sort((a, b) => (a.scopeName || '').localeCompare(b.scopeName || '', undefined, { sensitivity: 'base' }))
                    .reduce((acc, scope) => acc.concat(renderScopeCategoryCards(scope)), []);
                if (cards.length) {
                    html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${cards.join('')}</div>`;
                }
            } else {
                showEmpty = true;
            }
        } else {
            ensureActiveFolderKey();
            const activeFolder = normalizeScopeLabel(state.activeFolderKey || '');
            const tree = state.scopeTree || buildSecretTree(state.scopes);
            const node = getSecretTreeNode(activeFolder) || tree;

            const childNodes = node?.children instanceof Map ? Array.from(node.children.values()) : [];
            childNodes.sort((a, b) => (a.key || '').localeCompare(b.key || '', undefined, { sensitivity: 'base' }));
            const scopes = Array.isArray(node?.scopes) ? node.scopes.slice() : [];
            scopes.sort((a, b) => (a.scopeName || '').localeCompare(b.scopeName || '', undefined, { sensitivity: 'base' }));
            const folderCards = childNodes.map(renderSecretFolderCard).filter(Boolean);
            if (folderCards.length) {
                html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${folderCards.join('')}</div>`;
            }
            const scopeCards = scopes.reduce((acc, scopeEntry) => acc.concat(renderScopeCategoryCards(scopeEntry)), []);
            if (scopeCards.length) {
                html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${scopeCards.join('')}</div>`;
            }

            if (!folderCards.length && !scopeCards.length) {
                showEmpty = true;
            }
        }

        container.innerHTML = html;
        updateCreateScopeButtonVisibility({ isSearching });
        if (DOM['secret-list-empty']) {
            if (showEmpty) {
                if (isSearching && searchTerm) {
                    DOM['secret-list-empty'].innerHTML = `<p class="text-sm">No scope folders matched "${escapeHtml(searchTerm)}".</p>`;
                } else if (state.activeFolderKey) {
                    DOM['secret-list-empty'].innerHTML = '<p class="text-sm">No scope folders or scopes inside this path.</p>';
                } else {
                    DOM['secret-list-empty'].innerHTML = '<p class="text-sm">No scope folders yet. Create a scope to add one.</p>';
                }
            }
            DOM['secret-list-empty'].classList.toggle('hidden', !showEmpty);
        }

        highlightActiveSecretCard();
    }

    function updateCreateScopeButtonVisibility(options = {}) {
        const button = DOM['scope-create-btn'];
        if (!button) return;
        const isSearching = Object.prototype.hasOwnProperty.call(options, 'isSearching')
            ? !!options.isSearching
            : !!(state.searchTerm || '').trim();
        const showDetail = Object.prototype.hasOwnProperty.call(options, 'showDetail')
            ? !!options.showDetail
            : isSecretDetailVisible();
        button.classList.toggle('hidden', isSearching || showDetail);
    }

    function isSecretDetailVisible() {
        if (DOM['secret-detail-view']) {
            return !DOM['secret-detail-view'].classList.contains('hidden');
        }
        return !!state.selectedScopeKey;
    }

    function ensureActiveFolderKey() {
        const current = normalizeScopeLabel(state.activeFolderKey || '');
        if (!getSecretTreeNode(current)) {
            state.activeFolderKey = '';
        }
    }

    function getSecretTreeNode(path) {
        const normalized = normalizeScopeLabel(path || '');
        const tree = state.scopeTree || buildSecretTree(state.scopes);
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

    function countSecretScopesRecursive(node) {
        if (!node) return 0;
        let total = Array.isArray(node.scopes) ? node.scopes.length : 0;
        if (node.children instanceof Map) {
            node.children.forEach(child => {
                total += countSecretScopesRecursive(child);
            });
        }
        return total;
    }

    function renderSecretFolderCard(node) {
        if (!node) return '';
        const totalScopes = countSecretScopesRecursive(node);
        const childCount = node.children instanceof Map ? node.children.size : 0;
        const displayPath = node.key ? `/${node.key}` : '/';
        const label = formatSecretFolderLabel(node.label || node.key || 'Folder');
        const keyAttr = escapeAttribute(node.key || '');
        const ariaLabel = escapeAttribute(displayPath);

        return `
            <article class="pipeline-folder-card border border-[var(--border-primary)]" data-secret-folder="${keyAttr}" tabindex="0" role="button" aria-label="Open folder ${ariaLabel}">
                <div class="pipeline-folder-card-header">
                    <span class="pipeline-folder-icon">
                        <svg class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7h5l2 2h9a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V9a2 2 0 012-2z"/></svg>
                    </span>
                    <div class="pipeline-folder-text min-w-0">
                        <h3 class="pipeline-folder-title">${escapeHtml(label)}</h3>
                        <p class="pipeline-folder-path text-xs text-[var(--text-secondary)] truncate">${escapeHtml(displayPath)}</p>
                    </div>
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

    function describeSecretScope(scope) {
        const scopeLabel = normalizeScopeLabel(scope?.scopeName || '');
        const envSegments = scopeLabel.split('/').filter(Boolean);
        const title = envSegments.length ? envSegments[envSegments.length - 1] : 'Default';
        const parentPath = envSegments.length > 1 ? `/${envSegments.slice(0, -1).join('/')}` : '/';
        const fullPath = scopeLabel ? `/${scopeLabel}` : '/';
        return { title, parentPath, fullPath };
    }

function renderScopeCategoryCards(scope) {
    if (!scope) return [];
    return [
        renderScopeCategoryCard(scope, 'variables'),
        renderScopeCategoryCard(scope, 'secrets')
    ].filter(Boolean);
}

function buildScopeCategoryIcon(category, className = '') {
    const classes = ['scope-category-icon', className].filter(Boolean).join(' ').trim();
    if (category === 'variables') {
        return `<svg class="${classes}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 7h16"/><circle cx="9" cy="7" r="2" fill="currentColor" stroke="none"/><path d="M4 12h16"/><circle cx="15" cy="12" r="2" fill="currentColor" stroke="none"/><path d="M4 17h16"/><circle cx="7" cy="17" r="2" fill="currentColor" stroke="none"/></svg>`;
    }
    return `<svg class="${classes}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="5" y="11" width="14" height="10" rx="2"></rect><path d="M17 11V8a5 5 0 00-10 0v3"></path><path d="M12 16v2"></path></svg>`;
}

function renderScopeCategoryCard(scope, category) {
    if (!scope) return '';
    const isVariables = category === 'variables';
    const activeCategory = getSelectedScopeCategory();
    const isActive = scope.key === state.selectedScopeKey && activeCategory === category;
    const { title, fullPath } = describeSecretScope(scope);
    const variableCount = typeof scope.variableCountHint === 'number'
        ? scope.variableCountHint
        : (Array.isArray(scope.variables) ? scope.variables.length : 0);
    const secretCount = typeof scope.secretCountHint === 'number'
        ? scope.secretCountHint
        : (Array.isArray(scope.secrets) ? scope.secrets.length : 0);
    const triggerCount = scope.triggerCount || 0;
    const typeLabel = isVariables ? 'Variables' : 'Secrets';
    const typeIcon = buildScopeCategoryIcon(category, 'h-6 w-6');
    const typeCount = isVariables ? variableCount : secretCount;
    const ariaLabel = `Open ${typeLabel.toLowerCase()} for scope ${fullPath || '/'}`;

    const metaRows = [
        { label: typeLabel, value: typeCount },
        { label: 'Triggers', value: triggerCount }
    ];

    return `
        <article class="pipeline-card triggers-card scope-card env-scope-card bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded-lg shadow-sm transition-all duration-200 p-3 flex flex-col${isActive ? ' triggers-card--active env-scope-card--active' : ''} scope-card--${category}" data-secret-scope="${escapeAttribute(scope.key)}" data-secret-scope-type="${category}" tabindex="0" role="button" aria-label="${escapeAttribute(ariaLabel)}">
            <div class="pipeline-card-header flex items-start justify-between gap-3">
                <div class="pipeline-card-info">
                    <span class="triggers-card-icon" aria-hidden="true">${typeIcon}</span>
                    <div class="pipeline-card-text min-w-0">
                        <h3 class="pipeline-card-title">${escapeHtml(title)}</h3>
                    </div>
                </div>
                <span class="scope-type-badge scope-type-badge--${category}">${typeLabel}</span>
            </div>
            <div class="pipeline-card-meta">
                ${metaRows.map(row => `
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">${escapeHtml(row.label)}</span>
                        <span class="pipeline-card-meta-value">${escapeHtml(String(row.value))}</span>
                    </div>
                `).join('')}
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

    function sanitizeSecretSegments(raw) {
        if (!raw) return [];
        return String(raw)
            .split('/')
            .map(part => part.trim().replace(/[^A-Za-z0-9_.-]+/g, '-').replace(/^-+|-+$/g, ''))
            .filter(Boolean);
    }

    function renderSecretSearchSummary(count, term) {
        const safeTerm = escapeHtml(term);
        return `<div class="triggers-search-summary">Showing ${count} scope${count === 1 ? '' : 's'} for "${safeTerm}"</div>`;
    }

    function highlightActiveSecretCard() {
        const list = DOM['secret-list'];
        if (!list) return;
        const isListVisible = !DOM['secret-list-view'] || !DOM['secret-list-view'].classList.contains('hidden');
        const activeCategory = getSelectedScopeCategory();
        list.querySelectorAll('[data-secret-scope]').forEach(card => {
            const key = card.getAttribute('data-secret-scope');
            const cardCategory = card.getAttribute('data-secret-scope-type') || 'secrets';
            const shouldHighlight = isListVisible && key === state.selectedScopeKey && cardCategory === activeCategory;
            card.classList.toggle('env-scope-card--active', shouldHighlight);
        });
    }

    function buildSecretFolderHash(folderKey) {
        const normalized = normalizeScopeLabel(folderKey || '');
        if (!normalized) {
            return '#/scopes';
        }
        const segments = normalized.split('/').filter(Boolean).map(encodeURIComponent);
        const path = segments.join('/');
        return path ? `#/scopes/${path}` : '#/scopes';
    }

    function navigateToFolder(folderKey) {
        const hash = buildSecretFolderHash(folderKey);
        if (window.location.hash !== hash) {
            window.location.hash = hash;
        } else {
            handleRoute(hash);
        }
    }

    function resetSecretSelection(options = {}) {
        if (state.selectedScopeKey) {
            state.selectedScopeKey = null;
        }
        state.selectedSecret = null;
        state.selectedVariable = null;
        state.expandedVariable = null;
        setSelectedScopeCategory('secrets');
        clearSecretDetail();
        clearVariableDetail();
        if (options.showList !== false) {
            showSecretListView();
        }
        highlightActiveSecretCard();
        updateSecretItemStates();
        updateVariableItemStates();
    }

    function handleSecretListClick(event) {
        const deleteButton = event.target.closest('[data-secret-delete]');
        if (deleteButton) {
            event.preventDefault();
            const scopeKey = deleteButton.getAttribute('data-secret-delete') || '';
            openDeleteSecretScopeModal(scopeKey);
            event.stopPropagation();
            return;
        }

        const folderCard = event.target.closest('[data-secret-folder]');
        if (folderCard) {
            event.preventDefault();
            const key = folderCard.getAttribute('data-secret-folder') || '';
            resetSecretSelection({ showList: true });
            navigateToFolder(key);
            event.stopPropagation();
            return;
        }

        const scopeCard = event.target.closest('[data-secret-scope]');
        if (scopeCard) {
            event.preventDefault();
            const key = scopeCard.getAttribute('data-secret-scope');
            const category = scopeCard.getAttribute('data-secret-scope-type') || 'secrets';
            if (key) {
                setSelectedScopeCategory(category);
                Promise.resolve().then(() => selectScope(key, { silent: true, skipHash: true, category })).catch(err => console.error('Failed to open scope', err));
                navigateToScope(key, null, null, { category });
            }
            event.stopPropagation();
            return;
        }
    }

    function handleSecretListKeydown(event) {
        if (event.defaultPrevented) return;
        if (event.key !== 'Enter' && event.key !== ' ' && event.key !== 'Spacebar') return;

        const deleteButton = event.target.closest('[data-secret-delete]');
        if (deleteButton && deleteButton === document.activeElement) {
            event.preventDefault();
            const scopeKey = deleteButton.getAttribute('data-secret-delete') || '';
            openDeleteSecretScopeModal(scopeKey);
            event.stopPropagation();
            return;
        }

        const folderCard = event.target.closest('[data-secret-folder]');
        if (folderCard && folderCard === document.activeElement) {
            event.preventDefault();
            const key = folderCard.getAttribute('data-secret-folder') || '';
            resetSecretSelection({ showList: true });
            navigateToFolder(key);
            focusFirstSecretCard();
            event.stopPropagation();
            return;
        }

        const scopeCard = event.target.closest('[data-secret-scope]');
        if (scopeCard && scopeCard === document.activeElement) {
            event.preventDefault();
            const key = scopeCard.getAttribute('data-secret-scope');
            const category = scopeCard.getAttribute('data-secret-scope-type') || 'secrets';
            if (key) {
                setSelectedScopeCategory(category);
                Promise.resolve().then(() => selectScope(key, { silent: true, skipHash: true, category })).catch(err => console.error('Failed to open scope', err));
                navigateToScope(key, null, null, { category });
            }
            event.stopPropagation();
        }
    }

    function focusFirstSecretCard() {
        const list = DOM['secret-list'];
        if (!list) return;
        const first = list.querySelector('[data-secret-folder], [data-secret-scope]');
        if (first && typeof first.focus === 'function') {
            first.focus();
        }
    }

    function getSidebarContainer() {
        return document.getElementById('scopes-sidebar-tree')
            || document.getElementById('secrets-sidebar-tree');
    }

    function renderSidebarTree(targetContainer) {
        const container = targetContainer || getSidebarContainer();
        if (!container) return;
        renderSidebarTreeNodes(container);
    }

    function renderSidebarTreeNodes(container) {
        if (!container) return;

        const tree = state.scopeTree || buildSecretTree(state.scopes);
        const rootScopes = Array.isArray(tree?.scopes) ? tree.scopes.slice() : [];
        const childNodes = tree?.children instanceof Map ? Array.from(tree.children.values()) : [];

        rootScopes.sort((a, b) => (a.scopeName || '').localeCompare(b.scopeName || '', undefined, { sensitivity: 'base' }));
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

        container.querySelectorAll('[data-secret-toggle-folder]').forEach(button => {
            button.addEventListener('click', event => {
                event.stopPropagation();
                const path = button.getAttribute('data-secret-toggle-folder');
                toggleSidebarFolder(path);
            });
        });

        container.querySelectorAll('[data-secret-open-folder]').forEach(button => {
            button.addEventListener('click', event => {
                event.preventDefault();
                const path = button.getAttribute('data-secret-open-folder') || '';
                resetSecretSelection({ showList: true });
                navigateToFolder(path);
                event.stopPropagation();
            });
        });

        container.querySelectorAll('[data-secret-sidebar-scope]').forEach(button => {
            button.addEventListener('click', event => {
                event.preventDefault();
                const key = button.getAttribute('data-secret-sidebar-scope');
                const category = button.getAttribute('data-secret-sidebar-category') || 'secrets';
                if (key) {
                    setSelectedScopeCategory(category);
                    Promise.resolve().then(() => selectScope(key, { silent: true, skipHash: true, category })).catch(err => console.error('Failed to open scope', err));
                    navigateToScope(key, null, null, { category });
                }
                event.stopPropagation();
            });
        });

        renderSecretSuggestions();
    }


    function renderSidebarFolderNode(node, level) {
        if (!node) return '';
        const folderPath = node.key || '';
        const folderLabel = formatSecretFolderLabel(node.label);
        const isExpanded = shouldExpandFolder(folderPath);
        if (isExpanded) {
            ensureSidebarExpansionForPath(folderPath);
        }
        const isActiveFolder = folderPath && state.activeFolderKey === folderPath;

        const scopes = Array.isArray(node.scopes) ? node.scopes.slice() : [];
        scopes.sort((a, b) => (a.scopeName || '').localeCompare(b.scopeName || '', undefined, { sensitivity: 'base' }));

        const children = node.children instanceof Map ? Array.from(node.children.values()) : [];
        children.sort((a, b) => (a.key || '').localeCompare(b.key || '', undefined, { sensitivity: 'base' }));

        let innerHtml = '';
        if (children.length || scopes.length) {
            innerHtml += `<div class="scope-sidebar-children ${isExpanded ? '' : 'hidden'}" data-secret-folder-children="${escapeAttribute(folderPath)}">`;
            innerHtml += renderSidebarTreeNodesList(node, level + 1);
            innerHtml += '</div>';
        }

        return `
            <li data-secret-folder-node="${escapeAttribute(folderPath)}">
                <div class="flex items-center justify-between p-1 text-[var(--text-primary)] rounded-md pipeline-sidebar-folder-row ${isActiveFolder ? 'bg-[var(--bg-tertiary)]' : ''} hover:bg-[var(--bg-tertiary)]">
                    <div class="flex items-center flex-grow min-w-0">
                        <button type="button" class="sidebar-toggle-btn flex items-center justify-center h-5 w-5 rounded mr-1 text-[var(--text-secondary)] hover:bg-[var(--bg-hover)]" data-secret-toggle-folder="${escapeAttribute(folderPath)}" aria-expanded="${isExpanded ? 'true' : 'false'}" aria-label="${escapeAttribute((isExpanded ? 'Collapse' : 'Expand') + ' ' + folderLabel)}">
                            <svg class="h-4 w-4 chevron ${isExpanded ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>
                        </button>
                        <button type="button" class="pipeline-sidebar-folder flex items-center gap-2 flex-grow text-left min-w-0 p-1 rounded hover:bg-[var(--bg-hover)]" data-secret-open-folder="${escapeAttribute(folderPath)}">
                            <svg class="h-4 w-4 text-[var(--text-secondary)] flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>
                            <span class="truncate">${escapeHtml(folderLabel)}</span>
                        </button>
                    </div>
                </div>
                ${innerHtml}
            </li>
        `;
    }

    function renderSidebarTreeNodesList(node, level) {
        const childEntries = node.children instanceof Map ? Array.from(node.children.values()) : [];
        const scopeEntries = Array.isArray(node.scopes) ? node.scopes.slice() : [];

        if (!childEntries.length && !scopeEntries.length) {
            return '';
        }

        childEntries.sort((a, b) => (a.key || '').localeCompare(b.key || '', undefined, { sensitivity: 'base' }));
        scopeEntries.sort((a, b) => (a.scopeName || '').localeCompare(b.scopeName || '', undefined, { sensitivity: 'base' }));

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

    function renderSecretSuggestions(activeScopeKey = state.selectedScopeKey || DEFAULT_SCOPE_KEY) {
        const panel = DOM['secret-suggestion-panel'];
        const list = DOM['secret-suggestion-list'];
        const emptyState = DOM['secret-suggestion-empty'];
        if (!panel || !list || !emptyState) {
            return;
        }

        const suggestions = [];
        if (state.scopeMap instanceof Map) {
            state.scopeMap.forEach(scope => {
                if (scope?.isVirtual) return;
                const secrets = Array.isArray(scope.secrets) ? scope.secrets.filter(Boolean) : [];
                if (!secrets.length) return;
                const scopeLabel = scope.scopeName ? `/${scope.scopeName}` : '/ (default)';
                suggestions.push({
                    key: scope.key,
                    label: scopeLabel,
                    count: secrets.length,
                    preview: secrets.slice(0, 5),
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
                const identity = parseSecretIdentity(name);
                const repoSlug = identity.repoOwner && identity.repoName ? `${identity.repoOwner}/${identity.repoName}` : '';
                const displayBase = identity.name || name;
                const displayName = repoSlug ? `${repoSlug}/${displayBase}` : displayBase;
                return `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-secret-suggestion-value="${valueAttr}" data-secret-suggestion-scope="${scopeAttr}">${escapeHtml(displayName)}</button>`;
            });
            const remaining = entry.count - entry.preview.length;
            if (remaining > 0) {
                pills.push(`<span class="env-suggestion-pill env-suggestion-pill--more">+${remaining} more</span>`);
            }
            const countLabel = `${entry.count} ${entry.count === 1 ? 'secret' : 'secrets'}`;
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

    function handleSecretSuggestionClick(event) {
        const button = event.target.closest('[data-secret-suggestion-value]');
        if (!button) return;
        if (!DOM['secret-edit-modal'] || DOM['secret-edit-modal'].classList.contains('hidden')) {
            return;
        }
        const secretValue = button.getAttribute('data-secret-suggestion-value') || '';
        if (!secretValue) return;
        event.preventDefault();
        const identity = parseSecretIdentity(secretValue);
        const nameInput = DOM['secret-edit-name'];
        if (nameInput && !nameInput.readOnly) {
            nameInput.value = identity.name || secretValue;
            try {
                const caret = nameInput.value.length;
                nameInput.setSelectionRange(caret, caret);
            } catch {}
        }
        const repoInput = DOM['secret-edit-repo'];
        if (repoInput && !repoInput.disabled) {
            const repoSlug = identity.repoOwner && identity.repoName ? `${identity.repoOwner}/${identity.repoName}` : '';
            repoInput.value = repoSlug;
        }
        nameInput?.focus();
    }

    function renderSidebarScopeEntry(scope) {
        if (!scope) return '';
        const categories = ['variables', 'secrets'];
        const { title, fullPath } = describeSecretScope(scope);
        const displayLabel = scope.label || title || 'Default';
        const displayPath = fullPath || '/';
        const activeCategory = getSelectedScopeCategory();

        return categories.map(category => {
            const isActive = scope.key === state.selectedScopeKey && activeCategory === category;
            const icon = buildScopeCategoryIcon(category, 'h-4 w-4 flex-shrink-0');
            const badge = category === 'variables' ? 'Vars' : 'Secrets';
            const ariaLabel = `Open ${category} for ${displayPath}`;
            return `<li>
                <button type="button" class="w-full flex items-center justify-between gap-2 text-left px-3 py-1.5 rounded-md text-sm transition ${isActive ? 'bg-[var(--bg-hover)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'}" data-secret-sidebar-scope="${escapeAttribute(scope.key)}" data-secret-sidebar-category="${category}" aria-label="${escapeAttribute(ariaLabel)}">
                    <span class="flex items-center gap-2 min-w-0">
                        ${icon}
                        <span class="truncate">${escapeHtml(displayLabel)}</span>
                    </span>
                    <span class="text-xs uppercase tracking-wide text-[var(--text-secondary)]">${badge}</span>
                </button>
            </li>`;
        }).join('');
    }

    function formatSecretFolderLabel(label) {
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

    function getSelectedScopeFolderPath() {
        if (!state.selectedScopeKey) return '';
        const scope = state.scopeMap instanceof Map ? state.scopeMap.get(state.selectedScopeKey) : null;
        return normalizeScopeLabel(scope?.folderPath || '');
    }

    function getScopeParentFolderPath(scope) {
        const folderPath = normalizeScopeLabel(scope?.folderPath || '');
        if (!folderPath) return '';
        const segments = folderPath.split('/').filter(Boolean);
        if (!segments.length) return '';
        segments.pop();
        return segments.join('/');
    }

    function getSelectedScopeCategory() {
        return state.selectedScopeCategory === 'variables' ? 'variables' : 'secrets';
    }

    function setSelectedScopeCategory(category) {
        const normalized = category === 'variables' ? 'variables' : 'secrets';
        state.selectedScopeCategory = normalized;
        const detail = DOM['secret-detail'];
        if (detail) {
            detail.dataset.scopeCategory = normalized;
        }
    }

    function shouldExpandFolder(path) {
        const normalized = normalizeScopeLabel(path || '');
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
        const selectedFolder = getSelectedScopeFolderPath();
        if (selectedFolder && (selectedFolder === normalized || selectedFolder.startsWith(`${normalized}/`))) {
            return true;
        }
        return false;
    }

    function toggleSidebarFolder(path) {
        const normalized = normalizeScopeLabel(path || '');
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
        const normalized = normalizeScopeLabel(path || '');
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

    function buildScopeHash(scope, { category = null, secretName = null, variableName = null } = {}) {
        if (!scope) return '#/scopes';
        const envSegment = encodeScopeSegment(scope?.scopeName || '');
        let suffix = '';
        if (secretName) {
            suffix = `/secrets/${encodeURIComponent(secretName)}`;
        } else if (variableName) {
            suffix = `/variables/${encodeURIComponent(variableName)}`;
        } else if (category === 'variables') {
            suffix = '/variables';
        } else if (category === 'secrets') {
            suffix = '/secrets';
        }
        return `#/scopes/${envSegment}${suffix}`;
    }

    function navigateToScope(scopeKey, secretName = null, variableName = null, options = {}) {
        const scope = state.scopeMap.get(scopeKey) || state.scopeMap.get(DEFAULT_SCOPE_KEY);
        if (!scope) {
            window.location.hash = '#/scopes';
            return;
        }
        const hash = buildScopeHash(scope, { category: options.category, secretName, variableName });
        window.location.hash = hash;
    }

    async function selectScope(scopeKey, options = {}) {
        if (options.category) {
            setSelectedScopeCategory(options.category);
        } else if (!state.selectedScopeCategory) {
            setSelectedScopeCategory('secrets');
        }
        const scope = state.scopeMap.get(scopeKey);
        if (!scope) {
            state.selectedScopeKey = null;
            state.activeFolderKey = '';
            renderSidebarTree();
            showSecretListView();
            return;
        }

        state.selectedScopeKey = scope.key;
        const folderKey = getScopeParentFolderPath(scope);
        setActiveFolder(folderKey, { ensure: true, refreshList: false });
        showSecretDetailView();
        await Promise.all([
            ensureScopeSecretsLoaded(scope, options.forceReload),
            ensureScopeVariablesLoaded(scope, options.forceReload)
        ]);
        renderScopeDetail(scope);
        highlightActiveSecretCard();

        if (!options.silent && !options.skipHash) {
            navigateToScope(scope.key, null, null, { category: getSelectedScopeCategory() });
        }
    }

    async function ensureScopeSecretsLoaded(scope, force = false) {
        if (scope.fetchPromise) {
            await scope.fetchPromise;
            return scope.secrets;
        }
        if (!force && scope.fetched) {
            return scope.secrets;
        }
        if (!context || typeof context.fetchData !== 'function') return [];

        const baseUrl = scope.scopeName ? `/v1/secrets?env=${encodeURIComponent(scope.scopeName)}` : '/v1/secrets';
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
                            source: normalizeSecretSourceKey(item.source || 'database'),
                            createdAt: item.created_at || item.createdAt || '',
                            updatedAt: item.updated_at || item.updatedAt || '',
                        };
                    }
                    return { name: '', source: 'database', createdAt: '', updatedAt: '' };
                }).filter(entry => entry.name);

                const globalMeta = state.secretSources instanceof Map ? state.secretSources : new Map();
                state.secretSources = globalMeta;
                if (!(scope.secretMeta instanceof Map)) {
                    scope.secretMeta = new Map();
                }
                if (scope.secretMeta instanceof Map) {
                    scope.secretMeta.forEach((_, secretName) => {
                        globalMeta.delete(makeSecretValueCacheKey(secretName, scope.scopeName || ''));
                    });
                    scope.secretMeta.clear();
                }

                const names = normalizedEntries.map(entry => {
                    const meta = {
                        source: entry.source,
                        createdAt: entry.createdAt,
                        updatedAt: entry.updatedAt,
                    };
                    scope.secretMeta.set(entry.name, meta);
                    globalMeta.set(makeSecretValueCacheKey(entry.name, scope.scopeName || ''), meta);
                    return entry.name;
                });
                names.sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
                scope.secrets = names;
                scope.fetched = true;
                scope.secretCountHint = names.length;
                scope.hasSecretSources = scope.hasSecretSources || names.length > 0;
                return names;
            } catch (error) {
                console.error('Failed to load secrets for scope', scope.scopeName, error);
                scope.secrets = [];
                scope.secretCountHint = 0;
                if (!Array.isArray(scope.variables) || scope.variables.length === 0) {
                    scope.hasSecretSources = false;
                }
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

    async function ensureScopeVariablesLoaded(scope, force = false) {
        if (scope.variableFetchPromise) {
            await scope.variableFetchPromise;
            return scope.variables;
        }
        if (!force && scope.variablesFetched) {
            return scope.variables;
        }
        if (!context || typeof context.fetchData !== 'function') return [];

        const baseUrl = scope.scopeName ? `/v1/variables?env=${encodeURIComponent(scope.scopeName)}` : '/v1/variables';
        const url = baseUrl.includes('?') ? `${baseUrl}&include_source=true` : `${baseUrl}?include_source=true`;
        scope.variableFetchPromise = (async () => {
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
                            source: normalizeVariableSourceKey(item.source || 'database'),
                            createdAt: item.created_at || item.createdAt || '',
                            updatedAt: item.updated_at || item.updatedAt || '',
                        };
                    }
                    return { name: '', source: 'database', createdAt: '', updatedAt: '' };
                }).filter(entry => entry.name);

                const globalMeta = state.variableSources instanceof Map ? state.variableSources : new Map();
                state.variableSources = globalMeta;
                if (!(scope.variableMeta instanceof Map)) {
                    scope.variableMeta = new Map();
                }
                if (scope.variableMeta instanceof Map) {
                    scope.variableMeta.clear();
                }

                const names = normalizedEntries.map(entry => {
                    const meta = {
                        source: entry.source,
                        createdAt: entry.createdAt,
                        updatedAt: entry.updatedAt,
                    };
                    scope.variableMeta.set(entry.name, meta);
                    const cacheKey = makeScopeValueCacheKey(entry.name, scope.scopeName || '');
                    globalMeta.set(cacheKey, meta);
                    return entry.name;
                });
                names.sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
                scope.variables = names;
                scope.variablesFetched = true;
                scope.variableCountHint = names.length;
                scope.hasVariableSources = scope.hasVariableSources || names.length > 0;
                return names;
            } catch (error) {
                console.error('Failed to load scope variables for', scope.scopeName, error);
                scope.variables = [];
                scope.variableCountHint = 0;
                if (!Array.isArray(scope.secrets) || scope.secrets.length === 0) {
                    scope.hasVariableSources = false;
                }
                return [];
            } finally {
                scope.variableFetchPromise = null;
            }
        })();

        const names = await scope.variableFetchPromise;
        renderScopeCollection();
        return names;
    }

    function showSecretListView() {
        if (DOM['secret-list-view']) DOM['secret-list-view'].classList.remove('hidden');
        if (DOM['secret-detail-view']) DOM['secret-detail-view'].classList.add('hidden');
        if (DOM['secret-back-btn']) DOM['secret-back-btn'].classList.add('hidden');
        if (DOM['secret-search-container']) DOM['secret-search-container'].classList.remove('hidden');
        if (DOM['scope-create-btn']) DOM['scope-create-btn'].classList.remove('hidden');
        updateCreateScopeButtonVisibility({ isSearching: state.isSearching && !!(state.searchTerm || '').trim() });
    }

    function showSecretDetailView() {
        if (DOM['secret-list-view']) DOM['secret-list-view'].classList.add('hidden');
        if (DOM['secret-detail-view']) DOM['secret-detail-view'].classList.remove('hidden');
        if (DOM['secret-back-btn']) DOM['secret-back-btn'].classList.remove('hidden');
        if (DOM['secret-search-container']) DOM['secret-search-container'].classList.add('hidden');
        if (DOM['scope-create-btn']) DOM['scope-create-btn'].classList.add('hidden');
        updateCreateScopeButtonVisibility({ isSearching: false });
        setSelectedScopeCategory(state.selectedScopeCategory || 'secrets');
    }

    function renderScopeDetail(scope) {
        if (!DOM['secret-detail']) return;
        DOM['secret-detail'].classList.remove('hidden');

        if (DOM['secret-detail-title']) {
            DOM['secret-detail-title'].textContent = scope.scopeName ? `/${scope.scopeName}` : '/';
        }

        renderVariableList(scope);
        if (scope.variables && scope.variables.length) {
            const preferredVariable = scope.variables.includes(state.selectedVariable) ? state.selectedVariable : scope.variables[0];
            selectVariable(preferredVariable, { silent: true, disableCategorySwitch: true });
        } else {
            clearVariableDetail();
        }

        renderSecretList(scope);

        if (scope.secrets.length) {
            const preferredSecret = scope.secrets.includes(state.selectedSecret) ? state.selectedSecret : scope.secrets[0];
            selectSecret(preferredSecret, { silent: true, disableCategorySwitch: true });
        } else {
            clearSecretDetail();
        }
    }

    function renderSecretList(scope) {
        const container = DOM['secret-variable-list'];
        if (!container) return;

        const hasSecrets = Array.isArray(scope.secrets) && scope.secrets.length > 0;
        if (DOM['secret-variable-empty']) {
            DOM['secret-variable-empty'].classList.toggle('hidden', !hasSecrets);
        }

        const grouped = groupSecretsByRepository(hasSecrets ? scope.secrets : []);
        const sections = [];

        if (grouped.global.length) {
            sections.push(renderSecretSection('', grouped.global, { scopeKey: scope.key }));
        }

        grouped.repositories.forEach(entry => {
            sections.push(renderSecretSection(entry.repo, entry.variables, { scopeKey: scope.key }));
        });

        container.innerHTML = sections.join('');

        container.querySelectorAll('[data-secret-variable]').forEach(button => {
            button.addEventListener('click', () => {
                const name = button.getAttribute('data-secret-variable');
                selectSecret(name);
            });
        });

        container.querySelectorAll('[data-secret-variable-action]').forEach(button => {
            button.addEventListener('click', async event => {
                event.preventDefault();
                event.stopPropagation();
                const action = button.getAttribute('data-secret-variable-action');
                const item = button.closest('[data-secret-variable-item]');
                if (!item) return;
                const secretName = item.getAttribute('data-secret-variable-full');
                const scopeKey = item.getAttribute('data-secret-variable-scope') || scope.key;
                if (!secretName) return;
                const scopeRef = state.scopeMap.get(scopeKey) || state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY);
                if (!scopeRef) return;

                if (state.selectedSecret !== secretName) {
                    await selectSecret(secretName, { silent: true, skipHash: true });
                }

                switch (action) {
                    case 'edit':
                        openEditModal('update', { scopeKey: scopeRef.key, name: secretName });
                        break;
                    case 'delete':
                        openDeleteSecretModal(scopeRef, secretName);
                        break;
                    default:
                        break;
                }
            });
        });

        container.querySelectorAll('[data-secret-variable-clone]').forEach(button => {
            button.addEventListener('click', async event => {
                event.preventDefault();
                event.stopPropagation();
                const item = button.closest('[data-secret-variable-item]');
                if (!item) return;
                const secretName = item.getAttribute('data-secret-variable-full');
                const scopeKey = item.getAttribute('data-secret-variable-scope') || scope.key;
                if (!secretName) return;
                await cloneSecret(scopeKey, secretName);
            });
        });

        highlightActiveSecret(state.selectedSecret);
        updateSecretItemStates();
    }

    function renderVariableList(scope) {
        const container = DOM['variable-list'];
        if (!container) return;

        const hasVariables = Array.isArray(scope.variables) && scope.variables.length > 0;
        if (DOM['variable-empty']) {
            DOM['variable-empty'].classList.toggle('hidden', hasVariables);
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

        container.querySelectorAll('[data-variable-name]').forEach(button => {
            button.addEventListener('click', () => {
                const name = button.getAttribute('data-variable-name');
                selectVariable(name);
            });
        });

        container.querySelectorAll('[data-scope-variable-show]').forEach(button => {
            button.addEventListener('click', async event => {
                event.preventDefault();
                event.stopPropagation();
                const item = button.closest('[data-scope-variable-item]');
                if (!item) return;
                const variableName = item.getAttribute('data-scope-variable-full');
                const scopeKey = item.getAttribute('data-scope-variable-scope') || scope.key;
                if (state.selectedVariable !== variableName) {
                    await selectVariable(variableName, { silent: true, skipHash: true });
                }
                await toggleVariableValue(scopeKey, variableName, item, button);
            });
        });

        container.querySelectorAll('[data-scope-variable-action]').forEach(button => {
            button.addEventListener('click', async event => {
                event.preventDefault();
                event.stopPropagation();
                const action = button.getAttribute('data-scope-variable-action');
                const item = button.closest('[data-scope-variable-item]');
                if (!item) return;
                const variableName = item.getAttribute('data-scope-variable-full');
                const scopeKey = item.getAttribute('data-scope-variable-scope') || scope.key;
                if (!variableName) return;
                const scopeRef = state.scopeMap.get(scopeKey) || state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY);
                if (!scopeRef) return;

                if (state.selectedVariable !== variableName) {
                    await selectVariable(variableName, { silent: true, skipHash: true });
                }

                switch (action) {
                    case 'edit':
                        openVariableEditModal('update', { scopeKey: scopeRef.key, name: variableName });
                        break;
                    case 'delete':
                        openDeleteVariableModal(scopeRef, variableName);
                        break;
                    default:
                        break;
                }
            });
        });

        container.querySelectorAll('[data-scope-variable-clone]').forEach(button => {
            button.addEventListener('click', async event => {
                event.preventDefault();
                event.stopPropagation();
                const item = button.closest('[data-scope-variable-item]');
                if (!item) return;
                const variableName = item.getAttribute('data-scope-variable-full');
                const scopeKey = item.getAttribute('data-scope-variable-scope') || scope.key;
                if (!variableName) return;
                await cloneVariable(scopeKey, variableName);
            });
        });

        highlightActiveVariable(state.selectedVariable);
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
        const scopeRef = state.scopeMap.get(scopeKey) || state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY);

        const groups = items.map(item => {
            const isExpanded = state.expandedVariable === item.full;
            const isActive = item.full === state.selectedVariable;
            const sourceKey = getVariableSourceForEntry(item.full, scopeRef);
            const isEditable = isVariableSourceEditable(sourceKey);

            return `
                <div class="env-variable-item${isActive ? ' env-variable-item--active' : ''}${isExpanded ? ' env-variable-item--expanded' : ''}" data-scope-variable-item data-scope-variable-full="${escapeAttribute(item.full)}" data-scope-variable-scope="${escapeAttribute(scopeKey)}">
                    <div class="env-variable-info">
                        <button type="button" class="env-variable-btn${isActive ? ' env-variable-btn--active' : ''}" data-variable-name="${escapeAttribute(item.full)}">
                            <span class="truncate">${escapeHtml(item.display)}</span>
                        </button>
                    </div>
                    <div class="env-variable-inline-actions">
                        <button type="button" class="env-inline-icon" data-scope-variable-show title="Show value">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-7 11-7 11 7 11 7-4 7-11 7-11-7-11-7z"/><circle cx="12" cy="12" r="3"/></svg>
                        </button>
                        ${isEditable ? `
                        <button type="button" class="env-inline-icon" data-scope-variable-action="edit" title="Edit variable">
                            <svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.5L13.196 5.232z" />
                            </svg>
                        </button>
                        <button type="button" class="env-inline-icon env-inline-icon--danger" data-scope-variable-action="delete" title="Delete variable">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6l1-3h4l1 3"/></svg>
                        </button>
                        ` : `
                        <button type="button" class="env-inline-icon" data-scope-variable-clone="${escapeAttribute(item.full)}" title="Clone">
                            <svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2">
                                <path d="M16 7h-1V4a1 1 0 00-1-1H9a1 1 0 00-1 1v3H7a1 1 0 00-1 1v12a1 1 0 001 1h9a1 1 0 001-1V8a1 1 0 00-1-1zM9 4h5v3H9V4zm2.5 12a2.5 2.5 0 110-5 2.5 2.5 0 010 5z" />
                            </svg>
                        </button>
                        `}
                    </div>
                    <div class="env-variable-value" data-scope-variable-value></div>
                </div>`;
        }).join('');

        const heading = title ? `<h4>${escapeHtml(title)}</h4>` : '';

        return `<section class="env-variable-section">
            ${heading}
            <div class="env-variable-buttons">${groups}</div>
        </section>`;
    }

    function normalizeVariableSourceKey(key) {
        return String(key || '').trim().toLowerCase();
    }

    function formatVariableSourceLabel(key) {
        switch (normalizeVariableSourceKey(key)) {
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

    function getVariableMetadata(name, scopeRef) {
        if (!name) return null;
        let scope = scopeRef;
        if (typeof scope === 'string') {
            scope = state.scopeMap?.get(scope);
        }
        if (scope && scope.variableMeta instanceof Map && scope.variableMeta.has(name)) {
            return scope.variableMeta.get(name);
        }
        const cacheKey = makeScopeValueCacheKey(name, scope?.scopeName || '');
        if (state.variableSources instanceof Map && state.variableSources.has(cacheKey)) {
            return state.variableSources.get(cacheKey);
        }
        return null;
    }

    function getVariableSourceForEntry(name, scopeRef) {
        const meta = getVariableMetadata(name, scopeRef);
        return meta?.source || 'database';
    }

    function isVariableSourceEditable(key) {
        return normalizeVariableSourceKey(key) !== 'git';
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

        if (!options.disableCategorySwitch) {
            setSelectedScopeCategory('variables');
        }
        state.selectedVariable = name;
        highlightActiveVariable(name);
        await ensurePipelineVariableIndex();
        renderVariableDetail(scope, name);

        if (!options.silent && !options.skipHash) {
            navigateToScope(scope.key, null, name);
        }
    }

    function highlightActiveVariable(name) {
        const container = DOM['variable-list'];
        if (!container) return;
        container.querySelectorAll('[data-scope-variable-item]').forEach(item => {
            const button = item.querySelector('[data-variable-name]');
            const value = button ? button.getAttribute('data-variable-name') : null;
            const isActive = value === name && !!name;
            item.classList.toggle('env-variable-item--active', isActive);
            if (button) {
                button.classList.toggle('env-variable-btn--active', isActive);
            }
        });
        updateVariableItemStates();
    }

    function updateVariableItemStates() {
        const container = DOM['variable-list'];
        if (!container) return;
        container.querySelectorAll('[data-scope-variable-item]').forEach(item => {
            const variableFull = item.getAttribute('data-scope-variable-full');
            const scopeKey = item.getAttribute('data-scope-variable-scope') || '';
            const scopeRef = state.scopeMap.get(scopeKey) || state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY) || null;
            const envLabel = scopeRef?.scopeName || '';
            const cacheKey = makeScopeValueCacheKey(variableFull, envLabel);
            const value = state.variableValues.get(cacheKey) || '';
            const isExpanded = state.expandedVariable === variableFull;
            const showButton = item.querySelector('[data-scope-variable-show]');
            const valueContainer = item.querySelector('[data-scope-variable-value]');

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

        const cacheKey = makeScopeValueCacheKey(variableName, scopeRef.scopeName || '');
        let value = state.variableValues.get(cacheKey);
        if (value == null) {
            try {
                button.dataset.loading = 'true';
                button.classList.add('loading');
                button.disabled = true;
                value = await fetchVariableValue(scopeRef, variableName);
                state.variableValues.set(cacheKey, value ?? '');
            } catch (error) {
                console.error('Failed to fetch scope variable value:', error);
                showToast('Failed to load variable value.', 'error');
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
        const envLabel = scope?.scopeName || '';
        const cacheKey = makeScopeValueCacheKey(variableName, envLabel);

        if (state.variableValues.has(cacheKey)) {
            return state.variableValues.get(cacheKey);
        }
        if (state.variableValuePromises.has(cacheKey)) {
            return state.variableValuePromises.get(cacheKey);
        }

        let url = '';
        if (identity.repoOwner && identity.repoName) {
            url = `/v1/repositories/${encodeURIComponent(identity.repoOwner)}/${encodeURIComponent(identity.repoName)}/variables/${encodeURIComponent(identity.name)}`;
        } else {
            url = `/v1/variables/${encodeURIComponent(identity.name)}`;
        }
        if (envLabel) {
            url += `?env=${encodeURIComponent(envLabel)}`;
        }

        const promise = (async () => {
            const { status, data, error } = await fetchVariableResource(url);
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
                state.variableValuePromises.delete(cacheKey);
            }
        })();

        state.variableValuePromises.set(cacheKey, promise);
        const value = await promise;
        state.variableValues.set(cacheKey, value);
        return value;
    }

    function parseVariableIdentity(variableName) {
        const parts = String(variableName || '').split('/').filter(Boolean);
        if (parts.length === 3) {
            return {
                repoOwner: parts[0],
                repoName: parts[1],
                name: parts[2],
            };
        }
        return { repoOwner: '', repoName: '', name: parts[0] || '' };
    }

    function makeScopeValueCacheKey(variableName, envLabel) {
        return `${variableName}@@${envLabel || ''}`;
    }

    function groupSecretsByRepository(secrets) {
        const global = [];
        const repoMap = new Map();

        secrets.forEach(name => {
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

    function collectSecretKnownRepositorySlugs(extraSlug = '') {
        const repos = new Set();

        if (state.scopeMap instanceof Map) {
            state.scopeMap.forEach(scope => {
                (scope.secrets || []).forEach(secret => {
                    const identity = parseSecretIdentity(secret);
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

    function populateSecretRepoSuggestions(preselect = '') {
        const datalist = DOM['secret-repo-options'];
        if (!datalist) return;
        const entries = collectSecretKnownRepositorySlugs(preselect);
        datalist.innerHTML = entries.map(slug => `<option value="${escapeAttribute(slug)}"></option>`).join('');
    }

    function collectVariableKnownRepositorySlugs(extraSlug = '') {
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

    function populateVariableRepoSuggestions(preselect = '') {
        const datalist = DOM['variable-repo-options'];
        if (!datalist) return;
        const entries = collectVariableKnownRepositorySlugs(preselect);
        datalist.innerHTML = entries.map(slug => `<option value="${escapeAttribute(slug)}"></option>`).join('');
    }

    function renderVariableSuggestions(activeScopeKey = state.selectedScopeKey || DEFAULT_SCOPE_KEY) {
        const panel = DOM['variable-suggestion-panel'];
        const list = DOM['variable-suggestion-list'];
        const emptyState = DOM['variable-suggestion-empty'];
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
                    label: scope.scopeName ? `/${scope.scopeName}` : '/ (default)',
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

    function handleVariableSuggestionClick(event) {
        const button = event.target.closest('[data-env-suggestion-value]');
        if (!button) return;
        if (!DOM['variable-edit-modal'] || DOM['variable-edit-modal'].classList.contains('hidden')) {
            return;
        }
        const variableValue = button.getAttribute('data-env-suggestion-value') || '';
        if (!variableValue) return;
        event.preventDefault();
        const identity = parseVariableIdentity(variableValue);
        const nameInput = DOM['variable-edit-name'];
        if (nameInput && !nameInput.readOnly) {
            nameInput.value = identity.name || variableValue;
            try {
                const caret = nameInput.value.length;
                nameInput.setSelectionRange(caret, caret);
            } catch {}
        }
        const repoInput = DOM['variable-edit-repo'];
        if (repoInput && !repoInput.disabled) {
            const repoSlug = identity.repoOwner && identity.repoName ? `${identity.repoOwner}/${identity.repoName}` : '';
            repoInput.value = repoSlug;
        }
        nameInput?.focus();
    }

    function suggestVariableCloneName(scope, repoSlug, baseName) {
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

    function openVariableEditModal(mode, options = {}) {
        const scope = state.scopeMap.get(options.scopeKey || state.selectedScopeKey || DEFAULT_SCOPE_KEY);
        if (!scope) return;

        const header = DOM['variable-edit-modal']?.querySelector('h2');
        if (header) {
            header.textContent = mode === 'update' ? 'Update Variable' : 'Create Variable';
        }

        if (DOM['variable-edit-scope']) {
            DOM['variable-edit-scope'].textContent = scope.scopeName ? `Scope: /${scope.scopeName}` : 'Scope: /';
        }

        const identity = parseVariableIdentity(options.name || '');
        const existingRepoSlug = identity.repoOwner && identity.repoName ? `${identity.repoOwner}/${identity.repoName}` : '';
        const requestedRepoSlug = normalizeRepositorySlug(options.repository || options.repoSlug || '') || existingRepoSlug;
        populateVariableRepoSuggestions(requestedRepoSlug);

        if (DOM['variable-edit-name']) {
            let baseName = '';
            if (mode === 'update') {
                baseName = identity.repoOwner && identity.repoName ? identity.name : (options.name || '');
            } else {
                baseName = typeof options.nameSuggestion === 'string' ? options.nameSuggestion : '';
            }
            DOM['variable-edit-name'].value = baseName;
            DOM['variable-edit-name'].readOnly = mode === 'update';
        }
        if (DOM['variable-edit-repo']) {
            const input = DOM['variable-edit-repo'];
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
        if (DOM['variable-edit-value']) {
            const preset = mode === 'create' && typeof options.valuePreset === 'string'
                ? options.valuePreset
                : '';
            DOM['variable-edit-value'].value = preset;
        }
        if (DOM['variable-edit-form']) {
            DOM['variable-edit-form'].dataset.mode = mode;
            DOM['variable-edit-form'].dataset.scopeKey = scope.key;
            DOM['variable-edit-form'].dataset.variableName = options.name || '';
        }
        if (DOM['variable-edit-submit']) {
            DOM['variable-edit-submit'].textContent = mode === 'update' ? 'Save Value' : 'Create Variable';
        }

        renderVariableSuggestions(scope.key);
        openModal('variable-edit-modal');
    }

    async function handleSubmitVariable(event) {
        event.preventDefault();
        if (!context || typeof context.fetchData !== 'function') return;

        const form = event.currentTarget;
        const mode = form.dataset.mode || 'create';
        const scopeKey = form.dataset.scopeKey || DEFAULT_SCOPE_KEY;
        const nameInputValue = DOM['variable-edit-name']?.value.trim() || '';
        const repoInput = DOM['variable-edit-repo'];
        const value = DOM['variable-edit-value']?.value || '';

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
            urlBase = `/v1/repositories/${encodeURIComponent(targetRepoOwner)}/${encodeURIComponent(targetRepoName)}/variables/${encodeURIComponent(targetVarName)}`;
        } else {
            urlBase = `/v1/variables/${encodeURIComponent(targetVarName)}`;
        }
        const envLabel = scope.scopeName || '';
        const url = envLabel ? `${urlBase}?env=${encodeURIComponent(envLabel)}` : urlBase;

        try {
            await context.fetchData(url, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ value, source: repoSlug ? 'git' : undefined }),
            });
            closeModal('variable-edit-modal');
            await ensureScopeVariablesLoaded(scope, true);
            renderScopeDetail(scope);
            selectVariable(variableName, { silent: true, skipHash: true });
            renderScopeCollection();
            renderSidebarTree();
            showToast(mode === 'update' ? 'Variable updated.' : 'Variable created.', 'success');
        } catch (error) {
            console.error('Failed to save variable', error);
            showToast('Failed to save variable.', 'error');
        }
    }

    function openDeleteVariableModal(scope, name) {
        if (!DOM['variable-delete-modal']) return;
        const message = DOM['variable-delete-message'];
        if (message) {
            const scopeLabel = scope?.scopeName ? `/${scope.scopeName}` : '/';
            message.innerHTML = `Remove <strong>${escapeHtml(name)}</strong> from <strong>${escapeHtml(scopeLabel)}</strong>?`;
        }
        const confirmBtn = DOM['variable-confirm-delete-btn'];
        if (confirmBtn) {
            confirmBtn.dataset.scopeKey = scope?.key || '';
            confirmBtn.dataset.deleteMode = 'variable';
            confirmBtn.dataset.variableName = name;
        }
        openModal('variable-delete-modal');
    }

    async function handleConfirmVariableDelete() {
        const button = DOM['variable-confirm-delete-btn'];
        if (!button) return;
        const scopeKey = button.dataset.scopeKey;
        const variableName = button.dataset.variableName;
        const scope = state.scopeMap.get(scopeKey);
        if (!scope || !variableName) {
            closeModal('variable-delete-modal');
            return;
        }

        const success = await deleteVariable(scope, variableName);
        if (success) {
            closeModal('variable-delete-modal');
            await ensureScopeVariablesLoaded(scope, true);
            renderScopeDetail(scope);
            selectVariable(variableName, { silent: true, skipHash: true });
            renderScopeCollection();
            renderSidebarTree();
            showToast('Variable removed.', 'success');
        }
    }

    async function cloneVariable(scopeKey, variableName) {
        const scope = state.scopeMap.get(scopeKey);
        if (!scope) return;
        const identity = parseVariableIdentity(variableName);
        const repoSlug = identity.repoOwner && identity.repoName ? `${identity.repoOwner}/${identity.repoName}` : '';
        const suggestion = suggestVariableCloneName(scope, repoSlug, identity.name || variableName);
        openVariableEditModal('create', {
            scopeKey: scope.key,
            repoSlug,
            nameSuggestion: suggestion,
            valuePreset: '',
        });
    }

    async function deleteVariable(scope, name) {
        if (!context || typeof context.fetchData !== 'function') return false;
        const identity = parseVariableIdentity(name);
        const envLabel = scope?.scopeName || '';
        let urlBase = '';
        if (identity.repoOwner && identity.repoName) {
            urlBase = `/v1/repositories/${encodeURIComponent(identity.repoOwner)}/${encodeURIComponent(identity.repoName)}/variables/${encodeURIComponent(identity.name)}`;
        } else {
            urlBase = `/v1/variables/${encodeURIComponent(identity.name)}`;
        }
        const url = envLabel ? `${urlBase}?env=${encodeURIComponent(envLabel)}` : urlBase;

        try {
            await context.fetchData(url, { method: 'DELETE' });
            return true;
        } catch (error) {
            console.error('Failed to delete variable', error);
            showToast('Failed to delete variable.', 'error');
            return false;
        }
    }

    function openNewScopeModal(parentPath = '') {
        const modal = DOM['scope-new-modal'];
        if (!modal) return;
        state.pendingScopeParent = normalizeScopeLabel(parentPath || '');
        if (DOM['scope-new-parent']) {
            DOM['scope-new-parent'].textContent = state.pendingScopeParent ? `/${state.pendingScopeParent}` : '/';
        }
        if (DOM['scope-new-name']) {
            DOM['scope-new-name'].value = '';
            DOM['scope-new-name'].focus();
        }
        openModal('scope-new-modal');
    }

    function hideNewScopeModal() {
        closeModal('scope-new-modal');
        state.pendingScopeParent = '';
    }

    async function handleCreateScope(event) {
        event.preventDefault();
        if (!context || typeof context.fetchData !== 'function') return;

        const parentLabel = state.pendingScopeParent || '';
        const inputEl = DOM['scope-new-name'];
        const rawInput = inputEl ? inputEl.value : '';
        const segments = sanitizeScopeSegments(rawInput);
        if (!segments.length) {
            showToast('Scope name is required.', 'error');
            return;
        }

        const parentSegments = sanitizeScopeSegments(parentLabel);
        const combinedSegments = parentSegments.concat(segments);
        const envLabel = combinedSegments.join('/');
        const normalizedLabel = normalizeScopeLabel(envLabel);

        if (!normalizedLabel) {
            showToast('Scope name is required.', 'error');
            return;
        }

        const newScopeKey = buildScopeKey(normalizedLabel);
        if (state.scopeMap.has(newScopeKey)) {
            showToast(`Scope '/${normalizedLabel}' already exists.`, 'error');
            return;
        }

        const scopeChain = [];
        combinedSegments.forEach((_, index) => {
            const partial = normalizeScopeLabel(combinedSegments.slice(0, index + 1).join('/'));
            if (partial) {
                scopeChain.push(partial);
            }
        });

        try {
            for (const path of scopeChain) {
                await createSampleEntriesForScope(path);
            }
            showToast(`Scope '/${normalizedLabel}' created.`, 'success');
        } catch (error) {
            console.error('Failed to create scope', error);
            showToast('Failed to create scope.', 'error');
            return;
        }

        hideNewScopeModal();

        await preloadData(true);

        state.activeFolderKey = parentLabel;
        ensureSidebarExpansionForPath(parentLabel);

        if (state.scopeMap.has(newScopeKey)) {
            await selectScope(newScopeKey, { forceReload: true, skipHash: false });
            selectVariable(SAMPLE_SCOPE_VARIABLE, { silent: true, skipHash: true });
            selectSecret(SAMPLE_SCOPE_SECRET, { silent: true, skipHash: true });
        } else {
            renderScopeCollection();
        }
    }

    async function createSampleEntriesForScope(normalizedLabel) {
        if (!context || typeof context.fetchData !== 'function') return;
        const safeLabel = normalizeScopeLabel(normalizedLabel || '');
        const scopeKey = buildScopeKey(safeLabel);
        const scope = state.scopeMap.get(scopeKey);
        const hasSampleVariable = Array.isArray(scope?.variables) && scope.variables.includes(SAMPLE_SCOPE_VARIABLE);
        const hasSampleSecret = Array.isArray(scope?.secrets) && scope.secrets.includes(SAMPLE_SCOPE_SECRET);
        const needsVariable = !hasSampleVariable;
        const needsSecret = !hasSampleSecret;
        if (!needsVariable && !needsSecret) {
            return;
        }

        const tasks = [];
        if (needsVariable) {
            const variableUrlBase = `/v1/variables/${encodeURIComponent(SAMPLE_SCOPE_VARIABLE)}`;
            const sampleValue = SAMPLE_SCOPE_VALUE.replace('%SCOPE%', safeLabel || 'default');
            const variableUrl = safeLabel
                ? `${variableUrlBase}?env=${encodeURIComponent(safeLabel)}`
                : variableUrlBase;
            tasks.push(context.fetchData(variableUrl, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ value: sampleValue }),
            }));
        }
        if (needsSecret) {
            const secretUrlBase = `/v1/secrets/${encodeURIComponent(SAMPLE_SCOPE_SECRET)}`;
            const sampleSecretValue = SAMPLE_SCOPE_SECRET_VALUE.replace('%SCOPE%', safeLabel || 'default');
            const secretUrl = safeLabel
                ? `${secretUrlBase}?env=${encodeURIComponent(safeLabel)}`
                : secretUrlBase;
            tasks.push(context.fetchData(secretUrl, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ value: sampleSecretValue }),
            }));
        }
        await Promise.all(tasks);
    }

    function sanitizeScopeSegments(raw) {
        if (!raw) return [];
        return String(raw)
            .split('/')
            .map(part => part.trim().replace(/[^A-Za-z0-9_.-]+/g, '-').replace(/^-+|-+$/g, ''))
            .filter(Boolean);
    }

    function renderSecretSection(title, items, options = {}) {
        if (!Array.isArray(items) || !items.length) return '';
        const scopeKey = options.scopeKey || '';
        const scopeRef = state.scopeMap.get(scopeKey) || state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY);

        const groups = items.map(item => {
            const isActive = item.full === state.selectedSecret;
            const sourceKey = getSecretSourceForSecret(item.full, scopeRef);
            const isEditable = isSecretSourceEditable(sourceKey);

            return `
                <div class="env-variable-item${isActive ? ' env-variable-item--active' : ''}" data-secret-variable-item data-secret-variable-full="${escapeAttribute(item.full)}" data-secret-variable-scope="${escapeAttribute(scopeKey)}">
                    <div class="env-variable-info">
                        <button type="button" class="env-variable-btn${isActive ? ' env-variable-btn--active' : ''}" data-secret-variable="${escapeAttribute(item.full)}">
                            <span class="truncate">${escapeHtml(item.display)}</span>
                        </button>
                    </div>
                    <div class="env-variable-inline-actions">
                        ${isEditable ? `
                        <button type="button" class="env-inline-icon" data-secret-variable-action="edit" title="Edit secret">
                            <svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.5L13.196 5.232z" />
                            </svg>
                        </button>
                        <button type="button" class="env-inline-icon env-inline-icon--danger" data-secret-variable-action="delete" title="Delete secret">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6l1-3h4l1 3"/></svg>
                        </button>
                        ` : `
                        <button type="button" class="env-inline-icon" data-secret-variable-clone="${escapeAttribute(item.full)}" title="Clone">
                            <svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2">
                                <path d="M16 7h-1V4a1 1 0 00-1-1H9a1 1 0 00-1 1v3H7a1 1 0 00-1 1v12a1 1 0 001 1h9a1 1 0 001-1V8a1 1 0 00-1-1zM9 4h5v3H9V4zm2.5 12a2.5 2.5 0 110-5 2.5 2.5 0 010 5z" />
                            </svg>
                        </button>
                        `}
                    </div>
                    <div class="env-variable-value" data-secret-variable-value></div>
                </div>`;
        }).join('');

        const heading = title ? `<h4>${escapeHtml(title)}</h4>` : '';

        return `<section class="env-variable-section">
            ${heading}
            <div class="env-variable-buttons">${groups}</div>
        </section>`;
    }

    async function selectSecret(name, options = {}) {
        if (!name) {
            clearSecretDetail();
            return;
        }

        const scope = state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY);
        if (!scope) return;
        if (!scope.secrets.includes(name)) {
            if (!options.silent) {
                showToast('Secret not found in selected scope.', 'error');
            }
            return;
        }

        if (!options.disableCategorySwitch) {
            setSelectedScopeCategory('secrets');
        }
        state.selectedSecret = name;
        highlightActiveSecret(name);
        await ensurePipelineSecretIndex();
        renderSecretDetail(scope, name);

        if (!options.silent && !options.skipHash) {
            navigateToScope(scope.key, name);
        }
    }

    function highlightActiveSecret(name) {
        const container = DOM['secret-variable-list'];
        if (!container) return;
        container.querySelectorAll('[data-secret-variable-item]').forEach(item => {
            const button = item.querySelector('[data-secret-variable]');
            const value = button ? button.getAttribute('data-secret-variable') : null;
            const isActive = value === name && !!name;
            item.classList.toggle('env-variable-item--active', isActive);
            if (button) {
                button.classList.toggle('env-variable-btn--active', isActive);
            }
        });
        updateSecretItemStates();
    }

    function updateSecretItemStates() {
        const container = DOM['secret-variable-list'];
        if (!container) return;
        container.querySelectorAll('[data-secret-variable-item]').forEach(item => {
            const secretFull = item.getAttribute('data-secret-variable-full');
            const isActive = secretFull === state.selectedSecret;
            const valueContainer = item.querySelector('[data-secret-variable-value]');
        });
    }

    function parseSecretIdentity(secretName) {
        const parts = String(secretName || '').split('/').filter(Boolean);
        if (parts.length === 3) {
            return { repoOwner: parts[0], repoName: parts[1], name: parts[2] };
        }
        return { repoOwner: null, repoName: null, name: secretName };
    }

    function makeSecretValueCacheKey(secretName, scopeLabel) {
        return `${scopeLabel || ''}::${secretName}`;
    }

    async function fetchSecretResource(url, options = {}) {
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
            console.error('Secret fetch failed', { url: target, error });
            return { status: 0, data: null, error };
        }
    }

    async function fetchVariableResource(url, options = {}) {
        const base = (context && typeof context.apiBaseUrl === 'string') ? context.apiBaseUrl : '';
        const target = `${base || ''}${url}`;
        try {
            const response = await fetch(target, options);
            if (response.status === 404) {
                return { status: 404, data: null };
            }
            if (!response.ok) {
                const errorText = await response.text();
                return { status: response.status, data: null, error: new Error(errorText || 'Request failed') };
            }
            const contentType = response.headers.get('content-type') || '';
            if (contentType.includes('application/json')) {
                return { status: response.status, data: await response.json() };
            }
            return { status: response.status, data: await response.text() };
        } catch (error) {
            return { status: 500, data: null, error };
        }
    }

    async function ensurePipelineSecretIndex() {
        if (state.pipelineSecretIndexReady) return;
        if (state.pipelineSecretIndexPromise) {
            await state.pipelineSecretIndexPromise;
            return;
        }

        state.pipelineSecretIndexPromise = (async () => {
            if (!context || typeof context.fetchData !== 'function') return;
            try {
                const response = await context.fetchData('/v1/pipelines?include_source=true');
                const identifiers = normalizePipelineList(response);
                for (const identifier of identifiers) {
                    const yaml = await context.fetchData(`/v1/pipelines/${identifier.split('/').map(encodeURIComponent).join('/')}`);
                    if (typeof yaml !== 'string') continue;
                    const details = parseYaml(yaml);
                    const vars = extractPipelineSecrets(details);
                    const seed = state.pipelineMetaSeeds instanceof Map ? state.pipelineMetaSeeds.get(identifier) : null;
                    const meta = buildPipelineMeta(identifier, details, seed);
                    state.pipelineMetadata.set(identifier, meta);
                    vars.forEach(secret => {
                        const key = secret.trim();
                        if (!key) return;
                        const entries = state.pipelineSecretIndex.get(key) || new Set();
                        entries.add(identifier);
                        state.pipelineSecretIndex.set(key, entries);
                    });
                }
                state.pipelineSecretIndexReady = true;
            } catch (error) {
                console.error('Failed to build pipeline secret index:', error);
            }
        })();

        try {
            await state.pipelineSecretIndexPromise;
        } finally {
            state.pipelineSecretIndexPromise = null;
        }
    }

    async function ensurePipelineVariableIndex() {
        if (state.pipelineVariableIndexReady) return;
        if (state.pipelineVariableIndexPromise) {
            await state.pipelineVariableIndexPromise;
            return;
        }

        state.pipelineVariableIndexPromise = (async () => {
            if (!context || typeof context.fetchData !== 'function') return;
            try {
                const response = await context.fetchData('/v1/pipelines?include_source=true');
                const identifiers = normalizePipelineList(response);
                for (const identifier of identifiers) {
                    const yaml = await context.fetchData(`/v1/pipelines/${identifier.split('/').map(encodeURIComponent).join('/')}`);
                    if (typeof yaml !== 'string') continue;
                    const details = parseYaml(yaml);
                    const vars = extractScopeVariables(details);
                    const seed = state.pipelineMetaSeeds instanceof Map ? state.pipelineMetaSeeds.get(identifier) : null;
                    const meta = buildPipelineMeta(identifier, details, seed);
                    state.pipelineMetadata.set(identifier, meta);
                    vars.forEach(variable => {
                        const key = variable.trim();
                        if (!key) return;
                        const entries = state.pipelineVariableIndex.get(key) || new Set();
                        entries.add(identifier);
                        state.pipelineVariableIndex.set(key, entries);
                    });
                }
                state.pipelineVariableIndexReady = true;
            } catch (error) {
                console.error('Failed to build pipeline variable index:', error);
            }
        })();

        try {
            await state.pipelineVariableIndexPromise;
        } finally {
            state.pipelineVariableIndexPromise = null;
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

    function extractPipelineSecrets(details) {
        if (!details || !Array.isArray(details.steps)) return [];
        const secrets = new Set();
        details.steps.forEach(step => {
            if (step && Array.isArray(step.secrets)) {
                step.secrets.forEach(secret => {
                    if (secret && typeof secret === 'string') {
                        secrets.add(secret.trim());
                    }
                });
            }
        });
        return Array.from(secrets);
    }

    function extractScopeVariables(details) {
        if (!details || typeof details !== 'object') return [];
        const variables = new Set();

        function collectFromEnvironment(env) {
            if (!env || typeof env !== 'object') return;
            Object.keys(env).forEach(key => {
                if (!key) return;
                const value = env[key];
                if (value && typeof value === 'string') {
                    variables.add(value.trim());
                }
            });
        }

        collectFromEnvironment(details.environment);

        if (Array.isArray(details.steps)) {
            details.steps.forEach(step => {
                collectFromEnvironment(step?.environment);
                if (Array.isArray(step?.tasks)) {
                    step.tasks.forEach(task => collectFromEnvironment(task?.environment));
                }
            });
        }

        return Array.from(variables);
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
        const pipelineEntries = Array.from(state.pipelineVariableIndex.get(name) || []);
        renderRelatedCollection('variable-pipelines', pipelineEntries.map(renderPipelineDetail), 'No pipelines declare this variable.');

        const relatedTriggers = scope.triggers;
        renderRelatedCollection('variable-triggers', relatedTriggers.map(renderTriggerDetail), 'No triggers reference this scope.');
    }

    function renderSecretDetail(scope, name) {
        updateSecretDetailMeta(name, scope);
        const pipelineEntries = Array.from(state.pipelineSecretIndex.get(name) || []);
        renderRelatedCollection('secret-variable-pipelines', pipelineEntries.map(renderPipelineDetail), 'No pipelines declare this secret.');

        const relatedTriggers = scope.triggers;
        renderRelatedCollection('secret-variable-triggers', relatedTriggers.map(renderTriggerDetail), 'No triggers reference this scope.');
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
        const scopeLabel = trigger.scope ? `/${trigger.scope}` : '/';
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
        state.expandedVariable = null;
        if (DOM['variable-pipelines']) {
            DOM['variable-pipelines'].innerHTML = '';
            DOM['variable-pipelines'].dataset.empty = 'No pipelines declare this variable.';
        }
        if (DOM['variable-triggers']) {
            DOM['variable-triggers'].innerHTML = '';
            DOM['variable-triggers'].dataset.empty = 'No triggers reference this scope.';
        }
        if (DOM['variable-detail-label']) {
            DOM['variable-detail-label'].textContent = 'Select a variable to inspect details.';
        }
        ['variable-detail-source', 'variable-detail-updated', 'variable-detail-created'].forEach(id => {
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

    function clearSecretDetail() {
        state.selectedSecret = null;
        if (DOM['secret-variable-pipelines']) {
            DOM['secret-variable-pipelines'].innerHTML = '';
            DOM['secret-variable-pipelines'].dataset.empty = 'No pipelines declare this secret.';
        }
        if (DOM['secret-variable-triggers']) {
            DOM['secret-variable-triggers'].innerHTML = '';
            DOM['secret-variable-triggers'].dataset.empty = 'No triggers reference this scope.';
        }
        if (DOM['secret-variable-detail-label']) {
            DOM['secret-variable-detail-label'].textContent = 'Select a secret to inspect details.';
        }
        ['secret-variable-detail-source', 'secret-variable-detail-updated', 'secret-variable-detail-created'].forEach(id => {
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
        const labelEl = DOM['variable-detail-label'];
        const sourceEl = DOM['variable-detail-source'];
        const updatedEl = DOM['variable-detail-updated'];
        const createdEl = DOM['variable-detail-created'];
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

        const meta = getVariableMetadata(name, scope);
        labelEl.textContent = name;

        if (sourceValueEl && meta?.source) {
            const sourceKey = meta.source;
            sourceValueEl.textContent = formatVariableSourceLabel(sourceKey);
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

    function updateSecretDetailMeta(name, scope) {
        const labelEl = DOM['secret-variable-detail-label'];
        const sourceEl = DOM['secret-variable-detail-source'];
        const updatedEl = DOM['secret-variable-detail-updated'];
        const createdEl = DOM['secret-variable-detail-created'];
        if (!labelEl || !sourceEl || !updatedEl || !createdEl) return;
        const sourceValueEl = sourceEl.querySelector('.env-variable-detail-meta-value');
        const updatedValueEl = updatedEl.querySelector('.env-variable-detail-meta-value');
        const createdValueEl = createdEl.querySelector('.env-variable-detail-meta-value');

        if (!name) {
            labelEl.textContent = 'Select a secret to inspect details.';
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

        const meta = getSecretMetadata(name, scope);
        labelEl.textContent = name;

        if (sourceValueEl && meta?.source) {
            const sourceKey = meta.source;
            sourceValueEl.textContent = formatSecretSourceLabel(sourceKey);
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

    async function cloneSecret(scopeKey, secretName) {
        const scope = state.scopeMap.get(scopeKey) || state.scopeMap.get(state.selectedScopeKey || DEFAULT_SCOPE_KEY);
        if (!scope) return;
        const identity = parseSecretIdentity(secretName);
        const repoSlug = identity.repoOwner && identity.repoName ? `${identity.repoOwner}/${identity.repoName}` : '';
        const baseName = identity.name || secretName;
        const suggestion = suggestSecretCloneName(scope, repoSlug, baseName);
        openEditModal('create', {
            scopeKey: scope.key,
            repository: repoSlug,
            nameSuggestion: suggestion,
        });
    }
    
    function suggestSecretCloneName(scope, repoSlug, baseName) {
        const sanitizedBase = String(baseName || 'secret')
            .trim()
            .replace(/[^A-Za-z0-9_.-]+/g, '-')
            .replace(/^-+|-+$/g, '') || 'secret';
        const normalizedRepo = repoSlug || '';
        const existing = new Set((scope.secrets || []).map(name => name.toLowerCase()));
        const buildFull = candidate => (normalizedRepo ? `${normalizedRepo}/${candidate}` : candidate).toLowerCase();
        let candidate = `${sanitizedBase}_copy`;
        let counter = 2;
        while (existing.has(buildFull(candidate))) {
            candidate = `${sanitizedBase}_copy_${counter++}`;
        }
        return candidate;
    }

    function openEditModal(mode, options = {}) {
        const scopeKey = options.scopeKey || state.selectedScopeKey || DEFAULT_SCOPE_KEY;
        const scope = state.scopeMap.get(scopeKey);
        if (!scope) return;

        const header = DOM['secret-edit-modal']?.querySelector('h2');
        if (header) {
            header.textContent = mode === 'update' ? 'Update Secret' : 'Create Secret';
        }

        if (DOM['secret-edit-scope']) {
            const scopeLabel = scope.scopeName ? `/${scope.scopeName}` : '/';
            DOM['secret-edit-scope'].textContent = `Scope: ${scopeLabel}`;
        }

        const identity = parseSecretIdentity(options.name || '');
        const existingRepoSlug = identity.repoOwner && identity.repoName ? `${identity.repoOwner}/${identity.repoName}` : '';
        const requestedRepoSlug = normalizeRepositorySlug(options.repository || options.repoSlug || '') || existingRepoSlug;
        populateSecretRepoSuggestions(requestedRepoSlug);

        if (DOM['secret-edit-name']) {
            let baseName = '';
            if (mode === 'update') {
                baseName = identity.repoOwner && identity.repoName ? identity.name : (options.name || '');
            } else {
                baseName = typeof options.nameSuggestion === 'string' ? options.nameSuggestion : '';
            }
            DOM['secret-edit-name'].value = baseName;
            DOM['secret-edit-name'].readOnly = mode === 'update';
        }
        if (DOM['secret-edit-repo']) {
            const input = DOM['secret-edit-repo'];
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
        if (DOM['secret-edit-value']) {
            const preset = mode === 'create' && typeof options.valuePreset === 'string'
                ? options.valuePreset
                : '';
            DOM['secret-edit-value'].value = preset;
            if (mode === 'update') {
                DOM['secret-edit-value'].placeholder = 'Enter new value (leave blank to keep unchanged)';
            } else {
                DOM['secret-edit-value'].placeholder = 'Provide the secret value';
            }
        }
        if (DOM['secret-edit-form']) {
            DOM['secret-edit-form'].dataset.mode = mode;
            DOM['secret-edit-form'].dataset.scopeKey = scopeKey;
            DOM['secret-edit-form'].dataset.secretName = options.name || '';
        }
        if (DOM['secret-edit-submit']) {
            DOM['secret-edit-submit'].textContent = mode === 'update' ? 'Save Value' : 'Create Secret';
        }

        renderSecretSuggestions(scope.key);
        openModal('secret-edit-modal');
    }

    async function handleSubmitSecret(event) {
        event.preventDefault();
        if (!context || typeof context.fetchData !== 'function') return;

        const form = event.currentTarget;
        const mode = form.dataset.mode || 'create';
        const scopeKey = form.dataset.scopeKey || DEFAULT_SCOPE_KEY;
        const nameInputValue = DOM['secret-edit-name']?.value.trim() || '';
        const repoInput = DOM['secret-edit-repo'];
        const value = DOM['secret-edit-value']?.value || '';

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

        let secretName = form.dataset.secretName || '';
        let targetRepoOwner = '';
        let targetRepoName = '';
        let targetSecretName = '';

        if (mode === 'update') {
            const identity = parseSecretIdentity(secretName || nameInputValue);
            targetSecretName = identity.name || nameInputValue;
            targetRepoOwner = identity.repoOwner || '';
            targetRepoName = identity.repoName || '';
            if (targetRepoOwner && targetRepoName) {
                repoSlug = `${targetRepoOwner}/${targetRepoName}`;
            }
            secretName = identity.repoOwner && identity.repoName
                ? `${identity.repoOwner}/${identity.repoName}/${targetSecretName}`
                : targetSecretName;
        } else {
            if (!nameInputValue) {
                showToast('Secret name is required.', 'error');
                return;
            }
            if (repoSlug && nameInputValue.includes('/')) {
                showToast('Secret name should not include “/” when a repository is selected.', 'error');
                return;
            }
            targetSecretName = nameInputValue;
            if (repoSlug) {
                [targetRepoOwner, targetRepoName] = repoSlug.split('/');
            }
            secretName = repoSlug ? `${repoSlug}/${nameInputValue}` : nameInputValue;
        }

        if (!secretName || !targetSecretName) {
            showToast('Secret name is required.', 'error');
            return;
        }
        const scope = state.scopeMap.get(scopeKey);
        if (!scope) {
            showToast('Unknown scope selected.', 'error');
            return;
        }
        if (!value && mode === 'create') {
            showToast('Provide a value for the new secret.', 'error');
            return;
        }
        
        if (mode === 'update' && !value) {
            showToast('Secret value updated (unchanged).', 'info');
            closeModal('secret-edit-modal');
            return;
        }

        let urlBase = '';
        if (targetRepoOwner && targetRepoName) {
            urlBase = `/v1/repositories/${encodeURIComponent(targetRepoOwner)}/${encodeURIComponent(targetRepoName)}/secrets/${encodeURIComponent(targetSecretName)}`;
        } else {
            urlBase = `/v1/secrets/${encodeURIComponent(targetSecretName)}`;
        }
        const url = scope.scopeName ? `${urlBase}?env=${encodeURIComponent(scope.scopeName)}` : urlBase;

        try {
            await context.fetchData(url, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ value }),
            });
            showToast(mode === 'update' ? 'Secret value updated.' : 'Secret created.', 'success');
            closeModal('secret-edit-modal');
            await ensureScopeSecretsLoaded(scope, true);
            syncScopeVisibility(scope);
            renderScopeDetail(scope);
            selectSecret(secretName, { silent: true, skipHash: true });
            renderScopeCollection();
            renderSidebarTree();
        } catch (error) {
            console.error('Failed to save secret:', error);
            showToast('Failed to save secret.', 'error');
        }
    }

    function openDeleteSecretModal(scope, name) {
        const identity = parseSecretIdentity(name || '');
        const displayName = identity.repoOwner && identity.repoName
            ? `${identity.repoOwner}/${identity.repoName}/${identity.name}`
            : identity.name || name;
        if (DOM['secret-delete-message']) {
            const scopeLabel = scope.scopeName ? `/${scope.scopeName}` : '/';
            DOM['secret-delete-message'].textContent = `Remove “${displayName}” from ${scopeLabel} scope?`;
        }
        if (DOM['secret-confirm-delete-btn']) {
            const btn = DOM['secret-confirm-delete-btn'];
            btn.dataset.scopeKey = scope.key;
            btn.dataset.secretName = name;
            btn.dataset.deleteMode = 'variable';
        }
        openModal('secret-delete-modal');
    }

    function openDeleteSecretScopeModal(scopeKey) {
        // This functionality is disabled for the Scopes page to avoid accidental scope deletion.
        // Scopes should be managed from the dedicated Scopes page.
        showToast('Secret scopes are managed from the Scopes page.', 'info');
    }

    async function deleteSecret(scope, name) {
        if (!scope || !name) return false;

        const identity = parseSecretIdentity(name);
        const repoOwner = identity.repoOwner || '';
        const repoName = identity.repoName || '';
        const baseName = identity.name || name;

        let urlBase = '';
        if (repoOwner && repoName) {
            urlBase = `/v1/repositories/${encodeURIComponent(repoOwner)}/${encodeURIComponent(repoName)}/secrets/${encodeURIComponent(baseName)}`;
        } else {
            urlBase = `/v1/secrets/${encodeURIComponent(baseName)}`;
        }
        const url = scope.scopeName ? `${urlBase}?env=${encodeURIComponent(scope.scopeName)}` : urlBase;

        try {
            await context.deleteData(url);
            return true;
        } catch (error) {
            console.error('Failed to delete secret:', error);
            showToast('Failed to delete secret.', 'error');
            return false;
        }
    }

    async function handleConfirmDelete() {
        const button = DOM['secret-confirm-delete-btn'];
        if (!button) return;
        const scopeKey = button.dataset.scopeKey;
        const scope = state.scopeMap.get(scopeKey);
        if (!scope) {
            closeModal('secret-delete-modal');
            return;
        }

        const mode = button.dataset.deleteMode || 'variable';

        if (mode === 'scope') {
            // Deleting scopes from the Secrets page is disallowed.
            showToast('Secret scopes are managed from the Scopes page.', 'info');
            closeModal('secret-delete-modal');
            return;
        }

        const name = button.dataset.secretName;
        if (!name) {
            closeModal('secret-delete-modal');
            return;
        }

        const success = await deleteSecret(scope, name);
        if (success) {
            closeModal('secret-delete-modal');
            await ensureScopeSecretsLoaded(scope, true);
            syncScopeVisibility(scope);
            renderScopeDetail(scope);
            selectSecret(name, { silent: true, skipHash: true });
            renderScopeCollection();
            renderSidebarTree();
            showToast('Secret removed.', 'success');
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
        const info = parseSecretHash(hash);
        await preloadData();

        const normalizedPath = normalizeScopeLabel(info.envPath || '');
        const scopeKey = buildScopeKey(normalizedPath);
        const isRootPath = !info.folderMode && !info.isDefaultScope && normalizedPath === '';

        if (!info.folderMode && normalizedPath && !state.scopeMap.has(scopeKey)) {
            const scope = createScopeRecord(normalizedPath);
            state.scopeMap.set(scope.key, scope);
            state.scopes.push(scope);
            state.filteredScopes = state.scopes.slice();
            state.scopeTree = buildSecretTree(state.scopes);
        }

        const scopeExists = !info.folderMode && !isRootPath && state.scopeMap.has(scopeKey);

        if (scopeExists) {
            const targetCategory = info.category || (info.variableName ? 'variables' : 'secrets');
            await selectScope(scopeKey, { silent: true, skipHash: true, category: targetCategory });
            if (info.variableName) {
                await selectVariable(info.variableName, { silent: true, skipHash: true });
            } else if (info.secretName) {
                await selectSecret(info.secretName, { silent: true, skipHash: true });
            }
            return;
        }

        resetSecretSelection({ showList: true });
        setActiveFolder(normalizedPath, { ensure: true, refreshList: true, force: true });
        highlightActiveSecretCard();
    }

    function parseSecretHash(hash) {
        const raw = String(hash || '').replace(/^#/, '');
        const parts = raw.split('/').filter(Boolean);
        const path = parts[0] || 'scopes';
        let segments = parts.slice(1);
        let folderMode = false;
        if (segments[0] === 'folder') {
            folderMode = true;
            segments = segments.slice(1);
        }

        const isDefaultScope = !folderMode && segments.length > 0 && segments[0] === 'default';
        const decodedSegments = segments.map((segment, index) => decodeSecretSegment(segment, index, folderMode));
        let secretName = null;
        let variableName = null;
        let category = null;
        let envSegments = decodedSegments.slice();
        if (!folderMode && envSegments.length) {
            const last = envSegments[envSegments.length - 1];
            if (last === 'secrets' || last === 'variables') {
                category = last;
                envSegments = envSegments.slice(0, -1);
            }
        }
        if (!folderMode && envSegments.length >= 2) {
            const tailFirst = envSegments[envSegments.length - 2];
            if (tailFirst === 'secrets') {
                secretName = envSegments[envSegments.length - 1];
                envSegments = envSegments.slice(0, -2);
                category = 'secrets';
            } else if (tailFirst === 'variables') {
                variableName = envSegments[envSegments.length - 1];
                envSegments = envSegments.slice(0, -2);
                category = 'variables';
            }
        }

        if (!folderMode && !category && !secretName && !variableName) {
            folderMode = true;
        }

        const envPath = envSegments.filter(Boolean).join('/');

        return {
            path,
            envPath,
            secretName,
            variableName,
            category,
            folderMode,
            isDefaultScope,
        };
    }

    function renderSidebarForRoute() {
        renderSidebarTree();
    }

    function refresh(force = false) {
        preloadData(force).catch(error => console.error('Failed to refresh secret data:', error));
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
    global.pages.scopes = {
        init,
        handleRoute,
        refresh,
        renderSidebarForRoute,
    };
})(window.NopsAI = window.NopsAI || {});
