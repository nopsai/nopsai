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
        autocomplete: {
            secrets: [],
            environments: [],
            reusableSteps: [],
            fetchedAt: 0,
            isLoading: false,
            loadingPromise: null,
        },
        editorSuggestionContext: null,
        beforeUnloadHandler: null,
    };

    const DOM = {};
    let context = null;
    let pipelineRunsModule = null;

    const TOAST_TIMEOUT = 4000;
    const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;

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
            'pipeline-detail-description', 'pipeline-detail-path', 'pipeline-detail-version', 'pipeline-detail-source', 'pipeline-copy-btn',
            'pipeline-download-btn', 'pipeline-edit-btn', 'pipeline-save-btn', 'pipeline-cancel-btn',
            'pipeline-yaml-content', 'pipeline-yaml-editor', 'editor-container', 'line-numbers',
            'validation-status', 'yaml-view-actions', 'yaml-edit-actions', 'pipeline-graph',
            'pipeline-triggers', 'pipeline-recent-runs', 'pipelines-new-modal', 'pipelines-new-form',
            'pipelines-new-close', 'pipelines-new-cancel', 'pipelines-new-path', 'pipelines-new-name',
            'pipelines-delete-modal', 'pipelines-delete-message', 'pipelines-delete-confirm',
            'pipelines-delete-cancel', 'pipelines-delete-close', 'pipelines-sync-report', 'pipelines-search-container'
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
                updateEditorSuggestions();
            });
            DOM['pipeline-yaml-editor'].addEventListener('scroll', () => {
                if (DOM['line-numbers']) {
                    DOM['line-numbers'].scrollTop = DOM['pipeline-yaml-editor'].scrollTop;
                }
            });
            DOM['pipeline-yaml-editor'].addEventListener('click', () => {
                updateEditorSuggestions();
            });
            DOM['pipeline-yaml-editor'].addEventListener('keyup', (event) => {
                if (event.key === 'Shift' || event.key === 'Control' || event.key === 'Alt' || event.key === 'Meta') {
                    return;
                }
                updateEditorSuggestions();
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
                    updateEditorSuggestions();
                } else if (event.key === 'Escape') {
                    hideEditorSuggestions();
                }
            });
        }
    }

    function parsePipelineRoute(hash) {
        const clean = (hash || window.location.hash || '').replace(/^#/, '').replace(/^\//, '');
        if (!clean) {
            return { path: 'pipelines', segments: [], pipelineId: null, isEdit: false };
        }
        const parts = clean.split('/');
        if (parts[0] !== 'pipelines') {
            return { path: parts[0], segments: [], pipelineId: null, isEdit: false };
        }

        const segments = parts.slice(1).map(decodeURIComponent);
        let pipelineId = null;
        let isEdit = false;
        let pathSegments = [];

        if (segments.length > 0 && segments[segments.length - 1] === 'edit') {
            isEdit = true;
            segments.pop();
        }

        if (segments.length > 0 && !segments[segments.length - 1].includes('/')) {
            const potentialId = segments.join('/');
            if (state.pipelines.includes(potentialId)) {
                pipelineId = potentialId;
                pathSegments = segments;
            } else if (segments.length > 1) {
                pipelineId = segments.join('/');
                pathSegments = segments.slice(0, -1);
            } else {
                pipelineId = segments[0];
                pathSegments = [];
            }
            if (pipelineId && !state.pipelines.includes(pipelineId)){
                pipelineId = null;
                pathSegments = segments;
            }

        } else {
            pathSegments = segments;
        }

        const folderKey = pathSegments.join('/');

        return {
            path: 'pipelines',
            segments: segments,
            pipelineId: pipelineId,
            activeFolderKey: folderKey,
            isEdit: isEdit
        };
    }

    async function handleRoute(hash) {
        if (!context) return;

        const { DOM: globalDOM } = context;
        if (globalDOM.mainHeader) {
            globalDOM.mainHeader.innerHTML = 'Pipelines';
        }

        await refreshPipelines();

        const routeInfo = parsePipelineRoute(hash);

        if (state.isEditing) {
            const samePipeline = routeInfo.pipelineId && routeInfo.pipelineId === state.selectedId;
            const stayingInEdit = routeInfo.path === 'pipelines' && samePipeline && routeInfo.isEdit;
            if (!stayingInEdit) {
                if (editingPreventNavigation(hash)) {
                    return;
                }
            }
        }

        const { pipelineId, activeFolderKey, isEdit } = routeInfo;

        state.activeFolderKey = activeFolderKey || '';
        ensureSidebarExpansionForPath(state.activeFolderKey);

        if (pipelineRunsModule && typeof pipelineRunsModule.renderSidebarForRoute === 'function') {
            await pipelineRunsModule.renderSidebarForRoute('pipelines');
        }
        renderSidebarForRoute();

        if (pipelineId) {
            await selectPipeline(pipelineId, { autoEdit: isEdit });
        } else {
            showListView();
            renderPipelineList();
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
            console.error('Failed to *initiate* sync:', error);
            showToast('Failed to initiate sync request. Please try again.', 'error');
            renderSyncStatusCard({
                status: 'error',
                title: 'Sync failed',
                message: error?.message ? `Failed to send request: ${error.message}` : 'The sync request could not be sent. Please check server connectivity and try again.',
                raw: error,
                logs: state.syncLogEntries,
            });
            state.syncInProgress = false;
            state.lastSyncStatus = 'error';
            if (button) {
                button.disabled = false;
                button.classList.remove('cursor-wait', 'opacity-70');
            }
        }
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
            cardStatus = state.syncInProgress ? 'loading' : 'info'; // Fallback to 'info' if sync wasn't marked in progress
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
            title: title,
            message: message || defaultMessage,
            details: details,
            raw: event,
            logs: state.syncLogEntries,
        });

        if (cardStatus !== state.lastSyncStatus) {
            state.lastSyncStatus = cardStatus;
            const button = DOM['pipelines-refresh-btn'];

            if (cardStatus === 'success') {
                state.syncInProgress = false;
                showToast('Pipelines synced from repository.', 'success');
                await refreshPipelines(true);
                if (button) {
                    button.disabled = false;
                    button.classList.remove('cursor-wait', 'opacity-70');
                }
            } else if (cardStatus === 'error') {
                state.syncInProgress = false;
                showToast(message || 'Pipeline synchronization failed.', 'error');
                if (button) {
                    button.disabled = false;
                    button.classList.remove('cursor-wait', 'opacity-70');
                }
            } else {
                 state.syncInProgress = true;
                 if (button) {
                     button.disabled = true;
                     button.classList.add('cursor-wait', 'opacity-70');
                 }
            }
        } else if (cardStatus === 'loading') {
             const button = DOM['pipelines-refresh-btn'];
              if (button) {
                 button.disabled = true;
                 button.classList.add('cursor-wait', 'opacity-70');
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

        const status = options.status || 'info'; // 'loading', 'success', 'error', or 'info' as fallback
        const title = options.title ||
                      (status === 'success' ? 'Sync complete' :
                       status === 'error' ? 'Sync failed' :
                       status === 'loading' ? 'Syncing pipelines' : 'Sync Status');
        const message = options.message || '';

        let detailsHtml = formatSyncDetails(options.details);
        if (!detailsHtml && options.raw && options.status === 'error') {
             detailsHtml = formatSyncDetails(options.raw);
        }

        const logs = Array.isArray(options.logs) ? options.logs : [];
        const logsHtml = logs.length
            ? `<div class="pipeline-sync-log-wrap"><ul class="pipeline-sync-log">${logs.map(formatSyncLogEntry).join('')}</ul></div>`
            : '';

        let iconPath = 'M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182m0-4.991v4.99'; // Refresh/loading icon
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
                    ${detailsHtml || ''}
                    ${logsHtml}
                </div>
            </div>`;

        container.classList.remove('hidden');
    }

    function formatSyncLogEntry(entry) {
        if (!entry) return '';

        if (typeof entry === 'string') {
            entry = normalizeSyncLogItem(entry);
            if (!entry) return '';
        }

        const parsed = entry.parsed && typeof entry.parsed === 'object' ? entry.parsed : null;

        if (parsed) {
            const isoTime = typeof parsed.time === 'string' ? parsed.time : (typeof parsed.timestamp === 'string' ? parsed.timestamp : null);
            let timeDisplay = '';
            if (isoTime) {
                const date = new Date(isoTime);
                if (!Number.isNaN(date.getTime())) {
                    timeDisplay = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
                }
            }

            const level = (parsed.level || 'info').toString().toUpperCase();
            const message = parsed.message || '';

            const { level: _l, message: _m, time: _t, timestamp: _ts, ...rest } = parsed;

            const meta = Object.keys(rest).length
                ? `<details class="pipeline-sync-log-details">
                       <summary>Details</summary>
                       <pre>${escapeHtml(JSON.stringify(rest, null, 2))}</pre>
                   </details>`
                : '';

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

    function formatSyncDetails(details) {
        if (details == null) return '';

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
                }
                else if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
                    valueHtml = `<span>${escapeHtml(String(value))}</span>`;
                }
                else if (Array.isArray(value)) {
                    if (value.every(item => typeof item === 'string' || typeof item === 'number' || typeof item === 'boolean')) {
                         valueHtml = `<span>${escapeHtml(value.join(', '))}</span>`;
                    } else {
                         valueHtml = `<pre>${escapeHtml(JSON.stringify(value, null, 2))}</pre>`;
                    }
                }
                else {
                    valueHtml = `<pre>${escapeHtml(JSON.stringify(value, null, 2))}</pre>`;
                }
                return `<li><strong>${escapeHtml(key)}:</strong> ${valueHtml}</li>`;
            }).join('');
            return `<ul class="sync-detail-list">${items}</ul>`;
        }

        return `<p>${escapeHtml(String(details))}</p>`;
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

    async function preloadAutocompleteMetadata(force = false) {
        if (!context || typeof context.fetchData !== 'function') return;
        const now = Date.now();
        if (!force && state.autocomplete.fetchedAt && (now - state.autocomplete.fetchedAt) < AUTOCOMPLETE_REFRESH_INTERVAL) {
            return;
        }
        if (state.autocomplete.loadingPromise) {
            return state.autocomplete.loadingPromise;
        }

        const promise = (async () => {
            state.autocomplete.isLoading = true;
            try {
                const [secrets, envs, steps] = await Promise.all([
                    context.fetchData('/v1/secrets'),
                    context.fetchData('/v1/environments'),
                    context.fetchData('/v1/steps'),
                ]);
                state.autocomplete.secrets = normalizeAutocompleteList(secrets);
                state.autocomplete.environments = normalizeAutocompleteList(envs);
                state.autocomplete.reusableSteps = normalizeAutocompleteList(steps);
                state.autocomplete.fetchedAt = Date.now();
            } catch (error) {
                console.error('Failed to load autocomplete metadata for pipelines editor:', error);
            } finally {
                state.autocomplete.isLoading = false;
                state.autocomplete.loadingPromise = null;
                updateEditorSuggestions();
            }
        })();

        state.autocomplete.loadingPromise = promise;
        return promise;
    }

    function normalizeAutocompleteList(value) {
        if (!Array.isArray(value)) return [];
        return value
            .map(item => typeof item === 'string' ? item.trim() : '')
            .filter(Boolean);
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

    function calculateGraphLayout(items, container, nodeWidth, nodeHeight, hGap, vGap, isVertical = false) {
        if (!items || items.length === 0) return { nodes: [], edges: [], width: 0, height: 0 };

        const nodes = {};
        const adj = {};
        const itemNameKey = 'task_name' in items[0] ? 'task_name' : 'name';

        items.forEach((item, index) => {
            const name = item[itemNameKey];
            nodes[name] = { ...item, id: name, level: -1, parents: new Set(), children: new Set() };
            adj[name] = [];
        });

        items.forEach(item => {
            const name = item[itemNameKey];
            (item.depends_on || []).forEach(dep => {
                if (nodes[dep] && nodes[name]) {
                    adj[dep].push(name);
                    nodes[name].parents.add(dep);
                    nodes[dep].children.add(name);
                }
            });
        });

        let level = 0;
        let queue = Object.values(nodes).filter(node => node.parents.size === 0);
        let processedCount = 0;

        while(queue.length > 0) {
            const levelSize = queue.length;
            for(let i=0; i < levelSize; i++){
                const node = queue.shift();
                node.level = level;
                processedCount++;

                (adj[node.id] || []).forEach(childId => {
                    const childNode = nodes[childId];
                    childNode.parents.delete(node.id);
                    if (childNode.parents.size === 0) {
                        queue.push(childNode);
                    }
                });
            }
            level++;
        }

        if (processedCount < Object.keys(nodes).length) {
            console.warn("Cycle detected in graph, layout may be incorrect.");
            Object.values(nodes).filter(n => n.level === -1).forEach(n => n.level = level);
        }

        const levels = [];
        Object.values(nodes).forEach(node => {
            if (!levels[node.level]) levels[node.level] = [];
            levels[node.level].push(node);
        });

        let PADDING_X = 80;
        let PADDING_Y = 136;
        try {
            const isTaskGraph = (itemNameKey === 'task_name');
            const isMiniContainer = !!(container && container.classList && container.classList.contains('task-graph-mini-container'));
            if (isTaskGraph) {
                PADDING_X = isMiniContainer ? 12 : 24;
                PADDING_Y = isMiniContainer ? 12 : 24;
            }
        } catch {}
        let totalWidth, totalHeight;

        if (isVertical) {
            let maxNodesInLevel = 0;
            levels.forEach(l => { if(l) maxNodesInLevel = Math.max(maxNodesInLevel, l.length); });
            totalWidth = maxNodesInLevel * nodeWidth + (maxNodesInLevel > 1 ? (maxNodesInLevel - 1) * hGap : 0);
            totalHeight = levels.length * nodeHeight + (levels.length > 1 ? (levels.length - 1) * vGap : 0);

            levels.forEach((levelNodes, i) => {
                if (!levelNodes) return;
                const levelWidth = levelNodes.length * nodeWidth + (levelNodes.length > 1 ? (levelNodes.length - 1) * hGap : 0);
                const xOffset = (totalWidth - levelWidth) / 2;
                levelNodes.forEach((node, j) => {
                    node.x = j * (nodeWidth + hGap) + xOffset + PADDING_X / 2;
                    node.y = i * (nodeHeight + vGap) + PADDING_Y / 2;
                });
            });
        } else {
            let maxNodesInLevel = 0;
            levels.forEach(l => { if(l) maxNodesInLevel = Math.max(maxNodesInLevel, l.length); });
            totalWidth = levels.length * nodeWidth + (levels.length > 1 ? (levels.length - 1) * hGap : 0);
            totalHeight = maxNodesInLevel * nodeHeight + (maxNodesInLevel > 1 ? (maxNodesInLevel - 1) * vGap : 0);

            levels.forEach((levelNodes, i) => {
                if (!levelNodes) return;
                const levelHeight = levelNodes.length * nodeHeight + (levelNodes.length > 1 ? (levelNodes.length - 1) * vGap : 0);
                const yOffset = (totalHeight - levelHeight) / 2;
                levelNodes.forEach((node, j) => {
                    node.x = i * (nodeWidth + hGap) + PADDING_X / 2;
                    node.y = j * (nodeHeight + vGap) + yOffset + PADDING_Y / 2;
                });
            });
        }

        Object.values(nodes).forEach(node => {
            node.width = nodeWidth;
            node.height = nodeHeight;
        });

        const edges = [];
        Object.values(nodes).forEach(item => {
            (item.depends_on || []).forEach(dep => {
                const fromNode = nodes[dep];
                const toNode = nodes[item[itemNameKey]];
                if (fromNode && toNode) {
                    edges.push({ from: fromNode, to: toNode });
                }
            });
        });

        return {
            nodes: Object.values(nodes),
            edges,
            width: totalWidth + PADDING_X,
            height: totalHeight + PADDING_Y
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
        if (DOM['pipelines-search-container']) {
            DOM['pipelines-search-container'].classList.toggle('hidden', showDetail);
        }
        if (DOM['pipelines-new-btn']) {
            DOM['pipelines-new-btn'].classList.toggle('hidden', showDetail);
        }
        if (DOM['pipelines-refresh-btn']) {
            DOM['pipelines-refresh-btn'].classList.toggle('hidden', showDetail);
        }
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
        if (state.isEditing) {
            notifyEditingLock();
            restoreEditingRoute();
            return;
        }
        state.selectedId = null;
        state.isEditing = false;
        setActiveView('list');

        const expectedHash = buildFolderPathHash(state.activeFolderKey);
        if (window.location.hash !== expectedHash) {
            try {
                history.replaceState(null, '', expectedHash);
            } catch {
                window.location.hash = expectedHash;
            }
        }

        notifySidebarTreeUpdate();
    }

    async function selectPipeline(pipelineId, options = {}) {
        if (state.isEditing && pipelineId !== state.selectedId) {
            notifyEditingLock();
            restoreEditingRoute();
            return null;
        }
        const entry = await ensurePipelineSummary(pipelineId);
        if (!entry) {
            showToast(`Unable to load pipeline '${pipelineId}'.`, 'error');
            window.location.hash = buildFolderPathHash(state.activeFolderKey);
            return;
        }

        state.selectedId = pipelineId;
        state.currentYaml = entry?.yaml || '';
        state.isEditing = false;

        notifySidebarTreeUpdate();

        renderPipelineDetail(entry);
        setActiveView('detail');

        if (options.autoEdit) {
            enterEditMode();
        } else {
            const expectedHash = buildPipelineHash(pipelineId, false);
            if (window.location.hash !== expectedHash) {
                try {
                    history.replaceState(null, '', expectedHash);
                } catch {
                    window.location.hash = expectedHash;
                }
            }
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

    function buildPipelineHash(identifier, isEdit = false) {
        if (!identifier) return '#/pipelines';
        const segments = (identifier || '').split('/').filter(Boolean).map(encodeURIComponent);
        let hash = `#/pipelines/${segments.join('/')}`;
        if (isEdit) {
            hash += '/edit';
        }
        return hash;
    }

    function buildFolderPathHash(folderKey) {
        if (!folderKey) return '#/pipelines';
        const segments = (folderKey || '').split('/').filter(Boolean).map(encodeURIComponent);
        return `#/pipelines/${segments.join('/')}`;
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
                    
                    <div class="flex items-center justify-between p-1 text-[var(--text-primary)] rounded-md pipeline-sidebar-folder-row ${isActiveFolder ? 'bg-[var(--bg-tertiary)]' : ''} hover:bg-[var(--bg-tertiary)]">
                        <div class="flex items-center flex-grow min-w-0"> 
                            
                            <button type="button" class="sidebar-toggle-btn flex items-center justify-center h-5 w-5 rounded mr-1 text-[var(--text-secondary)] hover:bg-[var(--bg-hover)]" data-toggle-folder="${escapeAttribute(folderPath)}" aria-expanded="${isExpanded ? 'true' : 'false'}" aria-label="${escapeAttribute((isExpanded ? 'Collapse' : 'Expand') + ' ' + folderLabel)}">
                                <svg class="h-4 w-4 chevron ${isExpanded ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>
                            </button>
                            
                            <button type="button" class="pipeline-sidebar-folder flex items-center gap-2 flex-grow text-left min-w-0 p-1 rounded hover:bg-[var(--bg-hover)]" data-open-folder="${escapeAttribute(folderPath)}">
                                <svg class="h-4 w-4 text-[var(--text-secondary)] flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>
                                <span class="truncate">${escapeHtml(folderLabel)}</span>
                            </button>
                        </div>
                        
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
            event.preventDefault();
            event.stopPropagation();
            const folderPath = toggleBtn.dataset.toggleFolder || '';
            if (!(state.sidebarExpanded instanceof Set)) {
                state.sidebarExpanded = new Set();
            }

            if (state.sidebarExpanded.has(folderPath)) {
                state.sidebarExpanded.delete(folderPath);
            } else if (folderPath) {
                state.sidebarExpanded.add(folderPath);
            }

            const container = document.getElementById('pipelines-sidebar-tree');
            if (container) {
                renderSidebarTree(container);
            } else {
                 console.error("Sidebar container 'pipelines-sidebar-tree' not found.");
                 renderSidebarForRoute();
            }

            return;
        }


        const folderBtn = event.target.closest('[data-open-folder]');
        if (folderBtn) {
            if (state.isEditing) {
                notifyEditingLock();
                restoreEditingRoute();
                event.preventDefault();
                event.stopPropagation();
                return;
            }
            const folderPath = folderBtn.dataset.openFolder || '';
            if (!event.target.closest('[data-toggle-folder]')) {
                event.preventDefault();
                event.stopPropagation();
                window.location.hash = buildFolderPathHash(folderPath);
            }
            return;
        }

        
        const pipelineLink = event.target.closest('a[data-pipeline-link]');
        if (pipelineLink) {
            if (state.isEditing) {
                notifyEditingLock();
                restoreEditingRoute();
                event.preventDefault();
                event.stopPropagation();
                return;
            }
            const pipelineId = pipelineLink.dataset.pipelineLink;
            if (pipelineId) {
                const parentFolder = pipelineLink.dataset.parentFolder || '';
                ensureSidebarExpansionForPath(parentFolder);
                
                const container = document.getElementById('pipelines-sidebar-tree');
                if (container) renderSidebarTree(container);
            }
            return;
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
        const graphContainer = DOM['pipeline-graph'];
        if (!graphContainer) return;

        if (!yamlString) {
            graphContainer.innerHTML = '<p class="text-sm text-[var(--text-secondary)]">No definition available.</p>';
            return;
        }

        const parsed = parseYamlSafely(yamlString);
        if (!parsed || !Array.isArray(parsed.steps) || parsed.steps.length === 0) {
            graphContainer.innerHTML = '<p class="text-sm text-[var(--text-secondary)]">No steps defined in pipeline.</p>';
            return;
        }

        const steps = parsed.steps.map(s => ({
            name: s.name || 'unnamed',
            depends_on: s.depends_on || [],
        }));

        try {
            const isVerticalLayout = false;
            const nodeWidth = isVerticalLayout ? 160 : 120;
            const nodeHeight = isVerticalLayout ? 80 : 100;
            const hGap = isVerticalLayout ? 40 : 90;
            const vGap = isVerticalLayout ? 100 : 16;

            const layout = calculateGraphLayout(steps, graphContainer, nodeWidth, nodeHeight, hGap, vGap, isVerticalLayout);

            let svgContent = `<svg width="100%" height="auto" viewBox="0 0 ${layout.width} ${layout.height}" preserveAspectRatio="xMinYMin meet" xmlns="http://www.w3.org/2000/svg" style="width: 100%; height: auto; display: block;">
                <defs>
                     <radialGradient id="glassyIconGradientPipelineDef" cx="40%" cy="35%" r="80%" fx="30%" fy="30%">
                        <stop offset="0%" style="stop-color:rgba(254, 252, 232, 0.9)" /> <stop offset="50%" style="stop-color:rgba(250, 204, 21, 0.85)" /> <stop offset="100%" style="stop-color:rgba(217, 119, 6, 0.9)" /> </radialGradient>
                     <filter id="softIconShadowPipelineDef" x="-40%" y="-40%" width="180%" height="180%">
                        <feDropShadow dx="1" dy="3" stdDeviation="2.5" flood-color="#a16207" flood-opacity="0.25"/>
                     </filter>
                    <marker id="pipeline-def-arrowhead" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="5" markerHeight="5" orient="auto-start-reverse">
                        <path d="M0,0 L8,4 L0,8 Q2.4,4 0,0 Z" class="fill-current text-gray-400 dark:text-gray-500" />
                    </marker>
                </defs>
                <rect x="0" y="0" width="${layout.width}" height="${layout.height}" fill="transparent" style="pointer-events:all"></rect>`;

            let svgEdges = '';
            let svgNodes = '';

            const pathBetween = (fromNode, toNode) => {
                const iconRadius = 8;
                const arrowPad = 3;
                const fromCx = fromNode.x + fromNode.width / 2;
                const fromCy = fromNode.y + fromNode.height / 2;
                const toCx = toNode.x + toNode.width / 2;
                const toCy = toNode.y + toNode.height / 2;
                let sx, sy, tx, ty;
                if (isVerticalLayout) {
                    sx = fromCx; sy = fromCy + iconRadius + arrowPad;
                    tx = toCx; ty = toCy - iconRadius - arrowPad;
                    const curveY = sy + (ty - sy) * 0.5;
                    return `M ${sx} ${sy} C ${sx} ${curveY}, ${tx} ${curveY}, ${tx} ${ty}`;
                } else {
                    sx = fromCx + iconRadius + arrowPad; sy = fromCy;
                    tx = toCx - iconRadius - arrowPad; ty = toCy;
                    const curveX = sx + (tx - sx) * 0.5;
                    return `M ${sx} ${sy} C ${curveX} ${sy}, ${curveX} ${ty}, ${tx} ${ty}`;
                }
            };

            layout.edges.forEach(edge => {
                const d = pathBetween(edge.from, edge.to);
                svgEdges += `<path d="${d}" class="edge-path-halo" style="stroke-width: 5px; stroke-opacity: 0.8;"></path>`;
                svgEdges += `<path d="${d}" class="edge-path" style="stroke: var(--border-secondary); stroke-width: 1.5px;" marker-end="url(#pipeline-def-arrowhead)"></path>`;
            });

            layout.nodes.forEach(node => {
                const nodeCenterX = node.x + node.width / 2;
                const nodeCenterY = node.y + node.height / 2;
                const label = node.name || 'unnamed';

                svgNodes += `
                    <g class="graph-node graph-node-pipeline-def" data-step-name="${escapeAttribute(label)}">
                         {/* --- CHANGE r="15" to r="12" --- */}
                         <circle cx="${nodeCenterX}" cy="${nodeCenterY}" r="12"
                                 fill="url(#glassyIconGradientPipelineDef)"
                                 stroke="rgba(202, 138, 4, 0.25)" stroke-width="0.5"
                                 filter="url(#softIconShadowPipelineDef)"
                                 opacity="0.95"/>
                        {/* Adjust text y offsets slightly if needed due to smaller circle */}
                        <text x="${nodeCenterX}" y="${nodeCenterY + 35}" text-anchor="middle" class="pipeline-def-node-label">${escapeHtml(label)}</text>
                        <text x="${nodeCenterX}" y="${nodeCenterY + 48}" text-anchor="middle" class="pipeline-def-node-sublabel">Defined</text>
                    </g>`;
            });

            graphContainer.innerHTML = svgContent + svgEdges + svgNodes + `</svg>`;

            const svgElement = graphContainer.querySelector('svg');
            if (svgElement && typeof Panzoom === 'function') {

                if (graphContainer._panzoomInstance) {
                    try { graphContainer._panzoomInstance.destroy(); } catch {}
                    graphContainer._panzoomInstance = null;
                    if (graphContainer._wheelHandler) {
                         try { graphContainer.removeEventListener('wheel', graphContainer._wheelHandler); } catch {}
                         graphContainer._wheelHandler = null;
                    }
                }

                const panzoomInstance = Panzoom(svgElement, {
                    canvas: true,
                    maxScale: 3,
                    minScale: 0.1,
                    contain: 'outside'
                });

                graphContainer._panzoomInstance = panzoomInstance;
                graphContainer._wheelHandler = panzoomInstance.zoomWithWheel;

                graphContainer.addEventListener('wheel', graphContainer._wheelHandler, { passive: false });

                graphContainer.addEventListener('dblclick', (e) => {
                     if (e.target.closest('svg') && graphContainer._panzoomInstance) {
                        graphContainer._panzoomInstance.reset({ animate: true });
                        fitPipelineGraphToView(graphContainer, svgElement, layout);
                     }
                });

                fitPipelineGraphToView(graphContainer, svgElement, layout);

            } else if (typeof Panzoom !== 'function') {
                console.error("Panzoom library not found.");
            }

        } catch (error) {
            graphContainer.innerHTML = '<p class="text-sm text-red-500">Unable to render dependency graph.</p>';
            console.error('SVG Graph render error', error);
        }
    }

    function fitPipelineGraphToView(container, element, layout) {
        if (!container || !element || !layout || !container._panzoomInstance) return;
        const parentRect = container.getBoundingClientRect();
        const contentWidth = layout.width;
        const contentHeight = layout.height;

        if (!contentWidth || !contentHeight || !parentRect.width || !parentRect.height || parentRect.width <=0 || parentRect.height <= 0) return;

        container._panzoomInstance.reset({ animate: false });

        const fitPadding = 20;
        const availableWidth = Math.max(1, parentRect.width - fitPadding * 2);
        const availableHeight = Math.max(1, parentRect.height - fitPadding * 2);

        const contentW = Math.max(1, contentWidth);
        const contentH = Math.max(1, contentHeight);

        const fitScaleX = availableWidth / contentW;
        const fitScaleY = availableHeight / contentH;
        const fitScale = Math.min(fitScaleX, fitScaleY);
        const preferredScale = 0.1;

        const scale = Math.min(fitScale, preferredScale);
        const x = fitPadding;
        const y = fitPadding;

        container._panzoomInstance.zoom(scale, { animate: false });
        container._panzoomInstance.pan(x, y, { animate: false });
    }

    async function renderTriggers(pipelineId) {
        if (!DOM['pipeline-triggers']) return;
        const triggers = await getTriggersForPipeline(pipelineId);
        if (!triggers.length) {
            DOM['pipeline-triggers'].innerHTML = '<p class="text-sm text-[var(--text-secondary)]">No trigger manifests reference this pipeline.</p>';
            return;
        }

        const renderField = (label, value) => `
            <div class="flex items-start justify-between gap-3 text-xs">
                <span class="text-[var(--text-secondary)]">${escapeHtml(label)}</span>
                <span class="font-mono text-sm text-[var(--text-primary)] text-right">${escapeHtml(value || '—')}</span>
            </div>
        `;

        DOM['pipeline-triggers'].innerHTML = triggers.map(item => {
            const record = (item && typeof item === 'object' && 'trigger' in item)
                ? item
                : { trigger: item };
            const trigger = record.trigger || {};
            const repoSlug = record.repoSlug
                || (record.repoOwner && record.repoName ? `${record.repoOwner}/${record.repoName}` : 'config repo');
            const eventValue = trigger.on || 'event';
            const environmentValue = trigger.environment || 'default';
            const fields = [
                { label: 'on:', value: eventValue },
            ];

            if (Array.isArray(trigger.branches) && trigger.branches.length) {
                fields.push({ label: 'branches:', value: trigger.branches.join(', ') });
            } else if (Array.isArray(trigger.skip_branches) && trigger.skip_branches.length) {
                fields.push({ label: 'skip_branches:', value: trigger.skip_branches.join(', ') });
            } else {
                fields.push({ label: 'branches:', value: 'all branches' });
            }

            if (Array.isArray(trigger.tags) && trigger.tags.length) {
                fields.push({ label: 'tags:', value: trigger.tags.join(', ') });
            }

            fields.push({ label: 'environment:', value: environmentValue });

            const fieldsMarkup = fields.map(({ label, value }) => renderField(label, value)).join('');

            return `
                <div class="pipelines-trigger-card space-y-2">
                    <div>
                        <p class="text-xs uppercase tracking-wide text-[var(--text-secondary)]">Repo</p>
                        <p class="font-mono text-sm text-[var(--text-primary)] break-words">${escapeHtml(repoSlug)}</p>
                    </div>
                    <div class="grid gap-1">
                        ${fieldsMarkup}
                    </div>
                </div>`;
        }).join('');
    }

    function formatTriggerEventInfo(id, options = {}) {
        const opts = typeof options === 'object' && options !== null ? options : {};
        const fallback = opts.fallback || 'N/A';
        if (id === undefined || id === null) {
            return { text: fallback, full: fallback };
        }
        const raw = String(id).trim();
        if (!raw) {
            return { text: fallback, full: fallback };
        }
        const limit = Number(opts.limit);
        const hasLimit = Number.isFinite(limit) && limit > 0;
        const text = hasLimit && raw.length > limit ? `${raw.slice(0, limit)}…` : raw;
        return { text, full: raw };
    }

    function formatTriggerEventCardDisplay(id, options = {}) {
        const info = formatTriggerEventInfo(id, options);
        if (info.full === 'N/A') {
            return {
                display: info.text,
                title: escapeAttribute(info.full),
            };
        }

        const raw = info.full;
        const display = raw.length > 8 ? raw.slice(0, 8) + '...' : raw;
        return {
            display: display,
            title: escapeAttribute(raw),
        };
    }

    function escapeAttribute(value) {
        if (value === null || value === undefined) return '';
        return String(value)
            .replace(/&/g, '&amp;')
            .replace(/"/g, '&quot;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;');
    }
    async function renderRecentRuns(pipelineId) {
        if (!DOM['pipeline-recent-runs']) return;
        const runs = await getRecentRunsForPipeline(pipelineId);
        if (!runs.length) {
            DOM['pipeline-recent-runs'].innerHTML = '<p class="text-sm text-[var(--text-secondary)]">No recent runs found.</p>';
            return;
        }

        DOM['pipeline-recent-runs'].innerHTML = runs.slice(0, 5).map(run => {
            const timeAgo = formatRelativeTime(run.started_at || run.startedAt);
            const repoName = run.git_repo_name || 'N/A';
            const branch = run.git_ref ? run.git_ref.replace('refs/heads/', '') : 'manual';
            const runUrl = `#/pipelineruns/recent/${run.run_id}`;
            const shortRunId = (run.run_id || '...').slice(0, 8);
            const triggerCard = formatTriggerEventCardDisplay(run.trigger_event_id, { fallback: 'Manual/Unknown' });

            return `
                <a href="${runUrl}" class="pipelines-run-row block" title="Open run ${shortRunId}">
                    <div class="flex items-baseline justify-between gap-2 mb-1">
                        
                        
                        <div class="flex items-center gap-2 font-mono text-sm text-[var(--text-primary)] truncate" title="Run ID: ${escapeAttribute(run.run_id || '')}">
                            <svg class="h-3.5 w-3.5 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H5v-2H3v-2H1v-4a6 6 0 016-6h1.5" /></svg>
                            <span>${escapeHtml(shortRunId)}</span>
                        </div>
                        
                        <span class="text-xs text-[var(--text-secondary)] flex-shrink-0">${timeAgo}</span>
                    </div>
                    
                    
                    <div class="text-xs text-[var(--text-secondary)] font-mono truncate" title="Repository: ${escapeAttribute(repoName)}">
                         <svg class="inline-block h-3 w-3 mr-1 -mt-0.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" /></svg>
                        ${escapeHtml(repoName)}
                    </div>

                    
                    <div class="text-xs text-[var(--text-link)] font-mono truncate mt-0.5" title="Branch: ${escapeAttribute(branch)}">
                        <svg class="inline-block h-3 w-3 mr-1 -mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                        ${escapeHtml(branch)}
                    </div>

                    
                    <div class="text-xs text-[var(--text-secondary)] font-mono truncate mt-0.5" title="Trigger Event ID: ${triggerCard.title}">
                         <svg class="inline-block h-3 w-3 mr-1 -mt-0.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7a1 1 0 011-1h3.586a1 1 0 01.707.293l6.414 6.414a1 1 0 010 1.414l-4.586 4.586a1 1 0 01-1.414 0L7.293 13.707A1 1 0 017 13V9a1 1 0 011-1z" /></svg>
                        ${escapeHtml(triggerCard.display)}
                    </div>
                </a>`;
        }).join('');
    }

    function enterEditMode() {
        if (!state.selectedId || state.isEditing) return;
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
        const expectedHash = buildPipelineHash(state.selectedId, true);
        if (window.location.hash !== expectedHash) {
            try {
                history.replaceState(null, '', expectedHash);
            } catch { window.location.hash = expectedHash; }
        }
        handleValidation();
        updateLineNumbers();
        if (DOM['pipeline-yaml-editor']) {
            DOM['pipeline-yaml-editor'].focus();
        }
        bindBeforeUnload();
        ensureEditorSuggestionPanel();
        updateEditorSuggestions();
        preloadAutocompleteMetadata().catch(() => {});
    }

    function exitEditMode() {
        if (!state.isEditing) return;

        state.isEditing = false;
        if (DOM['yaml-view-actions']) DOM['yaml-view-actions'].classList.remove('hidden');
        if (DOM['yaml-edit-actions']) DOM['yaml-edit-actions'].classList.add('hidden');
        if (DOM['pipeline-yaml-content']) DOM['pipeline-yaml-content'].classList.remove('hidden');
        if (DOM['editor-container']) DOM['editor-container'].classList.add('hidden');
        if (DOM['validation-status']) DOM['validation-status'].classList.add('hidden');
        if (state.selectedId) {
        const expectedHash = buildPipelineHash(state.selectedId, false);
            if (window.location.hash !== expectedHash) {
                try {
                    history.replaceState(null, '', expectedHash);
                } catch { window.location.hash = expectedHash; }
            }
        }
        unbindBeforeUnload();
        hideEditorSuggestions();
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

    function editingPreventNavigation(targetHash) {
        if (!state.isEditing) return false;
        notifyEditingLock();
        restoreEditingRoute(targetHash);
        return true;
    }

    function restoreEditingRoute(targetHash) {
        const currentHash = normalizeHashForCompare(typeof targetHash === 'string' ? targetHash : window.location.hash);
        const expectedHash = normalizeHashForCompare(state.selectedId ? buildPipelineHash(state.selectedId, true) : '#/pipelines');
        if (currentHash === expectedHash) {
            return;
        }
        suppressNextRouteOnce();
        const desiredHash = state.selectedId ? buildPipelineHash(state.selectedId, true) : '#/pipelines';
        try {
            const url = new URL(window.location.href);
            url.hash = desiredHash.slice(1);
            history.replaceState(null, '', url.toString());
        } catch {
            window.location.hash = desiredHash;
        }
    }

    function suppressNextRouteOnce() {
        if (!context || !context.state) return;
        const appState = context.state;
        appState._suppressNextRoute = true;
        if (appState._suppressRouteTimeout) {
            try { clearTimeout(appState._suppressRouteTimeout); } catch {}
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
        let hash = rawHash;
        if (!hash) {
            hash = '#';
        }
        if (typeof hash !== 'string') {
            hash = String(hash || '');
        }
        hash = hash.trim();
        if (!hash.startsWith('#')) {
            hash = hash.startsWith('/') ? `#${hash.slice(1)}` : `#${hash}`;
        }
        hash = hash.replace(/^#\/+/, '#/');
        hash = hash.replace(/\/+$/g, '');
        return hash || '#';
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

    function ensureEditorSuggestionPanel() {
        if (DOM['pipeline-editor-suggestions'] || !DOM['editor-container']) return;
        const panel = document.createElement('div');
        panel.id = 'pipeline-editor-suggestions';
        panel.className = 'pipeline-editor-suggestions hidden';
        panel.innerHTML = `
            <div class="pipeline-editor-suggestions__header">
                <span id="pipeline-editor-suggestions-title">Suggestions</span>
                <span class="pipeline-editor-suggestions__hint">Click to insert</span>
            </div>
            <div id="pipeline-editor-suggestions-body" class="pipeline-editor-suggestions__body"></div>`;
        panel.addEventListener('click', handleSuggestionClick);
        DOM['editor-container'].appendChild(panel);
        DOM['pipeline-editor-suggestions'] = panel;
        DOM['pipeline-editor-suggestions-title'] = panel.querySelector('#pipeline-editor-suggestions-title');
        DOM['pipeline-editor-suggestions-body'] = panel.querySelector('#pipeline-editor-suggestions-body');
    }

    function hideEditorSuggestions() {
        state.editorSuggestionContext = null;
        if (DOM['pipeline-editor-suggestions']) {
            DOM['pipeline-editor-suggestions'].classList.add('hidden');
        }
    }

    function renderEditorSuggestions(payload) {
        ensureEditorSuggestionPanel();
        const panel = DOM['pipeline-editor-suggestions'];
        const titleEl = DOM['pipeline-editor-suggestions-title'];
        const bodyEl = DOM['pipeline-editor-suggestions-body'];
        if (!panel || !titleEl || !bodyEl) return;

        if (payload.loading) {
            titleEl.textContent = payload.title || 'Suggestions';
            bodyEl.innerHTML = '<div class="pipeline-editor-suggestions__empty">Loading suggestions…</div>';
            panel.classList.remove('hidden');
            return;
        }

        if (!payload.items || !payload.items.length) {
            hideEditorSuggestions();
            return;
        }

        titleEl.textContent = payload.title || 'Suggestions';
        bodyEl.innerHTML = payload.items.map(item => `
            <button type="button" class="pipeline-editor-suggestions__item" data-suggestion-value="${escapeAttribute(item.value)}">
                <span class="pipeline-editor-suggestions__item-label">${escapeHtml(item.label || item.value)}</span>
                ${item.hint ? `<span class="pipeline-editor-suggestions__item-hint">${escapeHtml(item.hint)}</span>` : ''}
            </button>
        `).join('');
        panel.classList.remove('hidden');
    }

    function handleSuggestionClick(event) {
        const target = event.target.closest('[data-suggestion-value]');
        if (!target) return;
        const value = target.getAttribute('data-suggestion-value');
        if (!value) return;
        applyEditorSuggestion(value);
    }

    function applyEditorSuggestion(value) {
        if (!state.editorSuggestionContext || !DOM['pipeline-yaml-editor']) return;
        const textarea = DOM['pipeline-yaml-editor'];
        const textLength = textarea.value.length;
        const rangeStart = Math.max(0, Math.min(state.editorSuggestionContext.rangeStart ?? textarea.selectionStart, textLength));
        const rangeEnd = Math.max(rangeStart, Math.min(state.editorSuggestionContext.rangeEnd ?? textarea.selectionEnd, textLength));
        const before = textarea.value.slice(0, rangeStart);
        const after = textarea.value.slice(rangeEnd);
        let suffix = state.editorSuggestionContext.insertSuffix || '';
        if (suffix && after.trimStart().startsWith(':')) {
            suffix = '';
        }
        textarea.value = before + value + suffix + after;
        const caret = rangeStart + value.length + suffix.length;
        textarea.selectionStart = textarea.selectionEnd = caret;
        textarea.focus();
        state.editorSuggestionContext = null;
        handleValidation();
        updateLineNumbers();
        updateEditorSuggestions();
    }

    function updateEditorSuggestions() {
        if (!state.isEditing || !DOM['pipeline-yaml-editor']) {
            hideEditorSuggestions();
            return;
        }
        ensureEditorSuggestionPanel();
        const textarea = DOM['pipeline-yaml-editor'];
        const text = textarea.value || '';
        const selectionStart = Math.min(textarea.selectionStart, textarea.selectionEnd);
        const selectionEnd = Math.max(textarea.selectionStart, textarea.selectionEnd);
        const contextInfo = detectSuggestionContext(text, selectionStart, selectionEnd);
        if (!contextInfo) {
            hideEditorSuggestions();
            return;
        }

        const requiresMetadata = contextInfo.type === 'secrets'
            || contextInfo.type === 'environment'
            || contextInfo.type === 'reusable-step';

        if (requiresMetadata) {
            const poolSize = contextInfo.type === 'secrets'
                ? state.autocomplete.secrets.length
                : contextInfo.type === 'environment'
                    ? state.autocomplete.environments.length
                    : state.autocomplete.reusableSteps.length;
            if (!poolSize) {
                preloadAutocompleteMetadata().catch(() => {});
                state.editorSuggestionContext = contextInfo;
                renderEditorSuggestions({ title: contextInfo.title, loading: true });
                return;
            }
        }

        const items = buildSuggestionItems(contextInfo, text);
        if (!items.length) {
            if (requiresMetadata && state.autocomplete.isLoading) {
                state.editorSuggestionContext = contextInfo;
                renderEditorSuggestions({ title: contextInfo.title, loading: true });
            } else {
                hideEditorSuggestions();
            }
            return;
        }

        state.editorSuggestionContext = contextInfo;
        renderEditorSuggestions({ title: contextInfo.title, items });
    }

    function detectSuggestionContext(text, selectionStart, selectionEnd) {
        if (typeof text !== 'string') return null;
        const lineInfo = getCurrentLineInfo(text, selectionStart);
        if (!lineInfo) return null;
        const beforeLine = text.slice(0, lineInfo.start);
        const trimmedLine = lineInfo.line.trim();
        if (trimmedLine.startsWith('#')) {
            return null;
        }

        const lineBeforeCaret = lineInfo.line.slice(0, lineInfo.column);

        const reusable = detectReusableStepContext(lineInfo, lineBeforeCaret, selectionEnd);
        if (reusable) {
            return reusable;
        }

        const inlineDepends = detectInlineKeyContext(lineInfo, selectionEnd, 'depends_on');
        if (inlineDepends) {
            return { ...inlineDepends, type: 'depends_on', title: 'Depends on' };
        }

        const dependsList = detectListEntryContext(lineInfo, selectionEnd, beforeLine, 'depends_on');
        if (dependsList) {
            return { ...dependsList, type: 'depends_on', title: 'Depends on' };
        }

        const secretsList = detectListEntryContext(lineInfo, selectionEnd, beforeLine, 'secrets');
        if (secretsList) {
            return { ...secretsList, type: 'secrets', title: 'Secrets', insertSuffix: '' };
        }

        const environmentContext = detectEnvironmentContext(lineInfo, selectionEnd, beforeLine);
        if (environmentContext) {
            return environmentContext;
        }

        return null;
    }

    function detectReusableStepContext(lineInfo, lineBeforeCaret, selectionEnd) {
        const match = lineBeforeCaret.match(/step:\s*"?([A-Za-z0-9_\/-]*)$/i);
        if (!match) return null;
        const segment = match[0];
        const prefix = (match[1] || '').trim();
        const matchIndex = lineBeforeCaret.lastIndexOf(segment);
        if (matchIndex === -1) return null;
        return {
            type: 'reusable-step',
            title: 'Reusable steps',
            prefix,
            rangeStart: lineInfo.start + matchIndex,
            rangeEnd: selectionEnd,
            insertSuffix: '',
        };
    }

    function detectEnvironmentContext(lineInfo, selectionEnd, beforeLine) {
        const parent = findParentBlock(beforeLine, ['environment'], lineInfo.indent);
        if (parent !== 'environment') {
            return null;
        }
        const colonIndex = lineInfo.line.indexOf(':', lineInfo.indent);
        const hasColon = colonIndex !== -1;
        const valueEnd = hasColon ? Math.min(colonIndex, lineInfo.column) : lineInfo.column;
        const rawPrefix = lineInfo.line.slice(lineInfo.indent, valueEnd);
        const prefix = rawPrefix.trim();
        const computedRangeEnd = hasColon && colonIndex < selectionEnd ? lineInfo.start + colonIndex : selectionEnd;
        const safeRangeEnd = Math.max(lineInfo.start + lineInfo.indent, computedRangeEnd);
        return {
            type: 'environment',
            title: 'Environment keys',
            prefix,
            rangeStart: lineInfo.start + lineInfo.indent,
            rangeEnd: safeRangeEnd,
            insertSuffix: hasColon ? '' : ': ',
        };
    }

    function detectInlineKeyContext(lineInfo, selectionEnd, keyName) {
        const colonIndex = lineInfo.line.indexOf(':');
        if (colonIndex === -1) return null;
        const key = lineInfo.line.slice(0, colonIndex).trim();
        if (key !== keyName || lineInfo.column <= colonIndex) return null;
        const valueStartLocal = colonIndex + 1;
        const whitespaceMatch = lineInfo.line.slice(valueStartLocal).match(/^\s*/);
        const valueStart = valueStartLocal + (whitespaceMatch ? whitespaceMatch[0].length : 0);
        const rangeStart = lineInfo.start + valueStart;
        const safeEnd = Math.max(rangeStart, selectionEnd);
        return {
            prefix: lineInfo.line.slice(valueStart, lineInfo.column).trim(),
            rangeStart,
            rangeEnd: safeEnd,
            insertSuffix: '',
        };
    }

    function detectListEntryContext(lineInfo, selectionEnd, beforeLine, keyName) {
        const trimmed = lineInfo.line.trimStart();
        if (!trimmed.startsWith('-')) return null;
        const parent = findParentBlock(beforeLine, [keyName], lineInfo.indent);
        if (parent !== keyName) return null;
        const dashMatch = lineInfo.line.match(/^(\s*-\s*)/);
        const valueStart = dashMatch ? dashMatch[0].length : lineInfo.indent;
        const rangeStart = lineInfo.start + valueStart;
        const safeEnd = Math.max(rangeStart, selectionEnd);
        return {
            prefix: lineInfo.line.slice(valueStart, lineInfo.column).trim(),
            rangeStart,
            rangeEnd: safeEnd,
            insertSuffix: '',
        };
    }

    function findParentBlock(beforeText, targetKeys, currentIndent) {
        if (!beforeText) return null;
        const lines = beforeText.split('\n');
        for (let i = lines.length - 1; i >= 0; i--) {
            const rawLine = lines[i];
            if (!rawLine) continue;
            const trimmed = rawLine.trim();
            if (!trimmed || trimmed.startsWith('#') || trimmed.startsWith('-')) {
                continue;
            }
            if (!trimmed.endsWith(':')) {
                continue;
            }
            const indent = rawLine.match(/^\s*/)[0].length;
            if (indent >= currentIndent) {
                continue;
            }
            const key = trimmed.slice(0, trimmed.indexOf(':')).trim();
            if (targetKeys.includes(key)) {
                return key;
            }
            return null;
        }
        return null;
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

    function buildSuggestionItems(contextInfo, text) {
        const prefix = contextInfo.prefix || '';
        if (contextInfo.type === 'depends_on') {
            const steps = collectStepNamesFromYamlText(text).map(name => ({ value: name, label: name }));
            return filterSuggestionPool(steps, prefix);
        }
        if (contextInfo.type === 'secrets') {
            const secrets = state.autocomplete.secrets.map(name => ({ value: name, label: name }));
            return filterSuggestionPool(secrets, prefix);
        }
        if (contextInfo.type === 'environment') {
            const envs = state.autocomplete.environments.map(name => ({ value: name, label: name }));
            return filterSuggestionPool(envs, prefix);
        }
        if (contextInfo.type === 'reusable-step') {
            const steps = state.autocomplete.reusableSteps.map(name => ({
                value: `step:${name}`,
                label: `step:${name}`,
                matchValue: name.toLowerCase(),
            }));
            return filterSuggestionPool(steps, prefix);
        }
        return [];
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

    function collectStepNamesFromYamlText(text) {
        const names = new Set();
        const parsed = parseYamlSafely(text);
        if (parsed && Array.isArray(parsed.steps)) {
            parsed.steps.forEach(step => {
                const candidate = step && (step.name || step.task_name);
                if (candidate) {
                    names.add(String(candidate));
                }
            });
        }
        if (!names.size) {
            const regex = /name:\s*([^\n]+)/gi;
            let match;
            while ((match = regex.exec(text))) {
                const raw = (match[1] || '').trim().replace(/^['"]|['"]$/g, '');
                if (raw) {
                    names.add(raw);
                }
            }
        }
        return Array.from(names);
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
        notifySidebarTreeUpdate();
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
        if (state.isEditing) {
            notifyEditingLock();
            event.preventDefault();
            event.stopPropagation();
            return;
        }
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
            window.location.hash = buildFolderPathHash(folderKey);
            event.preventDefault();
            event.stopPropagation();
            return;
        }

        const row = event.target.closest('[data-pipeline-id]');
        if (row) {
            const pipelineId = row.getAttribute('data-pipeline-id');
            if (pipelineId) {
                window.location.hash = `#/pipelines/${pipelineId}`;
            }
        }

        const pipelineCard = event.target.closest('[data-pipeline-id]');
        if (pipelineCard) {
            const pipelineId = pipelineCard.getAttribute('data-pipeline-id');
            if (pipelineId) {
                window.location.hash = buildPipelineHash(pipelineId);
            }
            event.preventDefault();
            event.stopPropagation();
            return;
        }
    }

    function handleListKeydown(event) {
        if (event.defaultPrevented) return;
        if (state.isEditing) {
            notifyEditingLock();
            event.preventDefault();
            return;
        }
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
        window.location.hash = buildPipelineHash(identifier, true);
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
            window.location.hash = buildFolderPathHash(state.activeFolderKey);
        } else {
            renderPipelineList();
            updateCounts();
            notifySidebarTreeUpdate();
        }
        closeDeleteModal();
        showToast('Pipeline deleted.', 'success');
    }

    function validatePipelineYaml(yamlString) {
        if (!window.jsyaml) return { error: 'YAML parser is unavailable.' };

        const knownPipelineKeys = new Set(['name', 'version', 'description', 'container_image', 'display_options', 'working_directory', 'environment', 'steps', 'timeout', 'llm_content_sharing', 'llm_output_sharing', 'llm_content_ignore']);
        const knownStepKeys = new Set(['name', 'include', 'sync', 'image', 'secrets', 'volumes', 'environment', 'tasks', 'condition', 'goal', 'script', 'depends_on', 'ignore_failure', 'llm_output_sharing']);
        const knownTaskKeys = new Set(['name', 'goal', 'script', 'depends_on', 'ignore_failure', 'llm_output_sharing']);
        const knownDisplayOptionsKeys = new Set(['github_view']);

        function findUnknownKeys(obj, knownKeys, path = '') {
            if (!obj || typeof obj !== 'object' || Array.isArray(obj)) {
                return []; // Only check keys of objects
            }
            let unknown = [];
            for (const key in obj) {
                if (!knownKeys.has(key)) {
                    unknown.push(path ? `${path}.${key}` : key);
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
            if (!pipeline) return { error: 'YAML is empty or invalid.' };
            if (typeof pipeline !== 'object') return { error: 'YAML root must be an object.' }; // Added check

            const unknownKeys = checkAllKeys(pipeline);
            if (unknownKeys.length > 0) {
                return { error: `Validation Error: Unknown field(s) found: ${unknownKeys.join(', ')}` };
            }
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
                        matches.push({
                            repoOwner: owner,
                            repoName: name,
                            repoSlug: `${owner}/${name}`,
                            trigger,
                        });
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
