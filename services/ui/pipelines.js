(function (global) {
    const PIPELINES_CARD_ICON_SVG = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M7 7h10" /><path d="M7 17h10" /><circle cx="7" cy="7" r="1.6" fill="currentColor" stroke="none" /><circle cx="17" cy="7" r="1.6" fill="currentColor" stroke="none" /><circle cx="7" cy="17" r="1.6" fill="currentColor" stroke="none" /><circle cx="17" cy="17" r="1.6" fill="currentColor" stroke="none" /><circle cx="12" cy="12" r="1.6" fill="currentColor" stroke="none" /></svg>`;
    const state = {
        pipelines: [],
        pipelineSources: new Map(),
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
        cloneContext: null,
        autocomplete: {
            secrets: [],
            environments: [],
            reusableSteps: [],
            fetchedAt: 0,
            isLoading: false,
            loadingPromise: null,
        },
        editorSuggestionContext: null,
        editorSuggestionItems: [],
        editorSuggestionIndex: -1,
        editorValidationErrors: [],
        beforeUnloadHandler: null,
        environmentSuggestions: [],
        environmentSuggestionCache: new Map(),
        environmentSuggestionPromise: null,
        environmentSuggestionLoadedAt: 0,
        environmentSuggestionActiveKey: null,
        editorPanelMode: null,
        editorPanelContext: null,
        lastEditorSelection: null,
        editorSuggestionPositionHandler: null,
        editorSuggestionAnimationFrame: null,
        suggestionPanelFloating: false,
        suggestionPanelOriginalParent: null,
        suggestionPanelOriginalNextSibling: null,
        suggestionPanelOverlayContainer: null,
    };

    const DOM = {};
    let context = null;
    let pipelineRunsModule = null;
    let textareaCaretMirror = null;

    const TOAST_TIMEOUT = 4000;
    const MAX_RECENT_RUNS = 5;
    const MAX_VISIBLE_TRIGGER_CARDS = 5;
    const AUTOCOMPLETE_REFRESH_INTERVAL = 5 * 60 * 1000;
    const PIPELINE_DIRECTIVES = [
        { key: 'name', hint: 'Pipeline display name' },
        { key: 'version', hint: 'Pipeline schema version' },
        { key: 'description', hint: 'Human readable summary' },
        { key: 'container_image', hint: 'Default container image' },
        { key: 'working_directory', hint: 'Default working directory' },
        { key: 'environment', hint: 'Global environment variables' },
        { key: 'steps', hint: 'List pipeline steps' },
        { key: 'timeout', hint: 'Pipeline timeout' },
        { key: 'llm_output_sharing', hint: 'Share LLM outputs across steps' },
        { key: 'llm_content_sharing', hint: 'Share LLM prompts across steps' },
        { key: 'llm_content_ignore', hint: 'Paths excluded from LLM context' },
        { key: 'display_options', hint: 'UI rendering preferences' },
    ];
    const STEP_DIRECTIVES = [
        { key: 'name', hint: 'Step name' },
        { key: 'include', hint: 'Include reusable step' },
        { key: 'sync', hint: 'Run step synchronously' },
        { key: 'image', hint: 'Override container image' },
        { key: 'secrets', hint: 'Step secrets' },
        { key: 'volumes', hint: 'Step volumes' },
        { key: 'environment', hint: 'Step environment variables' },
        { key: 'tasks', hint: 'Nested task list' },
        { key: 'condition', hint: 'Conditional execution' },
        { key: 'goal', hint: 'LLM goal prompt' },
        { key: 'script', hint: 'Shell script body' },
        { key: 'depends_on', hint: 'Upstream steps' },
        { key: 'ignore_failure', hint: 'Ignore failures' },
        { key: 'llm_output_sharing', hint: 'Share step LLM output' },
    ];
    const TASK_DIRECTIVES = [
        { key: 'name', hint: 'Task name' },
        { key: 'goal', hint: 'Task goal prompt' },
        { key: 'script', hint: 'Task script body' },
        { key: 'depends_on', hint: 'Dependent tasks' },
        { key: 'ignore_failure', hint: 'Ignore task errors' },
        { key: 'llm_output_sharing', hint: 'Share task LLM output' },
    ];
    const PIPELINE_DIRECTIVE_KEYS = PIPELINE_DIRECTIVES.map(item => item.key);
    const STEP_DIRECTIVE_KEYS = STEP_DIRECTIVES.map(item => item.key);
    const TASK_DIRECTIVE_KEYS = TASK_DIRECTIVES.map(item => item.key);
    const DIRECTIVE_VALUE_METADATA = {
        llm_output_sharing: { values: ['true', 'false'], title: 'Boolean value' },
        llm_content_sharing: { values: ['true', 'false'], title: 'Boolean value' },
        ignore_failure: { values: ['true', 'false'], title: 'Boolean value' },
        sync: { values: ['true', 'false'], title: 'Boolean value' },
    };
    const LIST_KEYS_WITH_NAME_TEMPLATE = new Set(['steps', 'tasks']);
    const LIST_KEYS_SIMPLE = new Set([
        'secrets', 'volumes', 'depends_on', 'artifacts', 'environment', 'llm_content_ignore',
    ]);

    function resetPipelineSources() {
        state.pipelineSources = new Map();
    }

    function normalizeSourceValue(raw) {
        if (raw == null) return '';
        const normalized = String(raw).trim().toLowerCase();
        if (!normalized) return '';
        if (normalized === 'git' || normalized === 'config' || normalized === 'config repository' || normalized === 'repository') return 'git';
        if (normalized === 'database' || normalized === 'db') return 'database';
        if (normalized === 'draft') return 'draft';
        if (normalized === 'local' || normalized.includes('repo file') || normalized.includes('repository file')) return 'local';
        return '';
    }

    function normalizeTriggerSourceValue(raw) {
        if (raw == null) return '';
        const value = String(raw).trim().toLowerCase();
        if (!value) return '';
        if (value.includes('git') || value.includes('config repository') || value === 'repository') return 'git';
        if (value.includes('draft')) return 'draft';
        if (value.includes('local') || value.includes('repo file') || value.includes('repository file')) return 'local';
        if (value.includes('database') || value === 'db') return 'database';
        return value;
    }

    function setPipelineSource(identifier, source) {
        if (!identifier) return;
        if (!(state.pipelineSources instanceof Map)) {
            state.pipelineSources = new Map();
        }
        const normalized = normalizeSourceValue(source);
        if (normalized) {
            state.pipelineSources.set(identifier, normalized);
        } else {
            state.pipelineSources.delete(identifier);
        }
    }

    function getSourceLabel(sourceKey) {
        switch (sourceKey) {
            case 'git':
                return 'Git';
            case 'database':
                return 'Database';
            case 'draft':
                return 'Draft';
            case 'local':
                return 'Local';
            default:
                return '';
        }
    }

    function getTriggerSourceLabelForPipeline(sourceKey) {
        switch (String(sourceKey || '').trim().toLowerCase()) {
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

    function resolvePipelineSource(pipelineId, fallback = '') {
        if (state.drafts.has(pipelineId)) {
            return 'Draft';
        }
        const storedKey = normalizeSourceValue(state.pipelineSources?.get(pipelineId));
        if (storedKey) {
            return getSourceLabel(storedKey);
        }
        const fallbackKey = normalizeSourceValue(fallback);
        if (fallbackKey) {
            return getSourceLabel(fallbackKey);
        }
        if (fallback) {
            return fallback;
        }
        return 'Database';
    }

    function updateCachedPipelineSource(pipelineId) {
        if (!pipelineId) return;
        const cached = state.pipelineCache.get(pipelineId);
        if (cached && cached.meta) {
            cached.meta = {
                ...cached.meta,
                source: resolvePipelineSource(pipelineId, cached.meta.source),
            };
            state.pipelineCache.set(pipelineId, cached);
        }
    }

    function isGitManagedPipeline(pipelineId, fallbackSource = '') {
        if (!pipelineId) return false;
        if (state.pipelineSources instanceof Map && state.pipelineSources.has(pipelineId)) {
            return normalizeSourceValue(state.pipelineSources.get(pipelineId)) === 'git';
        }
        const fallbackKey = normalizeSourceValue(fallbackSource);
        if (fallbackKey) {
            return fallbackKey === 'git';
        }
        const label = resolvePipelineSource(pipelineId, fallbackSource);
        return normalizeSourceValue(label) === 'git';
    }

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
            'pipeline-download-btn', 'pipeline-clone-btn', 'pipeline-edit-btn', 'pipeline-save-btn', 'pipeline-cancel-btn',
            'pipeline-yaml-content', 'pipeline-yaml-stage', 'pipeline-yaml-highlight', 'pipeline-yaml-editor', 'editor-container', 'line-numbers',
            'validation-status', 'yaml-view-actions', 'yaml-edit-actions', 'pipeline-graph',
            'pipeline-triggers', 'pipeline-recent-runs', 'pipelines-new-modal', 'pipelines-new-form',
            'pipelines-new-close', 'pipelines-new-cancel', 'pipelines-new-path', 'pipelines-new-name',
            'pipelines-delete-modal', 'pipelines-delete-message', 'pipelines-delete-confirm',
            'pipelines-delete-cancel', 'pipelines-delete-close', 'pipelines-clone-modal', 'pipelines-clone-form',
            'pipelines-clone-close', 'pipelines-clone-cancel', 'pipelines-clone-path', 'pipelines-clone-name', 'pipelines-clone-subtitle', 'pipelines-sync-report', 'pipelines-search-container',
            'pipeline-editor-wrapper', 'pipeline-suggestion-panel', 'pipeline-suggestion-list', 'pipeline-suggestion-empty',
            'pipeline-includes'
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
        if (DOM['pipeline-clone-btn']) {
            DOM['pipeline-clone-btn'].addEventListener('click', () => {
                if (!state.selectedId) return;
                openClonePipelineModal(state.selectedId).catch(error => {
                    console.error('Failed to open clone modal', error);
                    showToast('Unable to open clone modal. Please try again.', 'error');
                });
            });
        }
        if (DOM['pipeline-suggestion-panel']) {
            DOM['pipeline-suggestion-panel'].addEventListener('click', handlePipelineSuggestionClick);
        }

        if (DOM['pipelines-clone-close']) {
            DOM['pipelines-clone-close'].addEventListener('click', closeClonePipelineModal);
        }
        if (DOM['pipelines-clone-cancel']) {
            DOM['pipelines-clone-cancel'].addEventListener('click', closeClonePipelineModal);
        }
        if (DOM['pipelines-clone-form']) {
            DOM['pipelines-clone-form'].addEventListener('submit', handleClonePipeline);
        }

        if (DOM['pipeline-yaml-editor']) {
            DOM['pipeline-yaml-editor'].addEventListener('input', () => {
                handleValidation();
                updateLineNumbers();
                updateEditorSuggestions();
            });
            DOM['pipeline-yaml-editor'].addEventListener('scroll', () => {
                syncPipelineLineNumberScroll();
                syncPipelineHighlightScroll();
                updateInlineSuggestionPosition();
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
                    if (state.editorSuggestionItems.length && state.editorSuggestionContext) {
                        event.preventDefault();
                        const index = state.editorSuggestionIndex >= 0 ? state.editorSuggestionIndex : 0;
                        const item = state.editorSuggestionItems[index] || state.editorSuggestionItems[0];
                        applyEditorSuggestion(item);
                    } else {
                        event.preventDefault();
                        insertEditorIndent(event.target);
                        handleValidation();
                        updateLineNumbers();
                        updateEditorSuggestions();
                    }
                } else if (event.key === 'Enter') {
                    handleEditorEnterKey(event);
                } else if (event.key === 'Escape') {
                    hideEditorSuggestions();
                }
            });
        }

        updatePipelineNewButtonVisibility();
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

    function ingestPipelineListResponse(response) {
        resetPipelineSources();
        if (!Array.isArray(response)) {
            state.pipelines = [];
            notifyIncludePanelDataChanged();
            return;
        }

        const firstItem = response[0];
        if (firstItem && typeof firstItem === 'object' && !Array.isArray(firstItem)) {
            const identifiers = [];
            response.forEach(item => {
                if (!item || typeof item !== 'object') return;
                const identifier = item.id || item.identifier || item.pipeline || '';
                if (!identifier) return;
                identifiers.push(identifier);
                setPipelineSource(identifier, item.source);
                updateCachedPipelineSource(identifier);
            });
            state.pipelines = identifiers;
            notifyIncludePanelDataChanged();
        } else {
            state.pipelines = response
                .map(value => typeof value === 'string' ? value : String(value || ''))
                .filter(Boolean);
            state.pipelines.forEach(updateCachedPipelineSource);
            notifyIncludePanelDataChanged();
        }

        state.pipelines.sort((a, b) => a.localeCompare(b));
        notifyIncludePanelDataChanged();
    }

    async function refreshPipelines(force = false) {
        if (!state.pipelines.length || force) {
            const response = await context.fetchData('/v1/pipelines?include_source=true');
            ingestPipelineListResponse(response);
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
                    context.fetchData('/v1/variables'),
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
                if (state.editorPanelMode === 'include') {
                    renderPipelineIncludeSuggestions();
                } else if (state.editorPanelMode === 'secrets') {
                    renderPipelineSecretSuggestions();
                }
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
                source: resolvePipelineSource(pipelineId, 'Config Repository'),
            };
        }

        return {
            name: parsed.name || fallback.name,
            description: parsed.description || '',
            version: parsed.version || 'latest',
            path: fallback.path,
            source: resolvePipelineSource(pipelineId, parsed.source || 'Config Repository'),
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
        let PADDING_Y = 80;
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
        updatePipelineNewButtonVisibility({ showDetail });
    }

    function isPipelineDetailVisible() {
        if (DOM['pipelines-detail-view']) {
            return !DOM['pipelines-detail-view'].classList.contains('hidden');
        }
        return !!state.selectedId;
    }

    function updatePipelineNewButtonVisibility(options = {}) {
        const button = DOM['pipelines-new-btn'];
        if (!button) return;
        const isSearching = Object.prototype.hasOwnProperty.call(options, 'isSearching')
            ? !!options.isSearching
            : !!state.searchTerm.trim();
        const showDetail = Object.prototype.hasOwnProperty.call(options, 'showDetail')
            ? !!options.showDetail
            : isPipelineDetailVisible();
        button.classList.toggle('hidden', isSearching || showDetail);
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
        const isSearching = !!state.searchTerm.trim();

        if (!hasAnyContent) {
            state.activeFolderKey = '';
            if (DOM['pipelines-empty']) {
                DOM['pipelines-empty'].classList.remove('hidden');
            }
            DOM['pipelines-list-container'].innerHTML = '';
            updatePipelineNewButtonVisibility();
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
        updatePipelineNewButtonVisibility({ isSearching });
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
                        <svg class="h-4 w-4 mr-2 text-[var(--text-secondary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h10"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 17h10"/><circle cx="7" cy="7" r="1.5" fill="currentColor" stroke="none"/><circle cx="17" cy="7" r="1.5" fill="currentColor" stroke="none"/><circle cx="7" cy="17" r="1.5" fill="currentColor" stroke="none"/><circle cx="17" cy="17" r="1.5" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.5" fill="currentColor" stroke="none"/></svg>
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
        const labelSafe = escapeHtml(label);
        const totalPipelines = countPipelinesRecursive(node);
        const childCount = node.children ? node.children.size : 0;

        return `
            <article class="pipeline-folder-card border border-[var(--border-primary)]" data-folder-key="${keyAttr}" tabindex="0" role="button" aria-label="Open folder ${labelSafe}">
                <div class="pipeline-folder-card-header">
                    <span class="pipeline-folder-icon">
                        <svg class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <path d="M3 7h5l2 2h9a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V9a2 2 0 012-2z" />
                        </svg>
                    </span>
                    <h3 class="pipeline-folder-title">${labelSafe}</h3>
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
        const versionLabel = String(meta.version || 'latest');
        const resolvedSourceLabel = resolvePipelineSource(pipeline.id, meta.source || '') || 'Database';
        const sourceKey = normalizeSourceValue(resolvedSourceLabel || meta.source || '');
        const isGitManaged = sourceKey === 'git';
        const deleteTitle = isGitManaged
            ? 'This pipeline is managed via Git. Clone it to customize.'
            : 'Delete pipeline';
        const deleteButtonHtml = isGitManaged
            ? `<button class="pipelines-delete-button" type="button" disabled aria-disabled="true" title="${escapeAttribute(deleteTitle)}">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                                <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                                <path d="M4 7h16" />
                            </svg>
                        </button>`
            : `<button class="pipelines-delete-button" type="button" data-delete-pipeline="${idAttr}" title="${escapeAttribute(deleteTitle)}">
                            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                                <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                                <path d="M4 7h16" />
                            </svg>
                        </button>`;

        return `
            <article class="pipeline-card bg-[var(--bg-primary)] border border-[var(--border-primary)] rounded-lg shadow-sm transition-all duration-200 p-3 flex flex-col" data-pipeline-id="${idAttr}" tabindex="0" role="button" aria-label="Open pipeline ${escapeHtml(rawName)}">
                <div class="pipeline-card-header flex items-start justify-between gap-3">
                    <div class="flex items-start gap-3 min-w-0">
                        <span class="step-logo step-logo--card step-logo--pipelines" aria-hidden="true">${PIPELINES_CARD_ICON_SVG}</span>
                        <div class="pipeline-card-text min-w-0">
                            <h3 class="pipeline-card-title">${name}</h3>
                            <p class="pipeline-card-path">${pathLabel}</p>
                        </div>
                    </div>
                    <div class="pipeline-card-actions">
                        ${deleteButtonHtml}
                    </div>
                </div>
                <p class="pipeline-card-description">${description}</p>
                <div class="pipeline-card-meta">
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">Version</span>
                        <span class="pipeline-card-meta-value">${escapeHtml(versionLabel)}</span>
                    </div>
                    <div class="pipeline-card-meta-row">
                        <span class="pipeline-card-meta-label">Source</span>
                        <span class="pipeline-card-meta-value">${escapeHtml(resolvedSourceLabel)}</span>
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
        const pathLabel = formatPathLabel(meta.path);
        const versionLabel = String(meta.version || 'latest');
        const sourceLabel = resolvePipelineSource(state.selectedId, meta.source || '') || 'Database';
        const normalizedSourceKey = normalizeSourceValue(sourceLabel || meta.source || '');

        if (DOM['pipeline-detail-path']) {
            DOM['pipeline-detail-path'].textContent = pathLabel;
        }
        if (DOM['pipeline-detail-version']) {
            DOM['pipeline-detail-version'].textContent = versionLabel;
        }
        if (DOM['pipeline-detail-source']) {
            DOM['pipeline-detail-source'].textContent = sourceLabel;
        }

        const isGitSource = normalizedSourceKey === 'git';
        if (DOM['pipeline-edit-btn']) {
            DOM['pipeline-edit-btn'].classList.toggle('hidden', isGitSource);
        }
        if (DOM['pipeline-clone-btn']) {
            DOM['pipeline-clone-btn'].classList.toggle('hidden', !isGitSource);
            if (isGitSource) {
                DOM['pipeline-clone-btn'].dataset.pipelineId = state.selectedId || '';
            } else {
                DOM['pipeline-clone-btn'].dataset.pipelineId = '';
            }
        }

        renderYamlView(entry.yaml);

        exitEditMode();
        renderPipelineGraphFromYaml(entry.yaml);
        renderTriggers(state.selectedId);
        renderRecentRuns(state.selectedId);
        renderPipelineIncludes(entry.yaml);
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

            let svgContent = `<svg width="200" height="50" viewBox="0 0 ${layout.width} ${layout.height}" preserveAspectRatio="xMinYMin meet" xmlns="http://www.w3.org/2000/svg" style="width: 100%; height: auto; display: block;">
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

        const shouldScroll = triggers.length > MAX_VISIBLE_TRIGGER_CARDS;
        const html = `<ul class="triggers-pipeline-list ${shouldScroll ? 'triggers-list-scroll' : ''}">${triggers.map(item => {
            const record = (item && typeof item === 'object' && 'trigger' in item) ? item : { trigger: item };
            const trigger = record.trigger || {};
            const repoSlug = record.repoSlug || (record.repoOwner && record.repoName ? `${record.repoOwner}/${record.repoName}` : 'Config repository');
            const eventInfo = formatTriggerEventInfo(trigger.on, { fallback: 'N/A', limit: 32 });
            let branchValue = 'All branches';
            if (Array.isArray(trigger.branches) && trigger.branches.length) {
                branchValue = trigger.branches.join(', ');
            } else if (Array.isArray(trigger.skip_branches) && trigger.skip_branches.length) {
                branchValue = `Skip: ${trigger.skip_branches.join(', ')}`;
            }
            const tagsValue = Array.isArray(trigger.tags) && trigger.tags.length ? trigger.tags.join(', ') : '';
            const environmentValue = trigger.environment || 'default';
            const sourceLabel = getTriggerSourceLabelForPipeline(record.source || 'database');
            const linkSlug = repoSlug.split('/').filter(Boolean).map(encodeURIComponent).join('/');
            const rows = [
                { label: 'Event:', value: eventInfo.full },
                { label: 'Branches:', value: branchValue },
                { label: 'Environment:', value: environmentValue },
                { label: 'Source:', value: sourceLabel },
            ];
            if (tagsValue) {
                rows.splice(3, 0, { label: 'Tags:', value: tagsValue });
            }
            const detailMarkup = rows.map(({ label, value }) => `
                <dt class="triggers-detail-label">${escapeHtml(label)}</dt>
                <dd class="triggers-detail-value">${escapeHtml(value)}</dd>
            `).join('');

            return `
                <li class="triggers-pipeline-item">
                    <a href="#/triggers/${linkSlug}" class="triggers-pipeline-link" title="Open trigger ${escapeAttribute(repoSlug)}">
                        <span class="triggers-pipeline-name">${escapeHtml(repoSlug)}</span>
                        <dl class="triggers-detail-grid triggers-pipeline-details">
                            ${detailMarkup}
                        </dl>
                    </a>
                </li>`;
        }).join('')}</ul>`;

        DOM['pipeline-triggers'].innerHTML = html;

        const listElement = DOM['pipeline-triggers'].querySelector('.triggers-pipeline-list');
        if (!listElement) return;

        if (shouldScroll) {
            listElement.style.removeProperty('max-height');
            adjustPipelineTriggerScrollHeight(listElement);
        } else {
            listElement.style.removeProperty('max-height');
        }
    }

    function formatRunBranchRef(ref) {
        if (!ref) return 'manual';
        return String(ref).replace(/^refs\/heads\//, '');
    }

    function renderPipelineRunRow(run) {
        const runId = run.run_id || run.runId || '';
        const startedAt = run.started_at || run.startedAt;
        const branch = formatRunBranchRef(run.git_ref || run.gitRef);
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
                        <span class="triggers-run-row__pipeline">${escapeHtml(pipelineName)}</span>
                        <span class="triggers-run-row__time">${escapeHtml(timeAgo)}</span>
                    </div>
                    <div class="triggers-run-row__line triggers-run-row__line--status">
                        <span class="triggers-run-row__status">${escapeHtml(statusLabel)}</span>
                    </div>
                    <dl class="triggers-detail-grid triggers-run-details">
                        <dt class="triggers-detail-label">Branch:</dt>
                        <dd class="triggers-detail-value">${escapeHtml(branch)}</dd>
                        <dt class="triggers-detail-label">Run ID:</dt>
                        <dd class="triggers-detail-value">${escapeHtml(shortRunId)}</dd>
                        <dt class="triggers-detail-label">Trigger ID:</dt>
                        <dd class="triggers-detail-value">${escapeHtml(shortTriggerId)}</dd>
                    </dl>
                </div>
            </a>`;
    }

    function adjustPipelineRunsScrollHeight(listEl, attempt = 0) {
        requestAnimationFrame(() => {
            const items = Array.from(listEl.children).slice(0, MAX_RECENT_RUNS);
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
                    setTimeout(() => adjustPipelineRunsScrollHeight(listEl, attempt + 1), 60);
                } else {
                    listEl.style.removeProperty('max-height');
                }
                return;
            }

            listEl.style.maxHeight = `${maxHeight}px`;
        });
    }

    function adjustPipelineTriggerScrollHeight(listEl, attempt = 0) {
        requestAnimationFrame(() => {
            const items = Array.from(listEl.children).slice(0, MAX_VISIBLE_TRIGGER_CARDS);
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
                    setTimeout(() => adjustPipelineTriggerScrollHeight(listEl, attempt + 1), 60);
                } else {
                    listEl.style.removeProperty('max-height');
                }
                return;
            }

            listEl.style.maxHeight = `${maxHeight}px`;
        });
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

        const sortedRuns = runs.slice().sort((a, b) => new Date(b.started_at || b.startedAt || 0) - new Date(a.started_at || a.startedAt || 0));
        const shouldScroll = sortedRuns.length > MAX_RECENT_RUNS;
        const listHtml = `<ul class="triggers-runs-list ${shouldScroll ? 'triggers-runs-scroll' : ''}">
            ${sortedRuns.map(run => `<li class="triggers-runs-item">${renderPipelineRunRow(run)}</li>`).join('')}
        </ul>`;

        DOM['pipeline-recent-runs'].innerHTML = listHtml;

        const listElement = DOM['pipeline-recent-runs'].querySelector('.triggers-runs-list');
        if (!listElement) return;

        if (shouldScroll) {
            listElement.style.removeProperty('max-height');
            adjustPipelineRunsScrollHeight(listElement);
        } else {
            listElement.style.removeProperty('max-height');
        }
    }

    function renderPipelineEnvironmentSuggestions() {
        const panel = DOM['pipeline-suggestion-panel'];
        const list = DOM['pipeline-suggestion-list'];
        const emptyState = DOM['pipeline-suggestion-empty'];
        if (!panel || !list || !emptyState) return;

        if (!state.isEditing || state.editorPanelMode !== 'environment') {
            return;
        }

        updatePipelineSuggestionPanelVisibility();
        setPipelineSuggestionPanelCopy({
            title: 'Environment variables by scope',
            subtitle: 'Compare existing environment variables while editing your pipeline.',
            footnote: 'Click any variable to insert it into the pipeline definition.',
        });

        if (state.environmentSuggestionPromise && !state.environmentSuggestions.length) {
            emptyState.textContent = 'Loading environment variables…';
            emptyState.classList.remove('hidden');
            list.innerHTML = '';
            return;
        }

        if (!state.environmentSuggestions.length) {
            emptyState.textContent = 'No environment variables available yet.';
            emptyState.classList.remove('hidden');
            list.innerHTML = '';
            return;
        }

        emptyState.classList.add('hidden');
        const activeKey = state.environmentSuggestionActiveKey;
        const cards = state.environmentSuggestions.map(entry => {
            const pills = entry.preview.map(name => {
                const valueAttr = escapeAttribute(name);
                return `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-pipeline-suggestion="${valueAttr}">${escapeHtml(name)}</button>`;
            });
            const remaining = entry.count - entry.preview.length;
            if (remaining > 0) {
                pills.push(`<span class="env-suggestion-pill env-suggestion-pill--more">+${remaining} more</span>`);
            }
            const countLabel = `${entry.count} ${entry.count === 1 ? 'variable' : 'variables'}`;
            const activeClass = activeKey && entry.key === activeKey ? ' env-suggestion-item--active' : '';
            return `
                <article class="env-suggestion-item${activeClass}">
                    <div class="env-suggestion-env">
                        <span class="env-suggestion-env-label">${escapeHtml(entry.label)}</span>
                        <span class="env-suggestion-env-count">${escapeHtml(countLabel)}</span>
                    </div>
                    <div class="env-suggestion-variables">${pills.join('')}</div>
                </article>
            `;
        });
        list.innerHTML = cards.join('');
        updateFloatingSuggestionPanelPosition();
    }

    function renderPipelineSecretSuggestions() {
        const panel = DOM['pipeline-suggestion-panel'];
        const list = DOM['pipeline-suggestion-list'];
        const emptyState = DOM['pipeline-suggestion-empty'];
        if (!panel || !list || !emptyState) return;

        if (!state.isEditing || state.editorPanelMode !== 'secrets') {
            return;
        }

        updatePipelineSuggestionPanelVisibility();
        setPipelineSuggestionPanelCopy({
            title: 'Secrets catalogue',
            subtitle: 'Select a secret to insert its name into the editor.',
            footnote: 'Secrets are inserted as plain references; ensure the pipeline has access.',
        });

        const secrets = Array.isArray(state.autocomplete.secrets)
            ? state.autocomplete.secrets.filter(Boolean)
            : [];

        if (!secrets.length) {
            emptyState.textContent = state.autocomplete.isLoading ? 'Loading secrets…' : 'No secrets available yet.';
            emptyState.classList.remove('hidden');
            list.innerHTML = '';
            updateFloatingSuggestionPanelPosition();
            return;
        }

        emptyState.classList.add('hidden');

        const items = secrets.map(name => {
            const encoded = escapeAttribute(name);
            return `
                <article class="env-suggestion-item">
                    <div class="env-suggestion-env">
                        <span class="env-suggestion-env-label">Secret</span>
                        <span class="env-suggestion-env-count">${escapeHtml(name)}</span>
                    </div>
                    <div class="env-suggestion-variables">
                        <button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-pipeline-suggestion="${encoded}">
                            <span>${escapeHtml(name)}</span>
                        </button>
                    </div>
                </article>`;
        });

        list.innerHTML = items.join('');
        updateFloatingSuggestionPanelPosition();
    }

    function renderPipelineIncludeSuggestions() {
        const panel = DOM['pipeline-suggestion-panel'];
        const list = DOM['pipeline-suggestion-list'];
        const emptyState = DOM['pipeline-suggestion-empty'];
        if (!panel || !list || !emptyState) return;

        if (!state.isEditing || state.editorPanelMode !== 'include') {
            return;
        }

        updatePipelineSuggestionPanelVisibility();
        setPipelineSuggestionPanelCopy({
            title: 'Include targets',
            subtitle: 'Reusable steps and pipelines available to include.',
            footnote: 'Click a target to insert it into the include directive.',
        });

        const reusableSteps = Array.isArray(state.autocomplete.reusableSteps)
            ? state.autocomplete.reusableSteps.filter(Boolean)
            : [];
        const pipelines = Array.isArray(state.pipelines)
            ? state.pipelines.filter(Boolean)
            : [];
        const hasData = reusableSteps.length > 0 || pipelines.length > 0;

        if (!hasData) {
            emptyState.textContent = state.autocomplete.isLoading
                ? 'Loading include targets…'
                : 'No include targets available yet.';
            emptyState.classList.remove('hidden');
            list.innerHTML = '';
            return;
        }

        emptyState.classList.add('hidden');
        const sections = [];
        const STEP_LIMIT = 18;
        const PIPELINE_LIMIT = 18;

        if (reusableSteps.length) {
            const buttons = reusableSteps.slice(0, STEP_LIMIT).map(name => {
                const value = `step:${name}`;
                return `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-include-target="${escapeAttribute(value)}">${escapeHtml(value)}</button>`;
            });
            const remaining = reusableSteps.length - Math.min(reusableSteps.length, STEP_LIMIT);
            if (remaining > 0) {
                buttons.push(`<span class="env-suggestion-pill env-suggestion-pill--more">+${remaining} more</span>`);
            }
            sections.push(`
                <article class="env-suggestion-item">
                    <div class="env-suggestion-env">
                        <span class="env-suggestion-env-label">Reusable steps</span>
                        <span class="env-suggestion-env-count">${escapeHtml(`${reusableSteps.length} ${reusableSteps.length === 1 ? 'step' : 'steps'}`)}</span>
                    </div>
                    <div class="env-suggestion-variables">${buttons.join('')}</div>
                </article>`);
        }

        if (pipelines.length) {
            const buttons = pipelines.slice(0, PIPELINE_LIMIT).map(identifier => {
                const value = `pipeline:${identifier}`;
                return `<button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-include-target="${escapeAttribute(value)}">${escapeHtml(value)}</button>`;
            });
            const remaining = pipelines.length - Math.min(pipelines.length, PIPELINE_LIMIT);
            if (remaining > 0) {
                buttons.push(`<span class="env-suggestion-pill env-suggestion-pill--more">+${remaining} more</span>`);
            }
            sections.push(`
                <article class="env-suggestion-item">
                    <div class="env-suggestion-env">
                        <span class="env-suggestion-env-label">Pipelines</span>
                        <span class="env-suggestion-env-count">${escapeHtml(`${pipelines.length} ${pipelines.length === 1 ? 'pipeline' : 'pipelines'}`)}</span>
                    </div>
                    <div class="env-suggestion-variables">${buttons.join('')}</div>
                </article>`);
        }

        list.innerHTML = sections.join('');
        updateFloatingSuggestionPanelPosition();
    }

    function notifyIncludePanelDataChanged() {
        if (state.editorPanelMode === 'include') {
            renderPipelineIncludeSuggestions();
        } else {
            updateFloatingSuggestionPanelPosition();
        }
    }

    function updatePipelineSuggestionPanelVisibility() {
        const shouldShow = state.isEditing && !!state.editorPanelMode;
        if (DOM['pipeline-suggestion-panel']) {
            DOM['pipeline-suggestion-panel'].classList.toggle('hidden', !shouldShow);
        }
    }

    function handlePipelineSuggestionClick(event) {
        const includeButton = event.target.closest('[data-include-target]');
        if (includeButton) {
            const targetValue = includeButton.getAttribute('data-include-target') || '';
            if (!targetValue) return;
            event.preventDefault();
            insertIncludeSuggestionValue(targetValue);
            return;
        }
        const envButton = event.target.closest('[data-pipeline-suggestion]');
        if (envButton) {
            const variableName = envButton.getAttribute('data-pipeline-suggestion') || '';
            if (!variableName) return;
            event.preventDefault();
            insertPipelineSuggestionValue(variableName);
            return;
        }

        const directiveButton = event.target.closest('[data-directive-key]');
        if (directiveButton) {
            const directiveKey = directiveButton.getAttribute('data-directive-key') || '';
            if (!directiveKey) return;
            event.preventDefault();
            insertDirectiveSuggestionValue(directiveKey);
        }
    }

    function insertPipelineSuggestionValue(value) {
        if (!state.isEditing || !DOM['pipeline-yaml-editor']) {
            showToast('Enter edit mode to insert environment variables.', 'info');
            return;
        }
        const textarea = DOM['pipeline-yaml-editor'];
        restoreEditorSelection();
        const start = textarea.selectionStart ?? textarea.value.length;
        const end = textarea.selectionEnd ?? start;
        const before = textarea.value.slice(0, start);
        const after = textarea.value.slice(end);
        const identity = parseEnvironmentVariableIdentity(value);
        const insertion = identity.name || value;
        textarea.value = before + insertion + after;
        const caret = start + insertion.length;
        textarea.selectionStart = textarea.selectionEnd = caret;
        state.lastEditorSelection = { start: caret, end: caret };
        textarea.focus();
        handleValidation();
        updateLineNumbers();
        updateEditorSuggestions();
    }

    function insertDirectiveSuggestionValue(key) {
        if (!state.isEditing || !DOM['pipeline-yaml-editor']) {
            showToast('Enter edit mode to insert directives.', 'info');
            return;
        }
        if (!state.editorSuggestionContext || !/key$/.test(state.editorSuggestionContext.type || '')) {
            const textarea = DOM['pipeline-yaml-editor'];
            if (textarea) {
                textarea.focus();
                restoreEditorSelection();
                updateEditorSuggestions();
            }
        }
        if (!state.editorSuggestionContext || !/key$/.test(state.editorSuggestionContext.type || '')) {
            showToast('Place the cursor where a directive key is expected.', 'info');
            return;
        }
        restoreEditorSelection();
        applyEditorSuggestion({ value: key });
    }

    function insertIncludeSuggestionValue(value) {
        if (!state.isEditing || !DOM['pipeline-yaml-editor']) {
            showToast('Enter edit mode to insert include targets.', 'info');
            return;
        }
        if (!state.editorSuggestionContext || state.editorSuggestionContext.type !== 'include') {
            const textarea = DOM['pipeline-yaml-editor'];
            if (textarea) {
                textarea.focus();
                restoreEditorSelection();
                updateEditorSuggestions();
            }
        }
        if (!state.editorSuggestionContext || state.editorSuggestionContext.type !== 'include') {
            showToast('Place the cursor within an include value to insert a target.', 'info');
            return;
        }
        restoreEditorSelection();
        applyEditorSuggestion({ value });
    }

    function restoreEditorSelection() {
        const textarea = DOM['pipeline-yaml-editor'];
        const selection = state.lastEditorSelection;
        if (!textarea || !selection) return;
        const max = textarea.value.length;
        const start = Math.min(Math.max(selection.start ?? max, 0), max);
        const end = Math.min(Math.max(selection.end ?? start, 0), max);
        textarea.selectionStart = start;
        textarea.selectionEnd = end;
    }

    function setPipelineSuggestionPanelMode(mode, options = {}) {
        const normalized = (mode === 'environment' || mode === 'directive' || mode === 'include' || mode === 'secrets') ? mode : null;
        state.editorPanelMode = normalized;
        state.editorPanelContext = normalized ? { ...(options || {}) } : null;
        updatePipelineSuggestionPanelVisibility();
        if (!normalized) {
            return;
        }
        if (normalized === 'environment') {
            renderPipelineEnvironmentSuggestions();
            if (!state.environmentSuggestions.length && !state.environmentSuggestionPromise) {
                ensureEnvironmentSuggestionData().catch(error => console.error('Failed to preload environment suggestions:', error));
            }
        } else if (normalized === 'secrets') {
            renderPipelineSecretSuggestions();
        } else if (normalized === 'include') {
            renderPipelineIncludeSuggestions();
        } else if (normalized === 'directive') {
            renderPipelineDirectiveSuggestions(options.directiveType || 'pipeline-key');
        }
        updateFloatingSuggestionPanelPosition();
    }

    function setPipelineSuggestionPanelCopy(copy = {}) {
        const titleEl = document.getElementById('pipeline-suggestion-title');
        const subtitleEl = document.getElementById('pipeline-suggestion-subtitle');
        const footnoteEl = document.getElementById('pipeline-suggestion-footnote');
        if (titleEl && copy.title) titleEl.textContent = copy.title;
        if (subtitleEl && copy.subtitle) subtitleEl.textContent = copy.subtitle;
        if (footnoteEl && copy.footnote) footnoteEl.textContent = copy.footnote;
    }

    function renderPipelineDirectiveSuggestions(type) {
        if (!state.isEditing || state.editorPanelMode !== 'directive') {
            return;
        }
        const list = DOM['pipeline-suggestion-list'];
        const emptyState = DOM['pipeline-suggestion-empty'];
        if (!list || !emptyState) return;

        const definitions = getDirectiveDefinitions(type);
        setPipelineSuggestionPanelCopy({
            title: type === 'step-key'
                ? 'Step directives'
                : type === 'task-key'
                    ? 'Task directives'
                    : 'Pipeline directives',
            subtitle: 'Available keys based on the current indentation.',
            footnote: 'Click a directive to insert it at the cursor.',
        });

        if (!definitions.length) {
            emptyState.textContent = 'No directives available here.';
            emptyState.classList.remove('hidden');
            list.innerHTML = '';
            return;
        }

        emptyState.classList.add('hidden');
        list.innerHTML = `
            <div class="env-suggestion-item">
                <div class="env-suggestion-variables">
                    ${definitions.map(def => {
                        const hint = def.hint ? `<span class="env-suggestion-hint">${escapeHtml(def.hint)}</span>` : '';
                        return `
                            <button type="button" class="env-suggestion-pill env-suggestion-pill--action" data-directive-key="${escapeAttribute(def.key)}">
                                <span>${escapeHtml(def.key)}</span>
                                ${hint}
                            </button>`;
                    }).join('')}
                </div>
            </div>`;
        updateFloatingSuggestionPanelPosition();
    }

    function getDirectiveDefinitions(type) {
        switch (type) {
            case 'step-key':
                return STEP_DIRECTIVES;
            case 'task-key':
                return TASK_DIRECTIVES;
            case 'pipeline-key':
            default:
                return PIPELINE_DIRECTIVES;
        }
    }

    function parseEnvironmentVariableIdentity(rawValue) {
        const parts = String(rawValue || '').split('/').filter(Boolean);
        if (parts.length === 3) {
            return { repoOwner: parts[0], repoName: parts[1], name: parts[2] };
        }
        return { repoOwner: null, repoName: null, name: String(rawValue || '') };
    }

    async function ensureEnvironmentSuggestionData(force = false) {
        if (!context || typeof context.fetchData !== 'function') {
            return [];
        }
        if (!force && state.environmentSuggestions.length) {
            return state.environmentSuggestions;
        }
        if (state.environmentSuggestionPromise) {
            return state.environmentSuggestionPromise;
        }

        const promise = (async () => {
            const labels = await fetchEnvironmentScopeLabels();
            const summaries = [];
            for (const label of labels) {
                const variables = await fetchEnvironmentVariablesForLabel(label, force);
                summaries.push({
                    key: buildEnvironmentScopeKey(label),
                    label: label ? `/${label}` : '/ (default)',
                    count: variables.length,
                    preview: variables.slice(0, 5),
                });
            }
            state.environmentSuggestions = summaries;
            state.environmentSuggestionLoadedAt = Date.now();
            return summaries;
        })();

        state.environmentSuggestionPromise = promise;
        try {
            const result = await promise;
            renderPipelineEnvironmentSuggestions();
            return result;
        } catch (error) {
            console.error('Failed to load environment suggestions for pipelines editor:', error);
            state.environmentSuggestions = [];
            renderPipelineEnvironmentSuggestions();
            throw error;
        } finally {
            state.environmentSuggestionPromise = null;
        }
    }

    async function fetchEnvironmentScopeLabels() {
        const labels = new Set(['']);
        if (!context || typeof context.fetchData !== 'function') {
            return Array.from(labels);
        }
        try {
            const response = await context.fetchData('/v1/variables/scopes');
            if (Array.isArray(response)) {
                response.forEach(entry => {
                    const normalized = normalizeEnvironmentScopeLabel(entry);
                    if (normalized !== null && normalized !== undefined) {
                        labels.add(normalized);
                    }
                });
            }
        } catch (error) {
            console.error('Failed to fetch environment scope list for suggestions:', error);
        }
        return Array.from(labels).sort((a, b) => {
            if (a === b) return 0;
            if (a === '') return -1;
            if (b === '') return 1;
            return a.localeCompare(b, undefined, { sensitivity: 'base' });
        });
    }

    function normalizeEnvironmentScopeLabel(entry) {
        if (entry == null) {
            return '';
        }
        if (typeof entry === 'string') {
            return String(entry).trim().replace(/^\/+|\/+$/g, '');
        }
        if (typeof entry === 'object') {
            const value = entry.scope ?? entry.environment ?? entry.env ?? entry.name ?? entry.value ?? '';
            return String(value || '').trim().replace(/^\/+|\/+$/g, '');
        }
        return '';
    }

    function buildEnvironmentScopeKey(label) {
        return `env:${label || ''}`;
    }

    async function fetchEnvironmentVariablesForLabel(label, force = false) {
        const normalized = typeof label === 'string' ? label : '';
        if (!force && state.environmentSuggestionCache instanceof Map && state.environmentSuggestionCache.has(normalized)) {
            return state.environmentSuggestionCache.get(normalized);
        }
        if (!context || typeof context.fetchData !== 'function') {
            return [];
        }

        const baseUrl = normalized ? `/v1/variables?env=${encodeURIComponent(normalized)}` : '/v1/variables';
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
            console.error(`Failed to load environment variables for '/${normalized || ''}':`, error);
            return [];
        }
    }

    function enterEditMode() {
        if (!state.selectedId || state.isEditing) return;
        state.isEditing = true;
        if (DOM['yaml-view-actions']) DOM['yaml-view-actions'].classList.add('hidden');
        if (DOM['yaml-edit-actions']) DOM['yaml-edit-actions'].classList.remove('hidden');
        if (DOM['pipeline-yaml-content']) DOM['pipeline-yaml-content'].classList.add('hidden');
        if (DOM['pipeline-editor-wrapper']) DOM['pipeline-editor-wrapper'].classList.remove('hidden');
        if (DOM['editor-container']) DOM['editor-container'].classList.remove('hidden');
        if (DOM['validation-status']) DOM['validation-status'].classList.remove('hidden');
        enableFloatingSuggestionPanel();
        if (DOM['pipeline-yaml-editor']) {
            const cacheEntry = state.pipelineCache.get(state.selectedId);
            const yamlContent = cacheEntry?.yaml ?? state.currentYaml ?? '';
            DOM['pipeline-yaml-editor'].value = yamlContent;
            DOM['pipeline-yaml-editor'].focus();
            updatePipelineEditorHighlight();
        }
        setPipelineSuggestionPanelMode(null);
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
        ensureEditorSuggestionOverlay();
        updateEditorSuggestions();
        preloadAutocompleteMetadata().catch(() => {});
        ensureEnvironmentSuggestionData().catch(error => console.error('Failed to load environment suggestions:', error));
    }

    function exitEditMode() {
        if (!state.isEditing) return;

        state.isEditing = false;
        hideEditorSuggestions();
        state.editorValidationErrors = [];
        updateLineNumbers();
        if (DOM['yaml-view-actions']) DOM['yaml-view-actions'].classList.remove('hidden');
        if (DOM['yaml-edit-actions']) DOM['yaml-edit-actions'].classList.add('hidden');
        if (DOM['pipeline-yaml-content']) DOM['pipeline-yaml-content'].classList.remove('hidden');
        if (DOM['pipeline-editor-wrapper']) DOM['pipeline-editor-wrapper'].classList.add('hidden');
        if (DOM['editor-container']) DOM['editor-container'].classList.add('hidden');
        if (DOM['validation-status']) DOM['validation-status'].classList.add('hidden');
        disableFloatingSuggestionPanel();
        setPipelineSuggestionPanelMode(null);
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
        const errorMap = new Map();
        (state.editorValidationErrors || []).forEach(err => {
            if (!err || typeof err.line !== 'number') return;
            if (!errorMap.has(err.line)) {
                errorMap.set(err.line, []);
            }
            errorMap.get(err.line).push(err.message);
        });
        const numbersHtml = lines.map((_, idx) => {
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
        DOM['line-numbers'].innerHTML = `<div class="line-number-track">${numbersHtml}</div>`;
        syncPipelineLineNumberScroll();
    }

    function syncPipelineLineNumberScroll() {
        if (!DOM['line-numbers'] || !DOM['pipeline-yaml-editor']) return;
        const offset = DOM['pipeline-yaml-editor'].scrollTop || 0;
        DOM['line-numbers'].style.setProperty('--line-number-scroll', `${offset}px`);
    }

    function updatePipelineEditorHighlight() {
        if (!DOM['pipeline-yaml-highlight'] || !DOM['pipeline-yaml-editor']) return;
        const renderer = global.yaml && typeof global.yaml.renderTokens === 'function'
            ? global.yaml.renderTokens
            : null;
        const stage = DOM['pipeline-yaml-stage'];
        if (!renderer) {
            if (stage) stage.classList.remove('yaml-editor-stage--with-highlight');
            DOM['pipeline-yaml-highlight'].textContent = DOM['pipeline-yaml-editor'].value || '';
            return;
        }
        DOM['pipeline-yaml-highlight'].innerHTML = renderer(DOM['pipeline-yaml-editor'].value || '', escapeHtml) || '&nbsp;';
        if (stage) stage.classList.add('yaml-editor-stage--with-highlight');
        syncPipelineHighlightScroll();
    }

    function syncPipelineHighlightScroll() {
        if (!DOM['pipeline-yaml-highlight'] || !DOM['pipeline-yaml-editor']) return;
        const editor = DOM['pipeline-yaml-editor'];
        const x = editor.scrollLeft || 0;
        const y = editor.scrollTop || 0;
        DOM['pipeline-yaml-highlight'].style.transform = `translate(${-x}px, ${-y}px)`;
    }

    function renderYamlView(yamlString) {
        const target = DOM['pipeline-yaml-content'];
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

    function applyValidationResult(result) {
        const errors = (result && Array.isArray(result.errors)) ? result.errors : [];
        state.editorValidationErrors = errors;
        updateLineNumbers();

        if (!DOM['validation-status']) return;

        if (errors.length) {
            const items = errors.map(err => {
                const lineLabel = err.line ? `<span class="validation-box__line">Line ${err.line}</span>` : '';
                const message = `<div class="validation-box__message">${escapeHtml(err.message)}</div>`;
                const example = buildValidationExample(err.message);
                const exampleHtml = example ? `<pre class="validation-box__example"><code>${escapeHtml(example)}</code></pre>` : '';
                return `<div class="validation-box__item">${lineLabel}${message}${exampleHtml}</div>`;
            }).join('');
            DOM['validation-status'].innerHTML = `<div class="validation-box__header">Validation issues</div>${items}`;
            DOM['validation-status'].className = 'validation-box validation-box--error';
            if (DOM['pipeline-save-btn']) DOM['pipeline-save-btn'].disabled = true;
        } else {
            DOM['validation-status'].innerHTML = '<div class="validation-box__header">All good</div><div class="validation-box__message">Pipeline definition passes validation.</div>';
            DOM['validation-status'].className = 'validation-box validation-box--success';
            if (DOM['pipeline-save-btn']) DOM['pipeline-save-btn'].disabled = false;
        }
    }
    function renderPipelineIncludes(yamlString) {
        const container = DOM['pipeline-includes'];
        if (!container) return;

        const parsed = parseYamlSafely(yamlString);
        if (!parsed || !Array.isArray(parsed.steps) || parsed.steps.length === 0) {
            container.innerHTML = '<p class="text-sm text-[var(--text-secondary)]">No steps defined.</p>';
            return;
        }

        const includes = new Set();
        parsed.steps.forEach(step => {
            if (step && typeof step.include === 'string' && step.include.trim()) {
                includes.add(step.include.trim());
            }
        });

        if (includes.size === 0) {
            container.innerHTML = '<p class="text-sm text-[var(--text-secondary)]">No included dependencies found.</p>';
            return;
        }

        const renderStepLogo = global.NopsAI && global.NopsAI.ui && global.NopsAI.ui.renderStepLogo;
        const buildBadge = (svgMarkup, variant = 'list', logoClass = '') => {
            if (typeof renderStepLogo === 'function') {
                return renderStepLogo(variant, logoClass, svgMarkup);
            }
            return svgMarkup;
        };
        const items = Array.from(includes).sort();
        const html = `<ul class="triggers-pipeline-list">${items.map(item => {
            const isPipeline = item.startsWith('pipeline:');
            const isStep = item.startsWith('step:');
            let identifier = item;
            let icon = buildBadge('<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l4-4 4 4m0 6l-4 4-4-4" /></svg>', 'list', 'step-logo--system');
            let href = '#';
            let title = `Dependency: ${escapeHtml(item)}`;
            
            if (isPipeline) {
                identifier = item.substring('pipeline:'.length);
                href = `#/pipelines/${identifier.split('/').map(encodeURIComponent).join('/')}`;
                title = `Open pipeline ${escapeHtml(identifier)}`;
                icon = buildBadge('<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h10"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 17h10"/><circle cx="7" cy="7" r="1.5" fill="currentColor" stroke="none"/><circle cx="17" cy="7" r="1.5" fill="currentColor" stroke="none"/><circle cx="7" cy="17" r="1.5" fill="currentColor" stroke="none"/><circle cx="17" cy="17" r="1.5" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.5" fill="currentColor" stroke="none"/></svg>', 'list', 'step-logo--pipelines');
            } else if (isStep) {
                identifier = item.substring('step:'.length);
                href = `#/steps/${identifier.split('/').map(encodeURIComponent).join('/')}`;
                title = `Open step ${escapeHtml(identifier)}`;
                icon = buildBadge('<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 10l6-3 6 3-6 3-6-3z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 14l6 3 6-3"/></svg>', 'list', 'step-logo--steps');
            }

            return `
                <li class="triggers-pipeline-item">
                    <a href="${href}" class="triggers-pipeline-link" title="${title}">
                        <div class="flex items-center gap-2">
                            ${icon}
                            <span class="triggers-pipeline-name">${escapeHtml(item)}</span>
                        </div>
                    </a>
                </li>`;
        }).join('')}</ul>`;

        container.innerHTML = html;
    }
    function handleValidation() {
        if (!DOM['pipeline-yaml-editor']) return;
        updatePipelineEditorHighlight();
        const yamlString = DOM['pipeline-yaml-editor'].value;
        const result = validatePipelineYaml(yamlString);
        applyValidationResult(result);
        renderPipelineIncludes(yamlString);
    }

    function ensureEditorSuggestionOverlay() {
        if (DOM['pipeline-editor-autocomplete']) return;
        const overlay = document.createElement('div');
        overlay.id = 'pipeline-editor-autocomplete';
        overlay.className = 'pipeline-editor-autocomplete hidden';
        const ghost = document.createElement('span');
        ghost.className = 'pipeline-editor-autocomplete__ghost';
        overlay.appendChild(ghost);
        document.body.appendChild(overlay);
        DOM['pipeline-editor-autocomplete'] = overlay;
        DOM['pipeline-editor-autocomplete-ghost'] = ghost;

        if (!state.editorSuggestionPositionHandler) {
            const handler = () => {
                updateInlineSuggestionPosition();
            };
            state.editorSuggestionPositionHandler = handler;
            window.addEventListener('scroll', handler, true);
            window.addEventListener('resize', handler, true);
        }
    }

    function hideEditorSuggestions() {
        state.editorSuggestionContext = null;
        state.editorSuggestionItems = [];
        state.editorSuggestionIndex = -1;
        stopEditorSuggestionTracking();
        const overlay = DOM['pipeline-editor-autocomplete'];
        if (overlay) {
            overlay.style.top = '';
            overlay.style.left = '';
            overlay.style.transform = '';
            overlay.classList.add('hidden');
        }
        if (DOM['pipeline-editor-autocomplete-ghost']) {
            DOM['pipeline-editor-autocomplete-ghost'].textContent = '';
        }
    }

    function renderEditorSuggestions(payload) {
        ensureEditorSuggestionOverlay();
        const overlay = DOM['pipeline-editor-autocomplete'];
        const ghostEl = DOM['pipeline-editor-autocomplete-ghost'];
        const textarea = DOM['pipeline-yaml-editor'];
        if (!overlay || !ghostEl || !textarea) return;

        if (payload.loading) {
            hideEditorSuggestions();
            return;
        }

        if (!payload.items || !payload.items.length) {
            hideEditorSuggestions();
            return;
        }

        state.editorSuggestionItems = payload.items.slice();
        state.editorSuggestionIndex = 0;
        const activeItem = state.editorSuggestionItems[0];
        const preview = buildInlineSuggestionPreview(activeItem, state.editorSuggestionContext);
        if (!preview) {
            hideEditorSuggestions();
            return;
        }

        ghostEl.textContent = preview;
        overlay.classList.remove('hidden');
        updateInlineSuggestionPosition();
        startEditorSuggestionTracking();
    }

    function buildInlineSuggestionPreview(item, contextInfo) {
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

    function updateInlineSuggestionPosition() {
        if (!DOM['pipeline-yaml-editor'] || !DOM['pipeline-editor-autocomplete'] || !state.editorSuggestionContext) return;
        const textarea = DOM['pipeline-yaml-editor'];
        const overlay = DOM['pipeline-editor-autocomplete'];
        if (overlay.classList.contains('hidden')) return;

        const caret = calculateCaretOffset(textarea);
        if (!caret) return;

        const textareaRect = textarea.getBoundingClientRect();
        const docLeft = window.scrollX + textareaRect.left + caret.left;
        const docTop = window.scrollY + textareaRect.top + caret.top;

        overlay.style.transform = `translate3d(${docLeft}px, ${docTop}px, 0)`;
    }

    function startEditorSuggestionTracking() {
        if (state.editorSuggestionAnimationFrame != null) {
            return;
        }
        const step = () => {
            state.editorSuggestionAnimationFrame = window.requestAnimationFrame(() => {
                updateInlineSuggestionPosition();
                updateFloatingSuggestionPanelPosition();
                step();
            });
        };
        step();
    }

    function stopEditorSuggestionTracking(force = false) {
        if (!force && state.suggestionPanelFloating) {
            return;
        }
        if (state.editorSuggestionAnimationFrame != null) {
            window.cancelAnimationFrame(state.editorSuggestionAnimationFrame);
            state.editorSuggestionAnimationFrame = null;
        }
    }

    function ensureTextareaCaretMirror() {
        if (textareaCaretMirror && textareaCaretMirror.parentNode) {
            return textareaCaretMirror;
        }
        const mirror = document.createElement('div');
        mirror.className = 'textarea-caret-mirror';
        mirror.style.position = 'absolute';
        mirror.style.visibility = 'hidden';
        mirror.style.whiteSpace = 'pre-wrap';
        mirror.style.wordWrap = 'break-word';
        mirror.style.pointerEvents = 'none';
        mirror.style.top = '0';
        mirror.style.left = '-9999px';
        mirror.style.transform = 'translateX(0)';
        document.body.appendChild(mirror);
        textareaCaretMirror = mirror;
        return mirror;
    }

    function calculateCaretOffset(textarea) {
        if (!textarea) return null;
        const selectionStart = textarea.selectionStart;
        if (typeof selectionStart !== 'number') return null;

        const mirror = ensureTextareaCaretMirror();
        const computed = window.getComputedStyle(textarea);
        const properties = [
            'boxSizing', 'width', 'height', 'overflowX', 'overflowY',
            'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
            'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
            'fontStyle', 'fontVariant', 'fontWeight', 'fontStretch', 'fontSize',
            'fontFamily', 'lineHeight', 'textAlign', 'textTransform', 'textIndent',
            'letterSpacing', 'wordSpacing', 'tabSize'
        ];

        properties.forEach(prop => {
            mirror.style[prop] = computed[prop];
        });

        mirror.style.whiteSpace = textarea.getAttribute('wrap') === 'off' ? 'pre' : 'pre-wrap';
        mirror.style.wordWrap = textarea.getAttribute('wrap') === 'off' ? 'normal' : 'break-word';
        mirror.style.overflow = 'hidden';
        mirror.textContent = textarea.value.slice(0, selectionStart);

        const marker = document.createElement('span');
        marker.textContent = textarea.value.slice(selectionStart, selectionStart + 1) || '\u200b';
        mirror.appendChild(marker);

        const borderLeft = parseFloat(computed.borderLeftWidth) || 0;
        const borderTop = parseFloat(computed.borderTopWidth) || 0;
        const offsetLeft = marker.offsetLeft + borderLeft - (textarea.scrollLeft || 0);
        const offsetTop = marker.offsetTop + borderTop - (textarea.scrollTop || 0);

        mirror.textContent = '';

        return {
            left: offsetLeft,
            top: offsetTop,
        };
    }

    function enableFloatingSuggestionPanel() {
        if (state.suggestionPanelFloating) return;
        const panel = DOM['pipeline-suggestion-panel'];
        if (!panel) return;
        const parent = panel.parentNode;
        if (!parent) return;
        const panelWidth = panel.offsetWidth || 260;
        const nextSibling = panel.nextSibling;
        state.suggestionPanelOriginalParent = parent;
        state.suggestionPanelOriginalNextSibling = nextSibling;
        parent.removeChild(panel);
        const container = document.getElementById('page-content-wrapper') || document.body;
        if (container && container.classList) {
            container.classList.add('pipeline-suggestion-overlay-host');
        }
        container.appendChild(panel);
        panel.classList.add('pipeline-suggestion-overlay');
        panel.dataset.baseWidth = String(panelWidth);
        panel.style.left = '0px';
        panel.style.top = '0px';
        panel.style.transform = '';
        state.suggestionPanelOverlayContainer = container;
        state.suggestionPanelFloating = true;
        updateFloatingSuggestionPanelPosition();
        startEditorSuggestionTracking();
    }

    function disableFloatingSuggestionPanel() {
        if (!state.suggestionPanelFloating) return;
        const panel = DOM['pipeline-suggestion-panel'];
        if (!panel) return;
        panel.classList.remove('pipeline-suggestion-overlay');
        const originalParent = state.suggestionPanelOriginalParent;
        const referenceNode = state.suggestionPanelOriginalNextSibling;
        if (originalParent) {
            if (referenceNode && referenceNode.parentNode === originalParent) {
                originalParent.insertBefore(panel, referenceNode);
            } else {
                originalParent.appendChild(panel);
            }
        }
        state.suggestionPanelOriginalParent = null;
        state.suggestionPanelOriginalNextSibling = null;
        if (state.suggestionPanelOverlayContainer && state.suggestionPanelOverlayContainer.classList) {
            state.suggestionPanelOverlayContainer.classList.remove('pipeline-suggestion-overlay-host');
        }
        state.suggestionPanelOverlayContainer = null;
        state.suggestionPanelFloating = false;
        panel.style.transform = '';
        panel.style.left = '';
        panel.style.top = '';
        panel.style.width = '';
        panel.style.maxHeight = '';
        stopEditorSuggestionTracking(true);
    }

    function updateFloatingSuggestionPanelPosition() {
        if (!state.suggestionPanelFloating) return;
        const panel = DOM['pipeline-suggestion-panel'];
        const textarea = DOM['pipeline-yaml-editor'];
        const container = state.suggestionPanelOverlayContainer || document.getElementById('page-content-wrapper') || document.body;
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

        const actions = DOM['yaml-edit-actions'];
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

    function applyEditorSuggestion(item) {
        if (!state.editorSuggestionContext || !DOM['pipeline-yaml-editor'] || !item) return;
        const contextInfo = state.editorSuggestionContext;
        const textarea = DOM['pipeline-yaml-editor'];
        const textLength = textarea.value.length;
        const rangeStart = Math.max(0, Math.min(contextInfo.rangeStart ?? textarea.selectionStart, textLength));
        const rangeEnd = Math.max(rangeStart, Math.min(contextInfo.rangeEnd ?? textarea.selectionEnd, textLength));
        const before = textarea.value.slice(0, rangeStart);
        const after = textarea.value.slice(rangeEnd);
        let insertText = item.snippet ?? item.value;
        if (typeof insertText !== 'string') {
            insertText = String(insertText ?? '');
        }
        const prefixInsert = contextInfo.insertPrefix || '';
        let suffix = contextInfo.insertSuffix || '';
        if (item.overrideSuffix !== undefined) {
            suffix = item.overrideSuffix;
        } else if (suffix && after.trimStart().startsWith(':')) {
            suffix = '';
        }
        const finalText = prefixInsert + insertText + suffix;
        textarea.value = before + finalText + after;
        const caret = rangeStart + finalText.length;
        textarea.selectionStart = textarea.selectionEnd = caret;
        textarea.focus();
        hideEditorSuggestions();
        handleValidation();
        updateLineNumbers();
        updateEditorSuggestions();
    }

    function insertEditorIndent(target) {
        if (!target || typeof target.value !== 'string') return;
        const start = target.selectionStart ?? 0;
        const end = target.selectionEnd ?? start;
        const value = target.value;
        target.value = value.substring(0, start) + '  ' + value.substring(end);
        const caret = start + 2;
        target.selectionStart = target.selectionEnd = caret;
    }

    function handleEditorEnterKey(event) {
        const textarea = event.target;
        if (!textarea || typeof textarea.value !== 'string') {
            return;
        }

        const start = textarea.selectionStart ?? 0;
        const end = textarea.selectionEnd ?? start;
        if (start === null || end === null) {
            return;
        }

        event.preventDefault();

        const value = textarea.value;
        const lineInfo = getCurrentLineInfo(value, start);
        const before = value.slice(0, start);
        const after = value.slice(end);
        const indentMatch = lineInfo.line.match(/^\s*/);
        const currentIndent = indentMatch ? indentMatch[0] : '';
        const trimmed = lineInfo.line.trim();
        const parentBlock = findParentBlock(value.slice(0, lineInfo.start), ['steps', 'tasks'], lineInfo.indent);
        let newIndent = currentIndent;
        let listPrefix = '';

        if (/^-\s*name\s*:/i.test(trimmed)) {
            newIndent = ' '.repeat(lineInfo.indent + 2);
            listPrefix = '';
        } else if (trimmed.startsWith('-')) {
            newIndent = currentIndent;
            const parent = findParentBlock(value.slice(0, lineInfo.start), ['steps', 'tasks'], lineInfo.indent);
            if (parent && LIST_KEYS_WITH_NAME_TEMPLATE.has(parent)) {
                listPrefix = '- name: ';
            } else {
                listPrefix = '- ';
            }
        } else if (trimmed.endsWith(':')) {
            newIndent = currentIndent + '  ';
            const key = trimmed.slice(0, -1).trim();
            if (LIST_KEYS_WITH_NAME_TEMPLATE.has(key)) {
                listPrefix = '- name: ';
            } else if (LIST_KEYS_SIMPLE.has(key) && !parentBlock) {
                listPrefix = '- ';
            }
        } else {
            if (parentBlock && LIST_KEYS_WITH_NAME_TEMPLATE.has(parentBlock) && trimmed === '') {
                newIndent = ' '.repeat(lineInfo.indent);
                listPrefix = '- name: ';
            } else {
                newIndent = currentIndent;
            }
        }

        const insertion = `\n${newIndent}${listPrefix}`;
        textarea.value = before + insertion + after;
        const caret = before.length + insertion.length;
        textarea.selectionStart = textarea.selectionEnd = caret;
        textarea.focus();
        handleValidation();
        updateLineNumbers();
        updateEditorSuggestions();
    }

    function updateEditorSuggestions() {
        if (!state.isEditing || !DOM['pipeline-yaml-editor']) {
            setPipelineSuggestionPanelMode(null);
            hideEditorSuggestions();
            return;
        }
        ensureEditorSuggestionOverlay();
        const textarea = DOM['pipeline-yaml-editor'];
        const text = textarea.value || '';
        const selectionStart = Math.min(textarea.selectionStart, textarea.selectionEnd);
        const selectionEnd = Math.max(textarea.selectionStart, textarea.selectionEnd);
        state.lastEditorSelection = { start: selectionStart, end: selectionEnd };
        const contextInfo = detectSuggestionContext(text, selectionStart, selectionEnd);
        if (!contextInfo) {
            setPipelineSuggestionPanelMode(null);
            hideEditorSuggestions();
            return;
        }

        if (contextInfo.type === 'environment') {
            setPipelineSuggestionPanelMode('environment');
        } else if (contextInfo.type === 'secrets') {
            setPipelineSuggestionPanelMode('secrets');
        } else if (contextInfo.type === 'include') {
            setPipelineSuggestionPanelMode('include');
        } else if (contextInfo.type === 'pipeline-key' || contextInfo.type === 'step-key' || contextInfo.type === 'task-key') {
            setPipelineSuggestionPanelMode('directive', { directiveType: contextInfo.type });
        } else {
            setPipelineSuggestionPanelMode(null);
        }

        const requiresMetadata = contextInfo.type === 'secrets'
            || contextInfo.type === 'environment'
            || contextInfo.type === 'reusable-step'
            || contextInfo.type === 'include';

        if (requiresMetadata) {
            let poolSize;
            if (contextInfo.type === 'secrets') {
                poolSize = state.autocomplete.secrets.length;
            } else if (contextInfo.type === 'environment') {
                poolSize = state.autocomplete.environments.length;
            } else if (contextInfo.type === 'reusable-step') {
                poolSize = state.autocomplete.reusableSteps.length;
            } else {
                const stepsCount = state.autocomplete.reusableSteps.length;
                const pipelineCount = Array.isArray(state.pipelines) ? state.pipelines.length : 0;
                poolSize = stepsCount + pipelineCount;
            }
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

        const valueContext = detectDirectiveValueContext(lineInfo, selectionEnd);
        if (valueContext) {
            return valueContext;
        }

        const directiveContext = detectDirectiveKeyContext(lineInfo, selectionEnd, beforeLine, text);
        if (directiveContext) {
            return directiveContext;
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

        const local = lineInfo.line.slice(lineInfo.indent);
        const trimmedLocal = local.trimStart();
        if (trimmedLocal.startsWith('-')) {
            const dashMatch = local.match(/^-\s*/);
            const dashSegment = dashMatch ? dashMatch[0] : '-';
            const valueStartLocal = lineInfo.indent + dashSegment.length;
            const relativeText = lineInfo.line.slice(valueStartLocal, lineInfo.column);
            const trimmedValue = relativeText.trim();
            const relativeOffset = trimmedValue ? relativeText.indexOf(trimmedValue) : 0;
            const rangeStart = lineInfo.start + valueStartLocal + relativeOffset;
            const safeRangeEnd = Math.max(rangeStart, selectionEnd);
            return {
                type: 'environment',
                title: 'Environment keys',
                prefix: trimmedValue,
                rangeStart,
                rangeEnd: safeRangeEnd,
                insertSuffix: '',
                insertPrefix: dashSegment.endsWith(' ') ? '' : ' ',
            };
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
            insertPrefix: '',
        };
    }

    function detectDirectiveValueContext(lineInfo, selectionEnd) {
        const rawLine = lineInfo.line;
        if (!rawLine) return null;
        const colonIndex = rawLine.indexOf(':');
        if (colonIndex === -1 || lineInfo.column <= colonIndex) {
            return null;
        }

        const key = rawLine.slice(lineInfo.indent, colonIndex).trim();
        if (!key) {
            return null;
        }

        const afterColon = rawLine.slice(colonIndex + 1);
        const whitespaceMatch = afterColon.match(/^\s*/);
        const whitespace = whitespaceMatch ? whitespaceMatch[0] : '';
        const valueOffsetLocal = colonIndex + 1 + whitespace.length;
        const rawValue = rawLine.slice(valueOffsetLocal);
        const caretColumn = Math.min(lineInfo.column, rawLine.length);
        const caretLocal = Math.max(0, caretColumn - valueOffsetLocal);

        if (key === 'include') {
            if (caretLocal < 0) {
                return null;
            }

            let quoteChar = null;
            let rangeStart = lineInfo.start + valueOffsetLocal;
            let rangeEnd = rangeStart;
            let prefixSegment = rawValue.slice(0, caretLocal);
            let insertPrefix = '';
            let insertSuffix = '';

            if (rawValue.startsWith('"') || rawValue.startsWith("'")) {
                quoteChar = rawValue[0];
                const closingRelative = rawValue.indexOf(quoteChar, 1);
                if (closingRelative !== -1 && caretLocal > closingRelative) {
                    return null;
                }
                const sliceEnd = closingRelative !== -1 ? Math.min(caretLocal, closingRelative) : caretLocal;
                prefixSegment = sliceEnd > 0 ? rawValue.slice(1, sliceEnd) : '';
                rangeStart += 1;
                if (closingRelative !== -1) {
                    rangeEnd = lineInfo.start + valueOffsetLocal + closingRelative;
                } else {
                    rangeEnd = Math.max(rangeStart, lineInfo.start + valueOffsetLocal + caretLocal);
                    insertSuffix = quoteChar;
                }
            } else {
                let tokenLength = 0;
                while (tokenLength < rawValue.length) {
                    const ch = rawValue[tokenLength];
                    if (ch === '#' || ch === ' ' || ch === '\t') {
                        break;
                    }
                    tokenLength++;
                }
                if (tokenLength === 0 && caretLocal > 0) {
                    tokenLength = caretLocal;
                }
                if (caretLocal > tokenLength) {
                    return null;
                }
                rangeEnd = lineInfo.start + valueOffsetLocal + Math.max(tokenLength, caretLocal);
                insertPrefix = '"';
                insertSuffix = '"';
            }

            rangeEnd = Math.max(rangeStart, rangeEnd);
            const prefix = prefixSegment.trim();

            return {
                type: 'include',
                title: 'Include targets',
                prefix,
                rangeStart,
                rangeEnd,
                insertSuffix,
                insertPrefix,
                quoteChar,
            };
        }

        const metadata = DIRECTIVE_VALUE_METADATA[key];
        if (!metadata || !Array.isArray(metadata.values)) {
            return null;
        }

        const currentValueSegment = rawLine.slice(valueOffsetLocal, lineInfo.column);
        const trimmedValue = currentValueSegment.trim();
        const relativeOffset = trimmedValue ? currentValueSegment.indexOf(trimmedValue) : 0;
        const rangeStart = lineInfo.start + valueOffsetLocal + relativeOffset;
        const safeRangeEnd = Math.max(rangeStart, selectionEnd);

        return {
            type: 'directive-value',
            title: metadata.title || 'Value',
            key,
            prefix: trimmedValue,
            rangeStart,
            rangeEnd: safeRangeEnd,
            insertSuffix: '',
        };
    }

    function detectDirectiveKeyContext(lineInfo, selectionEnd, beforeLine, fullText) {
        const rawLine = lineInfo.line;
        if (!rawLine) return null;
        const trimmed = rawLine.trim();
        if (!trimmed || trimmed.startsWith('#')) return null;

        const colonIndex = rawLine.indexOf(':');
        if (colonIndex !== -1 && lineInfo.column > colonIndex) {
            return null;
        }

        let type = 'pipeline-key';
        let rangeStart;
        let rangeEnd;
        let prefix;
        let parent = findParentBlock(beforeLine, ['steps', 'tasks'], lineInfo.indent);

        if (trimmed.startsWith('-')) {
            const dashMatch = rawLine.match(/^(\s*-\s*)/);
            const dashSegment = dashMatch ? dashMatch[0] : '- ';
            const valueStartLocal = dashSegment.length;
            const valueSlice = rawLine.slice(valueStartLocal, lineInfo.column);
            rangeStart = lineInfo.start + valueStartLocal;
            const endIndex = colonIndex !== -1 ? Math.min(colonIndex, lineInfo.column) : lineInfo.column;
            prefix = rawLine.slice(valueStartLocal, endIndex).trim();
            parent = findParentBlock(beforeLine, ['steps', 'tasks'], lineInfo.indent);
            if (parent === 'steps') {
                type = 'step-key';
            } else if (parent === 'tasks') {
                type = 'task-key';
            } else {
                return null;
            }
        } else {
            if (parent === 'steps') {
                type = 'step-key';
            } else if (parent === 'tasks') {
                type = 'task-key';
            } else {
                type = 'pipeline-key';
                if (lineInfo.indent !== 0) {
                    return null;
                }
            }
            rangeStart = lineInfo.start + lineInfo.indent;
            const endIndex = colonIndex !== -1 ? Math.min(colonIndex, lineInfo.column) : lineInfo.column;
            prefix = rawLine.slice(lineInfo.indent, endIndex).trim();
        }

        const colonBound = colonIndex !== -1 ? Math.min(colonIndex, lineInfo.column) : lineInfo.column;
        rangeEnd = Math.max(rangeStart, lineInfo.start + colonBound);

        const insertionColumn = Math.max(0, rangeStart - lineInfo.start);
        const insertIndent = ' '.repeat(insertionColumn);

        const title = type === 'pipeline-key'
            ? 'Pipeline directives'
            : type === 'step-key'
                ? 'Step directives'
                : 'Task directives';

        const blockInfo = collectExistingKeysForContext(fullText, lineInfo, type, parent);
        return {
            type,
            title,
            prefix,
            rangeStart,
            rangeEnd,
            insertSuffix: ': ',
            insertIndent,
            indent: lineInfo.indent,
            existingKeys: blockInfo.keys,
            blockInfo,
            currentKey: prefix,
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
            insertPrefix: dashMatch && /\s$/.test(dashMatch[0]) ? '' : ' ',
        };
    }

    function getLineIndexForOffset(text, offset) {
        if (!text || offset <= 0) return 0;
        let count = 0;
        for (let i = 0; i < offset && i < text.length; i++) {
            if (text[i] === '\n') count++;
        }
        return count;
    }

    function collectExistingKeysForContext(text, lineInfo, type, parentKey) {
        if (typeof text !== 'string') {
            return { keys: new Set(), hasScript: false, hasGoal: false, hasTasks: false };
        }
        const lines = text.split('\n');
        const lineIndex = getLineIndexForOffset(text, lineInfo.start);
        if (type === 'pipeline-key') {
            return collectPipelineLevelKeys(lines);
        }
        if (type === 'step-key' || type === 'task-key') {
            return collectListItemKeys(lines, lineIndex);
        }
        return { keys: new Set() };
    }

    function collectPipelineLevelKeys(lines) {
        const keys = new Set();
        lines.forEach(line => {
            const trimmed = line.trim();
            if (!trimmed || trimmed.startsWith('#') || trimmed.startsWith('-')) return;
            const indent = line.match(/^\s*/)[0].length;
            if (indent !== 0) return;
            const match = trimmed.match(/^([A-Za-z0-9_]+)\s*:/);
            if (match) {
                keys.add(match[1]);
            }
        });
        return { keys };
    }

    function collectListItemKeys(lines, lineIndex) {
        const keys = new Set();
        let hasScript = false;
        let hasGoal = false;
        let hasTasks = false;
        let hasInclude = false;
        let start = lineIndex;
        let baseIndent = null;

        for (let i = lineIndex; i >= 0; i--) {
            const raw = lines[i];
            if (!raw || !raw.trim()) continue;
            const indent = raw.match(/^\s*/)[0].length;
            const trimmed = raw.trim();
            if (trimmed.startsWith('-')) {
                start = i;
                baseIndent = indent;
                break;
            }
            if (indent === 0) break;
        }

        if (baseIndent === null) {
            return { keys, hasScript, hasGoal, hasTasks };
        }

        const listItemIndent = baseIndent;
        for (let j = start; j < lines.length; j++) {
            const raw = lines[j];
            if (raw === undefined) break;
            const trimmed = raw.trim();
            const indent = raw.match(/^\s*/)[0].length;
            if (j !== start) {
                if (!trimmed) continue;
                if (indent < listItemIndent) break;
                if (indent === listItemIndent && trimmed.startsWith('-')) break;
            }
            const relativeIndent = indent - listItemIndent;
            if (relativeIndent > 2) continue;
            const match = trimmed.match(/^-?\s*([A-Za-z0-9_]+)\s*:/);
            if (match) {
                const key = match[1];
                keys.add(key);
                if (key === 'script') hasScript = true;
                if (key === 'goal') hasGoal = true;
                if (key === 'tasks') hasTasks = true;
                if (key === 'include') hasInclude = true;
            }
        }

        return { keys, hasScript, hasGoal, hasTasks, hasInclude };
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
        if (contextInfo.type === 'include') {
            const pool = [];
            if (Array.isArray(state.autocomplete.reusableSteps)) {
                state.autocomplete.reusableSteps.forEach(name => {
                    if (!name) return;
                    const value = `step:${name}`;
                    pool.push({ value, label: value, matchValue: name.toLowerCase() });
                });
            }
            if (Array.isArray(state.pipelines)) {
                state.pipelines.forEach(identifier => {
                    if (!identifier) return;
                    const value = `pipeline:${identifier}`;
                    pool.push({ value, label: value, matchValue: identifier.toLowerCase() });
                });
            }
            return filterSuggestionPool(pool, prefix, 16);
        }
        if (contextInfo.type === 'directive-value') {
            const metadata = DIRECTIVE_VALUE_METADATA[contextInfo.key];
            if (!metadata || !Array.isArray(metadata.values)) {
                return [];
            }
            const values = metadata.values.map(value => ({ value, label: value }));
            return filterSuggestionPool(values, prefix);
        }
        if (contextInfo.type === 'pipeline-key') {
            return filterSuggestionPool(buildDirectiveSuggestionItems(PIPELINE_DIRECTIVES, contextInfo), prefix, 12);
        }
        if (contextInfo.type === 'step-key') {
            return filterSuggestionPool(buildDirectiveSuggestionItems(STEP_DIRECTIVES, contextInfo), prefix, 12);
        }
        if (contextInfo.type === 'task-key') {
            return filterSuggestionPool(buildDirectiveSuggestionItems(TASK_DIRECTIVES, contextInfo), prefix, 12);
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

    function buildDirectiveSuggestionItems(definitions, contextInfo) {
        if (!Array.isArray(definitions)) return [];
        const existingKeys = contextInfo.existingKeys instanceof Set ? contextInfo.existingKeys : new Set(contextInfo.existingKeys || []);
        const blockInfo = contextInfo.blockInfo || {};
        const disallowed = new Set();

        if (contextInfo.type === 'step-key') {
            if (blockInfo.hasInclude) {
                disallowed.add('tasks');
                disallowed.add('goal');
                disallowed.add('script');
            }
            if (blockInfo.hasTasks) {
                disallowed.add('script');
                disallowed.add('goal');
                disallowed.add('include');
            }
            if (blockInfo.hasScript) {
                disallowed.add('tasks');
                disallowed.add('goal');
                disallowed.add('include');
            }
            if (blockInfo.hasGoal) {
                disallowed.add('tasks');
                disallowed.add('script');
                disallowed.add('include');
            }
        } else if (contextInfo.type === 'task-key') {
            if (blockInfo.hasScript) {
                disallowed.add('goal');
            }
            if (blockInfo.hasGoal) {
                disallowed.add('script');
            }
        }

        const prefixLower = (contextInfo.prefix || '').toLowerCase();
        return definitions.map(def => {
            if (disallowed.has(def.key)) {
                return null;
            }
            if (existingKeys.has(def.key)) {
                const keyLower = def.key.toLowerCase();
                if (!(prefixLower && keyLower.startsWith(prefixLower))) {
                    return null;
                }
            }
            return {
                value: def.key,
                label: def.key,
                snippet: def.key,
                hint: def.hint || '',
                overrideSuffix: '',
            };
        }).filter(Boolean);
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

    const ARRAY_KEYS = new Set(['steps', 'tasks', 'environment', 'secrets', 'volumes', 'depends_on', 'artifacts', 'llm_content_ignore']);

    const VALIDATION_EXAMPLES = [
        {
            pattern: /must contain 'include', 'tasks', 'goal', or 'script'/i,
            example: `steps:\n  - name: setup\n    tasks:\n      - name: install\n        script: |\n          echo "Installing"`
        },
        {
            pattern: /is an include step and cannot also contain tasks, goal, or script/i,
            example: `steps:\n  - name: use-template\n    include: "step:path/to/reusable"`
        },
        {
            pattern: /mixes tasks with goal\/script/i,
            example: `steps:\n  - name: lint\n    tasks:\n      - name: run-lint\n        script: |\n          npm run lint`
        },
        {
            pattern: /cannot define both 'goal' and 'script'/i,
            example: `steps:\n  - name: summarize\n    goal: "Summarize the changes"`
        },
        {
            pattern: /unknown field/i,
            example: `steps:\n  - name: build\n    image: node:20\n    tasks:\n      - name: install\n        script: |\n          npm install`
        },
        {
            pattern: /Pipeline name '.*' contains invalid characters/i,
            example: `name: valid-name\nversion: "1.0"`
        },
        {
            pattern: /Duplicate step name/i,
            example: `steps:\n  - name: build\n    tasks: [...]\n  - name: test\n    tasks: [...]`
        },
        {
            pattern: /At least one step is required/i,
            example: `steps:\n  - name: build\n    tasks:\n      - name: compile\n        script: |\n          make`
        },
        {
            pattern: /must define at least one task when using 'tasks'/i,
            example: `steps:\n  - name: build\n    tasks:\n      - name: compile\n        script: |\n          make`
        },
        {
            pattern: /'tasks' but the value is not an array/i,
            example: `steps:\n  - name: build\n    tasks:\n      - name: compile\n        script: |\n          make`
        },
        {
            pattern: /has an empty 'goal'/i,
            example: `steps:\n  - name: summarize\n    goal: "Describe the changes for release notes"`
        },
        {
            pattern: /has an empty 'script'/i,
            example: `steps:\n  - name: build\n    script: |\n      npm run build`
        },
        {
            pattern: /empty 'include'/i,
            example: `steps:\n  - name: reuse\n    include: "step:path/to/reusable"`
        },
        {
            pattern: /must define either 'goal' or 'script'/i,
            example: `steps:\n  - name: lint\n    tasks:\n      - name: run-lint\n        script: |\n          npm run lint`
        }
    ];

    function buildValidationExample(message) {
        if (!message) return '';
        for (const entry of VALIDATION_EXAMPLES) {
            if (entry.pattern.test(message)) {
                return entry.example;
            }
        }
        return '';
    }

    function buildYamlPathIndex(yamlString) {
        const index = new Map();
        if (typeof yamlString !== 'string' || !yamlString.length) {
            return index;
        }
        const lines = yamlString.split('\n');
        const stack = [];

        const pushContext = (indent, path, type) => {
            stack.push({ indent, path, type, nextIndex: 0 });
        };

        const popToIndent = indent => {
            while (stack.length && indent < stack[stack.length - 1].indent) {
                stack.pop();
            }
        };

        lines.forEach((line, idx) => {
            const lineNumber = idx + 1;
            const indentMatch = line.match(/^\s*/);
            const indent = indentMatch ? indentMatch[0].length : 0;
            const trimmed = line.trim();
            popToIndent(indent);
            if (!trimmed || trimmed.startsWith('#')) {
                return;
            }

            if (trimmed.startsWith('-')) {
                const parent = stack[stack.length - 1];
                if (!parent || parent.type !== 'array') {
                    return;
                }
                const itemIndex = parent.nextIndex++;
                const itemPath = `${parent.path}[${itemIndex}]`;
                index.set(itemPath, lineNumber);

                const rest = trimmed.slice(1).trim();
                const keyMatch = rest.match(/^([A-Za-z0-9_]+)\s*:/);
                pushContext(indent + 2, itemPath, 'object');
                if (keyMatch) {
                    const key = keyMatch[1];
                    index.set(`${itemPath}.${key}`, lineNumber);
                    const endsWithColon = rest.endsWith(':');
                    if (endsWithColon) {
                        const isArrayKey = ARRAY_KEYS.has(key);
                        pushContext(indent + 2, `${itemPath}.${key}`, isArrayKey ? 'array' : 'object');
                    }
                }
                return;
            }

            const keyMatch = trimmed.match(/^([A-Za-z0-9_]+)\s*:/);
            if (!keyMatch) {
                return;
            }
            const key = keyMatch[1];
            const parentPath = stack.length ? stack[stack.length - 1].path : '';
            const currentPath = parentPath ? `${parentPath}.${key}` : key;
            index.set(currentPath, lineNumber);

            const endsWithColon = trimmed.endsWith(':');
            if (endsWithColon) {
                const isArrayKey = ARRAY_KEYS.has(key);
                pushContext(indent + 2, currentPath, isArrayKey ? 'array' : 'object');
            }
        });

        return index;
    }

    async function savePipelineChanges() {
        if (!state.selectedId || !DOM['pipeline-yaml-editor']) return;
        const yamlString = DOM['pipeline-yaml-editor'].value;
        const validation = validatePipelineYaml(yamlString);
        applyValidationResult(validation);
        if (validation && Array.isArray(validation.errors) && validation.errors.length) {
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

        state.drafts.delete(state.selectedId);
        setPipelineSource(state.selectedId, 'database');
        updateCachedPipelineSource(state.selectedId);

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
        setPipelineSource(identifier, 'draft');
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
        updateCachedPipelineSource(identifier);
        state.pipelines.push(identifier);
        state.pipelines.sort((a, b) => a.localeCompare(b));
        notifyIncludePanelDataChanged();

        closeNewPipelineModal();
        window.location.hash = buildPipelineHash(identifier, true);
        showToast('Draft pipeline created. Fill in the YAML and save when ready.', 'info');    
    }

    function buildDefaultPipelineYaml(name) {
        return `name: ${name}\nversion: latest\ndescription: Describe what this pipeline does.\ncontainer_image: alpine:3.19\nsteps:\n  - name: build\n    goal: |\n      Replace this with build instructions for ${name}.\n`;
    }

    function generateCloneName(originalName) {
        const baseRaw = (originalName || 'pipeline').trim();
        const sanitizedBase = baseRaw.replace(/[^A-Za-z0-9_.-]/g, '-').replace(/^-+|-+$/g, '') || 'pipeline';
        let candidate = `${sanitizedBase}-copy`;
        let counter = 2;
        while (state.pipelines.includes(candidate)) {
            candidate = `${sanitizedBase}-copy-${counter}`;
            counter += 1;
        }
        return candidate;
    }

    function cloneYamlWithName(originalYaml, newName) {
        let updated = originalYaml;
        if (window.jsyaml && typeof window.jsyaml.load === 'function' && typeof window.jsyaml.dump === 'function') {
            try {
                const parsed = window.jsyaml.load(originalYaml);
                if (parsed && typeof parsed === 'object') {
                    parsed.name = newName;
                    updated = window.jsyaml.dump(parsed, { lineWidth: 120, noRefs: true });
                }
            } catch (error) {
                console.warn('Failed to reserialize cloned pipeline YAML with js-yaml', error);
            }
        }

        if (!/^\s*name\s*:/m.test(updated)) {
            return `name: ${newName}\n${updated}`;
        }
        return updated.replace(/(^\s*name\s*:\s*).*$/m, `$1${newName}`);
    }

    async function openClonePipelineModal(pipelineId) {
        if (!pipelineId || !DOM['pipelines-clone-modal']) return;
        const summary = await ensurePipelineSummary(pipelineId);
        if (!summary || !summary.yaml) {
            showToast('Unable to load pipeline definition for cloning.', 'error');
            return;
        }

        const meta = summary.meta ? { ...summary.meta } : extractMetaFromYaml(summary.yaml, pipelineId);
        state.cloneContext = {
            sourceId: pipelineId,
            yaml: summary.yaml,
            meta,
        };

        if (DOM['pipelines-clone-path']) {
            DOM['pipelines-clone-path'].value = meta.path || '';
        }
        if (DOM['pipelines-clone-name']) {
            DOM['pipelines-clone-name'].value = generateCloneName(meta.name || pipelineId);
        }
        if (DOM['pipelines-clone-subtitle']) {
            DOM['pipelines-clone-subtitle'].textContent = `Cloning from “${meta.name || pipelineId}”. Provide a new path and name.`;
        }

        DOM['pipelines-clone-modal'].classList.remove('hidden');
        requestAnimationFrame(() => DOM['pipelines-clone-modal'].classList.add('show'));
    }

    function closeClonePipelineModal() {
        if (!DOM['pipelines-clone-modal']) return;
        state.cloneContext = null;
        DOM['pipelines-clone-modal'].classList.remove('show');
        setTimeout(() => {
            DOM['pipelines-clone-modal'].classList.add('hidden');
            if (DOM['pipelines-clone-form']) {
                DOM['pipelines-clone-form'].reset();
            }
        }, 200);
    }

    async function handleClonePipeline(event) {
        event.preventDefault();
        if (!state.cloneContext) {
            closeClonePipelineModal();
            return;
        }

        const sourceMeta = state.cloneContext.meta || {};
        const pathRaw = DOM['pipelines-clone-path'].value.trim().replace(/^\/+|\/+$/g, '');
        const name = DOM['pipelines-clone-name'].value.trim();

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

        const clonedYaml = cloneYamlWithName(state.cloneContext.yaml, name);

        state.drafts.add(identifier);
        setPipelineSource(identifier, 'draft');
        const meta = {
            ...sourceMeta,
            name,
            path: pathRaw,
            source: 'Draft',
        };

        state.pipelineCache.set(identifier, {
            yaml: clonedYaml,
            meta,
            isDraft: true,
            fetchedAt: Date.now(),
        });
        updateCachedPipelineSource(identifier);

        state.pipelines.push(identifier);
        state.pipelines.sort((a, b) => a.localeCompare(b));
        notifyIncludePanelDataChanged();

        closeClonePipelineModal();
        window.location.hash = buildPipelineHash(identifier, true);
        showToast('Pipeline cloned. Review the draft and save when ready.', 'info');
    }

    function openDeleteModal(pipelineId) {
        if (isGitManagedPipeline(pipelineId, state.pipelineCache.get(pipelineId)?.meta?.source || '')) {
            showToast('This pipeline is managed via Git. Clone it to customize instead of deleting.', 'info');
            return;
        }
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
        if (isGitManagedPipeline(pipelineId, state.pipelineCache.get(pipelineId)?.meta?.source || '')) {
            showToast('This pipeline is managed via Git and cannot be deleted here.', 'error');
            closeDeleteModal();
            return;
        }

        const isDraft = state.drafts.has(pipelineId);
        if (!isDraft) {
            const url = `/v1/pipelines/${pipelineId.split('/').map(encodeURIComponent).join('/')}`;
            const response = await context.fetchData(url, { method: 'DELETE' });
            const status = typeof context.fetchData?.lastStatus === 'number' ? context.fetchData.lastStatus : null;
            if (response === null && (status === null || status >= 400)) {
                showToast('Failed to delete pipeline.', 'error');
                return;
            }
        }

        state.drafts.delete(pipelineId);
        if (state.pipelineSources instanceof Map) {
            state.pipelineSources.delete(pipelineId);
        }
        state.pipelineCache.delete(pipelineId);
        state.pipelines = state.pipelines.filter(id => id !== pipelineId);
        notifyIncludePanelDataChanged();
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
        if (!window.jsyaml) return { errors: [{ message: 'YAML parser is unavailable.' }] };

        const pathIndex = buildYamlPathIndex(yamlString);

        const knownPipelineKeys = new Set(['name', 'version', 'description', 'container_image', 'display_options', 'working_directory', 'environment', 'steps', 'timeout', 'llm_content_sharing', 'llm_output_sharing', 'llm_content_ignore']);
        const knownStepKeys = new Set(['name', 'include', 'sync', 'image', 'secrets', 'volumes', 'environment', 'tasks', 'condition', 'goal', 'script', 'depends_on', 'ignore_failure', 'llm_output_sharing']);
        const knownTaskKeys = new Set(['name', 'goal', 'script', 'depends_on', 'ignore_failure', 'llm_output_sharing']);
        const knownDisplayOptionsKeys = new Set(['github_view']);

        const createError = (message, pathHints = []) => {
            let line = null;
            for (const hint of pathHints) {
                if (!hint) continue;
                if (hint.startsWith('line:')) {
                    const direct = Number(hint.slice(5));
                    if (!Number.isNaN(direct) && direct > 0) {
                        line = direct;
                        break;
                    }
                    continue;
                }
                const candidate = pathIndex.get(hint);
                if (typeof candidate === 'number') {
                    line = candidate;
                    break;
                }
            }
            return { message, line };
        };

        function findUnknownKeys(obj, knownKeys, path = '') {
            if (!obj || typeof obj !== 'object' || Array.isArray(obj)) {
                return [];
            }
            const unknown = [];
            for (const key in obj) {
                if (!knownKeys.has(key)) {
                    unknown.push({ path: path ? `${path}.${key}` : key, key });
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
            if (!pipeline) return { errors: [createError('YAML is empty or invalid.', [''])] };
            if (typeof pipeline !== 'object') return { errors: [createError('YAML root must be an object.', [''])] };

            const unknownKeys = checkAllKeys(pipeline);
            if (unknownKeys.length > 0) {
                return {
                    errors: unknownKeys.map(item => createError(`Validation Error: Unknown field '${item.key}'.`, [item.path]))
                };
            }
            if (!pipeline.name) return { errors: [createError("Validation Error: 'name' is a required field.", ['name'])] };
            const allowed = /^[a-zA-Z0-9_.-]+$/;
            if (!allowed.test(pipeline.name)) {
                return { errors: [createError(`Validation Error: Pipeline name '${pipeline.name}' contains invalid characters.`, ['name'])] };
            }
            if (pipeline.version && !allowed.test(pipeline.version)) {
                return { errors: [createError(`Validation Error: Pipeline version '${pipeline.version}' contains invalid characters.`, ['version'])] };
            }
            if (!Array.isArray(pipeline.steps) || pipeline.steps.length === 0) {
                return { errors: [createError("Validation Error: At least one step is required in 'steps'.", ['steps'])] };
            }
            const stepNames = new Set();
            const stepTaskMaps = new Map();
            for (let index = 0; index < pipeline.steps.length; index++) {
                const step = pipeline.steps[index];
                const stepPath = `steps[${index}]`;
                if (!step || typeof step !== 'object') {
                    return { errors: [createError('Validation Error: A step is not a valid object.', [stepPath])] };
                }
                if (!step.name) {
                    return { errors: [createError('Validation Error: All steps require a name.', [`${stepPath}.name`, stepPath])] };
                }
                if (stepNames.has(step.name)) {
                    return { errors: [createError(`Validation Error: Duplicate step name '${step.name}' found.`, [`${stepPath}.name`, stepPath])] };
                }
                stepNames.add(step.name);

                const hasIncludeKey = Object.prototype.hasOwnProperty.call(step, 'include');
                const includeValue = hasIncludeKey ? step.include : null;
                const includeValid = typeof includeValue === 'string' && includeValue.trim().length > 0;
                if (hasIncludeKey && !includeValid) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' has an empty 'include' value.`, [`${stepPath}.include`, stepPath])] };
                }
                const isInclude = includeValid;

                const hasTasksKey = Object.prototype.hasOwnProperty.call(step, 'tasks');
                if (hasTasksKey && !Array.isArray(step.tasks)) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' has 'tasks' but the value is not an array.`, [`${stepPath}.tasks`, stepPath])] };
                }
                const hasTasks = Array.isArray(step.tasks) && step.tasks.length > 0;
                if (hasTasksKey && !hasTasks) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' must define at least one task when using 'tasks'.`, [`${stepPath}.tasks`, stepPath])] };
                }

                const hasGoalKey = Object.prototype.hasOwnProperty.call(step, 'goal');
                const goalValue = hasGoalKey ? step.goal : null;
                const hasGoalContent = typeof goalValue === 'string' && goalValue.trim().length > 0;

                const hasScriptKey = Object.prototype.hasOwnProperty.call(step, 'script');
                const scriptValue = hasScriptKey ? step.script : null;
                const hasScriptContent = typeof scriptValue === 'string' && scriptValue.trim().length > 0;

                if (hasGoalKey && !hasGoalContent) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' has an empty 'goal'.`, [`${stepPath}.goal`, stepPath])] };
                }
                if (hasScriptKey && !hasScriptContent) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' has an empty 'script'.`, [`${stepPath}.script`, stepPath])] };
                }

                if (hasGoalKey && hasScriptKey) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' cannot define both 'goal' and 'script'.`, [`${stepPath}.goal`, `${stepPath}.script`, stepPath])] };
                }

                const hasLegacyContent = hasGoalContent || hasScriptContent;

                if (!isInclude && !hasTasks && !hasLegacyContent) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' must contain 'include', 'tasks', 'goal', or 'script'.`, [`${stepPath}.include`, `${stepPath}.tasks`, `${stepPath}.goal`, `${stepPath}.script`, stepPath])] };
                }
                if (isInclude && (hasTasksKey || hasGoalKey || hasScriptKey)) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' is an include step and cannot also contain tasks, goal, or script.`, [`${stepPath}.include`, `${stepPath}.tasks`, `${stepPath}.goal`, `${stepPath}.script`, stepPath])] };
                }
                if (hasTasks && (hasGoalKey || hasScriptKey)) {
                    return { errors: [createError(`Validation Error: Step '${step.name}' mixes tasks with goal/script.`, [`${stepPath}.tasks`, `${stepPath}.goal`, `${stepPath}.script`, stepPath])] };
                }

                if (hasTasks) {
                    const taskNames = new Set();
                    for (let taskIndex = 0; taskIndex < step.tasks.length; taskIndex++) {
                        const task = step.tasks[taskIndex];
                        const taskPath = `${stepPath}.tasks[${taskIndex}]`;
                        if (!task || typeof task !== 'object' || !task.name) {
                            return { errors: [createError(`Validation Error: A task in step '${step.name}' is missing its name.`, [`${taskPath}.name`, taskPath])] };
                        }
                        if (taskNames.has(task.name)) {
                            return { errors: [createError(`Validation Error: Duplicate task name '${task.name}' in step '${step.name}'.`, [`${taskPath}.name`, taskPath])] };
                        }
                        taskNames.add(task.name);

                        const taskHasGoalKey = Object.prototype.hasOwnProperty.call(task, 'goal');
                        const taskGoalValue = taskHasGoalKey ? task.goal : null;
                        const taskHasGoalContent = typeof taskGoalValue === 'string' && taskGoalValue.trim().length > 0;

                        const taskHasScriptKey = Object.prototype.hasOwnProperty.call(task, 'script');
                        const taskScriptValue = taskHasScriptKey ? task.script : null;
                        const taskHasScriptContent = typeof taskScriptValue === 'string' && taskScriptValue.trim().length > 0;

                        if (taskHasGoalKey && taskHasScriptKey) {
                            return { errors: [createError(`Validation Error: Task '${task.name}' in step '${step.name}' cannot define both 'goal' and 'script'.`, [`${taskPath}.goal`, `${taskPath}.script`, taskPath])] };
                        }
                        if (taskHasGoalKey && !taskHasGoalContent) {
                            return { errors: [createError(`Validation Error: Task '${task.name}' in step '${step.name}' has an empty 'goal'.`, [`${taskPath}.goal`, taskPath])] };
                        }
                        if (taskHasScriptKey && !taskHasScriptContent) {
                            return { errors: [createError(`Validation Error: Task '${task.name}' in step '${step.name}' has an empty 'script'.`, [`${taskPath}.script`, taskPath])] };
                        }
                        if (!taskHasGoalContent && !taskHasScriptContent) {
                            return { errors: [createError(`Validation Error: Task '${task.name}' in step '${step.name}' must define either 'goal' or 'script'.`, [`${taskPath}.goal`, `${taskPath}.script`, taskPath])] };
                        }
                    }
                    for (let taskIndex = 0; taskIndex < step.tasks.length; taskIndex++) {
                        const task = step.tasks[taskIndex];
                        if (Array.isArray(task.depends_on)) {
                            for (const dep of task.depends_on) {
                                if (!taskNames.has(dep)) {
                                    const taskPath = `${stepPath}.tasks[${taskIndex}].depends_on`;
                                    return { errors: [createError(`Validation Error: Task '${task.name}' in step '${step.name}' depends on unknown task '${dep}'.`, [taskPath])] };
                                }
                            }
                        }
                    }
                    stepTaskMaps.set(step.name, taskNames);
                }
            }

            for (let index = 0; index < pipeline.steps.length; index++) {
                const step = pipeline.steps[index];
                if (Array.isArray(step.depends_on)) {
                    for (const dep of step.depends_on) {
                        if (!stepNames.has(dep)) {
                            return { errors: [createError(`Validation Error: Step '${step.name}' depends on unknown step '${dep}'.`, [`steps[${index}].depends_on`, `steps[${index}]`])] };
                        }
                    }
                }
            }

            return null;
        } catch (error) {
            if (error && error.mark && typeof error.mark.line === 'number') {
                return { errors: [createError(`YAML Parsing Error: ${error.message}`, [`line:${error.mark.line + 1}`])] };
            }
            return { errors: [createError(`YAML Parsing Error: ${error.message}`)] };
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

        const repos = await context.fetchData('/v1/overrides?include_source=true');
        if (!Array.isArray(repos) || repos.length === 0) {
            state.triggersIndex.set(pipelineId, []);
            return [];
        }

        const matches = [];
        for (const entry of repos) {
            let repoName = '';
            let rawSource = '';
            if (typeof entry === 'string') {
                repoName = entry;
                rawSource = 'database';
            } else if (entry && typeof entry === 'object') {
                repoName = entry.name || entry.repository_name || entry.slug || entry.repo || '';
                rawSource = entry.source || '';
            }
            repoName = String(repoName || '').trim();
            if (!repoName) continue;
            const [owner, name] = repoName.split('/');
            if (!owner || !name) continue;
            const url = `/v1/overrides/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`;
            const yaml = await context.fetchData(url);
            if (typeof yaml !== 'string') continue;
            const manifest = parseYamlSafely(yaml);
            if (!manifest || !Array.isArray(manifest.triggers)) continue;
            const normalizedSource = normalizeTriggerSourceValue(rawSource) || 'database';
            manifest.triggers.forEach(trigger => {
                const pipelineEntries = Array.isArray(trigger.pipelines) ? trigger.pipelines : [];
                pipelineEntries.forEach(entry => {
                    const path = typeof entry === 'string' ? entry : entry?.path;
                    if (normalizePipelineIdentifier(path) === pipelineId) {
                        matches.push({
                            repoOwner: owner,
                            repoName: name,
                            repoSlug: `${owner}/${name}`,
                            source: normalizedSource,
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
