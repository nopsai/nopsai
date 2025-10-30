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
        pipelineSourceIndex: null,
        pipelineMetaCache: new Map(),
        pipelineMetaPromises: new Map(),
        _prefetching: new Set(),
    };

    const DOM = {};
    let context = null;
    let initialized = false;

    const RUNS_CACHE_TTL = 60 * 1000;
    const MAX_RUNS = 5;

    function init(ctx = {}) {
        if (initialized) return;
        context = ctx;
        cacheDom();
        bindEvents();
        loadTriggers(true).catch(err => console.error('Failed to load triggers:', err));
        initialized = true;
    }

    function cacheDom() {
        const ids = [
            'triggers-search-container', 'triggers-search', 'triggers-clear-search', 'triggers-new-btn',
            'triggers-list', 'triggers-list-empty', 'triggers-detail', 'triggers-detail-name',
            'triggers-detail-source', 'triggers-detail-meta', 'triggers-meta-chips', 'triggers-yaml-content',
            'triggers-editor-container', 'triggers-line-numbers', 'triggers-yaml-editor',
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

    function rebuildTriggerSources(slugs) {
        const previous = state.triggerSources instanceof Map ? state.triggerSources : new Map();
        const map = new Map();
        if (Array.isArray(slugs)) {
            slugs.forEach(slug => {
                if (!slug) return;
                const normalizedSlug = String(slug).trim();
                if (!normalizedSlug) return;
                const existing = previous.get(normalizedSlug);
                if (existing) {
                    map.set(normalizedSlug, existing);
                } else {
                    map.set(normalizedSlug, 'database');
                }
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

    function normalizePipelineSourceKey(value) {
        if (value == null) return '';
        const key = String(value).trim().toLowerCase();
        if (!key) return '';
        if (key.includes('git')) return 'git';
        if (key.includes('draft')) return 'draft';
        if (key.includes('database') || key === 'db') return 'database';
        return key;
    }

    async function ensurePipelineSourceIndex() {
        if (state.pipelineSourceIndex instanceof Map) {
            return state.pipelineSourceIndex;
        }
        const map = new Map();
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
        if (!id) return { version: 'latest', source: getTriggerSourceLabel('database') };

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
                sourceKey = 'git';
                if (state.pipelineSourceIndex instanceof Map && !state.pipelineSourceIndex.has(id)) {
                    state.pipelineSourceIndex.set(id, sourceKey);
                }
            }
            const sourceLabel = getTriggerSourceLabel(sourceKey) || getTriggerSourceLabel('database');

            let version = 'latest';
            if (sourceKey !== 'git' && context && typeof context.fetchData === 'function') {
                const encodedId = id.split('/').map(encodeURIComponent).join('/');
                const yaml = await context.fetchData(`/v1/pipelines/${encodedId}`);
                if (typeof yaml === 'string') {
                    version = extractPipelineVersion(yaml);
                }
            }

            const meta = { version, source: sourceLabel };
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
            DOM['triggers-new-btn'].addEventListener('click', () => openModal('triggers-new-modal'));
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
            DOM['triggers-yaml-editor'].addEventListener('input', handleEditorInput);
            DOM['triggers-yaml-editor'].addEventListener('scroll', syncEditorScroll);
        }

        document.addEventListener('keydown', (event) => {
            if (event.key === 'Escape') {
                closeModal('triggers-new-modal');
                closeModal('triggers-delete-modal');
                closeModal('triggers-clone-modal');
            }
        });
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
            const slugs = await context.fetchData('/v1/overrides');
            if (Array.isArray(slugs)) {
                state.triggers = slugs
                    .map(slug => String(slug || '').trim())
                    .filter(Boolean)
                    .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
                state.triggerTree = buildTriggerTree(state.triggers);
                rebuildTriggerSources(state.triggers);
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
                const cards = triggerEntries.map(entry => renderTriggerCard(entry.slug));
                if (cards.length) {
                    html += `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${cards.join('')}</div>`;
                }
            }

            if (!activeNode?.children?.size && !(activeNode?.triggers && activeNode.triggers.length)) {
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
        if (DOM['triggers-new-btn']) {
            DOM['triggers-new-btn'].classList.toggle('hidden', show);
        }
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
        const labelAttr = escapeAttribute(label);
        const labelDisplay = escapeHtml(label);

        return `
            <article class="pipeline-folder-card" data-trigger-folder="${keyAttr}" tabindex="0" role="button" aria-label="Open folder ${labelAttr}">
                <div class="pipeline-folder-card-header">
                    <span class="pipeline-folder-icon">
                        <svg class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7h5l2 2h9a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V9a2 2 0 012-2z"/></svg>
                    </span>
                    <h3 class="pipeline-folder-title" title="${labelAttr}">${labelDisplay}</h3>
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
        const parts = slug.split('/').filter(Boolean);
        const name = parts.pop() || slug;
        const owner = parts.join('/') || 'root';
        const isActive = slug === state.selectedSlug;

        return `
            <article class="pipeline-card triggers-card bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded-lg shadow-sm transition-all duration-200 p-3 flex flex-col${isActive ? ' triggers-card--active' : ''}" data-trigger-slug="${escapeAttribute(slug)}" tabindex="0" role="button" aria-label="Open trigger ${escapeAttribute(slug)}">
                <div class="pipeline-card-header flex items-start justify-between gap-3">
                    <div class="pipeline-card-text min-w-0">
                        <h3 class="pipeline-card-title" title="${escapeAttribute(name)}">${escapeHtml(name)}</h3>
                        <p class="pipeline-card-path" title="${escapeAttribute(owner)}">${escapeHtml(owner)}</p>
                    </div>
                    <div class="pipeline-card-actions">
                        <button class="pipelines-delete-button" type="button" data-trigger-delete="${escapeAttribute(slug)}" title="Delete trigger">
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
                        <span class="pipeline-card-meta-label">Source</span>
                        <span class="pipeline-card-meta-value" title="${escapeAttribute(sourceLabel)}">${escapeHtml(sourceLabel)}</span>
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
        }

        updateTriggerActionButtons(slug, sourceLabel);
        renderMetaChips(info.summary);
        renderTriggerMeta(info.summary, slug);
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

    function renderTriggerMeta(summary, slug) {
        if (!DOM['triggers-detail-meta']) return;
        const repo = slug || 'N/A';
        const triggerCount = summary?.triggerCount ?? 0;
        const pipelineCount = summary?.pipelineCount ?? 0;
        const envCount = Array.isArray(summary?.environments) ? summary.environments.length : 0;
        const eventsLabel = summary?.events && summary.events.length
            ? summary.events.slice(0, 3).join(', ') + (summary.events.length > 3 ? '…' : '')
            : 'All events';

        const items = [
            { label: 'Repository', value: repo },
            { label: 'Rules', value: triggerCount },
            { label: 'Pipelines', value: pipelineCount },
            { label: 'Environments', value: envCount },
            { label: 'Events', value: eventsLabel },
        ];

        DOM['triggers-detail-meta'].innerHTML = items.map(({ label, value }) => `
            <span class="triggers-detail-meta-item">
                <span class="triggers-detail-meta-label">${escapeHtml(label)}:</span>
                <span class="triggers-detail-meta-value">${escapeHtml(String(value))}</span>
            </span>
        `).join('');
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
            const lines = (yaml || '').split('\n');
            target.innerHTML = lines.map((line, index) => `
                <div class="yaml-line">
                    <span class="yaml-line-number">${index + 1}</span>
                    <span class="yaml-line-text">${escapeHtml(line)}</span>
                </div>
            `).join('');
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
            const hash = buildPipelineHash(item.identifier);
            const versionLabel = `Version: ${escapeHtml(item.meta?.version || 'latest')}`;
            const sourceLabel = `Source: ${escapeHtml(item.meta?.source || getTriggerSourceLabel('database'))}`;
            return `
                <li class="triggers-pipeline-item">
                    <a href="${hash}" class="triggers-pipeline-link" title="Open ${escapeAttribute(item.display)}">
                        <span class="triggers-pipeline-name">${escapeHtml(item.display)}</span>
                        <span class="triggers-pipeline-path">${escapeHtml(item.pathLabel)}</span>
                        <span class="triggers-pipeline-meta">
                            <span class="triggers-pipeline-version">${versionLabel}</span>
                            <span class="triggers-pipeline-source">${sourceLabel}</span>
                        </span>
                    </a>
                </li>
            `;
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
                    <div class="triggers-run-row__line">
                        <span class="triggers-run-row__branch" title="Branch">${escapeHtml(branch)}</span>
                        <span class="triggers-run-row__status">${escapeHtml(statusLabel)}</span>
                    </div>
                    <div class="triggers-run-row__meta">Run ID: <span>${escapeHtml(shortRunId)}</span></div>
                    <div class="triggers-run-row__meta">Trigger ID: <span>${escapeHtml(shortTriggerId)}</span></div>
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

        if (DOM['triggers-yaml-content']) {
            DOM['triggers-yaml-content'].classList.add('hidden');
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
            validateCurrentYaml();
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
        if (DOM['triggers-editor-container']) {
            DOM['triggers-editor-container'].classList.add('hidden');
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
            DOM['triggers-validation-status'].classList.add('hidden');
            DOM['triggers-validation-status'].textContent = '';
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
        validateCurrentYaml();
    }

    function syncEditorScroll() {
        if (!DOM['triggers-line-numbers'] || !DOM['triggers-yaml-editor']) return;
        DOM['triggers-line-numbers'].scrollTop = DOM['triggers-yaml-editor'].scrollTop;
    }

    function updateLineNumbers(text) {
        if (!DOM['triggers-line-numbers']) return;
        const lines = text.split('\n').length;
        const content = Array.from({ length: lines }, (_, i) => i + 1).join('\n');
        DOM['triggers-line-numbers'].textContent = content;
    }

    function validateCurrentYaml() {
        if (!DOM['triggers-yaml-editor'] || !DOM['triggers-validation-status']) return;
        const value = DOM['triggers-yaml-editor'].value || '';
        try {
            const parsed = parseTriggerYaml(value);
            const summary = buildTriggerSummary(parsed);
            const message = `${summary.triggerCount} trigger${summary.triggerCount === 1 ? '' : 's'} · ${summary.pipelineCount} pipeline${summary.pipelineCount === 1 ? '' : 's'}`;
            DOM['triggers-validation-status'].textContent = `Looks good: ${message}`;
            DOM['triggers-validation-status'].classList.remove('hidden');
            DOM['triggers-validation-status'].classList.remove('text-red-500');
            DOM['triggers-validation-status'].classList.add('text-[var(--text-secondary)]');
            return true;
        } catch (error) {
            DOM['triggers-validation-status'].textContent = `Validation failed: ${error.message}`;
            DOM['triggers-validation-status'].classList.remove('hidden');
            DOM['triggers-validation-status'].classList.add('text-red-500');
            return false;
        }
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
        state.pendingDeleteSlug = target;
        if (DOM['triggers-delete-message']) {
            DOM['triggers-delete-message'].innerHTML = `Are you sure you want to delete the trigger <strong>${escapeHtml(target)}</strong>?`;
        }
        openModal('triggers-delete-modal');
    }

    async function handleDeleteConfirm() {
        if (!state.pendingDeleteSlug) return;
        const slug = state.pendingDeleteSlug;
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
        const yamlInput = DOM['triggers-new-yaml'];
        if (!repoInput || !yamlInput) return;

        const repo = (repoInput.value || '').trim();
        const yaml = (yamlInput.value || '').trim();
        if (!repo || !yaml) {
            showToast('Repository and YAML are required.', 'error');
            return;
        }

        try {
            parseTriggerYaml(yaml);
        } catch (error) {
            showToast(`Invalid YAML: ${error.message}`, 'error');
            return;
        }

        try {
            const [owner, name] = splitSlug(repo);
            await context.fetchData(`/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/x-yaml' },
                body: yaml,
            });

            closeModal('triggers-new-modal');
            repoInput.value = '';
            yamlInput.value = '';
            showToast(`Created trigger ${repo}.`, 'success');
            await loadTriggers(true);
            navigateToSlug(repo);
        } catch (error) {
            console.error('Failed to create trigger:', error);
            showToast('Failed to create trigger.', 'error');
        }
    }

    function openModal(id) {
        const el = DOM[id];
        if (!el) return;
        el.classList.remove('hidden');
        requestAnimationFrame(() => el.classList.add('opacity-100'));
    }

    function closeModal(id) {
        const el = DOM[id];
        if (!el) return;
        el.classList.remove('opacity-100');
        setTimeout(() => el.classList.add('hidden'), 200);
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
        const tags = new Set();
        const environments = new Set();

        triggers.forEach(trigger => {
            if (trigger?.on) {
                events.add(String(trigger.on));
            }
            (trigger?.branches || []).forEach(branch => branches.add(String(branch)));
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
        populateSidebarContainer(container);
    }

    function updateSidebarHighlight() {
        const container = document.getElementById('triggers-sidebar-list');
        if (!container) return;
        container.querySelectorAll('[data-trigger-sidebar-slug]').forEach(link => {
            const slug = link.getAttribute('data-trigger-sidebar-slug');
            link.classList.toggle('triggers-sidebar-link--active', slug === state.selectedSlug);
        });
    }

    function populateSidebarContainer(container) {
        if (!container) return;
        if (!state.triggers.length) {
            container.innerHTML = '<p class="triggers-sidebar-empty">No triggers found</p>';
            return;
        }

        const html = state.triggers.map(slug => {
            const isActive = slug === state.selectedSlug;
            const hash = buildTriggerHash(slug);
            return `
                <a href="${hash}" class="triggers-sidebar-link ${isActive ? 'triggers-sidebar-link--active' : ''}" data-trigger-sidebar-slug="${escapeAttribute(slug)}">
                    ${escapeHtml(slug)}
                </a>
            `;
        }).join('');

        container.innerHTML = html;
        updateSidebarHighlight();
    }

    function refreshSidebarListFromState() {
        const container = document.getElementById('triggers-sidebar-list');
        if (container) {
            populateSidebarContainer(container);
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
            const container = document.getElementById('triggers-sidebar-list');
            await renderSidebarList(container);
        },
    };
})(window.NopsAI = window.NopsAI || {});
