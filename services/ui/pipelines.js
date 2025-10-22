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
        sidebarExpanded: new Set(),
        syncLogEntries: [],
        syncInProgress: false,
        lastSyncStatus: null,
        pendingDelete: null,
        isEditing: false,
        currentYaml: '',
    };

    const DOM = {};
    let context = null;
    let pipelineRunsModule = null;

    const TOAST_TIMEOUT = 4000;

    function isPipelinesPageActive() {
        const page = document.querySelector('[data-page="pipelines"]');
        return !!(page && page.classList.contains('active'));
    }

    function notifySidebarTreeUpdate() {
        if (!isPipelinesPageActive()) return;
        if (!pipelineRunsModule || typeof pipelineRunsModule.renderSidebarForRoute !== 'function') {
            return;
        }
        try {
            const result = pipelineRunsModule.renderSidebarForRoute('pipelines');
            if (result && typeof result.then === 'function') {
                result.catch(err => console.error('Failed to refresh pipelines sidebar tree:', err));
            }
        } catch (error) {
            console.error('Failed to refresh pipelines sidebar tree:', error);
        }
    }

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
            'pipelines-delete-cancel', 'pipelines-delete-close', 'pipelines-sync-report'
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
            DOM['pipelines-refresh-btn'].addEventListener('click', () => {
                syncPipelinesFromRepo().catch(() => {});
            });
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

        const { pipelineId, autoEdit } = parsePipelineRoute(hash);
        if (pipelineId) {
            const folderPath = getFolderPathForPipelineId(pipelineId);
            if (folderPath) {
                ensureSidebarExpansionForPath(folderPath);
                state.activeFolderKey = folderPath;
            }
        } else {
            state.activeFolderKey = '';
            state.selectedId = null;
            state.sidebarExpanded = new Set();
        }

        const { DOM: globalDOM } = context;
        if (globalDOM.mainHeader) {
            globalDOM.mainHeader.textContent = 'Pipelines';
        }

        if (pipelineRunsModule && typeof pipelineRunsModule.renderSidebarForRoute === 'function') {
            await pipelineRunsModule.renderSidebarForRoute('pipelines');
        }

        await refreshPipelines();

        if (pipelineRunsModule && typeof pipelineRunsModule.renderSidebarForRoute === 'function') {
            await pipelineRunsModule.renderSidebarForRoute('pipelines');
        }

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
        notifySidebarTreeUpdate();
    }

    async function syncPipelinesFromRepo() {
        if (!context || typeof context.fetchData !== 'function') {
            await refreshPipelines(true);
            return;
        }

        const button = DOM['pipelines-refresh-btn'];
        if (button) {
            button.disabled = true;
            button.classList.add('cursor-wait', 'opacity-70');
        }

        state.syncLogEntries = [];
        state.syncInProgress = true;
        state.lastSyncStatus = 'loading';
        renderSyncStatusCard({
            status: 'loading',
            title: 'Syncing pipelines',
            message: 'Sync request sent. Waiting for the repository synchronization to finish…',
            logs: state.syncLogEntries,
        });

        try {
            const baseUrlRaw = (context && typeof context.apiBaseUrl === 'string') ? context.apiBaseUrl : '';
            const baseUrl = baseUrlRaw.replace(/\/+$/, '');
            const syncUrl = `${baseUrl}/v1/internal/config/sync`;

            const response = await fetch(syncUrl, { method: 'POST' });
            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(errorText || `Sync failed (${response.status})`);
            }
            showToast('Sync request accepted. Monitoring for completion…', 'info');
        } catch (error) {
            console.error('Failed to sync pipelines:', error);
            showToast('Sync failed. Please try again.', 'error');
            renderSyncStatusCard({
                status: 'error',
                title: 'Sync failed',
                message: error?.message ? error.message : 'The sync request was not successful. Please check the server logs and try again.',
                raw: error,
                logs: state.syncLogEntries,
            });
            state.syncInProgress = false;
            state.lastSyncStatus = 'error';
        } finally {
            if (button) {
                button.disabled = false;
                button.classList.remove('cursor-wait', 'opacity-70');
            }
        }
    }

    function formatSyncDetails(details) {
        if (!details) return '';
        if (typeof details === 'string') {
            return `<p>${escapeHtml(details)}</p>`;
        }
        if (Array.isArray(details)) {
            const items = details.map(item => {
                if (item == null) return '<li><span class="text-[var(--text-secondary)]">—</span></li>';
                if (typeof item === 'string' || typeof item === 'number' || typeof item === 'boolean') {
                    return `<li>${escapeHtml(String(item))}</li>`;
                }
                return `<li><pre>${escapeHtml(JSON.stringify(item, null, 2))}</pre></li>`;
            }).join('');
            return `<ul class="sync-detail-list">${items}</ul>`;
        }
        if (typeof details === 'object') {
            const entries = Object.entries(details);
            if (!entries.length) return '';
            const items = entries.map(([key, value]) => {
                let valueHtml;
                if (value == null) {
                    valueHtml = '<span class="text-[var(--text-secondary)]">—</span>';
                } else if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
                    valueHtml = `<span>${escapeHtml(String(value))}</span>`;
                } else if (Array.isArray(value)) {
                    if (value.every(item => typeof item === 'string' || typeof item === 'number' || typeof item === 'boolean')) {
                        valueHtml = `<span>${escapeHtml(value.join(', '))}</span>`;
                    } else {
                        valueHtml = `<pre>${escapeHtml(JSON.stringify(value, null, 2))}</pre>`;
                    }
                } else {
                    valueHtml = `<pre>${escapeHtml(JSON.stringify(value, null, 2))}</pre>`;
                }
                return `<li><strong>${escapeHtml(key)}:</strong> ${valueHtml}</li>`;
            }).join('');
            return `<ul class="sync-detail-list">${items}</ul>`;
        }
        return `<p>${escapeHtml(String(details))}</p>`;
    }

    function normalizeSyncResult(result) {
        if (result === null || result === undefined) {
            return { message: 'Sync completed. No additional details were provided.' };
        }
        if (typeof result === 'string') {
            return { message: result };
        }
        if (Array.isArray(result)) {
            return {
                message: 'Sync completed with the following updates:',
                details: result,
            };
        }
        if (typeof result === 'object') {
            const { message, summary, status, ...rest } = result;
            const primary = message || summary || status;
            const details = Object.keys(rest).length ? rest : null;
            return {
                message: primary || 'Sync completed successfully.',
                details,
            };
        }
        return { message: 'Sync completed successfully.' };
    }

    function renderSyncStatusCard(options) {
        const container = DOM['pipelines-sync-report'];
        if (!container) return;
        if (!options) {
            container.classList.add('hidden');
            container.innerHTML = '';
            return;
        }

        const status = options.status || 'info';
        const title = options.title || (status === 'success' ? 'Sync complete' : status === 'error' ? 'Sync failed' : 'Syncing pipelines');
        const message = options.message || '';
        let detailsHtml = formatSyncDetails(options.details);
        if (!detailsHtml && options.raw) {
            detailsHtml = formatSyncDetails(options.raw);
        }
        const logs = Array.isArray(options.logs) ? options.logs : [];
        const logsHtml = logs.length ? `<div class="pipeline-sync-log-wrap"><ul class="pipeline-sync-log">${logs.map(formatSyncLogEntry).join('')}</ul></div>` : '';

        let iconPath = 'M4 4.5v5h4.5m11-0.5v-5h-4.5m4.154 9.095A8.25 8.25 0 0112 20.25a8.25 8.25 0 01-7.654-5.095m0-6.31A8.25 8.25 0 0112 3.75a8.25 8.25 0 017.654 5.095';
        if (status === 'success') {
            iconPath = 'M5 13l4 4L19 7';
        } else if (status === 'error') {
            iconPath = 'M12 9v4m0 4h.01M5.455 5.455l13.09 13.09';
        }

        container.innerHTML = `
            <div class="pipeline-sync-card ${status}">
                <div class="sync-icon">
                    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <path d="${iconPath}" />
                    </svg>
                </div>
                <div class="flex-1 min-w-0">
                    <h3>${escapeHtml(title)}</h3>
                    ${message ? `<p>${escapeHtml(message)}</p>` : ''}
                    ${detailsHtml}
                    ${logsHtml}
                </div>
            </div>`;
        container.classList.remove('hidden');
    }

    async function handleConfigSyncEvent(event) {
        if (!event || typeof event !== 'object') return;

        const { status: rawStatus, logs, log, message, details, resetLogs } = event;
        let status = typeof rawStatus === 'string' ? rawStatus.toLowerCase() : '';
        if (!status && event.stage) status = String(event.stage).toLowerCase();

        if (resetLogs || status === 'started' || status === 'start') {
            state.syncLogEntries = [];
        }

        const normalizedLogs = [];

        if (Array.isArray(logs)) {
            logs.forEach(item => {
                const normalized = normalizeSyncLogItem(item);
                if (normalized) {
                    normalizedLogs.push(normalized);
                    pushSyncLogEntry(normalized);
                }
            });
        }

        if (log) {
            const normalized = normalizeSyncLogItem(log);
            if (normalized) {
                normalizedLogs.push(normalized);
                pushSyncLogEntry(normalized);
            }
        }

        let cardStatus = 'loading';
        if (['completed', 'complete', 'success', 'succeeded', 'done'].includes(status)) {
            cardStatus = 'success';
        } else if (['failed', 'error', 'errored'].includes(status)) {
            cardStatus = 'error';
        } else {
            cardStatus = 'loading';
        }

        const title = cardStatus === 'success'
            ? 'Sync complete'
            : cardStatus === 'error'
                ? 'Sync failed'
                : 'Sync in progress';

        const defaultMessage = cardStatus === 'success'
            ? 'Configuration synchronization from Git completed successfully.'
            : cardStatus === 'error'
                ? 'Configuration synchronization failed. Check the details below.'
                : 'Synchronization is in progress…';

        renderSyncStatusCard({
            status: cardStatus,
            title,
            message: message || defaultMessage,
            details,
            raw: event,
            logs: state.syncLogEntries,
        });

        if (cardStatus !== state.lastSyncStatus) {
            state.lastSyncStatus = cardStatus;
            if (cardStatus === 'success') {
                state.syncInProgress = false;
                showToast('Pipelines synced from repository.', 'success');
                await refreshPipelines(true);
            } else if (cardStatus === 'error') {
                state.syncInProgress = false;
                showToast(message || 'Pipeline synchronization failed.', 'error');
            } else {
                state.syncInProgress = true;
            }
        }
    }

    function formatSyncLogEntry(entry) {
        if (!entry) return '';
        if (typeof entry === 'string') {
            entry = normalizeSyncLogItem(entry);
            if (!entry) return '';
        }

        const parsed = entry.parsed && typeof entry.parsed === 'object' ? entry.parsed : null;
        if (parsed) {
            const iso = typeof parsed.time === 'string' ? parsed.time : (typeof parsed.timestamp === 'string' ? parsed.timestamp : null);
            let timeDisplay = '';
            if (iso) {
                const date = new Date(iso);
                if (!Number.isNaN(date.getTime())) {
                    timeDisplay = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
                }
            }
            const level = (parsed.level || 'info').toString().toUpperCase();
            const message = parsed.message || '';
            const { level: _l, message: _m, time: _t, timestamp: _ts, ...rest } = parsed;
            const meta = Object.keys(rest).length ? `<details class="pipeline-sync-log-details"><summary>Details</summary><pre>${escapeHtml(JSON.stringify(rest, null, 2))}</pre></details>` : '';
            return `<li>
                <div class="sync-log-line">
                    <span class="sync-log-time">${escapeHtml(timeDisplay)}</span>
                    <span class="sync-log-level sync-log-level-${escapeHtml(level.toLowerCase())}">${escapeHtml(level)}</span>
                    <span class="sync-log-message">${escapeHtml(message)}</span>
                </div>
                ${meta}
            </li>`;
        }
        return `<li><div class="sync-log-line"><span class="sync-log-message">${escapeHtml(entry.raw || String(entry))}</span></div></li>`;
    }

    function normalizeSyncLogItem(item) {
        if (item == null) return null;
        if (typeof item === 'string') {
            const trimmed = item.trim();
            if (!trimmed) return null;
            try {
                const parsed = JSON.parse(trimmed);
                return { raw: trimmed, parsed };
            } catch {
                return { raw: trimmed };
            }
        }
        if (typeof item === 'object') {
            if (item.raw || item.parsed) return item;
            return { raw: JSON.stringify(item), parsed: item };
        }
        return { raw: String(item) };
    }

    function pushSyncLogEntry(entry) {
        if (!entry) return;
        if (!Array.isArray(state.syncLogEntries)) {
            state.syncLogEntries = [];
        }
        state.syncLogEntries.push(entry);
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
                : '';
        }
    }

    function showListView() {
        state.selectedId = null;
        state.isEditing = false;
        setActiveView('list');
        notifySidebarTreeUpdate();
    }

    async function selectPipeline(pipelineId, options = {}) {
        const entry = await ensurePipelineSummary(pipelineId);
        if (!entry) {
            showToast(`Unable to load pipeline '${pipelineId}'.`, 'error');
            showListView();
            return;
        }

        state.selectedId = pipelineId;
        state.activeFolderKey = getFolderPathForPipelineId(pipelineId);
        state.currentYaml = entry?.yaml || '';
        state.isEditing = false;

        ensureSidebarExpansionForPath(state.activeFolderKey);
        notifySidebarTreeUpdate();

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

        const pipelinesHtml = pipelineCards.length
            ? `<div class="pipelines-card-grid pipelines-card-grid--pipelines">${pipelineCards.join('')}</div>`
            : '';

        const foldersHtml = folderCards.length
            ? `<div class="pipelines-card-grid pipelines-card-grid--folders">${folderCards.join('')}</div>`
            : '';

        const gridHtml = pipelinesHtml || foldersHtml
            ? `${pipelinesHtml}${foldersHtml}`
            : `<div class="pipeline-folder-empty-state">No pipelines in this folder yet.</div>`;

        DOM['pipelines-list-container'].innerHTML = gridHtml;
    }

    function getFolderPathForPipelineId(pipelineId) {
        if (!pipelineId) return '';
        const segments = pipelineId.split('/').filter(Boolean);
        segments.pop();
        return segments.join('/');
    }

    function getActiveSidebarFolder() {
        const explicit = (state.activeFolderKey || '').trim();
        if (explicit) return explicit;
        return getFolderPathForPipelineId(state.selectedId || '');
    }

    function ensureSidebarExpansionForPath(path) {
        if (!(state.sidebarExpanded instanceof Set)) {
            state.sidebarExpanded = new Set();
        }
        const segments = (path || '').split('/').filter(Boolean);
        let current = '';
        segments.forEach(segment => {
            current = current ? `${current}/${segment}` : segment;
            state.sidebarExpanded.add(current);
        });
    }

    function shouldExpandFolder(path, activeFolder, activePipelineFolder) {
        if (!path) return true;
        if (!(state.sidebarExpanded instanceof Set)) {
            state.sidebarExpanded = new Set();
        }
        if (state.sidebarExpanded.has(path)) return true;
        const hasActiveFolder = activeFolder && (activeFolder === path || activeFolder.startsWith(`${path}/`));
        if (hasActiveFolder) return true;
        const hasActivePipeline = activePipelineFolder && (activePipelineFolder === path || activePipelineFolder.startsWith(`${path}/`));
        return hasActivePipeline;
    }

    function buildPipelineHash(identifier) {
        const segments = (identifier || '').split('/').filter(Boolean).map(encodeURIComponent);
        return segments.length ? `#/pipelines/${segments.join('/')}` : '#/pipelines';
    }

    function renderSidebarTreeNodes(node, level, activeFolder, activePipeline) {
        const childEntries = node && node.children instanceof Map ? Array.from(node.children.entries()) : [];
        const pipelineEntries = Array.isArray(node?.pipelines) ? node.pipelines.slice() : [];

        if (!childEntries.length && pipelineEntries.length === 0) {
            return '';
        }

        childEntries.sort((a, b) => {
            const labelA = (a[0] || '').toLowerCase();
            const labelB = (b[0] || '').toLowerCase();
            return labelA.localeCompare(labelB);
        });

        pipelineEntries.sort((a, b) => {
            const nameA = (a.meta?.name || a.id || '').toLowerCase();
            const nameB = (b.meta?.name || b.id || '').toLowerCase();
            return nameA.localeCompare(nameB);
        });

        let html = `<ul class="${level > 0 ? 'pl-4' : ''} space-y-1">`;

        const activePipelineFolder = getFolderPathForPipelineId(activePipeline);

        childEntries.forEach(([segment, childNode]) => {
            const folderPath = (childNode && childNode.key) || segment || '';
            const isExpanded = shouldExpandFolder(folderPath, activeFolder, activePipelineFolder);
            if (isExpanded) ensureSidebarExpansionForPath(folderPath);
            const isActiveFolder = !!folderPath && folderPath === activeFolder;
            const folderLabel = formatPathLabel(childNode?.label || segment || '');
            const childrenHtml = renderSidebarTreeNodes(childNode, level + 1, activeFolder, activePipeline);

            html += `
                <li data-pipeline-folder="${escapeAttribute(folderPath)}">
                    <div class="flex items-center justify-between p-2 text-[var(--text-primary)] rounded-md pipeline-sidebar-folder-row ${isActiveFolder ? 'bg-[var(--bg-tertiary)]' : ''}">
                        <button type="button" class="pipeline-sidebar-toggle mr-2 text-[var(--text-secondary)]" data-toggle-folder="${escapeAttribute(folderPath)}" aria-expanded="${isExpanded ? 'true' : 'false'}" aria-label="${escapeAttribute((isExpanded ? 'Collapse' : 'Expand') + ' ' + folderLabel)}">
                            <svg class="h-4 w-4 chevron ${isExpanded ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>
                        </button>
                        <button type="button" class="pipeline-sidebar-folder flex items-center gap-2 flex-grow text-left" data-open-folder="${escapeAttribute(folderPath)}">
                            <svg class="h-4 w-4 text-[var(--text-secondary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>
                            <span class="truncate">${escapeHtml(folderLabel)}</span>
                        </button>
                    </div>
                    <div class="pipeline-sidebar-children ${isExpanded ? '' : 'hidden'}" data-folder-children="${escapeAttribute(folderPath)}">
                        ${childrenHtml}
                    </div>
                </li>`;
        });

        pipelineEntries.forEach(pipeline => {
            const pipelineId = pipeline.id;
            const pipelineName = pipeline.meta?.name || pipelineId.split('/').pop() || pipelineId;
            const isActive = state.selectedId === pipelineId;
            const pipelineHref = buildPipelineHash(pipelineId);
            const parentFolder = getFolderPathForPipelineId(pipelineId);
            html += `
                <li data-pipeline-id="${escapeAttribute(pipelineId)}">
                    <a href="${pipelineHref}" class="sidebar-link flex items-center p-2 text-[var(--text-primary)] rounded-md transition-colors duration-200 ${isActive ? 'active' : ''}" data-navigo data-pipeline-link="${escapeAttribute(pipelineId)}" data-parent-folder="${escapeAttribute(parentFolder)}">
                        <svg class="h-4 w-4 mr-2 text-[var(--text-secondary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 4h8l4 4v12a2 2 0 01-2 2H6a2 2 0 01-2-2V6a2 2 0 012-2z"/></svg>
                        <span class="truncate">${escapeHtml(pipelineName)}</span>
                    </a>
                </li>`;
        });

        html += '</ul>';
        return html;
    }

    function renderSidebarTree(container) {
        if (!container) return;
        if (!(state.sidebarExpanded instanceof Set)) {
            state.sidebarExpanded = new Set();
        }

        const activeFolder = getActiveSidebarFolder();
        const activePipeline = state.selectedId || '';
        ensureSidebarExpansionForPath(activeFolder);
        ensureSidebarExpansionForPath(getFolderPathForPipelineId(activePipeline));

        const tree = buildGroupedPipelines();
        const treeHtml = renderSidebarTreeNodes(tree, 0, activeFolder, activePipeline);
        container.innerHTML = treeHtml || `<p class="px-2 text-sm text-[var(--text-secondary)]">No pipelines available.</p>`;

        if (!container.dataset.sidebarBound) {
            container.addEventListener('click', handleSidebarTreeClick);
            container.dataset.sidebarBound = 'true';
        }
    }

    function handleSidebarTreeClick(event) {
        const toggleBtn = event.target.closest('[data-toggle-folder]');
        if (toggleBtn) {
            const folderPath = toggleBtn.dataset.toggleFolder || '';
            if (!(state.sidebarExpanded instanceof Set)) {
                state.sidebarExpanded = new Set();
            }
            let listNeedsRefresh = false;
            if (state.sidebarExpanded.has(folderPath)) {
                state.sidebarExpanded.delete(folderPath);
                if (state.activeFolderKey === folderPath) {
                    const parentSegments = folderPath.split('/').filter(Boolean);
                    parentSegments.pop();
                    state.activeFolderKey = parentSegments.join('/');
                    listNeedsRefresh = true;
                }
            } else if (folderPath) {
                state.sidebarExpanded.add(folderPath);
                state.activeFolderKey = folderPath;
                listNeedsRefresh = true;
            }
            if (listNeedsRefresh) {
                renderPipelineList();
            }
            notifySidebarTreeUpdate();
            event.preventDefault();
            event.stopPropagation();
            return;
        }

        const folderBtn = event.target.closest('[data-open-folder]');
        if (folderBtn) {
            const folderPath = folderBtn.dataset.openFolder || '';
            state.activeFolderKey = folderPath;
            state.selectedId = null;
            ensureSidebarExpansionForPath(folderPath);
            showListView();
            renderPipelineList();
            event.preventDefault();
            event.stopPropagation();
            return;
        }

        const pipelineLink = event.target.closest('[data-pipeline-link]');
        if (pipelineLink) {
            const parentFolder = pipelineLink.dataset.parentFolder || '';
            ensureSidebarExpansionForPath(parentFolder);
            state.activeFolderKey = parentFolder;
            // allow navigation to proceed naturally
        }
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

    function renderFolderCard(node) {
        const keyAttr = escapeHtml(node.key || '');
        const label = formatPathLabel(node.label || node.key || 'Folder');
        const totalPipelines = countPipelinesRecursive(node);
        const childCount = node.children ? node.children.size : 0;
        const description = getFolderDescription(node);

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
                        <p class="pipeline-folder-description" title="${escapeHtml(description)}">${escapeHtml(description)}</p>
                    </div>
                    <span class="pipeline-folder-chevron" aria-hidden="true">
                        <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <path d="M9 5l7 7-7 7" />
                        </svg>
                    </span>
                </div>
                <div class="pipeline-folder-meta">
                    <div class="pipeline-folder-meta-row">
                        <span class="pipeline-folder-meta-label">Pipelines:</span>
                        <span class="pipeline-folder-meta-value">${totalPipelines}</span>
                    </div>
                    <div class="pipeline-folder-meta-row">
                        <span class="pipeline-folder-meta-label">Sub folders:</span>
                        <span class="pipeline-folder-meta-value">${childCount}</span>
                    </div>
                </div>
            </article>`;
    }

    function getFolderDescription(node) {
        const direct = (node.pipelines || []).find(p => (p.meta?.description || '').trim());
        if (direct && direct.meta) {
            return direct.meta.description.trim();
        }
        if (node.children) {
            for (const child of node.children.values()) {
                const desc = getFolderDescription(child);
                if (desc && desc !== 'No description provided') {
                    return desc;
                }
            }
        }
        return 'No description provided';
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
            <article class="pipeline-card bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded-lg shadow-sm transition-all duration-200 p-3 flex flex-col" data-pipeline-id="${idAttr}" tabindex="0" role="button" aria-label="Open pipeline ${escapeHtml(rawName)}">
                <div class="pipeline-card-header flex items-start justify-between gap-3">
                    <div class="pipeline-card-text min-w-0">
                        <h3 class="pipeline-card-title" title="${name}">${name}</h3>
                        <p class="pipeline-card-path" title="${pathLabel}">${pathLabel}</p>
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
        notifySidebarTreeUpdate();
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
            ensureSidebarExpansionForPath(folderKey);
            notifySidebarTreeUpdate();
            return;
        }

        const folderCard = event.target.closest('[data-folder-key]');
        if (folderCard) {
            const folderKey = folderCard.getAttribute('data-folder-key') || '';
            state.activeFolderKey = folderKey;
            renderPipelineList();
            ensureSidebarExpansionForPath(folderKey);
            notifySidebarTreeUpdate();
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
            ensureSidebarExpansionForPath(folderKey);
            notifySidebarTreeUpdate();
            focusFirstListItem();
            return;
        }

        const folderCard = event.target.closest('[data-folder-key]');
        if (folderCard && folderCard === document.activeElement) {
            event.preventDefault();
            const folderKey = folderCard.getAttribute('data-folder-key') || '';
            state.activeFolderKey = folderKey;
            renderPipelineList();
            ensureSidebarExpansionForPath(folderKey);
            notifySidebarTreeUpdate();
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

    function renderSidebarForRoute() {
        const container = document.getElementById('pipelines-sidebar-tree');
        if (container) {
            renderSidebarTree(container);
        }
    }

    global.pages = global.pages || {};
    global.pages.pipelines = {
        init,
        handleRoute,
        refresh: () => refreshPipelines(true),
        renderSidebarForRoute,
        renderSidebarTree,
        handleConfigSyncEvent,
    };
})(window.NopsAI = window.NopsAI || {});
