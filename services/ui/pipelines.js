(function (global) {
    const state = {
        pipelines: [],
        searchTerm: '',
        selectedId: null,
        drafts: new Set(),
        pipelineCache: new Map(),
        triggersIndex: new Map(),
        runsCache: { fetchedAt: 0, runs: [] },
        activeFolderKey: '',
        pendingDelete: null,
        isEditing: false,
        currentYaml: '',
    };

    const DOM = {};
    let context = null;
    let pipelineRunsModule = null;

    const TOAST_TIMEOUT = 4000;

    function init(ctx) {
        context = ctx;
        pipelineRunsModule = (global.pages && global.pages.pipelineruns) ? global.pages.pipelineruns : null;

        const ids = [
            'pipelines-search', 'pipelines-list-view', 'pipelines-detail-view', 'pipelines-list-container',
            'pipelines-empty', 'pipelines-total-count', 'pipelines-filter-count', 'pipelines-refresh-btn',
            'pipelines-new-btn', 'pipelines-back-btn', 'pipelines-subtitle', 'pipeline-detail-name',
            'pipeline-detail-description', 'pipeline-detail-path', 'pipeline-detail-version', 'pipeline-copy-btn',
            'pipeline-download-btn', 'pipeline-edit-btn', 'pipeline-save-btn', 'pipeline-cancel-btn',
            'pipeline-yaml-content', 'pipeline-yaml-editor', 'editor-container', 'line-numbers',
            'validation-status', 'yaml-view-actions', 'yaml-edit-actions', 'pipeline-graph',
            'pipeline-triggers', 'pipeline-recent-runs', 'pipelines-new-modal', 'pipelines-new-form',
            'pipelines-new-close', 'pipelines-new-cancel', 'pipelines-new-path', 'pipelines-new-name',
            'pipelines-delete-modal', 'pipelines-delete-message', 'pipelines-delete-confirm',
            'pipelines-delete-cancel', 'pipelines-delete-close'
        ];

        ids.forEach(id => {
            const el = document.getElementById(id);
            if (el) {
                DOM[id] = el;
            }
        });

        if (DOM['pipelines-search']) {
            DOM['pipelines-search'].addEventListener('input', handleSearch);
        }

        if (DOM['pipelines-refresh-btn']) {
            DOM['pipelines-refresh-btn'].addEventListener('click', () => refreshPipelines(true));
        }

        if (DOM['pipelines-new-btn']) {
            DOM['pipelines-new-btn'].addEventListener('click', openNewPipelineModal);
        }

        if (DOM['pipelines-back-btn']) {
            DOM['pipelines-back-btn'].addEventListener('click', () => {
                showListView();
                try {
                    history.replaceState(null, '', '#/pipelines');
                } catch {
                    window.location.hash = '#/pipelines';
                }
            });
        }

        if (DOM['pipelines-list-container']) {
            DOM['pipelines-list-container'].addEventListener('click', handleListClick);
            DOM['pipelines-list-container'].addEventListener('keydown', handleListKeydown);
        }

        if (DOM['pipelines-new-close']) {
            DOM['pipelines-new-close'].addEventListener('click', closeNewPipelineModal);
        }
        if (DOM['pipelines-new-cancel']) {
            DOM['pipelines-new-cancel'].addEventListener('click', closeNewPipelineModal);
        }
        if (DOM['pipelines-new-form']) {
            DOM['pipelines-new-form'].addEventListener('submit', handleCreatePipeline);
        }

        if (DOM['pipelines-delete-close']) {
            DOM['pipelines-delete-close'].addEventListener('click', closeDeleteModal);
        }
        if (DOM['pipelines-delete-cancel']) {
            DOM['pipelines-delete-cancel'].addEventListener('click', closeDeleteModal);
        }
        if (DOM['pipelines-delete-confirm']) {
            DOM['pipelines-delete-confirm'].addEventListener('click', confirmDeletePipeline);
        }

        if (DOM['pipeline-edit-btn']) {
            DOM['pipeline-edit-btn'].addEventListener('click', enterEditMode);
        }
        if (DOM['pipeline-cancel-btn']) {
            DOM['pipeline-cancel-btn'].addEventListener('click', exitEditMode);
        }
        if (DOM['pipeline-save-btn']) {
            DOM['pipeline-save-btn'].addEventListener('click', savePipelineChanges);
        }
        if (DOM['pipeline-copy-btn']) {
            DOM['pipeline-copy-btn'].addEventListener('click', copyPipelineYaml);
        }
        if (DOM['pipeline-download-btn']) {
            DOM['pipeline-download-btn'].addEventListener('click', downloadPipelineYaml);
        }

        if (DOM['pipeline-yaml-editor']) {
            DOM['pipeline-yaml-editor'].addEventListener('input', () => {
                handleValidation();
                updateLineNumbers();
            });
            DOM['pipeline-yaml-editor'].addEventListener('scroll', () => {
                if (DOM['line-numbers']) {
                    DOM['line-numbers'].scrollTop = DOM['pipeline-yaml-editor'].scrollTop;
                }
            });
            DOM['pipeline-yaml-editor'].addEventListener('keydown', (event) => {
                if (event.key === 'Tab') {
                    event.preventDefault();
                    const start = event.target.selectionStart;
                    const end = event.target.selectionEnd;
                    event.target.value = event.target.value.substring(0, start) + '  ' + event.target.value.substring(end);
                    event.target.selectionStart = event.target.selectionEnd = start + 2;
                    handleValidation();
                    updateLineNumbers();
                }
            });
        }

        const themeToggle = document.getElementById('theme-toggle');
        if (themeToggle) {
            themeToggle.addEventListener('click', () => {
                setTimeout(() => {
                    configureMermaid();
                    if (state.selectedId) {
                        renderPipelineGraphFromYaml(state.pipelineCache.get(state.selectedId)?.yaml || '');
                    }
                }, 50);
            });
        }

        configureMermaid();
    }

    function configureMermaid() {
        if (!window.mermaid) return;
        const theme = document.documentElement.classList.contains('dark') ? 'dark' : 'default';
        window.mermaid.initialize({
            startOnLoad: false,
            theme,
            securityLevel: 'loose',
            fontFamily: 'Inter, sans-serif'
        });
    }

    function parsePipelineRoute(hash) {
        const clean = (hash || window.location.hash || '').replace(/^#/, '').replace(/^\//, '');
        if (!clean) {
            return { pipelineId: null };
        }
        const parts = clean.split('/');
        if (parts[0] !== 'pipelines') {
            return { pipelineId: null };
        }
        const isEdit = parts[parts.length - 1] === 'edit';
        const rawSegments = parts.slice(1, isEdit ? -1 : undefined).map(decodeURIComponent).filter(Boolean);
        return { pipelineId: rawSegments.join('/'), autoEdit: isEdit };
    }

    async function handleRoute(hash) {
        if (!context) return;
        const { DOM: globalDOM } = context;
        if (globalDOM.mainHeader) {
            globalDOM.mainHeader.textContent = 'Pipelines';
        }

        if (pipelineRunsModule && typeof pipelineRunsModule.renderSidebarForRoute === 'function') {
            await pipelineRunsModule.renderSidebarForRoute('pipelines');
        }

        await refreshPipelines();

        const { pipelineId, autoEdit } = parsePipelineRoute(hash);
        if (pipelineId) {
            await selectPipeline(pipelineId, { autoEdit });
        } else {
            showListView();
        }
    }

    async function refreshPipelines(force = false) {
        if (!state.pipelines.length || force) {
            const response = await context.fetchData('/v1/pipelines');
            state.pipelines = Array.isArray(response) ? response.slice() : [];
            state.pipelines.sort((a, b) => a.localeCompare(b));
            await preloadSummaries(state.pipelines);
        }

        renderPipelineList();
        updateCounts();
    }

    async function preloadSummaries(pipelineIds) {
        for (const identifier of pipelineIds) {
            await ensurePipelineSummary(identifier);
        }
    }

    async function ensurePipelineSummary(pipelineId) {
        const cacheEntry = state.pipelineCache.get(pipelineId);
        if (cacheEntry && cacheEntry.meta && cacheEntry.yaml) {
            return cacheEntry;
        }

        if (state.drafts.has(pipelineId) && cacheEntry) {
            return cacheEntry;
        }

        const yaml = await fetchPipelineYaml(pipelineId);
        if (yaml == null) {
            return null;
        }

        const meta = extractMetaFromYaml(yaml, pipelineId);
        const updatedEntry = {
            ...(cacheEntry || {}),
            yaml,
            meta,
            fetchedAt: Date.now(),
            isDraft: cacheEntry?.isDraft || false,
        };
        state.pipelineCache.set(pipelineId, updatedEntry);
        return updatedEntry;
    }

    async function fetchPipelineYaml(pipelineId) {
        if (state.drafts.has(pipelineId)) {
            return state.pipelineCache.get(pipelineId)?.yaml || null;
        }

        const url = `/v1/pipelines/${pipelineId.split('/').map(encodeURIComponent).join('/')}`;
        const result = await context.fetchData(url);
        if (typeof result !== 'string') {
            return null;
        }
        return result;
    }

function extractMetaFromYaml(yaml, pipelineId) {
        const fallback = parsePipelineIdentifier(pipelineId);
        const parsed = parseYamlSafely(yaml);
        if (!parsed || typeof parsed !== 'object') {
            return {
                name: fallback.name,
                description: '',
                version: 'latest',
                path: fallback.path,
                source: 'Config Repository',
            };
        }

        return {
            name: parsed.name || fallback.name,
            description: parsed.description || '',
            version: parsed.version || 'latest',
            path: fallback.path,
            source: parsed.source || 'Config Repository',
        };
    }

function parsePipelineIdentifier(identifier) {
    const trimmed = identifier.trim().replace(/^\/+|\/+$/g, '');
    if (!trimmed) {
        return { path: '', name: '' };
    }
    const parts = trimmed.split('/');
    const name = parts.pop();
    const path = parts.join('/');
    return { path, name };
}

function parseYamlSafely(text) {
    if (!text || !window.jsyaml) return null;
    try {
        return window.jsyaml.load(text);
    } catch {
        return null;
    }
}

function formatPathLabel(path) {
    return path ? path : 'root';
}

    function setActiveView(view) {
        const showDetail = view === 'detail';
        if (DOM['pipelines-list-view']) {
            DOM['pipelines-list-view'].classList.toggle('hidden', showDetail);
        }
        if (DOM['pipelines-detail-view']) {
            DOM['pipelines-detail-view'].classList.toggle('hidden', !showDetail);
        }
        if (DOM['pipelines-back-btn']) {
            DOM['pipelines-back-btn'].classList.toggle('hidden', !showDetail);
        }
        if (DOM['pipelines-subtitle']) {
            DOM['pipelines-subtitle'].textContent = showDetail
                ? 'Viewing pipeline definition and runtime insights.'
                : 'Browse pipelines and inspect their definitions.';
        }
    }

    function showListView() {
        state.selectedId = null;
        state.isEditing = false;
        setActiveView('list');
    }

    async function selectPipeline(pipelineId, options = {}) {
        const entry = await ensurePipelineSummary(pipelineId);
        if (!entry) {
            showToast(`Unable to load pipeline '${pipelineId}'.`, 'error');
            showListView();
            return;
        }

        state.selectedId = pipelineId;
        state.currentYaml = entry?.yaml || '';
        state.isEditing = false;

        renderPipelineDetail(entry);
        setActiveView('detail');

        if (options.autoEdit) {
            enterEditMode();
        }
    }

    function renderPipelineList() {
        if (!DOM['pipelines-list-container']) return;
        const tree = buildGroupedPipelines();
        const hasAnyContent = (tree.children && tree.children.size > 0) || (tree.pipelines && tree.pipelines.length > 0);

        if (!hasAnyContent) {
            state.activeFolderKey = '';
            if (DOM['pipelines-empty']) {
                DOM['pipelines-empty'].classList.remove('hidden');
            }
            DOM['pipelines-list-container'].innerHTML = '';
            return;
        }

        if (DOM['pipelines-empty']) {
            DOM['pipelines-empty'].classList.add('hidden');
        }

        const activeNode = resolveActiveFolderNode(tree);
        const breadcrumbHtml = renderFolderBreadcrumb();

        const childNodes = activeNode.children
            ? Array.from(activeNode.children.values()).sort((a, b) => a.label.localeCompare(b.label, undefined, { sensitivity: 'base' }))
            : [];
        const folderCards = childNodes.map(renderFolderCard);

        const pipelineCards = (activeNode.pipelines || [])
            .slice()
            .sort((a, b) => {
                const nameA = (a.meta?.name || a.id || '').toLowerCase();
                const nameB = (b.meta?.name || b.id || '').toLowerCase();
                return nameA.localeCompare(nameB);
            })
            .map(renderPipelineCard);

        const cards = folderCards.concat(pipelineCards);

        const gridHtml = cards.length
            ? `<div class="pipelines-card-grid">${cards.join('')}</div>`
            : `<div class="pipeline-folder-empty-state">No pipelines in this folder yet.</div>`;

        DOM['pipelines-list-container'].innerHTML = `${breadcrumbHtml}${gridHtml}`;
    }

    function focusFirstListItem() {
        if (!DOM['pipelines-list-container']) return;
        const next = DOM['pipelines-list-container'].querySelector('[data-folder-key], [data-pipeline-id]');
        if (next instanceof HTMLElement) {
            next.focus();
        }
    }

    function resolveActiveFolderNode(tree) {
        const node = getFolderNodeByKey(tree, state.activeFolderKey);
        state.activeFolderKey = node?.key || '';
        return node;
    }

    function getFolderNodeByKey(tree, key) {
        if (!key) return tree;
        const segments = key.split('/').filter(Boolean);
        let node = tree;
        for (const segment of segments) {
            if (!node.children || !node.children.has(segment)) {
                return tree;
            }
            node = node.children.get(segment);
        }
        return node;
    }

    function renderFolderBreadcrumb() {
        const key = state.activeFolderKey || '';
        const segments = key.split('/').filter(Boolean);
        if (segments.length === 0) {
            return `<nav class="pipeline-folder-breadcrumb" aria-label="Pipeline folders"><span class="pipeline-folder-crumb pipeline-folder-crumb--current">All Pipelines</span></nav>`;
        }

        let cumulative = '';
        const parts = [
            `<button type="button" class="pipeline-folder-crumb" data-folder-nav="">All Pipelines</button>`
        ];

        segments.forEach((segment, index) => {
            cumulative = cumulative ? `${cumulative}/${segment}` : segment;
            const label = formatPathLabel(segment);
            if (index === segments.length - 1) {
                parts.push(`<span class="pipeline-folder-crumb pipeline-folder-crumb--current">${escapeHtml(label)}</span>`);
            } else {
                parts.push(`<button type="button" class="pipeline-folder-crumb" data-folder-nav="${escapeHtml(cumulative)}">${escapeHtml(label)}</button>`);
            }
        });

        return `<nav class="pipeline-folder-breadcrumb" aria-label="Pipeline folders">${parts.join('<span class="pipeline-folder-crumb-separator">/</span>')}</nav>`;
    }

    function renderFolderCard(node) {
        const keyAttr = escapeHtml(node.key || '');
        const label = formatPathLabel(node.label || node.key || 'Folder');
        const totalPipelines = countPipelinesRecursive(node);
        const directPipelines = (node.pipelines || []).length;
        const childCount = node.children ? node.children.size : 0;
        const nestedPipelines = Math.max(totalPipelines - directPipelines, 0);
        const summaryParts = [];
        if (totalPipelines > 0) {
            summaryParts.push(`${totalPipelines} pipeline${totalPipelines === 1 ? '' : 's'}`);
        }
        if (childCount > 0) {
            summaryParts.push(`${childCount} folder${childCount === 1 ? '' : 's'}`);
        }
        const summary = summaryParts.join(' • ');

        return `
            <article class="pipeline-folder-card" data-folder-key="${keyAttr}" tabindex="0" role="button" aria-label="Open folder ${escapeHtml(label)}">
                <div class="pipeline-folder-card-header">
                    <span class="pipeline-folder-icon">
                        <svg class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <path d="M3 7h5l2 2h9a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V9a2 2 0 012-2z" />
                        </svg>
                    </span>
                    <div class="pipeline-folder-info">
                        <h3 class="pipeline-folder-title" title="${escapeHtml(label)}">${escapeHtml(label)}</h3>
                        ${summary ? `<p class="pipeline-folder-summary">${escapeHtml(summary)}</p>` : ''}
                    </div>
                    <span class="pipeline-folder-chevron" aria-hidden="true">
                        <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <path d="M9 5l7 7-7 7" />
                        </svg>
                    </span>
                </div>
                <div class="pipeline-folder-meta">
                    <div class="pipeline-folder-meta-row">
                        <span class="pipeline-folder-meta-label">Direct pipelines</span>
                        <span class="pipeline-folder-meta-value">${directPipelines}</span>
                    </div>
                    <div class="pipeline-folder-meta-row">
                        <span class="pipeline-folder-meta-label">Subfolders</span>
                        <span class="pipeline-folder-meta-value">${childCount}</span>
                    </div>
                    ${nestedPipelines ? `<div class="pipeline-folder-meta-row">
                        <span class="pipeline-folder-meta-label">Nested pipelines</span>
                        <span class="pipeline-folder-meta-value">${nestedPipelines}</span>
                    </div>` : ''}
                </div>
            </article>`;
    }

    function renderPipelineCard(pipeline) {
        const meta = pipeline.meta || {};
        const idAttr = escapeHtml(pipeline.id);
        const rawName = meta.name || 'Untitled pipeline';
        const rawPath = formatPathLabel(meta.path || 'root');
        const name = escapeHtml(rawName);
        const pathLabel = escapeHtml(rawPath);
        const description = escapeHtml(meta.description || 'No description provided.');
        const version = escapeHtml(meta.version || 'latest');
        const source = escapeHtml(meta.source || 'Config Repository');

        return `
            <article class="pipeline-card bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded-lg shadow-sm transition-all duration-200 p-4 flex flex-col" data-pipeline-id="${idAttr}" tabindex="0" role="button" aria-label="Open pipeline ${escapeHtml(rawName)}">
                <div class="pipeline-card-header flex items-start justify-between gap-3">
                    <div class="pipeline-card-info flex items-center gap-3 min-w-0">
                        <span class="pipeline-card-icon">
                            <svg class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M4 4h5l2 3h9a1 1 0 011 1v10a2 2 0 01-2 2H5a1 1 0 01-1-1V4z" />
                            </svg>
                        </span>
                        <div class="pipeline-card-text min-w-0">
                            <h3 class="pipeline-card-title" title="${name}">${name}</h3>
                            <p class="pipeline-card-path" title="${pathLabel}">${pathLabel}</p>
                        </div>
                    </div>
                    <button class="pipelines-delete-button" data-delete-pipeline="${idAttr}" title="Delete pipeline">
                        <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                            <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                            <path d="M4 7h16" />
                        </svg>
                    </button>
                </div>
                <p class="pipeline-card-description">${description}</p>
                <div class="pipeline-card-meta">
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">Version</span>
                        <span class="pipeline-card-meta-value" title="${version}">${version}</span>
                    </div>
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">Source</span>
                        <span class="pipeline-card-meta-value" title="${source}">${source}</span>
                    </div>
                </div>
            </article>`;
    }

    function countPipelinesRecursive(node) {
        const own = (node.pipelines || []).length;
        const childTotal = node.children
            ? Array.from(node.children.values()).reduce((sum, child) => sum + countPipelinesRecursive(child), 0)
            : 0;
        return own + childTotal;
    }

    function buildGroupedPipelines() {
        const search = state.searchTerm.trim().toLowerCase();
        const root = { label: 'root', key: '', pipelines: [], children: new Map() };

        state.pipelines.forEach(identifier => {
            let entry = state.pipelineCache.get(identifier);
            if (!entry) {
                entry = ensurePipelineSummary(identifier);
            }
            const meta = entry?.meta || extractMetaFromYaml(entry?.yaml || '', identifier);
            const identifierLower = identifier.toLowerCase();
            const metaText = `${meta.name || ''} ${meta.description || ''}`.toLowerCase();
            if (search && !(identifierLower.includes(search) || metaText.includes(search))) {
                return;
            }

            const pathSegments = (meta.path || '').split('/').filter(Boolean);
            let node = root;
            if (!pathSegments.length) {
                node.pipelines.push({ id: identifier, meta });
                return;
            }

            let cumulative = '';
            pathSegments.forEach((segment, idx) => {
                cumulative = cumulative ? `${cumulative}/${segment}` : segment;
                if (!node.children.has(segment)) {
                    node.children.set(segment, {
                        label: segment,
                        key: cumulative,
                        pipelines: [],
                        children: new Map(),
                    });
                }
                node = node.children.get(segment);
            });

            node.pipelines.push({ id: identifier, meta });
        });

        function sortNode(node) {
            node.pipelines.sort((a, b) => a.meta.name.localeCompare(b.meta.name));
            const sortedChildren = Array.from(node.children.entries())
                .sort(([a], [b]) => a.localeCompare(b));
            node.children = new Map(sortedChildren);
            node.children.forEach(child => sortNode(child));
            return node;
        }

        sortNode(root);
        return root;
    }

    function renderPipelineDetail(entry) {
        const meta = entry.meta || extractMetaFromYaml(entry.yaml, state.selectedId);

        if (DOM['pipeline-detail-name']) {
            DOM['pipeline-detail-name'].textContent = meta.name || state.selectedId;
        }
        if (DOM['pipeline-detail-description']) {
            DOM['pipeline-detail-description'].textContent = meta.description || 'No description provided.';
        }
        if (DOM['pipeline-detail-path']) {
            DOM['pipeline-detail-path'].textContent = formatPathLabel(meta.path);
        }
        if (DOM['pipeline-detail-version']) {
            DOM['pipeline-detail-version'].textContent = meta.version || 'latest';
        }
        if (DOM['pipeline-detail-source']) {
            DOM['pipeline-detail-source'].textContent = meta.source || 'Config Repository';
        }

        renderYamlView(entry.yaml);

        exitEditMode();
        renderPipelineGraphFromYaml(entry.yaml);
        renderTriggers(state.selectedId);
        renderRecentRuns(state.selectedId);
    }

    async function renderPipelineGraphFromYaml(yamlString) {
        if (!DOM['pipeline-graph']) return;
        if (!yamlString) {
            DOM['pipeline-graph'].innerHTML = '<p class="text-sm text-[var(--text-secondary)]">No definition available.</p>';
            return;
        }

        try {
            const graphDef = generateMermaidDefinition(yamlString);
            const { svg } = await window.mermaid.render(`pipeline-graph-${Date.now()}`, graphDef);
            DOM['pipeline-graph'].innerHTML = svg;
        } catch (error) {
            DOM['pipeline-graph'].innerHTML = '<p class="text-sm text-red-500">Unable to render dependency graph.</p>';
            console.error('Mermaid render error', error);
        }
    }

    async function renderTriggers(pipelineId) {
        if (!DOM['pipeline-triggers']) return;
        const triggers = await getTriggersForPipeline(pipelineId);
        if (!triggers.length) {
            DOM['pipeline-triggers'].innerHTML = '<p class="text-sm text-[var(--text-secondary)]">No trigger manifests reference this pipeline.</p>';
            return;
        }

        DOM['pipeline-triggers'].innerHTML = triggers.map(trigger => {
            const condition = trigger.branches?.length
                ? `branches: ${trigger.branches.join(', ')}`
                : trigger.skip_branches?.length
                    ? `skip_branches: ${trigger.skip_branches.join(', ')}`
                    : trigger.tags?.length
                        ? `tags: ${trigger.tags.join(', ')}`
                        : '';
            const environment = trigger.environment ? `<span class="pipelines-tag">${escapeHtml(trigger.environment)}</span>` : '';
            return `
                <div class="pipelines-trigger-card">
                    <div class="flex items-center justify-between gap-2">
                        <span class="font-mono text-sm text-[var(--text-primary)]">on: ${escapeHtml(trigger.on || 'event')}</span>
                        ${environment}
                    </div>
                    ${condition ? `<p class="text-xs text-[var(--text-secondary)] mt-2">${escapeHtml(condition)}</p>` : ''}
                </div>`;
        }).join('');
    }

    async function renderRecentRuns(pipelineId) {
        if (!DOM['pipeline-recent-runs']) return;
        const runs = await getRecentRunsForPipeline(pipelineId);
        if (!runs.length) {
            DOM['pipeline-recent-runs'].innerHTML = '<p class="text-sm text-[var(--text-secondary)]">No recent runs found.</p>';
            return;
        }

        DOM['pipeline-recent-runs'].innerHTML = runs.slice(0, 5).map(run => {
            const branch = run.git_ref ? run.git_ref.replace('refs/heads/', '') : 'manual';
            return `
                <div class="pipelines-run-row" title="Open run">
                    <div>
                        <p class="font-mono text-sm text-[var(--text-primary)]">${escapeHtml(run.git_commit_sha?.slice(0, 7) || '???')}</p>
                        <p class="text-xs text-[var(--text-secondary)]">${escapeHtml(branch)}</p>
                    </div>
                    <span class="text-xs text-[var(--text-secondary)]">${formatRelativeTime(run.started_at || run.startedAt)}</span>
                </div>`;
        }).join('');
    }

    function enterEditMode() {
        if (!state.selectedId) return;
        state.isEditing = true;
        if (DOM['yaml-view-actions']) DOM['yaml-view-actions'].classList.add('hidden');
        if (DOM['yaml-edit-actions']) DOM['yaml-edit-actions'].classList.remove('hidden');
        if (DOM['pipeline-yaml-content']) DOM['pipeline-yaml-content'].classList.add('hidden');
        if (DOM['editor-container']) DOM['editor-container'].classList.remove('hidden');
        if (DOM['validation-status']) DOM['validation-status'].classList.remove('hidden');
        if (DOM['pipeline-yaml-editor']) {
            const cacheEntry = state.pipelineCache.get(state.selectedId);
            const yamlContent = cacheEntry?.yaml ?? state.currentYaml ?? '';
            DOM['pipeline-yaml-editor'].value = yamlContent;
            DOM['pipeline-yaml-editor'].focus();
        }
        handleValidation();
        updateLineNumbers();
    }

    function exitEditMode() {
        state.isEditing = false;
        if (DOM['yaml-view-actions']) DOM['yaml-view-actions'].classList.remove('hidden');
        if (DOM['yaml-edit-actions']) DOM['yaml-edit-actions'].classList.add('hidden');
        if (DOM['pipeline-yaml-content']) DOM['pipeline-yaml-content'].classList.remove('hidden');
        if (DOM['editor-container']) DOM['editor-container'].classList.add('hidden');
        if (DOM['validation-status']) DOM['validation-status'].classList.add('hidden');
    }

    function updateLineNumbers() {
        if (!DOM['pipeline-yaml-editor'] || !DOM['line-numbers']) return;
        const lines = DOM['pipeline-yaml-editor'].value.split('\n');
        DOM['line-numbers'].textContent = lines.map((_, idx) => idx + 1).join('\n');
        DOM['line-numbers'].scrollTop = DOM['pipeline-yaml-editor'].scrollTop;
    }

    function updateLineNumbers() {
        if (!DOM['pipeline-yaml-editor'] || !DOM['line-numbers']) return;
        const lines = DOM['pipeline-yaml-editor'].value.split('\n');
        DOM['line-numbers'].textContent = lines.map((_, idx) => idx + 1).join('\n');
        DOM['line-numbers'].scrollTop = DOM['pipeline-yaml-editor'].scrollTop;
    }

    function renderYamlView(yamlString) {
        const target = DOM['pipeline-yaml-content'];
        if (!target) return;
        const lines = (yamlString || '').split('\n');
        target.innerHTML = lines.map((line, idx) => `
            <div class="yaml-line">
                <span class="yaml-line-number">${idx + 1}</span>
                <span class="yaml-line-text">${escapeHtml(line)}</span>
            </div>`).join('');
    }

    function handleValidation() {
        if (!DOM['pipeline-yaml-editor'] || !DOM['validation-status']) return;
        const yamlString = DOM['pipeline-yaml-editor'].value;
        const result = validatePipelineYaml(yamlString);
        if (result && result.error) {
            DOM['validation-status'].textContent = result.error;
            DOM['validation-status'].className = 'mt-2 text-xs text-red-500';
            if (DOM['pipeline-save-btn']) DOM['pipeline-save-btn'].disabled = true;
        } else {
            DOM['validation-status'].textContent = '✅ Pipeline definition is valid.';
            DOM['validation-status'].className = 'mt-2 text-xs text-green-500';
            if (DOM['pipeline-save-btn']) DOM['pipeline-save-btn'].disabled = false;
        }
    }

    async function savePipelineChanges() {
        if (!state.selectedId || !DOM['pipeline-yaml-editor']) return;
        const yamlString = DOM['pipeline-yaml-editor'].value;
        const validation = validatePipelineYaml(yamlString);
        if (validation && validation.error) {
            showToast('Resolve validation errors before saving.', 'error');
            return;
        }

        const url = `/v1/pipelines/${state.selectedId.split('/').map(encodeURIComponent).join('/')}`;
        const response = await context.fetchData(url, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/x-yaml' },
            body: yamlString,
        });

        if (response === null) {
            showToast('Failed to save pipeline.', 'error');
            return;
        }

        const entry = await ensurePipelineSummary(state.selectedId);
        if (entry) {
            entry.yaml = yamlString;
            entry.meta = extractMetaFromYaml(yamlString, state.selectedId);
            state.pipelineCache.set(state.selectedId, entry);
        }

        state.currentYaml = yamlString;
        exitEditMode();
        renderPipelineDetail(entry);
        renderPipelineList();
        showToast('Pipeline saved successfully.', 'success');
    }

    function copyPipelineYaml() {
        const entry = state.selectedId ? state.pipelineCache.get(state.selectedId) : null;
        if (!entry) return;
        navigator.clipboard.writeText(entry.yaml || '').then(() => {
            showToast('Pipeline YAML copied to clipboard.', 'success');
        }).catch(() => {
            showToast('Copy failed.', 'error');
        });
    }

    function downloadPipelineYaml() {
        const entry = state.selectedId ? state.pipelineCache.get(state.selectedId) : null;
        if (!entry) return;
        const blob = new Blob([entry.yaml || ''], { type: 'text/yaml' });
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = `${state.selectedId.replace(/\//g, '_')}.yaml`;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        URL.revokeObjectURL(url);
    }

    function handleSearch(event) {
        state.searchTerm = event.target.value || '';
        state.activeFolderKey = '';
        renderPipelineList();
        updateCounts();
    }

    function handleListClick(event) {
        const deleteButton = event.target.closest('[data-delete-pipeline]');
        if (deleteButton) {
            const pipelineId = deleteButton.getAttribute('data-delete-pipeline');
            if (pipelineId) {
                openDeleteModal(pipelineId);
            }
            event.preventDefault();
            event.stopPropagation();
            return;
        }

        const folderNav = event.target.closest('[data-folder-nav]');
        if (folderNav) {
            const folderKey = folderNav.getAttribute('data-folder-nav') || '';
            state.activeFolderKey = folderKey;
            renderPipelineList();
            return;
        }

        const folderCard = event.target.closest('[data-folder-key]');
        if (folderCard) {
            const folderKey = folderCard.getAttribute('data-folder-key') || '';
            state.activeFolderKey = folderKey;
            renderPipelineList();
            return;
        }

        const row = event.target.closest('[data-pipeline-id]');
        if (row) {
            const pipelineId = row.getAttribute('data-pipeline-id');
            if (pipelineId) {
                window.location.hash = `#/pipelines/${pipelineId}`;
            }
        }
    }

    function handleListKeydown(event) {
        if (event.defaultPrevented) return;
        if (event.key !== 'Enter' && event.key !== ' ' && event.key !== 'Spacebar') return;

        const folderNav = event.target.closest('[data-folder-nav]');
        if (folderNav && folderNav === document.activeElement) {
            event.preventDefault();
            const folderKey = folderNav.getAttribute('data-folder-nav') || '';
            state.activeFolderKey = folderKey;
            renderPipelineList();
            focusFirstListItem();
            return;
        }

        const folderCard = event.target.closest('[data-folder-key]');
        if (folderCard && folderCard === document.activeElement) {
            event.preventDefault();
            const folderKey = folderCard.getAttribute('data-folder-key') || '';
            state.activeFolderKey = folderKey;
            renderPipelineList();
            focusFirstListItem();
            return;
        }

        const pipelineCard = event.target.closest('[data-pipeline-id]');
        if (pipelineCard && pipelineCard === document.activeElement) {
            event.preventDefault();
            const pipelineId = pipelineCard.getAttribute('data-pipeline-id');
            if (pipelineId) {
                window.location.hash = `#/pipelines/${pipelineId}`;
            }
        }
    }

    function updateCounts() {
        if (DOM['pipelines-total-count']) {
            DOM['pipelines-total-count'].textContent = `${state.pipelines.length} pipeline${state.pipelines.length === 1 ? '' : 's'}`;
        }
        if (DOM['pipelines-filter-count']) {
            if (!state.searchTerm.trim()) {
                DOM['pipelines-filter-count'].classList.add('hidden');
            } else {
                const tree = buildGroupedPipelines();
                const total = countPipelinesRecursive(tree);
                DOM['pipelines-filter-count'].textContent = `• ${total} result${total === 1 ? '' : 's'}`;
                DOM['pipelines-filter-count'].classList.remove('hidden');
            }
        }
    }

    function openNewPipelineModal() {
        if (!DOM['pipelines-new-modal']) return;
        if (DOM['pipelines-new-path']) {
            DOM['pipelines-new-path'].value = state.activeFolderKey || '';
        }
        DOM['pipelines-new-name'].value = '';
        DOM['pipelines-new-modal'].classList.remove('hidden');
        requestAnimationFrame(() => DOM['pipelines-new-modal'].classList.add('show'));
    }

    function closeNewPipelineModal() {
        if (!DOM['pipelines-new-modal']) return;
        DOM['pipelines-new-modal'].classList.remove('show');
        setTimeout(() => DOM['pipelines-new-modal'].classList.add('hidden'), 200);
    }

    async function handleCreatePipeline(event) {
        event.preventDefault();
        const pathRaw = DOM['pipelines-new-path'].value.trim().replace(/^\/+|\/+$/g, '');
        const name = DOM['pipelines-new-name'].value.trim();
        if (!name) {
            showToast('Pipeline name is required.', 'error');
            return;
        }
        if (!/^[a-zA-Z0-9_.-]+$/.test(name)) {
            showToast('Pipeline name can only contain letters, numbers, dots, underscores, and hyphens.', 'error');
            return;
        }

        const identifier = pathRaw ? `${pathRaw}/${name}` : name;
        if (state.pipelines.includes(identifier)) {
            showToast('A pipeline with that identifier already exists.', 'error');
            return;
        }

        const yaml = buildDefaultPipelineYaml(name);
        state.drafts.add(identifier);
        state.pipelineCache.set(identifier, {
            yaml,
            meta: {
                name,
                description: 'Describe what this pipeline does.',
                version: 'latest',
                path: pathRaw,
                source: 'Draft',
            },
            isDraft: true,
            fetchedAt: Date.now(),
        });
        state.pipelines.push(identifier);
        state.pipelines.sort((a, b) => a.localeCompare(b));

        closeNewPipelineModal();
        renderPipelineList();
        updateCounts();
        window.location.hash = `#/pipelines/${identifier}/edit`;
        showToast('Draft pipeline created. Fill in the YAML and save when ready.', 'info');
    }

    function buildDefaultPipelineYaml(name) {
        return `name: ${name}\nversion: latest\ndescription: Describe what this pipeline does.\ncontainer_image: alpine:3.19\nsteps:\n  - name: build\n    goal: |\n      Replace this with build instructions for ${name}.\n`;
    }

    function openDeleteModal(pipelineId) {
        state.pendingDelete = pipelineId;
        if (DOM['pipelines-delete-message']) {
            DOM['pipelines-delete-message'].textContent = `Delete pipeline '${pipelineId}'? This action cannot be undone.`;
        }
        if (DOM['pipelines-delete-modal']) {
            DOM['pipelines-delete-modal'].classList.remove('hidden');
            requestAnimationFrame(() => DOM['pipelines-delete-modal'].classList.add('show'));
        }
    }

    function closeDeleteModal() {
        state.pendingDelete = null;
        if (!DOM['pipelines-delete-modal']) return;
        DOM['pipelines-delete-modal'].classList.remove('show');
        setTimeout(() => DOM['pipelines-delete-modal'].classList.add('hidden'), 200);
    }

    async function confirmDeletePipeline() {
        const pipelineId = state.pendingDelete;
        if (!pipelineId) return;

        const isDraft = state.drafts.has(pipelineId);
        if (!isDraft) {
            const url = `/v1/pipelines/${pipelineId.split('/').map(encodeURIComponent).join('/')}`;
            const response = await context.fetchData(url, { method: 'DELETE' });
            if (response === null) {
                showToast('Failed to delete pipeline.', 'error');
                return;
            }
        }

        state.drafts.delete(pipelineId);
        state.pipelineCache.delete(pipelineId);
        state.pipelines = state.pipelines.filter(id => id !== pipelineId);
        if (state.selectedId === pipelineId) {
            showListView();
            try {
                history.replaceState(null, '', '#/pipelines');
            } catch {
                window.location.hash = '#/pipelines';
            }
        }
        closeDeleteModal();
        renderPipelineList();
        updateCounts();
        showToast('Pipeline deleted.', 'success');
    }

    function validatePipelineYaml(yamlString) {
        if (!window.jsyaml) return { error: 'YAML parser is unavailable.' };
        try {
            const pipeline = window.jsyaml.load(yamlString);
            if (!pipeline) return { error: 'YAML is empty or invalid.' };
            if (!pipeline.name) return { error: "Validation Error: 'name' is a required field." };
            const allowed = /^[a-zA-Z0-9_.-]+$/;
            if (!allowed.test(pipeline.name)) {
                return { error: `Validation Error: Pipeline name '${pipeline.name}' contains invalid characters.` };
            }
            if (pipeline.version && !allowed.test(pipeline.version)) {
                return { error: `Validation Error: Pipeline version '${pipeline.version}' contains invalid characters.` };
            }
            if (!Array.isArray(pipeline.steps) || pipeline.steps.length === 0) {
                return { error: "Validation Error: At least one step is required in 'steps'." };
            }
            const stepNames = new Set();
            const stepTaskMaps = new Map();
            for (const step of pipeline.steps) {
                if (!step || typeof step !== 'object') {
                    return { error: 'Validation Error: A step is not a valid object.' };
                }
                if (!step.name) return { error: 'Validation Error: All steps require a name.' };
                if (stepNames.has(step.name)) {
                    return { error: `Validation Error: Duplicate step name '${step.name}' found.` };
                }
                stepNames.add(step.name);
                const isInclude = !!step.include;
                const hasTasks = Array.isArray(step.tasks) && step.tasks.length > 0;
                const hasLegacy = !!(step.goal || step.script);
                if (!isInclude && !hasTasks && !hasLegacy) {
                    return { error: `Validation Error: Step '${step.name}' must contain 'include', 'tasks', 'goal', or 'script'.` };
                }
                if (isInclude && (hasTasks || hasLegacy)) {
                    return { error: `Validation Error: Step '${step.name}' is an include step and cannot also contain tasks, goal, or script.` };
                }
                if (hasTasks && hasLegacy) {
                    return { error: `Validation Error: Step '${step.name}' mixes tasks with goal/script.` };
                }
                if (hasTasks) {
                    const taskNames = new Set();
                    for (const task of step.tasks) {
                        if (!task || typeof task !== 'object' || !task.name) {
                            return { error: `Validation Error: A task in step '${step.name}' is missing its name.` };
                        }
                        if (taskNames.has(task.name)) {
                            return { error: `Validation Error: Duplicate task name '${task.name}' in step '${step.name}'.` };
                        }
                        taskNames.add(task.name);
                    }
                    for (const task of step.tasks) {
                        if (Array.isArray(task.depends_on)) {
                            for (const dep of task.depends_on) {
                                if (!taskNames.has(dep)) {
                                    return { error: `Validation Error: Task '${task.name}' in step '${step.name}' depends on unknown task '${dep}'.` };
                                }
                            }
                        }
                    }
                    stepTaskMaps.set(step.name, taskNames);
                }
            }

            for (const step of pipeline.steps) {
                if (Array.isArray(step.depends_on)) {
                    for (const dep of step.depends_on) {
                        if (!stepNames.has(dep)) {
                            return { error: `Validation Error: Step '${step.name}' depends on unknown step '${dep}'.` };
                        }
                    }
                }
            }

            return null;
        } catch (error) {
            return { error: `YAML Parsing Error: ${error.message}` };
        }
    }

    function generateMermaidDefinition(yamlString) {
        const parsed = parseYamlSafely(yamlString);
        if (!parsed || !Array.isArray(parsed.steps)) {
            return 'graph TD; A[No steps defined]';
        }
        const nodes = new Set();
        const edges = [];
        parsed.steps.forEach(step => {
            if (!step || !step.name) return;
            const id = step.name.replace(/[^a-zA-Z0-9_]/g, '_');
            nodes.add({ id, label: step.name });
            (step.depends_on || []).forEach(dep => {
                const depId = String(dep).replace(/[^a-zA-Z0-9_]/g, '_');
                edges.push({ from: depId, to: id });
            });
        });
        let def = 'graph TD\n';
        nodes.forEach(node => {
            def += `    ${node.id}["${node.label}"]\n`;
        });
        edges.forEach(edge => {
            def += `    ${edge.from} --> ${edge.to}\n`;
        });
        return def;
    }

    async function getTriggersForPipeline(pipelineId) {
        if (state.triggersIndex.has(pipelineId)) {
            return state.triggersIndex.get(pipelineId);
        }

        const repos = await context.fetchData('/v1/overrides');
        if (!Array.isArray(repos) || repos.length === 0) {
            state.triggersIndex.set(pipelineId, []);
            return [];
        }

        const matches = [];
        for (const repo of repos) {
            const [owner, name] = repo.split('/');
            if (!owner || !name) continue;
            const url = `/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`;
            const yaml = await context.fetchData(url);
            if (typeof yaml !== 'string') continue;
            const manifest = parseYamlSafely(yaml);
            if (!manifest || !Array.isArray(manifest.triggers)) continue;
            manifest.triggers.forEach(trigger => {
                const pipelineEntries = Array.isArray(trigger.pipelines) ? trigger.pipelines : [];
                pipelineEntries.forEach(entry => {
                    const path = typeof entry === 'string' ? entry : entry?.path;
                    if (normalizePipelineIdentifier(path) === pipelineId) {
                        matches.push(trigger);
                    }
                });
            });
        }

        state.triggersIndex.set(pipelineId, matches);
        return matches;
    }

    function normalizePipelineIdentifier(value) {
        if (!value) return '';
        let str = String(value).trim();
        str = str.replace(/^\.nopsai\//, '');
        str = str.replace(/\.ya?ml$/i, '');
        return str.replace(/\/+/g, '/').replace(/^\//, '');
    }

    async function getRecentRunsForPipeline(pipelineId) {
        const now = Date.now();
        if (!state.runsCache.runs.length || (now - state.runsCache.fetchedAt) > 60000) {
            const runs = await context.fetchData('/v1/runs');
            if (Array.isArray(runs)) {
                state.runsCache = { runs, fetchedAt: now };
            }
        }

        const runs = state.runsCache.runs || [];
        const { path, name } = parsePipelineIdentifier(pipelineId);
        return runs.filter(run => {
            const runName = (run.pipeline_name || '').toLowerCase();
            const runPath = (run.pipeline_path || '').toLowerCase();
            return runName === name.toLowerCase() && runPath === (path || '').toLowerCase();
        });
    }

    function formatRelativeTime(dateString) {
        if (!dateString) return 'N/A';
        const delta = (Date.now() - new Date(dateString).getTime()) / 1000;
        if (Number.isNaN(delta)) return dateString;
        if (delta < 60) return 'Just now';
        if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
        if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`;
        return `${Math.floor(delta / 86400)}d ago`;
    }

    function escapeHtml(value) {
        return String(value ?? '').replace(/[&<>'"`]/g, c => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;', '`': '&#96;'
        })[c]);
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

    function renderSidebarForRoute() {
        // Navigation sidebar is rendered by the pipeline runs module; this function is a no-op
        // and exists for interface symmetry with other pages.
    }

    global.pages = global.pages || {};
    global.pages.pipelines = {
        init,
        handleRoute,
        refresh: () => refreshPipelines(true),
        renderSidebarForRoute,
    };
})(window.NopsAI = window.NopsAI || {});
